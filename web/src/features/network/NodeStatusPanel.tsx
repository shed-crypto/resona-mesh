import React from 'react';
import { HardDrive, Key, Activity, Clock } from 'lucide-react';
import { useNodeStatus } from '../../shared/hooks/useNodeStatus';
import { Badge } from '../../shared/components/Badge';

export const NodeStatusPanel: React.FC = () => {
  const { status, loading, error } = useNodeStatus();

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
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '16px',
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

      {/* Node Identity Key */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }} title={`Full Key: ${status.publisher_key}`}>
        <Key size={14} style={{ color: 'var(--text-muted)' }} />
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>{shortKey}</span>
      </div>

      {/* Storage Usage */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
        <HardDrive size={14} style={{ color: 'var(--text-muted)' }} />
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>
          {formatBytes(status.storage_used_bytes)} / {formatBytes(status.storage_quota_bytes)} ({usedPct.toFixed(1)}%)
        </span>
      </div>

      {/* Uptime */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
        <Clock size={14} style={{ color: 'var(--text-muted)' }} />
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '0.75rem' }}>
          {formatUptime(status.uptime_seconds)}
        </span>
      </div>
    </div>
  );
};
