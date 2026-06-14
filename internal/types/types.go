package types

type ResourceStatus struct {
	Exists    bool     `json:"exists"`
	Path      string   `json:"path"`
	Size      int64    `json:"size,omitempty"`
	Count     int      `json:"count,omitempty"`
	Sensitive bool     `json:"sensitive,omitempty"`
	RootKeys  []string `json:"rootKeys,omitempty"`
	Tables    []string `json:"tables,omitempty"`
	Skills    []string `json:"skills,omitempty"`
}

type CodexInspection struct {
	Path   string         `json:"path"`
	Config ResourceStatus `json:"config"`
	Auth   ResourceStatus `json:"auth"`
	Skills ResourceStatus `json:"skills"`
}

type ShareRequest struct {
	CodexDir       string   `json:"codexDir"`
	Port           int      `json:"port"`
	ShareConfig    bool     `json:"shareConfig"`
	ShareAuth      bool     `json:"shareAuth"`
	ShareSkills    bool     `json:"shareSkills"`
	ConfigRootKeys []string `json:"configRootKeys,omitempty"`
	ConfigTables   []string `json:"configTables,omitempty"`
	Skills         []string `json:"skills,omitempty"`
}

type ShareStatus struct {
	Running  bool     `json:"running"`
	Port     int      `json:"port"`
	LocalIPs []string `json:"localIPs"`
	URL      string   `json:"url"`
}

type PeerStatus struct {
	OK      bool   `json:"ok"`
	URL     string `json:"url"`
	Version string `json:"version"`
}

type SyncRequest struct {
	PeerIP         string   `json:"peerIP"`
	Port           int      `json:"port"`
	TargetDir      string   `json:"targetDir"`
	SyncConfig     bool     `json:"syncConfig"`
	SyncAuth       bool     `json:"syncAuth"`
	SyncSkills     bool     `json:"syncSkills"`
	ConfigRootKeys []string `json:"configRootKeys,omitempty"`
	ConfigTables   []string `json:"configTables,omitempty"`
	Skills         []string `json:"skills,omitempty"`
}

type SyncResult struct {
	OK         bool     `json:"ok"`
	Items      []string `json:"items"`
	BackupPath string   `json:"backupPath"`
}

type FileEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type ManifestResource struct {
	Enabled   bool        `json:"enabled"`
	Path      string      `json:"path,omitempty"`
	SHA256    string      `json:"sha256,omitempty"`
	Size      int64       `json:"size,omitempty"`
	Sensitive bool        `json:"sensitive,omitempty"`
	Count     int         `json:"count,omitempty"`
	Files     []FileEntry `json:"files,omitempty"`
	RootKeys  []string    `json:"rootKeys,omitempty"`
	Tables    []string    `json:"tables,omitempty"`
	Skills    []string    `json:"skills,omitempty"`
}

type ManifestShared struct {
	Config ManifestResource `json:"config"`
	Auth   ManifestResource `json:"auth"`
	Skills ManifestResource `json:"skills"`
}

type RemoteManifest struct {
	Schema  int            `json:"schema"`
	App     string         `json:"app"`
	Version string         `json:"version"`
	Shared  ManifestShared `json:"shared"`
}

type HealthResponse struct {
	App     string `json:"app"`
	Version string `json:"version"`
	Port    int    `json:"port"`
}
