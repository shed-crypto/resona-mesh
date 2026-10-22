import { useState } from 'react';
import { Waves, ShieldAlert } from 'lucide-react';
import { useAudioPlayer } from './shared/hooks/useAudioPlayer';
import { CatalogView } from './features/library/CatalogView';
import { AudioPlayerBar } from './features/player/AudioPlayerBar';
import { NodeStatusPanel } from './features/network/NodeStatusPanel';
import { ReportTrackModal } from './features/moderation/ReportTrackModal';
import { ModerationModal } from './features/moderation/ModerationModal';
import { Button } from './shared/components/Button';
import type { Track } from './types/track';

export function App() {
  const [reportTrack, setReportTrack] = useState<Track | null>(null);
  const [isModerationOpen, setIsModerationOpen] = useState(false);

  const {
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
    setVolume,
    setIsMuted,
  } = useAudioPlayer();

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
              backgroundColor: 'var(--bg-card)',
              border: '1px solid var(--accent-emerald)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--accent-emerald)',
              boxShadow: '0 0 12px rgba(16, 185, 129, 0.25)',
            }}
          >
            <Waves size={20} />
          </div>
          <div>
            <h1 style={{ fontSize: '1.15rem', fontWeight: 700, letterSpacing: '-0.02em', lineHeight: 1.2 }}>
              ResonaMesh
            </h1>
            <p style={{ fontSize: '0.72rem', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
              DECENTRALIZED AUDIO PROTOCOL
            </p>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
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
          onPlayTrack={playTrack}
          onTogglePlay={togglePlay}
          onReportTrack={(t) => setReportTrack(t)}
        />
      </main>

      {/* Bottom Floating Player Dock */}
      <AudioPlayerBar
        currentTrack={currentTrack}
        playerState={playerState}
        currentTime={currentTime}
        duration={duration}
        bufferedPct={bufferedPct}
        volume={volume}
        isMuted={isMuted}
        onTogglePlay={togglePlay}
        onSeek={seek}
        onVolumeChange={setVolume}
        onToggleMute={() => setIsMuted(!isMuted)}
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
      />
    </div>
  );
}

export default App;
