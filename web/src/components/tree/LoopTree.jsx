import { useMemo } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { agentInfo, statusColor, STATUS_RUNNING } from '../../utils/colors.js';
import { normalizeStatus, formatElapsed, parseTimestamp } from '../../utils/format.js';
import { StopLoopButton, StopSessionButton, WindDownLoopButton } from '../session/SessionControls.jsx';

export default function LoopTree() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { sessions, spawns, turns, loopRuns, selectedScope, expandedNodes } = state;

  var tree = useMemo(function () {
    var filteredRuns = loopRuns.filter(function (lr) {
      return lr.loop_name !== 'standalone-chat';
    });

    var sessionToRun = {};
    filteredRuns.forEach(function (lr) {
      if (lr.daemon_session_id > 0) {
        sessionToRun[lr.daemon_session_id] = lr;
      } else if (lr.turn_ids && lr.turn_ids.length) {
        lr.turn_ids.forEach(function (tid) {
          if (tid > 0) sessionToRun[tid] = lr;
        });
      }
    });

    var runGroups = {};
    var unclaimedByName = {};
    var standaloneSessions = [];

    sessions.forEach(function (session) {
      if (session.loop_name === 'standalone-chat') return;

      var matchedRun = sessionToRun[session.id];
      if (matchedRun) {
        if (!runGroups[matchedRun.id]) {
          runGroups[matchedRun.id] = { loopRun: matchedRun, sessions: [] };
        }
        runGroups[matchedRun.id].sessions.push(session);
        return;
      }

      if (session.loop_name) {
        if (!unclaimedByName[session.loop_name]) unclaimedByName[session.loop_name] = [];
        unclaimedByName[session.loop_name].push(session);
        return;
      }

      standaloneSessions.push(session);
    });

    Object.keys(unclaimedByName).forEach(function (loopName) {
      var loopSessions = unclaimedByName[loopName];
      var matchingRuns = filteredRuns.filter(function (lr) { return lr.loop_name === loopName; });

      if (matchingRuns.length === 1) {
        var run = matchingRuns[0];
        if (!runGroups[run.id]) {
          runGroups[run.id] = { loopRun: run, sessions: [] };
        }
        loopSessions.forEach(function (s) { runGroups[run.id].sessions.push(s); });
      } else if (matchingRuns.length > 1) {
        loopSessions.forEach(function (s) {
          var sTime = parseTimestamp(s.started_at);
          var bestRun = null;
          var bestDiff = Infinity;
          matchingRuns.forEach(function (lr) {
            var rTime = parseTimestamp(lr.started_at);
            var diff = Math.abs(sTime - rTime);
            if (diff < bestDiff) {
              bestDiff = diff;
              bestRun = lr;
            }
          });
          if (!bestRun) {
            standaloneSessions.push(s);
            return;
          }
          if (!runGroups[bestRun.id]) {
            runGroups[bestRun.id] = { loopRun: bestRun, sessions: [] };
          }
          runGroups[bestRun.id].sessions.push(s);
        });
      } else {
        var syntheticID = 'name-' + loopName;
        runGroups[syntheticID] = {
          loopRun: {
            id: 0,
            loop_name: loopName,
            status: 'completed',
            hex_id: '',
            cycle: 0,
            started_at: loopSessions[0] ? loopSessions[0].started_at : '',
          },
          sessions: loopSessions,
        };
      }
    });

    Object.keys(runGroups).forEach(function (key) {
      runGroups[key].sessions.sort(function (a, b) { return b.id - a.id; });
    });

    var sortedRuns = Object.values(runGroups).sort(function (a, b) {
      var aTime = parseTimestamp(a.loopRun.started_at) || (a.sessions[0] ? parseTimestamp(a.sessions[0].started_at) : 0);
      var bTime = parseTimestamp(b.loopRun.started_at) || (b.sessions[0] ? parseTimestamp(b.sessions[0].started_at) : 0);
      return bTime - aTime;
    });

    standaloneSessions.sort(function (a, b) { return b.id - a.id; });
    return { sortedRuns: sortedRuns, standaloneSessions: standaloneSessions };
  }, [sessions, loopRuns]);

  var spawnTree = useMemo(function () {
    var storeTurnToDaemonSession = {};
    (loopRuns || []).forEach(function (lr) {
      if (lr.daemon_session_id > 0 && lr.turn_ids) {
        lr.turn_ids.forEach(function (tid) {
          if (tid > 0) storeTurnToDaemonSession[tid] = lr.daemon_session_id;
        });
      }
    });

    var childrenByParent = {};
    var rootsBySession = {};
    var rootsByTurn = {};

    spawns.forEach(function (spawn) {
      if (spawn.parent_spawn_id > 0) {
        if (!childrenByParent[spawn.parent_spawn_id]) childrenByParent[spawn.parent_spawn_id] = [];
        childrenByParent[spawn.parent_spawn_id].push(spawn);
      } else {
        var storeTurnID = spawn.parent_turn_id || 0;
        var sessionKey = spawn.parent_daemon_session_id || storeTurnToDaemonSession[storeTurnID] || storeTurnID;
        if (sessionKey <= 0) return;
        if (!rootsBySession[sessionKey]) rootsBySession[sessionKey] = [];
        rootsBySession[sessionKey].push(spawn);
        if (storeTurnID > 0) {
          if (!rootsByTurn[storeTurnID]) rootsByTurn[storeTurnID] = [];
          rootsByTurn[storeTurnID].push(spawn);
        }
      }
    });

    return { childrenByParent: childrenByParent, rootsBySession: rootsBySession, rootsByTurn: rootsByTurn };
  }, [spawns, loopRuns]);

  var sessionMetaByID = useMemo(function () {
    var metaByID = {};
    var turnToDaemonSession = {};
    var modelTurnBySession = {};

    (loopRuns || []).forEach(function (lr) {
      var daemonSessionID = Number(lr && lr.daemon_session_id) || 0;
      if (daemonSessionID <= 0) return;

      if (!metaByID[daemonSessionID]) metaByID[daemonSessionID] = {};

      var steps = Array.isArray(lr && lr.steps) ? lr.steps : [];
      if (steps.length > 0) {
        var stepIndex = Number(lr && lr.step_index);
        if (!Number.isFinite(stepIndex) || stepIndex < 0) stepIndex = 0;
        if (stepIndex >= steps.length) stepIndex = stepIndex % steps.length;
        var step = steps[stepIndex] || steps[0] || {};
        if (step.position) metaByID[daemonSessionID].position = String(step.position);
      }

      var turnIDs = Array.isArray(lr && lr.turn_ids) ? lr.turn_ids : [];
      turnIDs.forEach(function (turnIDRaw) {
        var turnID = Number(turnIDRaw) || 0;
        if (turnID > 0) turnToDaemonSession[turnID] = daemonSessionID;
      });
    });

    (turns || []).forEach(function (turn) {
      if (!turn || turn.id <= 0 || !turn.agent_model) return;
      var daemonSessionID = turnToDaemonSession[turn.id] || 0;
      if (daemonSessionID <= 0) return;
      if (!metaByID[daemonSessionID]) metaByID[daemonSessionID] = {};

      var prevTurnID = modelTurnBySession[daemonSessionID] || 0;
      if (turn.id >= prevTurnID) {
        metaByID[daemonSessionID].model = String(turn.agent_model);
        modelTurnBySession[daemonSessionID] = turn.id;
      }
    });

    return metaByID;
  }, [loopRuns, turns]);

  var turnsByID = useMemo(function () {
    var map = {};
    (turns || []).forEach(function (turn) {
      if (!turn || turn.id <= 0) return;
      map[turn.id] = turn;
    });
    return map;
  }, [turns]);

  var turnStepByRun = useMemo(function () {
    var out = {};
    (loopRuns || []).forEach(function (lr) {
      var runKey = (lr && (lr.id || lr.loop_name)) || '';
      if (!runKey) return;
      var turnIDs = uniquePositiveIDs(Array.isArray(lr && lr.turn_ids) ? lr.turn_ids : []);
      if (!turnIDs.length) return;
      var steps = Array.isArray(lr && lr.steps) ? lr.steps : [];
      if (!steps.length) return;

      var stepByHex = {};
      var stepHexIDs = lr && typeof lr.step_hex_ids === 'object' ? lr.step_hex_ids : {};
      Object.keys(stepHexIDs).forEach(function (cycleStep) {
        var hex = String(stepHexIDs[cycleStep] || '').trim();
        if (!hex) return;
        var keyParts = String(cycleStep || '').split(':');
        if (keyParts.length !== 2) return;
        var parsedStepIndex = Number(keyParts[1]);
        if (!Number.isFinite(parsedStepIndex) || parsedStepIndex < 0 || parsedStepIndex >= steps.length) return;
        var matchedStep = steps[parsedStepIndex] || {};
        stepByHex[hex] = {
          profile: matchedStep.profile ? String(matchedStep.profile) : '',
          position: matchedStep.position ? String(matchedStep.position) : '',
          step_index: parsedStepIndex,
        };
      });

      var byTurn = {};
      var stepIndex = 0;
      var stepTurnsRemaining = stepTurns(steps[0]);

      turnIDs.forEach(function (turnID) {
        var mappedStep = null;
        var turn = turnsByID[turnID] || null;
        var turnStepHex = String(turn && turn.step_hex_id || '').trim();
        if (turnStepHex && stepByHex[turnStepHex]) {
          mappedStep = stepByHex[turnStepHex];
        }
        if (!mappedStep) {
          var fallbackStep = steps[stepIndex] || {};
          mappedStep = {
            profile: fallbackStep.profile ? String(fallbackStep.profile) : '',
            position: fallbackStep.position ? String(fallbackStep.position) : '',
            step_index: stepIndex,
          };
        }
        byTurn[turnID] = mappedStep;
        stepTurnsRemaining -= 1;
        if (stepTurnsRemaining <= 0) {
          stepIndex = (stepIndex + 1) % steps.length;
          stepTurnsRemaining = stepTurns(steps[stepIndex]);
        }
      });

      out[runKey] = byTurn;
    });
    return out;
  }, [loopRuns, turnsByID]);

  function toggleNode(nodeID) {
    dispatch({ type: 'TOGGLE_NODE', payload: nodeID });
  }

  function selectScope(scope) {
    dispatch({ type: 'SET_SELECTED_SCOPE', payload: scope });
  }

  if (!sessions.length && !loopRuns.length) {
    return (
      <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>
        No loop runs yet. Click "Start Loop" to begin.
      </div>
    );
  }

  return (
    <div style={{ flex: 1, overflow: 'auto', padding: '4px 0' }}>
      {tree.sortedRuns.map(function (group) {
        var lr = group.loopRun;
        var runKey = lr.id || lr.loop_name;
        var loopNodeID = 'looprun-' + runKey;
        var isRunning = !!STATUS_RUNNING[normalizeStatus(lr.status)];
        var loopRunID = Number(lr && lr.id) || 0;
        var expanded = loopNodeID in expandedNodes ? !!expandedNodes[loopNodeID] : isRunning;
        var loopColor = isRunning ? 'var(--purple)' : 'var(--text-2)';
        var sColor = statusColor(lr.status);
        var daemonSessionID = Number(lr && lr.daemon_session_id) || 0;
        var hasDaemonSession = daemonSessionID > 0;
        var loopScopeID = hasDaemonSession ? 'session-' + daemonSessionID : null;
        var runTurnIDs = uniquePositiveIDs(Array.isArray(lr && lr.turn_ids) ? lr.turn_ids : []);
        runTurnIDs.sort(function (a, b) { return b - a; });

        var daemonSession = group.sessions.find(function (s) { return s.id === daemonSessionID; }) || group.sessions[0] || null;
        var hasTurnRows = hasDaemonSession && runTurnIDs.length > 0;
        var latestTurnID = hasTurnRows ? runTurnIDs[0] : 0;
        var stepByTurn = turnStepByRun[runKey] || {};
        var turnCount = hasTurnRows ? runTurnIDs.length : group.sessions.length;

        var loopSelected = !!(loopScopeID && (selectedScope === loopScopeID || selectedScope === ('session-main-' + daemonSessionID)));
        if (!loopSelected && hasTurnRows) {
          loopSelected = runTurnIDs.some(function (turnID) {
            return selectedScope === ('turn-' + turnID) || selectedScope === ('turn-main-' + turnID);
          });
        }

        var rows = [];
        if (hasTurnRows) {
          runTurnIDs.forEach(function (turnID) {
            var turn = turnsByID[turnID] || null;
            var step = stepByTurn[turnID] || {};
            var daemonStatus = normalizeStatus(daemonSession && daemonSession.status);
            var latestTurn = turnID === latestTurnID;
            var latestDaemonLive = latestTurn && daemonSession && (
              STATUS_RUNNING[daemonStatus] ||
              daemonStatus === 'waiting' ||
              daemonStatus === 'waiting_for_spawns'
            );
            var latestDaemonRunning = latestTurn && daemonSession && STATUS_RUNNING[daemonStatus];
            var startedAt = (turn && turn.date) || (daemonSession && daemonSession.started_at) || lr.started_at || '';
            var endedAt = '';
            if (!latestDaemonRunning && turn && turn.duration_secs > 0) {
              endedAt = addSecondsISO(startedAt, turn.duration_secs);
            }
            if (!endedAt && !latestTurn && daemonSession && !STATUS_RUNNING[normalizeStatus(daemonSession.status)]) {
              endedAt = daemonSession.ended_at || '';
            }
            var displayStatus = '';
            if (latestDaemonLive) {
              displayStatus = daemonSession.status || '';
            }
            if (!displayStatus && turn && turn.build_state) {
              displayStatus = String(turn.build_state);
            }
            if (!displayStatus && latestTurn && daemonSession) {
              displayStatus = daemonSession.status || 'done';
            }
            if (!displayStatus) displayStatus = 'done';

            var rootSpawns = spawnTree.rootsByTurn[turnID] || [];
            if (!rootSpawns.length && runTurnIDs.length === 1) {
              rootSpawns = spawnTree.rootsBySession[daemonSessionID] || [];
            }

            rows.push({
              key: 'turn-' + turnID,
              displayID: turnID,
              scopeMode: 'turn',
              scopeID: turnID,
              stopSessionID: daemonSessionID,
              sessionRole: step.position || '',
              sessionModel: (turn && turn.agent_model) || (daemonSession && daemonSession.model) || '',
              rootSpawns: rootSpawns,
              session: {
                id: daemonSessionID || turnID,
                profile: (turn && turn.profile_name) || step.profile || (daemonSession && daemonSession.profile) || 'unknown',
                agent: (turn && turn.agent) || (daemonSession && daemonSession.agent) || '',
                model: (turn && turn.agent_model) || (daemonSession && daemonSession.model) || '',
                status: displayStatus,
                action: step.position || '',
                started_at: startedAt,
                ended_at: endedAt,
              },
            });
          });
        } else {
          group.sessions.forEach(function (session) {
            var sessionMeta = sessionMetaByID[session.id] || {};
            rows.push({
              key: 'session-' + session.id,
              displayID: session.id,
              scopeMode: 'session',
              scopeID: session.id,
              stopSessionID: session.id,
              sessionRole: sessionMeta.position || '',
              sessionModel: sessionMeta.model || session.model || '',
              rootSpawns: spawnTree.rootsBySession[session.id] || [],
              session: session,
            });
          });
        }

        return (
          <div key={runKey}>
            <div
              onClick={function () {
                if (loopScopeID) {
                  selectScope(loopScopeID);
                  if (!expanded) toggleNode(loopNodeID);
                } else {
                  toggleNode(loopNodeID);
                }
              }}
              style={{
                display: 'flex', alignItems: 'center', gap: 8,
                padding: '8px 12px', cursor: 'pointer',
                background: loopSelected ? 'rgba(123,140,255,0.07)' : 'transparent',
                borderLeft: loopSelected ? '2px solid var(--purple)' : '2px solid transparent',
                borderBottom: '1px solid var(--bg-3)',
                transition: 'all 0.12s ease',
              }}
              onMouseEnter={function (e) { if (!loopSelected) e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { if (!loopSelected) e.currentTarget.style.background = loopSelected ? 'rgba(123,140,255,0.07)' : 'transparent'; }}
            >
              <span
                onClick={function (e) { e.stopPropagation(); toggleNode(loopNodeID); }}
                style={{
                  width: 14, height: 14, display: 'flex', alignItems: 'center', justifyContent: 'center',
                  fontSize: 10, color: loopColor,
                  transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
                }}
              >{'\u25BE'}</span>

              <span style={{
                width: 7, height: 7, borderRadius: '50%', background: sColor, flexShrink: 0,
                boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
                animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
              }} />

              {isRunning ? (
                <span style={{ color: loopColor, fontSize: 13, animation: 'spin 2s linear infinite', display: 'inline-block', flexShrink: 0 }}>{'\u21BB'}</span>
              ) : (
                <span style={{ color: loopColor, fontSize: 13, flexShrink: 0 }}>{'\u21BB'}</span>
              )}

              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 700, color: 'var(--text-0)' }}>
                    {lr.loop_name || 'loop'}
                  </span>
                  {lr.hex_id && (
                    <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
                      {lr.hex_id.slice(0, 8)}
                    </span>
                  )}
                  {isRunning && (
                    <span style={{
                      padding: '1px 5px', background: 'rgba(123,140,255,0.12)', border: '1px solid rgba(123,140,255,0.25)',
                      borderRadius: 3, fontFamily: "'JetBrains Mono', monospace", fontSize: 8, color: 'var(--purple)', fontWeight: 600,
                    }}>RUNNING</span>
                  )}
                </div>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 1 }}>
                  {turnCount} turns
                  {lr.cycle > 0 ? ' \u00B7 cycle ' + (lr.cycle + 1) : ''}
                  {' \u00B7 '}
                  {formatElapsed(lr.started_at, lr.stopped_at)}
                </div>
              </div>

              {isRunning && loopRunID > 0 && (
                <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                  <WindDownLoopButton runID={loopRunID} />
                  <StopLoopButton runID={loopRunID} />
                </div>
              )}
            </div>

            {expanded && rows.map(function (row) {
              return (
                <TurnNode
                  key={row.key}
                  session={row.session}
                  displayID={row.displayID}
                  scopeMode={row.scopeMode}
                  scopeID={row.scopeID}
                  stopSessionID={row.stopSessionID}
                  sessionRole={row.sessionRole}
                  sessionModel={row.sessionModel}
                  selectedScope={selectedScope}
                  expandedNodes={expandedNodes}
                  rootSpawns={row.rootSpawns}
                  childrenByParent={spawnTree.childrenByParent}
                  onSelect={selectScope}
                  onToggle={toggleNode}
                />
              );
            })}
          </div>
        );
      })}

      {tree.standaloneSessions.length > 0 && (
        <div>
          {tree.sortedRuns.length > 0 && (
            <div style={{
              padding: '6px 12px', fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.08em',
              borderBottom: '1px solid var(--bg-3)', borderTop: '1px solid var(--bg-3)',
            }}>Standalone</div>
          )}
          {tree.standaloneSessions.map(function (session) {
            var sessionMeta = sessionMetaByID[session.id] || {};
            return (
              <TurnNode
                key={session.id}
                session={session}
                sessionRole={sessionMeta.position || ''}
                sessionModel={sessionMeta.model || session.model || ''}
                selectedScope={selectedScope}
                expandedNodes={expandedNodes}
                rootSpawns={spawnTree.rootsBySession[session.id] || []}
                childrenByParent={spawnTree.childrenByParent}
                onSelect={selectScope}
                onToggle={toggleNode}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}

function TurnNode({ session, displayID, scopeMode, scopeID, stopSessionID, sessionRole, sessionModel, selectedScope, expandedNodes, rootSpawns, childrenByParent, onSelect, onToggle }) {
  var effectiveMode = scopeMode === 'turn' ? 'turn' : 'session';
  var effectiveScopeID = Number(scopeID || session.id) || Number(session.id) || 0;
  var effectiveDisplayID = Number(displayID || session.id) || Number(session.id) || 0;
  var sessionNodeID = effectiveMode === 'turn' ? ('turn-' + effectiveScopeID) : ('session-' + effectiveScopeID);
  var mainScopeID = effectiveMode === 'turn' ? ('turn-main-' + effectiveScopeID) : ('session-main-' + effectiveScopeID);
  var selectedAll = selectedScope === sessionNodeID;
  var selectedMain = selectedScope === mainScopeID;
  var selected = selectedAll || selectedMain;
  var status = normalizeStatus(session.status);
  var isRunning = !!STATUS_RUNNING[status];
  var isWaiting = status === 'waiting' || status === 'waiting_for_spawns';
  var sColor = statusColor(session.status);
  var info = agentInfo(session.agent);
  var roleText = String(sessionRole || session.action || '').trim();
  var role = turnRole(roleText);
  var hasSpawns = rootSpawns.length > 0;
  var turnExpandKey = 'turn-' + effectiveMode + '-' + effectiveDisplayID;
  var expanded = turnExpandKey in expandedNodes ? !!expandedNodes[turnExpandKey] : isRunning;
  var rowIcon = role.icon || info.icon;
  var rowIconColor = role.color || info.color;

  return (
    <div>
      <div
        onClick={function () { onSelect(hasSpawns ? mainScopeID : sessionNodeID); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '5px 12px 5px 28px', cursor: 'pointer',
          background: selected ? (info.color + '10') : 'transparent',
          borderLeft: selected ? ('2px solid ' + info.color) : '2px solid transparent',
          transition: 'all 0.12s ease',
        }}
        onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
      >
        {hasSpawns ? (
          <span
            onClick={function (e) { e.stopPropagation(); onToggle(turnExpandKey); }}
            style={{
              width: 14, height: 14, display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 9, color: info.color, border: '1px solid ' + info.color + '40',
              borderRadius: 3, cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace",
              transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
            }}
          >{'\u25BE'}</span>
        ) : (
          <span style={{ width: 14 }} />
        )}

        <span style={{
          width: 6, height: 6, borderRadius: '50%', background: sColor, flexShrink: 0,
          boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
          animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />

        <span style={{ color: rowIconColor, fontSize: 10, flexShrink: 0 }}>{rowIcon}</span>

        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{session.profile || 'unknown'}</span>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>#{effectiveDisplayID || session.id}</span>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
              {formatElapsed(session.started_at, session.ended_at)}
            </span>
            {isWaiting && (
              <span style={{
                padding: '1px 5px',
                borderRadius: 3,
                border: '1px solid rgba(249, 226, 175, 0.25)',
                background: 'rgba(249, 226, 175, 0.08)',
                color: 'rgb(249, 226, 175)',
                fontFamily: "'JetBrains Mono', monospace",
                fontSize: 8,
                fontWeight: 700,
                letterSpacing: '0.05em',
              }}>WAITING</span>
            )}
          </div>
          {(roleText || session.agent || sessionModel) && (
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 1 }}>
              {roleText ? (
                <span style={{ color: role.color || 'var(--text-2)', fontWeight: 600 }}>
                  {(role.icon ? role.icon + ' ' : '') + roleText}
                </span>
              ) : null}
              {session.agent ? (
                <span style={{ color: 'var(--text-3)' }}>
                  {(roleText ? ' \u00B7 ' : '') + session.agent}
                </span>
              ) : null}
              {sessionModel ? (
                <span style={{ color: 'var(--text-3)' }}>
                  {((roleText || session.agent) ? ' \u00B7 ' : '') + sessionModel}
                </span>
              ) : null}
            </div>
          )}
        </div>

        {hasSpawns && (
          <span
            onClick={function (e) { e.stopPropagation(); }}
            title="Output scope"
            style={{
              display: 'inline-flex', alignItems: 'center', gap: 2,
              border: '1px solid var(--border)', borderRadius: 4,
              background: 'var(--bg-2)', padding: 1,
            }}
          >
            <button
              type="button"
              onClick={function (e) { e.stopPropagation(); onSelect(mainScopeID); }}
              style={{
                border: 'none', cursor: 'pointer',
                fontFamily: "'JetBrains Mono', monospace", fontSize: 8, fontWeight: 700,
                color: selectedMain ? 'var(--accent)' : 'var(--text-3)',
                background: selectedMain ? 'var(--accent)16' : 'transparent',
                borderRadius: 3, padding: '1px 4px', letterSpacing: '0.04em',
              }}
            >MAIN</button>
            <button
              type="button"
              onClick={function (e) { e.stopPropagation(); onSelect(sessionNodeID); }}
              style={{
                border: 'none', cursor: 'pointer',
                fontFamily: "'JetBrains Mono', monospace", fontSize: 8, fontWeight: 700,
                color: selectedAll ? 'var(--accent)' : 'var(--text-3)',
                background: selectedAll ? 'var(--accent)16' : 'transparent',
                borderRadius: 3, padding: '1px 4px', letterSpacing: '0.04em',
              }}
            >ALL</button>
          </span>
        )}

        {hasSpawns && (
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)',
            padding: '1px 5px', background: 'var(--bg-3)', borderRadius: 3,
          }}>{rootSpawns.length} spawn{rootSpawns.length !== 1 ? 's' : ''}</span>
        )}

        {isRunning && (
          <>
            <StopSessionButton sessionID={stopSessionID || session.id} />
            <span style={{
              width: 8, height: 8, border: '1.5px solid ' + info.color, borderTopColor: 'transparent',
              borderRadius: '50%', animation: 'spin 1s linear infinite', flexShrink: 0,
            }} />
          </>
        )}
      </div>

      {hasSpawns && expanded && rootSpawns.map(function (spawn) {
        return (
          <SpawnNode
            key={spawn.id}
            spawn={spawn}
            depth={0}
            childrenByParent={childrenByParent}
            selectedScope={selectedScope}
            expandedNodes={expandedNodes}
            onSelect={onSelect}
            onToggle={onToggle}
          />
        );
      })}
    </div>
  );
}

function turnRole(raw) {
  var key = String(raw || '').trim().toLowerCase();
  if (!key) return { icon: '', color: '' };
  if (key === 'manager') return { icon: '\u25C6', color: 'rgb(74, 230, 138)' };
  if (key === 'supervisor') return { icon: '\u2B22', color: 'rgb(249, 226, 175)' };
  if (key === 'lead') return { icon: '\u25B2', color: 'rgb(137, 180, 250)' };
  return { icon: '\u25CF', color: 'var(--text-2)' };
}

function stepTurns(step) {
  var count = Number(step && step.turns) || 1;
  return count > 0 ? count : 1;
}

function addSecondsISO(ts, seconds) {
  var base = parseTimestamp(ts);
  var delta = Number(seconds) || 0;
  if (!(base > 0) || delta <= 0) return '';
  return new Date(base + delta * 1000).toISOString();
}

function uniquePositiveIDs(ids) {
  var seen = {};
  var out = [];
  (Array.isArray(ids) ? ids : []).forEach(function (id) {
    var n = Number(id) || 0;
    if (n <= 0 || seen[n]) return;
    seen[n] = true;
    out.push(n);
  });
  return out;
}

function SpawnNode({ spawn, depth, childrenByParent, selectedScope, expandedNodes, onSelect, onToggle }) {
  var nodeID = 'spawn-' + spawn.id;
  var selected = selectedScope === nodeID;
  var children = childrenByParent[spawn.id] || [];
  var expanded = !!expandedNodes[nodeID];
  var status = normalizeStatus(spawn.status);
  var isRunning = !!STATUS_RUNNING[status];
  var sColor = statusColor(spawn.status);
  var hasPendingQuestion = status === 'awaiting_input' && !!spawn.question;
  var leftPad = 46 + depth * 18;

  return (
    <div>
      <div
        onClick={function () { onSelect(nodeID); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '4px 12px 4px ' + leftPad + 'px', cursor: 'pointer',
          background: selected ? (sColor + '12') : 'transparent',
          borderLeft: selected ? ('2px solid ' + sColor) : '2px solid transparent',
          transition: 'all 0.15s ease',
        }}
        onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
      >
        {children.length > 0 ? (
          <span
            onClick={function (e) { e.stopPropagation(); onToggle(nodeID); }}
            style={{
              width: 14, height: 14, display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 9, color: sColor, border: '1px solid ' + sColor + '40',
              borderRadius: 3, cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace",
              transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
            }}
          >{'\u25BE'}</span>
        ) : (
          <span style={{ width: 14, height: 14, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <span style={{ width: 4, height: 4, borderRadius: '50%', background: sColor + '60' }} />
          </span>
        )}

        <span style={{
          width: 6, height: 6, borderRadius: '50%', background: sColor, flexShrink: 0,
          boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
          animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />

        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>#{spawn.id}</span>
            <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{spawn.profile || 'spawn'}</span>
            {spawn.role && <span style={{ fontSize: 10, color: 'var(--text-3)' }}>as {spawn.role}</span>}
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
              {formatElapsed(spawn.started_at, spawn.completed_at)}
            </span>
          </div>
          {spawn.task && (
            <div style={{ fontSize: 10, color: 'var(--text-2)', marginTop: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
              {spawn.task}
            </div>
          )}
          {spawn.branch && (
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--purple)', marginTop: 1 }}>
              {spawn.branch}
            </div>
          )}
          {hasPendingQuestion && (
            <div style={{
              marginTop: 4, padding: '4px 8px', background: 'rgba(232,168,56,0.08)',
              border: '1px solid rgba(232,168,56,0.2)', borderRadius: 4,
              display: 'flex', alignItems: 'center', gap: 6,
            }}>
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--orange)', animation: 'pulse 1.5s ease-in-out infinite' }} />
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--orange)' }}>AWAITING RESPONSE</span>
            </div>
          )}
        </div>

        {isRunning && (
          <span style={{
            width: 10, height: 10, border: '2px solid ' + sColor, borderTopColor: 'transparent',
            borderRadius: '50%', animation: 'spin 1s linear infinite', flexShrink: 0,
          }} />
        )}
      </div>

      {expanded && children.map(function (child) {
        return (
          <SpawnNode
            key={child.id}
            spawn={child}
            depth={depth + 1}
            childrenByParent={childrenByParent}
            selectedScope={selectedScope}
            expandedNodes={expandedNodes}
            onSelect={onSelect}
            onToggle={onToggle}
          />
        );
      })}
    </div>
  );
}
