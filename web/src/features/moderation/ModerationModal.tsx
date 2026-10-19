import React, { useState, useEffect, useCallback } from 'react';
import { RefreshCw, FileText, Ban, CheckCircle2, Clock } from 'lucide-react';
import { Modal } from '../../shared/components/Modal';
import { Button } from '../../shared/components/Button';
import { Badge } from '../../shared/components/Badge';
import { apiClient } from '../../shared/api/client';
import type { Report, BlockedItem } from '../../types/track';

interface ModerationModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const ModerationModal: React.FC<ModerationModalProps> = ({ isOpen, onClose }) => {
  const [activeTab, setActiveTab] = useState<'reports' | 'blocked'>('reports');
  const [reports, setReports] = useState<Report[]>([]);
  const [blockedItems, setBlockedItems] = useState<BlockedItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [repRes, blkRes] = await Promise.all([
        apiClient.getReports(),
        apiClient.getBlocked(),
      ]);
      setReports(repRes.items || []);
      setBlockedItems(blkRes.items || []);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load moderation data';
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (isOpen) {
      fetchData();
    }
  }, [isOpen, fetchData]);

  const formatDate = (sec: number) => {
    if (!sec) return 'N/A';
    return new Date(sec * 1000).toLocaleString();
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Community Moderation & Policy">
      <div style={{ display: 'flex', flexDirection: 'column', gap: '18px' }}>
        {/* Navigation Tabs */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom: '1px solid var(--border-glass)',
            paddingBottom: '12px',
          }}
        >
          <div style={{ display: 'flex', gap: '8px' }}>
            <button
              onClick={() => setActiveTab('reports')}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                padding: '8px 14px',
                borderRadius: 'var(--radius-md)',
                backgroundColor: activeTab === 'reports' ? 'var(--bg-card)' : 'transparent',
                color: activeTab === 'reports' ? 'var(--text-primary)' : 'var(--text-secondary)',
                border: activeTab === 'reports' ? '1px solid var(--border-glass)' : '1px solid transparent',
                fontSize: '0.85rem',
                fontWeight: activeTab === 'reports' ? 600 : 400,
                cursor: 'pointer',
              }}
            >
              <FileText size={16} />
              <span>Outbox Reports</span>
              <span
                style={{
                  padding: '2px 6px',
                  borderRadius: 'var(--radius-full)',
                  backgroundColor: 'rgba(255, 255, 255, 0.08)',
                  fontSize: '0.7rem',
                  fontFamily: 'var(--font-mono)',
                }}
              >
                {reports.length}
              </span>
            </button>

            <button
              onClick={() => setActiveTab('blocked')}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                padding: '8px 14px',
                borderRadius: 'var(--radius-md)',
                backgroundColor: activeTab === 'blocked' ? 'var(--bg-card)' : 'transparent',
                color: activeTab === 'blocked' ? 'var(--text-primary)' : 'var(--text-secondary)',
                border: activeTab === 'blocked' ? '1px solid var(--border-glass)' : '1px solid transparent',
                fontSize: '0.85rem',
                fontWeight: activeTab === 'blocked' ? 600 : 400,
                cursor: 'pointer',
              }}
            >
              <Ban size={16} />
              <span>Blocked Content</span>
              <span
                style={{
                  padding: '2px 6px',
                  borderRadius: 'var(--radius-full)',
                  backgroundColor: 'rgba(244, 63, 94, 0.15)',
                  color: 'var(--accent-rose)',
                  fontSize: '0.7rem',
                  fontFamily: 'var(--font-mono)',
                }}
              >
                {blockedItems.length}
              </span>
            </button>
          </div>

          <Button
            variant="ghost"
            size="sm"
            onClick={fetchData}
            disabled={loading}
            icon={<RefreshCw size={14} className={loading ? 'spin' : ''} />}
          >
            Refresh
          </Button>
        </div>

        {error && (
          <div
            style={{
              padding: '10px 14px',
              backgroundColor: 'rgba(244, 63, 94, 0.1)',
              border: '1px solid rgba(244, 63, 94, 0.3)',
              borderRadius: 'var(--radius-md)',
              color: 'var(--accent-rose)',
              fontSize: '0.82rem',
            }}
          >
            {error}
          </div>
        )}

        {/* Tab 1: Outbox Reports */}
        {activeTab === 'reports' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', maxHeight: '420px', overflowY: 'auto' }}>
            {reports.length === 0 ? (
              <div
                style={{
                  padding: '36px 20px',
                  textAlign: 'center',
                  color: 'var(--text-muted)',
                  fontSize: '0.85rem',
                  border: '1px dashed var(--border-glass)',
                  borderRadius: 'var(--radius-md)',
                }}
              >
                No reports submitted. Use the report button on track cards to file abuse or copyright claims.
              </div>
            ) : (
              reports.map((r) => (
                <div
                  key={r.event_id}
                  style={{
                    padding: '14px',
                    backgroundColor: 'var(--bg-card)',
                    border: '1px solid var(--border-glass)',
                    borderRadius: 'var(--radius-md)',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '8px',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <Badge variant={r.reason_code === 'COPYRIGHT_CLAIM' ? 'rose' : 'amber'}>
                        {r.reason_code.replace(/_/g, ' ')}
                      </Badge>
                      <span
                        style={{
                          fontFamily: 'var(--font-mono)',
                          fontSize: '0.75rem',
                          color: 'var(--text-muted)',
                        }}
                      >
                        Track: {r.content_id.slice(0, 16)}...
                      </span>
                    </div>

                    <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                      {r.status === 'ACCEPTED' ? (
                        <span
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: '4px',
                            color: 'var(--accent-emerald)',
                            fontSize: '0.75rem',
                            fontWeight: 600,
                          }}
                        >
                          <CheckCircle2 size={14} /> Accepted by Moderator
                        </span>
                      ) : (
                        <span
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: '4px',
                            color: 'var(--accent-amber)',
                            fontSize: '0.75rem',
                            fontWeight: 600,
                          }}
                        >
                          <Clock size={14} /> Outbox (Pending Delivery)
                        </span>
                      )}
                    </div>
                  </div>

                  <div style={{ fontSize: '0.85rem', color: 'var(--text-primary)', lineHeight: 1.4 }}>
                    {r.description}
                  </div>

                  {r.evidence_ref && (
                    <div
                      style={{
                        fontSize: '0.75rem',
                        color: 'var(--text-secondary)',
                        fontFamily: 'var(--font-mono)',
                        wordBreak: 'break-all',
                      }}
                    >
                      Reference: {r.evidence_ref}
                    </div>
                  )}

                  <div
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      fontSize: '0.72rem',
                      color: 'var(--text-muted)',
                      borderTop: '1px solid rgba(255, 255, 255, 0.05)',
                      paddingTop: '6px',
                    }}
                  >
                    <span>Event: {r.event_id.slice(0, 16)}...</span>
                    <span>Filed: {formatDate(r.created_at)}</span>
                  </div>
                </div>
              ))
            )}
          </div>
        )}

        {/* Tab 2: Blocked Content */}
        {activeTab === 'blocked' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', maxHeight: '420px', overflowY: 'auto' }}>
            {blockedItems.length === 0 ? (
              <div
                style={{
                  padding: '36px 20px',
                  textAlign: 'center',
                  color: 'var(--text-muted)',
                  fontSize: '0.85rem',
                  border: '1px dashed var(--border-glass)',
                  borderRadius: 'var(--radius-md)',
                }}
              >
                No content is currently restricted under active community moderation policy.
              </div>
            ) : (
              blockedItems.map((b) => (
                <div
                  key={b.content_id}
                  style={{
                    padding: '14px',
                    backgroundColor: 'rgba(244, 63, 94, 0.05)',
                    border: '1px solid rgba(244, 63, 94, 0.2)',
                    borderRadius: 'var(--radius-md)',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '8px',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <Badge variant="rose">BLOCKED</Badge>
                      <span
                        style={{
                          fontFamily: 'var(--font-mono)',
                          fontSize: '0.75rem',
                          color: 'var(--accent-rose)',
                          fontWeight: 500,
                        }}
                      >
                        {b.reason_code}
                      </span>
                    </div>
                    <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
                      Enforced: {formatDate(b.blocked_at)}
                    </span>
                  </div>

                  <div style={{ fontSize: '0.85rem', color: 'var(--text-primary)' }}>
                    {b.reason_text}
                  </div>

                  <div
                    style={{
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '2px',
                      fontSize: '0.72rem',
                      fontFamily: 'var(--font-mono)',
                      color: 'var(--text-muted)',
                      backgroundColor: 'rgba(0, 0, 0, 0.2)',
                      padding: '6px 10px',
                      borderRadius: 'var(--radius-sm)',
                    }}
                  >
                    <span>Content ID: {b.content_id}</span>
                    {b.payload_hash && <span>Payload Hash: {b.payload_hash}</span>}
                    <span>Decision ID: {b.decision_event_id.slice(0, 20)}...</span>
                  </div>
                </div>
              ))
            )}
          </div>
        )}

        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '4px' }}>
          <Button variant="secondary" size="md" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </Modal>
  );
};
