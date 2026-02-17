import { useAppState } from '../../state/store.js';
import { normalizeStatus } from '../../utils/format.js';
import { statusColor } from '../../utils/colors.js';

export default function ProjectStatus() {
  var state = useAppState();
  var { sessions, spawns, issues, projectMeta, activePlan } = state;
  var projectName = (projectMeta && projectMeta.name) || 'project';

  var activeSessions = sessions.filter(function (s) {
    var status = normalizeStatus(s.status);
    return status === 'running' || status === 'starting' || status === 'in_progress';
  }).length;

  var planStatus = normalizeStatus(activePlan && activePlan.status ? activePlan.status : '');
  var planStatusColor = statusColor(planStatus || 'active');

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

      {activePlan && (
        <div style={{
          marginTop: 2,
          marginBottom: 8,
          display: 'inline-flex',
          alignItems: 'center',
          gap: 6,
          padding: '2px 8px',
          borderRadius: 4,
          background: 'var(--bg-3)',
          border: '1px solid var(--border)',
          fontFamily: "'JetBrains Mono', monospace",
          fontSize: 10,
        }}>
          <span style={{ color: 'var(--text-3)' }}>plan:</span>
          <span style={{ color: 'var(--text-1)' }}>{activePlan.id || '-'}</span>
          <span style={{ color: planStatusColor }}>{planStatus || 'active'}</span>
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
