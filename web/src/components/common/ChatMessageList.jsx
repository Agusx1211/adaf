import { useState, useRef, useEffect } from 'react';
import { timeAgo } from '../../utils/format.js';
import { EventBlockList, MarkdownContent, injectEventBlockStyles } from './EventBlocks.jsx';
import Modal from './Modal.jsx';

export default function ChatMessageList({
  messages,
  streamEvents,
  isStreaming,
  streamStatus,
  loading,
  emptyMessage,
  autoScroll,
}) {
  var evts = streamEvents || [];
  var msgs = messages || [];
  var empty = emptyMessage || 'No output yet';
  var shouldAutoScroll = autoScroll !== false;
  var status = streamStatus || (evts.length > 0 ? 'responding' : 'thinking');

  var [inspectedMessage, setInspectedMessage] = useState(null);
  var listRef = useRef(null);

  useEffect(function () { injectEventBlockStyles(); }, []);

  useEffect(function () {
    if (shouldAutoScroll && listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [msgs, evts, shouldAutoScroll]);

  if (loading) {
    return (
      <div ref={listRef} style={{ flex: 1, overflow: 'auto', padding: '6px 12px' }}>
        <div style={{ textAlign: 'center', color: 'var(--text-3)', padding: 40 }}>
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11 }}>Loading...</div>
        </div>
      </div>
    );
  }

  if (msgs.length === 0 && !isStreaming) {
    return (
      <div ref={listRef} style={{ flex: 1, overflow: 'auto', padding: '6px 12px' }}>
        <div style={{ textAlign: 'center', color: 'var(--text-3)', padding: 40 }}>
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, color: 'var(--text-2)' }}>
            {empty}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div ref={listRef} style={{ flex: 1, overflow: 'auto', padding: '6px 12px' }}>
      <div>
        {msgs.map(function (msg) {
          var isUser = msg.role === 'user';
          var msgEvents = msg._events || msg.events;
          var hasInspectData = !isUser && (msg._prompt || (msgEvents && msgEvents.length > 0));
          return (
            <div key={msg.id} style={{ marginBottom: 4 }}>
              <div style={{
                padding: '6px 10px', borderRadius: 2,
                background: isUser ? 'var(--bg-2)' : 'transparent',
                borderLeft: isUser ? '2px solid var(--accent)' : '2px solid var(--green)40',
              }}>
                <div style={{
                  fontSize: 9, fontWeight: 600,
                  color: isUser ? 'var(--accent)' : 'var(--green)',
                  marginBottom: 3, display: 'flex', alignItems: 'center', gap: 6,
                  textTransform: 'uppercase', letterSpacing: '0.05em',
                  fontFamily: "'JetBrains Mono', monospace",
                }}>
                  <span>{isUser ? 'You' : 'Agent'}</span>
                  <span style={{ fontWeight: 400, color: 'var(--text-3)', textTransform: 'none', letterSpacing: 'normal', fontSize: 9 }}>
                    {timeAgo(msg.created_at)}
                  </span>
                  {hasInspectData && (
                    <button
                      onClick={function (e) { e.stopPropagation(); setInspectedMessage(msg); }}
                      style={{
                        marginLeft: 'auto',
                        background: 'none',
                        border: '1px solid var(--border)',
                        borderRadius: 3,
                        padding: '2px 6px',
                        fontSize: 9,
                        color: 'var(--text-3)',
                        cursor: 'pointer',
                        fontFamily: "'JetBrains Mono', monospace",
                        textTransform: 'none',
                        letterSpacing: 'normal',
                      }}
                      title="View prompt and events"
                    >
                      inspect
                    </button>
                  )}
                </div>
                {isUser ? (
                  <MarkdownContent text={msg.content} style={{ fontSize: 13, color: 'var(--text-0)', lineHeight: 1.5 }} />
                ) : msgEvents && msgEvents.length > 0 ? (
                  <EventBlockList events={msgEvents} />
                ) : (
                  <MarkdownContent text={msg.content} />
                )}
              </div>
            </div>
          );
        })}

        {/* Streaming response bubble */}
        {isStreaming && (
          <div style={{ marginBottom: 4 }}>
            <div style={{
              padding: '6px 10px', borderRadius: 2,
              background: 'transparent',
              borderLeft: '2px solid var(--green)',
            }}>
              <div style={{
                fontSize: 9, fontWeight: 600, color: 'var(--green)',
                marginBottom: 3, display: 'flex', alignItems: 'center', gap: 6,
                textTransform: 'uppercase', letterSpacing: '0.05em',
                fontFamily: "'JetBrains Mono', monospace",
              }}>
                <span>Agent</span>
                <span style={{
                  fontWeight: 400, color: 'var(--accent)',
                  textTransform: 'none', letterSpacing: 'normal',
                  animation: 'pulse 1.5s ease-in-out infinite',
                }}>
                  {status === 'responding' ? 'responding\u2026' : 'thinking\u2026'}
                </span>
              </div>
              {evts.length > 0 ? (
                <EventBlockList events={evts} />
              ) : (
                <div style={{
                  fontSize: 11, color: 'var(--text-3)',
                  fontFamily: "'JetBrains Mono', monospace",
                }}>
                  Waiting for response...
                </div>
              )}
            </div>
          </div>
        )}
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
            {((inspectedMessage._events && inspectedMessage._events.length > 0) || (inspectedMessage.events && inspectedMessage.events.length > 0)) && (
              <div style={{ marginTop: inspectedMessage._prompt ? 16 : 0 }}>
                <div style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
                  color: 'var(--green)', textTransform: 'uppercase', letterSpacing: '0.05em',
                  marginBottom: 8,
                }}>
                  Structured Events ({(inspectedMessage._events || inspectedMessage.events).length})
                </div>
                <EventBlockList events={inspectedMessage._events || inspectedMessage.events} />
              </div>
            )}
          </div>
        </Modal>
      )}
    </div>
  );
}
