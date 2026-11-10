export interface PeerRecord {
  peer_id: string;
  addrs: string[];
  first_seen: string;
  last_seen: string;
  connected: boolean;
  reputation_score?: number;
  blocks_verified?: number;
  hash_mismatches?: number;
  stream_errors?: number;
  avg_latency_ms?: number;
  is_quarantined?: boolean;
  quarantined_until?: string;
}

export interface NodeStatus {
  version: string;
  publisher_key: string;
  data_dir: string;
  storage_quota_bytes: number;
  storage_used_bytes: number;
  total_tracks: number;
  connected_peers?: number;
  direct_peers?: number;
  relay_connected?: boolean;
  bootstrap_seeds?: string[];
  uptime_seconds: number;
  peer_id?: string;
  listen_addrs?: string[];
  is_moderator?: boolean;
}

export interface StorageUsage {
  quota_bytes: number;
  stored_bytes: number;
  reserved_bytes: number;
  available_bytes: number;
  pinned_bytes: number;
  unpinned_bytes: number;
}

export interface NodeSettings {
  storage_quota_bytes: number;
  keeper_enabled: boolean;
  p2p_port: number;
  http_addr: string;
}
