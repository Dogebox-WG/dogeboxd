package nix

import (
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func TestExposeProvidesTCPInterface(t *testing.T) {
	tests := []struct {
		name          string
		expose        dogeboxd.PupManifestExposeConfig
		interfaceName string
		want          bool
	}{
		{
			name: "tcp interface",
			expose: dogeboxd.PupManifestExposeConfig{
				Type:       "tcp",
				Interfaces: []string{"core-rpc"},
			},
			interfaceName: "core-rpc",
			want:          true,
		},
		{
			name: "http interface",
			expose: dogeboxd.PupManifestExposeConfig{
				Type:       "http",
				Interfaces: []string{"indexer-api"},
			},
			interfaceName: "indexer-api",
			want:          true,
		},
		{
			name: "http web ui without interface",
			expose: dogeboxd.PupManifestExposeConfig{
				Type:  "http",
				WebUI: true,
			},
			interfaceName: "indexer-api",
			want:          false,
		},
		{
			name: "unsupported expose type",
			expose: dogeboxd.PupManifestExposeConfig{
				Type:       "udp",
				Interfaces: []string{"indexer-api"},
			},
			interfaceName: "indexer-api",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exposeProvidesTCPInterface(tt.expose, tt.interfaceName)
			if got != tt.want {
				t.Fatalf("exposeProvidesTCPInterface() = %v, want %v", got, tt.want)
			}
		})
	}
}
