import { STATUSES } from '../../utils/colors.js';

export default function StatusDot({ status, size = 8 }) {
  return (
    <span style={{
      display: 'inline-block',
      width: size,
      height: size,
      borderRadius: '50%',
      background: STATUSES[status] || '#666',
      boxShadow: status === 'running' ? '0 0 8px ' + (STATUSES[status] || '#666') : 'none',
      animation: status === 'running' ? 'pulse 2s ease-in-out infinite' : 'none',
      flexShrink: 0,
    }} />
  );
}
