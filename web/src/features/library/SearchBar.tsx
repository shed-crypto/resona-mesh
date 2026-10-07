import React from 'react';
import { Search, Pin, X } from 'lucide-react';

interface SearchBarProps {
  search: string;
  onSearchChange: (val: string) => void;
  pinnedOnly: boolean;
  onPinnedOnlyChange: (val: boolean) => void;
}

export const SearchBar: React.FC<SearchBarProps> = ({
  search,
  onSearchChange,
  pinnedOnly,
  onPinnedOnlyChange,
}) => {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        width: '100%',
        maxWidth: '720px',
      }}
    >
      <div
        style={{
          position: 'relative',
          flex: 1,
          display: 'flex',
          alignItems: 'center',
        }}
      >
        <Search
          size={16}
          style={{
            position: 'absolute',
            left: '12px',
            color: 'var(--text-muted)',
            pointerEvents: 'none',
          }}
        />
        <input
          type="text"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="Search tracks, artists, metadata (FTS5 Ukrainian supported)..."
          style={{
            width: '100%',
            padding: '10px 36px 10px 38px',
            backgroundColor: 'var(--bg-card)',
            color: 'var(--text-primary)',
            border: '1px solid var(--border-glass)',
            borderRadius: 'var(--radius-md)',
            fontSize: '0.875rem',
            outline: 'none',
            transition: 'border-color 0.15s ease',
          }}
        />
        {search && (
          <button
            onClick={() => onSearchChange('')}
            style={{
              position: 'absolute',
              right: '10px',
              color: 'var(--text-muted)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              padding: '2px',
            }}
          >
            <X size={14} />
          </button>
        )}
      </div>

      <button
        onClick={() => onPinnedOnlyChange(!pinnedOnly)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          padding: '10px 14px',
          backgroundColor: pinnedOnly ? 'rgba(16, 185, 129, 0.15)' : 'var(--bg-card)',
          color: pinnedOnly ? 'var(--accent-emerald)' : 'var(--text-secondary)',
          border: pinnedOnly ? '1px solid rgba(16, 185, 129, 0.3)' : '1px solid var(--border-glass)',
          borderRadius: 'var(--radius-md)',
          fontSize: '0.875rem',
          fontWeight: 500,
          whiteSpace: 'nowrap',
          transition: 'all 0.15s ease',
        }}
      >
        <Pin size={15} style={{ fill: pinnedOnly ? 'currentColor' : 'none' }} />
        Pinned Only
      </button>
    </div>
  );
};
