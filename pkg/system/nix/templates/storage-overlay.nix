{ pkgs, ... }:

{
  # Ideally we'd use nix .fileSystems.<name> here, but it doesn't seem to work?

  systemd.services.mount-data-overlay = {
    description = "Mounts the selected storage device as an overlay at {{.DATA_DIR}}";
    before = [ "dogeboxd.service" ];
    wantedBy = [ "local-fs.target" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = "yes";
    };
    script = ''
      if ! ${pkgs.util-linux}/bin/mountpoint -q {{ .DATA_DIR }}; then
        ${pkgs.util-linux}/bin/mount {{ .STORAGE_DEVICE }} {{ .DATA_DIR }}
        ${pkgs.coreutils}/bin/chown {{.DBX_UID}}:{{.DBX_UID}} {{.DATA_DIR}}
        ${pkgs.coreutils}/bin/chmod u+rwX,g+rwX,o-rwx {{ .DATA_DIR }}
      else
        echo "{{ .DATA_DIR }} is already mounted, skipping mount operation"
      fi
    '';
  };
}
