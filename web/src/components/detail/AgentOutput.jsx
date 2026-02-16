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
  var { streamEvents, autoScroll, sessions, spawns, loopRuns, historicalEvents, currentProjectID, wsConnected, currentSessionSocketID } = state;
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
  var isRunning = (scopeInfo.kind === 'session' || scopeInfo.kind === 'session_main' || scopeInfo.kind === 'turn' || scopeInfo.kind === 'turn_main')
    ? sessionRunning
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

  var filteredEvents = useMemo(function () {
    if (!scope) return [];
    return streamEvents.filter(function (e) {
      var activeSpawnSet = scopeInfo.kind === 'turn' ? turnSpawnSet : descendantSpawnSet;
      return eventScopeMatches(scopeInfo, selectedSessionID, activeSpawnSet, e.scope);
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
    var events = parseHistoricalEvents(historicalEvents[historySessionID], historySessionID, function (sample) {
      if (!sample || typeof sample !== 'object') return;
      sample.scope = sample.scope || scope || ('session-' + historySessionID);
      sample.session_id = sample.session_id || historySessionID;
      parsedMissing.push(sample);
    });

    var scoped = events.filter(function (ev) {
      var activeSpawnSet = scopeInfo.kind === 'turn' ? turnSpawnSet : descendantSpawnSet;
      return eventScopeMatches(scopeInfo, selectedSessionID, activeSpawnSet, ev.scope);
    }).map(function (evt) {
      return annotateSource(evt, sessionsByID, spawnsByID);
    });

    return { events: scoped, missingSamples: parsedMissing };
  }, [historySessionID, historicalEvents, scope, scopeInfo, selectedSessionID, descendantSpawnSet, turnSpawnSet, sessionsByID, spawnsByID]);

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

    function latestEventTS(events) {
      var latest = 0;
      (events || []).forEach(function (evt) {
        var ts = Number(evt && evt._ts) || 0;
        if (ts > latest) latest = ts;
      });
      return latest || null;
    }

    displayBlocks.forEach(function (block) {
      if (block.type === 'initial_prompt') {
        if (assistantEvents.length > 0) {
          msgs.push({
            id: 'assistant-' + msgs.length,
            role: 'assistant',
            content: '',
            events: assistantEvents,
            created_at: latestEventTS(assistantEvents),
          });
          assistantEvents = [];
        }
        msgs.push({
          id: 'prompt-' + msgs.length,
          role: 'user',
          content: block.content,
          created_at: Number(block && block._ts) || null,
        });
      } else {
        assistantEvents.push(block);
      }
    });

    if (!isRunning && assistantEvents.length > 0) {
      msgs.push({
        id: 'assistant-' + msgs.length,
        role: 'assistant',
        content: '',
        events: assistantEvents,
        created_at: latestEventTS(assistantEvents),
      });
      assistantEvents = [];
    }

    return {
      messages: msgs,
      pendingStreamEvents: isRunning ? assistantEvents : [],
    };
  }, [displayBlocks, isRunning]);

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

function eventScopeMatches(scopeInfo, selectedSessionID, descendantSpawnSet, eventScope) {
  var scope = String(eventScope || '');
  if (scopeInfo.kind === 'turn_main') {
    if (selectedSessionID <= 0) return false;
    return scope === 'session-' + selectedSessionID;
  }
  if (scopeInfo.kind === 'turn') {
    if (selectedSessionID <= 0) return false;
    if (scope === 'session-' + selectedSessionID) return true;
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

function parseHistoricalEvents(events, sessionID, onMissing) {
  var output = [];
  var defaultScope = 'session-' + sessionID;
  var list = Array.isArray(events) ? events : [];

  function push(scope, type, payload, ts) {
    var event = Object.assign({ scope: scope || defaultScope, type: type }, payload || {});
    if (Number(ts) > 0) event.ts = Number(ts);
    output.push(event);
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

  function parseAssistant(scope, parsed, ts) {
    var blocks = extractContentBlocks(parsed);
    if (!blocks.length) {
      report('assistant_without_content_blocks', parsed && parsed.type ? parsed.type : 'assistant', parsed, '[assistant event]', scope);
      push(scope, 'text', { text: '[assistant event]' }, ts);
      return;
    }
    blocks.forEach(function (block) {
      if (!block || typeof block !== 'object') return;
      if (block.type === 'text' && block.text) {
        push(scope, 'text', { text: String(block.text) }, ts);
      } else if (block.type === 'thinking' && block.text) {
        push(scope, 'thinking', { text: String(block.text) }, ts);
      } else if (block.type === 'tool_use') {
        push(scope, 'tool_use', { tool: block.name || 'tool', input: block.input || {} }, ts);
      } else if (block.type === 'tool_result') {
        push(scope, 'tool_result', {
          tool: block.name || 'tool_result',
          result: block.content || block.output || block.text || '',
          isError: !!block.is_error,
        }, ts);
      } else {
        report('unknown_assistant_block', block.type || 'unknown', block, cropText(safeJSONString(block), 400), scope);
      }
    });
  }

  function parseUser(scope, parsed, ts) {
    var blocks = extractContentBlocks(parsed);
    blocks.forEach(function (block) {
      if (block && block.type === 'tool_result') {
        push(scope, 'tool_result', {
          tool: block.name || 'tool_result',
          result: block.content || block.output || block.text || safeJSONString(block),
          isError: !!block.is_error,
        }, ts);
      } else if (block && typeof block === 'object') {
        report('unknown_user_block', block.type || 'unknown', block, '', scope);
      }
    });
  }

  list.forEach(function (ev) {
    if (!ev || typeof ev !== 'object') return;
    var eventTS = parseTimestamp(ev.timestamp || ev.ts || ev.time || '');

    if (ev.type === 'meta') {
      var metaData = decodeData(ev.data);
      if (metaData && metaData.prompt) {
        push(defaultScope, 'initial_prompt', { text: String(metaData.prompt) }, eventTS);
      } else if (metaData && metaData.objective) {
        push(defaultScope, 'initial_prompt', { text: String(metaData.objective) }, eventTS);
      }
      return;
    }

    if (ev.type === 'claude_stream' || ev.type === 'stdout') {
      var parsedStoreEvent = decodeData(ev.data);
      if (parsedStoreEvent && typeof parsedStoreEvent === 'object' && parsedStoreEvent.type) {
        if (parsedStoreEvent.type === 'assistant') {
          parseAssistant(defaultScope, parsedStoreEvent, eventTS);
          return;
        }
        if (parsedStoreEvent.type === 'user') {
          parseUser(defaultScope, parsedStoreEvent, eventTS);
          return;
        }
        if (parsedStoreEvent.type === 'content_block_delta' && parsedStoreEvent.delta && parsedStoreEvent.delta.text) {
          push(defaultScope, 'text', { text: String(parsedStoreEvent.delta.text) }, eventTS);
          return;
        }
      }
      if (ev.data && String(ev.data).trim()) {
        push(defaultScope, 'text', { text: String(ev.data) }, eventTS);
      }
      return;
    }

    if (ev.type === 'prompt') {
      var promptData = decodeData(ev.data);
      var promptScope = wireScope(promptData, sessionID);
      if (promptData && promptData.prompt) {
        push(promptScope, 'initial_prompt', { text: String(promptData.prompt) }, eventTS);
      }
      return;
    }

    if (ev.type === 'event') {
      var wireData = decodeData(ev.data);
      var eventScope = wireScope(wireData, sessionID);
      var agentEvent = wireData && wireData.event ? wireData.event : wireData;
      if (typeof agentEvent === 'string') {
        agentEvent = decodeData(agentEvent);
      }
      if (!agentEvent || typeof agentEvent !== 'object') {
        report('invalid_wire_event_json', 'event', ev.data, '', eventScope);
        return;
      }
      if (agentEvent.type === 'system') {
        return;
      }
      if (agentEvent.type === 'assistant') {
        parseAssistant(eventScope, agentEvent, eventTS);
      } else if (agentEvent.type === 'user') {
        parseUser(eventScope, agentEvent, eventTS);
      } else if (agentEvent.type === 'content_block_delta' && agentEvent.delta && agentEvent.delta.text) {
        push(eventScope, 'text', { text: String(agentEvent.delta.text) }, eventTS);
      } else if (agentEvent.type === 'result') {
        push(eventScope, 'tool_result', { text: 'Result received.' }, eventTS);
      } else {
        report('unknown_agent_event_type', agentEvent.type || 'event', agentEvent, cropText(safeJSONString(agentEvent), 400), eventScope);
      }
      return;
    }

    if (ev.type === 'finished') {
      var finishedData = decodeData(ev.data);
      if (finishedData && finishedData.wait_for_spawns) {
        push(defaultScope, 'text', { text: '[system] waiting for spawns (parent turn suspended)' }, eventTS);
      }
      return;
    }

    if (ev.type === 'raw') {
      var rawData = decodeData(ev.data);
      var rawScope = wireScope(rawData, sessionID);
      var rawText = '';
      if (typeof rawData === 'string') {
        rawText = rawData;
      } else if (rawData && typeof rawData.data === 'string') {
        rawText = rawData.data;
      } else if (ev.data && typeof ev.data === 'string') {
        rawText = ev.data;
      }
      if (rawText.trim()) {
        push(rawScope, 'text', { text: cropText(rawText) }, eventTS);
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

  var maxOverlap = Math.min(120, hist.length, live.length);
  var overlap = 0;
  for (var size = maxOverlap; size > 0; size--) {
    var matches = true;
    for (var i = 0; i < size; i++) {
      var left = hist[hist.length - size + i];
      var right = live[i];
      if (blockSignature(left) !== blockSignature(right)) {
        matches = false;
        break;
      }
    }
    if (matches) {
      overlap = size;
      break;
    }
  }

  return hist.concat(live.slice(overlap));
}

function blockSignature(block) {
  if (!block || typeof block !== 'object') return '';
  var scope = String(block._scope || block.scope || '');
  var type = String(block.type || 'text');
  if (type === 'tool_use') {
    return scope + '|' + type + '|' + stableString(block.tool) + '|' + stableString(block.input);
  }
  if (type === 'tool_result') {
    return scope + '|' + type + '|' + stableString(block.tool) + '|' + stableString(block.result) + '|' + (block.isError ? '1' : '0');
  }
  if (type === 'initial_prompt' || type === 'thinking' || type === 'text') {
    return scope + '|' + type + '|' + stableString(block.content || block.text || '');
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
