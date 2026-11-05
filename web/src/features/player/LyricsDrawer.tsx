import React, { useState, useEffect, useRef, useMemo } from 'react';
import { X, Edit2, Save, Mic2, AlertCircle } from 'lucide-react';
import type { Track } from '../../types/track';
import { parseLRC, findActiveLyricIndex } from '../../shared/utils/lrcParser';
import { apiClient } from '../../shared/api/client';

interface LyricsDrawerProps {
  currentTrack: Track;
  currentTime: number;
  isOpen: boolean;
  onClose: () => void;
  onSeek: (seconds: number) => void;
  onLyricsUpdated?: (newLyrics: string) => void;
}

export const LyricsDrawer: React.FC<LyricsDrawerProps> = ({
  currentTrack,
  currentTime,
  isOpen,
  onClose,
  onSeek,
  onLyricsUpdated,
}) => {
  const [activeLyrics, setActiveLyrics] = useState(currentTrack.lyrics || '');
  const [isEditing, setIsEditing] = useState(false);
  const [draftLyrics, setDraftLyrics] = useState(currentTrack.lyrics || '');
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const activeLineRef = useRef<HTMLDivElement | null>(null);
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);

  // Sync draft and active lyrics when track changes
  useEffect(() => {
    setActiveLyrics(currentTrack.lyrics || '');
    setDraftLyrics(currentTrack.lyrics || '');
    setIsEditing(false);
    setSaveError(null);
  }, [currentTrack.content_id, currentTrack.lyrics]);

  // Parse current lyrics into structured array
  const lyricLines = useMemo(() => {
    return parseLRC(activeLyrics);
  }, [activeLyrics]);

  // Find active line index based on current playback time
  const activeIndex = useMemo(() => {
    return findActiveLyricIndex(lyricLines, currentTime);
  }, [lyricLines, currentTime]);

  // Auto-scroll active lyric into center view
  useEffect(() => {
    if (!isOpen || isEditing || activeIndex < 0) return;
    if (activeLineRef.current && scrollContainerRef.current) {
      activeLineRef.current.scrollIntoView({
        behavior: 'smooth',
        block: 'center',
      });
    }
  }, [activeIndex, isOpen, isEditing]);

  const handleSaveLyrics = async () => {
    setIsSaving(true);
    setSaveError(null);
    try {
      await apiClient.updateTrackLyrics(currentTrack.content_id, draftLyrics);
      setActiveLyrics(draftLyrics);
      if (onLyricsUpdated) {
        onLyricsUpdated(draftLyrics);
      }
      setIsEditing(false);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to save lyrics';
      setSaveError(msg);
    } finally {
      setIsSaving(false);
    }
  };

  if (!isOpen) {
    return null;
  }

  return (
    <div
      style={{
        position: 'fixed',
        bottom: '96px',
        right: '24px',
        width: '440px',
        maxWidth: 'calc(100vw - 48px)',
        height: '520px',
        maxHeight: 'calc(100vh - 120px)',
        backgroundColor: 'rgba(26, 26, 29, 0.98)',
        backdropFilter: 'blur(24px)',
        border: '1px solid var(--border-active)',
        borderRadius: 'var(--radius-lg)',
        boxShadow: '0 20px 50px rgba(0, 0, 0, 0.8)',
        display: 'flex',
        flexDirection: 'column',
        zIndex: 110,
        overflow: 'hidden',
      }}
    >
      {/* Header */}
      <div
        style={{
          padding: '16px 20px',
          borderBottom: '1px solid var(--border-glass)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          backgroundColor: 'rgba(11, 13, 16, 0.6)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px', minWidth: 0 }}>
          <div
            style={{
              width: '32px',
              height: '32px',
              borderRadius: 'var(--radius-md)',
              backgroundColor: 'rgba(249, 115, 22, 0.15)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--accent-primary)',
              flexShrink: 0,
            }}
          >
            <Mic2 size={16} />
          </div>
          <div style={{ minWidth: 0 }}>
            <h3
              style={{
                margin: 0,
                fontSize: '0.92rem',
                fontWeight: 600,
                color: 'var(--text-primary)',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              Synchronized Lyrics
            </h3>
            <p
              style={{
                margin: 0,
                fontSize: '0.74rem',
                color: 'var(--text-muted)',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {currentTrack.title} &bull; {currentTrack.artist}
            </p>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <button
            onClick={() => {
              setIsEditing((prev) => !prev);
              setSaveError(null);
            }}
            style={{
              background: isEditing ? 'rgba(249, 115, 22, 0.15)' : 'none',
              border: isEditing ? '1px solid var(--accent-emerald)' : '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-sm)',
              color: isEditing ? 'var(--accent-primary)' : 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px 10px',
              fontSize: '0.75rem',
              display: 'flex',
              alignItems: 'center',
              gap: '5px',
              transition: 'all 0.15s ease',
            }}
            title={isEditing ? 'Cancel Editing' : 'Edit or Paste LRC Lyrics'}
          >
            <Edit2 size={13} />
            <span>{isEditing ? 'Cancel' : 'Edit .LRC'}</span>
          </button>

          <button
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              borderRadius: 'var(--radius-sm)',
              transition: 'color 0.15s ease',
            }}
            title="Close Lyrics Drawer"
          >
            <X size={16} />
          </button>
        </div>
      </div>

      {/* Body: Viewer or Editor */}
      {isEditing ? (
        <div
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            padding: '16px 20px',
            gap: '12px',
            overflow: 'hidden',
          }}
        >
          <div style={{ fontSize: '0.76rem', color: 'var(--text-muted)' }}>
            Paste standard LRC formatted lyrics with timestamps e.g. <code>[00:15.50] Lyric line</code>.
          </div>

          <textarea
            value={draftLyrics}
            onChange={(e) => setDraftLyrics(e.target.value)}
            placeholder="[00:01.00] Resona Mesh intro&#10;[00:08.50] Synchronized P2P audio streaming"
            style={{
              flex: 1,
              width: '100%',
              backgroundColor: 'rgba(11, 13, 16, 0.85)',
              border: '1px solid var(--border-glass)',
              borderRadius: 'var(--radius-md)',
              color: 'var(--text-primary)',
              fontFamily: 'monospace',
              fontSize: '0.82rem',
              padding: '12px',
              resize: 'none',
              outline: 'none',
              boxSizing: 'border-box',
            }}
          />

          {saveError && (
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                color: 'var(--accent-rose, #ef4444)',
                fontSize: '0.76rem',
              }}
            >
              <AlertCircle size={14} />
              <span>{saveError}</span>
            </div>
          )}

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
            <button
              onClick={() => setIsEditing(false)}
              disabled={isSaving}
              style={{
                background: 'none',
                border: '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-muted)',
                padding: '8px 16px',
                fontSize: '0.8rem',
                cursor: 'pointer',
              }}
            >
              Cancel
            </button>
            <button
              onClick={handleSaveLyrics}
              disabled={isSaving}
              style={{
                backgroundColor: 'var(--accent-primary)',
                border: 'none',
                borderRadius: 'var(--radius-sm)',
                color: '#000',
                fontWeight: 600,
                padding: '8px 18px',
                fontSize: '0.8rem',
                cursor: isSaving ? 'wait' : 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
              }}
            >
              <Save size={14} />
              <span>{isSaving ? 'Saving...' : 'Save Lyrics'}</span>
            </button>
          </div>
        </div>
      ) : lyricLines.length === 0 ? (
        /* Empty State */
        <div
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '32px 24px',
            textAlign: 'center',
            gap: '14px',
          }}
        >
          <div
            style={{
              width: '52px',
              height: '52px',
              borderRadius: 'var(--radius-full)',
              backgroundColor: 'rgba(255, 255, 255, 0.04)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--text-muted)',
            }}
          >
            <Mic2 size={24} />
          </div>
          <div>
            <div style={{ fontSize: '0.92rem', fontWeight: 600, color: 'var(--text-primary)' }}>
              No Synchronized Lyrics Found
            </div>
            <div style={{ fontSize: '0.78rem', color: 'var(--text-muted)', marginTop: '4px', maxWidth: '300px' }}>
              Add timed LRC lyrics to enable real-time karaoke tracking and click-to-seek playback.
            </div>
          </div>
          <button
            onClick={() => setIsEditing(true)}
            style={{
              backgroundColor: 'rgba(249, 115, 22, 0.15)',
              border: '1px solid var(--accent-emerald)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--accent-primary)',
              fontWeight: 500,
              padding: '8px 16px',
              fontSize: '0.8rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              marginTop: '4px',
            }}
          >
            <Edit2 size={13} />
            <span>Add .LRC Lyrics</span>
          </button>
        </div>
      ) : (
        /* Synchronized Karaoke View */
        <div
          ref={scrollContainerRef}
          style={{
            flex: 1,
            overflowY: 'auto',
            padding: '32px 20px',
            display: 'flex',
            flexDirection: 'column',
            gap: '16px',
            scrollBehavior: 'smooth',
          }}
        >
          {lyricLines.map((line, idx) => {
            const isActive = idx === activeIndex;
            const isPast = idx < activeIndex;

            return (
              <div
                key={`${idx}-${line.timeSec}`}
                ref={isActive ? activeLineRef : null}
                onClick={() => onSeek(line.timeSec)}
                style={{
                  padding: '8px 12px',
                  borderRadius: 'var(--radius-md)',
                  cursor: 'pointer',
                  transition: 'all 0.25s cubic-bezier(0.16, 1, 0.3, 1)',
                  backgroundColor: isActive ? 'rgba(249, 115, 22, 0.12)' : 'transparent',
                  borderLeft: isActive ? '3px solid var(--accent-emerald)' : '3px solid transparent',
                  color: isActive
                    ? 'var(--accent-primary)'
                    : isPast
                    ? 'rgba(255, 255, 255, 0.4)'
                    : 'rgba(255, 255, 255, 0.85)',
                  fontSize: isActive ? '1.08rem' : '0.88rem',
                  fontWeight: isActive ? 600 : 400,
                  textShadow: 'none',
                  transform: isActive ? 'scale(1.02)' : 'none',
                  userSelect: 'none',
                }}
                onMouseEnter={(e) => {
                  if (!isActive) {
                    e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.04)';
                    e.currentTarget.style.color = 'var(--text-primary)';
                  }
                }}
                onMouseLeave={(e) => {
                  if (!isActive) {
                    e.currentTarget.style.backgroundColor = 'transparent';
                    e.currentTarget.style.color = isPast
                      ? 'rgba(255, 255, 255, 0.4)'
                      : 'rgba(255, 255, 255, 0.85)';
                  }
                }}
                title={`Click to seek to ${line.text}`}
              >
                {line.text || '♪'}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
