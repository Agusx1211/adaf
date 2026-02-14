import { useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import TabBar from '../common/TabBar.jsx';
import EventStream from '../feed/EventStream.jsx';
import { SessionMessageBar } from '../session/SessionControls.jsx';
import { normalizeStatus, formatNumber } from '../../utils/format.js';
import { scopeColor, scopeShortLabel, STATUS_RUNNING } from '../../utils/colors.js';
import { withAlpha } from '../../utils/format.js';

export default function BottomBar() {
  var state = useAppState();
  var dispatch = useDispatch();
  var [activeTab, setActiveTab] = useState('events');
  var { streamEvents, selectedScope, autoScroll, spawns, rightLayer } = state;

  var tabs = [
    { id: 'events', label: 'Event Stream', icon: '\u25B8', color: 'var(--green)', count: streamEvents.length },
    { id: 'activity', label: 'Activity', icon: '\u25AA', color: 'var(--text-2)' },
  ];

  function renderActivityFeed() {
    var entries = (state.activity || []).slice().reverse().slice(0, 80);
    if (!entries.length) {
      return <div style={{ padding: 16, textAlign: 'center', color: 'var(--text-3)', fontSize: 11 }}>No activity yet.</div>;
    }

    return (
      <div style={{ flex: 1, overflow: 'auto', padding: '4px 0' }}>
        {entries.map(function (entry, idx) {
          var type = normalizeStatus(entry.type || 'text');
          var color = type === 'tool_use' ? '#f9e2af' : type === 'tool_result' ? '#a6e3a1' : '#b4befe';
          var icon = type === 'tool_use' ? '\u2699' : type === 'tool_result' ? '\u2713' : '\u2022';
          return (
            <div key={entry.id || idx} style={{
              display: 'flex', gap: 8, padding: '3px 12px', fontSize: 11,
              fontFamily: "'JetBrains Mono', monospace",
            }}>
              <span style={{ color: color, flexShrink: 0 }}>{icon}</span>
              <span style={{ color: scopeColor(entry.scope), fontSize: 9, flexShrink: 0 }}>{scopeShortLabel(entry.scope)}</span>
              <span style={{ color: 'var(--text-1)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{entry.text}</span>
            </div>
          );
        })}
      </div>
    );
  }

  return (
    <div style={{
      height: 200, flexShrink: 0, display: 'flex', flexDirection: 'column',
      borderTop: '1px solid var(--border)', background: 'var(--bg-1)',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <TabBar tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab} />
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '0 12px' }}>
          {selectedScope && (
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)',
              display: 'flex', alignItems: 'center', gap: 4,
            }}>
              scope: <b style={{ color: scopeColor(selectedScope) }}>{scopeShortLabel(selectedScope)}</b>
              <button
                onClick={function () { dispatch({ type: 'SET_SELECTED_SCOPE', payload: null }); }}
                style={{ background: 'none', border: 'none', color: 'var(--text-3)', cursor: 'pointer', fontSize: 10 }}
                title="Clear scope"
              >{'\u00D7'}</button>
            </span>
          )}
          <button
            onClick={function () { dispatch({ type: 'TOGGLE_AUTO_SCROLL' }); }}
            style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              background: autoScroll ? 'var(--accent)15' : 'var(--bg-3)',
              border: '1px solid ' + (autoScroll ? 'var(--accent)40' : 'var(--border)'),
              color: autoScroll ? 'var(--accent)' : 'var(--text-3)',
              padding: '2px 6px', borderRadius: 3, cursor: 'pointer',
            }}
          >auto-scroll {autoScroll ? 'on' : 'off'}</button>
        </div>
      </div>
      {activeTab === 'events' ? <EventStream /> : renderActivityFeed()}
      <SessionMessageBar />
    </div>
  );
}
