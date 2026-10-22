import React, { useState } from 'react';
import { Flag, CheckCircle2, AlertCircle, Loader2 } from 'lucide-react';
import { Modal } from '../../shared/components/Modal';
import { Button } from '../../shared/components/Button';
import { apiClient } from '../../shared/api/client';
import type { Track } from '../../types/track';

interface ReportTrackModalProps {
  track: Track | null;
  isOpen: boolean;
  onClose: () => void;
  onSubmitted?: () => void;
}

export const ReportTrackModal: React.FC<ReportTrackModalProps> = ({
  track,
  isOpen,
  onClose,
  onSubmitted,
}) => {
  const [reasonCode, setReasonCode] = useState('COPYRIGHT_CLAIM');
  const [description, setDescription] = useState('');
  const [evidenceRef, setEvidenceRef] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const resetForm = () => {
    setReasonCode('COPYRIGHT_CLAIM');
    setDescription('');
    setEvidenceRef('');
    setLoading(false);
    setError(null);
    setSuccess(false);
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!track) return;

    if (!description.trim()) {
      setError('Please provide a brief description of the policy violation or claim.');
      return;
    }

    setLoading(true);
    setError(null);

    try {
      await apiClient.submitReport({
        content_id: track.content_id,
        reason_code: reasonCode,
        description: description.trim(),
        evidence_reference: evidenceRef.trim(),
      });
      setSuccess(true);
      if (onSubmitted) {
        onSubmitted();
      }
      setTimeout(() => {
        handleClose();
      }, 1500);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to submit report';
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  if (!track) return null;

  return (
    <Modal isOpen={isOpen} onClose={handleClose} title="Report Track / Policy Violation">
      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '18px' }}>
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

        {success && (
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
            <span>Report cryptographically signed and stored in outbox for moderator review.</span>
          </div>
        )}

        <div
          style={{
            padding: '12px 14px',
            backgroundColor: 'var(--bg-card)',
            border: '1px solid var(--border-glass)',
            borderRadius: 'var(--radius-md)',
            fontSize: '0.85rem',
          }}
        >
          <div style={{ color: 'var(--text-muted)', fontSize: '0.75rem', marginBottom: '2px' }}>
            Reported Content
          </div>
          <div style={{ fontWeight: 600, color: 'var(--text-primary)' }}>
            {track.title}
          </div>
          <div style={{ color: 'var(--text-secondary)', fontSize: '0.8rem' }}>
            {track.artist} • <span style={{ fontFamily: 'var(--font-mono)' }}>{track.content_id.slice(0, 16)}...</span>
          </div>
        </div>

        <div>
          <label
            style={{
              display: 'block',
              fontSize: '0.85rem',
              fontWeight: 500,
              marginBottom: '6px',
              color: 'var(--text-primary)',
            }}
          >
            Reason Code
          </label>
          <select
            value={reasonCode}
            onChange={(e) => setReasonCode(e.target.value)}
            disabled={loading || success}
            style={{
              width: '100%',
              padding: '10px 14px',
              backgroundColor: 'var(--bg-card)',
              border: '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-md)',
              color: 'var(--text-primary)',
              fontSize: '0.85rem',
              outline: 'none',
              boxSizing: 'border-box',
            }}
          >
            <option value="COPYRIGHT_CLAIM">Copyright Claim (Unauthorized Redistribution)</option>
            <option value="LICENSE_MISMATCH">License Mismatch (Incompatible Terms)</option>
            <option value="MISLEADING_METADATA">Misleading Metadata / Impersonation</option>
            <option value="OTHER">Other Policy Violation</option>
          </select>
        </div>

        <div>
          <label
            style={{
              display: 'block',
              fontSize: '0.85rem',
              fontWeight: 500,
              marginBottom: '6px',
              color: 'var(--text-primary)',
            }}
          >
            Statement / Details <span style={{ color: 'var(--accent-rose)' }}>*</span>
          </label>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Explain the reason for this report. Avoid sensitive personal information."
            rows={3}
            disabled={loading || success}
            style={{
              width: '100%',
              padding: '10px 14px',
              backgroundColor: 'var(--bg-card)',
              border: '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-md)',
              color: 'var(--text-primary)',
              fontSize: '0.85rem',
              outline: 'none',
              boxSizing: 'border-box',
              resize: 'vertical',
            }}
          />
        </div>

        <div>
          <label
            style={{
              display: 'block',
              fontSize: '0.85rem',
              fontWeight: 500,
              marginBottom: '6px',
              color: 'var(--text-primary)',
            }}
          >
            Evidence Reference / Link (Optional)
          </label>
          <input
            type="text"
            value={evidenceRef}
            onChange={(e) => setEvidenceRef(e.target.value)}
            placeholder="https://example.com/original-license-or-work"
            disabled={loading || success}
            style={{
              width: '100%',
              padding: '10px 14px',
              backgroundColor: 'var(--bg-card)',
              border: '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-md)',
              color: 'var(--text-primary)',
              fontFamily: 'var(--font-mono)',
              fontSize: '0.8rem',
              outline: 'none',
              boxSizing: 'border-box',
            }}
          />
          <p
            style={{
              fontSize: '0.75rem',
              color: 'var(--text-muted)',
              marginTop: '4px',
            }}
          >
            Links are handled as plain text references and will never be crawled automatically.
          </p>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '6px' }}>
          <Button variant="secondary" size="md" type="button" onClick={handleClose} disabled={loading}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="md"
            type="submit"
            disabled={loading || success}
            icon={loading ? <Loader2 size={16} className="spin" /> : <Flag size={16} />}
          >
            {loading ? 'Signing...' : success ? 'Submitted!' : 'Submit Report'}
          </Button>
        </div>
      </form>
    </Modal>
  );
};
