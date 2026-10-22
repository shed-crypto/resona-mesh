import { useState, useEffect, useCallback } from 'react';
import type { Playlist, ShareablePlaylistManifest } from '../../types/playlist';
import type { Track } from '../../types/track';

const STORAGE_KEY = 'resonamesh_playlists_v1';

export function usePlaylists() {
  const [playlists, setPlaylists] = useState<Playlist[]>(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved) {
        return JSON.parse(saved);
      }
    } catch {
      // Fallback on corrupt storage
    }
    return [
      {
        id: 'favorites',
        title: 'Favorites',
        description: 'Locally pinned and favorited tracks',
        trackIds: [],
        createdAt: Date.now(),
        updatedAt: Date.now(),
      },
    ];
  });

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(playlists));
    } catch (err) {
      console.warn('Failed to save playlists to localStorage', err);
    }
  }, [playlists]);

  const createPlaylist = useCallback((title: string, description?: string): Playlist => {
    const trimmedTitle = title.trim();
    if (!trimmedTitle) {
      throw new Error('Playlist title cannot be empty');
    }

    const newPlaylist: Playlist = {
      id: 'pl_' + Date.now().toString(36) + Math.random().toString(36).substring(2, 6),
      title: trimmedTitle,
      description: description?.trim() || '',
      trackIds: [],
      createdAt: Date.now(),
      updatedAt: Date.now(),
    };

    setPlaylists((prev) => [newPlaylist, ...prev]);
    return newPlaylist;
  }, []);

  const deletePlaylist = useCallback((playlistId: string) => {
    if (playlistId === 'favorites') {
      return; // Protect default favorites playlist
    }
    setPlaylists((prev) => prev.filter((p) => p.id !== playlistId));
  }, []);

  const addTrackToPlaylist = useCallback((playlistId: string, trackId: string) => {
    setPlaylists((prev) =>
      prev.map((p) => {
        if (p.id !== playlistId) return p;
        if (p.trackIds.includes(trackId)) return p;
        return {
          ...p,
          trackIds: [...p.trackIds, trackId],
          updatedAt: Date.now(),
        };
      })
    );
  }, []);

  const removeTrackFromPlaylist = useCallback((playlistId: string, trackId: string) => {
    setPlaylists((prev) =>
      prev.map((p) => {
        if (p.id !== playlistId) return p;
        return {
          ...p,
          trackIds: p.trackIds.filter((id) => id !== trackId),
          updatedAt: Date.now(),
        };
      })
    );
  }, []);

  const exportPlaylistManifest = useCallback((playlist: Playlist, allTracks: Track[]): string => {
    const trackMap = new Map(allTracks.map((t) => [t.content_id, t]));
    const matchedTracks = playlist.trackIds
      .map((id) => trackMap.get(id))
      .filter((t): t is Track => t !== undefined)
      .map((t) => ({
        contentId: t.content_id,
        title: t.title,
        artist: t.artist,
        durationMs: t.duration_ms,
      }));

    const manifest: ShareablePlaylistManifest = {
      version: 'resonamesh:playlist:v1',
      title: playlist.title,
      description: playlist.description,
      exportedAt: Date.now(),
      tracks: matchedTracks,
    };

    return JSON.stringify(manifest, null, 2);
  }, []);

  const importPlaylistManifest = useCallback((jsonStr: string): Playlist | null => {
    try {
      const manifest = JSON.parse(jsonStr) as ShareablePlaylistManifest;
      if (manifest.version !== 'resonamesh:playlist:v1' || !Array.isArray(manifest.tracks)) {
        throw new Error('Invalid playlist manifest format');
      }

      const imported: Playlist = {
        id: 'pl_imp_' + Date.now().toString(36),
        title: manifest.title || 'Imported Swarm Playlist',
        description: manifest.description || 'Imported via P2P link',
        trackIds: manifest.tracks.map((t) => t.contentId).filter(Boolean),
        createdAt: Date.now(),
        updatedAt: Date.now(),
      };

      setPlaylists((prev) => [imported, ...prev]);
      return imported;
    } catch (err) {
      console.warn('Failed to import playlist manifest', err);
      return null;
    }
  }, []);

  return {
    playlists,
    createPlaylist,
    deletePlaylist,
    addTrackToPlaylist,
    removeTrackFromPlaylist,
    exportPlaylistManifest,
    importPlaylistManifest,
  };
}
