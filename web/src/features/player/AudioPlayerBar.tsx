import React, { useState } from 'react';
import {
  Play,
  Pause,
  Loader2,
  WifiOff,
  SkipBack,
  SkipForward,
  Shuffle,
  Repeat,
  ListMusic,
  Trash2,
  Download,
} from 'lucide-react';
import type { Track } from '../../types/track';
import type { PlayerState } from '../../shared/hooks/useAudioPlayer';
import { WaveformScrubber } from './WaveformScrubber';
import { VolumeControl } from './VolumeControl';
import { Badge } from '../../shared/components/Badge';
import { generateCoverDataUri } from '../../shared/utils/artwork';
import { apiClient } from '../../shared/api/client';

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
  // Queue & Navigation
  queue?: Track[];
  queueIndex?: number;
  isShuffle?: boolean;
  isRepeat?: boolean;
  onPlayNext?: () => void;
  onPlayPrevious?: () => void;
  onToggleShuffle?: () => void;
  onToggleRepeat?: () => void;
  onSelectQueueTrack?: (track: Track) => void;
  onClearQueue?: () => void;
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
  queue = [],
  queueIndex = -1,
  isShuffle = false,
  isRepeat = false,
  onPlayNext,
  onPlayPrevious,
  onToggleShuffle,
  onToggleRepeat,
  onSelectQueueTrack,
  onClearQueue,
}) => {
  const [isQueueOpen, setIsQueueOpen] = useState(false);

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
      {/* Floating Playback Queue Drawer */}
      {isQueueOpen && (
        <div
          style={{
            position: 'fixed',
            bottom: '96px',
            right: '24px',
            width: '380px',
            maxHeight: '460px',
            backgroundColor: 'rgba(16, 19, 26, 0.96)',
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
              <ListMusic size={17} style={{ color: 'var(--accent-emerald)' }} />
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
                      backgroundColor: isCurrent ? 'rgba(16, 185, 129, 0.12)' : 'transparent',
                      border: isCurrent ? '1px solid rgba(16, 185, 129, 0.3)' : '1px solid transparent',
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
                            color: isCurrent ? 'var(--accent-emerald)' : 'var(--text-primary)',
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
                      <Badge variant="emerald">Now Playing</Badge>
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
        {/* Left: Track Information with Procedural Vinyl Art */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '14px', width: '280px', minWidth: '200px' }}>
          <div
            style={{
              width: '48px',
              height: '48px',
              borderRadius: 'var(--radius-md)',
              overflow: 'hidden',
              border: isPlaying ? '1px solid var(--accent-emerald)' : '1px solid var(--border-glass)',
              flexShrink: 0,
              boxShadow: isPlaying ? '0 0 14px rgba(16, 185, 129, 0.25)' : 'none',
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
            gap: '6px',
            flex: 1,
            maxWidth: '720px',
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
                  color: isShuffle ? 'var(--accent-emerald)' : 'var(--text-muted)',
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
                width: '40px',
                height: '40px',
                borderRadius: 'var(--radius-full)',
                backgroundColor: isPlaying ? 'var(--accent-emerald)' : 'var(--text-primary)',
                color: '#000',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                transition: 'all 0.15s ease',
                boxShadow: isPlaying ? '0 0 16px rgba(16, 185, 129, 0.4)' : 'none',
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
                  color: isRepeat ? 'var(--accent-emerald)' : 'var(--text-muted)',
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

        {/* Right: Signal Status, Queue Drawer Toggle & Volume */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            gap: '14px',
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
            <Badge variant="emerald">Live Stream</Badge>
          )}

          {/* Queue Drawer Toggle */}
          <button
            onClick={() => setIsQueueOpen((prev) => !prev)}
            style={{
              background: isQueueOpen ? 'rgba(16, 185, 129, 0.18)' : 'none',
              border: isQueueOpen ? '1px solid var(--accent-emerald)' : '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: isQueueOpen ? 'var(--accent-emerald)' : 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px 8px',
              display: 'flex',
              alignItems: 'center',
              gap: '4px',
              fontSize: '0.75rem',
              transition: 'all 0.15s ease',
            }}
            title="Toggle Swarm Playback Queue"
          >
            <ListMusic size={16} />
            <span>{queue.length}</span>
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
              padding: '6px 8px',
              display: 'flex',
              alignItems: 'center',
              textDecoration: 'none',
              transition: 'all 0.15s ease',
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
            <Download size={16} />
          </a>

          <VolumeControl
            volume={volume}
            isMuted={isMuted}
            onVolumeChange={onVolumeChange}
            onToggleMute={onToggleMute}
          />
        </div>
      </div>
    </>
  );
};
