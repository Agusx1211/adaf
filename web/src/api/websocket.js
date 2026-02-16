import { useEffect, useRef, useCallback } from 'react';
import { buildWSURL } from './client.js';
import { useDispatch, useAppState, normalizeSpawns } from '../state/store.js';
import { normalizeStatus, safeJSONString, stringifyToolPayload, cropText, arrayOrEmpty } from '../utils/format.js';

export function useSessionSocket(sessionID) {
  var dispatch = useDispatch();
  var wsRef = useRef(null);
  var reconnectRef = useRef(null);

  var addStreamEvent = useCallback(function (entry) {
    dispatch({
      type: 'ADD_STREAM_EVENT',
      payload: {
        id: Date.now().toString(36) + Math.random().toString(36).slice(2, 8),
        ts: Number.isFinite(Number(entry.ts)) ? Number(entry.ts) : Date.now(),
        scope: entry.scope || 'session-0',
        type: entry.type || 'text',
        text: entry.text != null ? String(entry.text) : '',
        tool: entry.tool || '',
        input: entry.input || '',
        result: entry.result || '',
      },
    });
  }, [dispatch]);

  var handleAgentStreamEvent = useCallback(function (scope, rawEvent) {
    var event = asObject(rawEvent);
    if (!event || typeof event !== 'object') {
      addStreamEvent({ scope: scope, type: 'text', text: safeJSONString(rawEvent) });
      return;
    }

    if (event.type === 'assistant') {
      var blocks = extractContentBlocks(event);
      if (!blocks.length) {
        addStreamEvent({ scope: scope, type: 'text', text: '[assistant event]' });
        return;
      }
      blocks.forEach(function (block) {
        if (!block || typeof block !== 'object') return;
        if (block.type === 'text' && block.text) {
          addStreamEvent({ scope: scope, type: 'text', text: String(block.text) });
        } else if (block.type === 'thinking' && block.text) {
          addStreamEvent({ scope: scope, type: 'thinking', text: String(block.text) });
        } else if (block.type === 'tool_use') {
          addStreamEvent({ scope: scope, type: 'tool_use', tool: block.name || 'tool', input: stringifyToolPayload(block.input || {}) });
        } else if (block.type === 'tool_result') {
          addStreamEvent({ scope: scope, type: 'tool_result', tool: block.name || 'tool_result', result: stringifyToolPayload(block.content || block.output || block.text || '') });
        } else {
          addStreamEvent({ scope: scope, type: 'text', text: safeJSONString(block) });
        }
      });
      return;
    }

    if (event.type === 'user') {
      var userBlocks = extractContentBlocks(event);
      userBlocks.forEach(function (block) {
        if (block && block.type === 'tool_result') {
          addStreamEvent({ scope: scope, type: 'tool_result', tool: block.name || 'tool_result', result: stringifyToolPayload(block.content || block.output || block.text || safeJSONString(block)) });
        }
      });
      return;
    }

    if (event.type === 'content_block_delta') {
      var delta = event.delta && (event.delta.text || event.delta.partial_json);
      if (delta) addStreamEvent({ scope: scope, type: 'text', text: String(delta) });
      return;
    }

    if (event.type === 'result') {
      addStreamEvent({ scope: scope, type: 'tool_result', text: 'Result received.' });
      return;
    }

    addStreamEvent({ scope: scope, type: 'text', text: '[' + (event.type || 'event') + '] ' + safeJSONString(event) });
  }, [addStreamEvent]);

  var ingestEnvelope = useCallback(function (sid, envelope) {
    if (!envelope || typeof envelope !== 'object') return;
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
        addStreamEvent({ scope: 'session-' + sid, type: 'initial_prompt', text: String(data.prompt) });
      }
      return;
    }

    if (type === 'event') {
      var wireEvent = data && data.event ? data.event : data;
      var eventScope = (data && data.spawn_id > 0) ? 'spawn-' + data.spawn_id : 'session-' + sid;
      handleAgentStreamEvent(eventScope, wireEvent);
      return;
    }

    if (type === 'raw') {
      var rawText = typeof data === 'string' ? data : (data && typeof data.data === 'string' ? data.data : safeJSONString(data));
      var rawScope = 'session-' + sid;
      if (data && data.spawn_id > 0) {
        rawScope = 'spawn-' + data.spawn_id;
      } else if (data && data.session_id < 0) {
        rawScope = 'spawn-' + (-data.session_id);
      }
      addStreamEvent({ scope: rawScope, type: 'text', text: cropText(rawText) });
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

    // Unknown types - silently ignore
  }, [dispatch, addStreamEvent, handleAgentStreamEvent]);

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
        addStreamEvent({ scope: 'session-' + sessionID, type: 'text', text: String(event.data || '') });
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
  }, [sessionID, dispatch, addStreamEvent, ingestEnvelope]);
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
