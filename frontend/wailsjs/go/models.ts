export namespace types {
	
	export class ResourceStatus {
	    exists: boolean;
	    path: string;
	    size?: number;
	    count?: number;
	    sensitive?: boolean;
	    rootKeys?: string[];
	    tables?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ResourceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exists = source["exists"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.count = source["count"];
	        this.sensitive = source["sensitive"];
	        this.rootKeys = source["rootKeys"];
	        this.tables = source["tables"];
	    }
	}
	export class CodexInspection {
	    path: string;
	    config: ResourceStatus;
	    auth: ResourceStatus;
	    skills: ResourceStatus;
	
	    static createFrom(source: any = {}) {
	        return new CodexInspection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.config = this.convertValues(source["config"], ResourceStatus);
	        this.auth = this.convertValues(source["auth"], ResourceStatus);
	        this.skills = this.convertValues(source["skills"], ResourceStatus);
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
	export class ManifestResource {
	    enabled: boolean;
	    path?: string;
	    sha256?: string;
	    size?: number;
	    sensitive?: boolean;
	    count?: number;
	    files?: FileEntry[];
	    rootKeys?: string[];
	    tables?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ManifestResource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.path = source["path"];
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	        this.sensitive = source["sensitive"];
	        this.count = source["count"];
	        this.files = this.convertValues(source["files"], FileEntry);
	        this.rootKeys = source["rootKeys"];
	        this.tables = source["tables"];
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
	export class ManifestShared {
	    config: ManifestResource;
	    auth: ManifestResource;
	    skills: ManifestResource;
	
	    static createFrom(source: any = {}) {
	        return new ManifestShared(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], ManifestResource);
	        this.auth = this.convertValues(source["auth"], ManifestResource);
	        this.skills = this.convertValues(source["skills"], ManifestResource);
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
	export class PeerStatus {
	    ok: boolean;
	    url: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new PeerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.url = source["url"];
	        this.version = source["version"];
	    }
	}
	export class RemoteManifest {
	    schema: number;
	    app: string;
	    version: string;
	    shared: ManifestShared;
	
	    static createFrom(source: any = {}) {
	        return new RemoteManifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.app = source["app"];
	        this.version = source["version"];
	        this.shared = this.convertValues(source["shared"], ManifestShared);
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
	
	export class ShareRequest {
	    codexDir: string;
	    port: number;
	    shareConfig: boolean;
	    shareAuth: boolean;
	    shareSkills: boolean;
	    configRootKeys?: string[];
	    configTables?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ShareRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codexDir = source["codexDir"];
	        this.port = source["port"];
	        this.shareConfig = source["shareConfig"];
	        this.shareAuth = source["shareAuth"];
	        this.shareSkills = source["shareSkills"];
	        this.configRootKeys = source["configRootKeys"];
	        this.configTables = source["configTables"];
	    }
	}
	export class ShareStatus {
	    running: boolean;
	    port: number;
	    localIPs: string[];
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new ShareStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.localIPs = source["localIPs"];
	        this.url = source["url"];
	    }
	}
	export class SyncRequest {
	    peerIP: string;
	    port: number;
	    targetDir: string;
	    syncConfig: boolean;
	    syncAuth: boolean;
	    syncSkills: boolean;
	    configRootKeys?: string[];
	    configTables?: string[];
	
	    static createFrom(source: any = {}) {
	        return new SyncRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.peerIP = source["peerIP"];
	        this.port = source["port"];
	        this.targetDir = source["targetDir"];
	        this.syncConfig = source["syncConfig"];
	        this.syncAuth = source["syncAuth"];
	        this.syncSkills = source["syncSkills"];
	        this.configRootKeys = source["configRootKeys"];
	        this.configTables = source["configTables"];
	    }
	}
	export class SyncResult {
	    ok: boolean;
	    items: string[];
	    backupPath: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.items = source["items"];
	        this.backupPath = source["backupPath"];
	    }
	}

}

