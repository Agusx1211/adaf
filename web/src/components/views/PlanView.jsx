import { useAppState, useDispatch } from '../../state/store.js';
import { normalizeStatus } from '../../utils/format.js';
import { statusColor, statusIcon } from '../../utils/colors.js';
import { withAlpha } from '../../utils/format.js';

export default function PlanView() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { plans, activePlan, selectedPlan } = state;

  if (!activePlan && !plans.length) {
    return <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>No plans loaded.</div>;
  }

  if (!activePlan) {
    return (
      <div style={{ overflow: 'auto', flex: 1 }}>
        <div style={{ padding: 12, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>Select a plan.</div>
        {plans.map(function (plan) {
          var selected = selectedPlan === plan.id;
          return (
            <div key={plan.id}
              onClick={function () { dispatch({ type: 'SET_SELECTED_PLAN', payload: plan.id }); }}
            style={{
              padding: '10px 14px', borderBottom: '1px solid var(--border)', cursor: 'pointer',
              background: selected ? 'var(--bg-3)' : 'transparent',
              transition: 'background 0.15s ease',
            }}
            onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-2)'; }}
            onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
            >
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>{plan.id}</span>
              <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-0)', marginLeft: 8 }}>{plan.title || plan.id}</span>
            </div>
          );
        })}
      </div>
    );
  }

  var phases = activePlan.phases || [];
  var completeCount = phases.filter(function (p) { return normalizeStatus(p.status) === 'complete'; }).length;
  var percent = phases.length ? Math.round((completeCount / phases.length) * 100) : 0;
  var activePlanID = activePlan && activePlan.id ? activePlan.id : '';

  return (
    <div style={{ overflow: 'auto', flex: 1 }}>
      {/* Plan overview */}
      <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)' }}>
        <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-0)', marginBottom: 8 }}>{activePlan.title || activePlan.id || 'Plan'}</div>
        <div style={{ height: 4, background: 'var(--bg-4)', borderRadius: 2, overflow: 'hidden', marginBottom: 4 }}>
          <div style={{ height: '100%', width: percent + '%', background: 'var(--green)', borderRadius: 2, transition: 'width 0.5s ease' }} />
        </div>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>{percent}% complete</div>
      </div>

      {/* Plan selector if multiple */}
      {plans.length > 1 && plans.map(function (plan) {
        var selected = (selectedPlan || activePlanID) === plan.id;
        return (
          <div key={plan.id}
            onClick={function () { dispatch({ type: 'SET_SELECTED_PLAN', payload: plan.id }); }}
            style={{
              padding: '6px 14px', borderBottom: '1px solid var(--bg-3)', cursor: 'pointer',
              background: selected ? 'var(--bg-3)' : 'transparent', fontSize: 11,
              transition: 'background 0.15s ease',
            }}
            onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-2)'; }}
            onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
          >
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>{plan.id}</span>
            <span style={{ marginLeft: 6, color: 'var(--text-1)' }}>{plan.title || plan.id}</span>
          </div>
        );
      })}

      {/* Phases */}
      {phases.map(function (phase) {
        var status = normalizeStatus(phase.status || 'not_started');
        var color = statusColor(status);
        var marker = status === 'complete' ? '\u2713' : status === 'in_progress' ? '\u25C9' : status === 'blocked' ? '\u2717' : '\u25CB';

        return (
          <div key={phase.id} style={{
            padding: '10px 14px', borderBottom: '1px solid var(--border)',
            borderLeft: '3px solid ' + color,
          }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ color: color, fontSize: 12 }}>{marker}</span>
              <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-0)' }}>{phase.title || phase.id || 'Phase'}</span>
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 9, padding: '1px 5px',
                background: withAlpha(color, 0.14), border: '1px solid ' + withAlpha(color, 0.28),
                borderRadius: 3, color: color,
              }}>{status}</span>
            </div>
            {phase.description && <div style={{ fontSize: 11, color: 'var(--text-2)', marginTop: 4, lineHeight: 1.4 }}>{phase.description}</div>}
            {phase.depends_on && phase.depends_on.length > 0 && (
              <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 4 }}>
                depends on: {phase.depends_on.join(', ')}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
