import React, { useState } from 'react';
import { HardDrive, Key, Activity, Clock, Radio, Settings, UserPlus, Share2, Check, Globe } from 'lucide-react';
import { useNodeStatus } from '../../shared/hooks/useNodeStatus';
import { Badge } from '../../shared/components/Badge';
import { ConnectPeerModal } from './ConnectPeerModal';
import { SettingsModal } from './SettingsModal';
import { UpdaterModal } from './UpdaterModal';
import { useAutoUpdater } from '../../shared/hooks/useAutoUpdater';
import { Sparkles, Loader2 } from 'lucide-react';

export const NodeStatusPanel: React.FC = () => {
  const { status, loading, error, refresh } = useNodeStatus();
  const [isConnectOpen, setIsConnectOpen] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isUpdaterOpen, setIsUpdaterOpen] = useState(false);
  const { state: updaterState } = useAutoUpdater();
  const [copiedAddress, setCopiedAddress] = useState(false);
  const [copiedKey, setCopiedKey] = useState(false);

  const dialableAddrs = (status?.listen_addrs || []).filter(Boolean);
  const primaryAddr = dialableAddrs.find((a) => !a.includes('/127.0.0.1/') && !a.includes('/169.254.')) || dialableAddrs[0];

  const handleCopyMyAddress = () => {
    if (!primaryAddr) return;
    navigator.clipboard.writeText(primaryAddr);
    setCopiedAddress(true);
    setTimeout(() => setCopiedAddress(false), 2500);
  };

  const handleCopyMyKey = () => {
    if (!status?.publisher_key) return;
    navigator.clipboard.writeText(status.publisher_key);
    setCopiedKey(true);
    setTimeout(() => setCopiedKey(false), 2500);
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
          {/* Swarm P2P Auto-Updater Indicator */}
          {updaterState.status === 'available' && (
            <button
              onClick={() => setIsUpdaterOpen(true)}
              title={`New update v${updaterState.available_release?.version} available. Click to inspect & download over P2P.`}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '4px',
                padding: '2px 8px',
                borderRadius: 'var(--radius-full)',
                backgroundColor: 'rgba(56, 189, 248, 0.15)',
                border: '1px solid rgba(56, 189, 248, 0.4)',
                color: 'var(--accent-cyan)',
                fontSize: '0.72rem',
                fontWeight: 600,
                cursor: 'pointer',
              }}
            >
              <Sparkles size={11} />
              <span>Update v{updaterState.available_release?.version}</span>
            </button>
          )}

          {updaterState.status === 'downloading' && (
            <button
              onClick={() => setIsUpdaterOpen(true)}
              title="Downloading release chunks from swarm peers"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '4px',
                padding: '2px 8px',
                borderRadius: 'var(--radius-full)',
                backgroundColor: 'rgba(56, 189, 248, 0.15)',
                border: '1px solid var(--accent-cyan)',
                color: 'var(--accent-cyan)',
                fontSize: '0.72rem',
                fontWeight: 600,
                cursor: 'pointer',
              }}
            >
              <Loader2 size={11} className="animate-spin" />
              <span>{updaterState.progress_pct.toFixed(0)}%</span>
            </button>
          )}

          {updaterState.status === 'ready_to_restart' && (
            <button
              onClick={() => setIsUpdaterOpen(true)}
              title="Release binary verified and ready to apply. Click to restart."
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '4px',
                padding: '2px 8px',
                borderRadius: 'var(--radius-full)',
                backgroundColor: 'rgba(16, 185, 129, 0.18)',
                border: '1px solid var(--accent-emerald)',
                color: 'var(--accent-emerald)',
                fontSize: '0.72rem',
                fontWeight: 600,
                cursor: 'pointer',
                boxShadow: '0 0 10px rgba(16, 185, 129, 0.25)',
              }}
            >
              <Check size={11} />
              <span>Restart & Update</span>
            </button>
          )}

        </div>

        {/* Direct Swarm Peers (Seeds & Clients) */}
        <button
          type="button"
          onClick={() => setIsConnectOpen(true)}
          title="Swarm Peers (Direct Audio Streamers & Seeds). Click to dial/connect manually."
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
              color: (status.direct_peers ?? status.connected_peers ?? 0) > 0 ? 'var(--accent-emerald)' : 'var(--text-muted)',
            }}
          />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>
            {status.direct_peers ?? status.connected_peers ?? 0} {(status.direct_peers ?? status.connected_peers ?? 0) === 1 ? 'peer' : 'peers'}
          </span>
          <UserPlus size={12} style={{ color: 'var(--text-muted)', marginLeft: '2px' }} />
        </button>

        {/* Cloud Relay / Directing Server Status Badge */}
        <div
          title={status.relay_connected ? 'Cloud Swarm Relay: Connected & Routing' : 'Cloud Swarm Relay: Searching for beacon...'}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '5px',
            padding: '2px 7px',
            borderRadius: 'var(--radius-sm)',
            backgroundColor: status.relay_connected ? 'rgba(0, 255, 204, 0.08)' : 'rgba(255, 255, 255, 0.03)',
            border: status.relay_connected ? '1px solid rgba(0, 255, 204, 0.25)' : '1px solid var(--border-glass)',
            fontSize: '0.72rem',
            color: status.relay_connected ? 'var(--accent-sky)' : 'var(--text-muted)',
          }}
        >
          <Globe
            size={13}
            style={{
              color: status.relay_connected ? 'var(--accent-emerald)' : 'var(--text-muted)',
            }}
          />
          <span style={{ fontWeight: 500 }}>
            {status.relay_connected ? 'Relay Online' : 'Relay Standby'}
          </span>
        </div>

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
        <button
          type="button"
          onClick={handleCopyMyKey}
          title={copiedKey ? 'Ключ скопійовано!' : `Скопіювати публічний ключ вузла:\n${status.publisher_key}`}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            background: copiedKey ? 'rgba(16, 185, 129, 0.15)' : 'none',
            border: copiedKey ? '1px solid rgba(16, 185, 129, 0.4)' : '1px solid transparent',
            padding: '2px 7px',
            borderRadius: 'var(--radius-sm)',
            cursor: 'pointer',
            color: copiedKey ? 'var(--accent-emerald)' : 'inherit',
            transition: 'all 0.2s',
          }}
          onMouseEnter={(e) => {
            if (!copiedKey) e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.05)';
          }}
          onMouseLeave={(e) => {
            if (!copiedKey) e.currentTarget.style.backgroundColor = 'transparent';
          }}
        >
          {copiedKey ? <Check size={13} /> : <Key size={14} style={{ color: 'var(--text-muted)' }} />}
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>
            {copiedKey ? 'Скопійовано!' : shortKey}
          </span>
        </button>

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
      <UpdaterModal isOpen={isUpdaterOpen} onClose={() => setIsUpdaterOpen(false)} />
    </>
  );
};
