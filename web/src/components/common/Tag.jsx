export default function Tag({ children, color = 'var(--text-2)', bg }) {
  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      padding: '1px 6px',
      background: bg || (color + '11'),
      border: '1px solid ' + color + '22',
      borderRadius: 3,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 10,
      color: color,
      letterSpacing: '0.03em',
    }}>
      {children}
    </span>
  );
}
