import { useMemo, useEffect, useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { fetchSessionRecordingEvents } from '../../api/hooks.js';
import { normalizeStatus, cropText, safeJSONString } from '../../utils/format.js';
import { STATUS_RUNNING, scopeColor } from '../../utils/colors.js';
import { reportMissingUISample } from '../../api/missingUISamples.js';
import { injectEventBlockStyles, stateEventsToBlocks } from '../common/EventBlocks.jsx';
import ChatMessageList from '../common/ChatMessageList.jsx';
import { buildSpawnScopeMaps, parseScope } from '../../utils/scopes.js';

export default function AgentOutput({ scope }) {
  var state = useAppState();
  var dispatch = useDispatch();
  var { streamEvents, autoScroll, sessions, spawns, loopRuns, historicalEvents, currentProjectID } = state;
  var [loadingHistory, setLoadingHistory] = useState(false);

  useEffect(function () { injectEventBlockStyles(); }, []);

  var scopeInfo = useMemo(function () {
    return parseScope(scope);
  }, [scope]);

  var scopeMaps = useMemo(function () {
    return buildSpawnScopeMaps(spawns, loopRuns);
  }, [spawns, loopRuns]);

  var selectedSessionID = useMemo(function () {
    if (scopeInfo.kind === 'session' || scopeInfo.kind === 'session_main') return scopeInfo.id;
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

  var sessionRunning = !!(selectedSession && STATUS_RUNNING[normalizeStatus(selectedSession.status)]);
  var spawnRunning = !!(selectedSpawn && STATUS_RUNNING[normalizeStatus(selectedSpawn.status)]);
  var isRunning = (scopeInfo.kind === 'session' || scopeInfo.kind === 'session_main')
    ? sessionRunning
    : (scopeInfo.kind === 'spawn' ? (sessionRunning || spawnRunning) : false);
  var isCompleted = !isRunning;

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
      return eventScopeMatches(scopeInfo, selectedSessionID, descendantSpawnSet, e.scope);
    }).map(function (evt) {
      return annotateSource(evt, sessionsByID, spawnsByID);
    });
  }, [streamEvents, scope, scopeInfo, selectedSessionID, descendantSpawnSet, sessionsByID, spawnsByID]);

  var blockEvents = useMemo(function () {
    return stateEventsToBlocks(filteredEvents);
  }, [filteredEvents]);

  // Historical replay for completed sessions/spawns.
  // Spawn replay is sourced from the owning daemon session event stream.
  var historySessionID = selectedSessionID;

  useEffect(function () {
    if (!historySessionID || !isCompleted) return;
    if (blockEvents.length > 0) return;
    if (historicalEvents[historySessionID]) return;
    setLoadingHistory(true);
    fetchSessionRecordingEvents(historySessionID, currentProjectID, dispatch)
      .then(function () { setLoadingHistory(false); })
      .catch(function () { setLoadingHistory(false); });
  }, [historySessionID, isCompleted, blockEvents.length, historicalEvents, currentProjectID, dispatch]);

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
      return eventScopeMatches(scopeInfo, selectedSessionID, descendantSpawnSet, ev.scope);
    }).map(function (evt) {
      return annotateSource(evt, sessionsByID, spawnsByID);
    });

    return { events: scoped, missingSamples: parsedMissing };
  }, [historySessionID, historicalEvents, scope, scopeInfo, selectedSessionID, descendantSpawnSet, sessionsByID, spawnsByID]);

  useEffect(function () {
    if (!historicalData.missingSamples || historicalData.missingSamples.length === 0) return;
    historicalData.missingSamples.forEach(function (sample) {
      reportMissingUISample(currentProjectID, sample);
    });
  }, [historicalData.missingSamples, currentProjectID]);

  var historicalBlocks = useMemo(function () {
    return stateEventsToBlocks(historicalData.events);
  }, [historicalData.events]);

  // Use historical blocks only when no live stream blocks are available.
  var displayBlocks = blockEvents.length > 0 ? blockEvents : historicalBlocks;

  // Transform flat blocks into ChatMessageList messages format.
  var transformed = useMemo(function () {
    var msgs = [];
    var assistantEvents = [];

    displayBlocks.forEach(function (block) {
      if (block.type === 'initial_prompt') {
        if (assistantEvents.length > 0) {
          msgs.push({
            id: 'assistant-' + msgs.length,
            role: 'assistant',
            content: '',
            events: assistantEvents,
            created_at: null,
          });
          assistantEvents = [];
        }
        msgs.push({
          id: 'prompt-' + msgs.length,
          role: 'user',
          content: block.content,
          created_at: null,
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
        created_at: null,
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
      loading={loadingHistory}
      emptyMessage="No output yet"
      autoScroll={autoScroll}
      showSourceLabels={scopeInfo.kind === 'session'}
      scrollContextKey={scope || ''}
    />
  );
}

function eventScopeMatches(scopeInfo, selectedSessionID, descendantSpawnSet, eventScope) {
  var scope = String(eventScope || '');
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

  function push(scope, type, payload) {
    output.push(Object.assign({ scope: scope || defaultScope, type: type }, payload || {}));
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

  function parseAssistant(scope, parsed) {
    var blocks = extractContentBlocks(parsed);
    if (!blocks.length) {
      report('assistant_without_content_blocks', parsed && parsed.type ? parsed.type : 'assistant', parsed, '[assistant event]', scope);
      push(scope, 'text', { text: '[assistant event]' });
      return;
    }
    blocks.forEach(function (block) {
      if (!block || typeof block !== 'object') return;
      if (block.type === 'text' && block.text) {
        push(scope, 'text', { text: String(block.text) });
      } else if (block.type === 'thinking' && block.text) {
        push(scope, 'thinking', { text: String(block.text) });
      } else if (block.type === 'tool_use') {
        push(scope, 'tool_use', { tool: block.name || 'tool', input: block.input || {} });
      } else if (block.type === 'tool_result') {
        push(scope, 'tool_result', {
          tool: block.name || 'tool_result',
          result: block.content || block.output || block.text || '',
          isError: !!block.is_error,
        });
      } else {
        report('unknown_assistant_block', block.type || 'unknown', block, cropText(safeJSONString(block), 400), scope);
      }
    });
  }

  function parseUser(scope, parsed) {
    var blocks = extractContentBlocks(parsed);
    blocks.forEach(function (block) {
      if (block && block.type === 'tool_result') {
        push(scope, 'tool_result', {
          tool: block.name || 'tool_result',
          result: block.content || block.output || block.text || safeJSONString(block),
          isError: !!block.is_error,
        });
      } else if (block && typeof block === 'object') {
        report('unknown_user_block', block.type || 'unknown', block, '', scope);
      }
    });
  }

  list.forEach(function (ev) {
    if (!ev || typeof ev !== 'object') return;

    if (ev.type === 'meta') {
      var metaData = decodeData(ev.data);
      if (metaData && metaData.prompt) {
        push(defaultScope, 'initial_prompt', { text: String(metaData.prompt) });
      } else if (metaData && metaData.objective) {
        push(defaultScope, 'initial_prompt', { text: String(metaData.objective) });
      }
      return;
    }

    if (ev.type === 'claude_stream' || ev.type === 'stdout') {
      var parsedStoreEvent = decodeData(ev.data);
      if (parsedStoreEvent && typeof parsedStoreEvent === 'object' && parsedStoreEvent.type) {
        if (parsedStoreEvent.type === 'assistant') {
          parseAssistant(defaultScope, parsedStoreEvent);
          return;
        }
        if (parsedStoreEvent.type === 'user') {
          parseUser(defaultScope, parsedStoreEvent);
          return;
        }
        if (parsedStoreEvent.type === 'content_block_delta' && parsedStoreEvent.delta && parsedStoreEvent.delta.text) {
          push(defaultScope, 'text', { text: String(parsedStoreEvent.delta.text) });
          return;
        }
      }
      if (ev.data && String(ev.data).trim()) {
        push(defaultScope, 'text', { text: String(ev.data) });
      }
      return;
    }

    if (ev.type === 'prompt') {
      var promptData = decodeData(ev.data);
      var promptScope = wireScope(promptData, sessionID);
      if (promptData && promptData.prompt) {
        push(promptScope, 'initial_prompt', { text: String(promptData.prompt) });
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
      if (agentEvent.type === 'assistant') {
        parseAssistant(eventScope, agentEvent);
      } else if (agentEvent.type === 'user') {
        parseUser(eventScope, agentEvent);
      } else if (agentEvent.type === 'content_block_delta' && agentEvent.delta && agentEvent.delta.text) {
        push(eventScope, 'text', { text: String(agentEvent.delta.text) });
      } else if (agentEvent.type === 'result') {
        push(eventScope, 'tool_result', { text: 'Result received.' });
      } else {
        report('unknown_agent_event_type', agentEvent.type || 'event', agentEvent, cropText(safeJSONString(agentEvent), 400), eventScope);
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
        push(rawScope, 'text', { text: cropText(rawText) });
      }
    }
  });

  return output;
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
