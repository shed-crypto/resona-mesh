import React, { useState } from 'react';
import { Radio, CheckCircle2, AlertCircle, Loader2, Copy, Check, Share2 } from 'lucide-react';
import { Modal } from '../../shared/components/Modal';
import { Button } from '../../shared/components/Button';
import { apiClient } from '../../shared/api/client';
import { useNodeStatus } from '../../shared/hooks/useNodeStatus';

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

  const dialableAddrs = (status?.listen_addrs || []).filter(Boolean);
  const primaryAddr = dialableAddrs.find((a) => !a.includes('/127.0.0.1/') && !a.includes('/169.254.')) || dialableAddrs[0];

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
      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
        {/* Your Multiaddress Sharing Section */}
        <div
          style={{
            padding: '14px',
            backgroundColor: 'rgba(255, 255, 255, 0.03)',
            border: '1px solid var(--border-glass)',
            borderRadius: 'var(--radius-md)',
            display: 'flex',
            flexDirection: 'column',
            gap: '8px',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '6px' }}>
            <span style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-primary)', display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Share2 size={14} style={{ color: 'var(--accent-sky)' }} />
              Ваша адреса для прямого підключення (Share):
            </span>
            {primaryAddr && (
              <button
                type="button"
                onClick={() => handleCopy(primaryAddr)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  background: copiedAddr === primaryAddr ? 'rgba(16, 185, 129, 0.2)' : 'rgba(255, 255, 255, 0.08)',
                  border: copiedAddr === primaryAddr ? '1px solid rgba(16, 185, 129, 0.4)' : '1px solid var(--border-glass)',
                  borderRadius: 'var(--radius-sm)',
                  padding: '3px 8px',
                  color: copiedAddr === primaryAddr ? 'var(--accent-emerald)' : 'var(--text-secondary)',
                  cursor: 'pointer',
                  fontSize: '0.75rem',
                  fontWeight: 500,
                  transition: 'all 0.2s',
                }}
              >
                {copiedAddr === primaryAddr ? <Check size={12} /> : <Copy size={12} />}
                {copiedAddr === primaryAddr ? 'Скопійовано!' : 'Копіювати'}
              </button>
            )}
          </div>

          {dialableAddrs.length > 0 ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', marginTop: '4px' }}>
              {dialableAddrs.map((addr) => {
                const isCopied = copiedAddr === addr;
                return (
                  <div
                    key={addr}
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
                        fontSize: '0.73rem',
                        color: 'var(--text-secondary)',
                        wordBreak: 'break-all',
                        userSelect: 'all',
                      }}
                    >
                      {addr}
                    </code>
                    <button
                      type="button"
                      onClick={() => handleCopy(addr)}
                      title="Копіювати цю адресу"
                      style={{
                        background: 'none',
                        border: 'none',
                        padding: '4px',
                        cursor: 'pointer',
                        color: isCopied ? 'var(--accent-emerald)' : 'var(--text-muted)',
                        flexShrink: 0,
                      }}
                    >
                      {isCopied ? <Check size={14} /> : <Copy size={14} />}
                    </button>
                  </div>
                );
              })}
            </div>
          ) : (
            <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
              {status?.peer_id ? `Peer ID: ${status.peer_id}` : 'Адреса вузла визначається...'}
            </span>
          )}
          <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
            Надішліть цю адресу іншому користувачу, щоб він вставив її нижче для прямого з'єднання.
          </span>
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
