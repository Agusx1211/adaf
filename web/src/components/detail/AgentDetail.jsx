import { useMemo } from 'react';
import { statusColor, STATUSES } from '../../utils/colors.js';
import { formatElapsed, normalizeStatus, parseTimestamp, timeAgo } from '../../utils/format.js';
import { useAppState } from '../../state/store.js';
import StatusDot from '../common/StatusDot.jsx';
import AgentBadge from '../common/AgentBadge.jsx';
import Tag from '../common/Tag.jsx';
import SectionHeader from '../common/SectionHeader.jsx';
import MessageBubble from './MessageBubble.jsx';

export default function AgentDetail({ scope }) {
  var state = useAppState();
  var { sessions, spawns, messages } = state;

  var agent = useMemo(function () {
    if (!scope) return null;
    if (scope.indexOf('session-') === 0) {
      var sessionID = parseInt(scope.slice(8), 10);
      return sessions.find(function (s) { return s.id === sessionID; }) || null;
    }
    if (scope.indexOf('spawn-') === 0) {
      var spawnID = parseInt(scope.slice(6), 10);
      return spawns.find(function (s) { return s.id === spawnID; }) || null;
    }
    return null;
  }, [scope, sessions, spawns]);

  var agentMessages = useMemo(function () {
    if (!agent) return [];
    var spawnID = agent.id;
    return messages.filter(function (m) {
      return Number(m.spawn_id) === spawnID;
    });
  }, [agent, messages]);

  if (!agent) {
    return (
      <div style={{
        height: '100%', display: 'flex', flexDirection: 'column',
        alignItems: 'center', justifyContent: 'center', gap: 12, color: 'var(--text-3)',
      }}>
        <span style={{ fontSize: 32, opacity: 0.3 }}>{'\u25C9'}</span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12 }}>Select an agent from the tree</span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, opacity: 0.5 }}>Click any node to view details & communication</span>
      </div>
    );
  }

  var info = agentInfo(agent.agent);
  var isSpawn = scope && scope.indexOf('spawn-') === 0;

  var properties = [
    ['Agent', <AgentBadge agent={agent.agent || 'generic'} small />],
    ['Model', <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-1)' }}>{agent.model || 'n/a'}</span>],
    ['Profile', <span style={{ fontSize: 11, color: 'var(--text-1)' }}>{agent.profile || 'n/a'}</span>],
    ['Status', <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: statusColor(agent.status), textTransform: 'uppercase', fontWeight: 600 }}>{agent.status}</span>],
    ['Elapsed', <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--text-1)' }}>{formatElapsed(agent.started_at, agent.ended_at || agent.completed_at)}</span>],
  ];

  if (isSpawn) {
    if (agent.role) properties.push(['Role', <span style={{ fontSize: 11, color: 'var(--text-1)' }}>{agent.role}</span>]);
    if (agent.branch) properties.push(['Branch', <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--green)' }}>{agent.branch}</span>]);
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', animation: 'fadeIn 0.2s ease' }}>
      {/* Header */}
      <div style={{
        padding: '14px 16px',
        borderBottom: '1px solid var(--border)',
        background: 'linear-gradient(135deg, ' + info.color + '08, transparent)',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 }}>
          <span style={{ fontSize: 18, color: info.color }}>{info.icon}</span>
          <span style={{ fontWeight: 700, fontSize: 16 }}>{agent.profile || 'Agent'}</span>
          <StatusDot status={agent.status} size={10} />
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
            color: statusColor(agent.status), textTransform: 'uppercase', fontWeight: 600, letterSpacing: '0.05em',
          }}>{agent.status}</span>
        </div>
        {(agent.task || agent.action) && (
          <div style={{ fontSize: 12, color: 'var(--text-2)', lineHeight: 1.5 }}>{agent.task || agent.action}</div>
        )}
      </div>

      {/* Properties grid */}
      <div style={{
        display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 1,
        background: 'var(--border)', borderBottom: '1px solid var(--border)',
      }}>
        {properties.map(function (pair, i) {
          return (
            <div key={i} style={{
              padding: '6px 12px', background: 'var(--bg-1)',
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            }}>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>{pair[0]}</span>
              {pair[1]}
            </div>
          );
        })}
      </div>

      {/* Communication */}
      <SectionHeader count={agentMessages.length}>Communication</SectionHeader>
      <div style={{ flex: 1, overflow: 'auto', padding: 8 }}>
        {agentMessages.length === 0 ? (
          <div style={{ padding: 20, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>No messages yet</div>
        ) : (
          agentMessages.map(function (msg) {
            return <MessageBubble key={msg.id} msg={msg} />;
          })
        )}
      </div>
    </div>
  );
}

function agentInfo(agent) {
  var AGENT_TYPES = {
    claude: { color: '#E8A838', icon: '\u26A1', bg: 'rgba(232,168,56,0.08)' },
    codex: { color: '#4AE68A', icon: '\u25C6', bg: 'rgba(74,230,138,0.08)' },
    gemini: { color: '#7B8CFF', icon: '\u2726', bg: 'rgba(123,140,255,0.08)' },
    vibe: { color: '#FF6B9D', icon: '\u25C8', bg: 'rgba(255,107,157,0.08)' },
    opencode: { color: '#5BCEFC', icon: '\u25C9', bg: 'rgba(91,206,252,0.08)' },
    generic: { color: '#9CA3AF', icon: '\u25CB', bg: 'rgba(156,163,175,0.08)' },
  };
  return AGENT_TYPES[agent] || AGENT_TYPES.generic;
}
