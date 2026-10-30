import React, { useState, useEffect, useRef, useMemo } from 'react';
import {
  Minimize2,
  Play,
  Pause,
  SkipBack,
  SkipForward,
  Shuffle,
  Repeat,
  Mic2,
  ListMusic,
  Sliders,
  Volume2,
  VolumeX,
  Gauge,
  Timer,
  Disc,
} from 'lucide-react';
import type { Track } from '../../types/track';
import type { PlayerState } from '../../shared/hooks/useAudioPlayer';
import { WaveformScrubber } from './WaveformScrubber';
import { Badge } from '../../shared/components/Badge';
import { generateCoverDataUri } from '../../shared/utils/artwork';
import { apiClient } from '../../shared/api/client';
import { parseLRC, findActiveLyricIndex } from '../../shared/utils/lrcParser';
import { EqualizerRack, type EqualizerRackProps } from './EqualizerDrawer';
import type { SleepTimerOption } from './useSleepTimer';

interface FullscreenPlayerModalProps {
  isOpen: boolean;
  onClose: () => void;
  currentTrack: Track | null;
  playerState: PlayerState;
  currentTime: number;
  duration: number;
  bufferedPct: number;
  volume: number;
  isMuted: boolean;
  onTogglePlay: () => void;
  onSeek: (seconds: number) => void;
  onVolumeChange: (val: number) => void;
  onToggleMute: () => void;
  playbackRate: number;
  onPlaybackRateChange?: (rate: number) => void;
  // Sleep timer
  sleepTimerOption: SleepTimerOption;
  sleepRemainingFormatted: string | null;
  onStartSleepTimer: (option: SleepTimerOption) => void;
  // Queue & Navigation
  queue: Track[];
  queueIndex: number;
  isShuffle: boolean;
  isRepeat: boolean;
  onPlayNext?: () => void;
  onPlayPrevious?: () => void;
  onToggleShuffle?: () => void;
  onToggleRepeat?: () => void;
  onSelectQueueTrack?: (track: Track) => void;
  // Web Audio Visualizer & Equalizer
  analyser: AnalyserNode | null;
  equalizerProps: EqualizerRackProps;
}

type StudioTab = 'lyrics' | 'queue' | 'equalizer';

export const FullscreenPlayerModal: React.FC<FullscreenPlayerModalProps> = ({
  isOpen,
  onClose,
  currentTrack,
  playerState,
  currentTime,
  duration,
  bufferedPct,
  volume,
  isMuted,
  onTogglePlay,
  onSeek,
  onVolumeChange,
  onToggleMute,
  playbackRate,
  onPlaybackRateChange,
  sleepTimerOption,
  sleepRemainingFormatted,
  onStartSleepTimer,
  queue,
  queueIndex,
  isShuffle,
  isRepeat,
  onPlayNext,
  onPlayPrevious,
  onToggleShuffle,
  onToggleRepeat,
  onSelectQueueTrack,
  analyser,
  equalizerProps,
}) => {
  const [activeTab, setActiveTab] = useState<StudioTab>('lyrics');
  const [isRateMenuOpen, setIsRateMenuOpen] = useState(false);
  const [isSleepMenuOpen, setIsSleepMenuOpen] = useState(false);

  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const activeLyricRef = useRef<HTMLDivElement | null>(null);
  const lyricScrollContainerRef = useRef<HTMLDivElement | null>(null);

  const isPlaying = playerState === 'playing';

  // Keyboard shortcut listener (F to toggle/close, Escape to close, Space to play/pause)
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA') return;

      if (e.key === 'Escape' || e.key === 'f' || e.key === 'F') {
        e.preventDefault();
        onClose();
      } else if (e.code === 'Space') {
        e.preventDefault();
        onTogglePlay();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose, onTogglePlay]);

  // Real-time Web Audio Analyser spectrum visualizer canvas
  useEffect(() => {
    if (!isOpen) return;
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    let animId: number;
    const bufferLength = analyser ? analyser.frequencyBinCount : 32;
    const dataArray = new Uint8Array(bufferLength);

    const render = () => {
      animId = requestAnimationFrame(render);
      const width = canvas.width;
      const height = canvas.height;

      ctx.clearRect(0, 0, width, height);

      if (analyser && isPlaying) {
        analyser.getByteFrequencyData(dataArray);
      } else {
        // Idle ambient gentle wave
        const now = Date.now() / 600;
        for (let i = 0; i < bufferLength; i++) {
          dataArray[i] = Math.max(0, Math.sin(now + i * 0.4) * 10 + 6);
        }
      }

      const barCount = 32;
      const gap = 3;
      const barWidth = (width - (barCount - 1) * gap) / barCount;

      for (let i = 0; i < barCount; i++) {
        const dataIdx = Math.floor((i / barCount) * (bufferLength * 0.85));
        const val = dataArray[dataIdx] || 0;
        const barHeight = Math.max(3, (val / 255) * height);
        const x = i * (barWidth + gap);
        const y = height - barHeight;

        const gradient = ctx.createLinearGradient(0, y, 0, height);
        gradient.addColorStop(0, '#f97316');
        gradient.addColorStop(1, '#ea580c');

        ctx.fillStyle = gradient;
        ctx.beginPath();
        ctx.roundRect(x, y, barWidth, barHeight, [2, 2, 0, 0]);
        ctx.fill();
      }
    };

    render();
    return () => cancelAnimationFrame(animId);
  }, [isOpen, analyser, isPlaying]);

  // Synchronized Lyrics parsing
  const lyricLines = useMemo(() => {
    return parseLRC(currentTrack?.lyrics || '');
  }, [currentTrack?.lyrics]);

  const activeLyricIndex = useMemo(() => {
    return findActiveLyricIndex(lyricLines, currentTime);
  }, [lyricLines, currentTime]);

  // Auto-scroll lyrics to center
  useEffect(() => {
    if (activeTab !== 'lyrics' || activeLyricIndex < 0) return;
    if (activeLyricRef.current && lyricScrollContainerRef.current) {
      activeLyricRef.current.scrollIntoView({
        behavior: 'smooth',
        block: 'center',
      });
    }
  }, [activeTab, activeLyricIndex]);

  if (!isOpen || !currentTrack) return null;

  const fallbackCoverUri = generateCoverDataUri(currentTrack.content_id, currentTrack.title);
  const coverArtUri = currentTrack.has_cover
    ? apiClient.getCoverUrl(currentTrack.content_id)
    : fallbackCoverUri;

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: '#090b0e',
        zIndex: 200,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        color: 'var(--text-primary)',
      }}
    >
      {/* Top Studio Bar */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '16px 28px',
          borderBottom: '1px solid var(--border-glass)',
          backgroundColor: 'rgba(11, 13, 16, 0.8)',
          backdropFilter: 'blur(16px)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <div
            style={{
              width: '32px',
              height: '32px',
              borderRadius: 'var(--radius-md)',
              backgroundColor: 'rgba(249, 115, 22, 0.15)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--accent-primary)',
            }}
          >
            <Disc size={18} />
          </div>
          <div>
            <div style={{ fontSize: '0.94rem', fontWeight: 600 }}>ResonaMesh Studio Mode</div>
            <div style={{ fontSize: '0.74rem', color: 'var(--text-muted)' }}>
              Decentralized Hi-Fi Audio Suite &bull; Press ESC or F to exit
            </div>
          </div>
        </div>

        <button
          onClick={onClose}
          style={{
            background: 'none',
            border: '1px solid var(--border-glass)',
            borderRadius: 'var(--radius-sm)',
            color: 'var(--text-muted)',
            cursor: 'pointer',
            padding: '8px 12px',
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            fontSize: '0.8rem',
            transition: 'all 0.15s ease',
          }}
          title="Exit Fullscreen (ESC)"
        >
          <Minimize2 size={16} />
          <span>Exit Studio</span>
        </button>
      </div>

      {/* Main Split-Pane Workspace */}
      <div
        style={{
          flex: 1,
          display: 'flex',
          overflow: 'hidden',
          padding: '24px 32px',
          gap: '32px',
        }}
      >
        {/* Left Pane: Artwork, Metadata, Live Spectrum & Deck Controls */}
        <div
          style={{
            flex: '1 1 45%',
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'space-between',
            alignItems: 'center',
            backgroundColor: 'rgba(16, 19, 26, 0.6)',
            border: '1px solid var(--border-glass)',
            borderRadius: 'var(--radius-xl)',
            padding: '32px',
            backdropFilter: 'blur(20px)',
            maxWidth: '560px',
            margin: '0 auto',
            width: '100%',
          }}
        >
          {/* Vinyl & Artwork Display */}
          <div
            style={{
              position: 'relative',
              width: '240px',
              height: '240px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <img
              src={coverArtUri}
              alt={currentTrack.title}
              style={{
                width: '100%',
                height: '100%',
                borderRadius: 'var(--radius-lg)',
                objectFit: 'cover',
                boxShadow: '0 12px 32px rgba(0, 0, 0, 0.6)',
                border: isPlaying ? '2px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                transition: 'box-shadow 0.3s ease, border-color 0.3s ease',
              }}
            />
          </div>

          {/* Track Details */}
          <div style={{ textAlign: 'center', width: '100%', marginTop: '16px' }}>
            <h2
              style={{
                fontSize: '1.4rem',
                fontWeight: 700,
                margin: 0,
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {currentTrack.title}
            </h2>
            <p
              style={{
                fontSize: '0.94rem',
                color: 'var(--text-muted)',
                margin: '6px 0 12px 0',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {currentTrack.artist}
            </p>
            <div style={{ display: 'flex', justifyContent: 'center', gap: '8px' }}>
              <Badge variant="primary">{currentTrack.bitrate_kbps} kbps</Badge>
              <Badge variant="cyan">{currentTrack.license_code}</Badge>
            </div>
          </div>

          {/* Real-time Frequency Spectrum Visualizer */}
          <div style={{ width: '100%', padding: '12px 0' }}>
            <canvas
              ref={canvasRef}
              width={420}
              height={54}
              style={{
                width: '100%',
                height: '54px',
                display: 'block',
              }}
            />
          </div>

          {/* Scrubber & Waveform */}
          <div style={{ width: '100%' }}>
            <WaveformScrubber
              currentTime={currentTime}
              duration={duration}
              bufferedPct={bufferedPct}
              onSeek={onSeek}
            />
          </div>

          {/* Playback Controls & Utility Rack */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              width: '100%',
              paddingTop: '12px',
            }}
          >
            {/* Speed & Sleep Utilities */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              {/* Playback Speed */}
              <div style={{ position: 'relative' }}>
                <button
                  onClick={() => {
                    setIsRateMenuOpen((prev) => !prev);
                    setIsSleepMenuOpen(false);
                  }}
                  style={{
                    background: playbackRate !== 1.0 ? 'rgba(249, 115, 22, 0.18)' : 'none',
                    border: '1px solid var(--border-glass)',
                    borderRadius: 'var(--radius-sm)',
                    color: playbackRate !== 1.0 ? 'var(--accent-primary)' : 'var(--text-muted)',
                    padding: '6px 8px',
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                    fontSize: '0.74rem',
                  }}
                  title="Playback Rate"
                >
                  <Gauge size={15} />
                  <span>{playbackRate}x</span>
                </button>

                {isRateMenuOpen && (
                  <div
                    style={{
                      position: 'absolute',
                      bottom: '38px',
                      left: 0,
                      backgroundColor: 'rgba(16, 19, 26, 0.96)',
                      backdropFilter: 'blur(20px)',
                      border: '1px solid var(--border-active)',
                      borderRadius: 'var(--radius-md)',
                      boxShadow: '0 8px 24px rgba(0, 0, 0, 0.7)',
                      padding: '4px',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '2px',
                      zIndex: 220,
                    }}
                  >
                    {[0.5, 0.75, 1.0, 1.25, 1.5, 2.0].map((rate) => (
                      <button
                        key={rate}
                        onClick={() => {
                          onPlaybackRateChange?.(rate);
                          setIsRateMenuOpen(false);
                        }}
                        style={{
                          background: playbackRate === rate ? 'rgba(249, 115, 22, 0.15)' : 'none',
                          border: 'none',
                          borderRadius: 'var(--radius-sm)',
                          color: playbackRate === rate ? 'var(--accent-primary)' : 'var(--text-primary)',
                          cursor: 'pointer',
                          padding: '6px 12px',
                          fontSize: '0.75rem',
                          textAlign: 'left',
                        }}
                      >
                        {rate}x
                      </button>
                    ))}
                  </div>
                )}
              </div>

              {/* Sleep Timer */}
              <div style={{ position: 'relative' }}>
                <button
                  onClick={() => {
                    setIsSleepMenuOpen((prev) => !prev);
                    setIsRateMenuOpen(false);
                  }}
                  style={{
                    background: sleepTimerOption !== 0 ? 'rgba(249, 115, 22, 0.18)' : 'none',
                    border: '1px solid var(--border-glass)',
                    borderRadius: 'var(--radius-sm)',
                    color: sleepTimerOption !== 0 ? 'var(--accent-primary)' : 'var(--text-muted)',
                    padding: '6px 8px',
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                    fontSize: '0.74rem',
                  }}
                  title="Sleep Timer"
                >
                  <Timer size={15} />
                  <span>{sleepRemainingFormatted || 'Timer'}</span>
                </button>

                {isSleepMenuOpen && (
                  <div
                    style={{
                      position: 'absolute',
                      bottom: '38px',
                      left: 0,
                      backgroundColor: 'rgba(16, 19, 26, 0.96)',
                      backdropFilter: 'blur(20px)',
                      border: '1px solid var(--border-active)',
                      borderRadius: 'var(--radius-md)',
                      boxShadow: '0 8px 24px rgba(0, 0, 0, 0.7)',
                      padding: '4px',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '2px',
                      zIndex: 220,
                    }}
                  >
                    {[
                      { label: 'Off', val: 0 as SleepTimerOption },
                      { label: '15 min', val: 15 as SleepTimerOption },
                      { label: '30 min', val: 30 as SleepTimerOption },
                      { label: '45 min', val: 45 as SleepTimerOption },
                      { label: '60 min', val: 60 as SleepTimerOption },
                      { label: 'End of Track', val: -1 as SleepTimerOption },
                    ].map((opt) => (
                      <button
                        key={opt.label}
                        onClick={() => {
                          onStartSleepTimer(opt.val);
                          setIsSleepMenuOpen(false);
                        }}
                        style={{
                          background: sleepTimerOption === opt.val ? 'rgba(249, 115, 22, 0.15)' : 'none',
                          border: 'none',
                          borderRadius: 'var(--radius-sm)',
                          color: sleepTimerOption === opt.val ? 'var(--accent-primary)' : 'var(--text-primary)',
                          cursor: 'pointer',
                          padding: '6px 12px',
                          fontSize: '0.75rem',
                          textAlign: 'left',
                          whiteSpace: 'nowrap',
                        }}
                      >
                        {opt.label}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </div>

            {/* Core Transport Controls */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
              {onToggleShuffle && (
                <button
                  onClick={onToggleShuffle}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: isShuffle ? 'var(--accent-primary)' : 'var(--text-muted)',
                    cursor: 'pointer',
                    padding: '6px',
                  }}
                  title="Toggle Shuffle"
                >
                  <Shuffle size={18} />
                </button>
              )}

              {onPlayPrevious && (
                <button
                  onClick={onPlayPrevious}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: 'var(--text-primary)',
                    cursor: 'pointer',
                    padding: '6px',
                  }}
                  title="Previous Track"
                >
                  <SkipBack size={20} />
                </button>
              )}

              <button
                onClick={onTogglePlay}
                style={{
                  width: '46px',
                  height: '46px',
                  borderRadius: 'var(--radius-full)',
                  backgroundColor: 'var(--accent-primary)',
                  color: '#000',
                  border: 'none',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'pointer',
                  boxShadow: '0 2px 8px rgba(0, 0, 0, 0.3)',
                }}
              >
                {isPlaying ? <Pause size={22} /> : <Play size={22} style={{ marginLeft: '2px' }} />}
              </button>

              {onPlayNext && (
                <button
                  onClick={onPlayNext}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: 'var(--text-primary)',
                    cursor: 'pointer',
                    padding: '6px',
                  }}
                  title="Next Track"
                >
                  <SkipForward size={20} />
                </button>
              )}

              {onToggleRepeat && (
                <button
                  onClick={onToggleRepeat}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: isRepeat ? 'var(--accent-primary)' : 'var(--text-muted)',
                    cursor: 'pointer',
                    padding: '6px',
                  }}
                  title="Toggle Repeat"
                >
                  <Repeat size={18} />
                </button>
              )}
            </div>

            {/* Volume Control */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', width: '120px' }}>
              <button
                onClick={onToggleMute}
                style={{ background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer' }}
              >
                {isMuted || volume === 0 ? <VolumeX size={16} /> : <Volume2 size={16} />}
              </button>
              <input
                type="range"
                min="0"
                max="1"
                step="0.01"
                value={isMuted ? 0 : volume}
                onChange={(e) => onVolumeChange(parseFloat(e.target.value))}
                style={{ width: '100%', accentColor: 'var(--accent-primary)', cursor: 'pointer' }}
              />
            </div>
          </div>
        </div>

        {/* Right Pane: Multi-Tab Studio Workspace (Lyrics, Queue, Equalizer) */}
        <div
          style={{
            flex: '1 1 55%',
            display: 'flex',
            flexDirection: 'column',
            backgroundColor: 'rgba(16, 19, 26, 0.6)',
            border: '1px solid var(--border-glass)',
            borderRadius: 'var(--radius-xl)',
            backdropFilter: 'blur(20px)',
            overflow: 'hidden',
          }}
        >
          {/* Studio Tab Header */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              borderBottom: '1px solid var(--border-glass)',
              backgroundColor: 'rgba(11, 13, 16, 0.5)',
              padding: '6px 16px',
              gap: '8px',
            }}
          >
            {[
              { id: 'lyrics' as StudioTab, label: 'Synchronized Lyrics', icon: <Mic2 size={15} /> },
              { id: 'queue' as StudioTab, label: `Swarm Queue (${queue.length})`, icon: <ListMusic size={15} /> },
              { id: 'equalizer' as StudioTab, label: 'Studio Equalizer', icon: <Sliders size={15} /> },
            ].map((tab) => {
              const isSelected = activeTab === tab.id;
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id)}
                  style={{
                    background: isSelected ? 'rgba(249, 115, 22, 0.15)' : 'none',
                    border: isSelected ? '1px solid var(--accent-primary)' : '1px solid transparent',
                    borderRadius: 'var(--radius-md)',
                    color: isSelected ? 'var(--accent-primary)' : 'var(--text-muted)',
                    cursor: 'pointer',
                    padding: '8px 14px',
                    fontSize: '0.82rem',
                    fontWeight: isSelected ? 600 : 400,
                    display: 'flex',
                    alignItems: 'center',
                    gap: '6px',
                    transition: 'all 0.15s ease',
                  }}
                >
                  {tab.icon}
                  <span>{tab.label}</span>
                </button>
              );
            })}
          </div>

          {/* Tab Content 1: Lyrics */}
          {activeTab === 'lyrics' && (
            <div
              ref={lyricScrollContainerRef}
              style={{
                flex: 1,
                overflowY: 'auto',
                padding: '40px 32px',
                display: 'flex',
                flexDirection: 'column',
                gap: '20px',
                scrollBehavior: 'smooth',
              }}
            >
              {lyricLines.length === 0 ? (
                <div
                  style={{
                    flex: 1,
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: 'var(--text-muted)',
                    gap: '12px',
                  }}
                >
                  <Mic2 size={32} opacity={0.4} />
                  <p style={{ margin: 0, fontSize: '0.9rem' }}>No synchronized lyrics available for this track</p>
                </div>
              ) : (
                lyricLines.map((line, idx) => {
                  const isActive = idx === activeLyricIndex;
                  const isPast = idx < activeLyricIndex;
                  return (
                    <div
                      key={`${idx}-${line.timeSec}`}
                      ref={isActive ? activeLyricRef : null}
                      onClick={() => onSeek(line.timeSec)}
                      style={{
                        padding: '10px 16px',
                        borderRadius: 'var(--radius-md)',
                        cursor: 'pointer',
                        transition: 'all 0.25s ease',
                        backgroundColor: isActive ? 'rgba(249, 115, 22, 0.12)' : 'transparent',
                        borderLeft: isActive ? '3px solid var(--accent-primary)' : '3px solid transparent',
                        color: isActive
                          ? 'var(--accent-primary)'
                          : isPast
                          ? 'rgba(255, 255, 255, 0.35)'
                          : 'rgba(255, 255, 255, 0.85)',
                        fontSize: isActive ? '1.25rem' : '1.02rem',
                        fontWeight: isActive ? 600 : 400,
                        textShadow: 'none',
                        transform: isActive ? 'scale(1.02)' : 'none',
                      }}
                    >
                      {line.text || '♪'}
                    </div>
                  );
                })
              )}
            </div>
          )}

          {/* Tab Content 2: Swarm Playback Queue */}
          {activeTab === 'queue' && (
            <div
              style={{
                flex: 1,
                overflowY: 'auto',
                padding: '16px',
                display: 'flex',
                flexDirection: 'column',
                gap: '8px',
              }}
            >
              {queue.length === 0 ? (
                <div
                  style={{
                    flex: 1,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: 'var(--text-muted)',
                    fontSize: '0.9rem',
                  }}
                >
                  Queue is empty
                </div>
              ) : (
                queue.map((t, idx) => {
                  const isCurrent = idx === queueIndex;
                  return (
                    <div
                      key={`${t.content_id}-${idx}`}
                      onClick={() => onSelectQueueTrack?.(t)}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        padding: '12px 16px',
                        backgroundColor: isCurrent ? 'rgba(249, 115, 22, 0.12)' : 'rgba(255, 255, 255, 0.02)',
                        border: isCurrent ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                        borderRadius: 'var(--radius-md)',
                        cursor: 'pointer',
                        transition: 'background-color 0.15s ease',
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', gap: '12px', minWidth: 0 }}>
                        <span style={{ fontSize: '0.8rem', color: 'var(--text-muted)', width: '24px' }}>
                          {idx + 1}
                        </span>
                        <div style={{ minWidth: 0 }}>
                          <div
                            style={{
                              fontSize: '0.88rem',
                              fontWeight: isCurrent ? 600 : 500,
                              color: isCurrent ? 'var(--accent-primary)' : 'var(--text-primary)',
                              whiteSpace: 'nowrap',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                            }}
                          >
                            {t.title}
                          </div>
                          <div
                            style={{
                              fontSize: '0.74rem',
                              color: 'var(--text-muted)',
                              whiteSpace: 'nowrap',
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                            }}
                          >
                            {t.artist}
                          </div>
                        </div>
                      </div>
                      {isCurrent && <Badge variant="primary">Now Playing</Badge>}
                    </div>
                  );
                })
              )}
            </div>
          )}

          {/* Tab Content 3: Studio Equalizer Rack */}
          {activeTab === 'equalizer' && (
            <div style={{ flex: 1, padding: '16px', overflowY: 'auto' }}>
              <EqualizerRack {...equalizerProps} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
