import { useState, useEffect, useRef, useCallback } from 'react';
import type { Track } from '../../types/track';
import { apiClient } from '../api/client';

export type PlayerState = 'idle' | 'connecting' | 'buffering' | 'playing' | 'paused' | 'stalled';

export function useAudioPlayer() {
  const [currentTrack, setCurrentTrack] = useState<Track | null>(null);
  const [playerState, setPlayerState] = useState<PlayerState>('idle');
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [bufferedPct, setBufferedPct] = useState(0);
  const [volume, setVolume] = useState(0.85);
  const [isMuted, setIsMuted] = useState(false);

  const audioRef = useRef<HTMLAudioElement | null>(null);

  // Initialize single persistent audio instance
  useEffect(() => {
    const audio = new Audio();
    audio.preload = 'auto';
    audioRef.current = audio;

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

  // Sync with Media Session API
  useEffect(() => {
    if ('mediaSession' in navigator && currentTrack) {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: currentTrack.title,
        artist: currentTrack.artist,
        album: 'ResonaMesh Swarm',
      });

      navigator.mediaSession.setActionHandler('play', () => {
        audioRef.current?.play().catch(console.error);
      });
      navigator.mediaSession.setActionHandler('pause', () => {
        audioRef.current?.pause();
      });
      navigator.mediaSession.setActionHandler('seekto', (details) => {
        if (details.seekTime !== undefined && audioRef.current) {
          audioRef.current.currentTime = details.seekTime;
        }
      });
    }
  }, [currentTrack]);

  const playTrack = useCallback(async (track: Track) => {
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

    setCurrentTrack(track);
    setPlayerState('connecting');
    setCurrentTime(0);
    setBufferedPct(0);
    setDuration(track.duration_ms > 0 ? track.duration_ms / 1000 : 0);

    const streamUrl = apiClient.getStreamUrl(track.content_id);
    audio.src = streamUrl;
    audio.load();

    try {
      await audio.play();
    } catch (err: unknown) {
      console.warn('Playback initiation failed or interrupted:', err);
    }
  }, [currentTrack]);

  const togglePlay = useCallback(() => {
    const audio = audioRef.current;
    if (!audio || !currentTrack) return;

    if (audio.paused) {
      audio.play().catch(console.error);
    } else {
      audio.pause();
    }
  }, [currentTrack]);

  const seek = useCallback((targetTimeSeconds: number) => {
    const audio = audioRef.current;
    if (!audio) return;

    audio.currentTime = targetTimeSeconds;
    setCurrentTime(targetTimeSeconds);
  }, []);

  return {
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
    isPlaying: playerState === 'playing',
  };
}
