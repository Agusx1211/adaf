import { useAppState, useDispatch } from '../../state/store.js';
import { cropText } from '../../utils/format.js';

export default function DocsView() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { docs } = state;
  var selectedDoc = state.selectedDoc;

  if (!docs.length) {
    return <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>No docs available.</div>;
  }

  return (
    <div style={{ overflow: 'auto', flex: 1 }}>
      {docs.map(function (doc) {
        var selected = selectedDoc === doc.id;
        return (
          <div key={doc.id} style={{
            padding: '10px 14px', borderBottom: '1px solid var(--border)', cursor: 'pointer',
            background: selected ? 'var(--bg-3)' : 'transparent',
            transition: 'background 0.15s ease',
          }}
          onClick={function () { dispatch({ type: 'SET_SELECTED_DOC', payload: doc.id }); }}
          onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-2)'; }}
          onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
              <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-0)' }}>{doc.title || doc.id || 'Document'}</span>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>{doc.id}</span>
            </div>
            <div style={{ fontSize: 11, color: 'var(--text-2)', lineHeight: 1.4 }}>
              {cropText((doc.content || '').replace(/\s+/g, ' ').trim(), 160)}
            </div>
          </div>
        );
      })}
    </div>
  );
}
