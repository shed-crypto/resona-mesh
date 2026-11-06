import React, { useState } from 'react';
import { Play, Pause, Pin, Trash2, ShieldCheck, Flag, Ban, ListPlus, Download } from 'lucide-react';
import type { Track } from '../../types/track';
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
  const [isHovered, setIsHovered] = useState(false);

  const formatDuration = (ms: number) => {
    if (!ms) return '0:00';
    const totalSeconds = Math.floor(ms / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${minutes}:${seconds.toString().padStart(2, '0')}`;
  };

  const fallbackCoverUri = generateCoverDataUri(track.content_id, track.title);
  const coverArtUri = track.has_cover ? apiClient.getCoverUrl(track.content_id) : fallbackCoverUri;

  return (
    <div
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '10px 16px',
        backgroundColor: isActiveTrack
          ? 'rgba(249, 115, 22, 0.08)'
          : isHovered
          ? 'var(--bg-hover)'
          : 'var(--bg-surface)',
        border: isActiveTrack
          ? '1px solid rgba(249, 115, 22, 0.3)'
          : '1px solid var(--border-glass)',
        borderRadius: 'var(--radius-md)',
        transition: 'all 0.15s ease',
        gap: '16px',
      }}
    >
      {/* Play Button, Cover Artwork & Title Section */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flex: 1, minWidth: 0 }}>
        <button
          onClick={track.blocked ? undefined : onPlay}
          disabled={track.blocked}
          title={track.blocked ? `Blocked: ${track.block_reason || 'Policy violation'}` : (isActiveTrack && isPlaying ? 'Pause' : 'Play')}
          style={{
            width: '36px',
            height: '36px',
            borderRadius: 'var(--radius-full)',
            backgroundColor: track.blocked
              ? 'rgba(244, 63, 94, 0.15)'
              : isActiveTrack && isPlaying
              ? 'var(--accent-primary)'
              : isHovered
              ? 'rgba(255, 255, 255, 0.15)'
              : 'rgba(255, 255, 255, 0.08)',
            color: track.blocked
              ? 'var(--accent-rose)'
              : isActiveTrack && isPlaying
              ? '#ffffff'
              : 'var(--text-primary)',
            border: 'none',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
            transition: 'all 0.15s ease',
            cursor: track.blocked ? 'not-allowed' : 'pointer',
          }}
        >
          {track.blocked ? (
            <Ban size={16} />
          ) : isActiveTrack && isPlaying ? (
            <Pause size={16} style={{ fill: 'currentColor' }} />
          ) : (
            <Play size={16} style={{ fill: 'currentColor', marginLeft: '2px' }} />
          )}
        </button>

        {/* Cover Artwork */}
        <div
          style={{
            width: '38px',
            height: '38px',
            borderRadius: 'var(--radius-sm)',
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
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span
              style={{
                fontSize: '0.9rem',
                fontWeight: 600,
                color: track.blocked ? 'var(--text-muted)' : isActiveTrack ? 'var(--accent-primary)' : 'var(--text-primary)',
                textDecoration: track.blocked ? 'line-through' : 'none',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
              title={track.title}
            >
              {track.title}
            </span>
            {track.signature && !track.blocked && (
              <span title="Verified Ed25519 author signature" style={{ display: 'inline-flex', alignItems: 'center' }}>
                <ShieldCheck size={13} style={{ color: 'var(--accent-primary)', flexShrink: 0 }} />
              </span>
            )}
          </div>
          <div
            style={{
              fontSize: '0.78rem',
              color: 'var(--text-secondary)',
              marginTop: '1px',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            {track.artist}
          </div>
        </div>
      </div>

      {/* Audio Quality & Duration Metadata */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '16px', flexShrink: 0 }}>
        {track.genre && (
          <span
            style={{
              fontSize: '0.68rem',
              padding: '2px 6px',
              borderRadius: '4px',
              backgroundColor: 'rgba(249, 115, 22, 0.08)',
              border: '1px solid rgba(249, 115, 22, 0.25)',
              color: 'var(--accent-primary)',
              whiteSpace: 'nowrap',
            }}
          >
            {track.genre}
          </span>
        )}
        {track.bpm && track.bpm > 0 ? (
          <span
            style={{
              fontFamily: 'var(--font-mono)',
              fontSize: '0.68rem',
              padding: '2px 6px',
              borderRadius: '4px',
              backgroundColor: 'rgba(255, 255, 255, 0.04)',
              border: '1px solid var(--border-glass)',
              color: 'var(--text-muted)',
              whiteSpace: 'nowrap',
            }}
          >
            {track.bpm} BPM
          </span>
        ) : null}
        <span
          style={{
            fontFamily: 'var(--font-mono)',
            fontSize: '0.7rem',
            padding: '2px 6px',
            borderRadius: '4px',
            backgroundColor: 'rgba(255, 255, 255, 0.05)',
            border: '1px solid var(--border-glass)',
            color: 'var(--text-secondary)',
          }}
        >
          {track.bitrate_kbps >= 320 ? '320 kbps' : `${track.bitrate_kbps} kbps`}
        </span>

        <span
          style={{
            fontFamily: 'var(--font-mono)',
            fontSize: '0.78rem',
            color: 'var(--text-secondary)',
            minWidth: '36px',
            textAlign: 'right',
          }}
        >
          {formatDuration(track.duration_ms)}
        </span>

        {/* Action Controls */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '2px' }}>
          {onAddToPlaylist && (
            <button
              onClick={onAddToPlaylist}
              title="Add to Playlist"
              style={{
                padding: '6px',
                borderRadius: 'var(--radius-sm)',
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
              <ListPlus size={15} />
            </button>
          )}

          <button
            onClick={onTogglePin}
            title={track.pinned ? 'Pinned (retained in storage)' : 'Pin track'}
            style={{
              padding: '6px',
              borderRadius: 'var(--radius-sm)',
              color: track.pinned ? 'var(--accent-primary)' : 'var(--text-muted)',
              backgroundColor: track.pinned ? 'rgba(249, 115, 22, 0.12)' : 'transparent',
              border: 'none',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Pin size={15} style={{ fill: track.pinned ? 'currentColor' : 'none' }} />
          </button>

          {!track.blocked && (
            <a
              href={apiClient.getDownloadUrl(track.content_id)}
              download
              title="Download audio"
              style={{
                padding: '6px',
                borderRadius: 'var(--radius-sm)',
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
            >
              <Download size={15} />
            </a>
          )}

          {onReport && (
            <button
              onClick={onReport}
              title="Report content"
              style={{
                padding: '6px',
                borderRadius: 'var(--radius-sm)',
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
              <Flag size={15} />
            </button>
          )}

          <button
            onClick={onDelete}
            title="Delete from local node"
            style={{
              padding: '6px',
              borderRadius: 'var(--radius-sm)',
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
            <Trash2 size={15} />
          </button>
        </div>
      </div>
    </div>
  );
};
