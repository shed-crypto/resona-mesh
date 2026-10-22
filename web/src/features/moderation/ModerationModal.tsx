import React, { useState, useEffect, useCallback } from 'react';
import { RefreshCw, FileText, Ban, CheckCircle2, Clock, ShieldAlert, ShieldCheck, Scale, Send, Check } from 'lucide-react';
import { Modal } from '../../shared/components/Modal';
import { Button } from '../../shared/components/Button';
import { Badge } from '../../shared/components/Badge';
import { apiClient } from '../../shared/api/client';
import { useNodeStatus } from '../../shared/hooks/useNodeStatus';
import type { Report, BlockedItem } from '../../types/track';

interface ModerationModalProps {
  isOpen: boolean;
  onClose: () => void;
  onDecisionApplied?: () => void;
}

const REASON_CODES = [
  { value: 'COPYRIGHT_CLAIM', label: 'Copyright & License Claim (DMCA / Infringement)' },
  { value: 'NSFW_HARMFUL', label: 'NSFW / Harmful or Illicit Audio Content' },
  { value: 'MALWARE', label: 'Malicious Payload / Archive Bomb' },
  { value: 'LICENSE_MISMATCH', label: 'Incompatible License / Attribute Violation' },
  { value: 'COMMUNITY_POLICY', label: 'General Community Guideline Violation' },
];

export const ModerationModal: React.FC<ModerationModalProps> = ({ isOpen, onClose, onDecisionApplied }) => {
  const [activeTab, setActiveTab] = useState<'reports' | 'blocked' | 'admin'>('reports');
  const [reports, setReports] = useState<Report[]>([]);
  const [blockedItems, setBlockedItems] = useState<BlockedItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Admin Action Form State
  const [adminTargetId, setAdminTargetId] = useState('');
  const [adminAction, setAdminAction] = useState<'BLOCK' | 'RESTORE'>('BLOCK');
  const [adminReasonCode, setAdminReasonCode] = useState('COPYRIGHT_CLAIM');
  const [adminReasonNotes, setAdminReasonNotes] = useState('');
  const [adminExecuting, setAdminExecuting] = useState(false);
  const [adminSuccess, setAdminSuccess] = useState<string | null>(null);

  const { status: nodeStatus, refresh: refreshNodeStatus } = useNodeStatus();

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
      refreshNodeStatus();
    }
  }, [isOpen, fetchData, refreshNodeStatus]);

  const handleActOnReport = (r: Report) => {
    setAdminTargetId(r.content_id);
    setAdminAction('BLOCK');
    setAdminReasonCode(r.reason_code || 'COPYRIGHT_CLAIM');
    setAdminReasonNotes(`Enforcing filed report #${r.event_id.slice(0, 12)}: ${r.description}`);
    setActiveTab('admin');
  };

  const handleExecuteDecision = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!adminTargetId.trim()) {
      setError('Content ID is required');
      return;
    }

    setAdminExecuting(true);
    setError(null);
    setAdminSuccess(null);

    try {
      const res = await apiClient.createDecision({
        content_id: adminTargetId.trim(),
        action: adminAction,
        reason_code: adminReasonCode,
        reason_text: adminReasonNotes.trim() || `Decision applied by moderator`,
      });

      setAdminSuccess(`Cryptographic decision ${res.decision?.event_id ? `#${res.decision.event_id.slice(0, 16)}` : ''} applied successfully`);
      setAdminTargetId('');
      setAdminReasonNotes('');
      await fetchData();
      onDecisionApplied?.();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to apply decision';
      setError(msg);
    } finally {
      setAdminExecuting(false);
    }
  };

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

            <button
              onClick={() => setActiveTab('admin')}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                padding: '8px 14px',
                borderRadius: 'var(--radius-md)',
                backgroundColor: activeTab === 'admin' ? 'var(--bg-card)' : 'transparent',
                color: activeTab === 'admin' ? 'var(--accent-amber)' : 'var(--text-secondary)',
                border: activeTab === 'admin' ? '1px solid var(--border-glass)' : '1px solid transparent',
                fontSize: '0.85rem',
                fontWeight: activeTab === 'admin' ? 600 : 400,
                cursor: 'pointer',
              }}
            >
              <Scale size={16} />
              <span>Admin Actions</span>
              {nodeStatus?.is_moderator && (
                <ShieldCheck size={14} style={{ color: 'var(--accent-emerald)' }} />
              )}
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

        {adminSuccess && (
          <div
            style={{
              padding: '10px 14px',
              backgroundColor: 'rgba(16, 185, 129, 0.1)',
              border: '1px solid rgba(16, 185, 129, 0.3)',
              borderRadius: 'var(--radius-md)',
              color: 'var(--accent-emerald)',
              fontSize: '0.82rem',
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
            }}
          >
            <Check size={16} />
            <span>{adminSuccess}</span>
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

                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
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
                          <Clock size={14} /> Pending Delivery
                        </span>
                      )}

                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => handleActOnReport(r)}
                      >
                        Resolve
                      </Button>
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

                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
                        Enforced: {formatDate(b.blocked_at)}
                      </span>
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => {
                          setAdminTargetId(b.content_id);
                          setAdminAction('RESTORE');
                          setAdminReasonCode(b.reason_code);
                          setAdminReasonNotes(`Restoring content after review`);
                          setActiveTab('admin');
                        }}
                      >
                        Unblock
                      </Button>
                    </div>
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

        {/* Tab 3: Admin Actions / Decision Console */}
        {activeTab === 'admin' && (
          <form onSubmit={handleExecuteDecision} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                padding: '12px 14px',
                backgroundColor: nodeStatus?.is_moderator ? 'rgba(16, 185, 129, 0.08)' : 'rgba(245, 158, 11, 0.08)',
                border: nodeStatus?.is_moderator ? '1px solid rgba(16, 185, 129, 0.25)' : '1px solid rgba(245, 158, 11, 0.25)',
                borderRadius: 'var(--radius-md)',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                {nodeStatus?.is_moderator ? (
                  <ShieldCheck size={20} style={{ color: 'var(--accent-emerald)', flexShrink: 0 }} />
                ) : (
                  <ShieldAlert size={20} style={{ color: 'var(--accent-amber)', flexShrink: 0 }} />
                )}
                <div>
                  <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    {nodeStatus?.is_moderator ? 'Authorized Moderator Node' : 'Self-Sovereign Node Authority'}
                  </div>
                  <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                    Decisions are cryptographically signed with your Ed25519 identity and broadcast over the P2P wire.
                  </div>
                </div>
              </div>

              <Badge variant={nodeStatus?.is_moderator ? 'emerald' : 'amber'}>
                {nodeStatus?.is_moderator ? 'ROOT TRUST' : 'LOCAL TRUST'}
              </Badge>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
              <label style={{ fontSize: '0.82rem', fontWeight: 600, color: 'var(--text-secondary)' }}>
                Target Content ID (SHA-256 Hash):
              </label>
              <input
                type="text"
                value={adminTargetId}
                onChange={(e) => setAdminTargetId(e.target.value)}
                placeholder="e.g. 5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8"
                required
                style={{
                  width: '100%',
                  padding: '9px 12px',
                  backgroundColor: 'rgba(0, 0, 0, 0.3)',
                  border: '1px solid var(--border-glass)',
                  borderRadius: 'var(--radius-md)',
                  color: 'var(--text-primary)',
                  fontSize: '0.85rem',
                  fontFamily: 'var(--font-mono)',
                  outline: 'none',
                }}
              />
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                <label style={{ fontSize: '0.82rem', fontWeight: 600, color: 'var(--text-secondary)' }}>
                  Moderation Action:
                </label>
                <div style={{ display: 'flex', gap: '8px' }}>
                  <button
                    type="button"
                    onClick={() => setAdminAction('BLOCK')}
                    style={{
                      flex: 1,
                      padding: '8px',
                      borderRadius: 'var(--radius-md)',
                      backgroundColor: adminAction === 'BLOCK' ? 'rgba(244, 63, 94, 0.2)' : 'rgba(255, 255, 255, 0.04)',
                      color: adminAction === 'BLOCK' ? 'var(--accent-rose)' : 'var(--text-muted)',
                      border: adminAction === 'BLOCK' ? '1px solid var(--accent-rose)' : '1px solid transparent',
                      fontWeight: 600,
                      fontSize: '0.82rem',
                      cursor: 'pointer',
                    }}
                  >
                    BLOCK CONTENT
                  </button>
                  <button
                    type="button"
                    onClick={() => setAdminAction('RESTORE')}
                    style={{
                      flex: 1,
                      padding: '8px',
                      borderRadius: 'var(--radius-md)',
                      backgroundColor: adminAction === 'RESTORE' ? 'rgba(16, 185, 129, 0.2)' : 'rgba(255, 255, 255, 0.04)',
                      color: adminAction === 'RESTORE' ? 'var(--accent-emerald)' : 'var(--text-muted)',
                      border: adminAction === 'RESTORE' ? '1px solid var(--accent-emerald)' : '1px solid transparent',
                      fontWeight: 600,
                      fontSize: '0.82rem',
                      cursor: 'pointer',
                    }}
                  >
                    RESTORE CONTENT
                  </button>
                </div>
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                <label style={{ fontSize: '0.82rem', fontWeight: 600, color: 'var(--text-secondary)' }}>
                  Reason Category:
                </label>
                <select
                  value={adminReasonCode}
                  onChange={(e) => setAdminReasonCode(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '9px 12px',
                    backgroundColor: 'rgba(0, 0, 0, 0.3)',
                    border: '1px solid var(--border-glass)',
                    borderRadius: 'var(--radius-md)',
                    color: 'var(--text-primary)',
                    fontSize: '0.85rem',
                    outline: 'none',
                    cursor: 'pointer',
                  }}
                >
                  {REASON_CODES.map((rc) => (
                    <option key={rc.value} value={rc.value} style={{ backgroundColor: '#14171d' }}>
                      {rc.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
              <label style={{ fontSize: '0.82rem', fontWeight: 600, color: 'var(--text-secondary)' }}>
                Decision Reason & Legal Justification:
              </label>
              <textarea
                value={adminReasonNotes}
                onChange={(e) => setAdminReasonNotes(e.target.value)}
                placeholder="Enter justification, copyright citation, or compliance notes..."
                rows={3}
                style={{
                  width: '100%',
                  padding: '9px 12px',
                  backgroundColor: 'rgba(0, 0, 0, 0.3)',
                  border: '1px solid var(--border-glass)',
                  borderRadius: 'var(--radius-md)',
                  color: 'var(--text-primary)',
                  fontSize: '0.85rem',
                  outline: 'none',
                  resize: 'vertical',
                }}
              />
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '6px' }}>
              <Button
                type="submit"
                variant={adminAction === 'BLOCK' ? 'danger' : 'primary'}
                size="md"
                disabled={adminExecuting || !adminTargetId.trim()}
                icon={<Send size={15} />}
              >
                {adminExecuting ? 'Signing & Broadcasting...' : `Sign & Execute ${adminAction}`}
              </Button>
            </div>
          </form>
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
