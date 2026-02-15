import { useAppState, useDispatch } from '../../state/store.js';
import ProjectStatus from '../project/ProjectStatus.jsx';
import AgentTree from '../tree/AgentTree.jsx';
import IssuesView from '../views/IssuesView.jsx';
import DocsView from '../views/DocsView.jsx';
import PlanView from '../views/PlanView.jsx';
import LogsView from '../views/LogsView.jsx';
import ConfigView from '../views/ConfigView.jsx';
import { NewSessionButton } from '../session/SessionControls.jsx';
import { STATUSES } from '../../utils/colors.js';

export default function LeftPanel() {
  var state = useAppState();
  var { leftView, sessions, spawns } = state;

  function renderContent() {
    if (leftView === 'agents') return <AgentTree />;
    if (leftView === 'standalone') return (
      <div style={{ padding: 20, textAlign: 'center', color: 'var(--text-3)' }}>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, marginBottom: 8 }}>
          Standalone Chat
        </div>
        <div style={{ fontSize: 10, lineHeight: 1.5, opacity: 0.7 }}>
          Chat with your standalone agent in the center panel. Select a standalone profile and start working.
        </div>
      </div>
    );
    if (leftView === 'pm') return (
      <div style={{ padding: 20, textAlign: 'center', color: 'var(--text-3)' }}>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, marginBottom: 8 }}>
          PM Chat
        </div>
        <div style={{ fontSize: 10, lineHeight: 1.5, opacity: 0.7 }}>
          Chat with your PM assistant in the center panel. Ask about plans, issues, priorities, or project status.
        </div>
      </div>
    );
    if (leftView === 'issues') return <IssuesView />;
    if (leftView === 'docs') return <DocsView />;
    if (leftView === 'plan') return <PlanView />;
    if (leftView === 'logs') return <LogsView />;
    if (leftView === 'config') return <ConfigView />;
    return null;
  }

  return (
    <div style={{
      width: 440, flexShrink: 0, display: 'flex', flexDirection: 'column',
      borderRight: '1px solid var(--border)', background: 'var(--bg-1)',
    }}>
      <ProjectStatus />

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
