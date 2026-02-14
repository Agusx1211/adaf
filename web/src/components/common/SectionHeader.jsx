export default function SectionHeader({ children, count, action }) {
  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      padding: '10px 14px',
      borderBottom: '1px solid var(--border)',
      background: 'var(--bg-2)',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-1)', letterSpacing: '0.06em', textTransform: 'uppercase' }}>
          {children}
        </span>
        {count !== undefined && (
          <span style={{
            background: 'var(--bg-4)',
            color: 'var(--text-2)',
            fontFamily: "'JetBrains Mono', monospace",
            fontSize: 10,
            padding: '1px 6px',
            borderRadius: 3,
            fontWeight: 500,
          }}>
            {count}
          </span>
        )}
      </div>
      {action}
    </div>
  );
}
