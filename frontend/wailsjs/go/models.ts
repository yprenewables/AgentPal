export namespace types {
	
	export class APIKeyProfileRequest {
	    name: string;
	    provider: string;
	    baseUrl: string;
	    apiKey: string;
	
	    static createFrom(source: any = {}) {
	        return new APIKeyProfileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	    }
	}
	export class ApplyChange {
	    kind: string;
	    name: string;
	    action: string;
	
	    static createFrom(source: any = {}) {
	        return new ApplyChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.action = source["action"];
	    }
	}
	export class ApplyRequest {
	    codexDir: string;
	    centralHost: string;
	    centralPort: number;
	    changes: ApplyChange[];
	
	    static createFrom(source: any = {}) {
	        return new ApplyRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codexDir = source["codexDir"];
	        this.centralHost = source["centralHost"];
	        this.centralPort = source["centralPort"];
	        this.changes = this.convertValues(source["changes"], ApplyChange);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ApplyResult {
	    ok: boolean;
	    backupPath: string;
	    rolledBack: boolean;
	    applied: string[];
	
	    static createFrom(source: any = {}) {
	        return new ApplyResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.backupPath = source["backupPath"];
	        this.rolledBack = source["rolledBack"];
	        this.applied = source["applied"];
	    }
	}
	export class AuthProfile {
	    id: string;
	    name: string;
	    type: string;
	    provider: string;
	    source: string;
	    active: boolean;
	    apiKeyPreview?: string;
	    baseUrl?: string;
	    chatgptUserId?: string;
	    chatgptExpiresAt?: string;
	    currentProfileId?: string;
	    reservedName?: string;
	    contentHash?: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.provider = source["provider"];
	        this.source = source["source"];
	        this.active = source["active"];
	        this.apiKeyPreview = source["apiKeyPreview"];
	        this.baseUrl = source["baseUrl"];
	        this.chatgptUserId = source["chatgptUserId"];
	        this.chatgptExpiresAt = source["chatgptExpiresAt"];
	        this.currentProfileId = source["currentProfileId"];
	        this.reservedName = source["reservedName"];
	        this.contentHash = source["contentHash"];
	    }
	}
	export class AuthSyncItem {
	    name: string;
	    status: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthSyncItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	    }
	}
	export class AuthSyncReport {
	    items: AuthSyncItem[];
	
	    static createFrom(source: any = {}) {
	        return new AuthSyncReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], AuthSyncItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BackupInfo {
	    device: string;
	    profile: AuthProfile;
	    size: number;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new BackupInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device = source["device"];
	        this.profile = this.convertValues(source["profile"], AuthProfile);
	        this.size = source["size"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Bootstrap {
	    mode: string;
	    codexDir: string;
	    centralHost: string;
	    centralPort: number;
	    port: number;
	    deviceName: string;
	    backupDir: string;
	    version: string;
	    restartRequired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Bootstrap(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.codexDir = source["codexDir"];
	        this.centralHost = source["centralHost"];
	        this.centralPort = source["centralPort"];
	        this.port = source["port"];
	        this.deviceName = source["deviceName"];
	        this.backupDir = source["backupDir"];
	        this.version = source["version"];
	        this.restartRequired = source["restartRequired"];
	    }
	}
	export class CenterMCP {
	    name: string;
	    sha256: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new CenterMCP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	    }
	}
	export class ScanError {
	    path: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.error = source["error"];
	    }
	}
	export class FileEntry {
	    path: string;
	    sha256: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	    }
	}
	export class CenterSkill {
	    name: string;
	    sha256: string;
	    files: FileEntry[];
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new CenterSkill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.sha256 = source["sha256"];
	        this.files = this.convertValues(source["files"], FileEntry);
	        this.size = source["size"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CenterManifest {
	    schema: number;
	    app: string;
	    version: string;
	    // Go type: time
	    generatedAt: any;
	    skills: CenterSkill[];
	    mcp: CenterMCP[];
	    errors?: ScanError[];
	
	    static createFrom(source: any = {}) {
	        return new CenterManifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.app = source["app"];
	        this.version = source["version"];
	        this.generatedAt = this.convertValues(source["generatedAt"], null);
	        this.skills = this.convertValues(source["skills"], CenterSkill);
	        this.mcp = this.convertValues(source["mcp"], CenterMCP);
	        this.errors = this.convertValues(source["errors"], ScanError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class CentralLoginState {
	    loggedIn: boolean;
	    subject?: string;
	    // Go type: time
	    expiresAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new CentralLoginState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loggedIn = source["loggedIn"];
	        this.subject = source["subject"];
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DiffItem {
	    kind: string;
	    name: string;
	    status: string;
	    selected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiffItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.selected = source["selected"];
	    }
	}
	export class ConfigDiff {
	    items: DiffItem[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], DiffItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConfigRefresh {
	    connected: boolean;
	    error?: string;
	    manifest: CenterManifest;
	    diff: ConfigDiff;
	
	    static createFrom(source: any = {}) {
	        return new ConfigRefresh(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.error = source["error"];
	        this.manifest = this.convertValues(source["manifest"], CenterManifest);
	        this.diff = this.convertValues(source["diff"], ConfigDiff);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CurrentAuthStatus {
	    apiKeyPreview?: string;
	    baseUrl?: string;
	    chatgptUserId?: string;
	    chatgptExpiresAt?: string;
	    apiKeyImported: boolean;
	    chatgptImported: boolean;
	    currentProfileId?: string;
	
	    static createFrom(source: any = {}) {
	        return new CurrentAuthStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiKeyPreview = source["apiKeyPreview"];
	        this.baseUrl = source["baseUrl"];
	        this.chatgptUserId = source["chatgptUserId"];
	        this.chatgptExpiresAt = source["chatgptExpiresAt"];
	        this.apiKeyImported = source["apiKeyImported"];
	        this.chatgptImported = source["chatgptImported"];
	        this.currentProfileId = source["currentProfileId"];
	    }
	}
	
	
	export class LoginStart {
	    loginId: string;
	    pollToken: string;
	    authorizeUrl: string;
	    // Go type: time
	    expiresAt: any;
	
	    static createFrom(source: any = {}) {
	        return new LoginStart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loginId = source["loginId"];
	        this.pollToken = source["pollToken"];
	        this.authorizeUrl = source["authorizeUrl"];
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LoginStatus {
	    status: string;
	    subject?: string;
	    accessToken?: string;
	    expiresIn?: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new LoginStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.subject = source["subject"];
	        this.accessToken = source["accessToken"];
	        this.expiresIn = source["expiresIn"];
	        this.error = source["error"];
	    }
	}
	export class ProfileSwitchPreview {
	    profile: AuthProfile;
	    provider: string;
	    providerConflict: boolean;
	    conflictDescription?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileSwitchPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], AuthProfile);
	        this.provider = source["provider"];
	        this.providerConflict = source["providerConflict"];
	        this.conflictDescription = source["conflictDescription"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProfileSwitchResult {
	    ok: boolean;
	    profile: AuthProfile;
	    backupPath: string;
	    providerConflict: boolean;
	    conflictDescription?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileSwitchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.profile = this.convertValues(source["profile"], AuthProfile);
	        this.backupPath = source["backupPath"];
	        this.providerConflict = source["providerConflict"];
	        this.conflictDescription = source["conflictDescription"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SettingsRequest {
	    mode: string;
	    codexDir: string;
	    centralHost: string;
	    centralPort: number;
	    port: number;
	    deviceName: string;
	    backupDir: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.codexDir = source["codexDir"];
	        this.centralHost = source["centralHost"];
	        this.centralPort = source["centralPort"];
	        this.port = source["port"];
	        this.deviceName = source["deviceName"];
	        this.backupDir = source["backupDir"];
	    }
	}
	export class UpdateAsset {
	    name: string;
	    url: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new UpdateAsset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	        this.size = source["size"];
	    }
	}
	export class UpdateInfo {
	    currentVersion: string;
	    latestVersion: string;
	    hasUpdate: boolean;
	    releaseUrl: string;
	    assets: UpdateAsset[];
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.hasUpdate = source["hasUpdate"];
	        this.releaseUrl = source["releaseUrl"];
	        this.assets = this.convertValues(source["assets"], UpdateAsset);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

