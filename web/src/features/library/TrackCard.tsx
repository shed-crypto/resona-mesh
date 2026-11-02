import React from 'react';
import { Play, Pause, Pin, Trash2, ShieldCheck, Flag, Ban, ListPlus, Download } from 'lucide-react';
import type { Track } from '../../types/track';
import { Badge } from '../../shared/components/Badge';
import { generateCoverDataUri } from '../../shared/utils/artwork';
import { apiClient } from '../../shared/api/client';

interface TrackCardProps {
  track: Track;
  isPlaying: boolean;
  isActiveTrack: boolean;
  onPlay: () => void;
  onTogglePin: () => void;
  onDelete: () => void;
  onReport?: () => void;
  onAddToPlaylist?: () => void;
}

export const TrackCard: React.FC<TrackCardProps> = ({
  track,
  isPlaying,
  isActiveTrack,
  onPlay,
  onTogglePin,
  onDelete,
  onReport,
  onAddToPlaylist,
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
  const fallbackCoverUri = generateCoverDataUri(track.content_id, track.title);
  const coverArtUri = track.has_cover ? apiClient.getCoverUrl(track.content_id) : fallbackCoverUri;

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
      {/* Play Button, Cover Artwork & Title Section */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '14px', flex: 1, minWidth: 0 }}>
        <button
          onClick={track.blocked ? undefined : onPlay}
          disabled={track.blocked}
          title={track.blocked ? `Blocked: ${track.block_reason || 'Policy violation'}` : (isActiveTrack && isPlaying ? 'Pause' : 'Play')}
          style={{
            width: '42px',
            height: '42px',
            borderRadius: 'var(--radius-full)',
            backgroundColor: track.blocked
              ? 'rgba(244, 63, 94, 0.15)'
              : isActiveTrack && isPlaying
              ? 'var(--accent-emerald)'
              : 'var(--bg-card)',
            color: track.blocked
              ? 'var(--accent-rose)'
              : isActiveTrack && isPlaying
              ? '#000'
              : 'var(--text-primary)',
            border: track.blocked ? '1px solid rgba(244, 63, 94, 0.3)' : '1px solid var(--border-glass)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
            transition: 'all 0.15s ease',
            cursor: track.blocked ? 'not-allowed' : 'pointer',
            opacity: track.blocked ? 0.7 : 1,
          }}
        >
          {track.blocked ? (
            <Ban size={18} />
          ) : isActiveTrack && isPlaying ? (
            <Pause size={18} style={{ fill: 'currentColor' }} />
          ) : (
            <Play size={18} style={{ fill: 'currentColor', marginLeft: '2px' }} />
          )}
        </button>

        {/* Procedural Vinyl Disc Artwork */}
        <div
          style={{
            width: '42px',
            height: '42px',
            borderRadius: 'var(--radius-md)',
            overflow: 'hidden',
            border: '1px solid var(--border-glass)',
            flexShrink: 0,
          }}
        >
          <img
            src={coverArtUri}
            alt=""
            onError={(e) => {
              e.currentTarget.src = fallbackCoverUri;
            }}
            style={{ width: '100%', height: '100%', objectFit: 'cover' }}
          />
        </div>

        <div style={{ minWidth: 0, flex: 1 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <h3
              style={{
                fontSize: '0.95rem',
                fontWeight: 600,
                color: track.blocked ? 'var(--text-muted)' : isActiveTrack ? 'var(--accent-emerald)' : 'var(--text-primary)',
                textDecoration: track.blocked ? 'line-through' : 'none',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {track.title}
            </h3>
            {track.blocked && (
              <span title={track.block_reason || 'Content blocked by policy'}>
                <Badge variant="rose">BLOCKED</Badge>
              </span>
            )}
            {track.signature && !track.blocked && (
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
          {onAddToPlaylist && (
            <button
              onClick={onAddToPlaylist}
              title="Add to Playlist"
              style={{
                padding: '8px',
                borderRadius: 'var(--radius-md)',
                color: 'var(--text-muted)',
                backgroundColor: 'transparent',
                border: 'none',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                transition: 'color 0.15s ease',
              }}
            >
              <ListPlus size={16} />
            </button>
          )}

          <button
            onClick={onTogglePin}
            title={track.pinned ? 'Pinned (retained in storage)' : 'Pin track'}
            style={{
              padding: '8px',
              borderRadius: 'var(--radius-md)',
              color: track.pinned ? 'var(--accent-emerald)' : 'var(--text-muted)',
              backgroundColor: track.pinned ? 'rgba(16, 185, 129, 0.1)' : 'transparent',
              border: 'none',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Pin size={16} style={{ fill: track.pinned ? 'currentColor' : 'none' }} />
          </button>

          {!track.blocked && (
            <a
              href={apiClient.getDownloadUrl(track.content_id)}
              download
              title={`Download ${track.artist ? `${track.artist} - ` : ''}${track.title}.mp3`}
              style={{
                padding: '8px',
                borderRadius: 'var(--radius-md)',
                color: 'var(--text-muted)',
                backgroundColor: 'transparent',
                border: 'none',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                textDecoration: 'none',
                transition: 'color 0.15s ease',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--text-primary)')}
              onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-muted)')}
            >
              <Download size={16} />
            </a>
          )}

          <button
            onClick={onDelete}
            title="Delete from local node"
            style={{
              padding: '8px',
              borderRadius: 'var(--radius-md)',
              color: 'var(--text-muted)',
              backgroundColor: 'transparent',
              border: 'none',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Trash2 size={16} />
          </button>

          {onReport && (
            <button
              onClick={onReport}
              title="Report track policy violation"
              style={{
                padding: '8px',
                borderRadius: 'var(--radius-md)',
                color: 'var(--text-muted)',
                backgroundColor: 'transparent',
                border: 'none',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <Flag size={16} />
            </button>
          )}
        </div>
      </div>
    </div>
  );
};
