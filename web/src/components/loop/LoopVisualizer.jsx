import { useAppState } from '../../state/store.js';
import { agentInfo, STATUSES } from '../../utils/colors.js';
import { normalizeStatus, formatElapsed, timeAgo } from '../../utils/format.js';
import StatusDot from '../common/StatusDot.jsx';
import SectionHeader from '../common/SectionHeader.jsx';

export default function LoopVisualizer() {
  var state = useAppState();
  var loop = state.loopRun;

  if (!loop) {
    return (
      <div style={{
        height: '100%', display: 'flex', flexDirection: 'column',
        alignItems: 'center', justifyContent: 'center', gap: 12, color: 'var(--text-3)',
      }}>
        <span style={{ fontSize: 32, opacity: 0.3 }}>{'\u21BB'}</span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12 }}>No active loop run</span>
      </div>
    );
  }

  var steps = loop.steps || [];
  var currentStep = loop.step_index || 0;
  var cycle = loop.cycle || 0;
  var isRunning = normalizeStatus(loop.status) === 'running';

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      {/* Header */}
      <div style={{
        padding: '14px 16px', borderBottom: '1px solid var(--border)',
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 14, fontWeight: 700, color: 'var(--text-0)' }}>
              {'\u21BB'} {loop.loop_name || 'loop'}
            </span>
            <StatusDot status={isRunning ? 'running' : 'completed'} size={10} />
          </div>
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', marginTop: 2 }}>
            Run: {loop.hex_id || loop.id}
          </div>
        </div>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 12,
          padding: '6px 14px', background: 'var(--bg-2)', borderRadius: 6, border: '1px solid var(--border)',
        }}>
          <div style={{ textAlign: 'center' }}>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 18, fontWeight: 700, color: 'var(--accent)' }}>{cycle + 1}</div>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 8, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.1em' }}>Cycle</div>
          </div>
          <div style={{ width: 1, height: 28, background: 'var(--border)' }} />
          <div style={{ textAlign: 'center' }}>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 18, fontWeight: 700, color: 'var(--text-2)' }}>{steps.length || '?'}</div>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 8, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.1em' }}>Steps</div>
          </div>
        </div>
      </div>

      {/* Steps visualization */}
      {steps.length > 0 && (
        <div style={{ padding: 16, borderBottom: '1px solid var(--border)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 0 }}>
            {steps.map(function (step, i) {
              var isCurrent = i === currentStep;
              var isDone = i < currentStep;
              var stepColor = isDone ? 'var(--green)' : isCurrent ? 'var(--accent)' : 'var(--text-3)';

              return (
                <div key={i} style={{ flex: 1, display: 'flex', alignItems: 'center' }}>
                  <div style={{ flex: 0, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6, position: 'relative' }}>
                    <div style={{
                      width: isCurrent ? 40 : 32, height: isCurrent ? 40 : 32,
                      borderRadius: '50%', border: '2px solid ' + stepColor,
                      background: isCurrent ? 'var(--accent)15' : 'var(--bg-1)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      fontSize: isCurrent ? 16 : 13, color: stepColor,
                      boxShadow: isCurrent ? '0 0 16px var(--accent)30' : 'none',
                      transition: 'all 0.3s ease', position: 'relative',
                    }}>
                      {isDone ? '\u2713' : (i + 1)}
                      {isCurrent && (
                        <div style={{
                          position: 'absolute', inset: -4, borderRadius: '50%',
                          border: '1px solid var(--accent)40', animation: 'pulse 2s ease-in-out infinite',
                        }} />
                      )}
                    </div>
                    <div style={{ textAlign: 'center' }}>
                      <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600, color: stepColor }}>
                        {step.profile || 'step-' + (i + 1)}
                      </div>
                      {step.position && (
                        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 8, color: 'var(--text-3)' }}>
                          {step.position}
                        </div>
                      )}
                    </div>
                  </div>
                  {i < steps.length - 1 && (
                    <div style={{
                      flex: 1, height: 2,
                      background: isDone ? 'var(--green)' : 'var(--bg-4)',
                      marginBottom: 30, marginLeft: 4, marginRight: 4, borderRadius: 1,
                    }} />
                  )}
                </div>
              );
            })}
            <div style={{
              display: 'flex', alignItems: 'center', marginBottom: 30,
              color: 'var(--text-3)', fontFamily: "'JetBrains Mono', monospace", fontSize: 16, marginLeft: 8,
            }}>{'\u21BB'}</div>
          </div>
        </div>
      )}

      {/* Info */}
      <div style={{ flex: 1, overflow: 'auto', padding: 16 }}>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--text-2)' }}>
          Status: {loop.status} {'\u00B7'} Elapsed: {formatElapsed(loop.started_at)}
        </div>
      </div>
    </div>
  );
}
