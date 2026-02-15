import { useMemo, useRef, useEffect } from 'react';
import { useAppState } from '../../state/store.js';
import { EventBlockList, injectEventBlockStyles, stateEventsToBlocks } from '../common/EventBlocks.jsx';

export default function AgentOutput({ scope }) {
  var state = useAppState();
  var { streamEvents, autoScroll } = state;
  var containerRef = useRef(null);

  useEffect(function () { injectEventBlockStyles(); }, []);

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

  useEffect(function () {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [blockEvents, autoScroll]);

  if (!scope) {
    return (
      <div style={{
        height: '100%', display: 'flex', flexDirection: 'column',
        alignItems: 'center', justifyContent: 'center', gap: 12, color: 'var(--text-3)',
      }}>
        <span style={{ fontSize: 32, opacity: 0.3 }}>{'\u25A3'}</span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12 }}>Select an agent from the tree</span>
      </div>
    );
  }

  if (blockEvents.length === 0) {
    return (
      <div style={{
        height: '100%', display: 'flex', flexDirection: 'column',
        alignItems: 'center', justifyContent: 'center', gap: 12, color: 'var(--text-3)',
      }}>
        <span style={{ fontSize: 32, opacity: 0.3 }}>{'\u25A3'}</span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12 }}>No output yet</span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, opacity: 0.5 }}>Events will appear here as the agent runs</span>
      </div>
    );
  }

  return (
    <div ref={containerRef} style={{ height: '100%', overflow: 'auto', padding: '16px 20px' }}>
      <div style={{ maxWidth: 800, margin: '0 auto' }}>
        <EventBlockList events={blockEvents} />
      </div>
    </div>
  );
}
