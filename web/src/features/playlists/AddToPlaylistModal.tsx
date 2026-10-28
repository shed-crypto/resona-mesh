import React, { useState } from 'react';
import { ListPlus, Plus, Check, X } from 'lucide-react';
import type { Playlist } from '../../types/playlist';
import type { Track } from '../../types/track';
import { Button } from '../../shared/components/Button';
import { Badge } from '../../shared/components/Badge';

interface AddToPlaylistModalProps {
  track: Track | null;
  isOpen: boolean;
  onClose: () => void;
  playlists: Playlist[];
  onAddToPlaylist: (playlistId: string, trackId: string) => void;
  onCreateAndAdd: (title: string, trackId: string) => void;
}

export const AddToPlaylistModal: React.FC<AddToPlaylistModalProps> = ({
  track,
  isOpen,
  onClose,
  playlists,
  onAddToPlaylist,
  onCreateAndAdd,
}) => {
  const [isCreating, setIsCreating] = useState(false);
  const [newTitle, setNewTitle] = useState('');
  const [addedId, setAddedId] = useState<string | null>(null);

  if (!isOpen || !track) return null;

  const handleAdd = (playlistId: string) => {
    onAddToPlaylist(playlistId, track.content_id);
    setAddedId(playlistId);
    setTimeout(() => {
      setAddedId(null);
      onClose();
    }, 600);
  };

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newTitle.trim()) return;
    onCreateAndAdd(newTitle.trim(), track.content_id);
    setNewTitle('');
    setIsCreating(false);
    onClose();
  };

  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        backdropFilter: 'blur(6px)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 250,
        padding: '20px',
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: '420px',
          backgroundColor: 'var(--bg-surface)',
          border: '1px solid var(--border-active)',
          borderRadius: 'var(--radius-lg)',
          boxShadow: '0 20px 48px rgba(0, 0, 0, 0.75)',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '16px 20px',
            borderBottom: '1px solid var(--border-glass)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <ListPlus size={18} style={{ color: 'var(--accent-emerald)' }} />
            <h3 style={{ fontSize: '1rem', fontWeight: 600 }}>Add to Playlist</h3>
          </div>
          <button
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              padding: '4px',
            }}
          >
            <X size={16} />
          </button>
        </div>

        <div style={{ padding: '12px 20px', borderBottom: '1px solid var(--border-glass)', backgroundColor: 'var(--bg-card)' }}>
          <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'var(--text-primary)' }}>
            {track.title}
          </div>
          <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
            {track.artist}
          </div>
        </div>

        <div
          style={{
            padding: '12px 20px',
            maxHeight: '260px',
            overflowY: 'auto',
            display: 'flex',
            flexDirection: 'column',
            gap: '6px',
          }}
        >
          {playlists.map((pl) => {
            const alreadyIn = pl.trackIds.includes(track.content_id);
            const isJustAdded = addedId === pl.id;

            return (
              <div
                key={pl.id}
                onClick={() => (!alreadyIn ? handleAdd(pl.id) : undefined)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '10px 14px',
                  borderRadius: 'var(--radius-md)',
                  backgroundColor: isJustAdded ? 'rgba(16, 185, 129, 0.15)' : 'var(--bg-card)',
                  border: isJustAdded ? '1px solid var(--accent-emerald)' : '1px solid var(--border-glass)',
                  cursor: alreadyIn ? 'default' : 'pointer',
                  opacity: alreadyIn ? 0.6 : 1,
                  transition: 'all 0.15s ease',
                }}
              >
                <div>
                  <div style={{ fontSize: '0.85rem', fontWeight: 500 }}>{pl.title}</div>
                  <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                    {pl.trackIds.length} tracks
                  </div>
                </div>

                {isJustAdded ? (
                  <Badge variant="emerald"><Check size={12} /> Added</Badge>
                ) : alreadyIn ? (
                  <Badge variant="neutral">In Playlist</Badge>
                ) : (
                  <Button variant="ghost" size="sm">Add</Button>
                )}
              </div>
            );
          })}
        </div>

        {/* Create new playlist inside modal */}
        <div style={{ padding: '14px 20px', borderTop: '1px solid var(--border-glass)', backgroundColor: 'var(--bg-canvas)' }}>
          {isCreating ? (
            <form onSubmit={handleCreateSubmit} style={{ display: 'flex', gap: '8px' }}>
              <input
                type="text"
                placeholder="New playlist name..."
                value={newTitle}
                onChange={(e) => setNewTitle(e.target.value)}
                autoFocus
                style={{
                  flex: 1,
                  padding: '6px 10px',
                  backgroundColor: 'var(--bg-surface)',
                  border: '1px solid var(--border-glass)',
                  borderRadius: 'var(--radius-sm)',
                  color: 'var(--text-primary)',
                  fontSize: '0.82rem',
                }}
              />
              <Button type="submit" variant="primary" size="sm">
                Create & Add
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setIsCreating(false)}>
                Cancel
              </Button>
            </form>
          ) : (
            <Button
              variant="secondary"
              size="sm"
              icon={<Plus size={14} />}
              onClick={() => setIsCreating(true)}
              style={{ width: '100%', justifyContent: 'center' }}
            >
              Create New Playlist
            </Button>
          )}
        </div>
      </div>
    </div>
  );
};
