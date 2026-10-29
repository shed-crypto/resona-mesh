import type { Track, Manifest, TrackListResponse, Report, Decision, BlockedItem } from '../../types/track';
import type { NodeStatus, PeerRecord, StorageUsage, NodeSettings } from '../../types/node';

const API_BASE = '/api/v1';

export const apiClient = {
  async getTracks(params: {
    q?: string;
    limit?: number;
    offset?: number;
    pinned?: boolean;
  } = {}): Promise<TrackListResponse> {
    const searchParams = new URLSearchParams();
    if (params.q) searchParams.set('q', params.q);
    if (params.limit !== undefined) searchParams.set('limit', params.limit.toString());
    if (params.offset !== undefined) searchParams.set('offset', params.offset.toString());
    if (params.pinned) searchParams.set('pinned', 'true');

    const res = await fetch(`${API_BASE}/tracks?${searchParams.toString()}`);
    if (!res.ok) {
      throw new Error(`Failed to fetch tracks: ${res.statusText}`);
    }
    return res.json();
  },

  async getTrack(id: string): Promise<Track> {
    const res = await fetch(`${API_BASE}/tracks/${id}`);
    if (!res.ok) {
      throw new Error(`Failed to fetch track: ${res.statusText}`);
    }
    return res.json();
  },

  async getManifest(id: string): Promise<Manifest> {
    const res = await fetch(`${API_BASE}/tracks/${id}/manifest`);
    if (!res.ok) {
      throw new Error(`Failed to fetch manifest: ${res.statusText}`);
    }
    return res.json();
  },

  async setPinned(id: string, pinned: boolean): Promise<{ content_id: string; pinned: boolean }> {
    const res = await fetch(`${API_BASE}/tracks/${id}/pin`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ pinned }),
    });
    if (!res.ok) {
      throw new Error(`Failed to update pin state: ${res.statusText}`);
    }
    return res.json();
  },

  async deleteTrack(id: string): Promise<{ status: string; content_id: string }> {
    const res = await fetch(`${API_BASE}/tracks/${id}`, {
      method: 'DELETE',
    });
    if (!res.ok) {
      throw new Error(`Failed to delete track: ${res.statusText}`);
    }
    return res.json();
  },

  async importTrack(formData: FormData): Promise<{ manifest: Manifest; track: Track }> {
    const res = await fetch(`${API_BASE}/tracks/import`, {
      method: 'POST',
      body: formData,
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || `Import failed with status ${res.status}`);
    }
    return res.json();
  },

  async getNodeStatus(): Promise<NodeStatus> {
    const res = await fetch(`${API_BASE}/node/status`);
    if (!res.ok) {
      throw new Error(`Failed to fetch node status: ${res.statusText}`);
    }
    return res.json();
  },

  async getPeers(): Promise<{ peers: PeerRecord[]; total: number }> {
    const res = await fetch(`${API_BASE}/peers`);
    if (!res.ok) {
      throw new Error(`Failed to fetch peers: ${res.statusText}`);
    }
    return res.json();
  },

  async syncPeers(): Promise<{ status: string; imported_tracks: number }> {
    const res = await fetch(`${API_BASE}/peers/sync`, {
      method: 'POST',
    });
    if (!res.ok) {
      throw new Error(`Failed to sync peers: ${res.statusText}`);
    }
    return res.json();
  },

  async getStorage(): Promise<StorageUsage> {
    const res = await fetch(`${API_BASE}/storage`);
    if (!res.ok) {
      throw new Error(`Failed to fetch storage metrics: ${res.statusText}`);
    }
    return res.json();
  },

  async getSettings(): Promise<NodeSettings> {
    const res = await fetch(`${API_BASE}/settings`);
    if (!res.ok) {
      throw new Error(`Failed to fetch settings: ${res.statusText}`);
    }
    return res.json();
  },

  async updateSettings(settings: Partial<NodeSettings>): Promise<NodeSettings> {
    const res = await fetch(`${API_BASE}/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(settings),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || `Failed to update settings`);
    }
    return res.json();
  },

  async connectPeer(addr: string): Promise<{ status: string; peer_id: string }> {
    const res = await fetch(`${API_BASE}/peers/connect`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ addr }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || `Failed to connect to peer`);
    }
    return res.json();
  },

  async submitReport(params: {
    content_id: string;
    payload_hash?: string;
    reason_code: string;
    description: string;
    evidence_reference?: string;
  }): Promise<Report> {
    const res = await fetch(`${API_BASE}/reports`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || 'Failed to submit report');
    }
    return res.json();
  },

  async getReports(): Promise<{ items: Report[]; total: number }> {
    const res = await fetch(`${API_BASE}/reports`);
    if (!res.ok) {
      throw new Error(`Failed to fetch reports: ${res.statusText}`);
    }
    return res.json();
  },

  async getDecisions(): Promise<{ items: Decision[]; total: number }> {
    const res = await fetch(`${API_BASE}/moderation/decisions`);
    if (!res.ok) {
      throw new Error(`Failed to fetch decisions: ${res.statusText}`);
    }
    return res.json();
  },

  async getBlocked(): Promise<{ items: BlockedItem[]; total: number }> {
    const res = await fetch(`${API_BASE}/moderation/blocked`);
    if (!res.ok) {
      throw new Error(`Failed to fetch blocked content: ${res.statusText}`);
    }
    return res.json();
  },

  async createDecision(params: {
    content_id: string;
    payload_hash?: string;
    action: 'BLOCK' | 'RESTORE';
    reason_code: string;
    reason_text?: string;
  }): Promise<{ status: string; decision: Decision }> {
    const res = await fetch(`${API_BASE}/moderation/decisions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || 'Failed to execute moderation decision');
    }
    return res.json();
  },

  getStreamUrl(id: string): string {
    return `${API_BASE}/stream/${id}`;
  },

  getCoverUrl(id: string): string {
    return `${API_BASE}/tracks/${id}/cover`;
  },
};

