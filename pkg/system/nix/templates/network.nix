{ config, pkgs, ... }:

{
  services.create_ap.enable = false; # Disable create_ap in case it was enabled by the T6 Installer.

  networking = {
    {{if .USE_ETHERNET}}
    interfaces = {
      {{.INTERFACE}} = {
        useDHCP = true;
      };
    };
    {{else if .USE_WIRELESS}}
    wireless = {
      enable = true;
      interfaces = [ "{{.INTERFACE}}" ];
      networks = {
        "{{.WIFI_SSID}}" = {
          psk = "{{.WIFI_PASSWORD}}";
        };
      };
    };
    {{end}}
  };
}
