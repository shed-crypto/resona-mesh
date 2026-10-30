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
        backgroundColor: 'var(--bg-surface)',
        border: '1px solid var(--border-glass)',
        borderRadius: 'var(--radius-lg)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '8px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Sparkles size={16} style={{ color: 'var(--accent-primary)' }} />
          <h2 style={{ fontSize: '0.9rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            Recommended For You
          </h2>
          {topArtists.length > 0 && (
            <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
              (based on your affinity for <strong style={{ color: 'var(--text-primary)' }}>{topArtists.join(', ')}</strong>)
            </span>
          )}
        </div>
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))',
          gap: '10px',
        }}
      >
        {tracks.map((track) => {
          const isActive = currentTrack?.content_id === track.content_id;
          const fallbackCoverUri = generateCoverDataUri(track.content_id, track.title);
          const coverUri = track.has_cover ? apiClient.getCoverUrl(track.content_id) : fallbackCoverUri;

          return (
            <div
              key={`rec-${track.content_id}`}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                padding: '8px 12px',
                backgroundColor: isActive ? 'rgba(249, 115, 22, 0.12)' : 'var(--bg-card)',
                border: isActive ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-md)',
                transition: 'all 0.15s ease',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px', minWidth: 0, flex: 1 }}>
                <div
                  style={{
                    width: '34px',
                    height: '34px',
                    borderRadius: 'var(--radius-sm)',
                    overflow: 'hidden',
                    flexShrink: 0,
                    border: '1px solid var(--border-glass)',
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
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                    <span
                      style={{
                        fontSize: '0.8rem',
                        fontWeight: 600,
                        color: 'var(--text-primary)',
                        whiteSpace: 'nowrap',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                      }}
                      title={track.title}
                    >
                      {track.title}
                    </span>
                    {track.signature && (
                      <ShieldCheck size={12} style={{ color: 'var(--accent-primary)', flexShrink: 0 }} />
                    )}
                  </div>
                  <div
                    style={{
                      fontSize: '0.72rem',
                      color: 'var(--text-secondary)',
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                    }}
                  >
                    {track.artist}
                  </div>
                </div>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '4px', flexShrink: 0 }}>
                {onAddToPlaylist && (
                  <button
                    onClick={() => onAddToPlaylist(track)}
                    title="Add to Playlist"
                    style={{
                      padding: '4px',
                      borderRadius: 'var(--radius-sm)',
                      color: 'var(--text-muted)',
                      backgroundColor: 'transparent',
                      border: 'none',
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                    }}
                  >
                    <ListPlus size={14} />
                  </button>
                )}
                <button
                  onClick={() => onPlayTrack(track)}
                  title={isActive && isPlaying ? 'Pause' : 'Play'}
                  style={{
                    width: '28px',
                    height: '28px',
                    borderRadius: 'var(--radius-full)',
                    backgroundColor: isActive && isPlaying ? 'var(--accent-primary)' : 'rgba(255, 255, 255, 0.08)',
                    color: isActive && isPlaying ? '#ffffff' : 'var(--text-primary)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    border: 'none',
                    cursor: 'pointer',
                    transition: 'all 0.15s ease',
                  }}
                >
                  <Play size={12} style={{ fill: 'currentColor', marginLeft: '1px' }} />
                </button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
