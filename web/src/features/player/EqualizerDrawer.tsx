import React from 'react';
import { Sliders, X, RotateCcw, Power } from 'lucide-react';
import {
  EQUALIZER_BANDS,
  EQUALIZER_PRESETS,
  type EqualizerPresetKey,
} from './useAudioEqualizer';

interface EqualizerDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  isEnabled: boolean;
  preset: EqualizerPresetKey;
  gains: number[];
  onBandGainChange: (index: number, gain: number) => void;
  onSelectPreset: (preset: Exclude<EqualizerPresetKey, 'custom'>) => void;
  onToggleBypass: () => void;
  onResetFlat: () => void;
}

const PRESET_LABELS: Record<Exclude<EqualizerPresetKey, 'custom'>, string> = {
  flat: 'Flat',
  bass_boost: 'Bass Boost',
  vocal_presence: 'Vocal',
  electronic_club: 'Club',
  acoustic_warm: 'Warm',
};

export const EqualizerDrawer: React.FC<EqualizerDrawerProps> = ({
  isOpen,
  onClose,
  isEnabled,
  preset,
  gains,
  onBandGainChange,
  onSelectPreset,
  onToggleBypass,
  onResetFlat,
}) => {
  if (!isOpen) return null;

  return (
    <div
      style={{
        position: 'fixed',
        bottom: '96px',
        right: '24px',
        width: '440px',
        maxWidth: 'calc(100vw - 48px)',
        backgroundColor: 'rgba(16, 19, 26, 0.96)',
        backdropFilter: 'blur(24px)',
        border: '1px solid var(--border-active)',
        borderRadius: 'var(--radius-lg)',
        boxShadow: '0 20px 50px rgba(0, 0, 0, 0.8)',
        display: 'flex',
        flexDirection: 'column',
        zIndex: 110,
        overflow: 'hidden',
      }}
    >
      {/* Header */}
      <div
        style={{
          padding: '16px 20px',
          borderBottom: '1px solid var(--border-glass)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          backgroundColor: 'rgba(11, 13, 16, 0.6)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <div
            style={{
              width: '32px',
              height: '32px',
              borderRadius: 'var(--radius-md)',
              backgroundColor: isEnabled
                ? 'rgba(16, 185, 129, 0.15)'
                : 'rgba(255, 255, 255, 0.05)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: isEnabled ? 'var(--accent-emerald)' : 'var(--text-muted)',
              transition: 'all 0.2s ease',
            }}
          >
            <Sliders size={16} />
          </div>
          <div>
            <h3
              style={{
                margin: 0,
                fontSize: '0.92rem',
                fontWeight: 600,
                color: 'var(--text-primary)',
              }}
            >
              5-Band Studio Equalizer
            </h3>
            <p
              style={{
                margin: 0,
                fontSize: '0.74rem',
                color: 'var(--text-muted)',
              }}
            >
              Hardware-accelerated parametric Biquad filters
            </p>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          {/* Bypass Toggle */}
          <button
            onClick={onToggleBypass}
            style={{
              background: isEnabled ? 'rgba(16, 185, 129, 0.15)' : 'none',
              border: isEnabled ? '1px solid var(--accent-emerald)' : '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: isEnabled ? 'var(--accent-emerald)' : 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px 10px',
              fontSize: '0.75rem',
              display: 'flex',
              alignItems: 'center',
              gap: '5px',
              transition: 'all 0.15s ease',
            }}
            title={isEnabled ? 'Bypass Equalizer' : 'Enable Equalizer'}
          >
            <Power size={13} />
            <span>{isEnabled ? 'Active' : 'Bypassed'}</span>
          </button>

          {/* Reset Flat */}
          <button
            onClick={onResetFlat}
            style={{
              background: 'none',
              border: '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              transition: 'all 0.15s ease',
            }}
            title="Reset to Flat (0 dB)"
          >
            <RotateCcw size={14} />
          </button>

          {/* Close */}
          <button
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              borderRadius: 'var(--radius-sm)',
              transition: 'color 0.15s ease',
            }}
            title="Close Equalizer"
          >
            <X size={16} />
          </button>
        </div>
      </div>

      {/* Preset Selector Badges */}
      <div
        style={{
          display: 'flex',
          gap: '8px',
          padding: '12px 20px',
          backgroundColor: 'rgba(11, 13, 16, 0.3)',
          borderBottom: '1px solid var(--border-glass)',
          overflowX: 'auto',
        }}
      >
        {(Object.keys(EQUALIZER_PRESETS) as Array<Exclude<EqualizerPresetKey, 'custom'>>).map((key) => {
          const isSelected = preset === key;
          return (
            <button
              key={key}
              onClick={() => onSelectPreset(key)}
              style={{
                background: isSelected ? 'rgba(16, 185, 129, 0.15)' : 'rgba(255, 255, 255, 0.03)',
                border: isSelected ? '1px solid var(--accent-emerald)' : '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-full)',
                color: isSelected ? 'var(--accent-emerald)' : 'var(--text-muted)',
                padding: '4px 12px',
                fontSize: '0.74rem',
                fontWeight: isSelected ? 600 : 400,
                cursor: 'pointer',
                whiteSpace: 'nowrap',
                transition: 'all 0.15s ease',
              }}
            >
              {PRESET_LABELS[key]}
            </button>
          );
        })}
        {preset === 'custom' && (
          <span
            style={{
              backgroundColor: 'rgba(56, 189, 248, 0.12)',
              border: '1px solid rgba(56, 189, 248, 0.4)',
              borderRadius: 'var(--radius-full)',
              color: '#38bdf8',
              padding: '4px 12px',
              fontSize: '0.74rem',
              fontWeight: 600,
              whiteSpace: 'nowrap',
            }}
          >
            Custom
          </span>
        )}
      </div>

      {/* 5-Band Slider Rack */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '30px 24px',
          gap: '16px',
        }}
      >
        {EQUALIZER_BANDS.map((band, idx) => {
          const gain = gains[idx] ?? 0;
          return (
            <div
              key={band.frequency}
              style={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                gap: '12px',
                flex: 1,
              }}
            >
              {/* dB Readout */}
              <span
                style={{
                  fontSize: '0.74rem',
                  fontFamily: 'monospace',
                  color: !isEnabled
                    ? 'var(--text-muted)'
                    : gain > 0
                    ? 'var(--accent-emerald)'
                    : gain < 0
                    ? '#38bdf8'
                    : 'var(--text-muted)',
                  fontWeight: 600,
                  minWidth: '40px',
                  textAlign: 'center',
                }}
              >
                {gain > 0 ? `+${gain.toFixed(1)}` : gain.toFixed(1)} dB
              </span>

              {/* Vertical Slider Track Container */}
              <div
                style={{
                  height: '160px',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  position: 'relative',
                  width: '32px',
                }}
              >
                {/* Center 0 dB guide line */}
                <div
                  style={{
                    position: 'absolute',
                    top: '50%',
                    left: '4px',
                    right: '4px',
                    height: '1px',
                    backgroundColor: 'rgba(255, 255, 255, 0.1)',
                    pointerEvents: 'none',
                  }}
                />

                <input
                  type="range"
                  min="-12"
                  max="12"
                  step="0.5"
                  value={gain}
                  disabled={!isEnabled}
                  onChange={(e) => onBandGainChange(idx, parseFloat(e.target.value))}
                  style={{
                    transform: 'rotate(-90deg)',
                    width: '150px',
                    cursor: isEnabled ? 'pointer' : 'not-allowed',
                    accentColor: 'var(--accent-emerald)',
                    opacity: isEnabled ? 1 : 0.4,
                  }}
                />
              </div>

              {/* Frequency Label */}
              <span
                style={{
                  fontSize: '0.78rem',
                  fontWeight: 500,
                  color: isEnabled ? 'var(--text-primary)' : 'var(--text-muted)',
                }}
              >
                {band.label}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
};
