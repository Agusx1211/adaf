import { timeAgo, normalizeStatus } from '../../utils/format.js';

var typeColors = { ask: 'var(--orange)', question: 'var(--orange)', directive: 'var(--blue)', message: 'var(--text-2)', reply: 'var(--green)' };
var typeIcons = { ask: '?', question: '?', directive: '\u2192', message: '\u25C8', reply: '\u2713' };

export default function MessageBubble({ msg }) {
  var type = normalizeStatus(msg.type || 'message');
  if (type !== 'ask' && type !== 'reply' && type !== 'question' && type !== 'directive') type = 'message';
  var color = typeColors[type] || typeColors.message;
  var icon = typeIcons[type] || '\u25C8';
  var direction = normalizeStatus(msg.direction) === 'parent_to_child' ? '\u2193' : '\u2191';

  return (
    <div style={{ marginBottom: 8, animation: 'slideIn 0.2s ease-out' }}>
      <div style={{
        padding: '8px 10px',
        background: msg.direction === 'parent_to_child' ? 'var(--bg-3)' : 'var(--bg-2)',
        borderRadius: 6,
        borderLeft: '2px solid ' + color,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
          <span style={{
            width: 14, height: 14, borderRadius: 3,
            background: color + '20', color: color,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontSize: 9, fontWeight: 700, fontFamily: "'JetBrains Mono', monospace",
          }}>{icon}</span>
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: color,
            textTransform: 'uppercase', fontWeight: 600, letterSpacing: '0.05em',
          }}>{type}</span>
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
            {direction} {msg.spawn_id ? 'spawn #' + msg.spawn_id : ''}
          </span>
          <span style={{ marginLeft: 'auto', fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
            {timeAgo(msg.created_at)}
          </span>
        </div>
        <div style={{ fontSize: 12, color: 'var(--text-1)', lineHeight: 1.5 }}>{msg.content}</div>
      </div>
    </div>
  );
}
