import React from 'react';

export type BadgeVariant = 'emerald' | 'cyan' | 'amber' | 'rose' | 'neutral' | 'primary';

interface BadgeProps {
  children: React.ReactNode;
  variant?: BadgeVariant;
  className?: string;
}

const variantStyles: Record<BadgeVariant, { bg: string; color: string; border: string }> = {
  primary: {
    bg: 'rgba(249, 115, 22, 0.12)',
    color: '#f97316',
    border: '1px solid rgba(249, 115, 22, 0.25)',
  },
  emerald: {
    bg: 'rgba(34, 197, 94, 0.12)',
    color: '#22c55e',
    border: '1px solid rgba(34, 197, 94, 0.25)',
  },
  cyan: {
    bg: 'rgba(56, 189, 248, 0.12)',
    color: '#38bdf8',
    border: '1px solid rgba(56, 189, 248, 0.25)',
  },
  amber: {
    bg: 'rgba(245, 158, 11, 0.12)',
    color: '#f59e0b',
    border: '1px solid rgba(245, 158, 11, 0.25)',
  },
  rose: {
    bg: 'rgba(244, 63, 94, 0.12)',
    color: '#f43f5e',
    border: '1px solid rgba(244, 63, 94, 0.25)',
  },
  neutral: {
    bg: 'rgba(255, 255, 255, 0.05)',
    color: '#a1a1aa',
    border: '1px solid rgba(255, 255, 255, 0.08)',
  },
};

export const Badge: React.FC<BadgeProps> = ({ children, variant = 'neutral', className = '' }) => {
  const style = variantStyles[variant];

  return (
    <span
      className={className}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '4px',
        padding: '2px 8px',
        borderRadius: 'var(--radius-full)',
        fontSize: '0.72rem',
        fontWeight: 500,
        fontFamily: 'var(--font-mono)',
        backgroundColor: style.bg,
        color: style.color,
        border: style.border,
        whiteSpace: 'nowrap',
      }}
    >
      {children}
    </span>
  );
};
