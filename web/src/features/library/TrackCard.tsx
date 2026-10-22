import React from 'react';
import { Play, Pause, Pin, Trash2, ShieldCheck } from 'lucide-react';
import type { Track } from '../../types/track';
import { Badge } from '../../shared/components/Badge';

interface TrackCardProps {
  track: Track;
  isPlaying: boolean;
  isActiveTrack: boolean;
  onPlay: () => void;
  onTogglePin: () => void;
  onDelete: () => void;
}

export const TrackCard: React.FC<TrackCardProps> = ({
  track,
  isPlaying,
  isActiveTrack,
  onPlay,
  onTogglePin,
  onDelete,
}) => {
  const formatDuration = (ms: number) => {
    if (!ms) return '0:00';
    const totalSeconds = Math.floor(ms / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${minutes}:${seconds.toString().padStart(2, '0')}`;
  };

  const formatBytes = (bytes: number) => {
    if (bytes < 1024 * 1024) {
      return `${(bytes / 1024).toFixed(1)} KiB`;
    }
    return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
  };

  const isFullAvailable = track.availability_pct >= 99.9;

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '14px 18px',
        backgroundColor: isActiveTrack ? 'var(--bg-hover)' : 'var(--bg-surface)',
        border: isActiveTrack
          ? '1px solid var(--accent-emerald)'
          : '1px solid var(--border-glass)',
        borderRadius: 'var(--radius-lg)',
        boxShadow: isActiveTrack ? '0 0 20px rgba(16, 185, 129, 0.15)' : 'none',
        transition: 'all 0.2s ease',
        gap: '16px',
      }}
    >
      {/* Play Button & Title Section */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '14px', flex: 1, minWidth: 0 }}>
        <button
          onClick={onPlay}
          style={{
            width: '42px',
            height: '42px',
            borderRadius: 'var(--radius-full)',
            backgroundColor: isActiveTrack && isPlaying ? 'var(--accent-emerald)' : 'var(--bg-card)',
            color: isActiveTrack && isPlaying ? '#000' : 'var(--text-primary)',
            border: '1px solid var(--border-glass)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
            transition: 'all 0.15s ease',
          }}
        >
          {isActiveTrack && isPlaying ? (
            <Pause size={18} style={{ fill: 'currentColor' }} />
          ) : (
            <Play size={18} style={{ fill: 'currentColor', marginLeft: '2px' }} />
          )}
        </button>

        <div style={{ minWidth: 0, flex: 1 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <h3
              style={{
                fontSize: '0.95rem',
                fontWeight: 600,
                color: isActiveTrack ? 'var(--accent-emerald)' : 'var(--text-primary)',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {track.title}
            </h3>
            {track.signature && (
              <span title="Cryptographically signed with Ed25519" style={{ display: 'inline-flex', alignItems: 'center' }}>
                <ShieldCheck
                  size={14}
                  style={{ color: 'var(--accent-emerald)', flexShrink: 0 }}
                />
              </span>
            )}
          </div>
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              fontSize: '0.8rem',
              color: 'var(--text-secondary)',
              marginTop: '2px',
            }}
          >
            <span style={{ fontWeight: 500 }}>{track.artist}</span>
            <span>•</span>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>
              {formatDuration(track.duration_ms)}
            </span>
            <span>•</span>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>
              {track.bitrate_kbps} kbps
            </span>
          </div>
        </div>
      </div>

      {/* Availability & Technical Metadata */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexShrink: 0 }}>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: '4px' }}>
          <Badge
            variant={
              isFullAvailable
                ? 'emerald'
                : track.availability_pct > 0
                ? 'amber'
                : 'neutral'
            }
          >
            {isFullAvailable
              ? '100% Seeded'
              : track.availability_pct > 0
              ? `${track.availability_pct.toFixed(0)}% Cached`
              : '0% Remote Swarm'}
          </Badge>
          <span
            style={{
              fontFamily: 'var(--font-mono)',
              fontSize: '0.7rem',
              color: 'var(--text-muted)',
            }}
          >
            {track.block_count} chunks • {formatBytes(track.total_size_bytes)}
          </span>
        </div>

        <Badge variant="neutral">{track.license_code || 'CC-BY-4.0'}</Badge>

        {/* Action Controls */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
          <button
            onClick={onTogglePin}
            title={track.pinned ? 'Pinned (retained in storage)' : 'Pin track'}
            style={{
              padding: '8px',
              borderRadius: 'var(--radius-md)',
              color: track.pinned ? 'var(--accent-emerald)' : 'var(--text-muted)',
              backgroundColor: track.pinned ? 'rgba(16, 185, 129, 0.1)' : 'transparent',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Pin size={16} style={{ fill: track.pinned ? 'currentColor' : 'none' }} />
          </button>

          <button
            onClick={onDelete}
            title="Delete from local node"
            style={{
              padding: '8px',
              borderRadius: 'var(--radius-md)',
              color: 'var(--text-muted)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Trash2 size={16} />
          </button>
        </div>
      </div>
    </div>
  );
};
