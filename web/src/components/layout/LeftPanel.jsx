import { useAppState, useDispatch } from '../../state/store.js';
import { formatNumber } from '../../utils/format.js';
import ProjectStatus from '../project/ProjectStatus.jsx';
import AgentTree from '../tree/AgentTree.jsx';
import SectionHeader from '../common/SectionHeader.jsx';
import IssuesView from '../views/IssuesView.jsx';
import DocsView from '../views/DocsView.jsx';
import PlanView from '../views/PlanView.jsx';
import LogsView from '../views/LogsView.jsx';
import { NewSessionButton } from '../session/SessionControls.jsx';
import { STATUSES } from '../../utils/colors.js';

var LEFT_VIEWS = [
  { id: 'agents', label: 'Agents', icon: '\u2699' },
  { id: 'issues', label: 'Issues', icon: '\u26A0' },
  { id: 'docs', label: 'Docs', icon: '\u2630' },
  { id: 'plan', label: 'Plan', icon: '\u2261' },
  { id: 'logs', label: 'Logs', icon: '\u2263' },
];

export default function LeftPanel() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { leftView, sessions, spawns, issues, docs, plans, activePlan, turns } = state;

  var counts = {
    agents: sessions.length + spawns.length,
    issues: issues.length,
    docs: docs.length,
    plan: activePlan && activePlan.phases ? activePlan.phases.length : plans.length,
    logs: turns.length,
  };

  function setView(view) {
    dispatch({ type: 'SET_LEFT_VIEW', payload: view });
  }

  function renderContent() {
    if (leftView === 'agents') return <AgentTree />;
    if (leftView === 'issues') return <IssuesView />;
    if (leftView === 'docs') return <DocsView />;
    if (leftView === 'plan') return <PlanView />;
    if (leftView === 'logs') return <LogsView />;
    return null;
  }

  return (
    <div style={{
      width: 440, flexShrink: 0, display: 'flex', flexDirection: 'column',
      borderRight: '1px solid var(--border)', background: 'var(--bg-1)',
    }}>
      <ProjectStatus />

      {/* Tabs */}
      <div style={{ display: 'flex', borderBottom: '1px solid var(--border)', background: 'var(--bg-2)' }}>
        {LEFT_VIEWS.map(function (tab, idx) {
          var active = tab.id === leftView;
          return (
            <button
              key={tab.id}
              onClick={function () { setView(tab.id); }}
              style={{
                flex: 1, padding: '7px 4px', border: 'none',
                background: active ? 'var(--bg-1)' : 'transparent',
                color: active ? 'var(--text-0)' : 'var(--text-3)',
                fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                cursor: 'pointer',
                borderBottom: active ? '2px solid var(--accent)' : '2px solid transparent',
                display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 4,
                transition: 'all 0.15s ease',
              }}
            >
              <span style={{ fontSize: 8, opacity: 0.5 }}>{idx + 1}</span>
              <span>{tab.label}</span>
              <span style={{
                fontSize: 9, padding: '0 3px', borderRadius: 2,
                background: active ? 'var(--accent)30' : 'var(--bg-4)',
                color: active ? 'var(--accent)' : 'var(--text-3)',
              }}>{formatNumber(counts[tab.id] || 0)}</span>
            </button>
          );
        })}
      </div>

      {/* View header for agents */}
      {leftView === 'agents' && (
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '8px 14px', borderBottom: '1px solid var(--border)',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-1)' }}>Sessions</span>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
              {sessions.length} turns {'\u00B7'} {spawns.length} spawns
            </span>
          </div>
          <NewSessionButton />
        </div>
      )}

      {/* Content */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        {renderContent()}
      </div>

      {/* Legend for agents view */}
      {leftView === 'agents' && (
        <div style={{
          padding: '8px 14px', borderTop: '1px solid var(--border)',
          display: 'flex', gap: 12, flexWrap: 'wrap',
        }}>
          {Object.entries(STATUSES).map(function ([name, color]) {
            return (
              <div key={name} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                <span style={{ width: 6, height: 6, borderRadius: '50%', background: color }} />
                <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 8, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  {name}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
