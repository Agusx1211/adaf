import { useMemo, useEffect, useState, useRef } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { fetchSessionRecordingEvents } from '../../api/hooks.js';
import { normalizeStatus, cropText, safeJSONString, parseTimestamp } from '../../utils/format.js';
import { STATUS_RUNNING, scopeColor } from '../../utils/colors.js';
import { reportMissingUISample } from '../../api/missingUISamples.js';
import { injectEventBlockStyles, stateEventsToBlocks } from '../common/EventBlocks.jsx';
import ChatMessageList from '../common/ChatMessageList.jsx';
import { buildSpawnScopeMaps, parseScope } from '../../utils/scopes.js';

export default function AgentOutput({ scope }) {
  var state = useAppState();
  var dispatch = useDispatch();
  var { streamEvents, autoScroll, sessions, spawns, loopRuns, turns, historicalEvents, currentProjectID, wsConnected, currentSessionSocketID } = state;
  var [loadingHistory, setLoadingHistory] = useState(false);
  var historySocketLiveRef = useRef({});

  useEffect(function () { injectEventBlockStyles(); }, []);

  var scopeInfo = useMemo(function () {
    return parseScope(scope);
  }, [scope]);

  var scopeMaps = useMemo(function () {
    return buildSpawnScopeMaps(spawns, loopRuns);
  }, [spawns, loopRuns]);

  var selectedSessionID = useMemo(function () {
    if (scopeInfo.kind === 'session' || scopeInfo.kind === 'session_main') return scopeInfo.id;
    if (scopeInfo.kind === 'turn' || scopeInfo.kind === 'turn_main') return scopeMaps.turnToSession[scopeInfo.id] || 0;
    if (scopeInfo.kind === 'spawn') return scopeMaps.spawnToSession[scopeInfo.id] || 0;
    return 0;
  }, [scopeInfo, scopeMaps]);

  var selectedSession = useMemo(function () {
    if (!selectedSessionID) return null;
    return sessions.find(function (s) { return s.id === selectedSessionID; }) || null;
  }, [selectedSessionID, sessions]);

  var selectedSpawn = useMemo(function () {
    if (scopeInfo.kind !== 'spawn') return null;
    return spawns.find(function (s) { return s.id === scopeInfo.id; }) || null;
  }, [scopeInfo, spawns]);

  var selectedTurn = useMemo(function () {
    if (scopeInfo.kind !== 'turn' && scopeInfo.kind !== 'turn_main') return null;
    return turns.find(function (turn) { return turn && turn.id === scopeInfo.id; }) || null;
  }, [scopeInfo, turns]);

  var descendantSpawnSet = useMemo(function () {
    var result = {};
    if (scopeInfo.kind !== 'session' || !selectedSessionID) return result;
    var ids = scopeMaps.sessionToSpawnIDs[selectedSessionID] || [];
    ids.forEach(function (id) { result[id] = true; });
    return result;
  }, [scopeInfo, selectedSessionID, scopeMaps]);

  var turnSpawnSet = useMemo(function () {
    var result = {};
    if (scopeInfo.kind !== 'turn' || scopeInfo.id <= 0) return result;
    var ids = scopeMaps.turnToSpawnIDs[scopeInfo.id] || [];
    ids.forEach(function (id) { result[id] = true; });
    return result;
  }, [scopeInfo, scopeMaps]);

  var sessionRunning = !!(selectedSession && STATUS_RUNNING[normalizeStatus(selectedSession.status)]);
  var spawnRunning = !!(selectedSpawn && STATUS_RUNNING[normalizeStatus(selectedSpawn.status)]);
  var selectedTurnRunning = !!(selectedTurn && STATUS_RUNNING[normalizeStatus(selectedTurn.build_state)]);
  var selectedTurnHasState = !!(selectedTurn && normalizeStatus(selectedTurn.build_state) !== 'unknown');
  var isRunning = (scopeInfo.kind === 'session' || scopeInfo.kind === 'session_main' || scopeInfo.kind === 'turn' || scopeInfo.kind === 'turn_main')
    ? ((scopeInfo.kind === 'turn' || scopeInfo.kind === 'turn_main')
      ? (selectedTurnHasState ? selectedTurnRunning : sessionRunning)
      : sessionRunning)
    : (scopeInfo.kind === 'spawn' ? (sessionRunning || spawnRunning) : false);

  var sessionsByID = useMemo(function () {
    var index = {};
    sessions.forEach(function (s) { if (s && s.id > 0) index[s.id] = s; });
    return index;
  }, [sessions]);

  var spawnsByID = useMemo(function () {
    var index = {};
    spawns.forEach(function (s) { if (s && s.id > 0) index[s.id] = s; });
    return index;
  }, [spawns]);

  var turnIDByHex = useMemo(function () {
    var index = {};
    turns.forEach(function (turn) {
      if (!turn || turn.id <= 0) return;
      var key = normalizeTurnHex(turn.hex_id);
      if (!key) return;
      index[key] = turn.id;
    });
    return index;
  }, [turns]);

  var filteredEvents = useMemo(function () {
    if (!scope) return [];
    return streamEvents.filter(function (e) {
      var activeSpawnSet = scopeInfo.kind === 'turn' ? turnSpawnSet : descendantSpawnSet;
      return eventScopeMatches(scopeInfo, selectedSessionID, activeSpawnSet, e);
    }).map(function (evt) {
      return annotateSource(evt, sessionsByID, spawnsByID);
    });
  }, [streamEvents, scope, scopeInfo, selectedSessionID, descendantSpawnSet, turnSpawnSet, sessionsByID, spawnsByID]);

  var blockEvents = useMemo(function () {
    return stateEventsToBlocks(filteredEvents);
  }, [filteredEvents]);

  // Historical replay is sourced from the owning daemon session event stream,
  // and merged with live events to preserve continuity across wait/resume cycles.
  var historySessionID = selectedSessionID;

  useEffect(function () {
    if (!historySessionID) return;
    var socketLiveForHistory = !!(wsConnected && Number(currentSessionSocketID || 0) === historySessionID);
    var wasSocketLive = !!historySocketLiveRef.current[historySessionID];
    var justReconnected = socketLiveForHistory && !wasSocketLive;
    historySocketLiveRef.current[historySessionID] = socketLiveForHistory;

    var hasCachedHistory = !!historicalEvents[historySessionID];
    if (hasCachedHistory && !justReconnected) return;

    var shouldShowLoading = !isRunning && blockEvents.length === 0;
    if (shouldShowLoading) setLoadingHistory(true);
    fetchSessionRecordingEvents(historySessionID, currentProjectID, dispatch)
      .then(function () { if (shouldShowLoading) setLoadingHistory(false); })
      .catch(function () { if (shouldShowLoading) setLoadingHistory(false); });
  }, [historySessionID, isRunning, blockEvents.length, historicalEvents, currentProjectID, dispatch, wsConnected, currentSessionSocketID]);

  var historicalData = useMemo(function () {
    if (!historySessionID || !historicalEvents[historySessionID]) {
      return { events: [], missingSamples: [] };
    }

    var parsedMissing = [];
    var events = parseHistoricalEvents(historicalEvents[historySessionID], historySessionID, turnIDByHex, function (sample) {
      if (!sample || typeof sample !== 'object') return;
      sample.scope = sample.scope || scope || ('session-' + historySessionID);
      sample.session_id = sample.session_id || historySessionID;
      parsedMissing.push(sample);
    });

    var scoped = events.filter(function (ev) {
      var activeSpawnSet = scopeInfo.kind === 'turn' ? turnSpawnSet : descendantSpawnSet;
      return eventScopeMatches(scopeInfo, selectedSessionID, activeSpawnSet, ev);
    }).map(function (evt) {
      return annotateSource(evt, sessionsByID, spawnsByID);
    });

    return { events: scoped, missingSamples: parsedMissing };
  }, [historySessionID, historicalEvents, scope, scopeInfo, selectedSessionID, descendantSpawnSet, turnSpawnSet, sessionsByID, spawnsByID, turnIDByHex]);

  useEffect(function () {
    if (!historicalData.missingSamples || historicalData.missingSamples.length === 0) return;
    historicalData.missingSamples.forEach(function (sample) {
      reportMissingUISample(currentProjectID, sample);
    });
  }, [historicalData.missingSamples, currentProjectID]);

  var historicalBlocks = useMemo(function () {
    return stateEventsToBlocks(historicalData.events);
  }, [historicalData.events]);

  // Merge historical+live so resumed parent turns preserve context before/after wait-for-spawns.
  var displayBlocks = useMemo(function () {
    return mergeHistoricalAndLiveBlocks(historicalBlocks, blockEvents);
  }, [historicalBlocks, blockEvents]);

  // Transform flat blocks into ChatMessageList messages format.
  var transformed = useMemo(function () {
    var msgs = [];
    var assistantEvents = [];
    var assistantEventsTurnID = 0;
    var currentPromptTurnID = 0;
    var lastPromptTurnID = 0;
    var seenPromptByTurn = {};
    var previousPromptSig = '';
    var previousPromptTurnID = 0;
    var promptMessageIndexByTurn = {};
    var assistantMessageIndexByTurn = {};

    function latestEventTS(events) {
      var latest = 0;
      (events || []).forEach(function (evt) {
        var ts = Number(evt && evt._ts) || 0;
        if (ts > latest) latest = ts;
      });
      return latest || null;
    }

    function normalizePositiveTurnID(value) {
      var turnID = Number(value || 0);
      if (!Number.isFinite(turnID) || turnID <= 0) return 0;
      return turnID;
    }

    function shiftMessageIndexMaps(insertAt) {
      Object.keys(promptMessageIndexByTurn).forEach(function (key) {
        if (promptMessageIndexByTurn[key] >= insertAt) {
          promptMessageIndexByTurn[key] += 1;
        }
      });
      Object.keys(assistantMessageIndexByTurn).forEach(function (key) {
        if (assistantMessageIndexByTurn[key] >= insertAt) {
          assistantMessageIndexByTurn[key] += 1;
        }
      });
    }

    function appendBlockToAssistantMessage(index, block) {
      if (!Number.isFinite(index) || index < 0 || index >= msgs.length) return false;
      var msg = msgs[index];
      if (!msg || msg.role !== 'assistant') return false;
      if (!Array.isArray(msg.events)) msg.events = [];
      msg.events.push(block);
      var blockTS = Number(block && block._ts) || 0;
      if (!msg.created_at || Number(msg.created_at) < blockTS) {
        msg.created_at = blockTS || msg.created_at || null;
      }
      return true;
    }

    function attachBlockToTurnMessage(turnID, block) {
      var normalizedTurnID = normalizePositiveTurnID(turnID);
      if (!normalizedTurnID) return false;

      var turnKey = String(normalizedTurnID);
      var assistantIndex = assistantMessageIndexByTurn[turnKey];
      if (appendBlockToAssistantMessage(assistantIndex, block)) {
        return true;
      }

      var promptIndex = promptMessageIndexByTurn[turnKey];
      if (!Number.isFinite(promptIndex) || promptIndex < 0 || promptIndex >= msgs.length) return false;

      var insertAt = promptIndex + 1;
      shiftMessageIndexMaps(insertAt);
      msgs.splice(insertAt, 0, {
        id: 'assistant-turn-' + turnKey + '-' + insertAt,
        role: 'assistant',
        content: '',
        events: [block],
        created_at: Number(block && block._ts) || null,
      });
      assistantMessageIndexByTurn[turnKey] = insertAt;
      return true;
    }

    function flushAssistantEvents() {
      if (!assistantEvents.length) {
        assistantEventsTurnID = 0;
        return;
      }

      msgs.push({
        id: 'assistant-' + msgs.length,
        role: 'assistant',
        content: '',
        events: assistantEvents,
        created_at: latestEventTS(assistantEvents),
      });
      var flushTurnID = normalizePositiveTurnID(assistantEventsTurnID || currentPromptTurnID);
      if (flushTurnID > 0) {
        assistantMessageIndexByTurn[String(flushTurnID)] = msgs.length - 1;
      }
      assistantEvents = [];
      assistantEventsTurnID = 0;
    }

    displayBlocks.forEach(function (block) {
      if (block.type === 'initial_prompt') {
        var resolvedPromptTurnID = resolvePromptTurnID(block, turnIDByHex);
        var promptSig = normalizePromptSignature(block.content);
        var dedupeKey = resolvedPromptTurnID > 0 ? (String(resolvedPromptTurnID) + '|' + promptSig) : '';
        var adjacentPromptDuplicate = !!previousPromptSig
          && previousPromptSig === promptSig
          && (
            previousPromptTurnID <= 0
            || resolvedPromptTurnID <= 0
            || previousPromptTurnID === resolvedPromptTurnID
          );

        if (adjacentPromptDuplicate) {
          if (resolvedPromptTurnID > 0) {
            seenPromptByTurn[dedupeKey] = true;
            lastPromptTurnID = resolvedPromptTurnID;
            currentPromptTurnID = resolvedPromptTurnID;
          } else if (previousPromptTurnID > 0) {
            lastPromptTurnID = previousPromptTurnID;
            currentPromptTurnID = previousPromptTurnID;
          }
          if (msgs.length > 0) {
            var latestMsg = msgs[msgs.length - 1];
            var blockTS = Number(block && block._ts) || 0;
            if (latestMsg && latestMsg.role === 'user' && (!latestMsg.created_at || Number(latestMsg.created_at) <= 0) && blockTS > 0) {
              latestMsg.created_at = blockTS;
            }
          }
          previousPromptTurnID = resolvedPromptTurnID > 0 ? resolvedPromptTurnID : previousPromptTurnID;
          return;
        }

        if (resolvedPromptTurnID > 0) {
          if (seenPromptByTurn[dedupeKey]) {
            previousPromptSig = promptSig;
            previousPromptTurnID = resolvedPromptTurnID;
            currentPromptTurnID = resolvedPromptTurnID;
            return;
          }
          seenPromptByTurn[dedupeKey] = true;
        } else if (previousPromptTurnID <= 0 && previousPromptSig && previousPromptSig === promptSig) {
          return;
        }

        flushAssistantEvents();

        msgs.push({
          id: 'prompt-' + msgs.length,
          role: 'user',
          content: block.content,
          created_at: Number(block && block._ts) || null,
          _continuationLabel: continuationLabelForPrompt(block, lastPromptTurnID, turnIDByHex),
        });
        if (resolvedPromptTurnID > 0) {
          currentPromptTurnID = resolvedPromptTurnID;
          lastPromptTurnID = resolvedPromptTurnID;
          promptMessageIndexByTurn[String(resolvedPromptTurnID)] = msgs.length - 1;
        } else {
          currentPromptTurnID = 0;
        }
        previousPromptSig = promptSig;
        previousPromptTurnID = resolvedPromptTurnID;
      } else {
        previousPromptSig = '';
        previousPromptTurnID = 0;
        var blockTurnID = normalizePositiveTurnID(block && (block._turnID || block.turn_id || block.turnID));

        if (blockTurnID > 0 && blockTurnID !== normalizePositiveTurnID(assistantEventsTurnID || currentPromptTurnID)) {
          if (attachBlockToTurnMessage(blockTurnID, block)) {
            return;
          }
        }

        assistantEvents.push(block);
        if (assistantEventsTurnID <= 0) {
          assistantEventsTurnID = blockTurnID > 0 ? blockTurnID : normalizePositiveTurnID(currentPromptTurnID);
        }
      }
    });

    if (!isRunning && assistantEvents.length > 0) {
      flushAssistantEvents();
    }

    return {
      messages: msgs,
      pendingStreamEvents: isRunning ? assistantEvents : [],
    };
  }, [displayBlocks, isRunning, turnIDByHex]);

  if (!scope) {
    return (
      <div style={{
        height: '100%', display: 'flex', flexDirection: 'column',
        alignItems: 'center', justifyContent: 'center', gap: 12, color: 'var(--text-3)',
      }}>
        <span style={{ fontSize: 32, opacity: 0.3 }}>{'\u25A3'}</span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12 }}>Select a session from the sidebar</span>
      </div>
    );
  }

  return (
    <ChatMessageList
      messages={transformed.messages}
      streamEvents={transformed.pendingStreamEvents}
      isStreaming={!!isRunning}
      loading={loadingHistory && displayBlocks.length === 0}
      emptyMessage="No output yet"
      autoScroll={autoScroll}
      showSourceLabels={scopeInfo.kind === 'session' || scopeInfo.kind === 'turn'}
      scrollContextKey={scope || ''}
    />
  );
}

function eventScopeMatches(scopeInfo, selectedSessionID, descendantSpawnSet, event) {
  var scope = String(event && (event.scope || event._scope) || '');
  var eventTurnID = resolveEventTurnID(event);
  if (scopeInfo.kind === 'turn_main') {
    if (selectedSessionID <= 0) return false;
    if (scope !== 'session-' + selectedSessionID) return false;
    return eventTurnID > 0 && eventTurnID === scopeInfo.id;
  }
  if (scopeInfo.kind === 'turn') {
    if (selectedSessionID <= 0) return false;
    if (scope === 'session-' + selectedSessionID) {
      return eventTurnID > 0 && eventTurnID === scopeInfo.id;
    }
    if (scope.indexOf('spawn-') === 0) {
      var turnSpawnID = parseInt(scope.slice(6), 10);
      return !Number.isNaN(turnSpawnID) && !!descendantSpawnSet[turnSpawnID];
    }
    return false;
  }
  if (scopeInfo.kind === 'session_main') {
    if (selectedSessionID <= 0) return false;
    return scope === 'session-' + selectedSessionID;
  }
  if (scopeInfo.kind === 'session') {
    if (selectedSessionID <= 0) return false;
    if (scope === 'session-' + selectedSessionID) return true;
    if (scope.indexOf('spawn-') === 0) {
      var spawnID = parseInt(scope.slice(6), 10);
      return !Number.isNaN(spawnID) && !!descendantSpawnSet[spawnID];
    }
    return false;
  }
  if (scopeInfo.kind === 'spawn') {
    return scope === 'spawn-' + scopeInfo.id;
  }
  return true;
}

function resolveEventTurnID(event) {
  if (!event || typeof event !== 'object') return 0;
  var turnID = Number(event.turn_id || event.turnID || event._turnID || 0);
  if (!Number.isFinite(turnID) || turnID <= 0) return 0;
  return turnID;
}

function annotateSource(event, sessionsByID, spawnsByID) {
  if (!event || typeof event !== 'object') return event;
  if (event._sourceLabel && event._sourceColor) return event;

  var scope = String(event.scope || event._scope || '');
  var sourceLabel = 'agent';
  var spawnID = 0;

  if (scope.indexOf('session-') === 0) {
    var sessionID = parseInt(scope.slice(8), 10);
    if (!Number.isNaN(sessionID) && sessionID > 0) {
      var session = sessionsByID[sessionID];
      sourceLabel = session && session.profile ? session.profile : ('session-' + sessionID);
    }
  } else if (scope.indexOf('spawn-') === 0) {
    spawnID = parseInt(scope.slice(6), 10);
    if (!Number.isNaN(spawnID) && spawnID > 0) {
      var spawn = spawnsByID[spawnID];
      sourceLabel = spawn && spawn.profile ? spawn.profile : ('spawn-' + spawnID);
    }
  }

  return Object.assign({}, event, {
    _sourceLabel: sourceLabel,
    _sourceColor: scopeColor(scope || 'session-0'),
    _spawnID: spawnID,
  });
}

function parseHistoricalEvents(events, sessionID, turnIDByHex, onMissing) {
  var output = [];
  var defaultScope = 'session-' + sessionID;
  var list = Array.isArray(events) ? events : [];
  var activeTurnID = 0;

  function positiveID(value) {
    var n = Number(value || 0);
    if (!Number.isFinite(n) || n <= 0) return 0;
    return n;
  }

  function push(scope, type, payload, ts, turnID, eventID, blockIndex) {
    var event = Object.assign({ scope: scope || defaultScope, type: type }, payload || {});
    if (Number(ts) > 0) event.ts = Number(ts);
    var resolvedTurnID = positiveID(turnID) || positiveID(event.turn_id || event.turnID || 0);
    if (resolvedTurnID > 0) event.turn_id = resolvedTurnID;
    var normalizedEventID = String(eventID || '');
    if (normalizedEventID) event.event_id = normalizedEventID;
    if (Number.isFinite(Number(blockIndex))) event.block_index = Number(blockIndex);
    output.push(event);
  }

  function resolveTurnIDFromHex(value) {
    var key = normalizeTurnHex(value);
    if (!key) return 0;
    var mapped = Number(turnIDByHex && turnIDByHex[key] || 0);
    return positiveID(mapped);
  }

  function report(reason, eventType, payload, fallbackText, scope) {
    if (!onMissing) return;
    onMissing({
      source: 'recording_history',
      reason: reason,
      scope: scope || defaultScope,
      session_id: sessionID,
      event_type: eventType || '',
      fallback_text: fallbackText || '',
      payload: payload,
    });
  }

  function resolveEventID(wireData, agentEvent) {
    return String(
      (wireData && (wireData.event_id || wireData.eventID || wireData.uuid)) ||
      (wireData && wireData.raw && wireData.raw.uuid) ||
      (agentEvent && agentEvent.raw && agentEvent.raw.uuid) ||
      (agentEvent && agentEvent.uuid) ||
      ''
    );
  }

  function parseAssistant(scope, parsed, ts, turnID, eventID) {
    var blocks = extractContentBlocks(parsed);
    if (!blocks.length) {
      report('assistant_without_content_blocks', parsed && parsed.type ? parsed.type : 'assistant', parsed, '[assistant event]', scope);
      push(scope, 'text', { text: '[assistant event]' }, ts, turnID, eventID, 0);
      return;
    }
    blocks.forEach(function (block, blockIndex) {
      if (!block || typeof block !== 'object') return;
      if (block.type === 'text' && block.text) {
        push(scope, 'text', { text: String(block.text) }, ts, turnID, eventID, blockIndex);
      } else if (block.type === 'thinking' && block.text) {
        push(scope, 'thinking', { text: String(block.text) }, ts, turnID, eventID, blockIndex);
      } else if (block.type === 'tool_use') {
        push(scope, 'tool_use', {
          tool: block.name || 'tool',
          input: block.input || {},
          tool_call_id: block.id || block.tool_use_id || '',
        }, ts, turnID, eventID, blockIndex);
      } else if (block.type === 'tool_result') {
        push(scope, 'tool_result', {
          tool: block.name || 'tool_result',
          result: block.content || block.output || block.text || '',
          isError: !!block.is_error,
          tool_call_id: block.tool_use_id || block.id || '',
        }, ts, turnID, eventID, blockIndex);
      } else {
        report('unknown_assistant_block', block.type || 'unknown', block, cropText(safeJSONString(block), 400), scope);
      }
    });
  }

  function parseUser(scope, parsed, ts, turnID, eventID) {
    var blocks = extractContentBlocks(parsed);
    blocks.forEach(function (block, blockIndex) {
      if (block && block.type === 'tool_result') {
        push(scope, 'tool_result', {
          tool: block.name || 'tool_result',
          result: block.content || block.output || block.text || safeJSONString(block),
          isError: !!block.is_error,
          tool_call_id: block.tool_use_id || block.id || '',
        }, ts, turnID, eventID, blockIndex);
      } else if (block && typeof block === 'object') {
        report('unknown_user_block', block.type || 'unknown', block, '', scope);
      }
    });
  }

  list.forEach(function (ev) {
    if (!ev || typeof ev !== 'object') return;
    var eventTS = parseTimestamp(ev.timestamp || ev.ts || ev.time || '');
    var envelopeTurnID = positiveID(ev.turn_id || ev.turnID || 0);
    if (envelopeTurnID > 0) {
      activeTurnID = envelopeTurnID;
    }
    var currentTurnID = envelopeTurnID > 0 ? envelopeTurnID : activeTurnID;

    if (ev.type === 'meta') {
      var metaData = decodeData(ev.data);
      if (metaData && metaData.prompt) {
        push(defaultScope, 'initial_prompt', { text: String(metaData.prompt) }, eventTS, currentTurnID);
      } else if (metaData && metaData.objective) {
        push(defaultScope, 'initial_prompt', { text: String(metaData.objective) }, eventTS, currentTurnID);
      }
      return;
    }

    if (ev.type === 'claude_stream' || ev.type === 'stdout') {
      var parsedStoreEvent = decodeData(ev.data);
      if (parsedStoreEvent && typeof parsedStoreEvent === 'object' && parsedStoreEvent.type) {
        var streamEventID = resolveEventID(parsedStoreEvent, parsedStoreEvent);
        if (parsedStoreEvent.type === 'assistant') {
          parseAssistant(defaultScope, parsedStoreEvent, eventTS, currentTurnID, streamEventID);
          return;
        }
        if (parsedStoreEvent.type === 'user') {
          parseUser(defaultScope, parsedStoreEvent, eventTS, currentTurnID, streamEventID);
          return;
        }
        if (parsedStoreEvent.type === 'content_block_delta' && parsedStoreEvent.delta && parsedStoreEvent.delta.text) {
          push(defaultScope, 'text', { text: String(parsedStoreEvent.delta.text) }, eventTS, currentTurnID, streamEventID, 0);
          return;
        }
      }
      if (ev.data && String(ev.data).trim()) {
        push(defaultScope, 'text', { text: String(ev.data) }, eventTS, currentTurnID);
      }
      return;
    }

    if (ev.type === 'prompt') {
      var promptData = decodeData(ev.data);
      var promptScope = wireScope(promptData, sessionID);
      var promptTurnID = currentTurnID;
      var promptEventID = resolveEventID(promptData, promptData);
      if (promptData && typeof promptData === 'object') {
        var promptTurnCandidate = positiveID(promptData.turn_id || promptData.turnID || promptData.session_id || promptData.sessionID || 0);
        if (promptTurnCandidate > 0) {
          promptTurnID = promptTurnCandidate;
        } else {
          promptTurnID = resolveTurnIDFromHex(promptData.turn_hex_id || promptData.turnHexID || '');
        }
      }
      if (promptTurnID > 0) {
        activeTurnID = promptTurnID;
      }
      if (promptData && promptData.prompt) {
        push(promptScope, 'initial_prompt', {
          text: String(promptData.prompt),
          turn_hex_id: String(promptData.turn_hex_id || promptData.turnHexID || ''),
          turn_id: promptTurnID > 0 ? promptTurnID : 0,
          is_resume: !!(promptData.is_resume || promptData.isResume),
        }, eventTS, promptTurnID, promptEventID, 0);
      }
      return;
    }

    if (ev.type === 'event') {
      var wireData = decodeData(ev.data);
      var eventScope = wireScope(wireData, sessionID);
      var wireTurnID = currentTurnID;
      if (wireData && typeof wireData === 'object') {
        var wireTurnCandidate = positiveID(wireData.turn_id || wireData.turnID || wireData.session_id || wireData.sessionID || 0);
        if (wireTurnCandidate > 0) {
          wireTurnID = wireTurnCandidate;
        }
      }
      var agentEvent = wireData && wireData.event ? wireData.event : wireData;
      if (typeof agentEvent === 'string') {
        agentEvent = decodeData(agentEvent);
      }
      if (!agentEvent || typeof agentEvent !== 'object') {
        report('invalid_wire_event_json', 'event', ev.data, '', eventScope);
        return;
      }
      var wireEventID = resolveEventID(wireData, agentEvent);
      if (wireTurnID <= 0) {
        var eventTurnCandidate = positiveID(agentEvent.turn_id || agentEvent.turnID || 0);
        if (eventTurnCandidate > 0) {
          wireTurnID = eventTurnCandidate;
        }
      }
      if (wireTurnID > 0) {
        activeTurnID = wireTurnID;
      }
      if (agentEvent.type === 'system') {
        return;
      }
      if (agentEvent.type === 'assistant') {
        parseAssistant(eventScope, agentEvent, eventTS, wireTurnID, wireEventID);
      } else if (agentEvent.type === 'user') {
        parseUser(eventScope, agentEvent, eventTS, wireTurnID, wireEventID);
      } else if (agentEvent.type === 'content_block_delta' && agentEvent.delta && agentEvent.delta.text) {
        push(eventScope, 'text', { text: String(agentEvent.delta.text) }, eventTS, wireTurnID, wireEventID, 0);
      } else if (agentEvent.type === 'result') {
        push(eventScope, 'tool_result', { text: 'Result received.' }, eventTS, wireTurnID, wireEventID, 0);
      } else {
        report('unknown_agent_event_type', agentEvent.type || 'event', agentEvent, cropText(safeJSONString(agentEvent), 400), eventScope);
      }
      return;
    }

    if (ev.type === 'finished') {
      var finishedData = decodeData(ev.data);
      var finishedScope = wireScope(finishedData, sessionID);
      var finishedTurnID = currentTurnID;
      if (finishedData && typeof finishedData === 'object') {
        var finishedTurnCandidate = positiveID(finishedData.turn_id || finishedData.turnID || finishedData.session_id || finishedData.sessionID || 0);
        if (finishedTurnCandidate > 0) {
          finishedTurnID = finishedTurnCandidate;
        }
      }
      if (finishedTurnID > 0) {
        activeTurnID = finishedTurnID;
      }
      if (finishedData && finishedData.wait_for_spawns) {
        push(finishedScope, 'text', { text: '[system] waiting for spawns (parent turn suspended)' }, eventTS, finishedTurnID);
      }
      return;
    }

    if (ev.type === 'raw') {
      var rawData = decodeData(ev.data);
      var rawScope = wireScope(rawData, sessionID);
      var rawTurnID = currentTurnID;
      if (rawData && typeof rawData === 'object') {
        var rawTurnCandidate = positiveID(rawData.turn_id || rawData.turnID || rawData.session_id || rawData.sessionID || 0);
        if (rawTurnCandidate > 0) {
          rawTurnID = rawTurnCandidate;
        }
      }
      if (rawTurnID > 0) {
        activeTurnID = rawTurnID;
      }
      var rawText = '';
      if (typeof rawData === 'string') {
        rawText = rawData;
      } else if (rawData && typeof rawData.data === 'string') {
        rawText = rawData.data;
      } else if (ev.data && typeof ev.data === 'string') {
        rawText = ev.data;
      }
      if (rawText.trim()) {
        push(rawScope, 'text', { text: cropText(rawText) }, eventTS, rawTurnID);
      }
    }
  });

  return output;
}

function mergeHistoricalAndLiveBlocks(historicalBlocks, liveBlocks) {
  var hist = Array.isArray(historicalBlocks) ? historicalBlocks : [];
  var live = Array.isArray(liveBlocks) ? liveBlocks : [];
  if (!hist.length) return live;
  if (!live.length) return hist;

  var histSig = hist.map(blockSignature);
  var liveSig = live.map(blockSignature);

  // Fast path: live stream is usually a suffix replay of historical data.
  var overlap = longestSuffixPrefixOverlap(histSig, liveSig);
  if (overlap > 0) {
    return hist.concat(live.slice(overlap));
  }

  // Fallback: if live starts from earlier history (e.g. reconnect snapshot),
  // consume the longest live prefix that already exists contiguously in history.
  var containedPrefix = longestContainedPrefix(histSig, liveSig);
  if (containedPrefix > 0) {
    return hist.concat(live.slice(containedPrefix));
  }

  return hist.concat(live);
}

function longestSuffixPrefixOverlap(histSig, liveSig) {
  var maxOverlap = Math.min(histSig.length, liveSig.length);
  for (var size = maxOverlap; size > 0; size--) {
    var start = histSig.length - size;
    var matches = true;
    for (var i = 0; i < size; i++) {
      if (histSig[start + i] !== liveSig[i]) {
        matches = false;
        break;
      }
    }
    if (matches) return size;
  }
  return 0;
}

function longestContainedPrefix(histSig, liveSig) {
  if (!histSig.length || !liveSig.length) return 0;
  var anchor = liveSig[0];
  var best = 0;

  for (var i = 0; i < histSig.length; i++) {
    if (histSig[i] !== anchor) continue;
    var matched = 0;
    while (i + matched < histSig.length && matched < liveSig.length) {
      if (histSig[i + matched] !== liveSig[matched]) break;
      matched += 1;
    }
    if (matched > best) best = matched;
    if (best === liveSig.length) return best;
  }

  return best;
}

function blockSignature(block) {
  if (!block || typeof block !== 'object') return '';
  var scope = String(block._scope || block.scope || '');
  var eventID = String(block._eventID || block.event_id || '');
  var blockIndex = Number(block._blockIndex || block.block_index || 0) || 0;
  var type = String(block.type || 'text');
  var toolCallID = String(block._toolCallID || block.tool_call_id || block.toolCallID || '');
  if (eventID) {
    return scope + '|event|' + eventID + '|' + String(blockIndex) + '|' + type;
  }
  if (toolCallID && (type === 'tool_use' || type === 'tool_result')) {
    return scope + '|tool_call|' + type + '|' + toolCallID;
  }
  if (type === 'tool_use') {
    return scope + '|' + type + '|' + stableString(block.tool) + '|' + stableString(block.input);
  }
  if (type === 'tool_result') {
    return scope + '|' + type + '|' + stableString(block.tool) + '|' + normalizeToolResultSignature(block.result || block.content || block.text || '') + '|' + (block.isError ? '1' : '0');
  }
  if (type === 'initial_prompt' || type === 'thinking' || type === 'text') {
    return scope + '|' + type + '|' + normalizeBlockText(block.content || block.text || '');
  }
  return scope + '|' + type + '|' + stableString(block.content || block.text || block.result || '');
}

function stableString(value) {
  if (value == null) return '';
  if (typeof value === 'string') return value.length > 500 ? value.slice(0, 500) : value;
  try {
    var json = JSON.stringify(value);
    return json.length > 500 ? json.slice(0, 500) : json;
  } catch (_) {
    var text = String(value);
    return text.length > 500 ? text.slice(0, 500) : text;
  }
}

function normalizeToolResultSignature(value) {
  if (value == null) return '';
  if (typeof value === 'string') {
    var raw = value;
    var trimmed = raw.trim();
    if (!trimmed) return '';
    if (trimmed[0] === '{' || trimmed[0] === '[') {
      try {
        return stableCanonicalString(JSON.parse(trimmed));
      } catch (_) {}
    }
    return raw.length > 500 ? raw.slice(0, 500) : raw;
  }
  return stableCanonicalString(value);
}

function stableCanonicalString(value) {
  try {
    var normalized = canonicalizeSignatureValue(value);
    var json = JSON.stringify(normalized);
    return json.length > 500 ? json.slice(0, 500) : json;
  } catch (_) {
    return stableString(value);
  }
}

function canonicalizeSignatureValue(value) {
  if (Array.isArray(value)) return value.map(canonicalizeSignatureValue);
  if (value && typeof value === 'object') {
    var normalized = {};
    Object.keys(value).sort().forEach(function (key) {
      normalized[key] = canonicalizeSignatureValue(value[key]);
    });
    return normalized;
  }
  return value;
}

function normalizePromptSignature(value) {
  return String(value || '').trim().replace(/\s+/g, ' ');
}

function normalizeBlockText(value) {
  return String(value || '').trim().replace(/\s+/g, ' ');
}

function normalizeTurnHex(raw) {
  return String(raw || '').trim().toLowerCase();
}

function resolvePromptTurnID(block, turnIDByHex) {
  if (!block || typeof block !== 'object') return 0;
  var directID = Number(block._turnID || block.turn_id || block.turnID || 0);
  if (Number.isFinite(directID) && directID > 0) return directID;
  var turnHex = normalizeTurnHex(block._turnHexID || block.turn_hex_id || block.turnHexID || '');
  if (!turnHex) return 0;
  var mapped = Number(turnIDByHex && turnIDByHex[turnHex] || 0);
  if (Number.isFinite(mapped) && mapped > 0) return mapped;
  return 0;
}

function continuationLabelForPrompt(block, lastPromptTurnID, turnIDByHex) {
  if (!block || block.type !== 'initial_prompt' || !block._isResume) return '';
  var currentTurnID = resolvePromptTurnID(block, turnIDByHex);
  if (currentTurnID > 0 && lastPromptTurnID > 0 && currentTurnID === lastPromptTurnID) {
    return '';
  }
  var fromTurnID = lastPromptTurnID > 0 ? lastPromptTurnID : (currentTurnID > 1 ? currentTurnID - 1 : 0);
  if (fromTurnID > 0) {
    return 'Continues from turn ' + fromTurnID;
  }
  return 'Continues from previous turn';
}

function decodeData(raw) {
  if (raw == null) return null;
  if (typeof raw === 'string') {
    try { return JSON.parse(raw); } catch (_) { return raw; }
  }
  return raw;
}

function wireScope(data, fallbackSessionID) {
  var fallbackScope = 'session-' + fallbackSessionID;
  if (!data || typeof data !== 'object') return fallbackScope;
  var spawnID = Number(data.spawn_id || data.spawnID || 0);
  if (Number.isFinite(spawnID) && spawnID > 0) {
    return 'spawn-' + spawnID;
  }
  var sessionID = Number(data.session_id || data.sessionID || 0);
  if (Number.isFinite(sessionID) && sessionID < 0) {
    return 'spawn-' + (-sessionID);
  }
  if (Number.isFinite(sessionID) && sessionID > 0) {
    // In loop recordings this is often a turn ID, not daemon session ID.
    if (fallbackSessionID > 0 && sessionID !== fallbackSessionID) return fallbackScope;
    return 'session-' + sessionID;
  }
  return fallbackScope;
}

function extractContentBlocks(event) {
  if (!event || typeof event !== 'object') return [];
  if (event.message && Array.isArray(event.message.content)) return event.message.content;
  if (Array.isArray(event.content)) return event.content;
  return [];
}
