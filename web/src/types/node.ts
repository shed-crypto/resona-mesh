export interface NodeStatus {
  version: string;
  publisher_key: string;
  data_dir: string;
  storage_quota_bytes: number;
  storage_used_bytes: number;
  total_tracks: number;
  uptime_seconds: number;
}
