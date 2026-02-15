import { useAppState } from '../../state/store.js';
import { SessionMessageBar, StopSessionButton } from '../session/SessionControls.jsx';
import { normalizeStatus, formatElapsed } from '../../utils/format.js';
import { statusColor, STATUS_RUNNING } from '../../utils/colors.js';

export default function BottomBar() {
  var state = useAppState();
  var { sessions } = state;

  var runningSessions = sessions.filter(function (s) {
    return !!STATUS_RUNNING[normalizeStatus(s.status)];
  });

  return (
    <div style={{
      height: 200, flexShrink: 0, display: 'flex', flexDirection: 'column',
      borderTop: '1px solid var(--border)', background: 'var(--bg-1)',
    }}>
      <div style={{
        padding: '6px 12px', borderBottom: '1px solid var(--border)',
        display: 'flex', alignItems: 'center', gap: 8,
      }}>
        <span style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
          color: 'var(--text-1)',
        }}>Running Sessions</span>
        <span style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
          padding: '1px 5px', borderRadius: 3,
          background: runningSessions.length > 0 ? 'var(--green)20' : 'var(--bg-3)',
          color: runningSessions.length > 0 ? 'var(--green)' : 'var(--text-3)',
        }}>{runningSessions.length}</span>
      </div>

      <div style={{ flex: 1, overflow: 'auto' }}>
        {runningSessions.length === 0 ? (
          <div style={{
            padding: 24, textAlign: 'center', color: 'var(--text-3)',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
          }}>No running sessions</div>
        ) : (
          runningSessions.map(function (session) {
            var sColor = statusColor(session.status);
            return (
              <div key={session.id} style={{
                display: 'flex', alignItems: 'center', gap: 8,
                padding: '6px 12px', borderBottom: '1px solid var(--bg-3)',
              }}
              onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
              >
                <span style={{
                  width: 6, height: 6, borderRadius: '50%', background: sColor, flexShrink: 0,
                  boxShadow: '0 0 6px ' + sColor,
                  animation: 'pulse 2s ease-in-out infinite',
                }} />
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)',
                  flexShrink: 0,
                }}>#{session.id}</span>
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
                  color: 'var(--text-0)', flex: 1, overflow: 'hidden',
                  textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                }}>{session.profile || 'unknown'}</span>
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)',
                  flexShrink: 0,
                }}>({session.agent || 'agent'})</span>
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)',
                  flexShrink: 0,
                }}>{formatElapsed(session.started_at, session.ended_at)}</span>
                <StopSessionButton sessionID={session.id} />
              </div>
            );
          })
        )}
      </div>

      <SessionMessageBar />
    </div>
  );
}
