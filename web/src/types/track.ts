export interface Track {
  content_id: string;
  title: string;
  artist: string;
  description: string;
  duration_ms: number;
  bitrate_kbps: number;
  total_size_bytes: number;
  block_size: number;
  block_count: number;
  publisher_key: string;
  published_at: number;
  license_code: string;
  signature: string;
  pinned: boolean;
  created_at: number;
  availability_pct: number;
}

export interface Manifest {
  schema_version: string;
  content_id: string;
  title: string;
  artist: string;
  duration_ms: number;
  bitrate_kbps: number;
  total_size_bytes: number;
  block_size: number;
  block_hashes: string[];
  publisher_key_hex: string;
  published_at: number;
  license_code: string;
  signature: string;
}

export interface TrackListResponse {
  items: Track[];
  total: number;
  limit: number;
  offset: number;
}
