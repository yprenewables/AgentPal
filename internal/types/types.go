package types

import "time"

type Bootstrap struct {
	Mode            string `json:"mode"`
	CodexDir        string `json:"codexDir"`
	CentralHost     string `json:"centralHost"`
	CentralPort     int    `json:"centralPort"`
	Port            int    `json:"port"`
	DeviceName      string `json:"deviceName"`
	BackupDir       string `json:"backupDir"`
	Version         string `json:"version"`
	RestartRequired bool   `json:"restartRequired"`
}

type SettingsRequest struct {
	Mode        string `json:"mode"`
	CodexDir    string `json:"codexDir"`
	CentralHost string `json:"centralHost"`
	CentralPort int    `json:"centralPort"`
	Port        int    `json:"port"`
	DeviceName  string `json:"deviceName"`
	BackupDir   string `json:"backupDir"`
}

type ServerOptions struct {
	CodexDir          string `json:"codexDir"`
	Port              int    `json:"port"`
	AdminJWTIssuer    string `json:"adminJWTIssuer,omitempty"`
	AdminJWTPublic    string `json:"adminJWTPublic,omitempty"`
	PortalLoginURL    string `json:"portalLoginURL,omitempty"`
	AllowTestAuth     bool   `json:"allowTestAuth,omitempty"`
	CasdoorURL        string `json:"casdoorURL,omitempty"`
	CasdoorClientID   string `json:"casdoorClientID,omitempty"`
	CasdoorScopes     string `json:"casdoorScopes,omitempty"`
	CasdoorAdminClaim string `json:"casdoorAdminClaim,omitempty"`
	OAuthRedirectURL  string `json:"oauthRedirectURL,omitempty"`
	UserStoreDir      string `json:"userStoreDir,omitempty"`
	AdminSubs         string `json:"adminSubs,omitempty"`
}

type AdminStatus struct {
	Running     bool        `json:"running"`
	Port        int         `json:"port"`
	CodexDir    string      `json:"codexDir"`
	Skills      int         `json:"skills"`
	MCP         int         `json:"mcp"`
	GeneratedAt time.Time   `json:"generatedAt"`
	ScanErrors  []ScanError `json:"scanErrors,omitempty"`
}

type CenterManifest struct {
	Schema      int           `json:"schema"`
	App         string        `json:"app"`
	Version     string        `json:"version"`
	GeneratedAt time.Time     `json:"generatedAt"`
	Skills      []CenterSkill `json:"skills"`
	MCP         []CenterMCP   `json:"mcp"`
	Errors      []ScanError   `json:"errors,omitempty"`
}
type CenterSkill struct {
	Name   string      `json:"name"`
	SHA256 string      `json:"sha256"`
	Files  []FileEntry `json:"files"`
	Size   int64       `json:"size"`
}
type CenterMCP struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type ScanError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

const (
	DiffSame      = "same"
	DiffNew       = "new"
	DiffChanged   = "changed"
	DiffLocalOnly = "local_only"
)

type DiffItem struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Selected bool   `json:"selected"`
}
type ConfigDiff struct {
	Items []DiffItem `json:"items"`
}
type ApplyChange struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Action string `json:"action"`
}
type ApplyRequest struct {
	CodexDir    string        `json:"codexDir"`
	CentralHost string        `json:"centralHost"`
	CentralPort int           `json:"centralPort"`
	Changes     []ApplyChange `json:"changes"`
}
type ApplyResult struct {
	OK         bool     `json:"ok"`
	BackupPath string   `json:"backupPath"`
	RolledBack bool     `json:"rolledBack"`
	Applied    []string `json:"applied"`
}
type ServerStatus struct {
	Running  bool   `json:"running"`
	Port     int    `json:"port"`
	CodexDir string `json:"codexDir"`
	Skills   int    `json:"skills"`
	MCP      int    `json:"mcp"`
	Error    string `json:"error,omitempty"`
}
type ConfigRefresh struct {
	Connected bool           `json:"connected"`
	Error     string         `json:"error,omitempty"`
	Manifest  CenterManifest `json:"manifest"`
	Diff      ConfigDiff     `json:"diff"`
}
type ProfileSwitchResult struct {
	OK                  bool        `json:"ok"`
	Profile             AuthProfile `json:"profile"`
	BackupPath          string      `json:"backupPath"`
	ProviderConflict    bool        `json:"providerConflict"`
	ConflictDescription string      `json:"conflictDescription,omitempty"`
}
type ProfileSwitchPreview struct {
	Profile             AuthProfile `json:"profile"`
	Provider            string      `json:"provider"`
	ProviderConflict    bool        `json:"providerConflict"`
	ConflictDescription string      `json:"conflictDescription,omitempty"`
}
type AuthProfile struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Provider         string `json:"provider"`
	Source           string `json:"source"`
	Active           bool   `json:"active"`
	APIKeyPreview    string `json:"apiKeyPreview,omitempty"`
	BaseURL          string `json:"baseUrl,omitempty"`
	ChatGPTUserID    string `json:"chatgptUserId,omitempty"`
	ChatGPTExpiresAt string `json:"chatgptExpiresAt,omitempty"`
	CurrentProfileID string `json:"currentProfileId,omitempty"`
	ReservedName     string `json:"reservedName,omitempty"`
	ContentHash      string `json:"contentHash,omitempty"`
}
type CurrentAuthStatus struct {
	APIKeyPreview    string `json:"apiKeyPreview,omitempty"`
	BaseURL          string `json:"baseUrl,omitempty"`
	ChatGPTUserID    string `json:"chatgptUserId,omitempty"`
	ChatGPTExpiresAt string `json:"chatgptExpiresAt,omitempty"`
	APIKeyImported   bool   `json:"apiKeyImported"`
	ChatGPTImported  bool   `json:"chatgptImported"`
	CurrentProfileID string `json:"currentProfileId,omitempty"`
}
type APIKeyProfileRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
}
type BackupInfo struct {
	Device    string      `json:"device"`
	Profile   AuthProfile `json:"profile"`
	Size      int64       `json:"size"`
	UpdatedAt time.Time   `json:"updatedAt"`
}
type RemoteBackup struct {
	Device    string      `json:"device"`
	Profile   AuthProfile `json:"profile"`
	Size      int64       `json:"size"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

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

type FileEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type UpdateAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type UpdateInfo struct {
	CurrentVersion string        `json:"currentVersion"`
	LatestVersion  string        `json:"latestVersion"`
	HasUpdate      bool          `json:"hasUpdate"`
	ReleaseURL     string        `json:"releaseUrl"`
	Assets         []UpdateAsset `json:"assets"`
}

type RemoteAuthProfile struct {
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	Type        string    `json:"type"`
	ContentHash string    `json:"contentHash"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type RemoteAuthProfileDetail struct {
	Name           string         `json:"name"`
	Provider       string         `json:"provider"`
	Type           string         `json:"type"`
	ContentHash    string         `json:"contentHash"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	Auth           map[string]any `json:"auth"`
	ConfigAuthToml string         `json:"configAuthToml"`
}

type LoginStart struct {
	LoginID      string    `json:"loginId"`
	PollToken    string    `json:"pollToken"`
	AuthorizeURL string    `json:"authorizeUrl"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type LoginStatus struct {
	Status      string `json:"status"`
	Subject     string `json:"subject,omitempty"`
	AccessToken string `json:"accessToken,omitempty"`
	ExpiresIn   int    `json:"expiresIn,omitempty"`
	Error       string `json:"error,omitempty"`
}

type CentralLoginState struct {
	LoggedIn  bool      `json:"loggedIn"`
	Subject   string    `json:"subject,omitempty"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

type AuthSyncItem struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type AuthSyncReport struct {
	Items []AuthSyncItem `json:"items"`
}
