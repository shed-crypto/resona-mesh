import React, { useState, useEffect } from 'react';
import {
  HardDrive,
  Sparkles,
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
  Download,
  KeyRound,
  FileText,
} from 'lucide-react';
import { Modal } from '../../shared/components/Modal';
import { Button } from '../../shared/components/Button';
import { apiClient, type EncryptedKeyBundle } from '../../shared/api/client';
import type { StorageUsage, NodeSettings } from '../../types/node';

interface SettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSettingsUpdated?: () => void;
  onOpenUpdater?: () => void;
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
  onOpenUpdater,
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

  // Identity Transfer State
  const [transferMode, setTransferMode] = useState<'view' | 'export' | 'import'>('view');
  const [exportPassphrase, setExportPassphrase] = useState('');
  const [exportedBundle, setExportedBundle] = useState<EncryptedKeyBundle | null>(null);
  const [copiedExport, setCopiedExport] = useState(false);
  const [exportLoading, setExportLoading] = useState(false);
  const [exportError, setExportError] = useState<string | null>(null);

  const [importPassphrase, setImportPassphrase] = useState('');
  const [importBundleText, setImportBundleText] = useState('');
  const [importLoading, setImportLoading] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [importSuccess, setImportSuccess] = useState<string | null>(null);

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
      setTransferMode('view');
      setExportedBundle(null);
      setExportError(null);
      setImportError(null);
      setImportSuccess(null);

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

  const handleExportIdentity = async (e: React.FormEvent) => {
    e.preventDefault();
    if (exportPassphrase.length < 8) {
      setExportError('Passphrase must be at least 8 characters');
      return;
    }
    setExportLoading(true);
    setExportError(null);
    try {
      const bundle = await apiClient.exportIdentity(exportPassphrase);
      setExportedBundle(bundle);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to export identity';
      setExportError(msg);
    } finally {
      setExportLoading(false);
    }
  };

  const handleImportIdentity = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!importBundleText.trim()) {
      setImportError('Please paste the encrypted JSON key bundle');
      return;
    }
    if (!importPassphrase) {
      setImportError('Please enter the decryption passphrase');
      return;
    }

    setImportLoading(true);
    setImportError(null);
    setImportSuccess(null);

    try {
      const bundleObj = JSON.parse(importBundleText.trim()) as EncryptedKeyBundle;
      const res = await apiClient.importIdentity(bundleObj, importPassphrase);
      setImportSuccess(`Identity imported successfully! Key: ${res.public_key.slice(0, 12)}... Please restart ResonaMesh to activate.`);
      setImportBundleText('');
      setImportPassphrase('');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to import identity bundle';
      setImportError(msg);
    } finally {
      setImportLoading(false);
    }
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
              <span style={{ fontSize: '0.85rem', fontFamily: 'var(--font-mono)', color: quotaMB === 0 ? 'var(--accent-amber)' : 'var(--accent-emerald)', fontWeight: 600 }}>
                {quotaMB === 0 ? '0 MB (Pure Streamer)' : quotaMB >= 1024 ? `${(quotaMB / 1024).toFixed(2)} GiB` : `${quotaMB} MiB`}
              </span>
            </div>

            {/* Quick preset buttons */}
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

            {/* Direct manual numeric input and range slider */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <input
                  type="range"
                  min={0}
                  max={10240}
                  step={128}
                  value={quotaMB}
                  onChange={(e) => setQuotaMB(Number(e.target.value))}
                  style={{
                    flex: 1,
                    accentColor: 'var(--accent-emerald)',
                    cursor: 'pointer',
                  }}
                />
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  <input
                    type="number"
                    min={0}
                    max={102400}
                    value={quotaMB}
                    onChange={(e) => {
                      const val = parseInt(e.target.value, 10);
                      setQuotaMB(isNaN(val) ? 0 : Math.max(0, val));
                    }}
                    style={{
                      width: '84px',
                      padding: '5px 8px',
                      backgroundColor: 'rgba(0, 0, 0, 0.35)',
                      border: '1px solid var(--border-glass)',
                      borderRadius: 'var(--radius-sm)',
                      color: 'var(--text-primary)',
                      fontFamily: 'var(--font-mono)',
                      fontSize: '0.8rem',
                      textAlign: 'right',
                    }}
                  />
                  <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>MB</span>
                </div>
              </div>
            </div>

            {/* Pure Streamer Mode Alert */}
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
                  <strong>Pure Online Streaming:</strong> 0 MB disk cache. Audio is streamed on-demand through volatile RAM without saving permanent chunk files to disk.
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
                    marginTop: '2px',
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

          {/* User Profile & Multi-Device Identity Transfer */}
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
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '8px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--text-primary)', fontWeight: 600 }}>
                <Key size={16} style={{ color: 'var(--accent-amber)' }} />
                <span>Account Identity & Multi-Device Transfer</span>
              </div>

              {/* Mode switcher tabs */}
              <div style={{ display: 'flex', gap: '4px' }}>
                <button
                  type="button"
                  onClick={() => setTransferMode('view')}
                  style={{
                    padding: '3px 8px',
                    fontSize: '0.72rem',
                    borderRadius: 'var(--radius-sm)',
                    border: transferMode === 'view' ? '1px solid var(--accent-primary)' : '1px solid var(--border-glass)',
                    backgroundColor: transferMode === 'view' ? 'rgba(249, 115, 22, 0.15)' : 'transparent',
                    color: transferMode === 'view' ? 'var(--accent-primary)' : 'var(--text-muted)',
                    cursor: 'pointer',
                  }}
                >
                  Public Key
                </button>
                <button
                  type="button"
                  onClick={() => setTransferMode('export')}
                  style={{
                    padding: '3px 8px',
                    fontSize: '0.72rem',
                    borderRadius: 'var(--radius-sm)',
                    border: transferMode === 'export' ? '1px solid var(--accent-emerald)' : '1px solid var(--border-glass)',
                    backgroundColor: transferMode === 'export' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
                    color: transferMode === 'export' ? 'var(--accent-emerald)' : 'var(--text-muted)',
                    cursor: 'pointer',
                  }}
                >
                  Export to Other PC
                </button>
                <button
                  type="button"
                  onClick={() => setTransferMode('import')}
                  style={{
                    padding: '3px 8px',
                    fontSize: '0.72rem',
                    borderRadius: 'var(--radius-sm)',
                    border: transferMode === 'import' ? '1px solid var(--accent-sky)' : '1px solid var(--border-glass)',
                    backgroundColor: transferMode === 'import' ? 'rgba(0, 255, 204, 0.15)' : 'transparent',
                    color: transferMode === 'import' ? 'var(--accent-sky)' : 'var(--text-muted)',
                    cursor: 'pointer',
                  }}
                >
                  Import Key
                </button>
              </div>
            </div>

            {transferMode === 'view' && (
              <>
                <p style={{ fontSize: '0.74rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                  Your unique Ed25519 public key certifies authorship of tracks and playlists across the decentralized swarm.
                </p>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <div
                    style={{
                      flex: 1,
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
                  {settings?.publisher_key && (
                    <button
                      type="button"
                      onClick={handleCopyKey}
                      style={{
                        padding: '8px 10px',
                        fontSize: '0.72rem',
                        borderRadius: 'var(--radius-sm)',
                        border: copiedKey ? '1px solid rgba(16, 185, 129, 0.4)' : '1px solid var(--border-glass)',
                        backgroundColor: copiedKey ? 'rgba(16, 185, 129, 0.15)' : 'rgba(255, 255, 255, 0.05)',
                        color: copiedKey ? 'var(--accent-emerald)' : 'var(--text-secondary)',
                        cursor: 'pointer',
                        display: 'flex',
                        alignItems: 'center',
                        gap: '4px',
                      }}
                    >
                      {copiedKey ? <Check size={13} /> : <Copy size={13} />}
                      <span>{copiedKey ? 'Copied!' : 'Copy'}</span>
                    </button>
                  )}
                </div>
              </>
            )}

            {transferMode === 'export' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                <p style={{ fontSize: '0.74rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                  Encrypt your private key with a passphrase to securely transfer it to another computer or device:
                </p>
                <div style={{ display: 'flex', gap: '8px' }}>
                  <input
                    type="password"
                    placeholder="Passphrase for encryption (min. 8 characters)..."
                    value={exportPassphrase}
                    onChange={(e) => setExportPassphrase(e.target.value)}
                    style={{
                      flex: 1,
                      padding: '7px 10px',
                      backgroundColor: 'rgba(0, 0, 0, 0.35)',
                      border: '1px solid var(--border-glass)',
                      borderRadius: 'var(--radius-sm)',
                      color: 'var(--text-primary)',
                      fontSize: '0.8rem',
                    }}
                  />
                  <Button
                    type="button"
                    size="sm"
                    variant="primary"
                    disabled={exportLoading || exportPassphrase.length < 8}
                    onClick={handleExportIdentity}
                    icon={<KeyRound size={14} />}
                  >
                    {exportLoading ? 'Encrypting...' : 'Export Key'}
                  </Button>
                </div>

                {exportError && (
                  <div style={{ color: 'var(--accent-rose)', fontSize: '0.74rem' }}>{exportError}</div>
                )}

                {exportedBundle && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginTop: '4px' }}>
                    <div
                      style={{
                        padding: '8px 10px',
                        backgroundColor: 'rgba(0, 0, 0, 0.45)',
                        border: '1px solid var(--border-glass)',
                        borderRadius: 'var(--radius-sm)',
                        fontFamily: 'var(--font-mono)',
                        fontSize: '0.7rem',
                        color: 'var(--text-secondary)',
                        wordBreak: 'break-all',
                        maxHeight: '70px',
                        overflowY: 'auto',
                      }}
                    >
                      {JSON.stringify(exportedBundle)}
                    </div>
                    <div style={{ display: 'flex', gap: '8px' }}>
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        onClick={() => {
                          navigator.clipboard.writeText(JSON.stringify(exportedBundle, null, 2));
                          setCopiedExport(true);
                          setTimeout(() => setCopiedExport(false), 2000);
                        }}
                        icon={copiedExport ? <Check size={13} /> : <Copy size={13} />}
                      >
                        {copiedExport ? 'Copied JSON!' : 'Copy JSON'}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        onClick={() => {
                          const jsonStr = JSON.stringify(exportedBundle, null, 2);
                          const blob = new Blob([jsonStr], { type: 'application/json' });
                          const url = URL.createObjectURL(blob);
                          const a = document.createElement('a');
                          a.href = url;
                          a.download = `resonamesh-identity-${exportedBundle.public_key.slice(0, 8)}.json`;
                          a.click();
                          URL.revokeObjectURL(url);
                        }}
                        icon={<Download size={13} />}
                      >
                        Download .json Keyfile
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            )}

            {transferMode === 'import' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                <p style={{ fontSize: '0.74rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                  Paste the encrypted JSON bundle from your other PC to transfer your identity:
                </p>
                <textarea
                  placeholder="Paste encrypted JSON bundle (starts with { &quot;kdf&quot;... })..."
                  value={importBundleText}
                  onChange={(e) => setImportBundleText(e.target.value)}
                  rows={2}
                  style={{
                    width: '100%',
                    padding: '8px 10px',
                    backgroundColor: 'rgba(0, 0, 0, 0.35)',
                    border: '1px solid var(--border-glass)',
                    borderRadius: 'var(--radius-sm)',
                    color: 'var(--text-primary)',
                    fontFamily: 'var(--font-mono)',
                    fontSize: '0.72rem',
                    resize: 'vertical',
                    boxSizing: 'border-box',
                  }}
                />
                <div style={{ display: 'flex', gap: '8px' }}>
                  <input
                    type="password"
                    placeholder="Enter decryption passphrase..."
                    value={importPassphrase}
                    onChange={(e) => setImportPassphrase(e.target.value)}
                    style={{
                      flex: 1,
                      padding: '7px 10px',
                      backgroundColor: 'rgba(0, 0, 0, 0.35)',
                      border: '1px solid var(--border-glass)',
                      borderRadius: 'var(--radius-sm)',
                      color: 'var(--text-primary)',
                      fontSize: '0.8rem',
                    }}
                  />
                  <Button
                    type="button"
                    size="sm"
                    variant="primary"
                    disabled={importLoading || !importBundleText || !importPassphrase}
                    onClick={handleImportIdentity}
                    icon={<FileText size={14} />}
                  >
                    {importLoading ? 'Importing...' : 'Apply Key'}
                  </Button>
                </div>

                {importError && (
                  <div style={{ color: 'var(--accent-rose)', fontSize: '0.74rem' }}>{importError}</div>
                )}

                {importSuccess && (
                  <div style={{ color: 'var(--accent-emerald)', fontSize: '0.74rem', fontWeight: 500 }}>
                    {importSuccess}
                  </div>
                )}
              </div>
            )}
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
              Public global endpoint of your node for direct P2P connections across the internet (STUN/UPnP).
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
              Automatically replicates rare tracks (&lt; 2 copies observed in swarm) in background. Yields instantly during playback.
            </p>
          </div>

          {/* Node networking summary */}
          {settings && (
            <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', display: 'flex', flexDirection: 'column', gap: '4px' }}>
              <div><ShieldCheck size={13} style={{ verticalAlign: 'middle', marginRight: '6px', color: 'var(--accent-emerald)' }} /> Local HTTP API: <code style={{ color: 'var(--text-primary)' }}>{settings.http_addr}</code></div>
              <div><ShieldCheck size={13} style={{ verticalAlign: 'middle', marginRight: '6px', color: 'var(--accent-emerald)' }} /> Swarm P2P Listen Port: <code style={{ color: 'var(--text-primary)' }}>{settings.p2p_port}</code></div>
            </div>
          )}

          {/* Autonomous P2P Client Updates */}
          {onOpenUpdater && (
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                padding: '12px 16px',
                backgroundColor: 'var(--bg-card)',
                borderRadius: 'var(--radius-md)',
                border: '1px solid var(--border-glass)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                <Sparkles size={16} style={{ color: 'var(--accent-cyan)' }} />
                <div>
                  <div style={{ fontSize: '0.82rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    Software Updates & Releases
                  </div>
                  <div style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
                    Autonomous cryptographic verification and P2P distribution
                  </div>
                </div>
              </div>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={() => {
                  onClose();
                  onOpenUpdater();
                }}
              >
                Check for Updates
              </Button>
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
