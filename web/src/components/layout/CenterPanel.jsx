import { useState } from 'react';
import { useAppState } from '../../state/store.js';
import { agentInfo } from '../../utils/colors.js';
import TabBar from '../common/TabBar.jsx';
import AgentDetail from '../detail/AgentDetail.jsx';
import AgentOutput from '../detail/AgentOutput.jsx';
import LoopVisualizer from '../loop/LoopVisualizer.jsx';
import PMChatView from '../views/PMChatView.jsx';

export default function CenterPanel() {
  var state = useAppState();
  var [activeTab, setActiveTab] = useState('detail');
  var { selectedScope, loopRun, leftView } = state;

  var isPM = leftView === 'pm';

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

      {/* Normal agent view */}
      <div style={{ display: isPM ? 'none' : 'flex', flexDirection: 'column', flex: 1, overflow: 'hidden' }}>
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
    </div>
  );
}
