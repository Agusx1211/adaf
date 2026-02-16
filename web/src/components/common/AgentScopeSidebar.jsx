import { useMemo } from 'react';
import { agentInfo, statusColor, STATUS_RUNNING } from '../../utils/colors.js';
import { normalizeStatus } from '../../utils/format.js';

export default function AgentScopeSidebar({
  spawns,
  selectedScope,
  onSelectScope,
  parentLabel,
  parentSubLabel,
  parentColor,
  parentActive,
  title,
  showAll,
  allLabel,
  allCount,
}) {
  var list = Array.isArray(spawns) ? spawns : [];
  var scope = String(selectedScope || '');
  var resolvedParentLabel = parentLabel || 'Parent';
  var resolvedParentSubLabel = parentSubLabel || 'main agent';
  var resolvedParentColor = parentColor || agentInfo('').color;
  var resolvedTitle = title || 'Agents';
  var resolvedShowAll = showAll !== false;
  var resolvedAllLabel = allLabel || 'All agents';
  var resolvedAllCount = Number.isFinite(Number(allCount)) ? Number(allCount) : (list.length + 1);

  var tree = useMemo(function () {
    var childrenByParent = {};
    var roots = [];
    list.forEach(function (spawn) {
      if (!spawn || spawn.id <= 0) return;
      if (spawn.parent_spawn_id > 0) {
        if (!childrenByParent[spawn.parent_spawn_id]) childrenByParent[spawn.parent_spawn_id] = [];
        childrenByParent[spawn.parent_spawn_id].push(spawn);
      } else {
        roots.push(spawn);
      }
    });
    return { roots: roots, childrenByParent: childrenByParent };
  }, [list]);

  var parentSelected = scope === 'parent';
  var allSelected = scope === 'all';

  return (
    <div style={{
      width: 240, borderLeft: '1px solid var(--border)',
      background: 'var(--bg-1)', overflow: 'auto', flexShrink: 0,
    }}>
      <div style={{
        padding: '8px 12px', borderBottom: '1px solid var(--border)',
        fontFamily: "'JetBrains Mono', monospace", fontSize: 9, fontWeight: 600,
        color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.1em',
      }}>
        {resolvedTitle}
      </div>

      <div
        onClick={function () { onSelectScope('parent'); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '8px 12px', cursor: 'pointer',
          background: parentSelected ? (resolvedParentColor + '12') : 'transparent',
          borderLeft: parentSelected ? ('2px solid ' + resolvedParentColor) : '2px solid transparent',
          transition: 'all 0.15s ease',
        }}
        onMouseEnter={function (e) { if (!parentSelected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!parentSelected) e.currentTarget.style.background = 'transparent'; }}
      >
        <span style={{
          width: 7, height: 7, borderRadius: '50%', flexShrink: 0,
          background: parentActive ? '#a6e3a1' : 'var(--text-3)',
          boxShadow: parentActive ? '0 0 6px #a6e3a1' : 'none',
          animation: parentActive ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
              color: 'var(--text-0)',
            }}>
              {resolvedParentLabel}
            </span>
          </div>
          <div style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
            color: 'var(--text-3)', marginTop: 1,
          }}>
            {resolvedParentSubLabel}
          </div>
        </div>
      </div>

      {resolvedShowAll && (
        <div
          onClick={function () { onSelectScope('all'); }}
          style={{
            display: 'flex', alignItems: 'center', gap: 8,
            padding: '8px 12px', cursor: 'pointer',
            background: allSelected ? 'var(--accent)12' : 'transparent',
            borderLeft: allSelected ? '2px solid var(--accent)' : '2px solid transparent',
            transition: 'all 0.15s ease',
          }}
          onMouseEnter={function (e) { if (!allSelected) e.currentTarget.style.background = 'var(--bg-3)'; }}
          onMouseLeave={function (e) { if (!allSelected) e.currentTarget.style.background = 'transparent'; }}
        >
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
            color: 'var(--text-2)', flexShrink: 0,
          }}>{'\u2261'}</span>
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
            color: 'var(--text-1)',
          }}>
            {resolvedAllLabel}
          </span>
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
            color: 'var(--text-3)', padding: '1px 5px',
            background: 'var(--bg-3)', borderRadius: 3, marginLeft: 'auto',
          }}>
            {resolvedAllCount}
          </span>
        </div>
      )}

      {(resolvedShowAll || tree.roots.length > 0) && (
        <div style={{ borderBottom: '1px solid var(--border)', margin: '4px 0' }} />
      )}

      {tree.roots.map(function (spawn) {
        return (
          <SpawnTreeNode
            key={spawn.id}
            spawn={spawn}
            depth={0}
            childrenByParent={tree.childrenByParent}
            selectedScope={scope}
            onSelect={onSelectScope}
          />
        );
      })}
    </div>
  );
}

function SpawnTreeNode({ spawn, depth, childrenByParent, selectedScope, onSelect }) {
  var children = childrenByParent[spawn.id] || [];
  var selected = selectedScope === ('spawn-' + spawn.id);
  var sColor = statusColor(spawn.status);
  var status = normalizeStatus(spawn.status);
  var isRunning = !!STATUS_RUNNING[status];
  var hasPendingQuestion = status === 'awaiting_input' && !!spawn.question;

  return (
    <div style={{ marginLeft: depth * 14 }}>
      <div
        onClick={function () { onSelect('spawn-' + spawn.id); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 7,
          padding: '6px 12px', cursor: 'pointer',
          background: selected ? (sColor + '12') : 'transparent',
          borderLeft: selected ? ('2px solid ' + sColor) : '2px solid transparent',
          transition: 'all 0.15s ease',
        }}
        onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
      >
        <span style={{
          width: 6, height: 6, borderRadius: '50%', flexShrink: 0,
          background: sColor,
          boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
          animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              color: 'var(--text-3)',
            }}>
              #{spawn.id}
            </span>
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
              color: 'var(--text-0)',
            }}>
              {spawn.profile || 'spawn'}
            </span>
            {spawn.role && (
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                color: 'var(--text-3)',
              }}>
                as {spawn.role}
              </span>
            )}
          </div>
          {hasPendingQuestion && (
            <div style={{
              marginTop: 3, display: 'flex', alignItems: 'center', gap: 4,
            }}>
              <span style={{
                width: 5, height: 5, borderRadius: '50%',
                background: '#89b4fa',
                animation: 'pulse 1.5s ease-in-out infinite',
              }} />
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 8,
                color: '#89b4fa', fontWeight: 600,
              }}>
                AWAITING RESPONSE
              </span>
            </div>
          )}
        </div>
        {isRunning && (
          <span style={{
            width: 8, height: 8, border: '1.5px solid ' + sColor, borderTopColor: 'transparent',
            borderRadius: '50%', animation: 'spin 1s linear infinite', flexShrink: 0,
          }} />
        )}
      </div>

      {children.map(function (child) {
        return (
          <SpawnTreeNode
            key={child.id}
            spawn={child}
            depth={depth + 1}
            childrenByParent={childrenByParent}
            selectedScope={selectedScope}
            onSelect={onSelect}
          />
        );
      })}
    </div>
  );
}
