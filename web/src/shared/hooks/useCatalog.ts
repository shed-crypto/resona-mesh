import { useState, useEffect, useCallback, useRef } from 'react';
import type { Track } from '../../types/track';
import { apiClient } from '../api/client';

const PAGE_SIZE = 20;

export function useCatalog() {
  const [tracks, setTracks] = useState<Track[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [pinnedOnly, setPinnedOnly] = useState(false);

  const tracksLengthRef = useRef(0);
  tracksLengthRef.current = tracks.length;

  const hasMore = tracks.length < total;

  // Initial load and filter change (page 0)
  const fetchInitialTracks = useCallback(async (showLoading = false) => {
    if (showLoading) {
      setLoading(true);
    }
    setError(null);
    try {
      const res = await apiClient.getTracks({
        q: search,
        pinned: pinnedOnly,
        limit: PAGE_SIZE,
        offset: 0,
      });
      setTracks(res.items || []);
      setTotal(res.total || 0);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to fetch tracks';
      setError(msg);
    } finally {
      if (showLoading) {
        setLoading(false);
      }
    }
  }, [search, pinnedOnly]);

  // Infinite Scroll fetch next page
  const fetchMoreTracks = useCallback(async () => {
    if (loadingMore || loading) return;
    const currentOffset = tracksLengthRef.current;
    setLoadingMore(true);
    try {
      const res = await apiClient.getTracks({
        q: search,
        pinned: pinnedOnly,
        limit: PAGE_SIZE,
        offset: currentOffset,
      });
      if (res.items && res.items.length > 0) {
        setTracks((prev) => {
          const seen = new Set(prev.map((t) => t.content_id));
          const fresh = res.items.filter((t) => !seen.has(t.content_id));
          return [...prev, ...fresh];
        });
      }
      setTotal(res.total || 0);
    } catch (err: unknown) {
      console.error('Failed to fetch next page:', err);
    } finally {
      setLoadingMore(false);
    }
  }, [search, pinnedOnly, loadingMore, loading]);

  useEffect(() => {
    fetchInitialTracks(true);
  }, [fetchInitialTracks]);

  // Periodic background refresh for metadata/seeders on currently visible items
  useEffect(() => {
    const interval = setInterval(async () => {
      const currentCount = tracksLengthRef.current;
      if (currentCount === 0) return;
      try {
        const res = await apiClient.getTracks({
          q: search,
          pinned: pinnedOnly,
          limit: Math.max(PAGE_SIZE, currentCount),
          offset: 0,
        });
        if (res.items) {
          setTracks((prev) => {
            const freshMap = new Map(res.items.map((t) => [t.content_id, t]));
            return prev.map((t) => freshMap.get(t.content_id) || t);
          });
          setTotal(res.total || 0);
        }
      } catch {
        // silent background failure
      }
    }, 6000);
    return () => clearInterval(interval);
  }, [search, pinnedOnly]);

  const refresh = async () => {
    try {
      await apiClient.syncPeers();
    } catch {
      // ignore sync errors if offline
    }
    await fetchInitialTracks(true);
  };

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
    loadingMore,
    hasMore,
    fetchMoreTracks,
    error,
    search,
    setSearch,
    pinnedOnly,
    setPinnedOnly,
    refresh,
    togglePin,
    deleteTrack,
  };
}
