import CommunicationFeed from '../feed/CommunicationFeed.jsx';

export default function RightSidebar() {
  return (
    <div style={{
      width: 340, flexShrink: 0, display: 'flex', flexDirection: 'column',
      borderLeft: '1px solid var(--border)', background: 'var(--bg-1)',
    }}>
      <CommunicationFeed />
    </div>
  );
}
