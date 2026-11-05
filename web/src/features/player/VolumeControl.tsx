import React from 'react';
import { Volume2, VolumeX, Volume1 } from 'lucide-react';

interface VolumeControlProps {
  volume: number;
  isMuted: boolean;
  onVolumeChange: (val: number) => void;
  onToggleMute: () => void;
}

export const VolumeControl: React.FC<VolumeControlProps> = ({
  volume,
  isMuted,
  onVolumeChange,
  onToggleMute,
}) => {
  const effectiveVolume = isMuted ? 0 : volume;

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
      <button
        onClick={onToggleMute}
        style={{
          background: 'none',
          border: 'none',
          color: isMuted ? 'var(--accent-rose)' : 'var(--text-secondary)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: '4px',
          cursor: 'pointer',
        }}
        title={isMuted ? 'Unmute' : 'Mute'}
      >
        {isMuted || effectiveVolume === 0 ? (
          <VolumeX size={17} />
        ) : effectiveVolume < 0.5 ? (
          <Volume1 size={17} />
        ) : (
          <Volume2 size={17} />
        )}
      </button>

      <input
        type="range"
        min={0}
        max={1}
        step={0.01}
        value={effectiveVolume}
        onChange={(e) => {
          onVolumeChange(parseFloat(e.target.value));
          if (isMuted) onToggleMute();
        }}
        className="volume-slider"
        style={{
          width: '74px',
          height: '4px',
          cursor: 'pointer',
        }}
      />
    </div>
  );
};
