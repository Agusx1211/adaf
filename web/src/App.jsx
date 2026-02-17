import { useEffect, useState, useMemo, useCallback, useRef } from 'react';
import { useAppState, useDispatch } from './state/store.js';
import { loadAuthToken, saveAuthToken, clearAuthToken, hasAuthToken } from './api/client.js';
import { usePolling, useViewData, useInitProjects, useLoopMessages } from './api/hooks.js';
import { useSessionSocket } from './api/websocket.js';
import { normalizeStatus, parseTimestamp } from './utils/format.js';
import { STATUS_RUNNING } from './utils/colors.js';
import { stateToHash, hashToActions } from './utils/deeplink.js';
import { buildSpawnScopeMaps, parseScope } from './utils/scopes.js';
import TopBar from './components/layout/TopBar.jsx';
import LeftPanel from './components/layout/LeftPanel.jsx';
import CenterPanel from './components/layout/CenterPanel.jsx';
import Modal from './components/common/Modal.jsx';
import ProjectPicker from './components/layout/ProjectPicker.jsx';

export default function App() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { currentProjectID, leftView, selectedScope, sessions, spawns, loopRun, authRequired, projects } = state;

  // Ref to suppress hash→state→hash circular updates
  var suppressHashUpdate = useRef(false);
  // Track previous view for pushState vs replaceState decision
  var prevViewRef = useRef(null);

  // Init auth token on mount
  useEffect(function () {
    loadAuthToken();
  }, []);

  // Deep link: parse initial hash on mount
  useEffect(function () {
    var actions = hashToActions(window.location.hash);
    if (actions.length) {
      suppressHashUpdate.current = true;
      actions.forEach(function (a) { dispatch(a); });
      // Allow state→hash sync to resume after this tick
      requestAnimationFrame(function () { suppressHashUpdate.current = false; });
    }
  }, [dispatch]);

  // Deep link: popstate listener for Back/Forward navigation
  useEffect(function () {
    function onPopState() {
      var actions = hashToActions(window.location.hash);
      if (actions.length) {
        suppressHashUpdate.current = true;
        actions.forEach(function (a) { dispatch(a); });
        requestAnimationFrame(function () { suppressHashUpdate.current = false; });
      }
    }
    window.addEventListener('popstate', onPopState);
    return function () { window.removeEventListener('popstate', onPopState); };
  }, [dispatch]);

  // Init projects
  useInitProjects();

  // Core polling
  usePolling();

  // Load view data when switching views
  useViewData(leftView, currentProjectID);

  // Deep link: sync state → URL hash
  var selectedIssue = state.selectedIssue;
  var selectedPlan = state.selectedPlan;
  var selectedWiki = state.selectedWiki;
  var selectedTurn = state.selectedTurn;
  var configSelection = state.configSelection;
  var standaloneChatID = state.standaloneChatID;

  useEffect(function () {
    if (suppressHashUpdate.current) return;
    var hash = stateToHash({
      leftView: leftView,
      selectedScope: selectedScope,
      selectedIssue: selectedIssue,
      selectedPlan: selectedPlan,
      selectedWiki: selectedWiki,
      selectedTurn: selectedTurn,
      configSelection: configSelection,
      standaloneChatID: standaloneChatID,
    });
    // Use pushState when the view changes (enables Back/Forward between views),
    // replaceState for selection changes within the same view.
    var prevView = prevViewRef.current;
    var currentHash = window.location.hash || '';
    if (hash === currentHash) {
      prevViewRef.current = leftView;
      return;
    }
    if (prevView !== null && prevView !== leftView) {
      history.pushState(null, '', hash);
    } else {
      history.replaceState(null, '', hash);
    }
    prevViewRef.current = leftView;
  }, [leftView, selectedScope, selectedIssue, selectedPlan, selectedWiki, selectedTurn, configSelection, standaloneChatID]);

  // Load loop messages when loop changes
  var loopID = loopRun && loopRun.id ? loopRun.id : 0;
  useLoopMessages(loopID, currentProjectID);

  // WebSocket for selected session
  var spawnScopeMaps = useMemo(function () {
    return buildSpawnScopeMaps(state.spawns, state.loopRuns);
  }, [state.spawns, state.loopRuns]);

  var targetSessionID = useMemo(function () {
    if (!selectedScope) {
      var running = sessions.find(function (s) { return !!STATUS_RUNNING[normalizeStatus(s.status)]; });
      return running ? running.id : (sessions.length ? sessions[0].id : 0);
    }
    var parsedScope = parseScope(selectedScope);
    if (parsedScope.kind === 'session' || parsedScope.kind === 'session_main') return parsedScope.id;
    if (parsedScope.kind === 'turn' || parsedScope.kind === 'turn_main') {
      var mappedTurnSession = spawnScopeMaps.turnToSession[parsedScope.id] || 0;
      if (mappedTurnSession > 0) return mappedTurnSession;
    }
    if (parsedScope.kind === 'spawn') {
      var mapped = spawnScopeMaps.spawnToSession[parsedScope.id] || 0;
      if (mapped > 0) return mapped;
    }
    return sessions.length ? sessions[0].id : 0;
  }, [selectedScope, sessions, spawnScopeMaps]);

  // Only open WebSocket for running sessions. Non-running sessions load
  // historical recordings via the REST API (see AgentOutput.jsx).
  var wsSessionID = useMemo(function () {
    if (!targetSessionID) return 0;
    var sess = sessions.find(function (s) { return s.id === targetSessionID; });
    if (!sess) return targetSessionID; // unknown status — try connecting
    var status = normalizeStatus(sess.status);
    return (STATUS_RUNNING[status] || status === 'waiting' || status === 'waiting_for_spawns')
      ? targetSessionID
      : 0;
  }, [targetSessionID, sessions]);

  useSessionSocket(wsSessionID);

  // Ensure tree defaults — expand running sessions
  useEffect(function () {
    if (Object.keys(state.expandedNodes).length > 0) return;
    var toExpand = [];
    sessions.forEach(function (s) { toExpand.push('session-' + s.id); });
    state.spawns.forEach(function (s) {
      if (normalizeStatus(s.status) === 'running' || normalizeStatus(s.status) === 'awaiting_input') {
        toExpand.push('spawn-' + s.id);
      }
    });
    if (toExpand.length) dispatch({ type: 'EXPAND_NODES', payload: toExpand });
  }, [sessions, state.spawns]);

  // Ensure default scope
  useEffect(function () {
    if (selectedScope) return;
    if (sessions.length === 0) return;
    var running = sessions.find(function (s) { return !!STATUS_RUNNING[normalizeStatus(s.status)]; });
    var defaultScope = running ? 'session-' + running.id : 'session-' + sessions[0].id;
    dispatch({ type: 'SET_SELECTED_SCOPE', payload: defaultScope });
  }, [sessions, selectedScope, dispatch]);

  var projectMetaName = state.projectMeta && state.projectMeta.name ? state.projectMeta.name : '';
  var titleProjectName = useMemo(function () {
    if (currentProjectID) {
      var selected = projects.find(function (p) { return p && String(p.id || '') === currentProjectID; });
      if (selected && selected.name) return selected.name;
    }
    if (projectMetaName) return projectMetaName;
    if (!currentProjectID && projects.length) {
      var defaultProject = projects.find(function (p) { return p && p.is_default; }) || projects[0];
      if (defaultProject && defaultProject.name) return defaultProject.name;
    }
    return 'project';
  }, [currentProjectID, projects, projectMetaName]);

  var runningAgents = useMemo(function () {
    var runningCount = 0;
    var runningSessionsByID = {};
    sessions.forEach(function (s) {
      if (STATUS_RUNNING[normalizeStatus(s.status)]) {
        runningCount++;
        runningSessionsByID[s.id] = true;
      }
    });
    spawns.forEach(function (s) {
      if (!STATUS_RUNNING[normalizeStatus(s.status)]) return;
      var owningSessionID = spawnScopeMaps.spawnToSession[s.id] || 0;
      if (owningSessionID <= 0) return;
      if (!runningSessionsByID[owningSessionID]) return;
      runningCount++;
    });
    return runningCount;
  }, [sessions, spawns, spawnScopeMaps]);

  useEffect(function () {
    var baseTitle = titleProjectName + ' - ' + runningAgents + ' running';
    if (runningAgents <= 0) {
      document.title = baseTitle;
      return;
    }

    var frames = ['|', '/', '-', '\\'];
    var frameIndex = 0;
    document.title = baseTitle + ' ' + frames[frameIndex];
    var intervalID = setInterval(function () {
      frameIndex = (frameIndex + 1) % frames.length;
      document.title = baseTitle + ' ' + frames[frameIndex];
    }, 500);
    return function () { clearInterval(intervalID); };
  }, [titleProjectName, runningAgents]);

  // Live clock for elapsed times
  var [, setTick] = useState(0);
  useEffect(function () {
    var iv = setInterval(function () { setTick(function (t) { return t + 1; }); }, 1000);
    return function () { clearInterval(iv); };
  }, []);

  // Auth modal
  var [showAuthModal, setShowAuthModal] = useState(false);
  useEffect(function () {
    if (authRequired && !hasAuthToken()) {
      setShowAuthModal(true);
    }
  }, [authRequired]);

  var handleAuth = useCallback(function (e) {
    e.preventDefault();
    var token = e.target.auth_token?.value || '';
    if (!token.trim()) return;
    saveAuthToken(token.trim());
    dispatch({ type: 'SET', payload: { authRequired: false } });
    setShowAuthModal(false);
  }, [dispatch]);

  // Show project picker when no project is selected and picker is needed
  if (state.needsProjectPicker && !currentProjectID) {
    return <ProjectPicker />;
  }

  return (
    <div style={{ width: '100vw', height: '100vh', display: 'flex', flexDirection: 'column', background: 'var(--bg-0)', overflow: 'hidden' }}>
      <TopBar />

      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        <LeftPanel />

        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
            <CenterPanel />
          </div>
        </div>
      </div>

      {showAuthModal && (
        <Modal title="Authentication Required" onClose={function () { setShowAuthModal(false); }}>
          <form onSubmit={handleAuth}>
            <div style={{ marginBottom: 12 }}>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Bearer Token</label>
              <input
                name="auth_token"
                type="text"
                placeholder="Paste your token"
                autoComplete="off"
                required
                style={{
                  width: '100%', padding: '6px 8px', background: 'var(--bg-3)',
                  border: '1px solid var(--border)', borderRadius: 4,
                  color: 'var(--text-0)', fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                }}
              />
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <button type="button"
                onClick={function () { clearAuthToken(); setShowAuthModal(false); }}
                style={{
                  padding: '6px 12px', border: '1px solid var(--red)40',
                  background: 'transparent', color: 'var(--red)', borderRadius: 4,
                  cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                }}
              >Clear Stored Token</button>
              <button type="button"
                onClick={function () { setShowAuthModal(false); }}
                style={{
                  padding: '6px 12px', border: '1px solid var(--border)',
                  background: 'var(--bg-2)', color: 'var(--text-1)', borderRadius: 4,
                  cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                }}
              >Cancel</button>
              <button type="submit" style={{
                padding: '6px 12px', border: '1px solid var(--accent)',
                background: 'var(--accent)', color: '#000', borderRadius: 4,
                cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
              }}>Connect</button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
