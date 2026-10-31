import React from 'react';
import { Sparkles, Play, ListPlus, ShieldCheck } from 'lucide-react';
import type { Track } from '../../types/track';
import { generateCoverDataUri } from '../../shared/utils/artwork';
import { apiClient } from '../../shared/api/client';

interface RecommendationsBarProps {
  tracks: Track[];
  topArtists: string[];
  currentTrack: Track | null;
  isPlaying: boolean;
  onPlayTrack: (track: Track) => void;
  onAddToPlaylist?: (track: Track) => void;
}

export const RecommendationsBar: React.FC<RecommendationsBarProps> = ({
  tracks,
  topArtists,
  currentTrack,
  isPlaying,
  onPlayTrack,
  onAddToPlaylist,
}) => {
  if (!tracks || tracks.length === 0) return null;

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: '12px',
        padding: '16px 18px',
        background: 'linear-gradient(135deg, rgba(20, 23, 29, 0.85) 0%, rgba(11, 13, 16, 0.95) 100%)',
        border: '1px solid var(--border-glass)',
        borderRadius: 'var(--radius-lg)',
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.3)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '8px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Sparkles size={16} style={{ color: 'var(--accent-cyan)' }} />
          <h2 style={{ fontSize: '0.9rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            Recommended For You
          </h2>
          {topArtists.length > 0 && (
            <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
              (based on your affinity for <strong style={{ color: 'var(--text-primary)' }}>{topArtists.join(', ')}</strong>)
            </span>
          )}
        </div>

        <span
          title="Computed strictly inside your browser from local playback stats. Zero telemetry sent over P2P wire."
          style={{
            fontSize: '0.68rem',
            fontFamily: 'var(--font-mono)',
            padding: '2px 8px',
            borderRadius: 'var(--radius-full)',
            backgroundColor: 'rgba(0, 240, 255, 0.08)',
            border: '1px solid rgba(0, 240, 255, 0.25)',
            color: 'var(--accent-cyan)',
            letterSpacing: '0.4px',
          }}
        >
          PRIVACY-PRESERVING VECTOR ENGINE
        </span>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))',
          gap: '12px',
        }}
      >
        {tracks.map((track) => {
          const isActive = currentTrack?.content_id === track.content_id;
          const fallbackCoverUri = generateCoverDataUri(track.content_id, track.title);
          const coverUri = track.has_cover ? apiClient.getCoverUrl(track.content_id) : fallbackCoverUri;

          return (
            <div
              key={track.content_id}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '10px',
                padding: '8px 12px',
                backgroundColor: isActive ? 'rgba(0, 240, 255, 0.08)' : 'rgba(255, 255, 255, 0.03)',
                border: isActive ? '1px solid rgba(0, 240, 255, 0.3)' : '1px solid rgba(255, 255, 255, 0.06)',
                borderRadius: 'var(--radius-md)',
                transition: 'all 0.15s ease',
              }}
            >
              <div
                style={{
                  width: '36px',
                  height: '36px',
                  borderRadius: 'var(--radius-md)',
                  overflow: 'hidden',
                  flexShrink: 0,
                  position: 'relative',
                }}
              >
                <img
                  src={coverUri}
                  alt=""
                  onError={(e) => {
                    e.currentTarget.src = fallbackCoverUri;
                  }}
                  style={{ width: '100%', height: '100%', objectFit: 'cover' }}
                />
              </div>

              <div style={{ flex: 1, minWidth: 0 }}>
                <div
                  style={{
                    fontSize: '0.82rem',
                    fontWeight: 600,
                    color: isActive ? 'var(--accent-cyan)' : 'var(--text-primary)',
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                  }}
                  title={track.title}
                >
                  {track.title}
                </div>
                <div
                  style={{
                    fontSize: '0.72rem',
                    color: 'var(--text-secondary)',
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                  }}
                >
                  <span>{track.artist}</span>
                  {track.signature && (
                    <ShieldCheck size={11} style={{ color: 'var(--accent-emerald)', flexShrink: 0 }} />
                  )}
                </div>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <button
                  onClick={() => onPlayTrack(track)}
                  title={isActive && isPlaying ? 'Pause' : 'Play Track'}
                  style={{
                    width: '28px',
                    height: '28px',
                    borderRadius: 'var(--radius-full)',
                    backgroundColor: isActive ? 'var(--accent-cyan)' : 'rgba(255, 255, 255, 0.08)',
                    color: isActive ? '#0b0d10' : 'var(--text-primary)',
                    border: 'none',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    cursor: 'pointer',
                    transition: 'transform 0.1s ease',
                  }}
                >
                  <Play size={12} style={{ fill: 'currentColor', marginLeft: '1px' }} />
                </button>

                {onAddToPlaylist && (
                  <button
                    onClick={() => onAddToPlaylist(track)}
                    title="Add to Playlist"
                    style={{
                      width: '26px',
                      height: '26px',
                      borderRadius: 'var(--radius-md)',
                      backgroundColor: 'transparent',
                      color: 'var(--text-muted)',
                      border: 'none',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      cursor: 'pointer',
                    }}
                  >
                    <ListPlus size={13} />
                  </button>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
