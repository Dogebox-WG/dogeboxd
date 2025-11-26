{ config, pkgs, lib, ... }:

{
  services.tailscale = {
    enable = {{ .TAILSCALE_ENABLED }};
    {{ if .TAILSCALE_AUTH_KEY }}
    authKeyFile = pkgs.writeText "tailscale-authkey" "{{ .TAILSCALE_AUTH_KEY }}";
    {{ end }}
    {{ if .TAILSCALE_PORT }}
    port = {{ .TAILSCALE_PORT }};
    {{ end }}
  };

  {{ if .TAILSCALE_ENABLED }}
  # Configure tailscale after boot with custom settings
  systemd.services.tailscale-autoconnect = {
    description = "Automatic connection to Tailscale";
    after = [ "network-pre.target" "tailscaled.service" ];
    wants = [ "network-pre.target" "tailscaled.service" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig.Type = "oneshot";
    script = ''
      # Wait for tailscaled to be ready
      sleep 5
      
      # Check current state
      status="$(${pkgs.tailscale}/bin/tailscale status -json 2>/dev/null | ${pkgs.jq}/bin/jq -r '.BackendState' 2>/dev/null || echo "NeedsLogin")"
      
      {{ if or .TAILSCALE_HOSTNAME .TAILSCALE_ADVERTISE_ROUTES .TAILSCALE_TAGS }}
      # Always apply settings (hostname, routes, tags) even if already connected
      echo "Applying Tailscale settings..."
      ${pkgs.tailscale}/bin/tailscale set \
        {{ if .TAILSCALE_HOSTNAME }}--hostname="{{ .TAILSCALE_HOSTNAME }}"{{ end }} \
        {{ if .TAILSCALE_ADVERTISE_ROUTES }}--advertise-routes="{{ .TAILSCALE_ADVERTISE_ROUTES }}"{{ end }} \
        {{ if .TAILSCALE_TAGS }}--advertise-tags="{{ .TAILSCALE_TAGS }}"{{ end }} \
        --accept-dns=true || true
      {{ end }}
      
      # If already running, we're done
      if [ "$status" = "Running" ]; then
        echo "Tailscale is connected"
        exit 0
      fi
      
      # Not connected yet - run tailscale up with auth key
      {{ if .TAILSCALE_AUTH_KEY }}
      echo "Connecting to Tailscale..."
      args=()
      {{ if .TAILSCALE_HOSTNAME }}
      args+=("--hostname={{ .TAILSCALE_HOSTNAME }}")
      {{ end }}
      {{ if .TAILSCALE_ADVERTISE_ROUTES }}
      args+=("--advertise-routes={{ .TAILSCALE_ADVERTISE_ROUTES }}")
      {{ end }}
      {{ if .TAILSCALE_TAGS }}
      args+=("--advertise-tags={{ .TAILSCALE_TAGS }}")
      {{ end }}
      args+=("--authkey={{ .TAILSCALE_AUTH_KEY }}")
      args+=("--accept-dns=true")
      
      ${pkgs.tailscale}/bin/tailscale up "''${args[@]}"
      {{ else }}
      echo "No auth key configured - Tailscale needs manual authentication"
      {{ end }}
    '';
  };
  {{ end }}

  # Allow tailscale UDP port through firewall
  networking.firewall = {
    {{ if .TAILSCALE_ENABLED }}
    # Enable Tailscale interface
    trustedInterfaces = [ "tailscale0" ];
    {{ if .TAILSCALE_PORT }}
    allowedUDPPorts = [ {{ .TAILSCALE_PORT }} ];
    {{ else }}
    allowedUDPPorts = [ 41641 ];
    {{ end }}
    {{ end }}
  };
}

