import React, { useState, useEffect } from 'react';
import { HardDrive, Server, ShieldCheck, CheckCircle2, AlertCircle, Loader2 } from 'lucide-react';
import { Modal } from '../../shared/components/Modal';
import { Button } from '../../shared/components/Button';
import { apiClient } from '../../shared/api/client';
import type { StorageUsage, NodeSettings } from '../../types/node';

interface SettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSettingsUpdated?: () => void;
}

export const SettingsModal: React.FC<SettingsModalProps> = ({
  isOpen,
  onClose,
  onSettingsUpdated,
}) => {
  const [storageUsage, setStorageUsage] = useState<StorageUsage | null>(null);
  const [settings, setSettings] = useState<NodeSettings | null>(null);
  const [keeperEnabled, setKeeperEnabled] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedSuccess, setSavedSuccess] = useState(false);

  const formatBytes = (bytes: number) => {
    if (bytes < 1024 * 1024) {
      return `${(bytes / 1024).toFixed(1)} KiB`;
    }
    if (bytes < 1024 * 1024 * 1024) {
      return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
    }
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
  };

  useEffect(() => {
    if (isOpen) {
      setLoading(true);
      setError(null);
      setSavedSuccess(false);

      Promise.all([apiClient.getStorage(), apiClient.getSettings()])
        .then(([storageData, settingsData]) => {
          setStorageUsage(storageData);
          setSettings(settingsData);
          setKeeperEnabled(settingsData.keeper_enabled);
        })
        .catch((err: unknown) => {
          const msg = err instanceof Error ? err.message : 'Failed to load node metrics';
          setError(msg);
        })
        .finally(() => {
          setLoading(false);
        });
    }
  }, [isOpen]);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setError(null);
    setSavedSuccess(false);

    try {
      const updated = await apiClient.updateSettings({
        keeper_enabled: keeperEnabled,
      });
      setSettings(updated);
      setSavedSuccess(true);
      if (onSettingsUpdated) {
        onSettingsUpdated();
      }
      setTimeout(() => {
        setSavedSuccess(false);
      }, 2000);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to update settings';
      setError(msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Node Storage & Swarm Settings">
      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: '36px 0' }}>
          <Loader2 size={24} className="spin" style={{ color: 'var(--accent-emerald)' }} />
        </div>
      ) : (
        <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: '22px' }}>
          {error && (
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '10px',
                padding: '12px 16px',
                backgroundColor: 'rgba(244, 63, 94, 0.1)',
                border: '1px solid rgba(244, 63, 94, 0.3)',
                borderRadius: 'var(--radius-md)',
                color: 'var(--accent-rose)',
                fontSize: '0.85rem',
              }}
            >
              <AlertCircle size={18} style={{ flexShrink: 0 }} />
              <span>{error}</span>
            </div>
          )}

          {savedSuccess && (
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '10px',
                padding: '12px 16px',
                backgroundColor: 'rgba(16, 185, 129, 0.1)',
                border: '1px solid rgba(16, 185, 129, 0.3)',
                borderRadius: 'var(--radius-md)',
                color: 'var(--accent-emerald)',
                fontSize: '0.85rem',
              }}
            >
              <CheckCircle2 size={18} style={{ flexShrink: 0 }} />
              <span>Settings updated successfully!</span>
            </div>
          )}

          {/* Storage Quota Breakdown */}
          {storageUsage && (
            <div
              style={{
                display: 'flex',
                flexDirection: 'column',
                gap: '12px',
                padding: '16px',
                backgroundColor: 'var(--bg-card)',
                borderRadius: 'var(--radius-md)',
                border: '1px solid var(--border-glass)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--text-primary)', fontWeight: 600 }}>
                  <HardDrive size={16} style={{ color: 'var(--accent-emerald)' }} />
                  <span>Storage Quota Utilization</span>
                </div>
                <span style={{ fontSize: '0.8rem', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                  {formatBytes(storageUsage.stored_bytes)} / {formatBytes(storageUsage.quota_bytes)}
                </span>
              </div>

              {/* Progress gauge */}
              <div
                style={{
                  width: '100%',
                  height: '8px',
                  borderRadius: 'var(--radius-full)',
                  backgroundColor: 'rgba(255, 255, 255, 0.08)',
                  overflow: 'hidden',
                  display: 'flex',
                }}
              >
                <div
                  style={{
                    height: '100%',
                    width: `${Math.min(100, (storageUsage.pinned_bytes / storageUsage.quota_bytes) * 100)}%`,
                    backgroundColor: 'var(--accent-emerald)',
                  }}
                  title={`Pinned: ${formatBytes(storageUsage.pinned_bytes)}`}
                />
                <div
                  style={{
                    height: '100%',
                    width: `${Math.min(100, (storageUsage.unpinned_bytes / storageUsage.quota_bytes) * 100)}%`,
                    backgroundColor: 'var(--accent-sky)',
                  }}
                  title={`Cached (LRU Evictable): ${formatBytes(storageUsage.unpinned_bytes)}`}
                />
              </div>

              {/* Metrics legend */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(3, 1fr)',
                  gap: '8px',
                  fontSize: '0.75rem',
                  color: 'var(--text-secondary)',
                  marginTop: '4px',
                }}
              >
                <div>
                  <span style={{ color: 'var(--accent-emerald)', fontWeight: 600 }}>● Pinned: </span>
                  {formatBytes(storageUsage.pinned_bytes)}
                </div>
                <div>
                  <span style={{ color: 'var(--accent-sky)', fontWeight: 600 }}>● Cached: </span>
                  {formatBytes(storageUsage.unpinned_bytes)}
                </div>
                <div>
                  <span style={{ color: 'var(--text-muted)', fontWeight: 600 }}>● Free: </span>
                  {formatBytes(storageUsage.available_bytes)}
                </div>
              </div>
            </div>
          )}

          {/* Voluntary Keeper Swarm Replication */}
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: '10px',
              padding: '16px',
              backgroundColor: 'var(--bg-card)',
              borderRadius: 'var(--radius-md)',
              border: '1px solid var(--border-glass)',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--text-primary)', fontWeight: 600 }}>
                <Server size={16} style={{ color: 'var(--accent-sky)' }} />
                <span>Voluntary Keeper Swarm Replication</span>
              </div>
              <label style={{ display: 'flex', alignItems: 'center', cursor: 'pointer', gap: '6px' }}>
                <input
                  type="checkbox"
                  checked={keeperEnabled}
                  onChange={(e) => setKeeperEnabled(e.target.checked)}
                  style={{ cursor: 'pointer', width: '16px', height: '16px', accentColor: 'var(--accent-emerald)' }}
                />
              </label>
            </div>
            <p style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', lineHeight: 1.45 }}>
              Automatically replicates rare audio files (&lt; 2 observed copies in the swarm) in the background.
              Adheres to strict thesis invariants: yields instantly when audio playback is active and pauses when quota is constrained.
            </p>
          </div>

          {/* Node networking summary */}
          {settings && (
            <div style={{ fontSize: '0.78rem', color: 'var(--text-muted)', display: 'flex', flexDirection: 'column', gap: '4px' }}>
              <div><ShieldCheck size={13} style={{ verticalAlign: 'middle', marginRight: '6px', color: 'var(--accent-emerald)' }} /> Local HTTP API: <code style={{ color: 'var(--text-primary)' }}>{settings.http_addr}</code></div>
              <div><ShieldCheck size={13} style={{ verticalAlign: 'middle', marginRight: '6px', color: 'var(--accent-emerald)' }} /> Swarm P2P Listen Port: <code style={{ color: 'var(--text-primary)' }}>{settings.p2p_port}</code></div>
            </div>
          )}

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '10px' }}>
            <Button variant="secondary" size="md" type="button" onClick={onClose} disabled={saving}>
              Close
            </Button>
            <Button
              variant="primary"
              size="md"
              type="submit"
              disabled={saving}
              icon={saving ? <Loader2 size={16} className="spin" /> : undefined}
            >
              {saving ? 'Saving...' : 'Save Settings'}
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
};
