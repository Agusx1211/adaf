import { useEffect, useState, useMemo, useCallback } from 'react';
import { useAppState, useDispatch } from './state/store.js';
import { loadAuthToken, saveAuthToken, clearAuthToken, hasAuthToken } from './api/client.js';
import { usePolling, useViewData, useInitProjects, useLoopMessages } from './api/hooks.js';
import { useSessionSocket } from './api/websocket.js';
import { normalizeStatus, parseTimestamp } from './utils/format.js';
import { STATUS_RUNNING } from './utils/colors.js';
import TopBar from './components/layout/TopBar.jsx';
import LeftPanel from './components/layout/LeftPanel.jsx';
import CenterPanel from './components/layout/CenterPanel.jsx';
import RightSidebar from './components/layout/RightSidebar.jsx';
import Modal from './components/common/Modal.jsx';

export default function App() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { currentProjectID, leftView, selectedScope, sessions, loopRun, authRequired } = state;

  // Init auth token on mount
  useEffect(function () {
    loadAuthToken();
  }, []);

  // Init projects
  useInitProjects();

  // Core polling
  usePolling();

  // Load view data when switching views
  useViewData(leftView, currentProjectID);

  // Load loop messages when loop changes
  var loopID = loopRun && loopRun.id ? loopRun.id : 0;
  useLoopMessages(loopID, currentProjectID);

  // WebSocket for selected session
  var targetSessionID = useMemo(function () {
    if (!selectedScope) {
      var running = sessions.find(function (s) { return !!STATUS_RUNNING[normalizeStatus(s.status)]; });
      return running ? running.id : (sessions.length ? sessions[0].id : 0);
    }
    if (selectedScope.indexOf('session-') === 0) {
      var sid = parseInt(selectedScope.slice(8), 10);
      return Number.isNaN(sid) ? 0 : sid;
    }
    if (selectedScope.indexOf('spawn-') === 0) {
      var spawnID = parseInt(selectedScope.slice(6), 10);
      if (!Number.isNaN(spawnID)) {
        var spawn = state.spawns.find(function (s) { return s.id === spawnID; });
        if (spawn) {
          if (spawn.parent_turn_id > 0) return spawn.parent_turn_id;
          if (spawn.child_turn_id > 0) return spawn.child_turn_id;
        }
      }
    }
    return sessions.length ? sessions[0].id : 0;
  }, [selectedScope, sessions, state.spawns]);

  useSessionSocket(targetSessionID);

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

  return (
    <div style={{ width: '100vw', height: '100vh', display: 'flex', flexDirection: 'column', background: 'var(--bg-0)', overflow: 'hidden' }}>
      <TopBar />

      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        <LeftPanel />

        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
            <CenterPanel />
            {leftView === 'agents' && <RightSidebar />}
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
