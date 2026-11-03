import React from 'react';
import { X, Sparkles, Download, Check, AlertTriangle, RefreshCw } from 'lucide-react';
import { useAutoUpdater } from '../../shared/hooks/useAutoUpdater';
import { Badge } from '../../shared/components/Badge';

interface UpdaterModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const UpdaterModal: React.FC<UpdaterModalProps> = ({ isOpen, onClose }) => {
  const {
    state,
    loading,
    actionMessage,
    checkForUpdates,
    startDownload,
    applyUpdateAndRestart,
  } = useAutoUpdater();

  if (!isOpen) return null;

  const formatBytes = (bytes: number) => {
    if (!bytes) return '0 B';
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
  };

  const rel = state.available_release;

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.75)',
        backdropFilter: 'blur(8px)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
        padding: '20px',
      }}
      onClick={onClose}
    >
      <div
        style={{
          width: '100%',
          maxWidth: '520px',
          backgroundColor: '#14171d',
          border: '1px solid var(--border-active)',
          borderRadius: 'var(--radius-lg)',
          boxShadow: '0 20px 48px rgba(0, 0, 0, 0.8)',
          overflow: 'hidden',
          display: 'flex',
          flexDirection: 'column',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '16px 20px',
            borderBottom: '1px solid var(--border-glass)',
            backgroundColor: 'rgba(255, 255, 255, 0.02)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <Sparkles size={18} style={{ color: 'var(--accent-emerald)' }} />
            <h2 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
              Autonomous P2P Client Auto-Updater
            </h2>
          </div>
          <button
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              padding: '4px',
              borderRadius: 'var(--radius-sm)',
            }}
          >
            <X size={18} />
          </button>
        </div>

        {/* Content */}
        <div style={{ padding: '20px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Current Version Bar */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '12px 16px',
              backgroundColor: 'rgba(255, 255, 255, 0.03)',
              borderRadius: 'var(--radius-md)',
              border: '1px solid var(--border-glass)',
            }}
          >
            <div>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>CURRENT VERSION</div>
              <div style={{ fontSize: '0.95rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '2px' }}>
                v{state.current_version}
              </div>
            </div>
            <button
              onClick={checkForUpdates}
              disabled={loading || state.status === 'downloading'}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                padding: '8px 12px',
                borderRadius: 'var(--radius-md)',
                backgroundColor: 'rgba(255, 255, 255, 0.06)',
                border: '1px solid var(--border-glass)',
                color: 'var(--text-primary)',
                cursor: loading ? 'not-allowed' : 'pointer',
                fontSize: '0.8rem',
                fontWeight: 500,
                transition: 'all 0.15s ease',
              }}
            >
              <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
              Check Swarm
            </button>
          </div>

          {actionMessage && (
            <div
              style={{
                padding: '10px 14px',
                backgroundColor: 'rgba(16, 185, 129, 0.1)',
                border: '1px solid rgba(16, 185, 129, 0.25)',
                borderRadius: 'var(--radius-md)',
                fontSize: '0.8rem',
                color: 'var(--accent-emerald)',
              }}
            >
              {actionMessage}
            </div>
          )}

          {/* Release States */}
          {state.status === 'idle' && (
            <div
              style={{
                padding: '24px 16px',
                textAlign: 'center',
                color: 'var(--text-secondary)',
                fontSize: '0.85rem',
              }}
            >
              No updates currently discovered in the local swarm. When an authentic release is gossiped by trusted authority, it will appear here.
            </div>
          )}

          {state.status === 'available' && rel && (
            <div
              style={{
                padding: '16px',
                backgroundColor: 'rgba(56, 189, 248, 0.05)',
                border: '1px solid rgba(56, 189, 248, 0.2)',
                borderRadius: 'var(--radius-md)',
                display: 'flex',
                flexDirection: 'column',
                gap: '12px',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--accent-cyan)' }}>
                  New Release v{rel.version}
                </span>
                <Badge variant="cyan">{rel.target_os}/{rel.target_arch}</Badge>
              </div>

              <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
                Payload Size: {formatBytes(rel.total_size_bytes)} ({state.total_blocks} chunks of 256 KiB)
              </div>

              {rel.release_notes && (
                <div
                  style={{
                    padding: '10px 12px',
                    backgroundColor: 'rgba(0, 0, 0, 0.3)',
                    borderRadius: 'var(--radius-sm)',
                    fontSize: '0.8rem',
                    color: 'var(--text-muted)',
                    fontFamily: 'var(--font-mono)',
                    whiteSpace: 'pre-wrap',
                  }}
                >
                  {rel.release_notes}
                </div>
              )}

              <button
                onClick={startDownload}
                disabled={loading}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '8px',
                  padding: '10px 16px',
                  borderRadius: 'var(--radius-md)',
                  backgroundColor: 'var(--accent-cyan)',
                  border: 'none',
                  color: '#0b0d10',
                  fontWeight: 600,
                  fontSize: '0.85rem',
                  cursor: 'pointer',
                  marginTop: '4px',
                }}
              >
                <Download size={16} />
                Download via P2P Swarm
              </button>
            </div>
          )}

          {state.status === 'downloading' && (
            <div
              style={{
                padding: '16px',
                backgroundColor: 'rgba(56, 189, 248, 0.05)',
                border: '1px solid rgba(56, 189, 248, 0.2)',
                borderRadius: 'var(--radius-md)',
                display: 'flex',
                flexDirection: 'column',
                gap: '12px',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '0.85rem', color: 'var(--text-primary)', fontWeight: 500 }}>
                  Retrieving Release Chunks
                </span>
                <span style={{ fontSize: '0.85rem', fontFamily: 'var(--font-mono)', color: 'var(--accent-cyan)' }}>
                  {state.progress_pct.toFixed(0)}%
                </span>
              </div>

              {/* Progress bar */}
              <div
                style={{
                  height: '6px',
                  backgroundColor: 'rgba(255, 255, 255, 0.1)',
                  borderRadius: 'var(--radius-full)',
                  overflow: 'hidden',
                }}
              >
                <div
                  style={{
                    height: '100%',
                    width: `${state.progress_pct}%`,
                    backgroundColor: 'var(--accent-cyan)',
                    transition: 'width 0.3s ease',
                  }}
                />
              </div>

              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                {state.downloaded_blocks} of {state.total_blocks} chunks verified
              </div>
            </div>
          )}

          {state.status === 'ready_to_restart' && (
            <div
              style={{
                padding: '16px',
                backgroundColor: 'rgba(16, 185, 129, 0.08)',
                border: '1px solid rgba(16, 185, 129, 0.3)',
                borderRadius: 'var(--radius-md)',
                display: 'flex',
                flexDirection: 'column',
                gap: '12px',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <Check size={18} style={{ color: 'var(--accent-emerald)' }} />
                <span style={{ fontSize: '0.95rem', fontWeight: 600, color: 'var(--accent-emerald)' }}>
                  Update Ready to Apply (v{rel?.version})
                </span>
              </div>

              <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
                All constituent blocks have been retrieved and cryptographically verified against the release authority signature.
              </div>

              <button
                onClick={applyUpdateAndRestart}
                disabled={loading}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '8px',
                  padding: '12px 16px',
                  borderRadius: 'var(--radius-md)',
                  backgroundColor: 'var(--accent-emerald)',
                  border: 'none',
                  color: '#0b0d10',
                  fontWeight: 600,
                  fontSize: '0.9rem',
                  cursor: 'pointer',
                  boxShadow: '0 0 16px rgba(16, 185, 129, 0.3)',
                }}
              >
                <Sparkles size={16} />
                Apply Atomic Update
              </button>
            </div>
          )}

          {state.status === 'error' && (
            <div
              style={{
                padding: '14px',
                backgroundColor: 'rgba(244, 63, 94, 0.1)',
                border: '1px solid rgba(244, 63, 94, 0.3)',
                borderRadius: 'var(--radius-md)',
                display: 'flex',
                alignItems: 'flex-start',
                gap: '10px',
              }}
            >
              <AlertTriangle size={18} style={{ color: 'var(--accent-rose)', flexShrink: 0, marginTop: '2px' }} />
              <div>
                <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'var(--accent-rose)' }}>
                  Update Error
                </div>
                <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginTop: '2px' }}>
                  {state.error_message || 'An error occurred during update processing.'}
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
