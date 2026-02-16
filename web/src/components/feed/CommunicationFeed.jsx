import { useMemo } from 'react';
import { useAppState } from '../../state/store.js';
import { normalizeStatus, timeAgo, parseTimestamp } from '../../utils/format.js';
import { buildSpawnScopeMaps, parseScope } from '../../utils/scopes.js';
import SectionHeader from '../common/SectionHeader.jsx';

export default function CommunicationFeed() {
  var state = useAppState();
  var { messages, spawns, loopRuns, selectedScope } = state;

  var spawnScopeMaps = useMemo(function () {
    return buildSpawnScopeMaps(spawns, loopRuns);
  }, [spawns, loopRuns]);

  var displayMessages = useMemo(function () {
    var list = messages.slice();

    // Add synthetic messages from spawn state
    var seenAsk = {};
    list.forEach(function (msg) {
      if (msg.type === 'ask' && msg.spawn_id) seenAsk[msg.spawn_id] = true;
    });

    spawns.forEach(function (spawn) {
      if (normalizeStatus(spawn.status) === 'awaiting_input' && spawn.question && !seenAsk[spawn.id]) {
        list.push({
          id: 'ask-' + spawn.id,
          spawn_id: spawn.id,
          type: 'ask',
          direction: 'child_to_parent',
          content: spawn.question,
          created_at: spawn.started_at || new Date().toISOString(),
          step_index: null,
        });
      }
      if (spawn.summary && normalizeStatus(spawn.status) === 'completed') {
        list.push({
          id: 'reply-' + spawn.id,
          spawn_id: spawn.id,
          type: 'reply',
          direction: 'child_to_parent',
          content: spawn.summary,
          created_at: spawn.completed_at || spawn.started_at || new Date().toISOString(),
          step_index: null,
        });
      }
    });

    // Filter by scope
    var parsedScope = parseScope(selectedScope);
    if (parsedScope.kind === 'spawn') {
      var targetSpawn = parsedScope.id;
      if (targetSpawn > 0) {
        list = list.filter(function (m) { return Number(m.spawn_id) === targetSpawn; });
      }
    } else if (parsedScope.kind === 'session' || parsedScope.kind === 'session_main') {
      var targetSession = parsedScope.id;
      if (targetSession > 0) {
        list = list.filter(function (m) {
          if (!m.spawn_id) return true;
          var mappedSession = spawnScopeMaps.spawnToSession[Number(m.spawn_id)] || 0;
          return mappedSession === targetSession;
        });
      }
    }

    list.sort(function (a, b) { return parseTimestamp(b.created_at) - parseTimestamp(a.created_at); });
    return list;
  }, [messages, spawns, selectedScope, spawnScopeMaps]);

  var pendingQuestions = displayMessages.filter(function (m) { return m.type === 'ask'; }).length;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <SectionHeader
        count={displayMessages.length}
        action={pendingQuestions > 0 && (
          <span style={{
            display: 'flex', alignItems: 'center', gap: 4,
            fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--orange)',
          }}>
            <span style={{ width: 5, height: 5, borderRadius: '50%', background: 'var(--orange)', animation: 'pulse 1.5s infinite' }} />
            {pendingQuestions} unanswered
          </span>
        )}
      >Communication</SectionHeader>
      <div style={{ flex: 1, overflow: 'auto' }}>
        {displayMessages.length === 0 ? (
          <div style={{ padding: 20, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>No messages yet</div>
        ) : displayMessages.map(function (msg) {
          var type = normalizeStatus(msg.type || 'message');
          if (type !== 'ask' && type !== 'reply') type = 'message';
          var typeColors = { ask: 'var(--orange)', reply: 'var(--green)', message: 'var(--text-2)' };
          var color = typeColors[type] || typeColors.message;
          var direction = normalizeStatus(msg.direction) === 'parent_to_child' ? '\u2193' : '\u2191';
          var spawnLabel = msg.spawn_id ? 'spawn #' + msg.spawn_id : (msg.step_index != null ? 'step ' + msg.step_index : 'loop');
          var spawn = msg.spawn_id ? spawns.find(function (s) { return s.id === Number(msg.spawn_id); }) : null;

          return (
            <div key={msg.id} style={{
              padding: '10px 14px', borderBottom: '1px solid var(--bg-3)', animation: 'slideIn 0.2s ease-out',
            }}
            onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
            onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
                <span style={{
                  padding: '1px 5px', background: color + '15', border: '1px solid ' + color + '30',
                  borderRadius: 3, fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                  color: color, textTransform: 'uppercase', fontWeight: 600,
                }}>{type}</span>
                <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-1)' }}>
                  {spawn ? spawn.profile || spawnLabel : spawnLabel}
                </span>
                <span style={{ color: 'var(--text-3)', fontSize: 10 }}>{direction}</span>
                <span style={{ marginLeft: 'auto', fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
                  {timeAgo(msg.created_at)}
                </span>
              </div>
              <div style={{ fontSize: 12, color: 'var(--text-1)', lineHeight: 1.4, marginLeft: 2 }}>
                {msg.content && msg.content.length > 120 ? msg.content.slice(0, 120) + '\u2026' : msg.content}
              </div>
              {type === 'ask' && (
                <div style={{
                  marginTop: 4, display: 'inline-flex', alignItems: 'center', gap: 4,
                  padding: '2px 8px', background: 'rgba(232,168,56,0.1)', borderRadius: 3,
                }}>
                  <span style={{ width: 5, height: 5, borderRadius: '50%', background: 'var(--orange)', animation: 'pulse 1.5s ease-in-out infinite' }} />
                  <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--orange)' }}>Needs reply</span>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
