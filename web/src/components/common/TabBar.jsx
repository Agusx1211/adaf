export default function TabBar({ tabs, activeTab, onTabChange }) {
  return (
    <div style={{
      display: 'flex',
      borderBottom: '1px solid var(--border)',
      background: 'var(--bg-1)',
    }}>
      {tabs.map(function (tab) {
        return (
          <button
            key={tab.id}
            onClick={function () { onTabChange(tab.id); }}
            style={{
              padding: '8px 14px',
              border: 'none',
              background: activeTab === tab.id ? 'var(--bg-2)' : 'transparent',
              color: activeTab === tab.id ? 'var(--text-0)' : 'var(--text-3)',
              fontFamily: "'JetBrains Mono', monospace",
              fontSize: 11,
              fontWeight: activeTab === tab.id ? 600 : 400,
              cursor: 'pointer',
              borderBottom: activeTab === tab.id ? '2px solid ' + (tab.color || 'var(--accent)') : '2px solid transparent',
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              transition: 'all 0.15s ease',
              letterSpacing: '0.02em',
            }}
          >
            {tab.icon && <span style={{ fontSize: 10, opacity: 0.7 }}>{tab.icon}</span>}
            {tab.label}
            {tab.count !== undefined && (
              <span style={{
                background: activeTab === tab.id ? (tab.color || 'var(--accent)') + '30' : 'var(--bg-4)',
                color: activeTab === tab.id ? (tab.color || 'var(--accent)') : 'var(--text-3)',
                fontSize: 9,
                padding: '0px 4px',
                borderRadius: 3,
                fontWeight: 600,
              }}>
                {tab.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
