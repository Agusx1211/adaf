import { useAppState } from '../../state/store.js';
import { normalizeStatus } from '../../utils/format.js';

export default function ProjectStatus() {
  var state = useAppState();
  var { sessions, spawns, issues, projectMeta, activePlan } = state;
  var projectName = (projectMeta && projectMeta.name) || 'project';

  var activeSessions = sessions.filter(function (s) {
    var status = normalizeStatus(s.status);
    return status === 'running' || status === 'starting' || status === 'in_progress';
  }).length;

  var phases = activePlan && activePlan.phases ? activePlan.phases : [];

  return (
    <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 13, fontWeight: 700, color: 'var(--accent)' }}>
          {'\u25C8'} {projectName}
        </span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', padding: '1px 6px', background: 'var(--bg-3)', borderRadius: 3 }}>
          .adaf/
        </span>
      </div>

      {/* Phase progress bars */}
      {phases.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          {phases.map(function (phase) {
            var status = normalizeStatus(phase.status || 'not_started');
            var progress = status === 'complete' ? 100 : status === 'in_progress' ? 50 : 0;
            return (
              <div key={phase.id} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                  color: progress > 0 ? 'var(--text-1)' : 'var(--text-3)',
                  width: 80, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                }}>
                  {phase.title || phase.id}
                </span>
                <div style={{ flex: 1, height: 3, background: 'var(--bg-4)', borderRadius: 2, overflow: 'hidden' }}>
                  <div style={{
                    height: '100%', width: progress + '%',
                    background: progress > 50 ? 'var(--green)' : progress > 0 ? 'var(--accent)' : 'transparent',
                    borderRadius: 2, transition: 'width 0.5s ease',
                  }} />
                </div>
                <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', minWidth: 24, textAlign: 'right' }}>
                  {progress}%
                </span>
              </div>
            );
          })}
        </div>
      )}

      {/* Stats row */}
      <div style={{ display: 'flex', gap: 12, marginTop: 10 }}>
        {[
          { label: 'Issues', value: issues.length, color: 'var(--orange)' },
          { label: 'Active', value: activeSessions, color: 'var(--green)' },
          { label: 'Sessions', value: sessions.length, color: 'var(--blue)' },
          { label: 'Spawns', value: spawns.length, color: 'var(--purple)' },
        ].map(function (stat) {
          return (
            <div key={stat.label} style={{ textAlign: 'center' }}>
              <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 14, fontWeight: 700, color: stat.color }}>{stat.value}</div>
              <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 8, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>{stat.label}</div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
