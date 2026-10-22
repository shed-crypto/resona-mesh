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
  blocked?: boolean;
  block_reason?: string;
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

export interface Report {
  event_id: string;
  content_id: string;
  payload_hash: string;
  reporter_key: string;
  reason_code: string;
  description: string;
  evidence_ref: string;
  created_at: number;
  nonce: string;
  signature: string;
  status: string;
  receipt_signature: string;
  receipt_received_at: number;
}

export interface Decision {
  event_id: string;
  schema_version: string;
  network_id: string;
  policy_id: string;
  policy_epoch: number;
  content_id: string;
  payload_hash: string;
  sequence: number;
  previous_event_id: string;
  action: string;
  reason_code: string;
  reason_text: string;
  moderator_key: string;
  issued_at: number;
  signature: string;
}

export interface BlockedItem {
  content_id: string;
  payload_hash: string;
  action: string;
  decision_event_id: string;
  reason_code: string;
  reason_text: string;
  blocked_at: number;
}

