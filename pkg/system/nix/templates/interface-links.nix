{ config, pkgs, lib, ... }:

{
  {{range .LINKS}}
  systemd.network.links."10-{{.NAME}}" = {
    matchConfig.PermanentMACAddress = "{{.MAC}}";
    linkConfig.Name = "{{.NAME}}";
  };
  {{end}}
}
