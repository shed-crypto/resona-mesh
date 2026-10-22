import { useState, useEffect, useCallback } from 'react';
import type { Track } from '../../types/track';
import { apiClient } from '../api/client';

export function useCatalog() {
  const [tracks, setTracks] = useState<Track[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [pinnedOnly, setPinnedOnly] = useState(false);

  const fetchTracks = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await apiClient.getTracks({
        q: search,
        pinned: pinnedOnly,
        limit: 50,
      });
      setTracks(res.items || []);
      setTotal(res.total || 0);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to fetch tracks';
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, [search, pinnedOnly]);

  useEffect(() => {
    fetchTracks();
  }, [fetchTracks]);

  const togglePin = async (id: string, currentPinned: boolean) => {
    try {
      await apiClient.setPinned(id, !currentPinned);
      setTracks((prev) =>
        prev.map((t) => (t.content_id === id ? { ...t, pinned: !currentPinned } : t))
      );
    } catch (err: unknown) {
      console.error('Failed to toggle pin state:', err);
    }
  };

  const deleteTrack = async (id: string) => {
    try {
      await apiClient.deleteTrack(id);
      setTracks((prev) => prev.filter((t) => t.content_id !== id));
      setTotal((prev) => Math.max(0, prev - 1));
    } catch (err: unknown) {
      console.error('Failed to delete track:', err);
    }
  };

  return {
    tracks,
    total,
    loading,
    error,
    search,
    setSearch,
    pinnedOnly,
    setPinnedOnly,
    refresh: fetchTracks,
    togglePin,
    deleteTrack,
  };
}
