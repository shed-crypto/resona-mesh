import { useState, useCallback } from 'react';
import { Waves, ShieldAlert, ListMusic } from 'lucide-react';
import { useAudioPlayer } from './shared/hooks/useAudioPlayer';
import { usePlaylists } from './shared/hooks/usePlaylists';
import { CatalogView } from './features/library/CatalogView';
import { AudioPlayerBar } from './features/player/AudioPlayerBar';
import { NodeStatusPanel } from './features/network/NodeStatusPanel';
import { ReportTrackModal } from './features/moderation/ReportTrackModal';
import { ModerationModal } from './features/moderation/ModerationModal';
import { PlaylistsModal } from './features/playlists/PlaylistsModal';
import { AddToPlaylistModal } from './features/playlists/AddToPlaylistModal';
import { Button } from './shared/components/Button';
import type { Track } from './types/track';

export function App() {
  const [reportTrack, setReportTrack] = useState<Track | null>(null);
  const [isModerationOpen, setIsModerationOpen] = useState(false);
  const [isPlaylistsOpen, setIsPlaylistsOpen] = useState(false);
  const [playlistTrackTarget, setPlaylistTrackTarget] = useState<Track | null>(null);
  const [allKnownTracks, setAllKnownTracks] = useState<Track[]>([]);
  const [catalogRefreshTrigger, setCatalogRefreshTrigger] = useState(0);

  const {
    audioElement,
    currentTrack,
    playerState,
    currentTime,
    duration,
    bufferedPct,
    volume,
    isMuted,
    playTrack,
    togglePlay,
    seek,
    playbackRate,
    setPlaybackRate,
    setVolume,
    setIsMuted,
    queue,
    queueIndex,
    isShuffle,
    isRepeat,
    playNext,
    playPrevious,
    toggleShuffle,
    toggleRepeat,
    clearQueue,
  } = useAudioPlayer();

  const {
    playlists,
    createPlaylist,
    deletePlaylist,
    addTrackToPlaylist,
    removeTrackFromPlaylist,
    exportPlaylistManifest,
    importPlaylistManifest,
  } = usePlaylists();

  const handleTracksLoaded = useCallback((loaded: Track[]) => {
    setAllKnownTracks((prev) => {
      const map = new Map(prev.map((t) => [t.content_id, t]));
      for (const t of loaded) {
        map.set(t.content_id, t);
      }
      return Array.from(map.values());
    });
  }, []);

  const handlePlayPlaylistTracks = useCallback((tracks: Track[]) => {
    if (tracks.length > 0) {
      playTrack(tracks[0], tracks);
    }
  }, [playTrack]);

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        minHeight: '100vh',
        backgroundColor: 'var(--bg-canvas)',
        color: 'var(--text-primary)',
        paddingBottom: currentTrack ? '104px' : '40px',
      }}
    >
      {/* Top Application Header */}
      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: '16px',
          padding: '18px 32px',
          borderBottom: '1px solid var(--border-glass)',
          backgroundColor: 'var(--bg-surface)',
          backdropFilter: 'blur(12px)',
          position: 'sticky',
          top: 0,
          zIndex: 50,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <div
            style={{
              width: '36px',
              height: '36px',
              borderRadius: 'var(--radius-md)',
              backgroundColor: 'rgba(249, 115, 22, 0.12)',
              border: '1px solid rgba(249, 115, 22, 0.25)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--accent-primary)',
            }}
          >
            <Waves size={20} />
          </div>
          <div>
            <h1 style={{ fontSize: '1.15rem', fontWeight: 700, letterSpacing: '-0.02em', lineHeight: 1.2 }}>
              ResonaMesh
            </h1>
            <p style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
              Decentralized Hi-Fi Audio
            </p>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setIsPlaylistsOpen(true)}
            icon={<ListMusic size={15} style={{ color: 'var(--accent-primary)' }} />}
          >
            Playlists ({playlists.length})
          </Button>

          <Button
            variant="ghost"
            size="sm"
            onClick={() => setIsModerationOpen(true)}
            icon={<ShieldAlert size={15} style={{ color: 'var(--accent-amber)' }} />}
          >
            Policy & Reports
          </Button>
          <NodeStatusPanel />
        </div>
      </header>

      {/* Main Catalog View Container */}
      <main
        style={{
          flex: 1,
          maxWidth: '1280px',
          width: '100%',
          margin: '0 auto',
          padding: '28px 24px',
        }}
      >
        <CatalogView
          currentTrack={currentTrack}
          isPlaying={playerState === 'playing'}
          onPlayTrack={(track) => playTrack(track)}
          onTogglePlay={togglePlay}
          onReportTrack={(t) => setReportTrack(t)}
          onAddToPlaylist={(t) => setPlaylistTrackTarget(t)}
          onOpenPlaylists={() => setIsPlaylistsOpen(true)}
          playlistsCount={playlists.length}
          onTracksLoaded={handleTracksLoaded}
          refreshTrigger={catalogRefreshTrigger}
        />
      </main>

      {/* Bottom Floating Player Dock with Queue & Artwork */}
      <AudioPlayerBar
        audioElement={audioElement}
        currentTrack={currentTrack}
        playerState={playerState}
        currentTime={currentTime}
        duration={duration}
        bufferedPct={bufferedPct}
        volume={volume}
        isMuted={isMuted}
        onTogglePlay={togglePlay}
        onSeek={seek}
        playbackRate={playbackRate}
        onPlaybackRateChange={setPlaybackRate}
        onVolumeChange={setVolume}
        onToggleMute={() => setIsMuted(!isMuted)}
        queue={queue}
        queueIndex={queueIndex}
        isShuffle={isShuffle}
        isRepeat={isRepeat}
        onPlayNext={playNext}
        onPlayPrevious={playPrevious}
        onToggleShuffle={toggleShuffle}
        onToggleRepeat={toggleRepeat}
        onSelectQueueTrack={(t) => playTrack(t, queue)}
        onClearQueue={clearQueue}
      />

      {/* Playlists Management Modal */}
      <PlaylistsModal
        isOpen={isPlaylistsOpen}
        onClose={() => setIsPlaylistsOpen(false)}
        playlists={playlists}
        allTracks={allKnownTracks}
        onCreatePlaylist={createPlaylist}
        onDeletePlaylist={deletePlaylist}
        onRemoveTrack={removeTrackFromPlaylist}
        onExportPlaylist={exportPlaylistManifest}
        onImportPlaylist={importPlaylistManifest}
        onPlayTracks={handlePlayPlaylistTracks}
      />

      {/* Add To Playlist Modal */}
      <AddToPlaylistModal
        track={playlistTrackTarget}
        isOpen={Boolean(playlistTrackTarget)}
        onClose={() => setPlaylistTrackTarget(null)}
        playlists={playlists}
        onAddToPlaylist={addTrackToPlaylist}
        onCreateAndAdd={(title, trackId) => {
          const pl = createPlaylist(title);
          addTrackToPlaylist(pl.id, trackId);
        }}
      />

      {/* Moderation & Abuse Reporting Modals */}
      <ReportTrackModal
        track={reportTrack}
        isOpen={Boolean(reportTrack)}
        onClose={() => setReportTrack(null)}
      />

      <ModerationModal
        isOpen={isModerationOpen}
        onClose={() => setIsModerationOpen(false)}
        onDecisionApplied={() => setCatalogRefreshTrigger((prev) => prev + 1)}
      />
    </div>
  );
}

export default App;
