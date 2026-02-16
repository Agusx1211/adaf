import { useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiBase } from '../../api/client.js';
import ProjectStatus from '../project/ProjectStatus.jsx';
import LoopTree from '../tree/LoopTree.jsx';
import IssuesView from '../views/IssuesView.jsx';
import DocsView from '../views/DocsView.jsx';
import PlanView from '../views/PlanView.jsx';
import LogsView from '../views/LogsView.jsx';
import ConfigView from '../views/ConfigView.jsx';
import StandaloneConversationList, { NewChatModal } from '../views/StandaloneConversationList.jsx';
import { NewSessionButton } from '../session/SessionControls.jsx';
import { STATUSES } from '../../utils/colors.js';

export default function LeftPanel() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { leftView, sessions, loopRuns } = state;
  var [showNewChat, setShowNewChat] = useState(false);
  var base = apiBase(state.currentProjectID);

  function renderContent() {
    if (leftView === 'loops') return <LoopTree />;
    if (leftView === 'standalone') return <StandaloneConversationList />;
    if (leftView === 'issues') return <IssuesView />;
    if (leftView === 'docs') return <DocsView />;
    if (leftView === 'plan') return <PlanView />;
    if (leftView === 'logs') return <LogsView />;
    if (leftView === 'config') return <ConfigView />;
    return null;
  }

  return (
    <div style={{
      width: 380, flexShrink: 0, display: 'flex', flexDirection: 'column',
      borderRight: '1px solid var(--border)', background: 'var(--bg-1)',
    }}>
      <ProjectStatus />

      {/* View header for standalone */}
      {leftView === 'standalone' && (
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '8px 14px', borderBottom: '1px solid var(--border)',
        }}>
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-1)' }}>Chats</span>
          <button
            onClick={function () { setShowNewChat(true); }}
            style={{
              padding: '4px 10px', border: '1px solid var(--accent)40',
              background: 'var(--accent)15', color: 'var(--accent)',
              borderRadius: 4, cursor: 'pointer',
              fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
            }}
          >+ New Chat</button>
        </div>
      )}

      {/* View header for loops */}
      {leftView === 'loops' && (
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '8px 14px', borderBottom: '1px solid var(--border)',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-1)' }}>Loops</span>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
              {loopRuns.length} runs {'\u00B7'} {sessions.length} turns
            </span>
          </div>
          <NewSessionButton />
        </div>
      )}

      {/* Content */}
      <div style={{ flex: 1, overflow: 'auto' }}>
        {renderContent()}
      </div>

      {/* New Chat modal */}
      {showNewChat && (
        <NewChatModal
          base={base}
          onCreated={function (inst) {
            setShowNewChat(false);
            dispatch({ type: 'SET_STANDALONE_CHAT_ID', payload: inst.id });
            if (window.__reloadChatInstances) window.__reloadChatInstances();
          }}
          onClose={function () { setShowNewChat(false); }}
        />
      )}
    </div>
  );
}
