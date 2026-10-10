export interface PeerRecord {
  peer_id: string;
  addrs: string[];
  first_seen: string;
  last_seen: string;
  connected: boolean;
}

export interface NodeStatus {
  version: string;
  publisher_key: string;
  data_dir: string;
  storage_quota_bytes: number;
  storage_used_bytes: number;
  total_tracks: number;
  connected_peers?: number;
  uptime_seconds: number;
}

