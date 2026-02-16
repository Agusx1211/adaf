import { useMemo, useEffect, useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { fetchTurnRecordingEvents } from '../../api/hooks.js';
import { normalizeStatus } from '../../utils/format.js';
import { STATUS_RUNNING } from '../../utils/colors.js';
import { reportMissingUISample } from '../../api/missingUISamples.js';
import { injectEventBlockStyles, stateEventsToBlocks } from '../common/EventBlocks.jsx';
import ChatMessageList from '../common/ChatMessageList.jsx';

export default function AgentOutput({ scope }) {
  var state = useAppState();
  var dispatch = useDispatch();
  var { streamEvents, autoScroll, sessions, historicalEvents, currentProjectID } = state;
  var [loadingHistory, setLoadingHistory] = useState(false);

  useEffect(function () { injectEventBlockStyles(); }, []);

  // Determine the turn ID for this scope
  var turnID = useMemo(function () {
    if (!scope) return 0;
    if (scope.indexOf('session-') === 0) {
      return parseInt(scope.slice(8), 10) || 0;
    }
    return 0;
  }, [scope]);

  // Check if session is running
  var sessionInfo = useMemo(function () {
    if (!turnID) return null;
    return sessions.find(function (s) { return s.id === turnID; }) || null;
  }, [turnID, sessions]);

  var isRunning = sessionInfo && STATUS_RUNNING[normalizeStatus(sessionInfo.status)];
  // Treat missing sessions as completed – the session metadata may have been
  // cleaned up, but recordings can still exist on disk.
  var isCompleted = !isRunning;

  // Live stream events for this scope
  var filteredEvents = useMemo(function () {
    if (!scope) return [];
    if (scope.indexOf('session-') === 0) {
      return streamEvents.filter(function (e) {
        return e.scope === scope || e.scope.indexOf('spawn-') === 0;
      });
    }
    if (scope.indexOf('spawn-') === 0) {
      return streamEvents.filter(function (e) { return e.scope === scope; });
    }
    return streamEvents;
  }, [streamEvents, scope]);

  var blockEvents = useMemo(function () {
    return stateEventsToBlocks(filteredEvents);
  }, [filteredEvents]);

  // Load historical events for completed sessions that have no live events
  useEffect(function () {
    if (!turnID || !isCompleted) return;
    if (blockEvents.length > 0) return;
    if (historicalEvents[turnID]) return;
    setLoadingHistory(true);
    fetchTurnRecordingEvents(turnID, currentProjectID, dispatch)
      .then(function () { setLoadingHistory(false); })
      .catch(function () { setLoadingHistory(false); });
  }, [turnID, isCompleted, blockEvents.length, historicalEvents, currentProjectID, dispatch]);

  // Convert historical recording events to display blocks.
  // Handles two formats:
  //   1. Store RecordingEvent: {type: "meta"|"claude_stream"|"stdout", data: "..."}
  //   2. Wire protocol (session events): {type: "prompt"|"event"|"raw", data: {...}}
  var historicalData = useMemo(function () {
    if (!turnID || !historicalEvents[turnID]) return { blocks: [], missingSamples: [] };
    var events = historicalEvents[turnID];
    var blocks = [];
    var missingSamples = [];
    var metaAgent = sessionInfo && sessionInfo.agent ? sessionInfo.agent : '';
    var metaModel = sessionInfo && sessionInfo.model ? sessionInfo.model : '';

    function queueMissingSample(reason, eventType, payload, fallbackText) {
      missingSamples.push({
        source: 'recording_history',
        reason: reason,
        scope: scope || ('session-' + turnID),
        session_id: turnID,
        event_type: eventType || '',
        agent: metaAgent || '',
        model: metaModel || '',
        fallback_text: fallbackText || '',
        payload: payload,
      });
    }

    function parseAgentEvent(parsed) {
      if (parsed && parsed.type === 'assistant' && parsed.message && Array.isArray(parsed.message.content)) {
        if (parsed.message.content.length === 0) {
          queueMissingSample('assistant_without_content_blocks', parsed.type || 'assistant', parsed, '[assistant event]');
          return true;
        }
        parsed.message.content.forEach(function (block) {
          if (block.type === 'text' && block.text) {
            blocks.push({ type: 'text', content: block.text });
          } else if (block.type === 'thinking' && block.text) {
            blocks.push({ type: 'thinking', content: block.text });
          } else if (block.type === 'tool_use') {
            blocks.push({ type: 'tool_use', tool: block.name || 'tool', input: block.input || '' });
          } else if (block.type === 'tool_result') {
            blocks.push({ type: 'tool_result', tool: block.name || '', result: block.content || block.output || block.text || '' });
          } else if (block && typeof block === 'object') {
            queueMissingSample('unknown_assistant_block', block.type || 'unknown', block, '');
          }
        });
        return true;
      }
      if (parsed && parsed.type === 'user' && parsed.message && Array.isArray(parsed.message.content)) {
        parsed.message.content.forEach(function (block) {
          if (block.type === 'tool_result') {
            blocks.push({ type: 'tool_result', tool: block.name || '', result: block.content || block.output || block.text || '', isError: !!block.is_error });
          } else if (block && typeof block === 'object') {
            queueMissingSample('unknown_user_block', block.type || 'unknown', block, '');
          }
        });
        return true;
      }
      if (parsed && parsed.type === 'result') {
        return true;
      }
      return false;
    }

    events.forEach(function (ev) {
      // --- Store RecordingEvent format ---
      if (ev.type === 'meta') {
        if (ev.data && typeof ev.data === 'string') {
          var sep = ev.data.indexOf('=');
          if (sep > 0) {
            var key = ev.data.slice(0, sep).trim().toLowerCase();
            var value = ev.data.slice(sep + 1).trim();
            if (key === 'agent' && value) metaAgent = value;
            if (key === 'model' && value) metaModel = value;
          }
        }
        try {
          var metaData = typeof ev.data === 'string' ? JSON.parse(ev.data) : ev.data;
          if (metaData && metaData.prompt) {
            blocks.push({ type: 'initial_prompt', content: metaData.prompt });
          } else if (metaData && metaData.objective) {
            blocks.push({ type: 'initial_prompt', content: metaData.objective });
          }
        } catch (_) {}
        return;
      }

      if (ev.type === 'claude_stream' || ev.type === 'stdout') {
        var parsedEvent = null;
        var parsedOK = false;
        try {
          parsedEvent = typeof ev.data === 'string' ? JSON.parse(ev.data) : ev.data;
          parsedOK = true;
          if (parseAgentEvent(parsedEvent)) return;
        } catch (_) {
          parsedOK = false;
        }
        if (parsedOK) {
          queueMissingSample('unparsed_recording_event', parsedEvent && parsedEvent.type ? parsedEvent.type : ev.type, parsedEvent, '');
        }
        if (ev.data && typeof ev.data === 'string' && ev.data.trim()) {
          blocks.push({ type: 'text', content: ev.data });
        }
        return;
      }

      // --- Wire protocol format (from session events file) ---
      if (ev.type === 'prompt') {
        var promptData = typeof ev.data === 'string' ? JSON.parse(ev.data) : ev.data;
        if (promptData && promptData.prompt) {
          blocks.push({ type: 'initial_prompt', content: promptData.prompt });
        }
        return;
      }

      if (ev.type === 'event') {
        try {
          var wireData = typeof ev.data === 'string' ? JSON.parse(ev.data) : ev.data;
          var agentEvent = wireData && wireData.event ? wireData.event : wireData;
          if (typeof agentEvent === 'string') agentEvent = JSON.parse(agentEvent);
          if (parseAgentEvent(agentEvent)) return;
          queueMissingSample('unparsed_wire_event', agentEvent && agentEvent.type ? agentEvent.type : ev.type, agentEvent, '');
        } catch (_) {
          queueMissingSample('invalid_wire_event_json', ev.type, ev.data, '');
        }
        return;
      }

      if (ev.type === 'raw') {
        try {
          var rawData = typeof ev.data === 'string' ? JSON.parse(ev.data) : ev.data;
          var rawText = typeof rawData === 'string' ? rawData : (rawData && rawData.data ? String(rawData.data) : '');
          if (rawText.trim()) {
            blocks.push({ type: 'text', content: rawText });
          }
        } catch (_) {
          if (ev.data && typeof ev.data === 'string' && ev.data.trim()) {
            blocks.push({ type: 'text', content: ev.data });
          }
        }
        return;
      }

      // Ignore wire lifecycle messages (started, finished, done, etc.)
    });

    return { blocks: blocks, missingSamples: missingSamples };
  }, [turnID, historicalEvents, sessionInfo, scope]);

  useEffect(function () {
    if (!historicalData.missingSamples || historicalData.missingSamples.length === 0) return;
    historicalData.missingSamples.forEach(function (sample) {
      reportMissingUISample(currentProjectID, sample);
    });
  }, [historicalData.missingSamples, currentProjectID]);

  // Use historical blocks if we have no live events
  var displayBlocks = blockEvents.length > 0 ? blockEvents : historicalData.blocks;

  // Transform flat blocks into ChatMessageList messages format
  var transformed = useMemo(function () {
    var msgs = [];
    var assistantEvents = [];

    displayBlocks.forEach(function (block) {
      if (block.type === 'initial_prompt') {
        // Flush any accumulated assistant events before the prompt
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

    // For completed sessions, flush remaining events into a final assistant message
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

    return { messages: msgs, pendingStreamEvents: isRunning ? assistantEvents : [] };
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
    />
  );
}
