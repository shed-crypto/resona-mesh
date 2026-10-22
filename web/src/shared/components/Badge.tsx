import React from 'react';

export type BadgeVariant = 'emerald' | 'cyan' | 'amber' | 'rose' | 'neutral';

interface BadgeProps {
  children: React.ReactNode;
  variant?: BadgeVariant;
  className?: string;
}

const variantStyles: Record<BadgeVariant, { bg: string; color: string; border: string }> = {
  emerald: {
    bg: 'rgba(16, 185, 129, 0.12)',
    color: '#10b981',
    border: '1px solid rgba(16, 185, 129, 0.3)',
  },
  cyan: {
    bg: 'rgba(6, 182, 212, 0.12)',
    color: '#06b6d4',
    border: '1px solid rgba(6, 182, 212, 0.3)',
  },
  amber: {
    bg: 'rgba(245, 158, 11, 0.12)',
    color: '#f59e0b',
    border: '1px solid rgba(245, 158, 11, 0.3)',
  },
  rose: {
    bg: 'rgba(244, 63, 94, 0.12)',
    color: '#f43f5e',
    border: '1px solid rgba(244, 63, 94, 0.3)',
  },
  neutral: {
    bg: 'rgba(255, 255, 255, 0.05)',
    color: '#9ca3af',
    border: '1px solid rgba(255, 255, 255, 0.1)',
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
