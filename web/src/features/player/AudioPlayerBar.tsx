import React, { useState, useEffect } from 'react';
import {
  Play,
  Pause,
  Loader2,
  HardDrive,
  WifiOff,
  SkipBack,
  SkipForward,
  Shuffle,
  Repeat,
  Radio,
  ListMusic,
  Trash2,
  Download,
  Mic2,
  Sliders,
  Gauge,
  Timer,
  Maximize2,
  MoreHorizontal,
} from 'lucide-react';
import type { Track } from '../../types/track';
import type { PlayerState } from '../../shared/hooks/useAudioPlayer';
import { LyricsDrawer } from './LyricsDrawer';
import { EqualizerDrawer } from './EqualizerDrawer';
import { FullscreenPlayerModal } from './FullscreenPlayerModal';
import { useAudioEqualizer } from './useAudioEqualizer';
import { useSleepTimer } from './useSleepTimer';
import { WaveformScrubber } from './WaveformScrubber';
import { VolumeControl } from './VolumeControl';
import { Badge } from '../../shared/components/Badge';
import { generateCoverDataUri } from '../../shared/utils/artwork';
import { apiClient } from '../../shared/api/client';
import { useNodeStatus } from '../../shared/hooks/useNodeStatus';

interface AudioPlayerBarProps {
  audioElement?: HTMLAudioElement | null;
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
  playbackRate?: number;
  onPlaybackRateChange?: (rate: number) => void;
  // Queue & Navigation
  queue?: Track[];
  queueIndex?: number;
  isShuffle?: boolean;
  isRepeat?: boolean;
  onPlayNext?: () => void;
  onPlayPrevious?: () => void;
  onToggleShuffle?: () => void;
  isRadio?: boolean;
  onToggleRadio?: () => void;
  onToggleRepeat?: () => void;
  onSelectQueueTrack?: (track: Track) => void;
  onClearQueue?: () => void;
}

export const AudioPlayerBar: React.FC<AudioPlayerBarProps> = ({
  audioElement,
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
  playbackRate = 1.0,
  onPlaybackRateChange,
  queue = [],
  queueIndex = -1,
  isShuffle = false,
  isRepeat = false,
  onPlayNext,
  onPlayPrevious,
  isRadio,
  onToggleRadio,
  onToggleShuffle,
  onToggleRepeat,
  onSelectQueueTrack,
  onClearQueue,
}) => {
  const [isQueueOpen, setIsQueueOpen] = useState(false);
  const [isLyricsOpen, setIsLyricsOpen] = useState(false);
  const [isEqOpen, setIsEqOpen] = useState(false);
  const [isFullscreenOpen, setIsFullscreenOpen] = useState(false);
  const [isMoreMenuOpen, setIsMoreMenuOpen] = useState(false);
  const { status: nodeStatus } = useNodeStatus();
  const connectedPeers = nodeStatus?.connected_peers ?? 0;

  const closeAllPopups = () => {
    setIsLyricsOpen(false);
    setIsEqOpen(false);
    setIsQueueOpen(false);
    setIsMoreMenuOpen(false);
  };

  const toggleLyrics = () => {
    const next = !isLyricsOpen;
    closeAllPopups();
    if (next) setIsLyricsOpen(true);
  };

  const toggleEq = () => {
    const next = !isEqOpen;
    closeAllPopups();
    if (next) setIsEqOpen(true);
  };

  const toggleQueue = () => {
    const next = !isQueueOpen;
    closeAllPopups();
    if (next) setIsQueueOpen(true);
  };

  const toggleMoreMenu = () => {
    const next = !isMoreMenuOpen;
    closeAllPopups();
    if (next) setIsMoreMenuOpen(true);
  };

  const eq = useAudioEqualizer(audioElement ?? null);
  const sleepTimer = useSleepTimer(
    audioElement ?? null,
    volume,
    isMuted,
    () => {
      if (playerState === 'playing') {
        onTogglePlay();
      }
    }
  );

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (isLyricsOpen || isEqOpen || isQueueOpen || isMoreMenuOpen) {
          closeAllPopups();
          return;
        }
      }
      const target = e.target as HTMLElement | null;
      if (target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA') return;
      if (e.key === 'f' || e.key === 'F') {
        setIsFullscreenOpen((prev) => !prev);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isLyricsOpen, isEqOpen, isQueueOpen, isMoreMenuOpen]);

  useEffect(() => {
    const anyOpen = isLyricsOpen || isEqOpen || isQueueOpen || isMoreMenuOpen;
    if (!anyOpen) return;

    const handlePointerDown = (e: MouseEvent | TouchEvent) => {
      const target = e.target as HTMLElement | null;
      if (!target) return;
      if (target.closest('[data-player-popup], [data-player-trigger]')) {
        return;
      }
      closeAllPopups();
    };

    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('touchstart', handlePointerDown);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('touchstart', handlePointerDown);
    };
  }, [isLyricsOpen, isEqOpen, isQueueOpen, isMoreMenuOpen]);

  if (!currentTrack) {
    return null;
  }

  const isBuffering = playerState === 'buffering' || playerState === 'connecting';
  const isPlaying = playerState === 'playing';
  const isStalled = playerState === 'stalled';

  const fallbackCoverUri = generateCoverDataUri(currentTrack.content_id, currentTrack.title);
  const coverArtUri = currentTrack.has_cover ? apiClient.getCoverUrl(currentTrack.content_id) : fallbackCoverUri;

  return (
    <>
      {/* Floating Synchronized Lyrics Drawer */}
      <LyricsDrawer
        currentTrack={currentTrack}
        currentTime={currentTime}
        isOpen={isLyricsOpen}
        onClose={() => setIsLyricsOpen(false)}
        onSeek={onSeek}
      />

      {/* Floating 5-Band Studio Equalizer Drawer */}
      <EqualizerDrawer
        isOpen={isEqOpen}
        onClose={() => setIsEqOpen(false)}
        isEnabled={eq.isEnabled}
        preset={eq.preset}
        gains={eq.gains}
        onBandGainChange={eq.setBandGain}
        onSelectPreset={eq.selectPreset}
        onToggleBypass={eq.toggleBypass}
        onResetFlat={eq.resetFlat}
      />

      {/* Fullscreen Studio Modal */}
      <FullscreenPlayerModal
        isOpen={isFullscreenOpen}
        onClose={() => setIsFullscreenOpen(false)}
        currentTrack={currentTrack}
        playerState={playerState}
        currentTime={currentTime}
        duration={duration}
        bufferedPct={bufferedPct}
        volume={volume}
        isMuted={isMuted}
        onTogglePlay={onTogglePlay}
        onSeek={onSeek}
        onVolumeChange={onVolumeChange}
        onToggleMute={onToggleMute}
        playbackRate={playbackRate}
        onPlaybackRateChange={onPlaybackRateChange}
        sleepTimerOption={sleepTimer.timerOption}
        sleepRemainingFormatted={sleepTimer.formattedRemaining}
        onStartSleepTimer={sleepTimer.startTimer}
        queue={queue}
        queueIndex={queueIndex}
        isShuffle={isShuffle}
        isRepeat={isRepeat}
        onPlayNext={onPlayNext}
        onPlayPrevious={onPlayPrevious}
        onToggleShuffle={onToggleShuffle}
        onToggleRepeat={onToggleRepeat}
        onSelectQueueTrack={onSelectQueueTrack}
        analyser={eq.analyser}
        equalizerProps={{
          isEnabled: eq.isEnabled,
          preset: eq.preset,
          gains: eq.gains,
          onBandGainChange: eq.setBandGain,
          onSelectPreset: eq.selectPreset,
          onToggleBypass: eq.toggleBypass,
          onResetFlat: eq.resetFlat,
        }}
      />

      {/* Floating Playback Queue Drawer */}
      {isQueueOpen && (
        <div
          data-player-popup="queue"
          style={{
            position: 'fixed',
            bottom: '96px',
            right: '24px',
            width: '380px',
            maxHeight: '460px',
            backgroundColor: 'rgba(26, 26, 29, 0.98)',
            backdropFilter: 'blur(20px)',
            border: '1px solid var(--border-active)',
            borderRadius: 'var(--radius-lg)',
            boxShadow: '0 16px 40px rgba(0, 0, 0, 0.75)',
            display: 'flex',
            flexDirection: 'column',
            zIndex: 110,
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '14px 18px',
              borderBottom: '1px solid var(--border-glass)',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <ListMusic size={17} style={{ color: 'var(--accent-primary)' }} />
              <span style={{ fontSize: '0.9rem', fontWeight: 600 }}>Up Next in Swarm Queue</span>
              <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>({queue.length})</span>
            </div>
            {queue.length > 0 && onClearQueue && (
              <button
                onClick={onClearQueue}
                style={{
                  background: 'none',
                  border: 'none',
                  color: 'var(--text-muted)',
                  cursor: 'pointer',
                  padding: '4px',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  fontSize: '0.75rem',
                }}
                title="Clear queue"
              >
                <Trash2 size={13} /> Clear
              </button>
            )}
          </div>

          <div
            style={{
              overflowY: 'auto',
              maxHeight: '380px',
              padding: '8px',
              display: 'flex',
              flexDirection: 'column',
              gap: '4px',
            }}
          >
            {queue.length === 0 ? (
              <div style={{ padding: '28px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '0.85rem' }}>
                Queue is empty. Play any track or playlist to queue up tracks!
              </div>
            ) : (
              queue.map((t, idx) => {
                const isCurrent = idx === queueIndex;
                const fallbackTrackCover = generateCoverDataUri(t.content_id, t.title);
                const trackCover = t.has_cover ? apiClient.getCoverUrl(t.content_id) : fallbackTrackCover;
                return (
                  <div
                    key={`${t.content_id}-${idx}`}
                    onClick={() => onSelectQueueTrack?.(t)}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '8px 12px',
                      borderRadius: 'var(--radius-md)',
                      backgroundColor: isCurrent ? 'rgba(249, 115, 22, 0.12)' : 'transparent',
                      border: isCurrent ? '1px solid var(--accent-primary)' : '1px solid transparent',
                      cursor: 'pointer',
                      transition: 'all 0.15s ease',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '10px', overflow: 'hidden' }}>
                      <img
                        src={trackCover}
                        alt=""
                        onError={(e) => {
                          e.currentTarget.src = fallbackTrackCover;
                        }}
                        style={{ width: '32px', height: '32px', borderRadius: '4px', flexShrink: 0, objectFit: 'cover' }}
                      />
                      <div style={{ overflow: 'hidden' }}>
                        <div
                          style={{
                            fontSize: '0.82rem',
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
                            fontSize: '0.72rem',
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
                    {isCurrent && (
                      <Badge variant="primary">Now Playing</Badge>
                    )}
                  </div>
                );
              })
            )}
          </div>
        </div>
      )}

      {/* Main Persistent Audio Bar */}
      <div
        style={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          height: '84px',
          backgroundColor: 'rgba(26, 26, 29, 0.96)',
          backdropFilter: 'blur(16px)',
          borderTop: '1px solid var(--border-glass)',
          boxShadow: '0 -4px 24px rgba(0, 0, 0, 0.4)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 24px',
          zIndex: 100,
          gap: '16px',
        }}
      >
        {/* Left: Track Information with Artwork & Stream Status (Clickable Now-Playing view) */}
        <div
          onClick={() => setIsFullscreenOpen(true)}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              setIsFullscreenOpen(true);
            }
          }}
          title="Open Fullscreen Studio (Now Playing)"
          style={{
            display: 'flex',
            cursor: 'pointer',
            padding: '4px 8px',
            margin: '-4px -8px',
            borderRadius: 'var(--radius-md)',
            transition: 'background-color 0.2s ease',
            alignItems: 'center',
            gap: '12px',
            width: '260px',
            minWidth: '180px',
            maxWidth: '280px',
            flexShrink: 0,
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              width: '46px',
              height: '46px',
              borderRadius: 'var(--radius-md)',
              overflow: 'hidden',
              border: isPlaying ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
              flexShrink: 0,
              
              transition: 'all 0.3s ease',
            }}
          >
            <img
              src={coverArtUri}
              alt="Track artwork"
              onError={(e) => {
                e.currentTarget.src = fallbackCoverUri;
              }}
              style={{ width: '100%', height: '100%', objectFit: 'cover' }}
            />
          </div>
          <div style={{ overflow: 'hidden', minWidth: 0, flex: 1 }}>
            <div
              style={{
                fontSize: '0.88rem',
                fontWeight: 600,
                color: 'var(--text-primary)',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
              title={currentTrack.title}
            >
              {currentTrack.title}
            </div>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                marginTop: '3px',
              }}
            >
              <span
                style={{
                  fontSize: '0.74rem',
                  color: 'var(--text-secondary)',
                  whiteSpace: 'nowrap',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  maxWidth: '120px',
                }}
                title={currentTrack.artist}
              >
                {currentTrack.artist}
              </span>
              {isStalled ? (
                <span
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '3px',
                    fontSize: '0.65rem',
                    color: 'var(--accent-rose)',
                    backgroundColor: 'rgba(244, 63, 94, 0.12)',
                    border: '1px solid rgba(244, 63, 94, 0.25)',
                    padding: '1px 6px',
                    borderRadius: 'var(--radius-full)',
                    whiteSpace: 'nowrap',
                    flexShrink: 0,
                  }}
                >
                  <WifiOff size={10} /> Stalled
                </span>
              ) : isBuffering ? (
                <span
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '4px',
                    fontSize: '0.65rem',
                    color: 'var(--accent-cyan)',
                    backgroundColor: 'rgba(6, 182, 212, 0.12)',
                    border: '1px solid rgba(6, 182, 212, 0.25)',
                    padding: '1px 6px',
                    borderRadius: 'var(--radius-full)',
                    whiteSpace: 'nowrap',
                    flexShrink: 0,
                  }}
                >
                  <Loader2 size={10} style={{ animation: 'spin 1s linear infinite' }} /> Buffering
                </span>
              ) : ((currentTrack.availability_pct ?? 0) >= 100) ? (
                <span
                  title="Audio is 100% cached on this machine. Ready for instant offline playback."
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '4px',
                    fontSize: '0.65rem',
                    color: 'var(--accent-emerald)',
                    backgroundColor: 'rgba(16, 185, 129, 0.12)',
                    border: '1px solid rgba(16, 185, 129, 0.25)',
                    padding: '1px 6px',
                    borderRadius: 'var(--radius-full)',
                    whiteSpace: 'nowrap',
                    flexShrink: 0,
                  }}
                >
                  <HardDrive size={10} /> Local
                </span>
              ) : connectedPeers > 0 ? (
                <span
                  title={`Streaming from P2P swarm (${connectedPeers} peers).`}
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '4px',
                    fontSize: '0.65rem',
                    color: 'var(--accent-primary)',
                    backgroundColor: 'rgba(249, 115, 22, 0.12)',
                    border: '1px solid rgba(249, 115, 22, 0.25)',
                    padding: '1px 6px',
                    borderRadius: 'var(--radius-full)',
                    whiteSpace: 'nowrap',
                    flexShrink: 0,
                  }}
                >
                  <span
                    style={{
                      width: '5px',
                      height: '5px',
                      borderRadius: '50%',
                      backgroundColor: 'var(--accent-primary)',
                    }}
                  />
                  P2P Stream
                </span>
              ) : (
                <span
                  title="No swarm seeders are currently online for this remote track."
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '4px',
                    fontSize: '0.65rem',
                    color: 'var(--accent-amber)',
                    backgroundColor: 'rgba(255, 170, 0, 0.12)',
                    border: '1px solid rgba(255, 170, 0, 0.25)',
                    padding: '1px 6px',
                    borderRadius: 'var(--radius-full)',
                    whiteSpace: 'nowrap',
                    flexShrink: 0,
                  }}
                >
                  <WifiOff size={10} /> No Seeders
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Center: Play Controls & Dual-Buffer Scrubber (Strict Absolute Viewport Centering) */}
        <div
          style={{
            position: 'absolute',
            left: '50%',
            transform: 'translateX(-50%)',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '4px',
            width: '100%',
            maxWidth: '520px',
            padding: '0 12px',
            pointerEvents: 'none',
            zIndex: 105,
          }}
        >
          <div style={{ width: '100%', display: 'flex', flexDirection: 'column', alignItems: 'center', pointerEvents: 'auto' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
            {/* Shuffle Button */}
            {onToggleShuffle && (
              <button
                onClick={onToggleShuffle}
                style={{
                  background: 'none',
                  border: 'none',
                  color: isShuffle ? 'var(--accent-primary)' : 'var(--text-muted)',
                  cursor: 'pointer',
                  padding: '6px',
                  display: 'flex',
                  alignItems: 'center',
                  transition: 'color 0.15s ease',
                }}
                title={isShuffle ? 'Shuffle Enabled' : 'Enable Shuffle'}
              >
                <Shuffle size={16} />
              </button>
            )}

            {/* Previous Track */}
            {onPlayPrevious && (
              <button
                onClick={onPlayPrevious}
                style={{
                  background: 'none',
                  border: 'none',
                  color: 'var(--text-primary)',
                  cursor: 'pointer',
                  padding: '6px',
                  display: 'flex',
                  alignItems: 'center',
                  transition: 'opacity 0.15s ease',
                }}
                title="Previous Track"
              >
                <SkipBack size={18} />
              </button>
            )}

            {/* Play / Pause Primary Button */}
            <button
              onClick={onTogglePlay}
              disabled={isBuffering}
              style={{
                width: '38px',
                height: '38px',
                borderRadius: 'var(--radius-full)',
                backgroundColor: isPlaying ? 'var(--accent-primary)' : 'var(--text-primary)',
                color: isPlaying ? '#ffffff' : '#141416',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                transition: 'all 0.15s ease',
                boxShadow: '0 2px 8px rgba(0, 0, 0, 0.3)',
                cursor: 'pointer',
              }}
            >
              {isBuffering ? (
                <Loader2 size={18} style={{ animation: 'spin 1s linear infinite' }} />
              ) : isPlaying ? (
                <Pause size={18} style={{ fill: 'currentColor' }} />
              ) : (
                <Play size={18} style={{ fill: 'currentColor', marginLeft: '2px' }} />
              )}
            </button>

            {/* Next Track */}
            {onPlayNext && (
              <button
                onClick={onPlayNext}
                style={{
                  background: 'none',
                  border: 'none',
                  color: 'var(--text-primary)',
                  cursor: 'pointer',
                  padding: '6px',
                  display: 'flex',
                  alignItems: 'center',
                  transition: 'opacity 0.15s ease',
                }}
                title="Next Track"
              >
                <SkipForward size={18} />
              </button>
            )}

            {/* Repeat Button */}
            {onToggleRepeat && (
              <button
                onClick={onToggleRepeat}
                style={{
                  background: 'none',
                  border: 'none',
                  color: isRepeat ? 'var(--accent-primary)' : 'var(--text-muted)',
                  cursor: 'pointer',
                  padding: '6px',
                  display: 'flex',
                  alignItems: 'center',
                  transition: 'color 0.15s ease',
                }}
                title={isRepeat ? 'Repeat Enabled' : 'Enable Repeat'}
              >
                <Repeat size={16} />
              </button>
            )}
          </div>

          <WaveformScrubber
            currentTime={currentTime}
            duration={duration}
            bufferedPct={bufferedPct}
            onSeek={onSeek}
          />
          </div>
        </div>

        {/* Right: Studio Audio Suite, Queue & Volume */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            gap: '8px',
            flexShrink: 0,
            flex: '0 0 auto',
            zIndex: 110,
          }}
        >
          {/* Synchronized Lyrics Toggle */}
          <button
            data-player-trigger="lyrics"
            onClick={toggleLyrics}
            style={{
              background: isLyricsOpen ? 'rgba(249, 115, 22, 0.18)' : 'none',
              border: isLyricsOpen ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: isLyricsOpen ? 'var(--accent-primary)' : 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px 9px',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '5px',
              fontSize: '0.75rem',
              transition: 'all 0.15s ease',
              height: '32px',
            }}
            title="Toggle Synchronized Lyrics & Karaoke"
          >
            <Mic2 size={15} />
            <span className="player-btn-label">Lyrics</span>
          </button>

          {/* Studio Equalizer Toggle */}
          <button
            data-player-trigger="eq"
            onClick={toggleEq}
            style={{
              background: isEqOpen ? 'rgba(249, 115, 22, 0.18)' : 'none',
              border: isEqOpen ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: isEqOpen ? 'var(--accent-primary)' : eq.isEnabled && eq.preset !== 'flat' ? 'var(--accent-primary)' : 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px 9px',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '5px',
              fontSize: '0.75rem',
              transition: 'all 0.15s ease',
              height: '32px',
            }}
            title="Toggle 5-Band Studio Equalizer"
          >
            <Sliders size={15} />
            <span className="player-btn-label">EQ</span>
          </button>

          {/* Queue Drawer Toggle */}
          <button
            data-player-trigger="queue"
            onClick={toggleQueue}
            style={{
              background: isQueueOpen ? 'rgba(249, 115, 22, 0.18)' : 'none',
              border: isQueueOpen ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: isQueueOpen ? 'var(--accent-primary)' : 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px 9px',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '5px',
              fontSize: '0.75rem',
              transition: 'all 0.15s ease',
              height: '32px',
            }}
            title="Toggle Swarm Playback Queue"
          >
            <ListMusic size={15} />
            {queue.length > 0 && <span style={{ fontWeight: 600 }}>{queue.length}</span>}
          </button>

          {/* More Audio Tools Popover (Radio, Speed, Timer, Download) */}
          <div style={{ position: 'relative' }}>
            <button
              data-player-trigger="more"
              onClick={toggleMoreMenu}
              style={{
                background: (isRadio || playbackRate !== 1.0 || sleepTimer.timerOption !== 0)
                  ? 'rgba(249, 115, 22, 0.18)'
                  : isMoreMenuOpen
                  ? 'rgba(255, 255, 255, 0.1)'
                  : 'none',
                border: (isRadio || playbackRate !== 1.0 || sleepTimer.timerOption !== 0)
                  ? '1px solid var(--accent-primary)'
                  : '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-sm)',
                color: (isRadio || playbackRate !== 1.0 || sleepTimer.timerOption !== 0)
                  ? 'var(--accent-primary)'
                  : 'var(--text-muted)',
                cursor: 'pointer',
                padding: '6px 8px',
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                transition: 'all 0.15s ease',
                height: '32px',
              }}
              title="More Audio Options (Radio, Speed, Timer, Download)"
            >
              <MoreHorizontal size={16} />
            </button>

            {isMoreMenuOpen && (
              <div
                data-player-popup="more"
                style={{
                  position: 'absolute',
                  bottom: '44px',
                  right: 0,
                  backgroundColor: 'rgba(20, 23, 29, 0.98)',
                  backdropFilter: 'blur(20px)',
                  border: '1px solid var(--border-active)',
                  borderRadius: 'var(--radius-md)',
                  boxShadow: '0 12px 32px rgba(0, 0, 0, 0.8)',
                  padding: '12px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '10px',
                  zIndex: 130,
                  minWidth: '220px',
                }}
              >
                {/* Radio Mode Toggle */}
                {onToggleRadio && (
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', paddingBottom: '8px', borderBottom: '1px solid var(--border-glass)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '0.78rem', color: 'var(--text-primary)' }}>
                      <Radio size={14} style={{ color: isRadio ? 'var(--accent-primary)' : 'var(--text-muted)' }} />
                      <span>Swarm Radio</span>
                    </div>
                    <button
                      type="button"
                      onClick={() => onToggleRadio()}
                      style={{
                        padding: '2px 8px',
                        fontSize: '0.72rem',
                        borderRadius: 'var(--radius-sm)',
                        border: isRadio ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                        backgroundColor: isRadio ? 'rgba(249, 115, 22, 0.2)' : 'transparent',
                        color: isRadio ? 'var(--accent-primary)' : 'var(--text-muted)',
                        cursor: 'pointer',
                      }}
                    >
                      {isRadio ? 'Active' : 'Off'}
                    </button>
                  </div>
                )}

                {/* Playback Speed */}
                <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', paddingBottom: '8px', borderBottom: '1px solid var(--border-glass)' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '0.78rem', color: 'var(--text-primary)' }}>
                    <Gauge size={14} style={{ color: playbackRate !== 1.0 ? 'var(--accent-primary)' : 'var(--text-muted)' }} />
                    <span>Speed: <strong style={{ color: 'var(--accent-primary)' }}>{playbackRate}x</strong></span>
                  </div>
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: '3px' }}>
                    {[0.5, 0.75, 1.0, 1.25, 1.5, 2.0].map((rate) => (
                      <button
                        key={rate}
                        type="button"
                        onClick={() => onPlaybackRateChange?.(rate)}
                        style={{
                          padding: '3px 0',
                          fontSize: '0.68rem',
                          borderRadius: 'var(--radius-xs)',
                          border: playbackRate === rate ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                          backgroundColor: playbackRate === rate ? 'rgba(249, 115, 22, 0.2)' : 'transparent',
                          color: playbackRate === rate ? 'var(--accent-primary)' : 'var(--text-secondary)',
                          cursor: 'pointer',
                          textAlign: 'center',
                        }}
                      >
                        {rate}x
                      </button>
                    ))}
                  </div>
                </div>

                {/* Sleep Timer */}
                <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', paddingBottom: '8px', borderBottom: '1px solid var(--border-glass)' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '0.78rem', color: 'var(--text-primary)' }}>
                    <Timer size={14} style={{ color: sleepTimer.timerOption !== 0 ? 'var(--accent-primary)' : 'var(--text-muted)' }} />
                    <span>Sleep Timer: <strong style={{ color: 'var(--accent-primary)' }}>{sleepTimer.formattedRemaining || 'Off'}</strong></span>
                  </div>
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '4px' }}>
                    {[
                      { label: 'Off', val: 0 },
                      { label: '15m', val: 15 },
                      { label: '30m', val: 30 },
                      { label: '45m', val: 45 },
                      { label: '60m', val: 60 },
                      { label: 'End', val: -1 },
                    ].map((opt) => (
                      <button
                        key={opt.val}
                        type="button"
                        onClick={() => sleepTimer.startTimer(opt.val as any)}
                        style={{
                          padding: '3px 0',
                          fontSize: '0.68rem',
                          borderRadius: 'var(--radius-xs)',
                          border: sleepTimer.timerOption === opt.val ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                          backgroundColor: sleepTimer.timerOption === opt.val ? 'rgba(249, 115, 22, 0.2)' : 'transparent',
                          color: sleepTimer.timerOption === opt.val ? 'var(--accent-primary)' : 'var(--text-secondary)',
                          cursor: 'pointer',
                          textAlign: 'center',
                        }}
                      >
                        {opt.label}
                      </button>
                    ))}
                  </div>
                </div>

                {/* Direct Audio Download */}
                <a
                  href={apiClient.getDownloadUrl(currentTrack.content_id)}
                  download
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '6px',
                    fontSize: '0.76rem',
                    color: 'var(--text-secondary)',
                    textDecoration: 'none',
                    padding: '4px 6px',
                    borderRadius: 'var(--radius-sm)',
                    transition: 'all 0.15s ease',
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.05)';
                    e.currentTarget.style.color = 'var(--text-primary)';
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.backgroundColor = 'transparent';
                    e.currentTarget.style.color = 'var(--text-secondary)';
                  }}
                >
                  <Download size={14} style={{ color: 'var(--accent-sky)' }} />
                  <span>Download Audio (.mp3)</span>
                </a>
              </div>
            )}
          </div>

          {/* Output Audio Volume Control */}
          <VolumeControl
            volume={volume}
            isMuted={isMuted}
            onVolumeChange={onVolumeChange}
            onToggleMute={onToggleMute}
          />

          {/* Fullscreen Studio Modal Launch Button */}
          <button
            onClick={() => setIsFullscreenOpen(true)}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              borderRadius: 'var(--radius-sm)',
              transition: 'color 0.15s ease, background-color 0.15s ease',
            }}
            title="Open Fullscreen Studio (Now Playing)"
            onMouseEnter={(e) => {
              e.currentTarget.style.color = 'var(--text-primary)';
              e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.06)';
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.color = 'var(--text-muted)';
              e.currentTarget.style.backgroundColor = 'transparent';
            }}
          >
            <Maximize2 size={16} />
          </button>
        </div>
      </div>

    </>
  );};
