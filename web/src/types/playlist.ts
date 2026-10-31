export interface Playlist {
  id: string;
  title: string;
  description?: string;
  trackIds: string[];
  createdAt: number;
  updatedAt: number;
}

export interface ShareablePlaylistManifest {
  version: 'resonamesh:playlist:v1';
  title: string;
  description?: string;
  exportedAt: number;
  tracks: Array<{
    contentId: string;
    title: string;
    artist: string;
    durationMs: number;
  }>;
}
