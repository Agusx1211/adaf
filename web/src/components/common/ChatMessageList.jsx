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
  showSourceLabels,
  scrollContextKey,
}) {
  var evts = streamEvents || [];
  var msgs = messages || [];
  var empty = emptyMessage || 'No output yet';
  var shouldAutoScroll = autoScroll !== false;
  var status = streamStatus || (evts.length > 0 ? 'responding' : 'thinking');

  var [inspectedMessage, setInspectedMessage] = useState(null);
  var [isPinnedBottom, setIsPinnedBottom] = useState(true);
  var listRef = useRef(null);
  var pinnedBottomRef = useRef(true);

  useEffect(function () { injectEventBlockStyles(); }, []);

  useEffect(function () {
    pinnedBottomRef.current = true;
    setIsPinnedBottom(true);
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [scrollContextKey]);

  function nearBottom(el) {
    if (!el) return true;
    var distance = el.scrollHeight - (el.scrollTop + el.clientHeight);
    return distance <= 24;
  }

  function updatePinnedBottom(next) {
    if (pinnedBottomRef.current === next) return;
    pinnedBottomRef.current = next;
    setIsPinnedBottom(next);
  }

  function handleScroll() {
    updatePinnedBottom(nearBottom(listRef.current));
  }

  useEffect(function () {
    if (shouldAutoScroll && listRef.current && pinnedBottomRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [msgs, evts, shouldAutoScroll, isStreaming]);

  var wrapperStyle = { flex: 1, minHeight: 0, position: 'relative', display: 'flex', flexDirection: 'column' };
  var listStyle = { flex: 1, minHeight: 0, overflow: 'auto', padding: '6px 12px' };
  var scrollHintVisible = shouldAutoScroll && !isPinnedBottom;

  var scrollHint = (
    <button
      type="button"
      onClick={function () {
        if (!listRef.current) return;
        listRef.current.scrollTop = listRef.current.scrollHeight;
        updatePinnedBottom(true);
      }}
      style={{
        position: 'absolute', right: 12, bottom: 10,
        fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
        padding: '4px 8px', borderRadius: 4, border: '1px solid var(--accent)40',
        background: 'var(--bg-2)', color: 'var(--accent)', cursor: 'pointer',
        zIndex: 3,
      }}
    >
      Jump to latest
    </button>
  );

  if (loading) {
    return (
      <div style={wrapperStyle}>
        <div ref={listRef} onScroll={handleScroll} style={listStyle}>
          <div style={{ textAlign: 'center', color: 'var(--text-3)', padding: 40 }}>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11 }}>Loading...</div>
          </div>
        </div>
        {scrollHintVisible && scrollHint}
      </div>
    );
  }

  if (msgs.length === 0 && !isStreaming) {
    return (
      <div style={wrapperStyle}>
        <div ref={listRef} onScroll={handleScroll} style={listStyle}>
          <div style={{ textAlign: 'center', color: 'var(--text-3)', padding: 40 }}>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, color: 'var(--text-2)' }}>
              {empty}
            </div>
          </div>
        </div>
        {scrollHintVisible && scrollHint}
      </div>
    );
  }

  return (
    <div style={wrapperStyle}>
      <div ref={listRef} onScroll={handleScroll} style={listStyle}>
        <div>
          {msgs.map(function (msg) {
            var isUser = msg.role === 'user';
            var msgEvents = Array.isArray(msg._events) ? msg._events : (Array.isArray(msg.events) ? msg.events : []);
            var hasPromptEvent = msgEvents.some(function (e) { return e && e.type === 'initial_prompt'; });
            var renderEvents = msgEvents;
            var ageLabel = timeAgo(msg.created_at);
            var continuationLabel = isUser ? String(msg._continuationLabel || '').trim() : '';
            if (!isUser && msg._prompt && msg._prompt.text && !hasPromptEvent) {
              renderEvents = [{ type: 'initial_prompt', content: msg._prompt.text }].concat(msgEvents);
            }
            var hasInspectData = !isUser && (msg._prompt || renderEvents.length > 0);
            return (
              <div key={msg.id} style={{ marginBottom: 4 }}>
                {!!continuationLabel && (
                  <div style={{
                    marginBottom: 4,
                    paddingLeft: 10,
                    fontFamily: "'JetBrains Mono', monospace",
                    fontSize: 10,
                    color: 'var(--text-3)',
                  }}>
                    {continuationLabel}
                  </div>
                )}
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
                    {ageLabel && (
                      <span style={{ fontWeight: 400, color: 'var(--text-3)', textTransform: 'none', letterSpacing: 'normal', fontSize: 9 }}>
                        {ageLabel}
                      </span>
                    )}
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
                  ) : renderEvents.length > 0 ? (
                    showSourceLabels ? renderLabeledEvents(renderEvents) : <EventBlockList events={renderEvents} />
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
                  showSourceLabels ? renderLabeledEvents(evts) : <EventBlockList events={evts} />
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
      {scrollHintVisible && scrollHint}
    </div>
  );
}

// --- Source-labeled event rendering for "all agents" view ---

function renderLabeledEvents(events) {
  // Group consecutive events by source, with _spawn_status as standalone items
  var groups = [];
  var currentLabel = null;
  var currentColor = null;
  var currentGroup = [];

  events.forEach(function (evt) {
    if (evt.type === '_spawn_status') {
      // Flush current group
      if (currentGroup.length > 0) {
        groups.push({ type: 'events', label: currentLabel, color: currentColor, events: currentGroup.slice() });
        currentGroup = [];
      }
      groups.push({ type: 'status', evt: evt });
      currentLabel = null;
      currentColor = null;
      return;
    }

    var label = evt._sourceLabel || 'parent';
    var color = evt._sourceColor || 'var(--text-2)';
    if (label !== currentLabel) {
      if (currentGroup.length > 0) {
        groups.push({ type: 'events', label: currentLabel, color: currentColor, events: currentGroup.slice() });
        currentGroup = [];
      }
      currentLabel = label;
      currentColor = color;
    }
    currentGroup.push(evt);
  });

  if (currentGroup.length > 0) {
    groups.push({ type: 'events', label: currentLabel, color: currentColor, events: currentGroup });
  }

  return groups.map(function (group, i) {
    if (group.type === 'status') {
      return renderSpawnStatusCard(group.evt, i);
    }
    return (
      <div key={'g-' + i}>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 6,
          marginTop: i > 0 ? 8 : 0, marginBottom: 4,
        }}>
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 9, fontWeight: 600,
            padding: '1px 6px', borderRadius: 8,
            background: (group.color || 'var(--text-3)') + '18',
            color: group.color || 'var(--text-3)',
            border: '1px solid ' + (group.color || 'var(--text-3)') + '30',
          }}>
            {group.label}
          </span>
        </div>
        <EventBlockList events={group.events} />
      </div>
    );
  });
}

function renderSpawnStatusCard(evt, key) {
  var action = evt._action || '';
  var profile = evt._profile || 'spawn';
  var color = evt._sourceColor || 'var(--text-3)';
  var failureDetail = String(evt._result || evt._summary || '').trim();
  if (failureDetail.length > 140) failureDetail = failureDetail.slice(0, 137) + '...';
  var text = '';

  if (action === 'started') {
    text = '[spawn #' + evt._spawnID + '] ' + profile + (evt._role ? ' (' + evt._role + ')' : '') + ' started';
  } else if (action === 'completed') {
    text = '[spawn #' + evt._spawnID + '] ' + profile + ' completed';
  } else if (action === 'failed') {
    text = '[spawn #' + evt._spawnID + '] ' + profile + ' failed';
    if (failureDetail) text += ': ' + failureDetail;
  } else if (action === 'asking') {
    text = '[spawn #' + evt._spawnID + '] ' + profile + ' is asking: ' + (evt._question || '');
  } else {
    text = '[spawn #' + evt._spawnID + '] ' + profile + ' ' + action;
  }

  return (
    <div key={'s-' + key} style={{
      margin: '6px 0', padding: '4px 10px',
      background: color + '08',
      border: '1px solid ' + color + '20',
      borderRadius: 4, textAlign: 'center',
    }}>
      <span style={{
        fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
        color: color,
      }}>
        {text}
      </span>
    </div>
  );
}
