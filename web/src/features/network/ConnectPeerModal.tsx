import React, { useState, useEffect } from 'react';
import { Radio, CheckCircle2, AlertCircle, Loader2, Copy, Check, Share2, Activity, RefreshCw, ShieldAlert } from 'lucide-react';
import { Modal } from '../../shared/components/Modal';
import { Button } from '../../shared/components/Button';
import { apiClient } from '../../shared/api/client';
import { useNodeStatus } from '../../shared/hooks/useNodeStatus';
import type { PeerRecord } from '../../types/node';

interface ConnectPeerModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConnected?: () => void;
}

export const ConnectPeerModal: React.FC<ConnectPeerModalProps> = ({
  isOpen,
  onClose,
  onConnected,
}) => {
  const { status } = useNodeStatus();
  const [multiaddr, setMultiaddr] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successPeerID, setSuccessPeerID] = useState<string | null>(null);
  const [copiedAddr, setCopiedAddr] = useState<string | null>(null);
  const [showAllAddrs, setShowAllAddrs] = useState(false);
  const [peers, setPeers] = useState<PeerRecord[]>([]);
  const [syncing, setSyncing] = useState(false);
  const [syncMsg, setSyncMsg] = useState<string | null>(null);

  const fetchPeers = async () => {
    try {
      const res = await apiClient.getPeers();
      setPeers(res.peers || []);
    } catch {
      // ignore
    }
  };

  useEffect(() => {
    if (isOpen) {
      fetchPeers();
    }
  }, [isOpen]);

  const handleSync = async () => {
    setSyncing(true);
    setSyncMsg(null);
    try {
      const res = await apiClient.syncPeers();
      setSyncMsg(`Synced! (${res.imported_tracks} tracks)`);
      fetchPeers();
      setTimeout(() => setSyncMsg(null), 3000);
    } catch (err: unknown) {
      setSyncMsg(err instanceof Error ? err.message : 'Sync failed');
    } finally {
      setSyncing(false);
    }
  };

  const rawAddrs = (status?.listen_addrs || []).filter(Boolean);
  // Filter out dummy APIPA (169.254.x.x) and loopback unless nothing else exists
  const cleanAddrs = rawAddrs.filter((a) => !a.includes('/169.254.') && !a.includes('/127.0.0.1/'));
  const dialableAddrs = cleanAddrs.length > 0 ? cleanAddrs : rawAddrs;
  const primaryAddr = dialableAddrs[0];
  const secondaryAddrs = dialableAddrs.slice(1);

  const handleCopy = (addr: string) => {
    navigator.clipboard.writeText(addr);
    setCopiedAddr(addr);
    setTimeout(() => setCopiedAddr(null), 2500);
  };

  const resetState = () => {
    setMultiaddr('');
    setLoading(false);
    setError(null);
    setSuccessPeerID(null);
    setShowAllAddrs(false);
  };

  const handleClose = () => {
    resetState();
    onClose();
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const addr = multiaddr.trim();
    if (!addr) {
      setError('Please enter a valid libp2p multiaddress.');
      return;
    }

    setLoading(true);
    setError(null);
    setSuccessPeerID(null);

    try {
      const res = await apiClient.connectPeer(addr);
      setSuccessPeerID(res.peer_id);
      if (onConnected) {
        onConnected();
      }
      setTimeout(() => {
        handleClose();
      }, 1500);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to connect to peer';
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title="Connect to Swarm Peer">
      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
        {/* Your Multiaddress Sharing Section */}
        <div
          style={{
            padding: '12px 14px',
            backgroundColor: 'rgba(255, 255, 255, 0.03)',
            border: '1px solid var(--border-glass)',
            borderRadius: 'var(--radius-md)',
            display: 'flex',
            flexDirection: 'column',
            gap: '8px',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '8px' }}>
            <span style={{ fontSize: '0.78rem', fontWeight: 600, color: 'var(--text-primary)', display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Share2 size={13} style={{ color: 'var(--accent-sky)' }} />
              Ваша адреса для підключення:
            </span>
            {secondaryAddrs.length > 0 && (
              <button
                type="button"
                onClick={() => setShowAllAddrs(!showAllAddrs)}
                style={{
                  background: 'none',
                  border: 'none',
                  color: 'var(--accent-sky)',
                  fontSize: '0.72rem',
                  cursor: 'pointer',
                  textDecoration: 'underline',
                  padding: 0,
                }}
              >
                {showAllAddrs ? 'Приховати' : `Ще адреси (${secondaryAddrs.length})`}
              </button>
            )}
          </div>

          {primaryAddr ? (
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: '8px',
                padding: '6px 10px',
                backgroundColor: 'var(--bg-card)',
                borderRadius: 'var(--radius-sm)',
                border: '1px solid rgba(255, 255, 255, 0.05)',
              }}
            >
              <code
                style={{
                  fontFamily: 'var(--font-mono)',
                  fontSize: '0.72rem',
                  color: 'var(--accent-sky)',
                  wordBreak: 'break-all',
                  userSelect: 'all',
                  lineHeight: 1.4,
                }}
              >
                {primaryAddr}
              </code>
              <button
                type="button"
                onClick={() => handleCopy(primaryAddr)}
                title="Копіювати адресу"
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  background: copiedAddr === primaryAddr ? 'rgba(16, 185, 129, 0.2)' : 'rgba(255, 255, 255, 0.08)',
                  border: copiedAddr === primaryAddr ? '1px solid rgba(16, 185, 129, 0.4)' : '1px solid var(--border-glass)',
                  borderRadius: 'var(--radius-sm)',
                  padding: '4px 8px',
                  color: copiedAddr === primaryAddr ? 'var(--accent-emerald)' : 'var(--text-secondary)',
                  cursor: 'pointer',
                  fontSize: '0.72rem',
                  fontWeight: 500,
                  flexShrink: 0,
                  transition: 'all 0.2s',
                }}
              >
                {copiedAddr === primaryAddr ? <Check size={12} /> : <Copy size={12} />}
                {copiedAddr === primaryAddr ? 'Скопійовано' : 'Копіювати'}
              </button>
            </div>
          ) : (
            <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
              {status?.peer_id ? `Peer ID: ${status.peer_id}` : 'Адреса вузла визначається...'}
            </span>
          )}

          {showAllAddrs && secondaryAddrs.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', maxHeight: '100px', overflowY: 'auto' }}>
              {secondaryAddrs.map((addr) => {
                const isCopied = copiedAddr === addr;
                return (
                  <div
                    key={addr}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      gap: '8px',
                      padding: '4px 8px',
                      backgroundColor: 'rgba(0, 0, 0, 0.3)',
                      borderRadius: 'var(--radius-sm)',
                      fontSize: '0.7rem',
                    }}
                  >
                    <code style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', wordBreak: 'break-all' }}>
                      {addr}
                    </code>
                    <button
                      type="button"
                      onClick={() => handleCopy(addr)}
                      style={{
                        background: 'none',
                        border: 'none',
                        cursor: 'pointer',
                        color: isCopied ? 'var(--accent-emerald)' : 'var(--text-muted)',
                        flexShrink: 0,
                        padding: '2px',
                      }}
                    >
                      {isCopied ? <Check size={12} /> : <Copy size={12} />}
                    </button>
                  </div>
                );
              })}
            </div>
          )}

          <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>
            Надішліть цю адресу іншому користувачу для прямого з'єднання через LAN або WAN.
          </span>
        </div>

        {/* Active Swarm Peers & Reputation Section */}
        <div
          style={{
            padding: '12px 14px',
            backgroundColor: 'rgba(255, 255, 255, 0.02)',
            border: '1px solid var(--border-glass)',
            borderRadius: 'var(--radius-md)',
            display: 'flex',
            flexDirection: 'column',
            gap: '10px',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-primary)', display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Activity size={14} style={{ color: 'var(--accent-primary)' }} />
              Connected Swarm Peers ({peers.length})
            </span>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              {syncMsg && (
                <span style={{ fontSize: '0.72rem', color: 'var(--accent-emerald)' }}>{syncMsg}</span>
              )}
              {peers.length > 0 && (
                <button
                  type="button"
                  onClick={handleSync}
                  disabled={syncing}
                  title="Trigger catalog sync with connected peers"
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                    background: 'none',
                    border: 'none',
                    color: 'var(--accent-primary)',
                    fontSize: '0.72rem',
                    cursor: 'pointer',
                    padding: 0,
                  }}
                >
                  <RefreshCw size={11} className={syncing ? 'spin' : ''} />
                  {syncing ? 'Syncing...' : 'Sync Catalog'}
                </button>
              )}
            </div>
          </div>

          {peers.length === 0 ? (
            <p style={{ margin: 0, fontSize: '0.75rem', color: 'var(--text-muted)' }}>
              No peers connected yet. Dial an address below or discover local peers via LAN mDNS.
            </p>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', maxHeight: '140px', overflowY: 'auto' }}>
              {peers.map((p) => {
                const isQuarantined = p.is_quarantined;
                const score = p.reputation_score ?? 100;
                const latency = p.avg_latency_ms;
                const blocks = p.blocks_verified ?? 0;
                const mismatches = p.hash_mismatches ?? 0;

                return (
                  <div
                    key={p.peer_id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '6px 10px',
                      backgroundColor: 'var(--bg-card)',
                      borderRadius: 'var(--radius-sm)',
                      border: isQuarantined
                        ? '1px solid rgba(244, 63, 94, 0.3)'
                        : '1px solid rgba(255, 255, 255, 0.05)',
                      fontSize: '0.75rem',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                      <span
                        style={{
                          width: '7px',
                          height: '7px',
                          borderRadius: '50%',
                          backgroundColor: isQuarantined
                            ? 'var(--accent-rose)'
                            : p.connected
                            ? 'var(--accent-emerald)'
                            : 'var(--text-muted)',
                          flexShrink: 0,
                        }}
                      />
                      <code
                        style={{
                          fontFamily: 'var(--font-mono)',
                          fontSize: '0.72rem',
                          color: 'var(--text-primary)',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                          maxWidth: '120px',
                        }}
                        title={p.peer_id}
                      >
                        {p.peer_id.slice(0, 8)}...{p.peer_id.slice(-6)}
                      </code>
                    </div>

                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexShrink: 0 }}>
                      {isQuarantined ? (
                        <span
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: '3px',
                            color: 'var(--accent-rose)',
                            fontSize: '0.68rem',
                            fontWeight: 600,
                            backgroundColor: 'rgba(244, 63, 94, 0.12)',
                            padding: '2px 6px',
                            borderRadius: 'var(--radius-sm)',
                          }}
                          title={p.quarantined_until ? `Quarantined until ${p.quarantined_until}` : 'Quarantined'}
                        >
                          <ShieldAlert size={10} /> Quarantined
                        </span>
                      ) : (
                        <span
                          style={{
                            color: score >= 100 ? 'var(--accent-emerald)' : 'var(--accent-amber)',
                            fontSize: '0.68rem',
                            fontWeight: 600,
                            backgroundColor: score >= 100 ? 'rgba(16, 185, 129, 0.1)' : 'rgba(245, 158, 11, 0.1)',
                            padding: '2px 6px',
                            borderRadius: 'var(--radius-sm)',
                          }}
                          title="Swarm reputation score"
                        >
                          {score.toFixed(0)} pts
                        </span>
                      )}

                      {latency && latency > 0 ? (
                        <span style={{ fontSize: '0.68rem', color: 'var(--text-muted)' }}>
                          ~{latency.toFixed(0)}ms
                        </span>
                      ) : null}

                      {blocks > 0 && (
                        <span style={{ fontSize: '0.68rem', color: 'var(--text-muted)' }}>
                          {blocks} blk
                        </span>
                      )}

                      {mismatches > 0 && (
                        <span style={{ fontSize: '0.68rem', color: 'var(--accent-rose)', fontWeight: 600 }}>
                          {mismatches} err
                        </span>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
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

        {successPeerID && (
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
            <span>Connected to peer: {successPeerID.slice(0, 12)}...</span>
          </div>
        )}

        <div>
          <label
            style={{
              display: 'block',
              fontSize: '0.85rem',
              fontWeight: 500,
              marginBottom: '8px',
              color: 'var(--text-primary)',
            }}
          >
            Peer Multiaddress
          </label>
          <input
            type="text"
            value={multiaddr}
            onChange={(e) => setMultiaddr(e.target.value)}
            placeholder="/ip4/192.168.1.12/tcp/4002/p2p/12D3KooW..."
            disabled={loading || Boolean(successPeerID)}
            style={{
              width: '100%',
              padding: '10px 14px',
              backgroundColor: 'var(--bg-card)',
              border: '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-md)',
              color: 'var(--text-primary)',
              fontFamily: 'var(--font-mono)',
              fontSize: '0.82rem',
              outline: 'none',
              boxSizing: 'border-box',
            }}
          />
          <p
            style={{
              fontSize: '0.75rem',
              color: 'var(--text-muted)',
              marginTop: '6px',
              lineHeight: 1.4,
            }}
          >
            Enter a standard libp2p multiaddress formatted with transport and remote PeerID to dial through LAN or explicit route.
          </p>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '10px' }}>
          <Button variant="secondary" size="md" type="button" onClick={handleClose} disabled={loading}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="md"
            type="submit"
            disabled={loading || Boolean(successPeerID)}
            icon={loading ? <Loader2 size={16} className="spin" /> : <Radio size={16} />}
          >
            {loading ? 'Dialing...' : successPeerID ? 'Connected!' : 'Connect Peer'}
          </Button>
        </div>
      </form>
    </Modal>
  );
};
