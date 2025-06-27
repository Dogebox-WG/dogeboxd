{ config, lib, pkgs, ... }:

let
  pupOverlay = self: super: {
    pup = import {{.NIX_FILE}} { inherit pkgs; };
  };

  pupConfig = import {{.NIX_FILE}} { inherit pkgs; };

  pupServices = if lib.hasAttr "services" pupConfig
    then pupConfig.services
    else {};

  pupEnclave = (pupConfig ? pupEnclave) && pupConfig.pupEnclave;
in
{
  # Maybe don't need this here at the top-level, only inside the container block?
  nixpkgs.overlays = lib.mkAfter (
    [ pupOverlay ] ++ lib.optionals pupEnclave [
      (import "/etc/nixos/nix/builders/nanopc-t6/optee/overlay.nix")
    ]
  );

  systemd.services."container-log-forwarder@pup-{{.PUP_ID}}" = {
    description = "Container Log Forwarder for pup-{{.PUP_ID}}";
    after = [ "container@pup-{{.PUP_ID}}.service" ];
    requires = [ "container@pup-{{.PUP_ID}}.service" ];
    serviceConfig = {
      ExecStart = "${pkgs.bash}/bin/bash -c '${pkgs.systemd}/bin/journalctl -M pup-{{.PUP_ID}} -f --no-hostname -o short-iso >> {{.CONTAINER_LOG_DIR}}/pup-{{.PUP_ID}}'";
      Restart = "always";
      User = "root";
      StandardOutput = "null";
      StandardError = "journal";
    };
    wantedBy = [ "multi-user.target" ];
  };

  containers.pup-{{.PUP_ID}} = {

    # If our pup is enabled, we set it to autostart on boot.
    autoStart = {{.PUP_ENABLED}};

    # Set up private networking. This will ensure the pup gets an internal IP
    # in the range of 10.69.0.0/8, be able to to dogeboxd at 10.69.0.1, but not
    # be able to talk to any other pups without proxying through dogeboxd.
    privateNetwork = true;
    hostAddress = "10.69.0.1";
    localAddress = "{{.INTERNAL_IP}}";

    forwardPorts = [
      {{ range .PUP_PORTS }}{{ if .PUBLIC }}{
        containerPort = {{ .PORT }};
        hostPort = {{ .PORT }};
        protocol = "tcp";
      }{{end}}{{end}}
    ];

    # Mount somewhere that can be used as storage for the pup.
    # The rest of the filesystem is marked as readonly (and ephemeral)
    bindMounts = lib.mkMerge [
      {
        "Persistent Storage" = {
          mountPoint = "/storage";
          hostPath   = "{{ .STORAGE_PATH }}";
          isReadOnly = false;
        };
        "PUP" = {
          mountPoint = "/pup";
          hostPath   = "{{ .PUP_PATH }}";
          isReadOnly = true;
        };
      }
      (lib.mkIf pupEnclave {
        "tee0"     = { mountPoint = "/dev/tee0";     hostPath = "/dev/tee0";     isReadOnly = false; };
        "teepriv0" = { mountPoint = "/dev/teepriv0"; hostPath = "/dev/teepriv0"; isReadOnly = false; };
        "usb-bus"  = { mountPoint = "/dev/bus/usb";  hostPath = "/dev/bus/usb";  isReadOnly = false; };
      })
    ];

    allowedDevices = lib.optionals pupEnclave [
      { node = "/dev/tee0";     modifier = "rwm"; }
      { node = "/dev/teepriv0"; modifier = "rwm"; }
      { node = "char-usb_device"; modifier = "rwm"; }
      { node = "char-hidraw";     modifier = "rwm"; }
    ];

    ephemeral = true;

    config = { config, pkgs, lib, ... }: {
      system.stateVersion = "24.11";

      # This needs to be set to false, otherwise the system
      # will try and use channels, which won't work, because
      # we are now using flakes for everything.
      system.copySystemConfiguration = lib.mkForce false;

      nixpkgs.overlays = lib.mkAfter (
        [ pupOverlay ] ++ lib.optionals pupEnclave [
           (import "/etc/nixos/nix/builders/nanopc-t6/optee/overlay.nix")
        ]
      );

      # Mark our root fs as readonly.
      fileSystems."/" = {
        device = "rootfs";
        options = [ "ro" ];
      };

      imports = lib.optionals pupEnclave [
        "/etc/nixos/nix/builders/nanopc-t6/optee/modules/tee-supplicant/default.nix"
      ];

      networking = {
        useHostResolvConf = lib.mkForce false;
        firewall = {
          enable = true;
          # If the pup has marked that is listens on ports
          # explicitly whitelist those in the container fw.
          allowedTCPPorts = [ {{ range .PUP_PORTS }}{{ .PORT }} {{end}}];
        };
        hosts = {
          # Helper so you can always hit dogebox(d) in DNS.
          "10.69.0.1" = [ "dogeboxd" "dogeboxd.local" "dogebox" "dogebox.local" ];
        };
      };

      # Create a group & user for running the pup executable as.
      # Explicitly set IDs so that bind mounts can be chown'd on the host.
      users.groups.pup = {
        gid = 69;
      };

      users.users.pup = {
        uid = 420;
        isSystemUser = true;
        isNormalUser = false;
        group =  "pup";
      };

      environment.systemPackages = with pkgs; (
        [ {{ range .SERVICES }}pup.{{.NAME}} {{end}} ] ++
        lib.optionals pupEnclave [ yubikey-personalization optee-client ]
      );

      # Give pup access to OP-TEE
      systemd.services.fix-device-perms = lib.mkIf pupEnclave {
        description = "Give the pup user access to the OP-TEE device";
        wantedBy    = [ "multi-user.target" ];
        before      = [ "tee-supplicant.service" ];
        serviceConfig = {
          Type      = "oneshot";
          ExecStart = "${pkgs.coreutils}/bin/chown pup:pup /dev/tee0";
        };
      };

      # Merge in any managed nix service that the pup wants to start.
      services = lib.mkMerge [
        pupServices
        {
          resolved.enable = true;
        }
      ];

      # Create a systemd service for any unmanaged binary the pup wants to start.
      {{range .SERVICES}}
      systemd.services.{{.NAME}} = {
        after = [ "network.target" ];
        wantedBy = [ "multi-user.target" ];

        serviceConfig = {
          ExecStart = "${pkgs.pup.{{.NAME}}}{{.EXEC}}";
          Restart = "always";
          User = "pup";
          Group = "pup";

          WorkingDirectory = "{{.CWD}}";

          Environment = [
            {{range .ENV}}
            "{{.KEY}}={{.VAL}}"
            {{end}}
            {{range $.PUP_ENV}}
            "{{.KEY}}={{.VAL}}"
            {{end}}
            {{range $.GLOBAL_ENV}}
            "{{.KEY}}={{.VAL}}"
            {{end}}
          ];

          PrivateTmp = true;
          ProtectSystem = "full";
          ProtectHome = "yes";
          NoNewPrivileges = true;
        };
      };
      {{end}}
    };
  };

  # Add a start condition to this container so it will only start in non-recovery mode.
  systemd.services."container@pup-{{.PUP_ID}}".serviceConfig.ExecCondition = "/run/wrappers/bin/dbx can-pup-start --data-dir {{.DATA_DIR}} --systemd --pup-id {{.PUP_ID}}";
}
