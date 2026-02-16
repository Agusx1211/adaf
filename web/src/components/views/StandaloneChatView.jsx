import { useState, useEffect, useRef, useCallback } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiCall, apiBase, buildWSURL } from '../../api/client.js';
import { reportMissingUISample } from '../../api/missingUISamples.js';
import { useToast } from '../common/Toast.jsx';
import { injectEventBlockStyles, cleanResponse } from '../common/EventBlocks.jsx';
import ChatMessageList from '../common/ChatMessageList.jsx';
import { agentInfo } from '../../utils/colors.js';

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
  var inputRef = useRef(null);
  var teamDropdownRef = useRef(null);
  var base = apiBase(state.currentProjectID);

  // Refs for per-chat session management
  var currentChatIDRef = useRef(chatID);
  var chatSessionsRef = useRef({}); // { [chatID]: { sessionID, events, ws, sending, finalized, promptData, base } }
  var dispatchRef = useRef(dispatch);
  dispatchRef.current = dispatch;

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
    } else {
      setSending(false);
      setStreamEvents([]);
      setActiveSessionID(null);
    }
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

  function pushEventForChat(forChatID, evt) {
    var entry = chatSessionsRef.current[forChatID];
    if (!entry) return;

    var events = entry.events;
    var last = events.length > 0 ? events[events.length - 1] : null;
    if (evt.type === 'text' && last && last.type === 'text') {
      last.content += evt.content;
    } else if (evt.type === 'thinking' && last && last.type === 'thinking') {
      last.content += evt.content;
    } else {
      events.push(evt);
    }

    // Update React state only if this is the currently viewed chat
    if (forChatID === currentChatIDRef.current) {
      setStreamEvents(events.slice());
    }

    // Update global status to 'responding' once we have content
    if (evt.type === 'text' || evt.type === 'tool_use' || evt.type === 'tool_result') {
      dispatchRef.current({ type: 'SET_STANDALONE_CHAT_STATUS', payload: { chatID: forChatID, status: 'responding' } });
    }
  }

  function finalizeChat(forChatID) {
    var entry = chatSessionsRef.current[forChatID];
    if (!entry || entry.finalized) return;
    entry.finalized = true;
    entry.sending = false;

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

    if (finalText || entry.events.length > 0) {
      saveAssistantResponseForChat(forChatID, finalText || '(no text output)', entry.events.slice(), entry.promptData, entry.base);
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
    if (forChatID === currentChatIDRef.current) {
      setSending(false);
      setActiveSessionID(null);
      setStreamEvents([]);
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

    var isStreaming = false;
    var hasRawText = false;
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

        if (type === 'prompt' && data) {
          entry.promptData = {
            text: data.text || data.prompt || '',
            truncated: !!data.truncated,
            turn_id: data.turn_id || null,
            session_id: data.session_id || null,
          };
          return;
        }

        if (type === 'event' && data) {
          var ev = data.event || data;
          if (typeof ev === 'string') {
            try { ev = JSON.parse(ev); } catch (_) { return; }
          }
          if (!ev || typeof ev !== 'object') return;

          if (ev.type === 'content_block_delta' && ev.delta) {
            isStreaming = true;
            if (ev.delta.text) pushEventForChat(forChatID, { type: 'text', content: ev.delta.text });
            if (ev.delta.thinking) pushEventForChat(forChatID, { type: 'thinking', content: ev.delta.thinking });
            return;
          }

          if (ev.type === 'assistant') {
            assistantTurnsSeen++;
            var blocks = (ev.message && Array.isArray(ev.message.content)) ? ev.message.content : (Array.isArray(ev.content) ? ev.content : []);
            var parsedAssistantBlocks = [];
            blocks.forEach(function (block) {
              if (!block) return;
              if (block.type === 'text' && block.text) {
                if (!isStreaming && !hasRawText) parsedAssistantBlocks.push({ type: 'text', content: block.text });
              } else if (block.type === 'thinking' && block.text) {
                if (!isStreaming) parsedAssistantBlocks.push({ type: 'thinking', content: block.text });
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
            // Skip replayed assistant turns from previous conversation rounds.
            // Keep the most-recent skipped blocks so we can recover if skip was wrong.
            if (assistantTurnsSeen <= turnsToSkip) {
              if (parsedAssistantBlocks.length > 0) {
                entry.skippedAssistantFallback = parsedAssistantBlocks.slice();
              }
              return;
            }
            parsedAssistantBlocks.forEach(function (evt) {
              pushEventForChat(forChatID, evt);
            });
            return;
          }

          if (ev.type === 'user') {
            // Skip user events that belong to already-saved turns
            if (assistantTurnsSeen < turnsToSkip) return;

            var userBlocks = (ev.message && Array.isArray(ev.message.content)) ? ev.message.content : (Array.isArray(ev.content) ? ev.content : []);
            userBlocks.forEach(function (block) {
              if (block && block.type === 'tool_result') {
                pushEventForChat(forChatID, {
                  type: 'tool_result', tool: block.name || '',
                  result: block.content || block.tool_content || block.output || block.text || '',
                  isError: !!block.is_error,
                });
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
          var rawText = typeof data === 'string' ? data : (data.data || '');
          if (rawText && rawText.indexOf('\x1b') === -1 && rawText.indexOf('[stderr]') === -1) {
            hasRawText = true;
            pushEventForChat(forChatID, { type: 'text', content: rawText });
          }
          return;
        }

        if (type === 'done' || type === 'loop_done') {
          finalizeChat(forChatID);
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
      finalizeChat(forChatID);
    });

    ws.addEventListener('close', function () {
      finalizeChat(forChatID);
    });
  }

  async function saveAssistantResponseForChat(forChatID, content, structuredEvents, promptData, capturedBase) {
    var assistantMsg = {
      id: Date.now(),
      role: 'assistant',
      content: content,
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
      await apiCall((capturedBase || base) + '/chat-instances/' + encodeURIComponent(forChatID) + '/response', 'POST', { content: content, events: structuredEvents || null });
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
    // When an agent resumes a session it re-streams ALL previous conversation
    // turns before the new response; we need to ignore those duplicates.
    var existingAssistantCount = 0;
    messages.forEach(function (m) { if (m.role === 'assistant') existingAssistantCount++; });

    // Create per-chat session entry
    var entry = {
      sessionID: null,
      events: [],
      skippedAssistantFallback: null,
      ws: null,
      sending: true,
      finalized: false,
      promptData: null,
      base: base,
      assistantTurnsToSkip: existingAssistantCount,
    };
    chatSessionsRef.current[chatID] = entry;

    setSending(true);
    setStreamEvents([]);
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
    } catch (err) {
      if (err && err.authRequired) return;
      showToast('Failed to stop: ' + (err.message || err), 'error');
    }
  }

  function handleKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) handleSend(e);
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

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', background: 'var(--bg-0)' }}>
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

      {/* Messages area */}
      <ChatMessageList
        messages={messages}
        streamEvents={streamEvents}
        isStreaming={sending}
        loading={loading}
        emptyMessage="Type a message to begin."
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
  );
}
