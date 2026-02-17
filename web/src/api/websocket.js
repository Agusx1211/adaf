import { useEffect, useRef, useCallback } from 'react';
import { buildWSURL } from './client.js';
import { useDispatch, useAppState, normalizeSpawns } from '../state/store.js';
import { safeJSONString, stringifyToolPayload, cropText } from '../utils/format.js';
import { reportMissingUISample } from './missingUISamples.js';

export function useSessionSocket(sessionID) {
  var state = useAppState();
  var dispatch = useDispatch();
  var wsRef = useRef(null);
  var reconnectRef = useRef(null);
  var contextRef = useRef({ projectID: '', sessions: [] });
  var activeTurnBySessionRef = useRef({});

  useEffect(function () {
    contextRef.current = {
      projectID: state.currentProjectID || '',
      sessions: Array.isArray(state.sessions) ? state.sessions : [],
    };
  }, [state.currentProjectID, state.sessions]);

  var addStreamEvent = useCallback(function (entry) {
    entry = entry || {};
    var normalized = {
      id: Date.now().toString(36) + Math.random().toString(36).slice(2, 8),
      ts: Number.isFinite(Number(entry.ts)) ? Number(entry.ts) : Date.now(),
      scope: entry.scope || 'session-0',
      type: entry.type || 'text',
      text: entry.text != null ? String(entry.text) : '',
      tool: entry.tool || '',
      input: entry.input || '',
      result: entry.result || '',
    };
    dispatch({
      type: 'ADD_STREAM_EVENT',
      payload: Object.assign({}, entry || {}, normalized),
    });
  }, [dispatch]);

  var reportMissing = useCallback(function (sample) {
    if (!sample || typeof sample !== 'object') return;

    var context = contextRef.current || {};
    var sid = Number(sample.session_id || 0);
    if (!sid && sessionID) sid = sessionID;

    var sessionMeta = findSessionByID(context.sessions, sid);

    reportMissingUISample(context.projectID, {
      source: sample.source || 'session_ws_event',
      reason: sample.reason || 'unknown_parse_gap',
      scope: sample.scope || (sid ? 'session-' + sid : ''),
      session_id: sid || 0,
      turn_id: Number(sample.turn_id || 0) || 0,
      spawn_id: Number(sample.spawn_id || 0) || 0,
      event_type: sample.event_type || '',
      agent: sample.agent || (sessionMeta && sessionMeta.agent) || '',
      model: sample.model || (sessionMeta && sessionMeta.model) || '',
      provider: sample.provider || '',
      fallback_text: sample.fallback_text || '',
      payload: sample.payload,
    });
  }, [sessionID]);

  var handleAgentStreamEvent = useCallback(function (scope, rawEvent, meta) {
    var turnID = Number(meta && meta.turnID || 0);
    function withTurn(entry) {
      if (turnID > 0) {
        return Object.assign({}, entry, { turn_id: turnID });
      }
      return entry;
    }
    var event = asObject(rawEvent);
    if (!event || typeof event !== 'object') {
      var fallbackRaw = safeJSONString(rawEvent);
      addStreamEvent(withTurn({ scope: scope, type: 'text', text: fallbackRaw }));
      reportMissing({
        source: 'session_ws_event',
        reason: 'event_not_object',
        scope: scope,
        session_id: sessionID,
        turn_id: turnID,
        event_type: typeof rawEvent,
        fallback_text: cropText(fallbackRaw, 400),
        payload: rawEvent,
      });
      return;
    }

    if (event.type === 'system') {
      // Ignore transport/system frames such as "subtype:init" so they do not pollute chat output.
      return;
    }

    if (event.type === 'assistant') {
      var blocks = extractContentBlocks(event);
      if (!blocks.length) {
        addStreamEvent(withTurn({ scope: scope, type: 'text', text: '[assistant event]' }));
        reportMissing({
          source: 'session_ws_event',
          reason: 'assistant_without_content_blocks',
          scope: scope,
          session_id: sessionID,
          turn_id: turnID,
          event_type: event.type || 'assistant',
          fallback_text: '[assistant event]',
          payload: event,
        });
        return;
      }
      blocks.forEach(function (block) {
        if (!block || typeof block !== 'object') return;
        if (block.type === 'text' && block.text) {
          addStreamEvent(withTurn({ scope: scope, type: 'text', text: String(block.text) }));
        } else if (block.type === 'thinking' && block.text) {
          addStreamEvent(withTurn({ scope: scope, type: 'thinking', text: String(block.text) }));
        } else if (block.type === 'tool_use') {
          addStreamEvent(withTurn({ scope: scope, type: 'tool_use', tool: block.name || 'tool', input: stringifyToolPayload(block.input || {}) }));
        } else if (block.type === 'tool_result') {
          addStreamEvent(withTurn({ scope: scope, type: 'tool_result', tool: block.name || 'tool_result', result: stringifyToolPayload(block.content || block.output || block.text || '') }));
        } else {
          var blockFallback = safeJSONString(block);
          addStreamEvent(withTurn({ scope: scope, type: 'text', text: blockFallback }));
          reportMissing({
            source: 'session_ws_event',
            reason: 'unknown_assistant_block',
            scope: scope,
            session_id: sessionID,
            turn_id: turnID,
            event_type: block.type || 'unknown',
            fallback_text: cropText(blockFallback, 400),
            payload: block,
          });
        }
      });
      return;
    }

    if (event.type === 'user') {
      var userBlocks = extractContentBlocks(event);
      userBlocks.forEach(function (block) {
        if (block && block.type === 'tool_result') {
          addStreamEvent(withTurn({ scope: scope, type: 'tool_result', tool: block.name || 'tool_result', result: stringifyToolPayload(block.content || block.output || block.text || safeJSONString(block)) }));
          return;
        }
        if (!block || typeof block !== 'object') return;
        reportMissing({
          source: 'session_ws_event',
          reason: 'unknown_user_block',
          scope: scope,
          session_id: sessionID,
          turn_id: turnID,
          event_type: block.type || 'unknown',
          payload: block,
        });
      });
      return;
    }

    if (event.type === 'content_block_delta') {
      var delta = event.delta && (event.delta.text || event.delta.partial_json);
      if (delta) {
        addStreamEvent(withTurn({ scope: scope, type: 'text', text: String(delta) }));
      } else {
        reportMissing({
          source: 'session_ws_event',
          reason: 'content_block_delta_without_renderable_text',
          scope: scope,
          session_id: sessionID,
          turn_id: turnID,
          event_type: event.type,
          payload: event,
        });
      }
      return;
    }

    if (event.type === 'result') {
      addStreamEvent(withTurn({ scope: scope, type: 'tool_result', text: 'Result received.' }));
      return;
    }

    var fallback = '[' + (event.type || 'event') + '] ' + safeJSONString(event);
    addStreamEvent(withTurn({ scope: scope, type: 'text', text: fallback }));
    reportMissing({
      source: 'session_ws_event',
      reason: 'unknown_agent_event_type',
      scope: scope,
      session_id: sessionID,
      turn_id: turnID,
      event_type: event.type || 'event',
      fallback_text: cropText(fallback, 400),
      payload: event,
    });
  }, [addStreamEvent, reportMissing, sessionID]);

  var ingestEnvelope = useCallback(function (sid, envelope) {
    if (!envelope || typeof envelope !== 'object') return;
    function positiveTurnID(value) {
      var n = Number(value || 0);
      if (!Number.isFinite(n) || n <= 0) return 0;
      return n;
    }
    function resolveTurnID(candidate) {
      var resolved = positiveTurnID(candidate);
      if (resolved > 0) {
        activeTurnBySessionRef.current[sid] = resolved;
        return resolved;
      }
      return positiveTurnID(activeTurnBySessionRef.current[sid]);
    }
    var type = envelope.type || 'event';
    var data = envelope.data;

    if (type === 'snapshot') {
      if (data && Array.isArray(data.spawns)) {
        dispatch({ type: 'MERGE_SPAWNS', payload: normalizeSpawns(data.spawns) });
      }
      // Replay recent messages from snapshot (includes prompt, event, etc.)
      if (data && Array.isArray(data.recent)) {
        data.recent.forEach(function (recentMsg) {
          if (recentMsg && recentMsg.type) {
            ingestEnvelope(sid, recentMsg);
          }
        });
      }
      return;
    }

    if (type === 'prompt') {
      // Extract and display the prompt as a formatted block
      if (data && data.prompt) {
        var promptScope = 'session-' + sid;
        var promptSpawnID = Number(data.spawn_id || data.spawnID || 0);
        var promptTurnID = Number(data.turn_id || data.turnID || 0);
        if (Number.isFinite(promptSpawnID) && promptSpawnID > 0) {
          promptScope = 'spawn-' + promptSpawnID;
        } else {
          var promptSessionID = Number(data.session_id || data.sessionID || 0);
          if (Number.isFinite(promptSessionID) && promptSessionID < 0) {
            promptScope = 'spawn-' + (-promptSessionID);
          } else if (Number.isFinite(promptSessionID) && promptSessionID > 0) {
            // In loop mode, prompt.session_id is the store turn ID.
            if (promptTurnID <= 0) promptTurnID = promptSessionID;
          }
        }
        promptTurnID = resolveTurnID(promptTurnID);
        addStreamEvent({
          scope: promptScope,
          type: 'initial_prompt',
          text: String(data.prompt),
          turn_hex_id: data.turn_hex_id || data.turnHexID || '',
          turn_id: promptTurnID > 0 ? promptTurnID : 0,
          is_resume: !!(data.is_resume || data.isResume),
        });
      }
      return;
    }

    if (type === 'event') {
      var wireEvent = data && data.event ? data.event : data;
      var eventScope = 'session-' + sid;
      var eventSpawnID = Number(data && (data.spawn_id || data.spawnID) || 0);
      var eventTurnID = Number(data && (data.turn_id || data.turnID) || 0);
      if (Number.isFinite(eventSpawnID) && eventSpawnID > 0) {
        eventScope = 'spawn-' + eventSpawnID;
      } else {
        var eventSessionID = Number(data && (data.session_id || data.sessionID) || 0);
        if (Number.isFinite(eventSessionID) && eventSessionID < 0) {
          eventScope = 'spawn-' + (-eventSessionID);
        } else if (eventTurnID <= 0 && Number.isFinite(eventSessionID) && eventSessionID > 0) {
          // In loop-mode wire payloads, positive session_id may be a turn id.
          eventTurnID = eventSessionID;
        }
      }
      if (eventTurnID <= 0 && wireEvent && typeof wireEvent === 'object') {
        eventTurnID = Number(wireEvent.turn_id || wireEvent.turnID || 0);
      }
      eventTurnID = resolveTurnID(eventTurnID);
      handleAgentStreamEvent(eventScope, wireEvent, { turnID: eventTurnID });
      return;
    }

    if (type === 'finished') {
      if (data && data.wait_for_spawns) {
        var waitingTurnID = resolveTurnID(data.turn_id || data.turnID || data.session_id || data.sessionID || 0);
        addStreamEvent({
          scope: 'session-' + sid,
          type: 'text',
          text: '[system] waiting for spawns (parent turn suspended)',
          turn_id: waitingTurnID > 0 ? waitingTurnID : 0,
        });
      }
      return;
    }

    if (type === 'raw') {
      var rawText = typeof data === 'string' ? data : (data && typeof data.data === 'string' ? data.data : safeJSONString(data));
      var rawScope = 'session-' + sid;
      var rawTurnID = Number(data && (data.turn_id || data.turnID) || 0);
      if (data && data.spawn_id > 0) {
        rawScope = 'spawn-' + data.spawn_id;
      } else if (data && data.session_id < 0) {
        rawScope = 'spawn-' + (-data.session_id);
      } else if (rawTurnID <= 0 && data && Number(data.session_id) > 0) {
        rawTurnID = Number(data.session_id);
      }
      rawTurnID = resolveTurnID(rawTurnID);
      addStreamEvent({ scope: rawScope, type: 'text', text: cropText(rawText), turn_id: rawTurnID > 0 ? rawTurnID : 0 });
      return;
    }

    if (type === 'spawn') {
      if (data && data.spawns) {
        dispatch({ type: 'MERGE_SPAWNS', payload: normalizeSpawns(data.spawns) });
      }
      return;
    }

    // Suppress noisy status messages from output
    if (type === 'started' || type === 'finished' || type === 'done') {
      return;
    }

    if (type === 'error') {
      if (data && data.error) {
        addStreamEvent({ scope: 'session-' + sid, type: 'text', text: 'Error: ' + String(data.error) });
      }
      return;
    }

    // Suppress loop lifecycle events from output
    if (type === 'loop_step_start' || type === 'loop_step_end' || type === 'loop_done') {
      return;
    }

    reportMissing({
      source: 'session_ws_envelope',
      reason: 'unknown_envelope_type',
      scope: 'session-' + sid,
      session_id: sid,
      event_type: type,
      payload: data,
    });
  }, [dispatch, addStreamEvent, handleAgentStreamEvent, reportMissing]);

  useEffect(function () {
    if (!sessionID) return;

    if (wsRef.current) {
      try { wsRef.current.close(); } catch (_) {}
      wsRef.current = null;
    }

    try {
      var ws = new WebSocket(buildWSURL('/ws/sessions/' + encodeURIComponent(String(sessionID))));
      wsRef.current = ws;
    } catch (_) {
      dispatch({ type: 'SET', payload: { wsConnected: false } });
      return;
    }

    wsRef.current.addEventListener('open', function () {
      dispatch({ type: 'SET', payload: { wsConnected: true, currentSessionSocketID: sessionID } });
    });

    wsRef.current.addEventListener('message', function (event) {
      try {
        var payload = JSON.parse(event.data);
        ingestEnvelope(sessionID, payload);
      } catch (_) {
        var rawText = String(event.data || '');
        addStreamEvent({ scope: 'session-' + sessionID, type: 'text', text: rawText });
        reportMissing({
          source: 'session_ws_message',
          reason: 'invalid_ws_payload_json',
          scope: 'session-' + sessionID,
          session_id: sessionID,
          event_type: 'message',
          fallback_text: cropText(rawText, 400),
          payload: rawText,
        });
      }
    });

    wsRef.current.addEventListener('error', function () {
      dispatch({ type: 'SET', payload: { wsConnected: false } });
    });

    wsRef.current.addEventListener('close', function () {
      dispatch({ type: 'SET', payload: { wsConnected: false } });
      wsRef.current = null;

      reconnectRef.current = setTimeout(function () {
        // Will reconnect on next effect cycle
      }, 1800);
    });

    return function () {
      if (reconnectRef.current) clearTimeout(reconnectRef.current);
      if (wsRef.current) {
        try { wsRef.current.close(); } catch (_) {}
        wsRef.current = null;
      }
    };
  }, [sessionID, dispatch, addStreamEvent, ingestEnvelope, reportMissing]);
}

export function useTerminalSocket(terminalRef) {
  var dispatch = useDispatch();
  var wsRef = useRef(null);

  var connect = useCallback(function () {
    if (wsRef.current) {
      try { wsRef.current.close(); } catch (_) {}
    }

    try {
      wsRef.current = new WebSocket(buildWSURL('/ws/terminal'));
    } catch (_) {
      dispatch({ type: 'SET', payload: { termWSConnected: false } });
      return;
    }

    wsRef.current.addEventListener('open', function () {
      dispatch({ type: 'SET', payload: { termWSConnected: true } });
    });

    wsRef.current.addEventListener('message', function (event) {
      if (terminalRef && terminalRef.current) {
        terminalRef.current.write(event.data);
      }
    });

    wsRef.current.addEventListener('close', function () {
      dispatch({ type: 'SET', payload: { termWSConnected: false } });
      wsRef.current = null;
    });

    wsRef.current.addEventListener('error', function () {
      dispatch({ type: 'SET', payload: { termWSConnected: false } });
    });
  }, [dispatch, terminalRef]);

  var disconnect = useCallback(function () {
    if (wsRef.current) {
      try { wsRef.current.close(); } catch (_) {}
      wsRef.current = null;
    }
    dispatch({ type: 'SET', payload: { termWSConnected: false } });
  }, [dispatch]);

  var sendData = useCallback(function (data) {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(data);
    }
  }, []);

  return { connect, disconnect, sendData };
}

function asObject(value) {
  if (value == null) return null;
  if (typeof value === 'object') return value;
  if (typeof value === 'string') {
    try { return JSON.parse(value); } catch (_) { return { text: value }; }
  }
  return null;
}

function extractContentBlocks(event) {
  if (!event || typeof event !== 'object') return [];
  if (event.message && Array.isArray(event.message.content)) return event.message.content.slice();
  if (event.content_block && typeof event.content_block === 'object') return [event.content_block];
  return [];
}

function findSessionByID(sessions, id) {
  if (!Array.isArray(sessions)) return null;
  var targetID = Number(id || 0);
  if (!targetID) return null;
  for (var i = 0; i < sessions.length; i++) {
    if (Number(sessions[i] && sessions[i].id) === targetID) return sessions[i];
  }
  return null;
}
