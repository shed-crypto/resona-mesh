import React from 'react';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ButtonSize = 'sm' | 'md' | 'lg';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  icon?: React.ReactNode;
  loading?: boolean;
}

export const Button: React.FC<ButtonProps> = ({
  children,
  variant = 'secondary',
  size = 'md',
  icon,
  loading = false,
  disabled,
  style = {},
  ...props
}) => {
  const getPadding = () => {
    switch (size) {
      case 'sm':
        return '6px 12px';
      case 'lg':
        return '12px 24px';
      default:
        return '8px 16px';
    }
  };

  const getFontSize = () => {
    switch (size) {
      case 'sm':
        return '0.8rem';
      case 'lg':
        return '1rem';
      default:
        return '0.875rem';
    }
  };

  const getVariantStyles = (): React.CSSProperties => {
    switch (variant) {
      case 'primary':
        return {
          backgroundColor: 'var(--accent-primary, #f97316)',
          color: '#ffffff',
          fontWeight: 600,
          boxShadow: '0 1px 3px rgba(0, 0, 0, 0.25)',
        };
      case 'ghost':
        return {
          backgroundColor: 'transparent',
          color: 'var(--text-secondary)',
          border: '1px solid transparent',
        };
      case 'danger':
        return {
          backgroundColor: 'rgba(244, 63, 94, 0.12)',
          color: 'var(--accent-rose)',
          border: '1px solid rgba(244, 63, 94, 0.25)',
        };
      case 'secondary':
      default:
        return {
          backgroundColor: 'var(--bg-card)',
          color: 'var(--text-primary)',
          border: '1px solid var(--border-glass)',
        };
    }
  };

  return (
    <button
      disabled={disabled || loading}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '8px',
        padding: getPadding(),
        fontSize: getFontSize(),
        borderRadius: 'var(--radius-md)',
        transition: 'all 0.15s ease',
        opacity: disabled || loading ? 0.5 : 1,
        cursor: disabled || loading ? 'not-allowed' : 'pointer',
        ...getVariantStyles(),
        ...style,
      }}
      {...props}
    >
      {loading ? (
        <span style={{ display: 'inline-block', animation: 'spin 1s linear infinite' }}>...</span>
      ) : (
        icon
      )}
      {children}
    </button>
  );
};
