import { useState } from 'react';
import { useAppState } from '../../state/store.js';
import TabBar from '../common/TabBar.jsx';
import AgentInfoBar from '../detail/AgentInfoBar.jsx';
import AgentOutput from '../detail/AgentOutput.jsx';
import LoopVisualizer from '../loop/LoopVisualizer.jsx';
import StandaloneChatView from '../views/StandaloneChatView.jsx';
import IssueDetailPanel from '../detail/IssueDetailPanel.jsx';
import WikiDetailPanel from '../detail/WikiDetailPanel.jsx';
import PlanDetailPanel from '../detail/PlanDetailPanel.jsx';
import LogDetailPanel from '../detail/LogDetailPanel.jsx';
import ConfigDetailPanel from '../detail/ConfigDetailPanel.jsx';

export default function CenterPanel() {
  var state = useAppState();
  var [activeTab, setActiveTab] = useState('output');
  var { selectedScope, loopRun, leftView } = state;

  var isStandalone = leftView === 'standalone';
  var isLoops = leftView === 'loops';

  var loopName = loopRun ? (loopRun.loop_name || 'loop') : null;

  var tabs = [
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
      {/* Standalone Chat - stays mounted, hidden when not active */}
      <div style={{ display: isStandalone ? 'flex' : 'none', flexDirection: 'column', flex: 1, overflow: 'hidden' }}>
        <StandaloneChatView />
      </div>

      {/* Loops view */}
      {isLoops && (
        <div style={{ display: 'flex', flexDirection: 'column', flex: 1, overflow: 'hidden' }}>
          <AgentInfoBar scope={selectedScope} />
          {tabs.length > 1 && <TabBar tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab} />}
          <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
            {activeTab === 'output' ? (
              <AgentOutput scope={selectedScope} />
            ) : activeTab === 'loop' ? (
              <LoopVisualizer />
            ) : null}
          </div>
        </div>
      )}

      {/* Detail panels for other views */}
      {leftView === 'issues' && <IssueDetailPanel />}
      {leftView === 'wiki' && <WikiDetailPanel />}
      {leftView === 'plan' && <PlanDetailPanel />}
      {leftView === 'logs' && <LogDetailPanel />}
      {leftView === 'config' && <ConfigDetailPanel />}
    </div>
  );
}
