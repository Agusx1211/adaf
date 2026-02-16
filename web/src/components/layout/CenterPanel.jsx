import { useState, useMemo, useCallback } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { agentInfo, STATUS_RUNNING } from '../../utils/colors.js';
import { normalizeStatus } from '../../utils/format.js';
import { buildSpawnScopeMaps, parseScope } from '../../utils/scopes.js';
import TabBar from '../common/TabBar.jsx';
import AgentScopeSidebar from '../common/AgentScopeSidebar.jsx';
import AgentInfoBar from '../detail/AgentInfoBar.jsx';
import AgentOutput from '../detail/AgentOutput.jsx';
import LoopVisualizer from '../loop/LoopVisualizer.jsx';
import StandaloneChatView from '../views/StandaloneChatView.jsx';
import IssueDetailPanel from '../detail/IssueDetailPanel.jsx';
import DocsDetailPanel from '../detail/DocsDetailPanel.jsx';
import PlanDetailPanel from '../detail/PlanDetailPanel.jsx';
import LogDetailPanel from '../detail/LogDetailPanel.jsx';
import ConfigDetailPanel from '../detail/ConfigDetailPanel.jsx';

export default function CenterPanel() {
  var state = useAppState();
  var dispatch = useDispatch();
  var [activeTab, setActiveTab] = useState('output');
  var { selectedScope, loopRun, leftView, sessions, spawns, loopRuns } = state;

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

  var scopeInfo = useMemo(function () {
    return parseScope(selectedScope);
  }, [selectedScope]);

  var scopeMaps = useMemo(function () {
    return buildSpawnScopeMaps(spawns, loopRuns);
  }, [spawns, loopRuns]);

  var selectedSessionID = useMemo(function () {
    if (scopeInfo.kind === 'session' || scopeInfo.kind === 'session_main') return scopeInfo.id;
    if (scopeInfo.kind === 'spawn') return scopeMaps.spawnToSession[scopeInfo.id] || 0;
    return 0;
  }, [scopeInfo, scopeMaps]);

  var selectedSession = useMemo(function () {
    if (selectedSessionID <= 0) return null;
    return sessions.find(function (session) { return session.id === selectedSessionID; }) || null;
  }, [sessions, selectedSessionID]);

  var sessionSpawns = useMemo(function () {
    if (selectedSessionID <= 0) return [];
    return spawns.filter(function (spawn) {
      return scopeMaps.spawnToSession[spawn.id] === selectedSessionID;
    });
  }, [spawns, scopeMaps, selectedSessionID]);

  var sidebarScope = useMemo(function () {
    if (scopeInfo.kind === 'spawn') return 'spawn-' + scopeInfo.id;
    if (scopeInfo.kind === 'session_main') return 'parent';
    return 'all';
  }, [scopeInfo]);

  var onLoopSidebarSelectScope = useCallback(function (nextScope) {
    if (selectedSessionID <= 0 || !nextScope) return;
    if (nextScope === 'parent') {
      dispatch({ type: 'SET_SELECTED_SCOPE', payload: 'session-main-' + selectedSessionID });
      return;
    }
    if (nextScope === 'all') {
      dispatch({ type: 'SET_SELECTED_SCOPE', payload: 'session-' + selectedSessionID });
      return;
    }
    if (nextScope.indexOf('spawn-') !== 0) return;
    var spawnID = parseInt(nextScope.slice(6), 10);
    if (Number.isNaN(spawnID) || spawnID <= 0) return;
    dispatch({ type: 'SET_SELECTED_SCOPE', payload: 'spawn-' + spawnID });
  }, [dispatch, selectedSessionID]);

  var showLoopSidebar = activeTab === 'output' && selectedSessionID > 0 && sessionSpawns.length > 0;
  var parentColor = agentInfo(selectedSession ? selectedSession.agent : '').color;
  var parentActive = !!(selectedSession && STATUS_RUNNING[normalizeStatus(selectedSession.status)]);
  var parentLabel = selectedSession && selectedSession.profile ? selectedSession.profile : (selectedSessionID > 0 ? ('session #' + selectedSessionID) : 'Main');

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
              <div style={{ flex: 1, display: 'flex', minHeight: 0, overflow: 'hidden' }}>
                <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                  <AgentOutput scope={selectedScope} />
                </div>
                {showLoopSidebar && (
                  <AgentScopeSidebar
                    spawns={sessionSpawns}
                    selectedScope={sidebarScope}
                    onSelectScope={onLoopSidebarSelectScope}
                    parentLabel={parentLabel}
                    parentSubLabel="main agent"
                    parentColor={parentColor}
                    parentActive={parentActive}
                    title="Agents"
                    showAll
                    allLabel="All agents"
                    allCount={sessionSpawns.length + 1}
                  />
                )}
              </div>
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
