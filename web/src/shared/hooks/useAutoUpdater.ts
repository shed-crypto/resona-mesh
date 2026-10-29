import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, type UpdateState } from '../api/client';

export function useAutoUpdater() {
  const [state, setState] = useState<UpdateState>({
    status: 'idle',
    current_version: '0.1.0-alpha',
    progress_pct: 0,
    downloaded_blocks: 0,
    total_blocks: 0,
  });
  const [loading, setLoading] = useState(false);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const pollTimerRef = useRef<number | null>(null);

  const fetchStatus = useCallback(async () => {
    try {
      const data = await apiClient.getUpdaterStatus();
      setState(data);
    } catch {
      // Ignore network errors during background polling
    }
  }, []);

  const checkForUpdates = useCallback(async () => {
    setLoading(true);
    setActionMessage(null);
    try {
      const data = await apiClient.checkUpdate();
      setState(data);
      if (data.status === 'available') {
        setActionMessage(`New release v${data.available_release?.version} found!`);
      } else {
        setActionMessage('You are running the latest version.');
      }
    } catch (err: any) {
      setActionMessage(`Check failed: ${err.message || 'unknown error'}`);
    } finally {
      setLoading(false);
    }
  }, []);

  const startDownload = useCallback(async () => {
    setLoading(true);
    try {
      const data = await apiClient.downloadUpdate();
      setState(data);
    } catch (err: any) {
      setActionMessage(`Download failed: ${err.message || 'unknown error'}`);
    } finally {
      setLoading(false);
    }
  }, []);

  const applyUpdateAndRestart = useCallback(async () => {
    setLoading(true);
    try {
      const res = await apiClient.applyUpdate();
      setActionMessage(res.message || 'Update applied! Please restart ResonaMesh.');
      await fetchStatus();
    } catch (err: any) {
      setActionMessage(`Apply failed: ${err.message || 'unknown error'}`);
    } finally {
      setLoading(false);
    }
  }, [fetchStatus]);

  useEffect(() => {
    fetchStatus();

    // Fast polling if currently downloading, otherwise 20s interval
    const intervalMs = state.status === 'downloading' ? 1000 : 20000;
    pollTimerRef.current = window.setInterval(fetchStatus, intervalMs);

    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
      }
    };
  }, [fetchStatus, state.status]);

  return {
    state,
    loading,
    actionMessage,
    checkForUpdates,
    startDownload,
    applyUpdateAndRestart,
  };
}
