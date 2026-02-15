import { useEffect, useCallback } from 'react';

export default function Modal({ title, children, onClose, maxWidth }) {
  var handleKeyDown = useCallback(function (e) {
    if (e.key === 'Escape') onClose();
  }, [onClose]);

  useEffect(function () {
    document.addEventListener('keydown', handleKeyDown);
    return function () { document.removeEventListener('keydown', handleKeyDown); };
  }, [handleKeyDown]);

  return (
    <>
      <div
        style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)',
          zIndex: 1000, animation: 'fadeIn 0.15s ease',
        }}
        onClick={onClose}
      />
      <section
        style={{
          position: 'fixed', top: '50%', left: '50%',
          transform: 'translate(-50%, -50%)',
          background: 'var(--bg-1)', border: '1px solid var(--border)',
          borderRadius: 8, minWidth: 400, maxWidth: maxWidth || 560, width: '90%',
          zIndex: 1001, animation: 'slideIn 0.2s ease-out',
          boxShadow: '0 20px 60px rgba(0,0,0,0.5)',
        }}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '12px 16px', borderBottom: '1px solid var(--border)',
        }}>
          <h3 style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-0)' }}>{title}</h3>
          <button
            onClick={onClose}
            style={{
              background: 'none', border: 'none', color: 'var(--text-3)',
              cursor: 'pointer', fontSize: 18, padding: '0 4px',
            }}
            aria-label="Close"
          >
            \u00D7
          </button>
        </div>
        <div style={{ padding: 16 }}>{children}</div>
      </section>
    </>
  );
}
