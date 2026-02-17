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

  var planStatus = normalizeStatus(activePlan.status || 'active');
  var planColor = statusColor(planStatus);
  var planIcon = statusIcon(planStatus);
  var activePlanID = activePlan && activePlan.id ? activePlan.id : '';

  return (
    <div style={{ overflow: 'auto', flex: 1 }}>
      <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
          <span style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-0)' }}>{activePlan.title || activePlan.id || 'Plan'}</span>
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 9, padding: '1px 6px',
            background: withAlpha(planColor, 0.14), border: '1px solid ' + withAlpha(planColor, 0.28),
            borderRadius: 3, color: planColor,
          }}>{planIcon} {planStatus}</span>
        </div>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>{activePlan.id}</div>
        {activePlan.description && (
          <div style={{ marginTop: 8, fontSize: 11, color: 'var(--text-2)', lineHeight: 1.5 }}>
            {activePlan.description}
          </div>
        )}
      </div>

      {plans.length > 1 && plans.map(function (plan) {
        var selected = (selectedPlan || activePlanID) === plan.id;
        var status = normalizeStatus(plan.status || 'active');
        var color = statusColor(status);
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
            <span style={{ marginLeft: 6, color: 'var(--text-1)', flex: 1 }}>{plan.title || plan.id}</span>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: color,
              padding: '1px 5px', borderRadius: 3,
              background: withAlpha(color, 0.14), border: '1px solid ' + withAlpha(color, 0.28),
            }}>{status}</span>
          </div>
        );
      })}
    </div>
  );
}
