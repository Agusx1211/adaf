import { useState, useEffect, useRef, useCallback } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiCall, apiBase, buildWSURL } from '../../api/client.js';
import { reportMissingUISample } from '../../api/missingUISamples.js';
import { useToast } from '../common/Toast.jsx';
import { injectEventBlockStyles, cleanResponse } from '../common/EventBlocks.jsx';
import ChatMessageList from '../common/ChatMessageList.jsx';
import { agentInfo, statusColor, STATUS_RUNNING } from '../../utils/colors.js';
import { normalizeStatus } from '../../utils/format.js';

export default function StandaloneChatView() {
  var state = useAppState();
  var dispatch = useDispatch();
  var showToast = useToast();
  var chatID = state.standaloneChatID || '';
  var [chatMeta, setChatMeta] = useState(null);
  var [messages, setMessages] = useState([]);
  var [loading, setLoading] = useState(false);
  var [sending, setSending] = useState(false);
  var [input, setInput] = useState('');
  var [activeSessionID, setActiveSessionID] = useState(null);
  var [streamEvents, setStreamEvents] = useState([]);
  var [teams, setTeams] = useState([]);
  var [showTeamDropdown, setShowTeamDropdown] = useState(false);
  var [spawns, setSpawns] = useState([]);
  var [focusScope, setFocusScope] = useState('parent');
  var inputRef = useRef(null);
  var teamDropdownRef = useRef(null);
  var base = apiBase(state.currentProjectID);

  // Refs for per-chat session management
  var currentChatIDRef = useRef(chatID);
  var chatSessionsRef = useRef({}); // { [chatID]: { sessionID, events, spawnEvents, allEvents, spawns, ws, sending, finalized, promptData, promptsByScope, base, parentProfile } }
  var dispatchRef = useRef(dispatch);
  dispatchRef.current = dispatch;
  var focusScopeRef = useRef('parent');

  useEffect(function () { injectEventBlockStyles(); }, []);

  // Keep currentChatIDRef in sync
  currentChatIDRef.current = chatID;

  // When chatID changes, swap React state to reflect the new chat's session
  useEffect(function () {
    var entry = chatSessionsRef.current[chatID];
    if (entry && entry.sending && !entry.finalized) {
      setSending(true);
      setStreamEvents(entry.events.slice());
      setActiveSessionID(entry.sessionID);
      setSpawns(entry.spawns ? entry.spawns.slice() : []);
    } else {
      setSending(false);
      setStreamEvents([]);
      setActiveSessionID(null);
      setSpawns([]);
    }
    setFocusScope('parent');
    focusScopeRef.current = 'parent';
  }, [chatID]);

  // Cleanup all WebSockets on unmount
  useEffect(function () {
    return function () {
      Object.keys(chatSessionsRef.current).forEach(function (id) {
        var entry = chatSessionsRef.current[id];
        if (entry && entry.ws && entry.ws.readyState <= 1) {
          try { entry.ws.close(); } catch (_) {}
        }
      });
    };
  }, []);

  // Load messages when chat instance changes
  var loadMessages = useCallback(async function () {
    if (!state.currentProjectID || !chatID) {
      setMessages([]);
      setChatMeta(null);
      return;
    }
    setLoading(true);
    try {
      var data = await apiCall(base + '/chat-instances/' + encodeURIComponent(chatID), 'GET', null, { allow404: true });
      setMessages(data || []);
    } catch (err) {
      if (!err.authRequired) console.error('Failed to load chat messages:', err);
      setMessages([]);
    } finally {
      setLoading(false);
    }
  }, [base, state.currentProjectID, chatID]);

  useEffect(function () { loadMessages(); }, [loadMessages]);

  // Load chat instance metadata
  useEffect(function () {
    if (!chatID || !state.currentProjectID) {
      setChatMeta(null);
      return;
    }
    apiCall(base + '/chat-instances', 'GET', null, { allow404: true })
      .then(function (list) {
        var found = (list || []).find(function (i) { return i.id === chatID; });
        setChatMeta(found || null);
      })
      .catch(function () {});
  }, [chatID, base, state.currentProjectID]);

  // Load available teams
  useEffect(function () {
    apiCall('/api/config/teams', 'GET', null, { allow404: true })
      .then(function (list) { setTeams(list || []); })
      .catch(function () {});
  }, [state.currentProjectID]);

  // Close team dropdown on outside click
  useEffect(function () {
    if (!showTeamDropdown) return;
    function handleClick(e) {
      if (teamDropdownRef.current && !teamDropdownRef.current.contains(e.target)) {
        setShowTeamDropdown(false);
      }
    }
    document.addEventListener('mousedown', handleClick);
    return function () { document.removeEventListener('mousedown', handleClick); };
  }, [showTeamDropdown]);

  async function handleTeamChange(newTeam) {
    setShowTeamDropdown(false);
    try {
      var updated = await apiCall(base + '/chat-instances/' + encodeURIComponent(chatID), 'PATCH', { team: newTeam, skills: chatMeta ? chatMeta.skills || [] : [] });
      setChatMeta(updated);
      showToast('Team updated', 'success');
    } catch (err) {
      if (err && err.authRequired) return;
      showToast('Failed to update team: ' + (err.message || err), 'error');
    }
  }

  // --- Per-chat WebSocket management ---

  function pushEventForChat(forChatID, evt, spawnID) {
    var entry = chatSessionsRef.current[forChatID];
    if (!entry) return;
    var eventScope = (spawnID && spawnID > 0) ? ('spawn-' + spawnID) : 'parent';
    var scopedEvt = Object.assign({}, evt, { scope: eventScope });

    // Route to correct events array
    var events;
    if (spawnID && spawnID > 0) {
      if (!entry.spawnEvents[spawnID]) entry.spawnEvents[spawnID] = [];
      events = entry.spawnEvents[spawnID];
    } else {
      events = entry.events;
    }

    var last = events.length > 0 ? events[events.length - 1] : null;
    if (scopedEvt.type === 'text' && last && last.type === 'text') {
      last.content += scopedEvt.content;
    } else if (scopedEvt.type === 'thinking' && last && last.type === 'thinking') {
      last.content += scopedEvt.content;
    } else {
      events.push(scopedEvt);
    }

    // Also push to allEvents with source tag
    var sourceLabel = '';
    var sourceColor = '';
    if (spawnID && spawnID > 0) {
      var spawnInfo = (entry.spawns || []).find(function (s) { return s.id === spawnID; });
      sourceLabel = spawnInfo ? spawnInfo.profile : ('spawn-' + spawnID);
      sourceColor = agentInfo(sourceLabel).color;
    } else {
      sourceLabel = entry.parentProfile || 'parent';
      sourceColor = agentInfo(entry.parentProfile || '').color;
    }
    var taggedEvt = Object.assign({}, scopedEvt, { _sourceLabel: sourceLabel, _sourceColor: sourceColor, _spawnID: spawnID || 0 });
    var allLast = entry.allEvents.length > 0 ? entry.allEvents[entry.allEvents.length - 1] : null;
    if (taggedEvt.type === 'text' && allLast && allLast.type === 'text' && allLast._spawnID === taggedEvt._spawnID) {
      allLast.content += taggedEvt.content;
    } else if (taggedEvt.type === 'thinking' && allLast && allLast.type === 'thinking' && allLast._spawnID === taggedEvt._spawnID) {
      allLast.content += taggedEvt.content;
    } else {
      entry.allEvents.push(taggedEvt);
    }

    // Update React state if this is the currently viewed chat
    if (forChatID === currentChatIDRef.current) {
      var scope = focusScopeRef.current;
      if (scope === 'parent' && (!spawnID || spawnID === 0)) {
        setStreamEvents(entry.events.slice());
      } else if (scope === 'all') {
        setStreamEvents(entry.allEvents.slice());
      } else if (scope.indexOf('spawn-') === 0 && spawnID && spawnID > 0) {
        var scopeSID = parseInt(scope.slice(6));
        if (scopeSID === spawnID) {
          setStreamEvents(entry.spawnEvents[spawnID].slice());
        }
      }
    }

    // Update global status to 'responding' once we have content
    if (scopedEvt.type === 'text' || scopedEvt.type === 'tool_use' || scopedEvt.type === 'tool_result') {
      dispatchRef.current({ type: 'SET_STANDALONE_CHAT_STATUS', payload: { chatID: forChatID, status: 'responding' } });
    }
  }

  function isSpawnStillActiveStatus(status) {
    var normalized = normalizeStatus(status);
    return !!STATUS_RUNNING[normalized] || normalized === 'awaiting_input';
  }

  function markSpawnsAsStopped(spawnList) {
    var list = Array.isArray(spawnList) ? spawnList : [];
    return list.map(function (spawn) {
      if (!spawn || !isSpawnStillActiveStatus(spawn.status)) return spawn;
      return Object.assign({}, spawn, { status: 'canceled' });
    });
  }

  function finalizeChat(forChatID, reason) {
    var entry = chatSessionsRef.current[forChatID];
    if (!entry || entry.finalized) return;
    entry.finalized = true;
    entry.sending = false;

    if (reason === 'stopped') {
      entry.spawns = markSpawnsAsStopped(entry.spawns);
    }

    // Some resumed sessions do not replay previous assistant turns. In that case,
    // the skip counter can hide the only assistant event; recover it as fallback.
    if (entry.events.length === 0 && entry.skippedAssistantFallback && entry.skippedAssistantFallback.length > 0) {
      entry.events = entry.skippedAssistantFallback.slice();
    }

    var textParts = [];
    entry.events.forEach(function (e) {
      if (e.type === 'text') textParts.push(e.content);
    });
    var finalText = cleanResponse(textParts.join(''));

    var payload = buildStandaloneEventPayload(entry.events, entry.allEvents, entry.spawnEvents);
    var hasPayloadEvents = payload.parent.length > 0 || payload.all.length > 0;
    if (finalText || hasPayloadEvents) {
      saveAssistantResponseForChat(forChatID, finalText || '(no text output)', payload, entry.promptData, entry.base);
    }

    // Close WS
    if (entry.ws && entry.ws.readyState <= 1) {
      try { entry.ws.close(); } catch (_) {}
    }

    // Clean up entry
    delete chatSessionsRef.current[forChatID];

    // Update global status
    dispatchRef.current({ type: 'SET_STANDALONE_CHAT_STATUS', payload: { chatID: forChatID, status: 'idle' } });

    // Update React state if current chat
    // Keep spawns visible so user can see final state — cleared on next message send
    if (forChatID === currentChatIDRef.current) {
      setSending(false);
      setActiveSessionID(null);
      setStreamEvents([]);
      setSpawns(entry.spawns ? entry.spawns.slice() : []);
    }
  }

  function startSessionWS(forChatID, sessionID) {
    var entry = chatSessionsRef.current[forChatID];
    if (!entry) return;

    var ws;
    try {
      ws = new WebSocket(buildWSURL('/ws/sessions/' + encodeURIComponent(String(sessionID))));
      entry.ws = ws;
    } catch (e) {
      console.error('Standalone Chat WebSocket error:', e);
      entry.sending = false;
      delete chatSessionsRef.current[forChatID];
      if (forChatID === currentChatIDRef.current) {
        setSending(false);
      }
      dispatchRef.current({ type: 'SET_STANDALONE_CHAT_STATUS', payload: { chatID: forChatID, status: 'idle' } });
      return;
    }

    // Per-spawn streaming state to avoid double-counting
    var streamState = {}; // { [spawnID]: { isStreaming, hasRawText } }
    function getStreamState(sid) {
      var key = sid || 0;
      if (!streamState[key]) streamState[key] = { isStreaming: false, hasRawText: false };
      return streamState[key];
    }

    // Track how many assistant turns we've seen to skip replayed old turns.
    var assistantTurnsSeen = 0;
    var turnsToSkip = entry.assistantTurnsToSkip || 0;

    function reportMissing(sample) {
      if (!sample || typeof sample !== 'object') return;
      reportMissingUISample(state.currentProjectID, {
        source: sample.source || 'standalone_ws_event',
        reason: sample.reason || 'unknown_parse_gap',
        scope: sample.scope || ('chat-' + forChatID),
        session_id: sample.session_id || sessionID || 0,
        event_type: sample.event_type || '',
        agent: sample.agent || (chatMeta && chatMeta.agent) || '',
        model: sample.model || (chatMeta && chatMeta.model) || '',
        fallback_text: sample.fallback_text || '',
        payload: sample.payload,
      });
    }

    ws.addEventListener('message', function (wsEvent) {
      try {
        var envelope = JSON.parse(wsEvent.data);
        var type = envelope.type;
        var data = envelope.data;

        // Debug: log all WS message types and any spawn_id present
        if (type !== 'event' && type !== 'raw') {
          console.log('[ADAF-DEBUG] ws msg type=' + type, data && typeof data === 'object' ? JSON.stringify(data).slice(0, 300) : '');
        }

        if (type === 'prompt' && data) {
          var promptText = data.text || data.prompt || '';
          var promptSessionID = Number(data.session_id || data.sessionID || 0) || 0;
          var promptSpawnID = promptSessionID < 0 ? -promptSessionID : 0;
          var promptPayload = {
            text: promptText,
            truncated: !!data.truncated,
            turn_id: data.turn_id || null,
            session_id: promptSessionID || null,
          };
          if (!entry.promptsByScope) entry.promptsByScope = {};
          entry.promptsByScope[promptSpawnID > 0 ? ('spawn-' + promptSpawnID) : 'parent'] = promptPayload;
          if (promptSpawnID === 0) {
            entry.promptData = promptPayload;
          }
          if (promptText) {
            pushEventForChat(forChatID, { type: 'initial_prompt', content: promptText }, promptSpawnID);
          }
          return;
        }

        // Handle spawn hierarchy updates
        if (type === 'spawn' && data && data.spawns) {
          var oldSpawns = entry.spawns || [];
          var newSpawns = data.spawns;

          // Inject status change events into allEvents for the "all" view
          newSpawns.forEach(function (ns) {
            var old = oldSpawns.find(function (os) { return os.id === ns.id; });
            if (!old) {
              // New spawn started
              entry.allEvents.push({
                type: '_spawn_status', _spawnID: ns.id,
                _action: 'started', _profile: ns.profile, _role: ns.role || '',
                _sourceLabel: ns.profile, _sourceColor: agentInfo(ns.profile).color,
                scope: 'spawn-' + ns.id,
              });
            } else if (old.status !== ns.status) {
              if (ns.status === 'completed' || ns.status === 'failed') {
                entry.allEvents.push({
                  type: '_spawn_status', _spawnID: ns.id,
                  _action: ns.status, _profile: ns.profile, _role: ns.role || '',
                  _sourceLabel: ns.profile, _sourceColor: agentInfo(ns.profile).color,
                  scope: 'spawn-' + ns.id,
                });
              }
              if (ns.status === 'awaiting_input' && ns.question) {
                entry.allEvents.push({
                  type: '_spawn_status', _spawnID: ns.id,
                  _action: 'asking', _profile: ns.profile, _question: ns.question,
                  _sourceLabel: ns.profile, _sourceColor: agentInfo(ns.profile).color,
                  scope: 'spawn-' + ns.id,
                });
              }
            }
          });

          entry.spawns = newSpawns;
          if (forChatID === currentChatIDRef.current) {
            setSpawns(newSpawns.slice());
            if (focusScopeRef.current === 'all') {
              setStreamEvents(entry.allEvents.slice());
            }
          }
          return;
        }

        // Handle snapshot (extract spawns if present)
        if (type === 'snapshot' && data) {
          if (data.spawns && data.spawns.length > 0) {
            entry.spawns = data.spawns;
            if (forChatID === currentChatIDRef.current) {
              setSpawns(data.spawns.slice());
            }
          }
          return;
        }

        if (type === 'event' && data) {
          var spawnID = data.spawn_id || 0;
          if (spawnID > 0) console.log('[ADAF-DEBUG] event with spawn_id=' + spawnID, JSON.stringify(data).slice(0, 200));
          var ss = getStreamState(spawnID);
          var ev = data.event || data;
          if (typeof ev === 'string') {
            try { ev = JSON.parse(ev); } catch (_) { return; }
          }
          if (!ev || typeof ev !== 'object') return;

          if (ev.type === 'content_block_delta' && ev.delta) {
            ss.isStreaming = true;
            if (ev.delta.text) pushEventForChat(forChatID, { type: 'text', content: ev.delta.text }, spawnID);
            if (ev.delta.thinking) pushEventForChat(forChatID, { type: 'thinking', content: ev.delta.thinking }, spawnID);
            return;
          }

          if (ev.type === 'assistant') {
            // Turn skipping only applies to parent events (replay on reconnect).
            var shouldSkipReplay = false;
            if (spawnID === 0) {
              assistantTurnsSeen++;
              shouldSkipReplay = assistantTurnsSeen <= turnsToSkip;
            }
            var blocks = (ev.message && Array.isArray(ev.message.content)) ? ev.message.content : (Array.isArray(ev.content) ? ev.content : []);
            var parsedAssistantBlocks = [];
            blocks.forEach(function (block) {
              if (!block) return;
              if (block.type === 'text' && block.text) {
                if (!ss.isStreaming && !ss.hasRawText) parsedAssistantBlocks.push({ type: 'text', content: block.text });
              } else if (block.type === 'thinking' && block.text) {
                if (!ss.isStreaming) parsedAssistantBlocks.push({ type: 'thinking', content: block.text });
              } else if (block.type === 'tool_use') {
                parsedAssistantBlocks.push({ type: 'tool_use', tool: block.name || 'tool', input: block.input || {} });
              } else if (block.type === 'tool_result') {
                parsedAssistantBlocks.push({
                  type: 'tool_result', tool: block.name || '',
                  result: block.content || block.tool_content || block.output || block.text || '',
                  isError: !!block.is_error,
                });
              } else if (block && typeof block === 'object') {
                reportMissing({
                  reason: 'unknown_assistant_block',
                  event_type: block.type || 'unknown',
                  payload: block,
                });
              }
            });
            // Keep skipped parent blocks so we can recover if skip logic over-skips.
            if (shouldSkipReplay) {
              if (parsedAssistantBlocks.length > 0) {
                entry.skippedAssistantFallback = parsedAssistantBlocks.slice();
              }
              return;
            }
            parsedAssistantBlocks.forEach(function (evt) {
              pushEventForChat(forChatID, evt, spawnID);
            });
            return;
          }

          if (ev.type === 'user') {
            // Skip user events that belong to already-saved turns (parent only)
            if (spawnID === 0 && assistantTurnsSeen < turnsToSkip) return;

            var userBlocks = (ev.message && Array.isArray(ev.message.content)) ? ev.message.content : (Array.isArray(ev.content) ? ev.content : []);
            userBlocks.forEach(function (block) {
              if (block && block.type === 'tool_result') {
                pushEventForChat(forChatID, {
                  type: 'tool_result', tool: block.name || '',
                  result: block.content || block.tool_content || block.output || block.text || '',
                  isError: !!block.is_error,
                }, spawnID);
              } else if (block && typeof block === 'object') {
                reportMissing({
                  reason: 'unknown_user_block',
                  event_type: block.type || 'unknown',
                  payload: block,
                });
              }
            });
            return;
          }
          reportMissing({
            reason: 'unknown_agent_event_type',
            event_type: ev.type || 'event',
            payload: ev,
          });
          return;
        }

        if (type === 'raw' && data) {
          var rawSpawnID = (typeof data === 'object') ? (data.spawn_id || 0) : 0;
          if (rawSpawnID > 0) console.log('[ADAF-DEBUG] raw with spawn_id=' + rawSpawnID, JSON.stringify(data).slice(0, 200));
          var rawSS = getStreamState(rawSpawnID);
          var rawText = typeof data === 'string' ? data : (data.data || '');
          if (rawText && rawText.indexOf('\x1b') === -1 && rawText.indexOf('[stderr]') === -1) {
            rawSS.hasRawText = true;
            pushEventForChat(forChatID, { type: 'text', content: rawText }, rawSpawnID);
          }
          return;
        }

        if (type === 'done' || type === 'loop_done') {
          finalizeChat(forChatID, 'done');
          return;
        }

        reportMissing({
          source: 'standalone_ws_envelope',
          reason: 'unknown_envelope_type',
          event_type: type || 'event',
          payload: data,
        });
      } catch (e) {
        reportMissing({
          source: 'standalone_ws_message',
          reason: 'invalid_ws_payload_json',
          event_type: 'message',
          fallback_text: String(wsEvent && wsEvent.data ? wsEvent.data : ''),
          payload: String(wsEvent && wsEvent.data ? wsEvent.data : ''),
        });
        console.error('Standalone Chat WebSocket parse error:', e);
      }
    });

    ws.addEventListener('error', function () {
      finalizeChat(forChatID, 'closed');
    });

    ws.addEventListener('close', function () {
      finalizeChat(forChatID, 'closed');
    });
  }

  async function saveAssistantResponseForChat(forChatID, content, structuredEvents, promptData, capturedBase) {
    var assistantMsg = {
      id: Date.now(),
      role: 'assistant',
      content: content,
      events: structuredEvents || null,
      _events: structuredEvents || null,
      _prompt: promptData || null,
      created_at: new Date().toISOString(),
    };

    // Update local messages only if this is the current chat
    if (forChatID === currentChatIDRef.current) {
      setMessages(function (prev) { return prev.concat([assistantMsg]); });
    }

    // Refresh conversation list to update title/timestamp
    if (window.__reloadChatInstances) window.__reloadChatInstances();

    try {
      await apiCall((capturedBase || base) + '/chat-instances/' + encodeURIComponent(forChatID) + '/response', 'POST', {
        content: content,
        events: structuredEvents || null,
      });
    } catch (err) {
      console.error('Failed to save assistant response:', err);
    }
  }

  // --- Handlers ---

  async function handleSend(e) {
    e.preventDefault();
    if (!chatID) return;

    // Check if this chat already has an active session
    var existingEntry = chatSessionsRef.current[chatID];
    if (existingEntry && existingEntry.sending) return;

    var msg = input.trim();
    setInput('');

    // Count existing assistant messages so we can skip replayed turns on resume.
    var existingAssistantCount = 0;
    messages.forEach(function (m) { if (m.role === 'assistant') existingAssistantCount++; });

    // Create per-chat session entry
    var entry = {
      sessionID: null,
      events: [],
      spawnEvents: {},
      allEvents: [],
      spawns: [],
      parentProfile: (chatMeta && chatMeta.profile) || '',
      skippedAssistantFallback: null,
      ws: null,
      sending: true,
      finalized: false,
      promptData: null,
      promptsByScope: {},
      base: base,
      assistantTurnsToSkip: existingAssistantCount,
    };
    chatSessionsRef.current[chatID] = entry;

    setSending(true);
    setStreamEvents([]);
    setSpawns([]);
    setFocusScope('parent');
    focusScopeRef.current = 'parent';
    dispatch({ type: 'SET_STANDALONE_CHAT_STATUS', payload: { chatID: chatID, status: 'thinking' } });

    if (msg) {
      var userMsg = {
        id: Date.now(),
        role: 'user',
        content: msg,
        created_at: new Date().toISOString(),
      };
      setMessages(function (prev) { return prev.concat([userMsg]); });
    }

    try {
      var result = await apiCall(base + '/chat-instances/' + encodeURIComponent(chatID), 'POST', {
        message: msg,
      });
      if (result && result.session_id) {
        entry.sessionID = result.session_id;
        if (chatID === currentChatIDRef.current) {
          setActiveSessionID(result.session_id);
        }
        startSessionWS(chatID, result.session_id);
      } else {
        entry.sending = false;
        delete chatSessionsRef.current[chatID];
        if (chatID === currentChatIDRef.current) {
          setSending(false);
        }
        dispatch({ type: 'SET_STANDALONE_CHAT_STATUS', payload: { chatID: chatID, status: 'idle' } });
      }
      // Refresh conversation list to update title after first message
      if (window.__reloadChatInstances) window.__reloadChatInstances();
    } catch (err) {
      entry.sending = false;
      delete chatSessionsRef.current[chatID];
      if (err.authRequired) return;
      showToast('Failed to send message: ' + (err.message || err), 'error');
      if (chatID === currentChatIDRef.current) {
        setSending(false);
      }
      dispatch({ type: 'SET_STANDALONE_CHAT_STATUS', payload: { chatID: chatID, status: 'idle' } });
    }
  }

  async function handleStop() {
    if (!activeSessionID) return;
    try {
      await apiCall(base + '/sessions/' + encodeURIComponent(String(activeSessionID)) + '/stop', 'POST', {});
      showToast('Stop signal sent', 'success');
      finalizeChat(chatID, 'stopped');
    } catch (err) {
      if (err && err.authRequired) return;
      showToast('Failed to stop: ' + (err.message || err), 'error');
    }
  }

  function handleKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) handleSend(e);
  }

  function switchFocusScope(newScope) {
    setFocusScope(newScope);
    focusScopeRef.current = newScope;
    var entry = chatSessionsRef.current[chatID];
    if (!entry) { setStreamEvents([]); return; }
    if (newScope === 'parent') {
      setStreamEvents(entry.events.slice());
    } else if (newScope === 'all') {
      setStreamEvents(entry.allEvents.slice());
    } else if (newScope.indexOf('spawn-') === 0) {
      var sid = parseInt(newScope.slice(6));
      setStreamEvents((entry.spawnEvents[sid] || []).slice());
    }
  }

  // Render

  if (!chatID) {
    return (
      <div style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg-0)' }}>
        <div style={{ textAlign: 'center', color: 'var(--text-3)' }}>
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 13, marginBottom: 8, color: 'var(--text-2)' }}>
            Select a chat or create a new one.
          </div>
        </div>
      </div>
    );
  }

  var headerTitle = chatMeta ? chatMeta.title : 'Chat';
  if (headerTitle.length > 60) headerTitle = headerTitle.slice(0, 60) + '\u2026';
  var headerProfile = chatMeta ? chatMeta.profile : '';
  var headerTeam = chatMeta ? chatMeta.team : '';
  var headerAgent = chatMeta ? chatMeta.agent : '';
  var agentColor = headerAgent ? agentInfo(headerAgent) : (headerProfile ? agentInfo(headerProfile) : null);

  // Determine focused spawn info for header bar
  var focusedSpawn = null;
  if (focusScope.indexOf('spawn-') === 0) {
    var focusedSID = parseInt(focusScope.slice(6));
    focusedSpawn = spawns.find(function (s) { return s.id === focusedSID; }) || null;
  }
  var displayMessages = filterStandaloneMessagesByScope(messages, focusScope);

  return (
    <div style={{ height: '100%', display: 'flex' }}>
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', background: 'var(--bg-0)', minWidth: 0 }}>
        {/* Header */}
        <div style={{
          padding: '10px 16px', borderBottom: '1px solid var(--border)',
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          background: 'var(--bg-1)', flexShrink: 0, gap: 10,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0, flex: 1 }}>
            {sending && (
              <span style={{
                width: 7, height: 7, borderRadius: '50%',
                background: streamEvents.length > 0 ? 'var(--green)' : 'var(--accent)',
                animation: 'pulse 1.5s ease-in-out infinite',
                flexShrink: 0,
              }} />
            )}
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 13, fontWeight: 600,
              color: 'var(--text-0)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
            }}>
              {headerTitle}
            </span>
            {headerProfile && (
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                padding: '1px 7px', borderRadius: 8, flexShrink: 0,
                background: agentColor ? agentColor.color + '18' : 'var(--bg-3)',
                color: agentColor ? agentColor.color : 'var(--text-2)',
                border: '1px solid ' + (agentColor ? agentColor.color + '30' : 'var(--border)'),
              }}>
                {headerProfile}
              </span>
            )}
            <span ref={teamDropdownRef} style={{ position: 'relative', flexShrink: 0 }}>
              <span
                onClick={function () { if (!sending) setShowTeamDropdown(!showTeamDropdown); }}
                style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                  padding: '1px 7px', borderRadius: 8, cursor: sending ? 'default' : 'pointer',
                  background: headerTeam ? 'var(--green)18' : 'var(--bg-3)',
                  color: headerTeam ? 'var(--green)' : 'var(--text-3)',
                  border: '1px solid ' + (headerTeam ? 'var(--green)30' : 'var(--border)'),
                }}
              >
                {headerTeam ? headerTeam + (function () {
                  var t = teams.find(function (t) { return t.name === headerTeam; });
                  return t && t.delegation && t.delegation.profiles ? ' (' + t.delegation.profiles.length + ')' : '';
                })() : '+ team'}
              </span>
              {showTeamDropdown && (
                <div style={{
                  position: 'absolute', top: '100%', left: 0, marginTop: 4, zIndex: 100,
                  background: 'var(--bg-2)', border: '1px solid var(--border)', borderRadius: 6,
                  boxShadow: '0 4px 12px rgba(0,0,0,0.3)', minWidth: 140, padding: '4px 0',
                }}>
                  {headerTeam && (
                    <div
                      onClick={function () { handleTeamChange(''); }}
                      style={{
                        padding: '5px 12px', cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace",
                        fontSize: 11, color: 'var(--text-3)',
                      }}
                      onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-3)'; }}
                      onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
                    >
                      (none)
                    </div>
                  )}
                  {teams.map(function (t) {
                    var isCurrent = t.name === headerTeam;
                    var profileCount = t.delegation && t.delegation.profiles ? t.delegation.profiles.length : 0;
                    return (
                      <div
                        key={t.name}
                        onClick={function () { if (!isCurrent) handleTeamChange(t.name); }}
                        style={{
                          padding: '5px 12px', cursor: isCurrent ? 'default' : 'pointer',
                          fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                          color: isCurrent ? 'var(--green)' : 'var(--text-1)',
                          fontWeight: isCurrent ? 600 : 400,
                        }}
                        onMouseEnter={function (e) { if (!isCurrent) e.currentTarget.style.background = 'var(--bg-3)'; }}
                        onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
                      >
                        {t.name}{profileCount > 0 ? ' (' + profileCount + ')' : ''}
                      </div>
                    );
                  })}
                  {teams.length === 0 && (
                    <div style={{
                      padding: '5px 12px', fontFamily: "'JetBrains Mono', monospace",
                      fontSize: 11, color: 'var(--text-3)',
                    }}>
                      No teams configured
                    </div>
                  )}
                </div>
              )}
            </span>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
            {sending ? (
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                padding: '2px 8px', borderRadius: 8,
                background: streamEvents.length > 0 ? 'var(--green)18' : 'var(--accent)18',
                color: streamEvents.length > 0 ? 'var(--green)' : 'var(--accent)',
                animation: 'pulse 1.5s ease-in-out infinite',
              }}>
                {streamEvents.length > 0 ? 'responding\u2026' : 'thinking\u2026'}
              </span>
            ) : (
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                padding: '2px 8px', borderRadius: 8,
                background: 'var(--bg-3)', color: 'var(--text-3)',
              }}>
                {messages.length} msg{messages.length !== 1 ? 's' : ''}
              </span>
            )}
          </div>
        </div>

        {/* Focus header bar when viewing a specific spawn */}
        {focusedSpawn && (
          <div style={{
            padding: '6px 12px', borderBottom: '1px solid var(--border)',
            background: statusColor(focusedSpawn.status) + '10',
            display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0,
          }}>
            <span
              onClick={function () { switchFocusScope('parent'); }}
              style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                padding: '2px 6px', borderRadius: 3, cursor: 'pointer',
                background: 'var(--bg-3)', color: 'var(--text-2)',
                border: '1px solid var(--border)',
              }}
            >
              {'\u2190'} Back
            </span>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
              color: 'var(--text-1)',
            }}>
              Viewing spawn #{focusedSpawn.id}
              {' \u2014 '}
              <span style={{ fontWeight: 600, color: statusColor(focusedSpawn.status) }}>{focusedSpawn.profile}</span>
              {focusedSpawn.role ? ' as ' + focusedSpawn.role : ''}
            </span>
          </div>
        )}

        {/* Focus header bar when viewing all agents */}
        {focusScope === 'all' && spawns.length > 0 && (
          <div style={{
            padding: '6px 12px', borderBottom: '1px solid var(--border)',
            background: 'var(--accent)08',
            display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0,
          }}>
            <span
              onClick={function () { switchFocusScope('parent'); }}
              style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                padding: '2px 6px', borderRadius: 3, cursor: 'pointer',
                background: 'var(--bg-3)', color: 'var(--text-2)',
                border: '1px solid var(--border)',
              }}
            >
              {'\u2190'} Back
            </span>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
              color: 'var(--text-2)',
            }}>
              All agents ({spawns.length + 1})
            </span>
          </div>
        )}

        {/* Messages area */}
        <ChatMessageList
          messages={displayMessages}
          streamEvents={streamEvents}
          isStreaming={sending}
          loading={loading}
          emptyMessage="Type a message to begin."
          showSourceLabels={focusScope === 'all'}
          scrollContextKey={chatID + ':' + focusScope}
        />

        {/* Input area */}
        <div style={{
          padding: '6px 12px', borderTop: '1px solid var(--border)',
          background: 'var(--bg-1)', flexShrink: 0,
        }}>
          <form onSubmit={handleSend} style={{
            display: 'flex', gap: 6, alignItems: 'center',
          }}>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 13,
              color: sending ? 'var(--text-3)' : 'var(--accent)', fontWeight: 700,
              flexShrink: 0, userSelect: 'none',
            }}>&gt;</span>
            <input
              ref={inputRef}
              type="text"
              value={input}
              onChange={function (e) { setInput(e.target.value); }}
              onKeyDown={handleKeyDown}
              placeholder={sending ? 'Agent is working...' : 'Message...'}
              disabled={sending}
              style={{
                flex: 1, padding: '7px 10px',
                background: 'var(--bg-2)', border: '1px solid var(--border)', borderRadius: 3,
                color: 'var(--text-0)', fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
                outline: 'none',
              }}
              autoComplete="off"
            />
            {sending ? (
              <button
                type="button"
                onClick={handleStop}
                style={{
                  padding: '7px 12px', background: 'transparent',
                  border: '1px solid var(--red)', borderRadius: 3,
                  color: 'var(--red)', fontFamily: "'JetBrains Mono', monospace",
                  fontSize: 10, fontWeight: 600, cursor: 'pointer',
                  display: 'flex', alignItems: 'center', gap: 4,
                }}
            >{'\u25A0'} Stop</button>
            ) : (
              <button
                type="submit"
                style={{
                  padding: '7px 12px',
                  background: 'var(--accent)',
                  border: 'none', borderRadius: 3,
                  color: '#000',
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
                  cursor: 'pointer',
                }}
              >Send</button>
            )}
          </form>
        </div>

      </div>

      {/* Spawn sidebar */}
      {spawns.length > 0 && (
        <SpawnSidebar
          spawns={spawns}
          focusScope={focusScope}
          onSwitchScope={switchFocusScope}
          parentProfile={headerProfile}
          sending={sending}
        />
      )}
    </div>
  );
}

function buildStandaloneEventPayload(parentEvents, allEvents, spawnEvents) {
  var bySpawn = {};
  var spawnMap = spawnEvents && typeof spawnEvents === 'object' ? spawnEvents : {};
  Object.keys(spawnMap).forEach(function (key) {
    var list = sanitizeStandaloneEvents(spawnMap[key], 'spawn-' + key, 'spawn-' + key, key);
    if (list.length > 0) bySpawn[key] = list;
  });
  return {
    version: 2,
    parent: sanitizeStandaloneEvents(parentEvents, 'parent', 'parent', 0),
    all: sanitizeStandaloneEvents(allEvents, 'all', '', 0),
    by_spawn: bySpawn,
  };
}

function sanitizeStandaloneEvents(events, defaultScope, defaultLabel, defaultSpawnID) {
  var list = Array.isArray(events) ? events : [];
  return list.map(function (evt) {
    if (!evt || typeof evt !== 'object') return null;
    var scoped = Object.assign({}, evt);
    if (!scoped.scope && defaultScope && defaultScope !== 'all') scoped.scope = defaultScope;
    if (!scoped._sourceLabel && defaultLabel) scoped._sourceLabel = defaultLabel;
    if (scoped._sourceColor == null && defaultLabel) scoped._sourceColor = agentInfo(defaultLabel).color;
    if (scoped._spawnID == null && defaultSpawnID != null) scoped._spawnID = Number(defaultSpawnID) || 0;
    return scoped;
  }).filter(function (evt) { return !!evt; });
}

function normalizeStandaloneEventPayload(rawEvents) {
  var parsed = rawEvents;
  if (typeof parsed === 'string') {
    try { parsed = JSON.parse(parsed); } catch (_) { parsed = null; }
  }

  if (Array.isArray(parsed)) {
    return {
      parent: sanitizeStandaloneEvents(parsed, 'parent', 'parent', 0),
      all: sanitizeStandaloneEvents(parsed, 'parent', 'parent', 0),
      bySpawn: {},
    };
  }

  if (!parsed || typeof parsed !== 'object') {
    return { parent: [], all: [], bySpawn: {} };
  }

  var bySpawn = {};
  var bySpawnRaw = parsed.by_spawn && typeof parsed.by_spawn === 'object' ? parsed.by_spawn : {};
  Object.keys(bySpawnRaw).forEach(function (key) {
    bySpawn[key] = sanitizeStandaloneEvents(bySpawnRaw[key], 'spawn-' + key, 'spawn-' + key, key);
  });

  var parent = sanitizeStandaloneEvents(parsed.parent, 'parent', 'parent', 0);
  var all = sanitizeStandaloneEvents(parsed.all, 'all', '', 0);
  if (all.length === 0 && parent.length > 0) {
    all = sanitizeStandaloneEvents(parent, 'parent', 'parent', 0);
  }

  return { parent: parent, all: all, bySpawn: bySpawn };
}

function eventsForStandaloneScope(rawEvents, focusScope) {
  var payload = normalizeStandaloneEventPayload(rawEvents);
  if (focusScope === 'all') {
    return payload.all;
  }
  if (focusScope && focusScope.indexOf('spawn-') === 0) {
    var spawnID = String(parseInt(focusScope.slice(6), 10) || 0);
    return payload.bySpawn[spawnID] || [];
  }
  return payload.parent;
}

function filterStandaloneMessagesByScope(messages, focusScope) {
  var list = Array.isArray(messages) ? messages : [];
  var filtered = [];

  list.forEach(function (msg) {
    if (!msg || typeof msg !== 'object') return;
    if (msg.role === 'user') {
      filtered.push(msg);
      return;
    }

    var rawEvents = msg.events != null ? msg.events : msg._events;
    var selectedEvents = eventsForStandaloneScope(rawEvents, focusScope);
    if (selectedEvents.length > 0) {
      filtered.push(Object.assign({}, msg, { _events: selectedEvents }));
      return;
    }

    // Legacy assistant messages without structured events only belong to parent scope.
    if (focusScope === 'parent' && msg.content) {
      filtered.push(Object.assign({}, msg, { _events: null }));
    }
  });

  return filtered;
}

// --- SpawnSidebar component ---

function SpawnSidebar({ spawns, focusScope, onSwitchScope, parentProfile, sending }) {
  // Build tree from spawns using parent_spawn_id
  var childrenByParent = {};
  var roots = [];
  spawns.forEach(function (s) {
    if (s.parent_spawn_id > 0) {
      if (!childrenByParent[s.parent_spawn_id]) childrenByParent[s.parent_spawn_id] = [];
      childrenByParent[s.parent_spawn_id].push(s);
    } else {
      roots.push(s);
    }
  });

  var parentColor = agentInfo(parentProfile || '').color;
  var parentSelected = focusScope === 'parent';
  var allSelected = focusScope === 'all';

  return (
    <div style={{
      width: 240, borderLeft: '1px solid var(--border)',
      background: 'var(--bg-1)', overflow: 'auto', flexShrink: 0,
    }}>
      {/* Header */}
      <div style={{
        padding: '8px 12px', borderBottom: '1px solid var(--border)',
        fontFamily: "'JetBrains Mono', monospace", fontSize: 9, fontWeight: 600,
        color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.1em',
      }}>
        Agents
      </div>

      {/* Parent node */}
      <div
        onClick={function () { onSwitchScope('parent'); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '8px 12px', cursor: 'pointer',
          background: parentSelected ? (parentColor + '12') : 'transparent',
          borderLeft: parentSelected ? ('2px solid ' + parentColor) : '2px solid transparent',
          transition: 'all 0.15s ease',
        }}
        onMouseEnter={function (e) { if (!parentSelected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!parentSelected) e.currentTarget.style.background = 'transparent'; }}
      >
        <span style={{
          width: 7, height: 7, borderRadius: '50%', flexShrink: 0,
          background: sending ? '#a6e3a1' : 'var(--text-3)',
          boxShadow: sending ? '0 0 6px #a6e3a1' : 'none',
          animation: sending ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
              color: 'var(--text-0)',
            }}>
              {parentProfile || 'Parent'}
            </span>
          </div>
          <div style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
            color: 'var(--text-3)', marginTop: 1,
          }}>
            main agent
          </div>
        </div>
      </div>

      {/* All node */}
      <div
        onClick={function () { onSwitchScope('all'); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '8px 12px', cursor: 'pointer',
          background: allSelected ? 'var(--accent)12' : 'transparent',
          borderLeft: allSelected ? '2px solid var(--accent)' : '2px solid transparent',
          transition: 'all 0.15s ease',
        }}
        onMouseEnter={function (e) { if (!allSelected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!allSelected) e.currentTarget.style.background = 'transparent'; }}
      >
        <span style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
          color: 'var(--text-2)', flexShrink: 0,
        }}>{'\u2261'}</span>
        <span style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
          color: 'var(--text-1)',
        }}>
          All agents
        </span>
        <span style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
          color: 'var(--text-3)', padding: '1px 5px',
          background: 'var(--bg-3)', borderRadius: 3, marginLeft: 'auto',
        }}>
          {spawns.length + 1}
        </span>
      </div>

      {/* Divider */}
      <div style={{ borderBottom: '1px solid var(--border)', margin: '4px 0' }} />

      {/* Spawn tree */}
      {roots.map(function (spawn) {
        return (
          <SpawnTreeNode
            key={spawn.id}
            spawn={spawn}
            depth={0}
            childrenByParent={childrenByParent}
            focusScope={focusScope}
            onSelect={onSwitchScope}
          />
        );
      })}
    </div>
  );
}

// --- SpawnTreeNode component ---

function SpawnTreeNode({ spawn, depth, childrenByParent, focusScope, onSelect }) {
  var children = childrenByParent[spawn.id] || [];
  var selected = focusScope === 'spawn-' + spawn.id;
  var sColor = statusColor(spawn.status);
  var statusLower = (spawn.status || '').toLowerCase().replace(/[^a-z0-9_]+/g, '_');
  var isRunning = !!STATUS_RUNNING[statusLower];
  var hasPendingQuestion = statusLower === 'awaiting_input' && !!spawn.question;

  return (
    <div style={{ marginLeft: depth * 14 }}>
      <div
        onClick={function () { onSelect('spawn-' + spawn.id); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 7,
          padding: '6px 12px', cursor: 'pointer',
          background: selected ? (sColor + '12') : 'transparent',
          borderLeft: selected ? ('2px solid ' + sColor) : '2px solid transparent',
          transition: 'all 0.15s ease',
        }}
        onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
      >
        <span style={{
          width: 6, height: 6, borderRadius: '50%', flexShrink: 0,
          background: sColor,
          boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
          animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              color: 'var(--text-3)',
            }}>
              #{spawn.id}
            </span>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
              color: 'var(--text-0)',
            }}>
              {spawn.profile || 'spawn'}
            </span>
            {spawn.role && (
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                color: 'var(--text-3)',
              }}>
                as {spawn.role}
              </span>
            )}
          </div>
          {hasPendingQuestion && (
            <div style={{
              marginTop: 3, display: 'flex', alignItems: 'center', gap: 4,
            }}>
              <span style={{
                width: 5, height: 5, borderRadius: '50%',
                background: '#89b4fa',
                animation: 'pulse 1.5s ease-in-out infinite',
              }} />
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 8,
                color: '#89b4fa', fontWeight: 600,
              }}>
                AWAITING RESPONSE
              </span>
            </div>
          )}
        </div>
        {isRunning && (
          <span style={{
            width: 8, height: 8, border: '1.5px solid ' + sColor, borderTopColor: 'transparent',
            borderRadius: '50%', animation: 'spin 1s linear infinite', flexShrink: 0,
          }} />
        )}
      </div>

      {children.map(function (child) {
        return (
          <SpawnTreeNode
            key={child.id}
            spawn={child}
            depth={depth + 1}
            childrenByParent={childrenByParent}
            focusScope={focusScope}
            onSelect={onSelect}
          />
        );
      })}
    </div>
  );
}
