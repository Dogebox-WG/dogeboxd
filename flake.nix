{
  inputs = {
    nixpkgs.url     = "github:NixOS/nixpkgs/nixos-25.05";
    flake-utils.url = "github:numtide/flake-utils";

    dpanel-src = {
      url   = "github:dogeorg/dpanel/b35a676cf7e66199013b312d96b79053b17d53c6";
      flake = false;
    };
  };

  outputs = { self, dpanel-src, nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        isLinux = builtins.match ".*-linux$" system != null;
        dogeboxdVendorHash = "sha256-MM9791hcxhcSwLTm0fy81WOxPfM9bXaiTxrBsieN3uA=";
      in {
        devShells.default = if isLinux then
          pkgs.mkShell {
            buildInputs = [
              pkgs.gnumake
              pkgs.systemd.dev
              pkgs.go_1_23
              pkgs.parted
              pkgs.util-linux
              pkgs.e2fsprogs
              pkgs.dosfstools
              pkgs.nixos-install-tools
              pkgs.nix
              pkgs.git
              pkgs.libxkbcommon
              pkgs.rsync
            ];
          }
        else
          pkgs.mkShell {
            shellHook = ''
              echo "🚫 Unsupported system: ${system}"
              echo "Dogeboxd development relies on systemd headers, which are only available on Linux. Please run in a VM."
              exit 1
            '';
          };

        packages = rec {
          dogeboxd = pkgs.buildGoModule {
            name = "dogeboxd";
            src = ./.;

            vendorHash = dogeboxdVendorHash;

            buildPhase = "make";

            installPhase = ''
              mkdir -p $out/dogeboxd/bin
              cp build/* $out/dogeboxd/bin/

              mkdir -p $out/dpanel
              cp -r ${dpanel-src}/. $out/dpanel/
            '';

            nativeBuildInputs = [ pkgs.go_1_23 ];
            buildInputs       = [ pkgs.systemd.dev ];

            meta = with pkgs.lib; {
              description = "Dogebox OS system manager service";
              homepage    = "https://github.com/dogebox-wg/dogeboxd";
              license     = licenses.mit;
              maintainers = with maintainers; [ dogecoinfoundation ];
              platforms   = platforms.linux;
            };
          };

          default = dogeboxd;
        };

        dbxSessionName = "dogeboxd";
        dbxStartCommand = "make dev";
      }
    );
}
