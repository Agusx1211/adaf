import { useState, useEffect, useCallback, createContext, useContext } from 'react';

var ToastContext = createContext(null);

export function ToastProvider({ children }) {
  var [toasts, setToasts] = useState([]);

  var showToast = useCallback(function (message, type) {
    var id = Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
    setToasts(function (prev) { return prev.concat([{ id, message, type: type || 'success' }]); });
    setTimeout(function () {
      setToasts(function (prev) { return prev.filter(function (t) { return t.id !== id; }); });
    }, 4200);
  }, []);

  return (
    <ToastContext.Provider value={showToast}>
      {children}
      <div style={{ position: 'fixed', bottom: 16, right: 16, zIndex: 2000, display: 'flex', flexDirection: 'column', gap: 8 }}>
        {toasts.map(function (toast) {
          return (
            <div key={toast.id} style={{
              padding: '8px 16px',
              background: toast.type === 'error' ? 'var(--red)' : 'var(--green)',
              color: '#fff',
              borderRadius: 6,
              fontSize: 12,
              fontFamily: "'JetBrains Mono', monospace",
              animation: 'slideIn 0.2s ease-out',
              boxShadow: '0 4px 12px rgba(0,0,0,0.3)',
            }}>
              {toast.message}
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  var ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within ToastProvider');
  return ctx;
}
