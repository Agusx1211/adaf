import { useState } from 'react';
import { agentInfo } from '../../utils/colors.js';
import { formatElapsed, normalizeStatus } from '../../utils/format.js';
import { statusColor } from '../../utils/colors.js';
import StatusDot from '../common/StatusDot.jsx';
import AgentBadge from '../common/AgentBadge.jsx';
import Tag from '../common/Tag.jsx';

export default function TreeNode({ node, depth = 0, isLast = true, onSelect, selectedId, parentColor }) {
  var [expanded, setExpanded] = useState(true);
  var info = agentInfo(node.agent);
  var isSelected = selectedId === node.id;
  var hasChildren = node.children && node.children.length > 0;
  var status = normalizeStatus(node.status);
  var isRunning = status === 'running' || status === 'starting' || status === 'in_progress';

  return (
    <div style={{ animation: 'slideIn 0.3s ease-out forwards', animationDelay: (depth * 60) + 'ms', opacity: 0 }}>
      {/* Node row */}
      <div
        onClick={function () { onSelect(node); }}
        style={{
          display: 'flex',
          alignItems: 'stretch',
          cursor: 'pointer',
          position: 'relative',
        }}
      >
        {/* Tree lines */}
        {depth > 0 && (
          <div style={{ display: 'flex', alignItems: 'stretch', flexShrink: 0 }}>
            {Array.from({ length: depth }).map(function (_, i) {
              return (
                <div key={i} style={{ width: 28, position: 'relative', flexShrink: 0 }}>
                  {i === depth - 1 && (
                    <>
                      <div style={{
                        position: 'absolute', left: 13, top: 0, width: 1,
                        height: isLast ? '50%' : '100%',
                        background: (parentColor || info.color) + '30',
                      }} />
                      <div style={{
                        position: 'absolute', left: 13, top: '50%', width: 14, height: 1,
                        background: (parentColor || info.color) + '30',
                      }} />
                    </>
                  )}
                  {i < depth - 1 && (
                    <div style={{
                      position: 'absolute', left: 13, top: 0, width: 1, height: '100%',
                      background: 'var(--border)', opacity: 0.4,
                    }} />
                  )}
                </div>
              );
            })}
          </div>
        )}

        {/* Node content */}
        <div style={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          padding: '8px 12px',
          background: isSelected ? (info.color + '12') : 'transparent',
          borderLeft: isSelected ? ('2px solid ' + info.color) : '2px solid transparent',
          borderRadius: '0 4px 4px 0',
          transition: 'all 0.15s ease',
        }}
        onMouseEnter={function (e) { if (!isSelected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!isSelected) e.currentTarget.style.background = 'transparent'; }}
        >
          {/* Expand/collapse */}
          {hasChildren ? (
            <span
              onClick={function (e) { e.stopPropagation(); setExpanded(!expanded); }}
              style={{
                width: 16, height: 16,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 10, color: info.color,
                border: '1px solid ' + info.color + '40',
                borderRadius: 3, flexShrink: 0, cursor: 'pointer',
                fontFamily: "'JetBrains Mono', monospace",
                transition: 'transform 0.2s ease',
                transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)',
              }}
            >
              {'\u25BE'}
            </span>
          ) : (
            <span style={{ width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
              <span style={{ width: 5, height: 5, borderRadius: '50%', background: info.color + '60' }} />
            </span>
          )}

          {/* Status */}
          <StatusDot status={node.status} />

          {/* Agent icon + name */}
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={{ color: info.color, fontSize: 13, fontWeight: 600 }}>{info.icon}</span>
              <span style={{ fontWeight: 600, fontSize: 13, color: 'var(--text-0)' }}>{node.name}</span>
              <AgentBadge agent={node.agent} small />
              {node.profile && <Tag color={info.color}>{node.profile}</Tag>}
            </div>
            <div style={{
              fontSize: 11, color: 'var(--text-2)', marginTop: 2,
              whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: '100%',
            }}>
              {node.task}
            </div>
          </div>

          {/* Meta */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexShrink: 0 }}>
            {node.turns && (
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
                T{node.turns.current}/{node.turns.max}
              </span>
            )}
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)',
              minWidth: 48, textAlign: 'right',
            }}>
              {formatElapsed(node.startedAt || node.started_at, node.endedAt || node.completed_at)}
            </span>
            {isRunning && (
              <span style={{
                width: 12, height: 12,
                border: '2px solid ' + info.color, borderTopColor: 'transparent',
                borderRadius: '50%', animation: 'spin 1s linear infinite',
              }} />
            )}
          </div>
        </div>
      </div>

      {/* Children */}
      {expanded && hasChildren && (
        <div>
          {node.children.map(function (child, i) {
            return (
              <TreeNode
                key={child.id}
                node={child}
                depth={depth + 1}
                isLast={i === node.children.length - 1}
                onSelect={onSelect}
                selectedId={selectedId}
                parentColor={info.color}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}
