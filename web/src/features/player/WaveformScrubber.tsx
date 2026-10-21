import React, { useRef, useState } from 'react';

interface WaveformScrubberProps {
  currentTime: number;
  duration: number;
  bufferedPct: number;
  onSeek: (seconds: number) => void;
}

export const WaveformScrubber: React.FC<WaveformScrubberProps> = ({
  currentTime,
  duration,
  bufferedPct,
  onSeek,
}) => {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [isDragging, setIsDragging] = useState(false);

  const formatTime = (secs: number) => {
    if (isNaN(secs) || secs < 0) return '0:00';
    const totalSeconds = Math.floor(secs);
    const m = Math.floor(totalSeconds / 60);
    const s = totalSeconds % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  };

  const currentPct = duration > 0 ? Math.min(100, (currentTime / duration) * 100) : 0;

  const handlePointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    setIsDragging(true);
    e.currentTarget.setPointerCapture(e.pointerId);
    handlePointerMove(e);
  };

  const handlePointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!containerRef.current || duration <= 0) return;
    const rect = containerRef.current.getBoundingClientRect();
    const pos = Math.max(0, Math.min(rect.width, e.clientX - rect.left));
    const pct = (pos / rect.width) * 100;

    if (isDragging) {
      const targetTime = (pct / 100) * duration;
      onSeek(targetTime);
    }
  };

  const handlePointerUp = (e: React.PointerEvent<HTMLDivElement>) => {
    if (isDragging) {
      setIsDragging(false);
      try {
        e.currentTarget.releasePointerCapture(e.pointerId);
      } catch {
        // ignore
      }
    }
  };

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '10px', width: '100%' }}>
      <span
        style={{
          fontFamily: 'var(--font-mono)',
          fontSize: '0.75rem',
          color: 'var(--text-secondary)',
          minWidth: '36px',
          textAlign: 'right',
        }}
      >
        {formatTime(currentTime)}
      </span>

      {/* Dual-buffer rail container */}
      <div
        ref={containerRef}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        style={{
          position: 'relative',
          flex: 1,
          height: '24px',
          display: 'flex',
          alignItems: 'center',
          cursor: 'pointer',
        }}
      >
        {/* Background track rail */}
        <div
          style={{
            position: 'absolute',
            left: 0,
            right: 0,
            height: '4px',
            backgroundColor: 'rgba(255, 255, 255, 0.1)',
            borderRadius: 'var(--radius-full)',
            overflow: 'hidden',
          }}
        >
          {/* Middle track: downloaded P2P chunks buffer */}
          <div
            style={{
              position: 'absolute',
              left: 0,
              top: 0,
              bottom: 0,
              width: `${bufferedPct}%`,
              backgroundColor: 'rgba(6, 182, 212, 0.45)',
              transition: 'width 0.2s ease',
            }}
          />

          {/* Top track: playback progress */}
          <div
            style={{
              position: 'absolute',
              left: 0,
              top: 0,
              bottom: 0,
              width: `${currentPct}%`,
              backgroundColor: 'var(--accent-emerald)',
              boxShadow: '0 0 8px rgba(16, 185, 129, 0.6)',
            }}
          />
        </div>

        {/* Playhead thumb */}
        <div
          style={{
            position: 'absolute',
            left: `${currentPct}%`,
            width: '12px',
            height: '12px',
            borderRadius: 'var(--radius-full)',
            backgroundColor: '#fff',
            boxShadow: '0 0 8px rgba(0, 0, 0, 0.5), 0 0 12px var(--accent-emerald)',
            transform: 'translateX(-50%)',
            transition: isDragging ? 'none' : 'transform 0.1s ease',
            pointerEvents: 'none',
          }}
        />
      </div>

      <span
        style={{
          fontFamily: 'var(--font-mono)',
          fontSize: '0.75rem',
          color: 'var(--text-muted)',
          minWidth: '36px',
        }}
      >
        {formatTime(duration)}
      </span>
    </div>
  );
};
