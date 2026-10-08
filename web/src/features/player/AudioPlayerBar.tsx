import React from 'react';
import { Play, Pause, Loader2, Music, WifiOff } from 'lucide-react';
import type { Track } from '../../types/track';
import type { PlayerState } from '../../shared/hooks/useAudioPlayer';
import { WaveformScrubber } from './WaveformScrubber';
import { VolumeControl } from './VolumeControl';
import { Badge } from '../../shared/components/Badge';

interface AudioPlayerBarProps {
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
}

export const AudioPlayerBar: React.FC<AudioPlayerBarProps> = ({
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
}) => {
  if (!currentTrack) {
    return null;
  }

  const isBuffering = playerState === 'buffering' || playerState === 'connecting';
  const isPlaying = playerState === 'playing';
  const isStalled = playerState === 'stalled';

  return (
    <div
      style={{
        position: 'fixed',
        bottom: 0,
        left: 0,
        right: 0,
        height: '84px',
        backgroundColor: 'rgba(20, 23, 29, 0.95)',
        backdropFilter: 'blur(16px)',
        borderTop: '1px solid var(--border-active)',
        boxShadow: '0 -8px 32px rgba(0, 0, 0, 0.6)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '0 24px',
        zIndex: 100,
        gap: '24px',
      }}
    >
      {/* Left: Track Information */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '14px', width: '280px', minWidth: '200px' }}>
        <div
          style={{
            width: '46px',
            height: '46px',
            borderRadius: 'var(--radius-md)',
            backgroundColor: 'var(--bg-card)',
            border: '1px solid var(--border-glass)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: isPlaying ? 'var(--accent-emerald)' : 'var(--text-muted)',
            flexShrink: 0,
          }}
        >
          <Music size={22} />
        </div>
        <div style={{ overflow: 'hidden' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span
              style={{
                fontSize: '0.9rem',
                fontWeight: 600,
                color: 'var(--text-primary)',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {currentTrack.title}
            </span>
          </div>
          <div
            style={{
              fontSize: '0.78rem',
              color: 'var(--text-secondary)',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              marginTop: '2px',
            }}
          >
            {currentTrack.artist}
          </div>
        </div>
      </div>

      {/* Center: Play Controls & Dual-Buffer Scrubber */}
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          gap: '4px',
          flex: 1,
          maxWidth: '720px',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
          <button
            onClick={onTogglePlay}
            disabled={isBuffering}
            style={{
              width: '38px',
              height: '38px',
              borderRadius: 'var(--radius-full)',
              backgroundColor: isPlaying ? 'var(--accent-emerald)' : 'var(--text-primary)',
              color: '#000',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              transition: 'all 0.15s ease',
              boxShadow: isPlaying ? '0 0 16px rgba(16, 185, 129, 0.4)' : 'none',
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
        </div>

        <WaveformScrubber
          currentTime={currentTime}
          duration={duration}
          bufferedPct={bufferedPct}
          onSeek={onSeek}
        />
      </div>

      {/* Right: Signal Status & Volume Control */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          gap: '16px',
          width: '280px',
          minWidth: '200px',
        }}
      >
        {isStalled ? (
          <Badge variant="rose">
            <WifiOff size={12} /> Stalled
          </Badge>
        ) : isBuffering ? (
          <Badge variant="cyan">Buffering P2P</Badge>
        ) : (
          <Badge variant="emerald">HTTP 206 Live</Badge>
        )}

        <VolumeControl
          volume={volume}
          isMuted={isMuted}
          onVolumeChange={onVolumeChange}
          onToggleMute={onToggleMute}
        />
      </div>
    </div>
  );
};
