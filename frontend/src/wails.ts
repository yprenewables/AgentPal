export type ResourceStatus = {
  exists: boolean;
  path: string;
  size?: number;
  count?: number;
  sensitive?: boolean;
  rootKeys?: string[];
  tables?: string[];
  skills?: string[];
};

export type CodexInspection = {
  path: string;
  config: ResourceStatus;
  auth: ResourceStatus;
  skills: ResourceStatus;
};

export type ShareStatus = {
  running: boolean;
  port: number;
  localIPs: string[];
  url: string;
};

export type PeerStatus = {
  ok: boolean;
  url: string;
  version: string;
};

export type ManifestResource = {
  enabled: boolean;
  path?: string;
  sha256?: string;
  size?: number;
  sensitive?: boolean;
  count?: number;
  files?: Array<{ path: string; sha256: string; size: number }>;
  rootKeys?: string[];
  tables?: string[];
  skills?: string[];
};

export type RemoteManifest = {
  schema: number;
  app: string;
  version: string;
  shared: {
    config: ManifestResource;
    auth: ManifestResource;
    skills: ManifestResource;
  };
};

export type SyncResult = {
  ok: boolean;
  items: string[];
  backupPath: string;
};

type Backend = {
  App: {
    GetLocalIPs(): Promise<string[]>;
    InspectCodexDir(path: string): Promise<CodexInspection>;
    BrowseFolder(): Promise<string>;
    GetDefaultCodexDir(): Promise<string>;
    GetBackupRoot(): Promise<string>;
    GetBackupExamplePath(): Promise<string>;
    StartSharing(req: unknown): Promise<ShareStatus>;
    StopSharing(): Promise<void>;
    TestConnection(ip: string): Promise<PeerStatus>;
    FetchRemoteManifest(ip: string): Promise<RemoteManifest>;
    SyncFromPeer(req: unknown): Promise<SyncResult>;
    OpenFolder(path: string): Promise<void>;
  };
};

declare global {
  interface Window {
    go?: { main?: Backend };
  }
}

export const api = window.go?.main?.App;
