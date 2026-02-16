import { useMemo } from 'react';
import { statusColor } from '../../utils/colors.js';
import { formatElapsed, normalizeStatus } from '../../utils/format.js';
import { useAppState } from '../../state/store.js';

export default function AgentInfoBar({ scope }) {
  var state = useAppState();
  var { sessions, spawns } = state;

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

  if (!agent) {
    return (
      <div style={{
        padding: '8px 16px', borderBottom: '1px solid var(--border)',
        display: 'flex', alignItems: 'center', gap: 8,
        fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--text-3)',
        background: 'var(--bg-1)', flexShrink: 0,
      }}>
        Select a session from the sidebar
      </div>
    );
  }

  var info = agentInfo(agent.agent);
  var sColor = statusColor(agent.status);
  var status = normalizeStatus(agent.status);
  var isSpawn = scope && scope.indexOf('spawn-') === 0;

  return (
    <div style={{
      padding: '6px 16px', borderBottom: '1px solid var(--border)',
      display: 'flex', alignItems: 'center', gap: 10,
      background: 'linear-gradient(90deg, ' + info.color + '06, transparent)',
      flexShrink: 0,
    }}>
      {/* Agent icon */}
      <span style={{ color: info.color, fontSize: 13, flexShrink: 0 }}>{info.icon}</span>

      {/* Profile name */}
      <span style={{ fontWeight: 700, fontSize: 13, color: 'var(--text-0)' }}>
        {agent.profile || 'Agent'}
      </span>

      {/* Status */}
      <span style={{
        display: 'flex', alignItems: 'center', gap: 4,
      }}>
        <span style={{
          width: 6, height: 6, borderRadius: '50%', background: sColor,
          animation: status === 'running' ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />
        <span style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
          color: sColor, textTransform: 'uppercase', fontWeight: 600,
        }}>{agent.status}</span>
      </span>

      {/* Separator */}
      <span style={{ width: 1, height: 14, background: 'var(--border)' }} />

      {/* Model */}
      {agent.model && (
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>
          {agent.model}
        </span>
      )}

      {/* Agent type */}
      {agent.agent && (
        <span style={{
          padding: '1px 5px', background: info.color + '12', border: '1px solid ' + info.color + '25',
          borderRadius: 3, fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: info.color,
        }}>{agent.agent}</span>
      )}

      {/* Role (for spawns) */}
      {isSpawn && agent.role && (
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
          as {agent.role}
        </span>
      )}

      {/* Spacer */}
      <span style={{ flex: 1 }} />

      {/* Elapsed */}
      <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', flexShrink: 0 }}>
        {formatElapsed(agent.started_at, agent.ended_at || agent.completed_at)}
      </span>

      {/* ID */}
      <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', flexShrink: 0 }}>
        #{agent.id}
      </span>
    </div>
  );
}

function agentInfo(agent) {
  var AGENT_TYPES = {
    claude: { color: '#E8A838', icon: '\u26A1' },
    codex: { color: '#4AE68A', icon: '\u25C6' },
    gemini: { color: '#7B8CFF', icon: '\u2726' },
    vibe: { color: '#FF6B9D', icon: '\u25C8' },
    opencode: { color: '#5BCEFC', icon: '\u25C9' },
    generic: { color: '#9CA3AF', icon: '\u25CB' },
  };
  return AGENT_TYPES[agent] || AGENT_TYPES.generic;
}
