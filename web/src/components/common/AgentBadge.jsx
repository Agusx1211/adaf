import { agentInfo } from '../../utils/colors.js';

export default function AgentBadge({ agent, small }) {
  var info = agentInfo(agent);
  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: 4,
      padding: small ? '1px 6px' : '2px 8px',
      background: info.bg,
      border: '1px solid ' + info.color + '33',
      borderRadius: 4,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: small ? 10 : 11,
      fontWeight: 500,
      color: info.color,
      letterSpacing: '0.02em',
    }}>
      <span style={{ fontSize: small ? 8 : 10 }}>{info.icon}</span>
      {agent}
    </span>
  );
}
