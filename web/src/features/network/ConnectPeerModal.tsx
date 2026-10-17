import React, { useState } from 'react';
import { Radio, CheckCircle2, AlertCircle, Loader2 } from 'lucide-react';
import { Modal } from '../../shared/components/Modal';
import { Button } from '../../shared/components/Button';
import { apiClient } from '../../shared/api/client';

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
  const [multiaddr, setMultiaddr] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successPeerID, setSuccessPeerID] = useState<string | null>(null);

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
