import { useState, useEffect, useRef, useCallback } from 'react';
import type { Track } from '../../types/track';
import { apiClient } from '../api/client';
import { generateCoverDataUri } from '../utils/artwork';
import { recordLocalPlay } from './useRecommendations';

export type PlayerState = 'idle' | 'connecting' | 'buffering' | 'playing' | 'paused' | 'stalled';

export function useAudioPlayer() {
  const [currentTrack, setCurrentTrack] = useState<Track | null>(null);
  const [playerState, setPlayerState] = useState<PlayerState>('idle');
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [bufferedPct, setBufferedPct] = useState(0);
  const [volume, setVolume] = useState(0.85);
  const [isMuted, setIsMuted] = useState(false);
  const [audioElement, setAudioElement] = useState<HTMLAudioElement | null>(null);

  // Playback Queue & Controls
  const [queue, setQueue] = useState<Track[]>([]);
  const [queueIndex, setQueueIndex] = useState<number>(-1);
  const [isShuffle, setIsShuffle] = useState(false);
  const [isRepeat, setIsRepeat] = useState(false);
  const [playbackRate, setPlaybackRateState] = useState<number>(1.0);
  const playbackRateRef = useRef<number>(1.0);

  const audioRef = useRef<HTMLAudioElement | null>(null);

  // Refs for current values accessible in audio event listeners
  const queueRef = useRef<Track[]>(queue);
  queueRef.current = queue;

  const queueIndexRef = useRef<number>(queueIndex);
  queueIndexRef.current = queueIndex;

  const isShuffleRef = useRef<boolean>(isShuffle);
  isShuffleRef.current = isShuffle;

  const isRepeatRef = useRef<boolean>(isRepeat);
  isRepeatRef.current = isRepeat;

  const currentTrackRef = useRef<Track | null>(currentTrack);
  currentTrackRef.current = currentTrack;

  // Initialize single persistent audio instance
  useEffect(() => {
    const audio = new Audio();
    audio.preload = 'auto';
    audioRef.current = audio;
    setAudioElement(audio);

    const onPlay = () => setPlayerState('playing');
    const onPause = () => setPlayerState('paused');
    const onWaiting = () => setPlayerState('buffering');
    const onStalled = () => setPlayerState('stalled');
    const onCanPlay = () => {
      if (audio.paused) {
        setPlayerState('paused');
      } else {
        setPlayerState('playing');
      }
    };

    const onTimeUpdate = () => {
      setCurrentTime(audio.currentTime);
      if (audio.duration && !isNaN(audio.duration) && isFinite(audio.duration)) {
        setDuration(audio.duration);
      }
    };

    const onProgress = () => {
      if (audio.buffered.length > 0 && audio.duration) {
        const bufferedEnd = audio.buffered.end(audio.buffered.length - 1);
        const pct = Math.min(100, (bufferedEnd / audio.duration) * 100);
        setBufferedPct(pct);
      }
    };

    const onEnded = () => {
      if (currentTrackRef.current) {
        recordLocalPlay(currentTrackRef.current.content_id, true);
      }

      if (isRepeatRef.current) {
        audio.currentTime = 0;
        audio.play().catch(console.error);
        return;
      }

      const q = queueRef.current;
      const idx = queueIndexRef.current;

      if (q.length > 0) {
        let nextIdx = idx + 1;
        if (isShuffleRef.current) {
          nextIdx = Math.floor(Math.random() * q.length);
        }

        if (nextIdx < q.length) {
          playTrackInternal(q[nextIdx], q, nextIdx);
          return;
        }
      }

      setPlayerState('paused');
      setCurrentTime(0);
    };

    audio.addEventListener('play', onPlay);
    audio.addEventListener('pause', onPause);
    audio.addEventListener('waiting', onWaiting);
    audio.addEventListener('stalled', onStalled);
    audio.addEventListener('canplay', onCanPlay);
    audio.addEventListener('timeupdate', onTimeUpdate);
    audio.addEventListener('progress', onProgress);
    audio.addEventListener('ended', onEnded);

    return () => {
      audio.pause();
      audio.src = '';
      audio.removeEventListener('play', onPlay);
      audio.removeEventListener('pause', onPause);
      audio.removeEventListener('waiting', onWaiting);
      audio.removeEventListener('stalled', onStalled);
      audio.removeEventListener('canplay', onCanPlay);
      audio.removeEventListener('timeupdate', onTimeUpdate);
      audio.removeEventListener('progress', onProgress);
      audio.removeEventListener('ended', onEnded);
    };
  }, []);

  // Update volume
  useEffect(() => {
    if (audioRef.current) {
      audioRef.current.volume = isMuted ? 0 : volume;
    }
  }, [volume, isMuted]);

  const playTrackInternal = useCallback(async (track: Track, newQueue?: Track[], newIndex?: number) => {
    const audio = audioRef.current;
    if (!audio) return;

    if (newQueue) {
      setQueue(newQueue);
      setQueueIndex(newIndex !== undefined ? newIndex : newQueue.findIndex((t) => t.content_id === track.content_id));
    } else if (newIndex !== undefined) {
      setQueueIndex(newIndex);
    }

    setCurrentTrack(track);
    setPlayerState('connecting');
    setCurrentTime(0);
    setBufferedPct(0);
    setDuration(track.duration_ms > 0 ? track.duration_ms / 1000 : 0);

    const streamUrl = apiClient.getStreamUrl(track.content_id);
    audio.src = streamUrl;
    audio.load();
    audio.playbackRate = playbackRateRef.current;
    audio.preservesPitch = true;
    recordLocalPlay(track.content_id, false);

    try {
      await audio.play();
    } catch (err: unknown) {
      console.warn('Playback initiation failed or interrupted:', err);
    }
  }, []);

  const playTrack = useCallback(async (track: Track, newQueue?: Track[]) => {
    const audio = audioRef.current;
    if (!audio) return;

    if (currentTrack?.content_id === track.content_id) {
      if (audio.paused) {
        await audio.play().catch(console.error);
      } else {
        audio.pause();
      }
      return;
    }

    const currentQ = newQueue || (queue.length > 0 ? queue : [track]);
    const idx = currentQ.findIndex((t) => t.content_id === track.content_id);
    await playTrackInternal(track, currentQ, idx >= 0 ? idx : 0);
  }, [currentTrack, queue, playTrackInternal]);

  const playNext = useCallback(() => {
    if (queue.length === 0) return;
    let nextIdx = queueIndex + 1;
    if (isShuffle) {
      nextIdx = Math.floor(Math.random() * queue.length);
    }
    if (nextIdx < queue.length) {
      playTrackInternal(queue[nextIdx], queue, nextIdx);
    } else if (isRepeat && queue.length > 0) {
      playTrackInternal(queue[0], queue, 0);
    }
  }, [queue, queueIndex, isShuffle, isRepeat, playTrackInternal]);

  const playPrevious = useCallback(() => {
    if (queue.length === 0) return;
    const audio = audioRef.current;
    if (audio && audio.currentTime > 3) {
      audio.currentTime = 0;
      setCurrentTime(0);
      return;
    }
    const prevIdx = queueIndex > 0 ? queueIndex - 1 : 0;
    playTrackInternal(queue[prevIdx], queue, prevIdx);
  }, [queue, queueIndex, playTrackInternal]);

  const toggleShuffle = useCallback(() => {
    setIsShuffle((prev) => !prev);
  }, []);

  const toggleRepeat = useCallback(() => {
    setIsRepeat((prev) => !prev);
  }, []);

  const addToQueue = useCallback((track: Track) => {
    setQueue((prev) => [...prev, track]);
  }, []);

  const clearQueue = useCallback(() => {
    setQueue([]);
    setQueueIndex(-1);
  }, []);

  const togglePlay = useCallback(() => {
    const audio = audioRef.current;
    if (!audio || !currentTrack) return;

    if (audio.paused) {
      audio.play().catch(console.error);
    } else {
      audio.pause();
    }
  }, [currentTrack]);

  const setPlaybackRate = useCallback((rate: number) => {
    setPlaybackRateState(rate);
    playbackRateRef.current = rate;
    if (audioRef.current) {
      audioRef.current.playbackRate = rate;
      audioRef.current.preservesPitch = true;
    }
  }, []);

  const seek = useCallback((targetTimeSeconds: number) => {
    const audio = audioRef.current;
    if (!audio) return;

    audio.currentTime = targetTimeSeconds;
    setCurrentTime(targetTimeSeconds);
  }, []);

  // Sync with Media Session API
  useEffect(() => {
    if ('mediaSession' in navigator && currentTrack) {
      const coverUri = generateCoverDataUri(currentTrack.content_id, currentTrack.title);

      navigator.mediaSession.metadata = new MediaMetadata({
        title: currentTrack.title,
        artist: currentTrack.artist,
        album: 'ResonaMesh Swarm',
        artwork: [
          { src: coverUri, sizes: '200x200', type: 'image/svg+xml' },
        ],
      });

      navigator.mediaSession.setActionHandler('play', () => {
        audioRef.current?.play().catch(console.error);
      });
      navigator.mediaSession.setActionHandler('pause', () => {
        audioRef.current?.pause();
      });
      navigator.mediaSession.setActionHandler('nexttrack', () => {
        playNext();
      });
      navigator.mediaSession.setActionHandler('previoustrack', () => {
        playPrevious();
      });
      navigator.mediaSession.setActionHandler('seekto', (details) => {
        if (details.seekTime !== undefined && audioRef.current) {
          audioRef.current.currentTime = details.seekTime;
        }
      });
    }
  }, [currentTrack, playNext, playPrevious]);

  return {
    audioElement,
    currentTrack,
    playerState,
    currentTime,
    duration,
    bufferedPct,
    volume,
    setVolume,
    isMuted,
    setIsMuted,
    playTrack,
    togglePlay,
    seek,
    playbackRate,
    setPlaybackRate,
    isPlaying: playerState === 'playing',
    // Queue & Controls
    queue,
    queueIndex,
    isShuffle,
    isRepeat,
    playNext,
    playPrevious,
    toggleShuffle,
    toggleRepeat,
    addToQueue,
    clearQueue,
  };
}
