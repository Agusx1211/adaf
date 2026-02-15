import { useState } from 'react';
import { useAppState } from '../../state/store.js';
import TabBar from '../common/TabBar.jsx';
import AgentDetail from '../detail/AgentDetail.jsx';
import AgentOutput from '../detail/AgentOutput.jsx';
import LoopVisualizer from '../loop/LoopVisualizer.jsx';
import PMChatView from '../views/PMChatView.jsx';
import StandaloneChatView from '../views/StandaloneChatView.jsx';
import IssueDetailPanel from '../detail/IssueDetailPanel.jsx';
import DocsDetailPanel from '../detail/DocsDetailPanel.jsx';
import PlanDetailPanel from '../detail/PlanDetailPanel.jsx';
import LogDetailPanel from '../detail/LogDetailPanel.jsx';
import ConfigDetailPanel from '../detail/ConfigDetailPanel.jsx';

export default function CenterPanel() {
  var state = useAppState();
  var [activeTab, setActiveTab] = useState('detail');
  var { selectedScope, loopRun, leftView } = state;

  var isPM = leftView === 'pm';
  var isStandalone = leftView === 'standalone';
  var isAgents = leftView === 'agents';

  var loopName = loopRun ? (loopRun.loop_name || 'loop') : null;

  var tabs = [
    {
      id: 'detail', label: 'Agent Detail', icon: '\u25C9',
      color: selectedScope ? undefined : 'var(--accent)',
    },
    {
      id: 'output', label: 'Output', icon: '\u25A3',
    },
  ];

  if (loopRun) {
    tabs.push({
      id: 'loop', label: 'Loop: ' + (loopName || ''), icon: '\u21BB',
      color: 'var(--purple)', count: 'C' + ((loopRun.cycle || 0) + 1),
    });
  }

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      {/* PM Chat - stays mounted, hidden when not active */}
      <div style={{ display: isPM ? 'flex' : 'none', flexDirection: 'column', flex: 1, overflow: 'hidden' }}>
        <PMChatView />
      </div>

      {/* Standalone Chat - stays mounted, hidden when not active */}
      <div style={{ display: isStandalone ? 'flex' : 'none', flexDirection: 'column', flex: 1, overflow: 'hidden' }}>
        <StandaloneChatView />
      </div>

      {/* Normal agent view - only for agents view */}
      {isAgents && (
        <div style={{ display: 'flex', flexDirection: 'column', flex: 1, overflow: 'hidden' }}>
          <TabBar tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab} />
          <div style={{ flex: 1, overflow: 'hidden' }}>
            {activeTab === 'detail' ? (
              <AgentDetail scope={selectedScope} />
            ) : activeTab === 'output' ? (
              <AgentOutput scope={selectedScope} />
            ) : activeTab === 'loop' ? (
              <LoopVisualizer />
            ) : null}
          </div>
        </div>
      )}

      {/* Detail panels for other views */}
      {leftView === 'issues' && <IssueDetailPanel />}
      {leftView === 'docs' && <DocsDetailPanel />}
      {leftView === 'plan' && <PlanDetailPanel />}
      {leftView === 'logs' && <LogDetailPanel />}
      {leftView === 'config' && <ConfigDetailPanel />}
    </div>
  );
}
