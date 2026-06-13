# AgentPal

AgentPal is a simple desktop tool for sharing and syncing Codex configuration between trusted machines on the same network.

The first version intentionally keeps the system small:

- Desktop GUI built with Go, Wails, and TypeScript
- macOS and Windows support
- Codex only
- Local network HTTP only, no HTTPS
- Default port: `28888`
- No account system
- No cloud service
- No auto discovery in v1

## MVP Goal

AgentPal lets one user share selected files from their Codex installation directory, while another user connects by IP address and syncs those files into their own Codex installation directory.

The sharing user only needs to:

1. Open AgentPal.
2. Select the Codex directory, defaulting to `~/.codex`.
3. Choose what to share.
4. Start sharing.
5. Tell the other user their IP address.

The receiving user only needs to:

1. Open AgentPal.
2. Enter the sharing user's IP address.
3. Test the connection.
4. Review the available items, selected by default.
5. Select the target Codex directory, defaulting to `~/.codex`.
6. Click Sync.

## Supported Resources

AgentPal v1 supports exactly these Codex resources:

- `config.toml`
- `auth.json`
- `skills/`

`skills/` is synced as a whole directory. v1 does not support selecting individual skills.

`auth.json` is sensitive and must not be selected by default on the sharing side.

## Default Paths

AgentPal uses the user's home directory for defaults.

Codex directory:

```text
~/.codex
```

Codex resources:

```text
~/.codex/config.toml
~/.codex/auth.json
~/.codex/skills/
```

AgentPal state directory:

```text
~/.agentpal
```

AgentPal backups:

```text
~/.agentpal/backups/YYYYMMDD-HHMMSS/
```

## GUI Scope

The MVP has two main pages.

### Share Page

The Share page is used by the machine that provides Codex configuration.

Fields and actions:

- Codex Directory: defaults to `~/.codex`
- Browse button
- Share `config.toml`: selected by default if found
- Share `auth.json`: not selected by default, even if found
- Share `skills`: selected by default if found
- Local IP display
- Port display: `28888`
- Open Sharing button
- Stop Sharing button

Example UI state:

```text
Share Codex Configuration

Codex Directory
[ ~/.codex                         ] [Browse]

Share Items
[x] config.toml    Found
[ ] auth.json      Found, sensitive
[x] skills         Found

Local IP
192.168.1.23

Port
28888

[Open Sharing] [Stop Sharing]
```

When sharing is active, AgentPal shows:

```text
Sharing is running.

Tell the other person to enter this IP:
192.168.1.23
```

### Sync Page

The Sync page is used by the receiving machine.

Fields and actions:

- Peer IP input
- Test Connection button
- Available item checklist
- Target Codex Directory: defaults to `~/.codex`
- Browse button
- Sync button

The receiving side defaults to selecting every item that the sharing side has made available.

Example UI state:

```text
Connect To AgentPal

Peer IP
[ 192.168.1.23                    ]

[Test Connection]

Available Items
[x] config.toml
[x] auth.json      Sensitive
[x] skills         Sync as a whole

Target Codex Directory
[ ~/.codex                         ] [Browse]

[Sync]
```

Before syncing, the UI should clearly state:

```text
Selected items will be backed up and replaced in the target Codex directory.
```

## Connection Model

The user enters only an IP address.

Example input:

```text
192.168.1.23
```

AgentPal internally resolves it to:

```text
http://192.168.1.23:28888
```

The implementation may also accept these forms:

```text
192.168.1.23:28888
http://192.168.1.23:28888
http://192.168.1.23:28888/manifest
```

v1 does not support HTTPS.

## HTTP Protocol

The sharing side runs a local HTTP server on:

```text
0.0.0.0:28888
```

Endpoints:

```text
GET /health
GET /manifest
GET /files/config.toml
GET /files/auth.json
GET /files/skills/<relative-path>
```

### Health Response

```json
{
  "app": "AgentPal",
  "version": "0.1.0",
  "port": 28888
}
```

### Manifest Response

```json
{
  "schema": 1,
  "app": "AgentPal",
  "version": "0.1.0",
  "shared": {
    "config": {
      "enabled": true,
      "path": "config.toml",
      "sha256": "...",
      "size": 1234
    },
    "auth": {
      "enabled": false,
      "path": "auth.json",
      "sensitive": true
    },
    "skills": {
      "enabled": true,
      "count": 8,
      "files": [
        {
          "path": "translation/SKILL.md",
          "sha256": "...",
          "size": 1000
        }
      ]
    }
  }
}
```

## Sync Behavior

The receiving side writes selected resources into the target Codex directory.

Mapping:

```text
remote config.toml -> <target_codex_dir>/config.toml
remote auth.json   -> <target_codex_dir>/auth.json
remote skills/**   -> <target_codex_dir>/skills/**
```

Sync flow:

1. Test the peer connection.
2. Fetch `/manifest`.
3. Let the user confirm selected resources.
4. Download selected files into a temporary directory.
5. Verify each downloaded file with `sha256`.
6. Back up existing local files or directories that will be replaced.
7. Apply files to the target Codex directory.
8. Write sync history.
9. Show the result.

## Backup Rules

v1 always backs up before overwriting.

If selected and target exists:

- `config.toml` is backed up, then overwritten.
- `auth.json` is backed up, then overwritten.
- `skills/` is backed up as a whole directory, then replaced.

Example backup layout:

```text
~/.agentpal/backups/20260611-153000/
  config.toml
  auth.json
  skills/
```

Because `skills/` is replaced as a whole, local-only skills in the target directory will be removed from the active Codex directory after sync. They remain available in the backup.

## Security Rules

AgentPal v1 is intended for trusted local networks only.

Required safety rules:

- Share only explicitly selected resources.
- Never select `auth.json` by default on the sharing side.
- Warn that `auth.json` may contain credentials.
- Reject skill paths containing `../`.
- Reject absolute skill paths.
- Reject Windows drive paths such as `C:\...`.
- Never read outside the selected sharing Codex directory.
- Never write outside the selected target Codex directory.
- Ignore `.git`.
- Ignore `.DS_Store`.
- Ignore `__pycache__`.
- Verify every downloaded file with `sha256` before applying it.
- Download to a temporary directory first, then apply after verification succeeds.

## Local State

AgentPal stores simple local state in `~/.agentpal`.

Example `config.json`:

```json
{
  "codex_dir": "~/.codex",
  "last_peer_ip": "192.168.1.23",
  "port": 28888
}
```

Example `history.json`:

```json
[
  {
    "time": "2026-06-11T15:30:00Z",
    "peer": "192.168.1.23:28888",
    "target": "~/.codex",
    "items": ["config.toml", "skills"],
    "backup": "~/.agentpal/backups/20260611-153000"
  }
]
```

v1 does not need a complex lock file.

## Wails Backend API Draft

The Go backend exposes methods to the TypeScript frontend.

```go
func (a *App) GetLocalIPs() ([]string, error)
func (a *App) InspectCodexDir(path string) (CodexInspection, error)
func (a *App) BrowseFolder() (string, error)

func (a *App) StartSharing(req ShareRequest) (ShareStatus, error)
func (a *App) StopSharing() error

func (a *App) TestConnection(ip string) (PeerStatus, error)
func (a *App) FetchRemoteManifest(ip string) (RemoteManifest, error)
func (a *App) SyncFromPeer(req SyncRequest) (SyncResult, error)

func (a *App) OpenFolder(path string) error
```

Request structs:

```go
type ShareRequest struct {
    CodexDir    string
    Port        int
    ShareConfig bool
    ShareAuth   bool
    ShareSkills bool
}

type SyncRequest struct {
    PeerIP     string
    Port       int
    TargetDir  string
    SyncConfig bool
    SyncAuth   bool
    SyncSkills bool
}
```

## Suggested Project Structure

```text
AgentPal/
  README.md
  go.mod
  main.go
  app.go
  wails.json

  internal/
    codex/
      inspect.go
      paths.go
    config/
      config.go
    peer/
      client.go
      normalize.go
    platform/
      ips.go
    security/
      paths.go
    share/
      server.go
      manifest.go
      files.go
    sync/
      download.go
      verify.go
      backup.go
      apply.go

  frontend/
    package.json
    src/
      App.tsx
      pages/
        Share.tsx
        Sync.tsx
      components/
        Notice.tsx
        ResourceChecklist.tsx
```

## Out Of Scope For v1

- HTTPS
- Login or account system
- Cloud service
- Automatic peer discovery
- OpenCode support
- Single skill selection
- Config merge
- Diff UI
- Rollback UI
- Two-way sync
- Remote delete sync
- Public internet deployment
