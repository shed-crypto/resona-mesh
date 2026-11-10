import React, { useState, useEffect } from 'react';
import {
  HardDrive,
  Server,
  ShieldCheck,
  CheckCircle2,
  AlertCircle,
  Loader2,
  Globe,
  Key,
  Copy,
  Check,
  Zap,
} from 'lucide-react';
import { Modal } from '../../shared/components/Modal';
import { Button } from '../../shared/components/Button';
import { apiClient } from '../../shared/api/client';
import type { StorageUsage, NodeSettings } from '../../types/node';

interface SettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSettingsUpdated?: () => void;
}

const QUOTA_PRESETS = [
  { label: '0 MB (Pure Streamer)', valueMB: 0 },
  { label: '512 MB', valueMB: 512 },
  { label: '1 GB', valueMB: 1024 },
  { label: '2 GB', valueMB: 2048 },
  { label: '5 GB', valueMB: 5120 },
  { label: '10 GB', valueMB: 10240 },
];

export const SettingsModal: React.FC<SettingsModalProps> = ({
  isOpen,
  onClose,
  onSettingsUpdated,
}) => {
  const [storageUsage, setStorageUsage] = useState<StorageUsage | null>(null);
  const [settings, setSettings] = useState<NodeSettings | null>(null);
  const [quotaMB, setQuotaMB] = useState<number>(2048);
  const [keeperEnabled, setKeeperEnabled] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedSuccess, setSavedSuccess] = useState(false);

  const [copiedKey, setCopiedKey] = useState(false);
  const [copiedWAN, setCopiedWAN] = useState(false);

  const formatBytes = (bytes: number) => {
    if (bytes <= 0) return '0.0 KiB';
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
          setQuotaMB(Math.round(settingsData.storage_quota_bytes / (1024 * 1024)));
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

  const handleCopyKey = () => {
    if (!settings?.publisher_key) return;
    navigator.clipboard.writeText(settings.publisher_key);
    setCopiedKey(true);
    setTimeout(() => setCopiedKey(false), 2000);
  };

  const handleCopyWAN = () => {
    if (!settings?.wan_address) return;
    navigator.clipboard.writeText(settings.wan_address);
    setCopiedWAN(true);
    setTimeout(() => setCopiedWAN(false), 2000);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setError(null);
    setSavedSuccess(false);

    try {
      const updated = await apiClient.updateSettings({
        keeper_enabled: keeperEnabled,
        storage_quota_bytes: quotaMB * 1024 * 1024,
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
        <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
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

          {/* Storage Quota Allocation & Adjustment */}
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: '14px',
              padding: '16px',
              backgroundColor: 'var(--bg-card)',
              borderRadius: 'var(--radius-md)',
              border: '1px solid var(--border-glass)',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--text-primary)', fontWeight: 600 }}>
                <HardDrive size={16} style={{ color: 'var(--accent-emerald)' }} />
                <span>Storage Cache Limit</span>
              </div>
              <span style={{ fontSize: '0.82rem', fontFamily: 'var(--font-mono)', color: quotaMB === 0 ? 'var(--accent-amber)' : 'var(--accent-emerald)', fontWeight: 600 }}>
                {quotaMB === 0 ? '0 MB (Pure Streamer)' : quotaMB >= 1024 ? `${(quotaMB / 1024).toFixed(1)} GiB` : `${quotaMB} MiB`}
              </span>
            </div>

            {/* Quick preset selector */}
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
              {QUOTA_PRESETS.map((preset) => (
                <button
                  key={preset.valueMB}
                  type="button"
                  onClick={() => setQuotaMB(preset.valueMB)}
                  style={{
                    padding: '5px 10px',
                    fontSize: '0.74rem',
                    borderRadius: 'var(--radius-sm)',
                    border: quotaMB === preset.valueMB ? '1px solid var(--accent-emerald)' : '1px solid var(--border-glass)',
                    backgroundColor: quotaMB === preset.valueMB ? 'rgba(16, 185, 129, 0.15)' : 'rgba(255, 255, 255, 0.03)',
                    color: quotaMB === preset.valueMB ? 'var(--accent-emerald)' : 'var(--text-secondary)',
                    cursor: 'pointer',
                    transition: 'all 0.15s ease',
                  }}
                >
                  {preset.label}
                </button>
              ))}
            </div>

            {/* Range slider for smooth quota adjustment */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <input
                type="range"
                min={0}
                max={10240}
                step={256}
                value={quotaMB}
                onChange={(e) => setQuotaMB(Number(e.target.value))}
                style={{
                  flex: 1,
                  accentColor: 'var(--accent-emerald)',
                  cursor: 'pointer',
                }}
              />
              <span style={{ fontSize: '0.75rem', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', minWidth: '60px', textAlign: 'right' }}>
                {quotaMB} MB
              </span>
            </div>

            {/* Zero Cache / Pure Streaming explanation callout */}
            {quotaMB === 0 && (
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  padding: '8px 12px',
                  borderRadius: 'var(--radius-sm)',
                  backgroundColor: 'rgba(255, 170, 0, 0.08)',
                  border: '1px solid rgba(255, 170, 0, 0.25)',
                  color: 'var(--accent-amber)',
                  fontSize: '0.74rem',
                  lineHeight: 1.4,
                }}
              >
                <Zap size={14} style={{ flexShrink: 0 }} />
                <span>
                  <strong>Pure Online Streaming Mode:</strong> Audio chunks are retained in RAM volatile buffer during playback and are not cached permanently to disk.
                </span>
              </div>
            )}

            {/* Current Utilization Progress bar */}
            {storageUsage && quotaMB > 0 && (
              <>
                <div
                  style={{
                    width: '100%',
                    height: '6px',
                    borderRadius: 'var(--radius-full)',
                    backgroundColor: 'rgba(255, 255, 255, 0.08)',
                    overflow: 'hidden',
                    display: 'flex',
                    marginTop: '4px',
                  }}
                >
                  <div
                    style={{
                      height: '100%',
                      width: `${Math.min(100, (storageUsage.pinned_bytes / (quotaMB * 1024 * 1024)) * 100)}%`,
                      backgroundColor: 'var(--accent-emerald)',
                    }}
                    title={`Pinned: ${formatBytes(storageUsage.pinned_bytes)}`}
                  />
                  <div
                    style={{
                      height: '100%',
                      width: `${Math.min(100, (storageUsage.unpinned_bytes / (quotaMB * 1024 * 1024)) * 100)}%`,
                      backgroundColor: 'var(--accent-sky)',
                    }}
                    title={`Cached: ${formatBytes(storageUsage.unpinned_bytes)}`}
                  />
                </div>

                <div
                  style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(3, 1fr)',
                    gap: '8px',
                    fontSize: '0.72rem',
                    color: 'var(--text-secondary)',
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
                    <span style={{ color: 'var(--text-muted)', fontWeight: 600 }}>● Used: </span>
                    {formatBytes(storageUsage.stored_bytes)}
                  </div>
                </div>
              </>
            )}
          </div>

          {/* User Profile & Device Identity Transfer */}
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
                <Key size={16} style={{ color: 'var(--accent-amber)' }} />
                <span>Node Identity (User Publisher Key)</span>
              </div>
              {settings?.publisher_key && (
                <button
                  type="button"
                  onClick={handleCopyKey}
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '4px',
                    padding: '3px 8px',
                    fontSize: '0.72rem',
                    borderRadius: 'var(--radius-sm)',
                    border: copiedKey ? '1px solid rgba(16, 185, 129, 0.4)' : '1px solid var(--border-glass)',
                    backgroundColor: copiedKey ? 'rgba(16, 185, 129, 0.15)' : 'rgba(255, 255, 255, 0.05)',
                    color: copiedKey ? 'var(--accent-emerald)' : 'var(--text-secondary)',
                    cursor: 'pointer',
                  }}
                >
                  {copiedKey ? <Check size={12} /> : <Copy size={12} />}
                  <span>{copiedKey ? 'Copied Key!' : 'Copy Key'}</span>
                </button>
              )}
            </div>
            <p style={{ fontSize: '0.74rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
              Цей криптографічний ключ Ed25519 закріплює за вами авторство треків і плейлистів у децентралізованому рої.
            </p>
            <div
              style={{
                fontFamily: 'var(--font-mono)',
                fontSize: '0.72rem',
                padding: '8px 10px',
                borderRadius: 'var(--radius-sm)',
                backgroundColor: 'rgba(0, 0, 0, 0.35)',
                border: '1px solid var(--border-glass)',
                color: 'var(--accent-primary)',
                wordBreak: 'break-all',
              }}
            >
              {settings?.publisher_key || 'Loading identity key...'}
            </div>
          </div>

          {/* Global WAN Multiaddress */}
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
                <Globe size={16} style={{ color: 'var(--accent-sky)' }} />
                <span>Global WAN P2P Multiaddress</span>
              </div>
              {settings?.wan_address && (
                <button
                  type="button"
                  onClick={handleCopyWAN}
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: '4px',
                    padding: '3px 8px',
                    fontSize: '0.72rem',
                    borderRadius: 'var(--radius-sm)',
                    border: copiedWAN ? '1px solid rgba(16, 185, 129, 0.4)' : '1px solid var(--border-glass)',
                    backgroundColor: copiedWAN ? 'rgba(16, 185, 129, 0.15)' : 'rgba(255, 255, 255, 0.05)',
                    color: copiedWAN ? 'var(--accent-emerald)' : 'var(--text-secondary)',
                    cursor: 'pointer',
                  }}
                >
                  {copiedWAN ? <Check size={12} /> : <Copy size={12} />}
                  <span>{copiedWAN ? 'Copied Multiaddr!' : 'Copy Multiaddr'}</span>
                </button>
              )}
            </div>
            <p style={{ fontSize: '0.74rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
              Публічна глобальна адреса вашого вузла для прямого з'єднання через інтернет (STUN/UPnP).
            </p>
            <div
              style={{
                fontFamily: 'var(--font-mono)',
                fontSize: '0.72rem',
                padding: '8px 10px',
                borderRadius: 'var(--radius-sm)',
                backgroundColor: 'rgba(0, 0, 0, 0.35)',
                border: '1px solid var(--border-glass)',
                color: settings?.wan_address ? 'var(--accent-sky)' : 'var(--text-muted)',
                wordBreak: 'break-all',
              }}
            >
              {settings?.wan_address || 'Awaiting WAN detection (UPnP / STUN active)...'}
            </div>
          </div>

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
            <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.45 }}>
              Автоматично реплікує рідкісні треки (&lt; 2 копій у рої) у фоновому режимі. Миттєво призупиняється при відтворенні музики.
            </p>
          </div>

          {/* Node networking summary */}
          {settings && (
            <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', display: 'flex', flexDirection: 'column', gap: '4px' }}>
              <div><ShieldCheck size={13} style={{ verticalAlign: 'middle', marginRight: '6px', color: 'var(--accent-emerald)' }} /> Local HTTP API: <code style={{ color: 'var(--text-primary)' }}>{settings.http_addr}</code></div>
              <div><ShieldCheck size={13} style={{ verticalAlign: 'middle', marginRight: '6px', color: 'var(--accent-emerald)' }} /> Swarm P2P Listen Port: <code style={{ color: 'var(--text-primary)' }}>{settings.p2p_port}</code></div>
            </div>
          )}

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '6px' }}>
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
