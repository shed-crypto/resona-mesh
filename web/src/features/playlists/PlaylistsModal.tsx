import React, { useState } from 'react';
import {
  ListMusic,
  Plus,
  Play,
  Trash2,
  Share2,
  Download,
  Check,
  X,
  Layers,
} from 'lucide-react';
import type { Playlist } from '../../types/playlist';
import type { Track } from '../../types/track';
import { Button } from '../../shared/components/Button';
import { Badge } from '../../shared/components/Badge';
import { generateCoverDataUri } from '../../shared/utils/artwork';
import { apiClient } from '../../shared/api/client';

interface PlaylistsModalProps {
  isOpen: boolean;
  onClose: () => void;
  playlists: Playlist[];
  allTracks: Track[];
  onCreatePlaylist: (title: string, description?: string) => Playlist;
  onDeletePlaylist: (id: string) => void;
  onRemoveTrack: (playlistId: string, trackId: string) => void;
  onExportPlaylist: (playlist: Playlist, allTracks: Track[]) => string;
  onImportPlaylist: (jsonStr: string) => Playlist | null;
  onPlayTracks: (tracks: Track[]) => void;
}

export const PlaylistsModal: React.FC<PlaylistsModalProps> = ({
  isOpen,
  onClose,
  playlists,
  allTracks,
  onCreatePlaylist,
  onDeletePlaylist,
  onRemoveTrack,
  onExportPlaylist,
  onImportPlaylist,
  onPlayTracks,
}) => {
  const [selectedPlaylistId, setSelectedPlaylistId] = useState<string>(playlists[0]?.id || 'favorites');
  const [isCreating, setIsCreating] = useState(false);
  const [newTitle, setNewTitle] = useState('');
  const [newDesc, setNewDesc] = useState('');
  const [importText, setImportText] = useState('');
  const [isImporting, setIsImporting] = useState(false);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  if (!isOpen) return null;

  const trackMap = new Map(allTracks.map((t) => [t.content_id, t]));
  const currentPlaylist = playlists.find((p) => p.id === selectedPlaylistId) || playlists[0];
  const playlistTracks = currentPlaylist
    ? currentPlaylist.trackIds.map((id) => trackMap.get(id)).filter((t): t is Track => t !== undefined)
    : [];

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newTitle.trim()) return;
    const created = onCreatePlaylist(newTitle, newDesc);
    setNewTitle('');
    setNewDesc('');
    setIsCreating(false);
    setSelectedPlaylistId(created.id);
  };

  const handleExport = (playlist: Playlist) => {
    const jsonStr = onExportPlaylist(playlist, allTracks);
    navigator.clipboard.writeText(jsonStr).then(() => {
      setCopiedId(playlist.id);
      setTimeout(() => setCopiedId(null), 2500);
    });
  };

  const handleImportSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!importText.trim()) return;
    const imported = onImportPlaylist(importText);
    if (imported) {
      setImportText('');
      setIsImporting(false);
      setSelectedPlaylistId(imported.id);
    } else {
      alert('Invalid playlist manifest JSON format. Please verify the copied text.');
    }
  };

  const handlePlayAll = () => {
    if (playlistTracks.length > 0) {
      onPlayTracks(playlistTracks);
      onClose();
    }
  };

  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.75)',
        backdropFilter: 'blur(8px)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 200,
        padding: '24px',
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: '860px',
          height: '620px',
          backgroundColor: 'var(--bg-surface)',
          border: '1px solid var(--border-active)',
          borderRadius: 'var(--radius-lg)',
          boxShadow: '0 24px 64px rgba(0, 0, 0, 0.8)',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        {/* Header */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '18px 24px',
            borderBottom: '1px solid var(--border-glass)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div
              style={{
                width: '32px',
                height: '32px',
                borderRadius: 'var(--radius-md)',
                backgroundColor: 'rgba(16, 185, 129, 0.12)',
                color: 'var(--accent-emerald)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <ListMusic size={18} />
            </div>
            <div>
              <h2 style={{ fontSize: '1.1rem', fontWeight: 600 }}>Swarm Playlists & Collections</h2>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Zero-telemetry local collections & P2P shareable manifests
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => setIsImporting((v) => !v)}
              icon={<Download size={14} />}
            >
              Import Link
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => setIsCreating((v) => !v)}
              icon={<Plus size={14} />}
            >
              New Playlist
            </Button>
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
              }}
            >
              <X size={18} />
            </button>
          </div>
        </div>

        {/* Create Playlist Form Drawer */}
        {isCreating && (
          <form
            onSubmit={handleCreate}
            style={{
              padding: '14px 24px',
              backgroundColor: 'rgba(16, 185, 129, 0.05)',
              borderBottom: '1px solid rgba(16, 185, 129, 0.2)',
              display: 'flex',
              gap: '12px',
              alignItems: 'center',
            }}
          >
            <input
              type="text"
              placeholder="Playlist Title (e.g. Synthwave Night Drive)..."
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              autoFocus
              style={{
                flex: 1,
                padding: '8px 12px',
                backgroundColor: 'var(--bg-card)',
                border: '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-primary)',
                fontSize: '0.85rem',
              }}
            />
            <input
              type="text"
              placeholder="Optional description..."
              value={newDesc}
              onChange={(e) => setNewDesc(e.target.value)}
              style={{
                flex: 1,
                padding: '8px 12px',
                backgroundColor: 'var(--bg-card)',
                border: '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-primary)',
                fontSize: '0.85rem',
              }}
            />
            <Button type="submit" variant="primary" size="sm">
              Save
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setIsCreating(false)}>
              Cancel
            </Button>
          </form>
        )}

        {/* Import Playlist Manifest Drawer */}
        {isImporting && (
          <form
            onSubmit={handleImportSubmit}
            style={{
              padding: '14px 24px',
              backgroundColor: 'rgba(6, 182, 212, 0.06)',
              borderBottom: '1px solid rgba(6, 182, 212, 0.2)',
              display: 'flex',
              flexDirection: 'column',
              gap: '8px',
            }}
          >
            <textarea
              rows={3}
              placeholder="Paste shared playlist JSON manifest from a peer..."
              value={importText}
              onChange={(e) => setImportText(e.target.value)}
              style={{
                padding: '8px 12px',
                backgroundColor: 'var(--bg-card)',
                border: '1px solid var(--border-glass)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-primary)',
                fontSize: '0.8rem',
                fontFamily: 'var(--font-mono)',
              }}
            />
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
              <Button type="submit" variant="primary" size="sm">
                Import Playlist
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setIsImporting(false)}>
                Cancel
              </Button>
            </div>
          </form>
        )}

        {/* Body: Left Sidebar (Playlists) + Right Detail (Tracks) */}
        <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
          {/* Sidebar */}
          <div
            style={{
              width: '260px',
              borderRight: '1px solid var(--border-glass)',
              overflowY: 'auto',
              padding: '12px',
              display: 'flex',
              flexDirection: 'column',
              gap: '4px',
              backgroundColor: 'var(--bg-canvas)',
            }}
          >
            {playlists.map((pl) => {
              const isSelected = pl.id === currentPlaylist?.id;
              return (
                <div
                  key={pl.id}
                  onClick={() => setSelectedPlaylistId(pl.id)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: '10px 12px',
                    borderRadius: 'var(--radius-md)',
                    backgroundColor: isSelected ? 'rgba(16, 185, 129, 0.12)' : 'transparent',
                    border: isSelected ? '1px solid rgba(16, 185, 129, 0.3)' : '1px solid transparent',
                    cursor: 'pointer',
                    transition: 'all 0.15s ease',
                  }}
                >
                  <div style={{ overflow: 'hidden' }}>
                    <div
                      style={{
                        fontSize: '0.85rem',
                        fontWeight: isSelected ? 600 : 500,
                        color: isSelected ? 'var(--accent-emerald)' : 'var(--text-primary)',
                        whiteSpace: 'nowrap',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                      }}
                    >
                      {pl.title}
                    </div>
                    <div style={{ fontSize: '0.72rem', color: 'var(--text-muted)' }}>
                      {pl.trackIds.length} {pl.trackIds.length === 1 ? 'track' : 'tracks'}
                    </div>
                  </div>
                  {isSelected && <Badge variant="emerald">Active</Badge>}
                </div>
              );
            })}
          </div>

          {/* Right Main Pane: Playlist Tracks */}
          <div
            style={{
              flex: 1,
              display: 'flex',
              flexDirection: 'column',
              overflow: 'hidden',
              backgroundColor: 'var(--bg-surface)',
            }}
          >
            {currentPlaylist ? (
              <>
                {/* Playlist Action Bar */}
                <div
                  style={{
                    padding: '16px 24px',
                    borderBottom: '1px solid var(--border-glass)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: '16px',
                  }}
                >
                  <div>
                    <h3 style={{ fontSize: '1.15rem', fontWeight: 600 }}>{currentPlaylist.title}</h3>
                    {currentPlaylist.description && (
                      <p style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginTop: '2px' }}>
                        {currentPlaylist.description}
                      </p>
                    )}
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <Button
                      variant="primary"
                      size="sm"
                      onClick={handlePlayAll}
                      disabled={playlistTracks.length === 0}
                      icon={<Play size={14} style={{ fill: 'currentColor' }} />}
                    >
                      Play All
                    </Button>
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => handleExport(currentPlaylist)}
                      icon={copiedId === currentPlaylist.id ? <Check size={14} /> : <Share2 size={14} />}
                    >
                      {copiedId === currentPlaylist.id ? 'Copied Manifest!' : 'Share'}
                    </Button>
                    {currentPlaylist.id !== 'favorites' && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          if (confirm(`Delete playlist "${currentPlaylist.title}"?`)) {
                            onDeletePlaylist(currentPlaylist.id);
                            setSelectedPlaylistId('favorites');
                          }
                        }}
                        icon={<Trash2 size={14} style={{ color: 'var(--accent-rose)' }} />}
                      >
                        Delete
                      </Button>
                    )}
                  </div>
                </div>

                {/* Track List */}
                <div
                  style={{
                    flex: 1,
                    overflowY: 'auto',
                    padding: '12px 24px',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '6px',
                  }}
                >
                  {playlistTracks.length === 0 ? (
                    <div
                      style={{
                        padding: '48px',
                        textAlign: 'center',
                        color: 'var(--text-muted)',
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        gap: '12px',
                      }}
                    >
                      <Layers size={36} opacity={0.4} />
                      <p style={{ fontSize: '0.9rem' }}>No tracks in this playlist yet.</p>
                      <p style={{ fontSize: '0.78rem' }}>
                        Click the "+" on any track card in the catalog to add it here.
                      </p>
                    </div>
                  ) : (
                    playlistTracks.map((track, idx) => {
                      const fallbackCover = generateCoverDataUri(track.content_id, track.title);
                      const cover = track.has_cover ? apiClient.getCoverUrl(track.content_id) : fallbackCover;
                      return (
                        <div
                          key={`${track.content_id}-${idx}`}
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                            padding: '10px 14px',
                            borderRadius: 'var(--radius-md)',
                            backgroundColor: 'var(--bg-card)',
                            border: '1px solid var(--border-glass)',
                            transition: 'border-color 0.15s ease',
                          }}
                        >
                          <div style={{ display: 'flex', alignItems: 'center', gap: '12px', overflow: 'hidden' }}>
                            <span style={{ fontSize: '0.8rem', color: 'var(--text-muted)', width: '20px' }}>
                              {idx + 1}
                            </span>
                            <img
                              src={cover}
                              alt=""
                              onError={(e) => {
                                e.currentTarget.src = fallbackCover;
                              }}
                              style={{ width: '36px', height: '36px', borderRadius: '4px', flexShrink: 0, objectFit: 'cover' }}
                            />
                            <div style={{ overflow: 'hidden' }}>
                              <div
                                style={{
                                  fontSize: '0.85rem',
                                  fontWeight: 600,
                                  whiteSpace: 'nowrap',
                                  overflow: 'hidden',
                                  textOverflow: 'ellipsis',
                                }}
                              >
                                {track.title}
                              </div>
                              <div
                                style={{
                                  fontSize: '0.75rem',
                                  color: 'var(--text-secondary)',
                                  whiteSpace: 'nowrap',
                                  overflow: 'hidden',
                                  textOverflow: 'ellipsis',
                                }}
                              >
                                {track.artist}
                              </div>
                            </div>
                          </div>

                          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                            <button
                              onClick={() => onPlayTracks([track, ...playlistTracks.filter((t) => t.content_id !== track.content_id)])}
                              style={{
                                background: 'none',
                                border: 'none',
                                color: 'var(--accent-emerald)',
                                cursor: 'pointer',
                                padding: '6px',
                              }}
                              title="Play Track"
                            >
                              <Play size={15} style={{ fill: 'currentColor' }} />
                            </button>
                            <button
                              onClick={() => onRemoveTrack(currentPlaylist.id, track.content_id)}
                              style={{
                                background: 'none',
                                border: 'none',
                                color: 'var(--text-muted)',
                                cursor: 'pointer',
                                padding: '6px',
                              }}
                              title="Remove from playlist"
                            >
                              <Trash2 size={15} />
                            </button>
                          </div>
                        </div>
                      );
                    })
                  )}
                </div>
              </>
            ) : (
              <div style={{ padding: '48px', textAlign: 'center', color: 'var(--text-muted)' }}>
                Select a playlist to view tracks
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
