import { useRef, useEffect, useMemo } from 'react';
import { useAppState } from '../../state/store.js';
import { formatTime, normalizeStatus, stringifyToolPayload, safeJSONString } from '../../utils/format.js';
import { scopeColor, scopeShortLabel } from '../../utils/colors.js';

export default function EventStream() {
  var state = useAppState();
  var containerRef = useRef(null);
  var { streamEvents, selectedScope, autoScroll } = state;

  var events = useMemo(function () {
    if (!selectedScope) return streamEvents;

    if (selectedScope.indexOf('session-') === 0) {
      return streamEvents.filter(function (e) {
        return e.scope === selectedScope || e.scope.indexOf('spawn-') === 0;
      });
    }
    if (selectedScope.indexOf('spawn-') === 0) {
      return streamEvents.filter(function (e) { return e.scope === selectedScope; });
    }
    return streamEvents;
  }, [streamEvents, selectedScope]);

  useEffect(function () {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [events, autoScroll]);

  function getEventStyle(type) {
    var normalized = normalizeStatus(type || 'text');
    if (normalized === 'thinking') return { color: '#9399b2', label: 'THINK' };
    if (normalized === 'tool_use') return { color: '#f9e2af', label: 'TOOL' };
    if (normalized === 'tool_result') return { color: '#a6e3a1', label: 'RESULT' };
    if (normalized === 'text') return { color: '#cdd6f4', label: 'TEXT' };
    return { color: '#a6adc8', label: normalized.toUpperCase().slice(0, 5) };
  }

  return (
    <div ref={containerRef} style={{ flex: 1, overflow: 'auto', fontFamily: "'JetBrains Mono', monospace", fontSize: 11 }}>
      {events.map(function (evt, i) {
        var style = getEventStyle(evt.type);
        var sColor = scopeColor(evt.scope);
        var body = '';

        if (evt.type === 'tool_use') {
          body = (evt.tool || 'tool') + ' \u2192 ' + stringifyToolPayload(evt.input || '');
        } else if (evt.type === 'tool_result') {
          body = evt.tool ? (evt.tool + ': ' + stringifyToolPayload(evt.result || evt.text || '')) : stringifyToolPayload(evt.result || evt.text || '');
        } else {
          body = evt.text || '';
        }

        return (
          <div key={evt.id || i} style={{
            display: 'flex', gap: 8, padding: '4px 12px',
            borderBottom: '1px solid var(--bg-3)',
          }}
          onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
          onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
          >
            <span style={{ display: 'flex', alignItems: 'center', gap: 4, flexShrink: 0, minWidth: 50 }}>
              <span style={{ width: 4, height: 4, borderRadius: '50%', background: sColor }} />
              <span style={{ fontSize: 9, color: sColor }}>{scopeShortLabel(evt.scope)}</span>
            </span>
            <span style={{
              color: style.color, fontWeight: 600, fontSize: 9, minWidth: 36,
              textAlign: 'center', padding: '1px 4px', background: style.color + '15',
              borderRadius: 2, flexShrink: 0,
            }}>{style.label}</span>
            <span style={{
              color: 'var(--text-1)', whiteSpace: 'pre-wrap', wordBreak: 'break-all', lineHeight: 1.5,
            }}>{body}</span>
          </div>
        );
      })}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 4, padding: '6px 12px',
        color: 'var(--text-3)', fontSize: 10,
      }}>
        <span style={{ animation: 'blink 1s step-end infinite' }}>{'\u258A'}</span>
        <span>Streaming...</span>
      </div>
    </div>
  );
}
