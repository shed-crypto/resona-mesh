import { useState, useEffect, useMemo } from 'react';
import type { Track } from '../../types/track';

const STORAGE_KEY = 'resona_local_affinity_v1';

interface PlayStat {
  contentId: string;
  playCount: number;
  completedCount: number;
  lastPlayedAt: number;
}

interface LocalAffinityStore {
  stats: Record<string, PlayStat>;
}

function getStoredAffinity(): LocalAffinityStore {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { stats: {} };
    return JSON.parse(raw);
  } catch {
    return { stats: {} };
  }
}

export function recordLocalPlay(contentId: string, completed = false) {
  if (!contentId) return;
  try {
    const store = getStoredAffinity();
    const stat = store.stats[contentId] || {
      contentId,
      playCount: 0,
      completedCount: 0,
      lastPlayedAt: Date.now(),
    };
    stat.playCount += 1;
    if (completed) {
      stat.completedCount += 1;
    }
    stat.lastPlayedAt = Date.now();
    store.stats[contentId] = stat;
    localStorage.setItem(STORAGE_KEY, JSON.stringify(store));
  } catch {
    // LocalStorage fallback
  }
}

export function useRecommendations(allTracks: Track[]) {
  const [affinityStore, setAffinityStore] = useState<LocalAffinityStore>(getStoredAffinity);

  // Re-read affinity on catalog changes or mount
  useEffect(() => {
    setAffinityStore(getStoredAffinity());
  }, [allTracks.length]);

  const { recommendedTracks, topArtists } = useMemo(() => {
    if (!allTracks || allTracks.length === 0) {
      return { recommendedTracks: [], topArtists: [] };
    }

    const stats = affinityStore.stats;

    // 1. Build artist affinity vector
    const artistScores: Record<string, number> = {};
    for (const track of allTracks) {
      const stat = stats[track.content_id];
      const artist = track.artist?.trim() || 'Unknown Artist';

      let score = 0;
      if (stat) {
        score += stat.playCount * 2 + stat.completedCount * 3;
      }
      if (track.pinned) {
        score += 5;
      }

      artistScores[artist] = (artistScores[artist] || 0) + score;
    }

    // Sort top favorite artists
    const sortedArtists = Object.entries(artistScores)
      .filter(([_, score]) => score > 0)
      .sort((a, b) => b[1] - a[1])
      .map(([artist]) => artist);

    // 2. Score candidate tracks
    // Exclude blocked tracks
    const candidates = allTracks.filter((t) => !t.blocked);

    const scoredTracks = candidates.map((track) => {
      const stat = stats[track.content_id];
      const playCount = stat ? stat.playCount : 0;
      const artist = track.artist?.trim() || 'Unknown Artist';
      const artistAffinity = artistScores[artist] || 0;

      let score = 0;

      // Affinity match bonus
      if (artistAffinity > 0) {
        score += artistAffinity * 2.5;
      }

      // Cryptographic signature trust bonus
      if (track.signature) {
        score += 3;
      }

      // Favor discovery over repetition: discount tracks already played frequently
      if (playCount > 0) {
        score -= playCount * 2;
      } else {
        // Fresh discovery boost
        score += 4;
      }

      return { track, score };
    });

    // Sort descending by discovery score
    scoredTracks.sort((a, b) => b.score - a.score);

    // Return top 5 recommendations
    const topRecommended = scoredTracks.slice(0, 5).map((item) => item.track);

    return {
      recommendedTracks: topRecommended,
      topArtists: sortedArtists.slice(0, 3),
    };
  }, [allTracks, affinityStore]);

  return {
    recommendedTracks,
    topArtists,
    refreshAffinity: () => setAffinityStore(getStoredAffinity()),
  };
}
