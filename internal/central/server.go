package central

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentpal/internal/authprofile"
	"agentpal/internal/codex"
	"agentpal/internal/constants"
	"agentpal/internal/security"
	"agentpal/internal/types"
)

type Server struct {
	options      types.ServerOptions
	httpServer   *http.Server
	BackupDir    string
	adminAuth    *PortalJWTVerifier
	devices      *deviceAuthorizations
	logins       *pendingLogins
	userProfiles *userProfileStore
	oauthClient  *http.Client
}

func NewServer(options types.ServerOptions) *Server {
	if options.Port == 0 {
		options.Port = constants.DefaultPort
	}
	s := &Server{options: options, devices: newDeviceAuthorizations()}
	s.logins = newPendingLogins()
	userStore := options.UserStoreDir
	if strings.TrimSpace(userStore) == "" {
		userStore = filepath.Join(os.TempDir(), "agentpal-user-profiles")
	}
	s.userProfiles = newUserProfileStore(userStore)
	s.oauthClient = &http.Client{Timeout: 15 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/v1/config/manifest", s.manifest)
	mux.HandleFunc("/v1/config/files/skills/", s.skill)
	mux.HandleFunc("/v1/config/mcp/", s.mcp)
	mux.HandleFunc("/v1/auth-backups", s.backups)
	mux.HandleFunc("/v1/auth-backups/", s.backup)
	mux.HandleFunc("/v1/admin/status", s.adminStatus)
	mux.HandleFunc("/agentpal/v1/admin/status", s.adminStatus)
	mux.HandleFunc("/v1/admin/users", s.adminUsers)
	mux.HandleFunc("/v1/admin/user-profiles", s.adminProfiles)
	mux.HandleFunc("/v1/admin/user-profiles/", s.adminProfiles)
	mux.HandleFunc("/v1/client/device/start", s.deviceStart)
	mux.HandleFunc("/v1/client/device/", s.deviceRoute)
	mux.HandleFunc("/agentpal", s.adminPage)
	mux.HandleFunc("/agentpal/", s.adminPage)
	mux.HandleFunc("/agentpal/authorize", s.authorizePage)
	mux.HandleFunc("/v1/auth/login/start", s.oauthStart)
	mux.HandleFunc("/v1/auth/login/status/", s.oauthStatus)
	mux.HandleFunc("/callback", s.oauthCallback)
	mux.HandleFunc("/v1/auth-profiles", s.authProfiles)
	mux.HandleFunc("/v1/auth-profiles/", s.authProfiles)
	if options.AdminJWTPublic != "" {
		if verifier, err := NewPortalJWTVerifierFromFile(options.AdminJWTPublic, options.AdminJWTIssuer); err == nil {
			s.adminAuth = verifier
		}
	}
	s.httpServer = &http.Server{Addr: ":" + strconv.Itoa(options.Port), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return s
}

func (s *Server) deviceRoute(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/approve") {
		s.deviceApprove(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/client/device/status/") {
		s.deviceStatus(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) authorizePage(w http.ResponseWriter, r *http.Request) {
	requestID := r.URL.Query().Get("request")
	if requestID == "" || s.adminAuth == nil {
		http.Error(w, "authorization request is unavailable", http.StatusBadRequest)
		return
	}
	claims, err := s.adminAuth.VerifySessionRequest(r)
	if err != nil {
		http.Error(w, "Portal login required", http.StatusUnauthorized)
		return
	}
	if err := s.approveDevice(requestID, claims.Subject); err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><meta name=viewport content=width=device-width><title>AgentPal authorized</title><style>body{font:16px system-ui;max-width:600px;margin:60px auto;padding:20px;color:#172033}</style><h1>AgentPal authorized</h1><p>This device can return to the AgentPal application.</p>")
}

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/agentpal" && r.URL.Path != "/agentpal/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, adminPageHTML)
}

const adminPageHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>AgentPal Central node</title>
<style>body{margin:0;background:#eef2f6;color:#172033;font:16px system-ui,sans-serif}main{max-width:980px;margin:auto;padding:40px 20px}.header{display:flex;justify-content:space-between;align-items:center;margin-bottom:24px}.brand{font-size:24px;font-weight:800}.muted{color:#667385}.grid{display:grid;grid-template-columns:repeat(3,1fr);gap:12px}.card{background:#fff;border:1px solid #d9e0e8;border-radius:8px;padding:18px}.card strong{display:block;font-size:22px;margin-top:6px}.card h2{margin:0 0 12px}.card h3{margin:18px 0 8px}.notice{background:#fff4d8;border-radius:8px;padding:14px;margin:16px 0}.error{background:#ffe8e8}label{display:block;margin:8px 0;color:#44506a}input,textarea{width:100%;box-sizing:border-box;padding:8px;border:1px solid #cdd6e0;border-radius:6px;font:inherit}textarea{font-family:ui-monospace,Menlo,monospace;font-size:13px}.row{display:grid;grid-template-columns:repeat(3,1fr);gap:12px;align-items:end}.button-row{margin-top:12px;display:flex;gap:8px}button{cursor:pointer;border:1px solid #cdd6e0;background:#f6f8fb;border-radius:6px;padding:8px 12px;font:inherit}.button-row button.primary{background:#172033;color:#fff;border-color:#172033}.profile-row{display:flex;justify-content:space-between;align-items:center;gap:12px;border-top:1px solid #e6ebf1;padding:10px 0}.profile-row small{display:block;color:#667385}.profile-row button{margin-left:8px}.link{border:none;background:none;color:#1b6fd6;padding:4px 6px}@media(max-width:680px){.grid,.row{grid-template-columns:1fr}.header{align-items:flex-start;gap:12px;flex-direction:column}}</style></head>
<body><main><div class="header"><div><div class="brand">AgentPal</div><div class="muted">Central node administration</div></div><div id="state" class="muted">Loading...</div></div>
 <section id="content"><div class="notice">Loading current node status...</div></section>
 <section class="card" style="margin-top:16px">
  <h2>Administrator token</h2>
  <p class="muted">Paste a Portal administrator JWT to manage reserved profiles. It is kept only in this browser tab.</p>
  <input id="token" type="password" placeholder="Portal admin JWT" autocomplete="off">
  <div class="button-row"><button class="primary" id="useToken">Use token</button></div>
 </section>
 <section class="card" style="margin-top:16px">
  <h2>User reserved profiles</h2>
  <div class="notice">auth.json contains plaintext credentials. This page is intended for trusted LAN administrators only.</div>
  <label>User sub <input id="sub" placeholder="Casdoor user id"></label>
  <div class="button-row"><button id="loadProfiles">Load profiles</button><button id="loadUsers">List users</button></div>
  <div id="subs"></div>
  <div id="profileList"></div>
  <h3>Create or overwrite</h3>
  <div class="row">
   <label>Name <input id="pName" placeholder="yangpu"></label>
   <label>Type <input id="pType" placeholder="api_key"></label>
   <label>Provider <input id="pProvider" placeholder="yangpu"></label>
  </div>
  <label>auth.json <textarea id="pAuth" rows="4" placeholder='{"OPENAI_API_KEY":"sk-..."}'></textarea></label>
  <label>config-auth.toml <textarea id="pConfig" rows="6" placeholder='model_provider = "yangpu"'></textarea></label>
  <div class="button-row"><button class="primary" id="saveProfile">Save</button><button id="deleteProfile">Delete</button></div>
  <div id="profileMessage"></div>
 </section>
 </main>
<script>
const state=document.getElementById('state'), content=document.getElementById('content');
const tokenKey='agentpal-admin-token';
document.getElementById('token').value=sessionStorage.getItem(tokenKey)||'';
function authHeaders(){const token=sessionStorage.getItem(tokenKey)||'';return token?{'Authorization':'Bearer '+token}:{};}
function message(text,isError){const el=document.getElementById('profileMessage');el.className=isError?'notice error':'notice';el.textContent=text;}
function clear(node){while(node.firstChild){node.removeChild(node.firstChild);}}
function make(tag,text,className){const n=document.createElement(tag);if(text!==undefined){n.textContent=text;}if(className){n.className=className;}return n;}
async function request(method,url,body){const options={method:method,headers:Object.assign({},authHeaders())};if(body!==undefined){options.headers['Content-Type']='application/json';options.body=JSON.stringify(body);}const response=await fetch(url,options);if(response.status===204){return null;}const text=await response.text();if(!response.ok){throw new Error(text||response.status);}return text?JSON.parse(text):null;}
function renderSubs(subs){const host=document.getElementById('subs');clear(host);if(!subs.length){host.appendChild(make('span','No users yet.','muted'));return;}subs.forEach(function(sub){const b=make('button',sub,'link');b.onclick=function(){document.getElementById('sub').value=sub;loadProfiles();};host.appendChild(b);});}
function renderProfiles(items){const host=document.getElementById('profileList');clear(host);if(!items||!items.length){host.appendChild(make('div','No reserved profiles for this user.','muted'));return;}items.forEach(function(item){const row=make('div','','profile-row');const info=make('div');info.appendChild(make('strong',item.name));info.appendChild(make('small',item.type+' / '+item.provider));row.appendChild(info);const actions=make('div');const edit=make('button','Edit','secondary');edit.onclick=function(){editProfile(item.name);};const del=make('button','Delete','danger');del.onclick=function(){deleteProfile(item.name);};actions.appendChild(edit);actions.appendChild(del);row.appendChild(actions);host.appendChild(row);});}
async function loadUsers(){try{const subs=await request('GET','v1/admin/users');renderSubs(subs);}catch(error){message(String(error),true);}}
async function loadProfiles(){const sub=document.getElementById('sub').value.trim();if(!sub){message('Enter a user sub first.',true);return;}try{const items=await request('GET','v1/admin/user-profiles/'+encodeURIComponent(sub));renderProfiles(items);message('Loaded '+(items?items.length:0)+' profile(s).');}catch(error){message(String(error),true);}}
async function editProfile(name){const sub=document.getElementById('sub').value.trim();if(!sub){return;}try{const detail=await request('GET','v1/admin/user-profiles/'+encodeURIComponent(sub)+'/'+encodeURIComponent(name));document.getElementById('pName').value=detail.name;document.getElementById('pType').value=detail.type;document.getElementById('pProvider').value=detail.provider;document.getElementById('pAuth').value=JSON.stringify(detail.auth,null,2);document.getElementById('pConfig').value=detail.configAuthToml;message('Loaded '+name+' for editing.');}catch(error){message(String(error),true);}}
async function saveProfile(){const sub=document.getElementById('sub').value.trim();const name=document.getElementById('pName').value.trim();if(!sub||!name){message('User sub and name are required.',true);return;}let auth;try{auth=JSON.parse(document.getElementById('pAuth').value||'{}');}catch(error){message('auth.json is not valid JSON.',true);return;}const body={type:document.getElementById('pType').value.trim(),provider:document.getElementById('pProvider').value.trim(),auth:auth,configAuthToml:document.getElementById('pConfig').value};try{await request('POST','v1/admin/user-profiles/'+encodeURIComponent(sub)+'/'+encodeURIComponent(name),body);message('Saved '+name+'.');loadProfiles();}catch(error){message(String(error),true);}}
async function deleteProfile(name){const sub=document.getElementById('sub').value.trim();if(!sub){message('Enter a user sub first.',true);return;}const target=name||document.getElementById('pName').value.trim();if(!target){message('Enter a profile name first.',true);return;}if(!confirm('Delete reserved profile '+target+' for '+sub+'?')){return;}try{await request('DELETE','v1/admin/user-profiles/'+encodeURIComponent(sub)+'/'+encodeURIComponent(target));message('Deleted '+target+'.');loadProfiles();}catch(error){message(String(error),true);}}
document.getElementById('useToken').onclick=function(){sessionStorage.setItem(tokenKey,document.getElementById('token').value.trim());message('Token stored in this tab.');};
document.getElementById('loadProfiles').onclick=loadProfiles;
document.getElementById('loadUsers').onclick=loadUsers;
document.getElementById('saveProfile').onclick=saveProfile;
document.getElementById('deleteProfile').onclick=function(){deleteProfile();};
 fetch('v1/admin/status',{credentials:'include'}).then(async response=>{if(!response.ok)throw new Error(response.status===401?'Administrator login required':'Central node status unavailable');return response.json()}).then(status=>{state.textContent='Running';content.innerHTML='<div class="grid"><div class="card"><span class="muted">Skills</span><strong>'+status.skills+'</strong></div><div class="card"><span class="muted">MCP servers</span><strong>'+status.mcp+'</strong></div><div class="card"><span class="muted">Listen port</span><strong>'+status.port+'</strong></div></div><div class="card" style="margin-top:12px"><span class="muted">Published Codex directory</span><p>'+status.codexDir+'</p><span class="muted">Last scan</span><p>'+new Date(status.generatedAt).toLocaleString()+'</p></div>'+(status.scanErrors&&status.scanErrors.length?'<div class="notice error"><strong>Scan errors</strong><p>'+status.scanErrors.map(error=>error.path+': '+error.error).join('<br>')+'</p></div>':'')}).catch(error=>{state.textContent='Unavailable';content.innerHTML='<div class="notice error">'+error.message+'</div>'});
 </script></body></html>`

func (s *Server) adminStatus(w http.ResponseWriter, r *http.Request) {
	manifest, err := BuildManifest(s.options.CodexDir, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, types.AdminStatus{
		Running: true, Port: s.options.Port, CodexDir: s.options.CodexDir,
		Skills: len(manifest.Skills), MCP: len(manifest.MCP), GeneratedAt: manifest.GeneratedAt,
		ScanErrors: manifest.Errors,
	})
}
func (s *Server) Handler() http.Handler          { return s.httpServer.Handler }
func (s *Server) Start() error                   { go func() { _ = s.httpServer.ListenAndServe() }(); return nil }
func (s *Server) Stop(ctx context.Context) error { return s.httpServer.Shutdown(ctx) }
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"app": constants.AppName, "version": constants.AppVersion, "role": "server", "port": s.options.Port})
}
func (s *Server) manifest(w http.ResponseWriter, _ *http.Request) {
	m, err := BuildManifest(s.options.CodexDir, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, m)
}
func (s *Server) skill(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/v1/config/files/skills/")
	if err := security.ValidateSkillRelPath(rel); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	m, err := BuildManifest(s.options.CodexDir, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	allowed := false
	for _, skill := range m.Skills {
		for _, file := range skill.Files {
			if file.Path == rel {
				allowed = true
			}
		}
	}
	if !allowed {
		http.NotFound(w, r)
		return
	}
	file, err := security.SafeJoin(s.options.CodexDir, "skills", rel)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.ServeFile(w, r, file)
}
func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	name := path.Base(strings.TrimSuffix(r.URL.Path, "/"))
	entries, err := codex.ReadMCP(path.Join(s.options.CodexDir, "config.toml"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for _, entry := range entries {
		if entry.Name == name {
			w.Header().Set("Content-Type", "application/toml; charset=utf-8")
			_, _ = w.Write(entry.TOML)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) backups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := s.BackupDir
	if root == "" {
		root = filepath.Join(os.TempDir(), "agentpal-auth-backups")
	}
	entries, _ := os.ReadDir(root)
	result := []types.BackupInfo{}
	for _, device := range entries {
		if !device.IsDir() {
			continue
		}
		deviceEntries, _ := os.ReadDir(filepath.Join(root, device.Name()))
		for _, item := range deviceEntries {
			if !item.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, device.Name(), item.Name(), "profile.json"))
			if err != nil {
				continue
			}
			var profile authprofile.AuthProfile
			if json.Unmarshal(data, &profile) != nil {
				continue
			}
			info, _ := item.Info()
			result = append(result, types.BackupInfo{Device: device.Name(), Profile: types.AuthProfile{ID: profile.ID, Name: profile.Name, Type: profile.Type, Provider: profile.Provider, Source: profile.Source}, Size: info.Size(), UpdatedAt: info.ModTime()})
		}
	}
	writeJSON(w, result)
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/auth-backups/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
		http.Error(w, "invalid backup path", http.StatusBadRequest)
		return
	}
	root := s.BackupDir
	if root == "" {
		root = filepath.Join(os.TempDir(), "agentpal-auth-backups")
	}
	target := filepath.Join(root, parts[0], parts[1])
	switch r.Method {
	case http.MethodGet:
		var buf bytes.Buffer
		if err := packDir(target, &buf); err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(buf.Bytes())
	case http.MethodPost:
		if r.URL.Query().Get("overwrite") != "true" {
			if _, err := os.Stat(target); err == nil {
				http.Error(w, "backup exists; use overwrite=true", http.StatusConflict)
				return
			}
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20+1))
		if err != nil || len(body) > 5<<20 {
			http.Error(w, "backup archive is too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := unpackBytes(body, target); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		if err := os.RemoveAll(target); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
func packDir(dir string, w io.Writer) error { return authprofile.Pack(dir, w) }
func unpackBytes(data []byte, target string) error {
	temp, err := os.MkdirTemp(filepath.Dir(target), ".agentpal-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	profile, err := authprofile.Unpack(bytes.NewReader(data), int64(len(data)), temp)
	if err != nil {
		return err
	}
	extracted := filepath.Join(temp, profile.ID)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(extracted, target)
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
