import { useState, useEffect, useRef, useCallback } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiCall, apiBase, buildWSURL } from '../../api/client.js';
import { useToast } from '../common/Toast.jsx';
import { timeAgo } from '../../utils/format.js';
import { EventBlockList, MarkdownContent, injectEventBlockStyles, cleanResponse } from '../common/EventBlocks.jsx';
import Modal from '../common/Modal.jsx';

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
  var [inspectedMessage, setInspectedMessage] = useState(null);
  var listRef = useRef(null);
  var wsRef = useRef(null);
  var inputRef = useRef(null);
  var promptRef = useRef(null);
  var base = apiBase(state.currentProjectID);

  useEffect(function () { injectEventBlockStyles(); }, []);

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

  // Auto-scroll on new content
  useEffect(function () {
    if (listRef.current) listRef.current.scrollTop = listRef.current.scrollHeight;
  }, [messages, streamEvents]);

  // WebSocket for streaming session output
  useEffect(function () {
    if (!activeSessionID) return;

    var ws;
    try {
      ws = new WebSocket(buildWSURL('/ws/sessions/' + encodeURIComponent(String(activeSessionID))));
      wsRef.current = ws;
    } catch (e) {
      console.error('Standalone Chat WebSocket error:', e);
      setSending(false);
      return;
    }

    var events = [];
    var isStreaming = false;
    var hasRawText = false;
    var finalized = false;
    promptRef.current = null;

    function pushEvent(evt) {
      var last = events.length > 0 ? events[events.length - 1] : null;
      if (evt.type === 'text' && last && last.type === 'text') {
        last.content += evt.content;
      } else if (evt.type === 'thinking' && last && last.type === 'thinking') {
        last.content += evt.content;
      } else {
        events.push(evt);
      }
      setStreamEvents(events.slice());
    }

    ws.addEventListener('message', function (wsEvent) {
      try {
        var envelope = JSON.parse(wsEvent.data);
        var type = envelope.type;
        var data = envelope.data;

        if (type === 'prompt' && data) {
          promptRef.current = {
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
            if (ev.delta.text) pushEvent({ type: 'text', content: ev.delta.text });
            if (ev.delta.thinking) pushEvent({ type: 'thinking', content: ev.delta.thinking });
            return;
          }

          if (ev.type === 'assistant') {
            var blocks = (ev.message && Array.isArray(ev.message.content)) ? ev.message.content : (Array.isArray(ev.content) ? ev.content : []);
            blocks.forEach(function (block) {
              if (!block) return;
              if (block.type === 'text' && block.text) {
                if (!isStreaming && !hasRawText) pushEvent({ type: 'text', content: block.text });
              } else if (block.type === 'thinking' && block.text) {
                if (!isStreaming) pushEvent({ type: 'thinking', content: block.text });
              } else if (block.type === 'tool_use') {
                pushEvent({ type: 'tool_use', tool: block.name || 'tool', input: block.input || {} });
              } else if (block.type === 'tool_result') {
                pushEvent({
                  type: 'tool_result', tool: block.name || '',
                  result: block.content || block.tool_content || block.output || block.text || '',
                  isError: !!block.is_error,
                });
              }
            });
            return;
          }

          if (ev.type === 'user') {
            var userBlocks = (ev.message && Array.isArray(ev.message.content)) ? ev.message.content : (Array.isArray(ev.content) ? ev.content : []);
            userBlocks.forEach(function (block) {
              if (block && block.type === 'tool_result') {
                pushEvent({
                  type: 'tool_result', tool: block.name || '',
                  result: block.content || block.tool_content || block.output || block.text || '',
                  isError: !!block.is_error,
                });
              }
            });
            return;
          }
          return;
        }

        if (type === 'raw' && data) {
          var rawText = typeof data === 'string' ? data : (data.data || '');
          if (rawText && rawText.indexOf('\x1b') === -1 && rawText.indexOf('[stderr]') === -1) {
            hasRawText = true;
            pushEvent({ type: 'text', content: rawText });
          }
          return;
        }

        if ((type === 'done' || type === 'loop_done') && !finalized) {
          finalized = true;
          var textParts = [];
          events.forEach(function (e) {
            if (e.type === 'text') textParts.push(e.content);
          });
          var finalText = cleanResponse(textParts.join(''));
          if (finalText || events.length > 0) {
            saveAssistantResponse(finalText || '(no text output)', events.slice());
          }
          setSending(false);
          setActiveSessionID(null);
          setStreamEvents([]);
          ws.close();
        }
      } catch (e) {
        console.error('Standalone Chat WebSocket parse error:', e);
      }
    });

    ws.addEventListener('error', function () {
      if (!finalized) {
        finalized = true;
        setSending(false);
        setActiveSessionID(null);
        setStreamEvents([]);
      }
    });
    ws.addEventListener('close', function () {
      wsRef.current = null;
      if (!finalized) {
        finalized = true;
        var textParts = [];
        events.forEach(function (e) {
          if (e.type === 'text') textParts.push(e.content);
        });
        var finalText = cleanResponse(textParts.join(''));
        if (finalText || events.length > 0) {
          saveAssistantResponse(finalText || '(session ended)', events.slice());
        }
        setSending(false);
        setActiveSessionID(null);
        setStreamEvents([]);
      }
    });

    return function () {
      if (ws && ws.readyState <= 1) ws.close();
    };
  }, [activeSessionID]);

  // Handlers

  async function saveAssistantResponse(content, structuredEvents) {
    var assistantMsg = {
      id: Date.now(),
      role: 'assistant',
      content: content,
      _events: structuredEvents || null,
      _prompt: promptRef.current || null,
      created_at: new Date().toISOString(),
    };
    promptRef.current = null;
    setMessages(function (prev) { return prev.concat([assistantMsg]); });
    // Refresh conversation list to update title/timestamp
    if (window.__reloadChatInstances) window.__reloadChatInstances();
    try {
      await apiCall(base + '/chat-instances/' + encodeURIComponent(chatID) + '/response', 'POST', { content: content });
    } catch (err) {
      console.error('Failed to save assistant response:', err);
    }
  }

  async function handleSend(e) {
    e.preventDefault();
    if (sending || !chatID) return;

    var msg = input.trim();
    setInput('');
    setSending(true);
    setStreamEvents([]);

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
        setActiveSessionID(result.session_id);
      } else {
        setSending(false);
      }
      // Refresh conversation list to update title after first message
      if (window.__reloadChatInstances) window.__reloadChatInstances();
    } catch (err) {
      if (err.authRequired) return;
      showToast('Failed to send message: ' + (err.message || err), 'error');
      setSending(false);
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
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 14, marginBottom: 12, color: 'var(--text-1)' }}>
            Standalone Chat
          </div>
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, opacity: 0.7, maxWidth: 400, margin: '0 auto', lineHeight: 1.6 }}>
            Select a chat from the sidebar or create a new one with + New Chat.
          </div>
        </div>
      </div>
    );
  }

  var headerTitle = chatMeta ? chatMeta.title : 'Chat';
  if (headerTitle.length > 40) headerTitle = headerTitle.slice(0, 40) + '\u2026';
  var headerProfile = chatMeta ? chatMeta.profile : '';

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', background: 'var(--bg-0)' }}>
      {/* Header */}
      <div style={{
        padding: '8px 16px', borderBottom: '1px solid var(--border)',
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        background: 'var(--bg-1)', flexShrink: 0,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-1)' }}>
            {headerTitle}
          </span>
          {headerProfile && (
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
              {headerProfile}
            </span>
          )}
        </div>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
          {messages.length} messages
        </span>
      </div>

      {/* Messages area */}
      <div ref={listRef} style={{ flex: 1, overflow: 'auto', padding: '16px 20px' }}>
        {loading ? (
          <div style={{ textAlign: 'center', color: 'var(--text-3)', padding: 60 }}>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11 }}>Loading...</div>
          </div>
        ) : messages.length === 0 && !sending ? (
          <div style={{ textAlign: 'center', color: 'var(--text-3)', padding: 60 }}>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 14, marginBottom: 12, color: 'var(--text-1)' }}>
              New Chat
            </div>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, opacity: 0.7, maxWidth: 400, margin: '0 auto', lineHeight: 1.6 }}>
              Type a message or press Enter to start agent work.
            </div>
          </div>
        ) : (
          <div style={{ maxWidth: 800, margin: '0 auto' }}>
            {messages.map(function (msg) {
              var isUser = msg.role === 'user';
              var hasInspectData = !isUser && (msg._prompt || (msg._events && msg._events.length > 0));
              return (
                <div key={msg.id} style={{ marginBottom: 16, animation: 'slideIn 0.15s ease-out' }}>
                  <div
                    onClick={function () { if (hasInspectData) setInspectedMessage(msg); }}
                    style={{
                      padding: '12px 16px', borderRadius: 8,
                      background: isUser ? 'var(--bg-2)' : 'var(--bg-1)',
                      border: '1px solid var(--border)',
                      borderLeft: isUser ? '3px solid var(--accent)' : '3px solid var(--green)',
                      cursor: hasInspectData ? 'pointer' : 'default',
                    }}>
                    <div style={{
                      fontSize: 10, fontWeight: 600,
                      color: isUser ? 'var(--accent)' : 'var(--green)',
                      marginBottom: 8, display: 'flex', alignItems: 'center', gap: 8,
                      textTransform: 'uppercase', letterSpacing: '0.05em',
                      fontFamily: "'JetBrains Mono', monospace",
                    }}>
                      <span>{isUser ? 'You' : 'Agent'}</span>
                      <span style={{ fontWeight: 400, color: 'var(--text-3)', textTransform: 'none', letterSpacing: 'normal' }}>
                        {timeAgo(msg.created_at)}
                      </span>
                      {hasInspectData && (
                        <span style={{ fontWeight: 400, color: 'var(--text-3)', textTransform: 'none', letterSpacing: 'normal', marginLeft: 'auto', fontSize: 9, opacity: 0.6 }}>
                          click to inspect
                        </span>
                      )}
                    </div>
                    {isUser ? (
                      <div style={{
                        fontSize: 13, color: 'var(--text-0)', lineHeight: 1.6,
                        whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                      }}>{msg.content}</div>
                    ) : msg._events && msg._events.length > 0 ? (
                      <EventBlockList events={msg._events} />
                    ) : (
                      <MarkdownContent text={msg.content} />
                    )}
                  </div>
                </div>
              );
            })}

            {/* Streaming response bubble */}
            {sending && (
              <div style={{ marginBottom: 16, animation: 'slideIn 0.15s ease-out' }}>
                <div style={{
                  padding: '12px 16px', borderRadius: 8,
                  background: 'var(--bg-1)', border: '1px solid var(--border)',
                  borderLeft: '3px solid var(--green)',
                }}>
                  <div style={{
                    fontSize: 10, fontWeight: 600, color: 'var(--green)',
                    marginBottom: 8, display: 'flex', alignItems: 'center', gap: 8,
                    textTransform: 'uppercase', letterSpacing: '0.05em',
                    fontFamily: "'JetBrains Mono', monospace",
                  }}>
                    <span>Agent</span>
                    <span style={{
                      fontWeight: 400, color: 'var(--accent)',
                      textTransform: 'none', letterSpacing: 'normal',
                      animation: 'pulse 1.5s ease-in-out infinite',
                    }}>
                      {streamEvents.length > 0 ? 'responding\u2026' : 'thinking\u2026'}
                    </span>
                  </div>
                  {streamEvents.length > 0 ? (
                    <EventBlockList events={streamEvents} />
                  ) : (
                    <div style={{
                      fontSize: 12, color: 'var(--text-3)',
                      fontFamily: "'JetBrains Mono', monospace",
                    }}>
                      Waiting for response...
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Input area */}
      <div style={{
        padding: '12px 20px', borderTop: '1px solid var(--border)',
        background: 'var(--bg-1)', flexShrink: 0,
      }}>
        <form onSubmit={handleSend} style={{
          maxWidth: 800, margin: '0 auto',
          display: 'flex', gap: 8, alignItems: 'center',
        }}>
          <input
            ref={inputRef}
            type="text"
            value={input}
            onChange={function (e) { setInput(e.target.value); }}
            onKeyDown={handleKeyDown}
            placeholder="Type a message or press Enter to start agent work..."
            disabled={sending}
            style={{
              flex: 1, padding: '10px 14px',
              background: 'var(--bg-2)', border: '1px solid var(--border)', borderRadius: 6,
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
                padding: '10px 16px', background: 'transparent',
                border: '1px solid var(--red)', borderRadius: 6,
                color: 'var(--red)', fontFamily: "'JetBrains Mono', monospace",
                fontSize: 11, fontWeight: 600, cursor: 'pointer',
                display: 'flex', alignItems: 'center', gap: 6,
              }}
            >{'\u25A0'} Stop</button>
          ) : (
            <button
              type="submit"
              style={{
                padding: '10px 16px',
                background: 'var(--accent)',
                border: 'none', borderRadius: 6,
                color: '#000',
                fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
                cursor: 'pointer',
              }}
            >Send</button>
          )}
        </form>
      </div>

      {/* Prompt Inspector Modal */}
      {inspectedMessage && (
        <Modal title="Prompt Inspector" maxWidth={900} onClose={function () { setInspectedMessage(null); }}>
          <div style={{ maxHeight: '70vh', overflow: 'auto' }}>
            {inspectedMessage._prompt && inspectedMessage._prompt.text ? (
              <div>
                <div style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
                  color: 'var(--accent)', textTransform: 'uppercase', letterSpacing: '0.05em',
                  marginBottom: 8,
                }}>
                  System Prompt
                  {inspectedMessage._prompt.truncated && (
                    <span style={{ color: 'var(--text-3)', fontWeight: 400, textTransform: 'none', marginLeft: 8 }}>(truncated)</span>
                  )}
                </div>
                <pre style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                  color: 'var(--text-1)', background: 'var(--bg-2)',
                  padding: 12, borderRadius: 6, border: '1px solid var(--border)',
                  whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                  lineHeight: 1.5, maxHeight: 500, overflow: 'auto',
                  margin: 0,
                }}>{inspectedMessage._prompt.text}</pre>
              </div>
            ) : null}
            {inspectedMessage._events && inspectedMessage._events.length > 0 && (
              <div style={{ marginTop: inspectedMessage._prompt ? 16 : 0 }}>
                <div style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
                  color: 'var(--green)', textTransform: 'uppercase', letterSpacing: '0.05em',
                  marginBottom: 8,
                }}>
                  Structured Events ({inspectedMessage._events.length})
                </div>
                <EventBlockList events={inspectedMessage._events} />
              </div>
            )}
          </div>
        </Modal>
      )}
    </div>
  );
}
