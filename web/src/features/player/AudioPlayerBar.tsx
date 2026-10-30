import React, { useState, useEffect } from 'react';
import {
  Play,
  Pause,
  Loader2,
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
} from 'lucide-react';
import type { Track } from '../../types/track';
import type { PlayerState } from '../../shared/hooks/useAudioPlayer';
import { LyricsDrawer } from './LyricsDrawer';
import { EqualizerDrawer } from './EqualizerDrawer';
import { FullscreenPlayerModal } from './FullscreenPlayerModal';
import { useAudioEqualizer } from './useAudioEqualizer';
import { useSleepTimer, type SleepTimerOption } from './useSleepTimer';
import { WaveformScrubber } from './WaveformScrubber';
import { VolumeControl } from './VolumeControl';
import { Badge } from '../../shared/components/Badge';
import { generateCoverDataUri } from '../../shared/utils/artwork';
import { apiClient } from '../../shared/api/client';

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
  const [isRateMenuOpen, setIsRateMenuOpen] = useState(false);
  const [isSleepMenuOpen, setIsSleepMenuOpen] = useState(false);
  const [isFullscreenOpen, setIsFullscreenOpen] = useState(false);

  const closeAllPopups = () => {
    setIsLyricsOpen(false);
    setIsEqOpen(false);
    setIsQueueOpen(false);
    setIsRateMenuOpen(false);
    setIsSleepMenuOpen(false);
  };

  const toggleLyrics = () => {
    const next = !isLyricsOpen;
    closeAllPopups();
    setIsLyricsOpen(next);
  };

  const toggleEq = () => {
    const next = !isEqOpen;
    closeAllPopups();
    setIsEqOpen(next);
  };

  const toggleQueue = () => {
    const next = !isQueueOpen;
    closeAllPopups();
    setIsQueueOpen(next);
  };

  const toggleRateMenu = () => {
    const next = !isRateMenuOpen;
    closeAllPopups();
    setIsRateMenuOpen(next);
  };

  const toggleSleepMenu = () => {
    const next = !isSleepMenuOpen;
    closeAllPopups();
    setIsSleepMenuOpen(next);
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
      const target = e.target as HTMLElement | null;
      if (target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA') return;
      if (e.key === 'f' || e.key === 'F') {
        setIsFullscreenOpen((prev) => !prev);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

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
        {/* Left: Track Information with Artwork & Stream Status */}
        <div
          style={{
            display: 'flex',
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
              ) : (
                <span
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '4px',
                    fontSize: '0.65rem',
                    color: 'var(--text-muted)',
                    backgroundColor: 'rgba(255, 255, 255, 0.05)',
                    border: '1px solid var(--border-glass)',
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
              )}
            </div>
          </div>
        </div>

        {/* Center: Play Controls & Dual-Buffer Scrubber */}
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '4px',
            flex: '1 1 auto',
            minWidth: '240px',
            maxWidth: '560px',
            margin: '0 auto',
            padding: '0 12px',
          }}
        >
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

        {/* Right: Studio Audio Suite, Queue & Volume */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            gap: '6px',
            flexShrink: 0,
            flex: '0 0 auto',
          }}
        >
                    {/* Swarm Radio Autonomous Auto-Queue */}
          {onToggleRadio && (
            <button
              onClick={onToggleRadio}
              style={{
                background: isRadio ? 'rgba(249, 115, 22, 0.18)' : 'none',
                border: isRadio ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-sm)',
                color: isRadio ? 'var(--accent-primary)' : 'var(--text-muted)',
                cursor: 'pointer',
                padding: '6px 8px',
                display: 'inline-flex',
                alignItems: 'center',
                gap: '4px',
                fontSize: '0.75rem',
                transition: 'all 0.15s ease',
                height: '32px',
              }}
              title={isRadio ? 'Swarm Radio: Active (Auto-queues similar tracks)' : 'Enable Swarm Radio (Autonomous auto-queue)'}
            >
              <Radio size={15} />
              <span className="player-btn-label">Radio</span>
            </button>
          )}

          {/* Synchronized Lyrics Toggle */}
          <button
            onClick={toggleLyrics}
            style={{
              background: isLyricsOpen ? 'rgba(249, 115, 22, 0.18)' : 'none',
              border: isLyricsOpen ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: isLyricsOpen ? 'var(--accent-primary)' : 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px 8px',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '4px',
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
            onClick={toggleEq}
            style={{
              background: isEqOpen ? 'rgba(249, 115, 22, 0.18)' : 'none',
              border: isEqOpen ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: isEqOpen ? 'var(--accent-primary)' : eq.isEnabled && eq.preset !== 'flat' ? 'var(--accent-primary)' : 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px 8px',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '4px',
              fontSize: '0.75rem',
              transition: 'all 0.15s ease',
              height: '32px',
            }}
            title="Toggle 5-Band Studio Equalizer"
          >
            <Sliders size={15} />
            <span className="player-btn-label">EQ</span>
          </button>

          {/* Playback Speed Popover */}
          <div style={{ position: 'relative' }}>
            <button
              onClick={toggleRateMenu}
              style={{
                background: playbackRate !== 1.0 ? 'rgba(249, 115, 22, 0.18)' : 'none',
                border: playbackRate !== 1.0 ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-sm)',
                color: playbackRate !== 1.0 ? 'var(--accent-primary)' : 'var(--text-muted)',
                cursor: 'pointer',
                padding: '6px 8px',
                display: 'inline-flex',
                alignItems: 'center',
                gap: '4px',
                fontSize: '0.75rem',
                transition: 'all 0.15s ease',
                height: '32px',
              }}
              title="Adjust Playback Speed"
            >
              <Gauge size={14} />
              <span>{playbackRate}x</span>
            </button>

            {isRateMenuOpen && (
              <div
                style={{
                  position: 'absolute',
                  bottom: '42px',
                  right: 0,
                  backgroundColor: 'rgba(26, 26, 29, 0.98)',
                  backdropFilter: 'blur(20px)',
                  border: '1px solid var(--border-active)',
                  borderRadius: 'var(--radius-md)',
                  boxShadow: '0 8px 24px rgba(0, 0, 0, 0.7)',
                  padding: '6px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '2px',
                  zIndex: 120,
                  minWidth: '80px',
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
                      fontWeight: playbackRate === rate ? 600 : 400,
                      transition: 'all 0.1s ease',
                    }}
                  >
                    {rate}x
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Sleep Timer Popover */}
          <div style={{ position: 'relative' }}>
            <button
              onClick={toggleSleepMenu}
              style={{
                background: sleepTimer.timerOption !== 0 ? 'rgba(249, 115, 22, 0.18)' : 'none',
                border: sleepTimer.timerOption !== 0 ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-sm)',
                color: sleepTimer.timerOption !== 0 ? 'var(--accent-primary)' : 'var(--text-muted)',
                cursor: 'pointer',
                padding: '6px 8px',
                display: 'inline-flex',
                alignItems: 'center',
                gap: '4px',
                fontSize: '0.75rem',
                transition: 'all 0.15s ease',
                height: '32px',
              }}
              title={sleepTimer.timerOption !== 0 ? `Sleep timer: ${sleepTimer.formattedRemaining} remaining` : 'Set Sleep Timer'}
            >
              <Timer size={14} />
              <span className={sleepTimer.timerOption !== 0 ? '' : 'player-btn-label'}>
                {sleepTimer.formattedRemaining || 'Timer'}
              </span>
            </button>

            {isSleepMenuOpen && (
              <div
                style={{
                  position: 'absolute',
                  bottom: '42px',
                  right: 0,
                  backgroundColor: 'rgba(26, 26, 29, 0.98)',
                  backdropFilter: 'blur(20px)',
                  border: '1px solid var(--border-active)',
                  borderRadius: 'var(--radius-md)',
                  boxShadow: '0 8px 24px rgba(0, 0, 0, 0.7)',
                  padding: '6px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '2px',
                  zIndex: 120,
                  minWidth: '120px',
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
                      sleepTimer.startTimer(opt.val);
                      setIsSleepMenuOpen(false);
                    }}
                    style={{
                      background: sleepTimer.timerOption === opt.val ? 'rgba(249, 115, 22, 0.15)' : 'none',
                      border: 'none',
                      borderRadius: 'var(--radius-sm)',
                      color: sleepTimer.timerOption === opt.val ? 'var(--accent-primary)' : 'var(--text-primary)',
                      cursor: 'pointer',
                      padding: '6px 12px',
                      fontSize: '0.75rem',
                      textAlign: 'left',
                      fontWeight: sleepTimer.timerOption === opt.val ? 600 : 400,
                      transition: 'all 0.1s ease',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Queue Drawer Toggle */}
          <button
            onClick={toggleQueue}
            style={{
              background: isQueueOpen ? 'rgba(249, 115, 22, 0.18)' : 'none',
              border: isQueueOpen ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: isQueueOpen ? 'var(--accent-primary)' : 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px 8px',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '4px',
              fontSize: '0.75rem',
              transition: 'all 0.15s ease',
              height: '32px',
            }}
            title="Toggle Swarm Playback Queue"
          >
            <ListMusic size={15} />
            {queue.length > 0 && <span>{queue.length}</span>}
          </button>

          {/* Download Audio Track */}
          <a
            href={apiClient.getDownloadUrl(currentTrack.content_id)}
            download
            style={{
              background: 'none',
              border: '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              textDecoration: 'none',
              transition: 'all 0.15s ease',
              height: '32px',
              width: '32px',
              boxSizing: 'border-box',
            }}
            title={`Download ${currentTrack.artist ? `${currentTrack.artist} - ` : ''}${currentTrack.title}.mp3`}
            onMouseEnter={(e) => {
              e.currentTarget.style.color = 'var(--text-primary)';
              e.currentTarget.style.borderColor = 'var(--border-active)';
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.color = 'var(--text-muted)';
              e.currentTarget.style.borderColor = 'var(--border-glass)';
            }}
          >
            <Download size={15} />
          </a>

          {/* Fullscreen Studio Toggle */}
          <button
            onClick={() => setIsFullscreenOpen(true)}
            style={{
              background: 'none',
              border: '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              transition: 'all 0.15s ease',
              height: '32px',
              width: '32px',
              boxSizing: 'border-box',
            }}
            title="Open Studio Mode (F)"
            onMouseEnter={(e) => {
              e.currentTarget.style.color = 'var(--accent-primary)';
              e.currentTarget.style.borderColor = 'var(--accent-primary)';
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.color = 'var(--text-muted)';
              e.currentTarget.style.borderColor = 'var(--border-glass)';
            }}
          >
            <Maximize2 size={15} />
          </button>

          <VolumeControl
            volume={volume}
            isMuted={isMuted}
            onVolumeChange={onVolumeChange}
            onToggleMute={onToggleMute}
          />
        </div>
      </div>
    </>
  );};
