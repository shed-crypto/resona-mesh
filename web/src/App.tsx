import { Waves } from 'lucide-react';
import { useAudioPlayer } from './shared/hooks/useAudioPlayer';
import { CatalogView } from './features/library/CatalogView';
import { AudioPlayerBar } from './features/player/AudioPlayerBar';
import { NodeStatusPanel } from './features/network/NodeStatusPanel';

export function App() {
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

        <NodeStatusPanel />
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
    </div>
  );
}

export default App;
