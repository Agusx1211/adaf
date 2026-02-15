import { useAppState, useDispatch } from '../../state/store.js';
import { normalizeStatus } from '../../utils/format.js';

export default function LogsView() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { turns } = state;
  var selectedTurn = state.selectedTurn;

  if (!turns.length) {
    return <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>No turns recorded yet.</div>;
  }

  return (
    <div style={{ overflow: 'auto', flex: 1 }}>
      {turns.map(function (turn) {
        var status = normalizeStatus(turn.build_state || 'unknown');
        var border = status === 'passing' ? '#a6e3a1' : '#f38ba8';
        var selected = selectedTurn === turn.id;

        return (
          <div
            key={turn.id}
            onClick={function () { dispatch({ type: 'SET_SELECTED_TURN', payload: turn.id }); }}
            style={{
              padding: '10px 14px', borderBottom: '1px solid var(--border)',
              borderLeft: '3px solid ' + border, cursor: 'pointer',
              background: selected ? 'var(--bg-3)' : 'transparent', transition: 'background 0.15s ease',
            }}
            onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-2)'; }}
            onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
                #{turn.id} [{turn.hex_id || '-'}]
              </span>
              <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{turn.profile_name || '-'}</span>
              <span style={{ fontSize: 10, color: 'var(--text-3)' }}>({turn.agent || '-'})</span>
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 9, padding: '1px 5px',
                background: border + '22', borderRadius: 3, color: border,
              }}>{turn.build_state || 'unknown'}</span>
            </div>
            <div style={{ fontSize: 11, color: 'var(--text-1)', lineHeight: 1.4 }}>{turn.objective || 'No objective'}</div>
            {turn.what_was_built && <div style={{ fontSize: 10, color: 'var(--text-2)', marginTop: 2 }}>{turn.what_was_built}</div>}
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 4 }}>
              {turn.agent_model || 'model n/a'} {'\u00B7'} {turn.duration_secs || 0}s
            </div>
          </div>
        );
      })}
    </div>
  );
}
