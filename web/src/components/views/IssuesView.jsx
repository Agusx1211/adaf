import { useAppState, useDispatch } from '../../state/store.js';
import { normalizeStatus } from '../../utils/format.js';
import { statusColor, statusIcon } from '../../utils/colors.js';
import { withAlpha } from '../../utils/format.js';

export default function IssuesView() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { issues, selectedIssue } = state;

  if (!issues.length) {
    return <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>No issues found.</div>;
  }

  return (
    <div style={{ overflow: 'auto', flex: 1 }}>
      {issues.map(function (issue) {
        var selected = selectedIssue === issue.id;
        return (
          <div
            key={issue.id}
            onClick={function () { dispatch({ type: 'SET_SELECTED_ISSUE', payload: issue.id }); }}
            style={{
              padding: '10px 14px', borderBottom: '1px solid var(--border)', cursor: 'pointer',
              background: selected ? 'var(--bg-3)' : 'transparent',
              transition: 'background 0.15s ease',
            }}
            onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-2)'; }}
            onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>#{issue.id}</span>
              <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-0)', flex: 1 }}>{issue.title || 'Untitled issue'}</span>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <StatusBadge status={issue.priority} />
              <StatusBadge status={issue.status} />
              {(issue.labels || []).map(function (label) {
                return <span key={label} style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-2)', padding: '1px 5px', background: 'var(--bg-4)', borderRadius: 3 }}>{label}</span>;
              })}
            </div>
            {selected && issue.description && (
              <div style={{ marginTop: 8, fontSize: 11, color: 'var(--text-2)', lineHeight: 1.5 }}>{issue.description}</div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function StatusBadge({ status }) {
  var color = statusColor(status);
  var icon = statusIcon(status);
  var normalized = normalizeStatus(status);
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 3,
      padding: '1px 6px', borderRadius: 3,
      background: withAlpha(color, 0.14), border: '1px solid ' + withAlpha(color, 0.28),
      fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: color,
    }}>
      <span>{icon}</span>
      <span>{normalized}</span>
    </span>
  );
}
