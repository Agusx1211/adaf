import { useEffect, useMemo, useRef, useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { agentInfo, statusColor, STATUS_RUNNING } from '../../utils/colors.js';
import { buildSpawnScopeMaps } from '../../utils/scopes.js';
import { cropText, formatElapsed, parseTimestamp } from '../../utils/format.js';

var LIVE_STATUSES = {
  running: true,
  active: true,
  in_progress: true,
  ongoing: true,
  waiting: true,
  waiting_for_spawns: true,
  awaiting_input: true,
  spawning: true,
};

var EDGE_STYLES = {
  turn: { color: 'rgba(167, 139, 250, 0.52)', width: 1.8, dash: [5, 5] },
  spawn: { color: 'rgba(91, 206, 252, 0.52)', width: 1.4, dash: [] },
};

var NODE_DRAW_ORDER = {
  spawn: 0,
  turn: 1,
  loop: 2,
};

export default function LoopVisualizer() {
  var state = useAppState();
  var dispatch = useDispatch();
  var loop = state.loopRun;

  if (!loop) {
    return (
      <div style={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 12,
        color: 'var(--text-3)',
      }}>
        <span style={{ fontSize: 32, opacity: 0.32 }}>{'\u25C8'}</span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12 }}>No active loop run</span>
      </div>
    );
  }

  var snapshot = useMemo(function () {
    return buildGraphSnapshot(loop, state.loopRuns, state.sessions, state.turns, state.spawns, state.activity);
  }, [loop, state.loopRuns, state.sessions, state.turns, state.spawns, state.activity]);

  var containerRef = useRef(null);
  var canvasRef = useRef(null);
  var simRef = useRef(makeEmptySimulation());
  var frameRef = useRef(0);
  var dprRef = useRef(1);
  var sizeRef = useRef({ w: 0, h: 0 });
  var cameraRef = useRef({ x: 0, y: 0, z: 0.82 });
  var hoverNodeRef = useRef('');
  var pointerRef = useRef({
    active: false,
    pointerID: 0,
    startX: 0,
    startY: 0,
    startCamX: 0,
    startCamY: 0,
    moved: false,
    hitNodeID: '',
  });

  var [selectedNodeID, setSelectedNodeID] = useState(snapshot.rootID);
  var [hoverNodeID, setHoverNodeID] = useState('');

  var runKeyRef = useRef('');
  useEffect(function () {
    if (runKeyRef.current === snapshot.runKey) return;
    runKeyRef.current = snapshot.runKey;
    setSelectedNodeID(snapshot.rootID);
    setHoverNodeID('');
    cameraRef.current = { x: 0, y: 0, z: 0.82 };
  }, [snapshot.runKey, snapshot.rootID]);

  useEffect(function () {
    syncSimulation(simRef.current, snapshot);
  }, [snapshot]);

  useEffect(function () {
    var scope = String(state.selectedScope || '');
    if (!scope) return;
    var nodeID = snapshot.scopeToNodeID[scope] || '';
    if (!nodeID) return;
    setSelectedNodeID(function (prev) { return prev === nodeID ? prev : nodeID; });
  }, [state.selectedScope, snapshot.scopeToNodeID]);

  useEffect(function () {
    if (snapshot.nodesByID[selectedNodeID]) return;
    setSelectedNodeID(snapshot.rootID);
  }, [selectedNodeID, snapshot.nodesByID, snapshot.rootID]);

  useEffect(function () {
    function drawFrame(now) {
      frameRef.current = requestAnimationFrame(drawFrame);
      drawScene(canvasRef.current, containerRef.current, simRef.current, cameraRef.current, {
        selectedNodeID: selectedNodeID,
        hoverNodeID: hoverNodeRef.current,
      }, now);
      stepSimulation(simRef.current);
    }

    frameRef.current = requestAnimationFrame(drawFrame);
    return function () {
      if (frameRef.current) cancelAnimationFrame(frameRef.current);
      frameRef.current = 0;
    };
  }, [selectedNodeID]);

  useEffect(function () {
    function onResize() {
      resizeCanvas(canvasRef.current, containerRef.current, sizeRef, dprRef);
    }

    onResize();
    var observer = null;
    if (containerRef.current && typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(onResize);
      observer.observe(containerRef.current);
    }

    window.addEventListener('resize', onResize);
    return function () {
      window.removeEventListener('resize', onResize);
      if (observer) observer.disconnect();
    };
  }, []);

  var selectedNode = snapshot.nodesByID[selectedNodeID] || snapshot.nodesByID[snapshot.rootID] || null;
  var selectedEvents = useMemo(function () {
    if (!selectedNode) return [];

    var scopes = {};
    if (selectedNode.scope) scopes[selectedNode.scope] = true;
    if (selectedNode.type === 'turn' && snapshot.sessionScope) scopes[snapshot.sessionScope] = true;

    return snapshot.events.filter(function (entry) {
      return !!scopes[entry.scope];
    }).slice(0, 12);
  }, [selectedNode, snapshot.events, snapshot.sessionScope]);

  function selectNode(node) {
    if (!node) return;
    setSelectedNodeID(node.id);
    hoverNodeRef.current = '';
    setHoverNodeID('');
    if (node.scope) {
      dispatch({ type: 'SET_SELECTED_SCOPE', payload: node.scope });
    }
  }

  function focusNode(nodeID) {
    var node = snapshot.nodesByID[nodeID];
    if (!node) return;
    selectNode(node);
  }

  function onPointerDown(event) {
    if (event.button !== 0) return;

    var world = screenToWorld(event.clientX, event.clientY, containerRef.current, cameraRef.current);
    var hitNodeID = hitNode(simRef.current, world.x, world.y) || '';

    pointerRef.current = {
      active: true,
      pointerID: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      startCamX: cameraRef.current.x,
      startCamY: cameraRef.current.y,
      moved: false,
      hitNodeID: hitNodeID,
    };

    try { event.currentTarget.setPointerCapture(event.pointerId); } catch (_) {}
  }

  function onPointerMove(event) {
    var world = screenToWorld(event.clientX, event.clientY, containerRef.current, cameraRef.current);

    if (!pointerRef.current.active) {
      var hoverID = hitNode(simRef.current, world.x, world.y) || '';
      hoverNodeRef.current = hoverID;
      setHoverNodeID(function (prev) { return prev === hoverID ? prev : hoverID; });
      return;
    }

    if (pointerRef.current.pointerID !== event.pointerId) return;

    var dx = event.clientX - pointerRef.current.startX;
    var dy = event.clientY - pointerRef.current.startY;

    if (Math.abs(dx) > 3 || Math.abs(dy) > 3) pointerRef.current.moved = true;

    if (pointerRef.current.moved) {
      cameraRef.current = {
        x: pointerRef.current.startCamX - dx / cameraRef.current.z,
        y: pointerRef.current.startCamY - dy / cameraRef.current.z,
        z: cameraRef.current.z,
      };
    }
  }

  function onPointerUp(event) {
    if (!pointerRef.current.active) return;
    if (pointerRef.current.pointerID !== event.pointerId) return;

    try { event.currentTarget.releasePointerCapture(event.pointerId); } catch (_) {}

    var wasMoved = pointerRef.current.moved;
    var hitNodeID = pointerRef.current.hitNodeID;

    pointerRef.current.active = false;

    if (!wasMoved && hitNodeID) {
      focusNode(hitNodeID);
    }
  }

  function onWheel(event) {
    event.preventDefault();

    var before = screenToWorld(event.clientX, event.clientY, containerRef.current, cameraRef.current);
    var factor = event.deltaY > 0 ? 0.9 : 1.1;
    var nextZoom = clamp(cameraRef.current.z * factor, 0.2, 2.8);

    var nextCamera = { x: cameraRef.current.x, y: cameraRef.current.y, z: nextZoom };
    var after = screenToWorld(event.clientX, event.clientY, containerRef.current, nextCamera);

    cameraRef.current = {
      x: cameraRef.current.x + (before.x - after.x),
      y: cameraRef.current.y + (before.y - after.y),
      z: nextZoom,
    };
  }

  function resetView() {
    cameraRef.current = { x: 0, y: 0, z: 0.82 };
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-0)' }} data-testid="session-graph-view">
      <div style={hudStyle()}>
        <div style={hudLogoStyle()}>ADAF</div>
        <div style={hudSepStyle()} />
        <HudPill label="active" value={String(snapshot.activeCount)} accent="var(--green)" pulse={snapshot.activeCount > 0} />
        <HudPill label="nodes" value={String(snapshot.nodes.length)} />
        <HudPill label="turns" value={String(snapshot.turnCount)} />
        <HudPill label="spawns" value={String(snapshot.spawnCount)} />
        <HudPill label="loop" value={snapshot.loopName} />
        <HudPill label="step" value={snapshot.stepLabel} />
        <HudPill label="elapsed" value={snapshot.elapsedLabel} />
        <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 8 }}>
          <button type="button" onClick={function () { cameraRef.current.z = clamp(cameraRef.current.z * 0.9, 0.2, 2.8); }} style={controlButtonStyle()} title="Zoom out">-</button>
          <button type="button" onClick={function () { cameraRef.current.z = clamp(cameraRef.current.z * 1.1, 0.2, 2.8); }} style={controlButtonStyle()} title="Zoom in">+</button>
          <button type="button" onClick={resetView} style={controlButtonStyle()} title="Reset pan and zoom">reset</button>
        </div>
      </div>

      <div ref={containerRef} style={surfaceStyle()}>
        <canvas
          ref={canvasRef}
          style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', cursor: pointerRef.current.active ? 'grabbing' : (hoverNodeID ? 'pointer' : 'grab') }}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerUp}
          onWheel={onWheel}
        />

        <div style={eventLogShellStyle()}>
          <div style={sectionHeadStyle()}>Event Log</div>
          <div style={{ flex: 1, overflow: 'auto', padding: '6px 8px' }} data-testid="session-graph-event-log">
            {snapshot.events.length === 0 ? (
              <div style={{ marginTop: 10, fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', textAlign: 'center' }}>No events yet.</div>
            ) : snapshot.events.map(function (entry) {
              var mappedNodeID = snapshot.scopeToNodeID[entry.scope] || '';
              var selected = mappedNodeID && mappedNodeID === selectedNodeID;
              return (
                <button
                  key={entry.id}
                  type="button"
                  onClick={function () { if (mappedNodeID) focusNode(mappedNodeID); }}
                  style={{
                    width: '100%',
                    border: 'none',
                    background: selected ? 'rgba(91,206,252,0.12)' : 'transparent',
                    color: 'inherit',
                    borderRadius: 6,
                    textAlign: 'left',
                    padding: '5px 6px',
                    marginBottom: 2,
                    cursor: mappedNodeID ? 'pointer' : 'default',
                    display: 'grid',
                    gridTemplateColumns: '6px 40px 1fr',
                    gap: 8,
                    alignItems: 'start',
                  }}
                >
                  <span style={{ width: 6, height: 6, borderRadius: '50%', marginTop: 4, background: entry.color }} />
                  <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>{entry.clock}</span>
                  <span style={{ fontSize: 10, lineHeight: 1.42, color: 'var(--text-2)' }}>{entry.text}</span>
                </button>
              );
            })}
          </div>
        </div>

        <div style={detailShellStyle()}>
          {!selectedNode ? (
            <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)', fontSize: 11 }}>Select a node</div>
          ) : (
            <>
              <div style={detailHeadStyle()}>
                <div style={{
                  width: 36,
                  height: 36,
                  borderRadius: 8,
                  background: agentInfo(selectedNode.agent).bg,
                  border: '1px solid ' + agentInfo(selectedNode.agent).color + '66',
                  color: agentInfo(selectedNode.agent).color,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontFamily: "'Outfit', sans-serif",
                  fontSize: 16,
                  fontWeight: 700,
                }}>{selectedNode.type === 'turn' ? 'T' : selectedNode.icon}</div>
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div style={{ fontFamily: "'Outfit', sans-serif", fontSize: 14, fontWeight: 600, color: 'var(--text-0)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{selectedNode.label}</div>
                  <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>{selectedNode.type} {selectedNode.idNumber > 0 ? ('#' + selectedNode.idNumber) : ''}</div>
                </div>
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace",
                  fontSize: 9,
                  color: statusColor(selectedNode.status),
                  border: '1px solid ' + statusColor(selectedNode.status) + '4d',
                  background: statusColor(selectedNode.status) + '1a',
                  borderRadius: 4,
                  padding: '2px 6px',
                  letterSpacing: '0.06em',
                  textTransform: 'uppercase',
                  fontWeight: 700,
                }}>{selectedNode.status}</span>
              </div>

              <div style={{ flex: 1, overflow: 'auto', padding: 12 }}>
                <DetailRow label="scope" value={selectedNode.scope || '--'} mono />
                <DetailRow label="agent" value={selectedNode.agent || '--'} />
                <DetailRow label="model" value={selectedNode.model || '--'} />
                <DetailRow label="role" value={selectedNode.role || '--'} />
                <DetailRow label="elapsed" value={selectedNode.startedAt ? formatElapsed(selectedNode.startedAt, selectedNode.endedAt) : '--'} mono />

                {(selectedNode.task || selectedNode.summary || selectedNode.question) && (
                  <div style={{ marginTop: 12 }}>
                    <div style={smallSectionTitleStyle()}>Context</div>
                    {selectedNode.task ? <InfoBox text={selectedNode.task} /> : null}
                    {selectedNode.summary ? <InfoBox text={selectedNode.summary} /> : null}
                    {selectedNode.question ? <InfoBox text={'Q: ' + selectedNode.question} /> : null}
                  </div>
                )}

                {selectedEvents.length > 0 && (
                  <div style={{ marginTop: 12 }}>
                    <div style={smallSectionTitleStyle()}>Latest Activity</div>
                    {selectedEvents.map(function (entry) {
                      return (
                        <div key={entry.id} style={{
                          display: 'grid',
                          gridTemplateColumns: '40px 1fr',
                          gap: 8,
                          fontSize: 10,
                          color: 'var(--text-2)',
                          padding: '4px 0',
                          borderBottom: '1px solid rgba(255,255,255,0.04)',
                        }}>
                          <span style={{ fontFamily: "'JetBrains Mono', monospace", color: 'var(--text-3)' }}>{entry.clock}</span>
                          <span>{entry.text}</span>
                        </div>
                      );
                    })}
                  </div>
                )}

                {selectedNode.children && selectedNode.children.length > 0 && (
                  <div style={{ marginTop: 12 }}>
                    <div style={smallSectionTitleStyle()}>Children ({selectedNode.children.length})</div>
                    {selectedNode.children.map(function (childID) {
                      var child = snapshot.nodesByID[childID];
                      if (!child) return null;
                      return (
                        <button
                          key={child.id}
                          type="button"
                          data-testid={'graph-child-' + child.id}
                          onClick={function () { focusNode(child.id); }}
                          style={{
                            width: '100%',
                            border: 'none',
                            borderRadius: 6,
                            background: selectedNodeID === child.id ? 'rgba(123,140,255,0.12)' : 'rgba(255,255,255,0.03)',
                            color: 'inherit',
                            textAlign: 'left',
                            padding: '6px 8px',
                            marginBottom: 4,
                            cursor: 'pointer',
                            display: 'grid',
                            gridTemplateColumns: '16px 1fr auto',
                            gap: 7,
                            alignItems: 'center',
                          }}
                        >
                          <span style={{ color: agentInfo(child.agent).color, fontFamily: "'JetBrains Mono', monospace", fontSize: 11 }}>{child.icon}</span>
                          <span style={{ fontSize: 11, color: 'var(--text-1)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{child.label}</span>
                          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: statusColor(child.status) }}>{child.status}</span>
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>
            </>
          )}
        </div>

        <div style={timelineStyle()}>
          <div style={{
            position: 'relative',
            height: 16,
            borderTop: '1px solid rgba(255,255,255,0.05)',
            borderBottom: '1px solid rgba(255,255,255,0.05)',
            background: 'rgba(8,10,14,0.62)',
            borderRadius: 4,
            overflow: 'hidden',
          }}>
            {snapshot.timelineTicks.map(function (tick) {
              return (
                <button
                  key={tick.id}
                  type="button"
                  onClick={function () {
                    var nodeID = snapshot.scopeToNodeID[tick.scope] || '';
                    if (nodeID) focusNode(nodeID);
                  }}
                  style={{
                    position: 'absolute',
                    left: tick.left + '%',
                    top: 1,
                    bottom: 1,
                    width: 2,
                    border: 'none',
                    background: tick.color,
                    opacity: 0.72,
                    cursor: 'pointer',
                    padding: 0,
                  }}
                  title={tick.title}
                />
              );
            })}
          </div>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            marginTop: 4,
            color: 'var(--text-3)',
            fontFamily: "'JetBrains Mono', monospace",
            fontSize: 9,
          }}>
            <LegendDot color={agentInfo('claude').color} text="Claude" />
            <LegendDot color={agentInfo('codex').color} text="Codex" />
            <LegendDot color={agentInfo('gemini').color} text="Gemini" />
            <LegendDot color={agentInfo('opencode').color} text="OpenCode" />
            <LegendLine color={EDGE_STYLES.spawn.color} text="spawn" />
            <LegendLine color={EDGE_STYLES.turn.color} text="turn" dashed />
            <span style={{ marginLeft: 'auto' }}>scroll zoom | drag pan | click inspect</span>
          </div>
        </div>
      </div>
    </div>
  );
}

function buildGraphSnapshot(loopRun, loopRuns, sessions, turns, spawns, activity) {
  var run = loopRun || {};
  var runID = Number(run.id) || 0;
  var runKey = runID > 0 ? String(runID) : String(run.hex_id || run.loop_name || 'loop');

  var sessionByID = {};
  (Array.isArray(sessions) ? sessions : []).forEach(function (session) {
    if (!session || session.id <= 0) return;
    sessionByID[session.id] = session;
  });

  var turnByID = {};
  (Array.isArray(turns) ? turns : []).forEach(function (turn) {
    if (!turn || turn.id <= 0) return;
    turnByID[turn.id] = turn;
  });

  var runTurnIDs = uniquePositiveIDs(run.turn_ids).sort(function (a, b) { return a - b; });
  var runTurnSet = {};
  runTurnIDs.forEach(function (turnID) { runTurnSet[turnID] = true; });

  var scopeMaps = buildSpawnScopeMaps(spawns, loopRuns);

  var daemonSessionID = Number(run.daemon_session_id) || 0;
  if (daemonSessionID <= 0) {
    runTurnIDs.some(function (turnID) {
      var sid = Number(scopeMaps.turnToSession && scopeMaps.turnToSession[turnID]) || 0;
      if (sid > 0) {
        daemonSessionID = sid;
        return true;
      }
      return false;
    });
  }

  if (daemonSessionID <= 0) {
    var loopName = String(run.loop_name || '').trim();
    var candidates = (Array.isArray(sessions) ? sessions : []).filter(function (session) {
      if (!session || session.id <= 0) return false;
      if (!loopName) return false;
      return String(session.loop_name || '').trim() === loopName;
    });

    if (candidates.length > 0) {
      candidates.sort(function (a, b) {
        var aRunning = isLiveStatus(a.status) ? 1 : 0;
        var bRunning = isLiveStatus(b.status) ? 1 : 0;
        if (aRunning !== bRunning) return bRunning - aRunning;

        var aTS = parseTimestamp(a.started_at);
        var bTS = parseTimestamp(b.started_at);
        if (aTS !== bTS) return bTS - aTS;

        return (Number(b.id) || 0) - (Number(a.id) || 0);
      });
      daemonSessionID = Number(candidates[0] && candidates[0].id) || 0;
    }
  }

  if (daemonSessionID <= 0) {
    var daemonCounts = {};
    (Array.isArray(spawns) ? spawns : []).forEach(function (spawn) {
      if (!spawnBelongsToRun(spawn, runTurnSet, 0)) return;
      var parentSID = Number(spawn && spawn.parent_daemon_session_id) || 0;
      var childSID = Number(spawn && spawn.child_daemon_session_id) || 0;
      if (parentSID > 0) daemonCounts[parentSID] = (daemonCounts[parentSID] || 0) + 1;
      if (childSID > 0) daemonCounts[childSID] = (daemonCounts[childSID] || 0) + 1;
    });

    var bestSID = 0;
    var bestCount = 0;
    Object.keys(daemonCounts).forEach(function (key) {
      var sid = Number(key) || 0;
      var count = Number(daemonCounts[key]) || 0;
      if (sid <= 0) return;
      if (count > bestCount) {
        bestSID = sid;
        bestCount = count;
      }
    });
    if (bestSID > 0) daemonSessionID = bestSID;
  }

  var daemonSession = daemonSessionID > 0 ? (sessionByID[daemonSessionID] || null) : null;
  var sessionScope = daemonSessionID > 0 ? ('session-' + daemonSessionID) : '';

  var steps = Array.isArray(run.steps) ? run.steps : [];
  var rootID = 'loop-' + runKey;

  var nodes = [];
  var edges = [];
  var nodesByID = {};
  var scopeToNodeID = {};

  function addNode(node) {
    nodes.push(node);
    nodesByID[node.id] = node;
    if (node.scope) scopeToNodeID[node.scope] = node.id;
  }

  function addEdge(from, to, type) {
    edges.push({ id: from + '::' + to, from: from, to: to, type: type });
  }

  var rootStatus = normalizeStatus(run.status || (daemonSession && daemonSession.status) || 'active');
  addNode({
    id: rootID,
    idNumber: runID,
    type: 'loop',
    label: String(run.loop_name || 'loop'),
    icon: '\u21BB',
    status: rootStatus,
    agent: 'generic',
    model: daemonSession && daemonSession.model ? String(daemonSession.model) : 'loop',
    role: 'orchestrator',
    scope: sessionScope,
    startedAt: run.started_at || (daemonSession && daemonSession.started_at) || '',
    endedAt: run.stopped_at || (daemonSession && daemonSession.ended_at) || '',
    task: '',
    summary: '',
    question: '',
    order: 0,
    parent: '',
    children: [],
  });

  if (sessionScope) {
    scopeToNodeID['session-main-' + daemonSessionID] = rootID;
  }

  var turnStepMap = mapRunTurnsToSteps(run, runTurnIDs, turnByID);
  var latestTurnID = runTurnIDs.length ? runTurnIDs[runTurnIDs.length - 1] : 0;
  var turnNodeByTurnID = {};

  runTurnIDs.forEach(function (turnID, idx) {
    var turn = turnByID[turnID] || null;
    var mappedStep = turnStepMap[turnID] || {};

    var turnStatus = normalizeTurnNodeStatus(turn, daemonSession, turnID === latestTurnID);
    var turnLabel = turn && turn.profile_name
      ? String(turn.profile_name)
      : (mappedStep.profile ? String(mappedStep.profile) : ('turn-' + turnID));

    var turnNodeID = 'turn-' + turnID;
    addNode({
      id: turnNodeID,
      idNumber: turnID,
      type: 'turn',
      label: turnLabel,
      icon: 'T' + (idx + 1),
      status: turnStatus,
      agent: turn && turn.agent ? String(turn.agent) : (daemonSession && daemonSession.agent ? String(daemonSession.agent) : 'generic'),
      model: turn && turn.agent_model ? String(turn.agent_model) : (daemonSession && daemonSession.model ? String(daemonSession.model) : ''),
      role: mappedStep.position || '',
      scope: 'turn-' + turnID,
      startedAt: (turn && turn.date) || (daemonSession && daemonSession.started_at) || run.started_at || '',
      endedAt: turn && Number(turn.duration_secs) > 0 ? addSecondsISO((turn && turn.date) || (daemonSession && daemonSession.started_at) || run.started_at || '', Number(turn.duration_secs)) : '',
      task: turn && turn.objective ? String(turn.objective) : '',
      summary: turn && turn.current_state ? String(turn.current_state) : '',
      question: '',
      order: idx + 1,
      parent: rootID,
      children: [],
    });

    scopeToNodeID['turn-main-' + turnID] = turnNodeID;
    turnNodeByTurnID[turnID] = turnNodeID;
    addEdge(rootID, turnNodeID, 'turn');
  });

  var runSpawns = (Array.isArray(spawns) ? spawns : []).filter(function (spawn) {
    return spawnBelongsToRun(spawn, runTurnSet, daemonSessionID);
  }).slice().sort(compareSpawn);

  var spawnSet = {};
  runSpawns.forEach(function (spawn) {
    if (!spawn || spawn.id <= 0) return;
    spawnSet[spawn.id] = true;
  });

  var pending = runSpawns.slice();
  var guard = pending.length + 2;
  for (var pass = 0; pass < guard && pending.length > 0; pass++) {
    var next = [];
    pending.forEach(function (spawn) {
      if (!spawn || spawn.id <= 0) return;
      var nodeID = 'spawn-' + spawn.id;

      if (nodesByID[nodeID]) return;

      var parentID = '';
      if (spawn.parent_spawn_id > 0 && spawnSet[spawn.parent_spawn_id]) {
        parentID = 'spawn-' + spawn.parent_spawn_id;
        if (!nodesByID[parentID]) {
          next.push(spawn);
          return;
        }
      } else if (spawn.parent_turn_id > 0 && turnNodeByTurnID[spawn.parent_turn_id]) {
        parentID = turnNodeByTurnID[spawn.parent_turn_id];
      } else {
        parentID = rootID;
      }

      addNode({
        id: nodeID,
        idNumber: Number(spawn.id) || 0,
        type: 'spawn',
        label: String(spawn.profile || spawn.role || ('spawn-' + spawn.id)),
        icon: agentInfo(inferSpawnAgent(spawn, turnByID, sessionByID)).icon,
        status: normalizeStatus(spawn.status || 'unknown'),
        agent: inferSpawnAgent(spawn, turnByID, sessionByID),
        model: inferSpawnModel(spawn, turnByID, sessionByID),
        role: String(spawn.role || ''),
        scope: 'spawn-' + spawn.id,
        startedAt: spawn.started_at || '',
        endedAt: spawn.completed_at || '',
        task: String(spawn.task || ''),
        summary: String(spawn.summary || ''),
        question: String(spawn.question || ''),
        order: parseTimestamp(spawn.started_at) || (10000 + Number(spawn.id) || 0),
        parent: parentID,
        children: [],
      });

      addEdge(parentID, nodeID, 'spawn');
    });

    pending = next;
  }

  edges.forEach(function (edge) {
    var parent = nodesByID[edge.from];
    if (!parent) return;
    parent.children.push(edge.to);
  });

  nodes.forEach(function (node) {
    node.children.sort(function (aID, bID) {
      var a = nodesByID[aID];
      var b = nodesByID[bID];
      if (!a || !b) return 0;
      if (a.type !== b.type) {
        if (a.type === 'turn') return -1;
        if (b.type === 'turn') return 1;
      }
      if (a.order !== b.order) return a.order - b.order;
      return String(a.id).localeCompare(String(b.id));
    });
  });

  var relevantScopes = {};
  if (sessionScope) relevantScopes[sessionScope] = true;
  nodes.forEach(function (node) {
    if (!node.scope) return;
    relevantScopes[node.scope] = true;
  });

  var events = (Array.isArray(activity) ? activity : [])
    .filter(function (entry) {
      if (!entry || !entry.scope) return false;
      if (!Object.keys(relevantScopes).length) return true;
      return !!relevantScopes[entry.scope];
    })
    .slice()
    .sort(function (a, b) {
      return (Number(b && b.ts) || 0) - (Number(a && a.ts) || 0);
    })
    .slice(0, 80)
    .map(function (entry, idx) {
      var ts = Number(entry && entry.ts) || 0;
      return {
        id: String(entry && entry.id ? entry.id : ('event-' + idx + '-' + ts)),
        scope: String(entry && entry.scope ? entry.scope : ''),
        text: cropText(String(entry && entry.text ? entry.text : ''), 200),
        ts: ts,
        clock: ts > 0 ? formatClock(ts) : '--:--:--',
        color: eventColor(entry),
        type: String(entry && entry.type ? entry.type : 'text'),
      };
    });

  var timelineStart = parseTimestamp(run.started_at) || (events.length ? events[events.length - 1].ts : Date.now());
  var timelineEnd = Date.now();
  if (timelineEnd <= timelineStart) timelineEnd = timelineStart + 1;

  var timelineTicks = events
    .filter(function (entry) { return entry.ts > 0; })
    .map(function (entry) {
      var pct = ((entry.ts - timelineStart) / (timelineEnd - timelineStart)) * 100;
      return {
        id: entry.id,
        scope: entry.scope,
        color: entry.color,
        left: clamp(pct, 0, 100),
        title: entry.clock + ' ' + entry.text,
      };
    });

  var activeCount = nodes.filter(function (node) { return isLiveStatus(node.status); }).length;
  var turnCount = nodes.filter(function (node) { return node.type === 'turn'; }).length;
  var spawnCount = nodes.filter(function (node) { return node.type === 'spawn'; }).length;

  var stepLabel = 'cycle ' + (Number(run.cycle || 0) + 1);
  if (steps.length > 0) {
    stepLabel += ' \u00b7 step ' + (Number(run.step_index || 0) + 1) + '/' + steps.length;
  }

  var root = nodesByID[rootID];

  return {
    runKey: runKey,
    loopName: String(run.loop_name || 'loop'),
    rootID: rootID,
    sessionScope: sessionScope,
    nodes: nodes,
    nodesByID: nodesByID,
    edges: edges,
    scopeToNodeID: scopeToNodeID,
    events: events,
    timelineTicks: timelineTicks,
    activeCount: activeCount,
    turnCount: turnCount,
    spawnCount: spawnCount,
    stepLabel: stepLabel,
    elapsedLabel: root ? formatElapsed(root.startedAt, root.endedAt) : '--',
  };
}

function makeEmptySimulation() {
  return {
    nodes: {},
    nodeIDs: [],
    edges: [],
    rootID: '',
    childrenByParent: {},
  };
}

function syncSimulation(sim, snapshot) {
  if (!sim || !snapshot) return;

  var oldNodes = sim.nodes || {};
  var nextNodes = {};

  snapshot.nodes.forEach(function (node) {
    var prev = oldNodes[node.id] || null;

    var base = {
      ...node,
      x: prev ? prev.x : 0,
      y: prev ? prev.y : 0,
      vx: prev ? prev.vx : 0,
      vy: prev ? prev.vy : 0,
      baseX: prev ? prev.baseX : 0,
      baseY: prev ? prev.baseY : 0,
      radius: nodeRadius(node.type),
    };

    if (!prev) {
      var parent = node.parent ? nextNodes[node.parent] || oldNodes[node.parent] : null;
      if (parent) {
        base.x = parent.x;
        base.y = parent.y;
      }
    }

    nextNodes[node.id] = base;
  });

  var childrenByParent = {};
  snapshot.edges.forEach(function (edge) {
    if (!childrenByParent[edge.from]) childrenByParent[edge.from] = [];
    childrenByParent[edge.from].push(edge.to);
  });

  Object.keys(childrenByParent).forEach(function (parentID) {
    childrenByParent[parentID].sort(function (aID, bID) {
      var a = nextNodes[aID];
      var b = nextNodes[bID];
      if (!a || !b) return 0;
      if (a.order !== b.order) return a.order - b.order;
      return String(a.id).localeCompare(String(b.id));
    });
  });

  var rootID = snapshot.rootID;
  if (nextNodes[rootID]) {
    nextNodes[rootID].baseX = 0;
    nextNodes[rootID].baseY = 0;
  }

  function assignTargets(parentID, depth, centerAngle, spread) {
    var kids = childrenByParent[parentID] || [];
    if (!kids.length) return;

    var parent = nextNodes[parentID];
    if (!parent) return;

    var radius = depth === 1 ? 285 : (220 + depth * 78);
    var localSpread = spread;
    if (!Number.isFinite(localSpread) || localSpread <= 0) {
      localSpread = Math.min(Math.PI * 1.2, Math.max(0.8, kids.length * 0.48));
    }

    var start = centerAngle - localSpread / 2;
    var step = kids.length > 1 ? localSpread / (kids.length - 1) : 0;

    kids.forEach(function (childID, idx) {
      var child = nextNodes[childID];
      if (!child) return;

      var angle = kids.length === 1 ? centerAngle : (start + idx * step);
      child.baseX = parent.baseX + Math.cos(angle) * radius;
      child.baseY = parent.baseY + Math.sin(angle) * radius;

      var nextSpread = Math.min(Math.PI * 0.95, Math.max(0.65, ((childrenByParent[childID] || []).length * 0.45) + 0.5));
      assignTargets(childID, depth + 1, angle, nextSpread);
    });
  }

  assignTargets(rootID, 1, Math.PI / 2, Math.PI * 1.15);

  sim.nodes = nextNodes;
  sim.nodeIDs = Object.keys(nextNodes);
  sim.nodeIDs.sort(function (a, b) {
    var na = nextNodes[a];
    var nb = nextNodes[b];
    if (!na || !nb) return 0;
    if (na.type !== nb.type) {
      return (NODE_DRAW_ORDER[na.type] || 0) - (NODE_DRAW_ORDER[nb.type] || 0);
    }
    return String(na.id).localeCompare(String(nb.id));
  });
  sim.edges = snapshot.edges.slice();
  sim.rootID = rootID;
  sim.childrenByParent = childrenByParent;
}

function stepSimulation(sim) {
  if (!sim || !sim.nodeIDs || !sim.nodeIDs.length) return;

  var ids = sim.nodeIDs;
  var nodes = sim.nodes;

  for (var i = 0; i < ids.length; i++) {
    var a = nodes[ids[i]];
    if (!a) continue;
    for (var j = i + 1; j < ids.length; j++) {
      var b = nodes[ids[j]];
      if (!b) continue;

      var dx = a.x - b.x;
      var dy = a.y - b.y;
      var d2 = dx * dx + dy * dy;
      if (d2 < 1) d2 = 1;

      var minDist = a.radius + b.radius + 52;
      if (d2 < minDist * minDist) {
        var d = Math.sqrt(d2);
        var push = (minDist - d) * 0.035;
        var ux = dx / d;
        var uy = dy / d;
        a.vx += ux * push;
        a.vy += uy * push;
        b.vx -= ux * push;
        b.vy -= uy * push;
      }
    }
  }

  ids.forEach(function (id) {
    var node = nodes[id];
    if (!node) return;

    var spring = node.type === 'loop' ? 0.022 : 0.018;
    node.vx += (node.baseX - node.x) * spring;
    node.vy += (node.baseY - node.y) * spring;

    node.vx *= 0.84;
    node.vy *= 0.84;

    node.x += node.vx;
    node.y += node.vy;
  });
}

function drawScene(canvas, container, sim, camera, selection, now) {
  if (!canvas || !container || !sim) return;

  resizeCanvas(canvas, container, null, null);

  var rect = container.getBoundingClientRect();
  if (!rect.width || !rect.height) return;

  var ctx = canvas.getContext('2d');
  if (!ctx) return;

  var dpr = window.devicePixelRatio || 1;
  var width = rect.width;
  var height = rect.height;

  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);

  drawBackground(ctx, width, height);

  sim.edges.forEach(function (edge) {
    var from = sim.nodes[edge.from];
    var to = sim.nodes[edge.to];
    if (!from || !to) return;

    var sp = worldToScreen(from.x, from.y, width, height, camera);
    var tp = worldToScreen(to.x, to.y, width, height, camera);

    var style = EDGE_STYLES[edge.type] || EDGE_STYLES.spawn;
    var midY = (sp.y + tp.y) / 2;

    ctx.beginPath();
    ctx.moveTo(sp.x, sp.y);
    ctx.bezierCurveTo(sp.x, midY, tp.x, midY, tp.x, tp.y);
    ctx.strokeStyle = style.color;
    ctx.lineWidth = style.width;
    ctx.setLineDash(style.dash || []);
    ctx.stroke();
    ctx.setLineDash([]);

    ctx.fillStyle = style.color;
    ctx.beginPath();
    ctx.arc(tp.x, tp.y, 2.2, 0, Math.PI * 2);
    ctx.fill();
  });

  sim.nodeIDs.forEach(function (id) {
    var node = sim.nodes[id];
    if (!node) return;

    var p = worldToScreen(node.x, node.y, width, height, camera);
    var scale = camera.z;
    var radius = Math.max(9, node.radius * scale);

    var info = agentInfo(node.agent || 'generic');
    var base = hexToRgb(info.color || '#9ca3af');
    var status = statusColor(node.status || 'unknown');

    var selected = selection.selectedNodeID === node.id;
    var hovered = selection.hoverNodeID === node.id;
    var running = isLiveStatus(node.status);

    if (running) {
      var pulse = Math.sin((now || 0) * 0.002 + node.order) * 0.22 + 0.78;
      var glow = ctx.createRadialGradient(p.x, p.y, radius * 0.3, p.x, p.y, radius * 2.8);
      glow.addColorStop(0, 'rgba(' + base.r + ', ' + base.g + ', ' + base.b + ', ' + (0.15 * pulse) + ')');
      glow.addColorStop(1, 'rgba(' + base.r + ', ' + base.g + ', ' + base.b + ', 0)');
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(p.x, p.y, radius * 2.8, 0, Math.PI * 2);
      ctx.fill();
    }

    if (selected) {
      ctx.beginPath();
      ctx.arc(p.x, p.y, radius + 9, 0, Math.PI * 2);
      ctx.strokeStyle = 'rgba(' + base.r + ', ' + base.g + ', ' + base.b + ', 0.58)';
      ctx.lineWidth = 1.7;
      ctx.setLineDash([6, 4]);
      ctx.stroke();
      ctx.setLineDash([]);
    }

    var fillAlpha = hovered ? 0.16 : 0.1;
    var strokeAlpha = hovered ? 0.82 : 0.46;

    if (node.type === 'loop') {
      drawHex(ctx, p.x, p.y, radius, {
        fill: 'rgba(' + base.r + ', ' + base.g + ', ' + base.b + ', ' + fillAlpha + ')',
        stroke: 'rgba(' + base.r + ', ' + base.g + ', ' + base.b + ', ' + strokeAlpha + ')',
        lineWidth: 2,
      });
    } else {
      ctx.beginPath();
      ctx.arc(p.x, p.y, radius, 0, Math.PI * 2);
      ctx.fillStyle = 'rgba(' + base.r + ', ' + base.g + ', ' + base.b + ', ' + fillAlpha + ')';
      ctx.fill();
      ctx.strokeStyle = 'rgba(' + base.r + ', ' + base.g + ', ' + base.b + ', ' + strokeAlpha + ')';
      ctx.lineWidth = node.type === 'turn' ? 2 : 1.5;
      ctx.stroke();

      if (node.type === 'turn') {
        ctx.beginPath();
        ctx.arc(p.x, p.y, radius + 6, 0, Math.PI * 2);
        ctx.strokeStyle = 'rgba(' + base.r + ', ' + base.g + ', ' + base.b + ', 0.24)';
        ctx.lineWidth = 1.2;
        ctx.stroke();
      }
    }

    var statusRGB = hexToRgb(status);
    ctx.beginPath();
    ctx.arc(p.x + radius * 0.62, p.y - radius * 0.62, 4.1, 0, Math.PI * 2);
    ctx.fillStyle = 'rgb(' + statusRGB.r + ', ' + statusRGB.g + ', ' + statusRGB.b + ')';
    ctx.fill();

    ctx.font = '600 ' + Math.max(10, radius * 0.46) + 'px Outfit, sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillStyle = 'rgba(' + base.r + ', ' + base.g + ', ' + base.b + ', 0.88)';
    ctx.fillText(node.type === 'turn' ? 'T' : node.icon, p.x, p.y + 0.5);

    if (camera.z > 0.34) {
      ctx.font = '500 ' + Math.max(10, Math.min(12, 10.5 * camera.z)) + 'px JetBrains Mono, monospace';
      ctx.fillStyle = hovered ? 'rgba(236,239,244,0.95)' : 'rgba(220,224,233,0.70)';
      ctx.fillText(cropText(node.label, 28), p.x, p.y + radius + 14);

      if (camera.z > 0.58) {
        ctx.font = '400 ' + Math.max(8, Math.min(10, 8.6 * camera.z)) + 'px JetBrains Mono, monospace';
        ctx.fillStyle = 'rgba(133,143,163,0.7)';
        var extra = node.status;
        if (node.type === 'spawn' && node.role) extra += ' | ' + node.role;
        ctx.fillText(cropText(extra, 26), p.x, p.y + radius + 26);
      }
    }
  });
}

function hitNode(sim, worldX, worldY) {
  if (!sim || !sim.nodeIDs) return '';

  for (var i = sim.nodeIDs.length - 1; i >= 0; i--) {
    var id = sim.nodeIDs[i];
    var node = sim.nodes[id];
    if (!node) continue;

    var dx = node.x - worldX;
    var dy = node.y - worldY;
    var r = node.radius + 10;
    if (dx * dx + dy * dy <= r * r) return node.id;
  }

  return '';
}

function worldToScreen(x, y, width, height, camera) {
  return {
    x: (x - camera.x) * camera.z + width / 2,
    y: (y - camera.y) * camera.z + height / 2,
  };
}

function screenToWorld(clientX, clientY, container, camera) {
  if (!container) return { x: 0, y: 0 };
  var rect = container.getBoundingClientRect();
  var sx = clientX - rect.left;
  var sy = clientY - rect.top;
  return {
    x: (sx - rect.width / 2) / camera.z + camera.x,
    y: (sy - rect.height / 2) / camera.z + camera.y,
  };
}

function resizeCanvas(canvas, container, sizeRef, dprRef) {
  if (!canvas || !container) return;

  var rect = container.getBoundingClientRect();
  var width = Math.max(1, Math.floor(rect.width));
  var height = Math.max(1, Math.floor(rect.height));
  var dpr = window.devicePixelRatio || 1;

  var targetW = Math.floor(width * dpr);
  var targetH = Math.floor(height * dpr);

  if (canvas.width !== targetW || canvas.height !== targetH) {
    canvas.width = targetW;
    canvas.height = targetH;
  }

  if (sizeRef && sizeRef.current) sizeRef.current = { w: width, h: height };
  if (dprRef) dprRef.current = dpr;
}

function drawBackground(ctx, width, height) {
  var grad = ctx.createLinearGradient(0, 0, 0, height);
  grad.addColorStop(0, '#07070f');
  grad.addColorStop(1, '#04050a');
  ctx.fillStyle = grad;
  ctx.fillRect(0, 0, width, height);

  var g1 = ctx.createRadialGradient(width * 0.16, height * 0.14, 12, width * 0.16, height * 0.14, width * 0.48);
  g1.addColorStop(0, 'rgba(123,140,255,0.20)');
  g1.addColorStop(1, 'rgba(123,140,255,0)');
  ctx.fillStyle = g1;
  ctx.fillRect(0, 0, width, height);

  var g2 = ctx.createRadialGradient(width * 0.82, height * 0.08, 10, width * 0.82, height * 0.08, width * 0.45);
  g2.addColorStop(0, 'rgba(91,206,252,0.17)');
  g2.addColorStop(1, 'rgba(91,206,252,0)');
  ctx.fillStyle = g2;
  ctx.fillRect(0, 0, width, height);
}

function drawHex(ctx, cx, cy, r, style) {
  ctx.beginPath();
  for (var i = 0; i < 6; i++) {
    var angle = Math.PI / 3 * i - Math.PI / 6;
    var x = cx + Math.cos(angle) * r;
    var y = cy + Math.sin(angle) * r;
    if (i === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  }
  ctx.closePath();
  ctx.fillStyle = style.fill;
  ctx.fill();
  ctx.strokeStyle = style.stroke;
  ctx.lineWidth = style.lineWidth || 1.5;
  ctx.stroke();
}

function mapRunTurnsToSteps(loopRun, turnIDs, turnByID) {
  var run = loopRun || {};
  var steps = Array.isArray(run.steps) ? run.steps : [];
  if (!steps.length) return {};

  var stepByHex = {};
  var stepHexIDs = run && typeof run.step_hex_ids === 'object' ? run.step_hex_ids : {};

  Object.keys(stepHexIDs).forEach(function (cycleStep) {
    var hex = String(stepHexIDs[cycleStep] || '').trim();
    if (!hex) return;

    var keyParts = String(cycleStep || '').split(':');
    if (keyParts.length !== 2) return;

    var stepIndex = Number(keyParts[1]);
    if (!Number.isFinite(stepIndex) || stepIndex < 0 || stepIndex >= steps.length) return;

    var step = steps[stepIndex] || {};
    stepByHex[hex] = {
      profile: String(step.profile || ''),
      position: String(step.position || ''),
      step_index: stepIndex,
    };
  });

  var byTurn = {};
  var fallbackStepIndex = 0;
  var remaining = stepTurns(steps[0]);

  turnIDs.forEach(function (turnID) {
    var turn = turnByID[turnID] || null;
    var mapped = null;

    if (turn && turn.step_hex_id) {
      mapped = stepByHex[String(turn.step_hex_id).trim()] || null;
    }

    if (!mapped) {
      var fallbackStep = steps[fallbackStepIndex] || {};
      mapped = {
        profile: String(fallbackStep.profile || ''),
        position: String(fallbackStep.position || ''),
        step_index: fallbackStepIndex,
      };
    }

    byTurn[turnID] = mapped;

    remaining -= 1;
    if (remaining <= 0) {
      fallbackStepIndex = (fallbackStepIndex + 1) % steps.length;
      remaining = stepTurns(steps[fallbackStepIndex]);
    }
  });

  return byTurn;
}

function stepTurns(step) {
  var count = Number(step && step.turns) || 1;
  return count > 0 ? count : 1;
}

function normalizeTurnNodeStatus(turn, daemonSession, latest) {
  var turnState = normalizeStatus(turn && turn.build_state ? turn.build_state : '');
  if (turnState === 'success') return 'completed';
  if (turnState && turnState !== 'unknown') return turnState;

  if (latest && daemonSession) {
    var daemonState = normalizeStatus(daemonSession.status || '');
    if (daemonState === 'success') return 'completed';
    if (daemonState && daemonState !== 'unknown') return daemonState;
  }

  return 'completed';
}

function spawnBelongsToRun(spawn, runTurnSet, daemonSessionID) {
  if (!spawn || spawn.id <= 0) return false;
  if (spawn.parent_turn_id > 0 && runTurnSet[spawn.parent_turn_id]) return true;
  if (spawn.child_turn_id > 0 && runTurnSet[spawn.child_turn_id]) return true;
  if (daemonSessionID > 0 && Number(spawn.parent_daemon_session_id || 0) === daemonSessionID) return true;
  if (daemonSessionID > 0 && Number(spawn.child_daemon_session_id || 0) === daemonSessionID) return true;
  return false;
}

function inferSpawnAgent(spawn, turnByID, sessionByID) {
  if (!spawn) return 'generic';

  var childTurn = turnByID[Number(spawn.child_turn_id) || 0] || null;
  if (childTurn && childTurn.agent) return String(childTurn.agent);

  var childSession = sessionByID[Number(spawn.child_daemon_session_id) || 0] || null;
  if (childSession && childSession.agent) return String(childSession.agent);

  var profile = String(spawn.profile || '').toLowerCase();
  if (profile.indexOf('claude') >= 0) return 'claude';
  if (profile.indexOf('codex') >= 0) return 'codex';
  if (profile.indexOf('gemini') >= 0) return 'gemini';
  if (profile.indexOf('vibe') >= 0) return 'vibe';
  if (profile.indexOf('opencode') >= 0) return 'opencode';

  return 'generic';
}

function inferSpawnModel(spawn, turnByID, sessionByID) {
  if (!spawn) return '';

  var childTurn = turnByID[Number(spawn.child_turn_id) || 0] || null;
  if (childTurn && childTurn.agent_model) return String(childTurn.agent_model);

  var childSession = sessionByID[Number(spawn.child_daemon_session_id) || 0] || null;
  if (childSession && childSession.model) return String(childSession.model);

  return '';
}

function compareSpawn(a, b) {
  var aTS = parseTimestamp(a && a.started_at);
  var bTS = parseTimestamp(b && b.started_at);
  if (aTS !== bTS) return aTS - bTS;
  return (Number(a && a.id) || 0) - (Number(b && b.id) || 0);
}

function isLiveStatus(status) {
  var key = normalizeStatus(status);
  return !!(LIVE_STATUSES[key] || STATUS_RUNNING[key]);
}

function normalizeStatus(value) {
  return String(value || 'unknown').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_');
}

function nodeRadius(type) {
  if (type === 'loop') return 46;
  if (type === 'turn') return 32;
  return 27;
}

function addSecondsISO(ts, seconds) {
  var start = parseTimestamp(ts);
  var delta = Number(seconds) || 0;
  if (!(start > 0) || delta <= 0) return '';
  return new Date(start + delta * 1000).toISOString();
}

function uniquePositiveIDs(ids) {
  var seen = {};
  var out = [];
  (Array.isArray(ids) ? ids : []).forEach(function (value) {
    var id = Number(value) || 0;
    if (id <= 0 || seen[id]) return;
    seen[id] = true;
    out.push(id);
  });
  return out;
}

function clamp(value, min, max) {
  if (value < min) return min;
  if (value > max) return max;
  return value;
}

function eventColor(entry) {
  var type = String(entry && entry.type ? entry.type : 'text').toLowerCase();
  if (type === 'tool_use') return '#5bcefc';
  if (type === 'tool_result') return '#a78bfa';
  if (type === 'thinking') return '#5c6270';
  return '#8b919f';
}

function formatClock(ts) {
  var d = new Date(ts);
  var hh = String(d.getHours()).padStart(2, '0');
  var mm = String(d.getMinutes()).padStart(2, '0');
  var ss = String(d.getSeconds()).padStart(2, '0');
  return hh + ':' + mm + ':' + ss;
}

function hexToRgb(hex) {
  var raw = String(hex || '#888888').replace('#', '');
  if (raw.length === 3) {
    raw = raw[0] + raw[0] + raw[1] + raw[1] + raw[2] + raw[2];
  }
  if (raw.length !== 6) return { r: 136, g: 136, b: 136 };
  return {
    r: parseInt(raw.slice(0, 2), 16),
    g: parseInt(raw.slice(2, 4), 16),
    b: parseInt(raw.slice(4, 6), 16),
  };
}

function HudPill({ label, value, accent, pulse }) {
  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: 6,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 9,
      color: 'var(--text-3)',
      padding: '3px 8px',
      borderRadius: 4,
      border: '1px solid rgba(255,255,255,0.08)',
      background: 'rgba(8,10,14,0.72)',
      textTransform: 'uppercase',
      letterSpacing: '0.05em',
      whiteSpace: 'nowrap',
    }}>
      {pulse ? (
        <span style={{
          width: 6,
          height: 6,
          borderRadius: '50%',
          background: accent || 'var(--green)',
          boxShadow: '0 0 8px ' + (accent || 'var(--green)'),
          animation: 'pulse 1.8s ease-in-out infinite',
        }} />
      ) : null}
      <span>{label}</span>
      <b style={{ color: accent || 'var(--text-1)', fontWeight: 700, textTransform: 'none' }}>{value}</b>
    </span>
  );
}

function DetailRow({ label, value, mono }) {
  return (
    <div style={{
      display: 'grid',
      gridTemplateColumns: '74px 1fr',
      gap: 8,
      alignItems: 'baseline',
      padding: '3px 0',
      borderBottom: '1px solid rgba(255,255,255,0.04)',
    }}>
      <span style={{ fontFamily: "'Outfit', sans-serif", fontSize: 9, color: 'var(--text-3)', letterSpacing: '0.08em', textTransform: 'uppercase' }}>{label}</span>
      <span style={{ fontFamily: mono ? "'JetBrains Mono', monospace" : "'Outfit', sans-serif", fontSize: 11, color: 'var(--text-1)', wordBreak: 'break-word' }}>{value || '--'}</span>
    </div>
  );
}

function InfoBox({ text }) {
  return (
    <div style={{
      marginTop: 6,
      border: '1px solid rgba(255,255,255,0.07)',
      borderRadius: 6,
      background: 'rgba(0,0,0,0.23)',
      padding: '8px 9px',
      color: 'var(--text-2)',
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 10,
      lineHeight: 1.45,
      whiteSpace: 'pre-wrap',
    }}>{text}</div>
  );
}

function LegendDot({ color, text }) {
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
      <span style={{ width: 7, height: 7, borderRadius: 2, background: color }} />
      <span>{text}</span>
    </span>
  );
}

function LegendLine({ color, text, dashed }) {
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
      <span style={{ width: 16, height: 2, background: color, borderBottom: dashed ? ('1px dashed ' + color) : 'none', borderRadius: 1 }} />
      <span>{text}</span>
    </span>
  );
}

function hudStyle() {
  return {
    height: 48,
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    padding: '0 14px',
    borderBottom: '1px solid rgba(255,255,255,0.06)',
    background: 'linear-gradient(180deg, rgba(5,5,9,0.96), rgba(5,5,9,0.74))',
    backdropFilter: 'blur(8px)',
  };
}

function hudLogoStyle() {
  return {
    fontFamily: "'Outfit', sans-serif",
    fontWeight: 800,
    fontSize: 12,
    letterSpacing: '0.16em',
    color: 'var(--accent)',
  };
}

function hudSepStyle() {
  return {
    width: 1,
    height: 16,
    background: 'rgba(255,255,255,0.10)',
  };
}

function controlButtonStyle() {
  return {
    border: '1px solid rgba(255,255,255,0.10)',
    background: 'rgba(8,10,14,0.68)',
    color: 'var(--text-2)',
    borderRadius: 4,
    height: 24,
    padding: '0 8px',
    fontFamily: "'JetBrains Mono', monospace",
    fontSize: 10,
    cursor: 'pointer',
  };
}

function surfaceStyle() {
  return {
    position: 'relative',
    flex: 1,
    overflow: 'hidden',
  };
}

function eventLogShellStyle() {
  return {
    position: 'absolute',
    left: 14,
    top: 12,
    bottom: 74,
    width: 278,
    border: '1px solid rgba(255,255,255,0.07)',
    borderRadius: 9,
    background: 'rgba(10,12,19,0.76)',
    backdropFilter: 'blur(12px)',
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
    zIndex: 4,
  };
}

function detailShellStyle() {
  return {
    position: 'absolute',
    right: 14,
    top: 12,
    bottom: 74,
    width: 334,
    border: '1px solid rgba(255,255,255,0.08)',
    borderRadius: 9,
    background: 'rgba(10,12,19,0.84)',
    backdropFilter: 'blur(14px)',
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
    zIndex: 4,
  };
}

function detailHeadStyle() {
  return {
    padding: '10px 12px',
    borderBottom: '1px solid rgba(255,255,255,0.06)',
    display: 'flex',
    alignItems: 'center',
    gap: 9,
  };
}

function sectionHeadStyle() {
  return {
    fontFamily: "'Outfit', sans-serif",
    fontSize: 10,
    fontWeight: 700,
    textTransform: 'uppercase',
    letterSpacing: '0.12em',
    color: 'var(--text-3)',
    padding: '9px 10px',
    borderBottom: '1px solid rgba(255,255,255,0.06)',
  };
}

function smallSectionTitleStyle() {
  return {
    fontFamily: "'Outfit', sans-serif",
    fontSize: 9,
    fontWeight: 700,
    textTransform: 'uppercase',
    letterSpacing: '0.11em',
    color: 'var(--text-3)',
    marginBottom: 4,
  };
}

function timelineStyle() {
  return {
    position: 'absolute',
    left: 14,
    right: 14,
    bottom: 10,
    zIndex: 5,
    pointerEvents: 'auto',
  };
}
