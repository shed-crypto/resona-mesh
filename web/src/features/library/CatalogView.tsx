import React, { useState, useEffect } from 'react';
import { Plus, Disc3, RefreshCw, AlertCircle, ListMusic } from 'lucide-react';
import type { Track } from '../../types/track';
import { useCatalog } from '../../shared/hooks/useCatalog';
import { useRecommendations } from '../../shared/hooks/useRecommendations';
import { TrackCard } from './TrackCard';
import { SearchBar } from './SearchBar';
import { ImportModal } from './ImportModal';
import { RecommendationsBar } from './RecommendationsBar';
import { Button } from '../../shared/components/Button';
import { Skeleton } from '../../shared/components/Skeleton';

interface CatalogViewProps {
  currentTrack: Track | null;
  isPlaying: boolean;
  onPlayTrack: (track: Track) => void;
  onTogglePlay?: () => void;
  onReportTrack?: (track: Track) => void;
  onAddToPlaylist?: (track: Track) => void;
  onOpenPlaylists?: () => void;
  playlistsCount?: number;
  onTracksLoaded?: (tracks: Track[]) => void;
  refreshTrigger?: number;
}

export const CatalogView: React.FC<CatalogViewProps> = ({
  currentTrack,
  isPlaying,
  onPlayTrack,
  onTogglePlay,
  onReportTrack,
  onAddToPlaylist,
  onOpenPlaylists,
  playlistsCount = 0,
  onTracksLoaded,
  refreshTrigger,
}) => {
  const {
    tracks,
    total,
    loading,
    error,
    search,
    setSearch,
    pinnedOnly,
    setPinnedOnly,
    refresh,
    togglePin,
    deleteTrack,
  } = useCatalog();

  const [isImportOpen, setIsImportOpen] = useState(false);
  const { recommendedTracks, topArtists } = useRecommendations(tracks);

  useEffect(() => {
    if (refreshTrigger !== undefined && refreshTrigger > 0) {
      refresh();
    }
  }, [refreshTrigger]);

  useEffect(() => {
    if (tracks.length > 0) {
      onTracksLoaded?.(tracks);
    }
  }, [tracks, onTracksLoaded]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '20px', width: '100%' }}>
      {/* Action Header */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: '16px',
        }}
      >
        <SearchBar
          search={search}
          onSearchChange={setSearch}
          pinnedOnly={pinnedOnly}
          onPinnedOnlyChange={setPinnedOnly}
        />

        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          {onOpenPlaylists && (
            <Button
              variant="secondary"
              size="md"
              icon={<ListMusic size={15} style={{ color: 'var(--accent-emerald)' }} />}
              onClick={onOpenPlaylists}
            >
              Playlists ({playlistsCount})
            </Button>
          )}
          <Button
            variant="secondary"
            size="md"
            icon={<RefreshCw size={15} />}
            onClick={refresh}
            title="Refresh Swarm Catalog"
          />
          <Button
            variant="primary"
            size="md"
            icon={<Plus size={16} />}
            onClick={() => setIsImportOpen(true)}
          >
            Import Track
          </Button>
        </div>
      </div>

      {/* Catalog Content States */}
      {loading ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          {[1, 2, 3, 4].map((i) => (
            <Skeleton key={i} height="70px" borderRadius="var(--radius-lg)" />
          ))}
        </div>
      ) : error ? (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '48px 16px',
            backgroundColor: 'var(--bg-surface)',
            border: '1px solid rgba(244, 63, 94, 0.2)',
            borderRadius: 'var(--radius-lg)',
            gap: '12px',
          }}
        >
          <AlertCircle size={36} style={{ color: 'var(--accent-rose)' }} />
          <h3 style={{ fontSize: '1rem', fontWeight: 600 }}>Catalog Synchronization Error</h3>
          <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>{error}</p>
          <Button variant="secondary" size="sm" onClick={refresh}>
            Retry Connection
          </Button>
        </div>
      ) : tracks.length === 0 ? (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '64px 20px',
            backgroundColor: 'var(--bg-surface)',
            border: '1px dashed var(--border-glass)',
            borderRadius: 'var(--radius-lg)',
            gap: '16px',
            textAlign: 'center',
          }}
        >
          <div
            style={{
              width: '56px',
              height: '56px',
              borderRadius: 'var(--radius-full)',
              backgroundColor: 'var(--bg-card)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--text-muted)',
            }}
          >
            <Disc3 size={28} />
          </div>
          <div>
            <h3 style={{ fontSize: '1.05rem', fontWeight: 600, color: 'var(--text-primary)' }}>
              {search ? 'No Matching Tracks in Swarm' : 'No Tracks Seeded Yet'}
            </h3>
            <p
              style={{
                fontSize: '0.85rem',
                color: 'var(--text-secondary)',
                maxWidth: '380px',
                marginTop: '4px',
              }}
            >
              {search
                ? `No audio matches for "${search}". Try searching another title or artist.`
                : 'Your local ResonaMesh catalog is empty. Import audio files to seed chunked blocks across peers.'}
            </p>
          </div>
          {!search && (
            <Button
              variant="primary"
              size="md"
              icon={<Plus size={16} />}
              onClick={() => setIsImportOpen(true)}
            >
              Import Your First Track
            </Button>
          )}
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
          {!search && !pinnedOnly && recommendedTracks.length > 0 && (
            <RecommendationsBar
              tracks={recommendedTracks}
              topArtists={topArtists}
              currentTrack={currentTrack}
              isPlaying={isPlaying}
              onPlayTrack={onPlayTrack}
              onAddToPlaylist={onAddToPlaylist}
            />
          )}

          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              padding: '0 4px',
              fontSize: '0.75rem',
              color: 'var(--text-muted)',
              fontFamily: 'var(--font-mono)',
            }}
          >
            <span>SHOWING {tracks.length} OF {total} AUDIO ASSETS</span>
            <span>SHARDED 256 KIB BLOCKS</span>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
            {tracks.map((track) => (
            <TrackCard
              key={track.content_id}
              track={track}
              isActiveTrack={currentTrack?.content_id === track.content_id}
              isPlaying={isPlaying}
              onPlay={() => {
                if (currentTrack?.content_id === track.content_id && onTogglePlay) {
                  onTogglePlay();
                } else {
                  onPlayTrack(track);
                }
              }}
              onTogglePin={() => togglePin(track.content_id, track.pinned)}
              onDelete={() => deleteTrack(track.content_id)}
              onReport={() => onReportTrack?.(track)}
              onAddToPlaylist={() => onAddToPlaylist?.(track)}
            />
          ))}
          </div>
        </div>
      )}

      {/* Import Modal */}
      <ImportModal
        isOpen={isImportOpen}
        onClose={() => setIsImportOpen(false)}
        onImportSuccess={refresh}
      />
    </div>
  );
};
