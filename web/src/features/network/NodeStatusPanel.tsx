import React, { useState } from 'react';
import { HardDrive, Key, Activity, Clock, Radio, Settings, UserPlus, Share2, Check } from 'lucide-react';
import { useNodeStatus } from '../../shared/hooks/useNodeStatus';
import { Badge } from '../../shared/components/Badge';
import { ConnectPeerModal } from './ConnectPeerModal';
import { SettingsModal } from './SettingsModal';

export const NodeStatusPanel: React.FC = () => {
  const { status, loading, error, refresh } = useNodeStatus();
  const [isConnectOpen, setIsConnectOpen] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [copiedAddress, setCopiedAddress] = useState(false);

  const dialableAddrs = (status?.listen_addrs || []).filter(Boolean);
  const primaryAddr = dialableAddrs.find((a) => !a.includes('/127.0.0.1/') && !a.includes('/169.254.')) || dialableAddrs[0];

  const handleCopyMyAddress = () => {
    if (!primaryAddr) return;
    navigator.clipboard.writeText(primaryAddr);
    setCopiedAddress(true);
    setTimeout(() => setCopiedAddress(false), 2500);
  };

  const formatBytes = (bytes: number) => {
    if (bytes < 1024 * 1024) {
      return `${(bytes / 1024).toFixed(1)} KiB`;
    }
    if (bytes < 1024 * 1024 * 1024) {
      return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
    }
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
  };

  const formatUptime = (secs: number) => {
    const hours = Math.floor(secs / 3600);
    const mins = Math.floor((secs % 3600) / 60);
    if (hours > 0) return `${hours}h ${mins}m`;
    return `${mins}m ${secs % 60}s`;
  };

  if (loading && !status) {
    return null;
  }

  if (error || !status) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          padding: '8px 14px',
          backgroundColor: 'rgba(244, 63, 94, 0.1)',
          border: '1px solid rgba(244, 63, 94, 0.3)',
          borderRadius: 'var(--radius-md)',
          fontSize: '0.8rem',
          color: 'var(--accent-rose)',
        }}
      >
        <Activity size={14} /> Node Offline
      </div>
    );
  }

  const shortKey = status.publisher_key
    ? `${status.publisher_key.slice(0, 8)}...${status.publisher_key.slice(-6)}`
    : 'anonymous';

  const usedPct =
    status.storage_quota_bytes > 0
      ? (status.storage_used_bytes / status.storage_quota_bytes) * 100
      : 0;

  return (
    <>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '14px',
          flexWrap: 'wrap',
          padding: '8px 16px',
          backgroundColor: 'var(--bg-surface)',
          border: '1px solid var(--border-glass)',
          borderRadius: 'var(--radius-lg)',
          fontSize: '0.8rem',
          color: 'var(--text-secondary)',
        }}
      >
        {/* Node Status Indicator */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <span
            style={{
              width: '8px',
              height: '8px',
              borderRadius: 'var(--radius-full)',
              backgroundColor: 'var(--accent-emerald)',
              boxShadow: '0 0 8px var(--accent-emerald)',
            }}
          />
          <span style={{ fontWeight: 600, color: 'var(--text-primary)' }}>ResonaMesh</span>
          <Badge variant="neutral">v{status.version}</Badge>
        </div>

        {/* P2P Swarm Peers Button */}
        <button
          type="button"
          onClick={() => setIsConnectOpen(true)}
          title="Click to manually dial/connect a swarm peer"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            background: 'none',
            border: 'none',
            padding: '2px 6px',
            borderRadius: 'var(--radius-sm)',
            cursor: 'pointer',
            color: 'inherit',
            transition: 'background 0.2s',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.05)')}
          onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
        >
          <Radio
            size={14}
            style={{
              color: (status.connected_peers ?? 0) > 0 ? 'var(--accent-emerald)' : 'var(--text-muted)',
            }}
          />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>
            {status.connected_peers ?? 0} {(status.connected_peers ?? 0) === 1 ? 'peer' : 'peers'}
          </span>
          <UserPlus size={12} style={{ color: 'var(--text-muted)', marginLeft: '2px' }} />
        </button>

        {/* Quick Multiaddress Copy Button */}
        {primaryAddr && (
          <button
            type="button"
            onClick={handleCopyMyAddress}
            title={copiedAddress ? 'Адресу скопійовано!' : `Скопіювати адресу вузла для прямого з'єднання:\n${primaryAddr}`}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '5px',
              background: copiedAddress ? 'rgba(16, 185, 129, 0.15)' : 'none',
              border: copiedAddress ? '1px solid rgba(16, 185, 129, 0.4)' : '1px solid rgba(255, 255, 255, 0.08)',
              padding: '2px 8px',
              borderRadius: 'var(--radius-sm)',
              cursor: 'pointer',
              color: copiedAddress ? 'var(--accent-emerald)' : 'var(--text-secondary)',
              transition: 'all 0.2s',
            }}
            onMouseEnter={(e) => {
              if (!copiedAddress) e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.05)';
            }}
            onMouseLeave={(e) => {
              if (!copiedAddress) e.currentTarget.style.backgroundColor = 'transparent';
            }}
          >
            {copiedAddress ? <Check size={13} /> : <Share2 size={13} style={{ color: 'var(--accent-sky)' }} />}
            <span style={{ fontSize: '0.75rem', fontWeight: 500 }}>
              {copiedAddress ? 'Скопійовано!' : 'Копіювати адресу'}
            </span>
          </button>
        )}

        {/* Node Identity Key */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }} title={`Full Key: ${status.publisher_key}`}>
          <Key size={14} style={{ color: 'var(--text-muted)' }} />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>{shortKey}</span>
        </div>

        {/* Storage Usage Button */}
        <button
          type="button"
          onClick={() => setIsSettingsOpen(true)}
          title="Click to view storage quota breakdown and settings"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            background: 'none',
            border: 'none',
            padding: '2px 6px',
            borderRadius: 'var(--radius-sm)',
            cursor: 'pointer',
            color: 'inherit',
            transition: 'background 0.2s',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.05)')}
          onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
        >
          <HardDrive size={14} style={{ color: 'var(--accent-sky)' }} />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>
            {formatBytes(status.storage_used_bytes)} / {formatBytes(status.storage_quota_bytes)} ({usedPct.toFixed(1)}%)
          </span>
        </button>

        {/* Uptime */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <Clock size={14} style={{ color: 'var(--text-muted)' }} />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>
            {formatUptime(status.uptime_seconds)}
          </span>
        </div>

        {/* Settings Button */}
        <button
          type="button"
          onClick={() => setIsSettingsOpen(true)}
          title="Open Node Settings"
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background: 'none',
            border: 'none',
            padding: '4px',
            borderRadius: 'var(--radius-sm)',
            cursor: 'pointer',
            color: 'var(--text-secondary)',
            transition: 'color 0.2s, background 0.2s',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.05)';
            e.currentTarget.style.color = 'var(--text-primary)';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.backgroundColor = 'transparent';
            e.currentTarget.style.color = 'var(--text-secondary)';
          }}
        >
          <Settings size={15} />
        </button>
      </div>

      <ConnectPeerModal
        isOpen={isConnectOpen}
        onClose={() => setIsConnectOpen(false)}
        onConnected={() => refresh()}
      />

      <SettingsModal
        isOpen={isSettingsOpen}
        onClose={() => setIsSettingsOpen(false)}
        onSettingsUpdated={() => refresh()}
      />
    </>
  );
};
