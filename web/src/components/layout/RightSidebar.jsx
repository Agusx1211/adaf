import { useAppState } from '../../state/store.js';
import CommunicationFeed from '../feed/CommunicationFeed.jsx';
import IssueDetailPanel from '../detail/IssueDetailPanel.jsx';
import DocsDetailPanel from '../detail/DocsDetailPanel.jsx';
import PlanDetailPanel from '../detail/PlanDetailPanel.jsx';
import LogDetailPanel from '../detail/LogDetailPanel.jsx';

var detailSidebarStyle = {
  width: 420,
  minWidth: 420,
  flexShrink: 0,
  display: 'flex',
  flexDirection: 'column',
  borderLeft: '1px solid var(--border)',
  background: 'var(--bg-1)',
};

var communicationSidebarStyle = {
  width: 340,
  flexShrink: 0,
  display: 'flex',
  flexDirection: 'column',
  borderLeft: '1px solid var(--border)',
  background: 'var(--bg-1)',
};

export default function RightSidebar() {
  var state = useAppState();
  if (state.leftView === 'issues') {
    return <div style={detailSidebarStyle}><IssueDetailPanel /></div>;
  }
  if (state.leftView === 'docs') {
    return <div style={detailSidebarStyle}><DocsDetailPanel /></div>;
  }
  if (state.leftView === 'plan') {
    return <div style={detailSidebarStyle}><PlanDetailPanel /></div>;
  }
  if (state.leftView === 'logs') {
    return <div style={detailSidebarStyle}><LogDetailPanel /></div>;
  }

  return (
    <div style={communicationSidebarStyle}>
      <CommunicationFeed />
    </div>
  );
}
