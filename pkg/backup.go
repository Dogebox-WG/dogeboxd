package dogeboxd

import "time"

type BackupTarget string

const (
	BackupTargetDownload  BackupTarget = "download"
	BackupTargetRemovable BackupTarget = "removable"
)

type BackupManifest struct {
	Version   int               `json:"version"`
	CreatedAt time.Time         `json:"createdAt"`
	DataDir   string            `json:"dataDir"`
	NixDir    string            `json:"nixDir"`
	Files     []BackupFileEntry `json:"files"`
}

type BackupFileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Sha256 string `json:"sha256"`
}
