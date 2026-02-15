import { useEffect, useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiBase, apiCall } from '../../api/client.js';
import { MarkdownContent, injectEventBlockStyles } from '../common/EventBlocks.jsx';
import SectionHeader from '../common/SectionHeader.jsx';
import { useToast } from '../common/Toast.jsx';

var buildStateOptions = ['unknown', 'passing', 'failing', 'in_progress', 'complete', 'stopped', 'cancelled'];

export default function LogDetailPanel() {
  var state = useAppState();
  var dispatch = useDispatch();
  var toast = useToast();
  var base = apiBase(state.currentProjectID);
  var turn = state.turns.find(function (item) { return item.id === state.selectedTurn; });

  var [isEditing, setIsEditing] = useState(false);
  var [saving, setSaving] = useState(false);
  var [editObjective, setEditObjective] = useState('');
  var [editBuilt, setEditBuilt] = useState('');
  var [editKeyDecisions, setEditKeyDecisions] = useState('');
  var [editChallenges, setEditChallenges] = useState('');
  var [editCurrentState, setEditCurrentState] = useState('');
  var [editKnownIssues, setEditKnownIssues] = useState('');
  var [editNextSteps, setEditNextSteps] = useState('');
  var [editBuildState, setEditBuildState] = useState('unknown');
  var [editDuration, setEditDuration] = useState('');

  useEffect(function () {
    injectEventBlockStyles();
  }, []);

  useEffect(function () {
    if (!turn) {
      setIsEditing(false);
      setEditObjective('');
      setEditBuilt('');
      setEditKeyDecisions('');
      setEditChallenges('');
      setEditCurrentState('');
      setEditKnownIssues('');
      setEditNextSteps('');
      setEditBuildState('unknown');
      setEditDuration('');
      return;
    }

    if (!isEditing) {
      setEditObjective(turn.objective || '');
      setEditBuilt(turn.what_was_built || '');
      setEditKeyDecisions(turn.key_decisions || '');
      setEditChallenges(turn.challenges || '');
      setEditCurrentState(turn.current_state || '');
      setEditKnownIssues(turn.known_issues || '');
      setEditNextSteps(turn.next_steps || '');
      setEditBuildState(turn.build_state || 'unknown');
      setEditDuration(String(turn.duration_secs || 0));
    }
  }, [turn && turn.id]);

  if (!turn) {
    return (
      <div style={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)' }}>
        Select a log
      </div>
    );
  }

  function buildMarkdown(data) {
    return (
      '# Turn #' + (data.id || '') + '\n\n' +
      '**Hex ID:** `' + (data.hex_id || 'n/a') + '`  \n' +
      (data.loop_run_hex_id ? ('**Loop Run:** `' + data.loop_run_hex_id + '`  \n') : '') +
      (data.step_hex_id ? ('**Step:** `' + data.step_hex_id + '`  \n') : '') +
      '**Plan:** ' + (data.plan_id || 'unassigned') + '  \n' +
      '**Agent:** ' + (data.agent || 'n/a') + '  \n' +
      '**Model:** ' + (data.agent_model || 'n/a') + '  \n' +
      '**Build:** ' + (data.build_state || 'unknown') + '  \n' +
      '**Duration:** ' + (data.duration_secs || 0) + 's  \n\n' +
      '## Objective\n\n' + (data.objective || '_No objective yet._') + '\n\n' +
      '## What Was Built\n\n' + (data.what_was_built || '_Not recorded._') + '\n\n' +
      '## Key Decisions\n\n' + (data.key_decisions || '_No key decisions yet._') + '\n\n' +
      '## Challenges\n\n' + (data.challenges || '_No challenges yet._') + '\n\n' +
      '## Current State\n\n' + (data.current_state || '_No current state yet._') + '\n\n' +
      '## Known Issues\n\n' + (data.known_issues || '_No known issues yet._') + '\n\n' +
      '## Next Steps\n\n' + (data.next_steps || '_No next steps yet._')
    );
  }

  function beginEdit() {
    if (!turn) return;
    setEditObjective(turn.objective || '');
    setEditBuilt(turn.what_was_built || '');
    setEditKeyDecisions(turn.key_decisions || '');
    setEditChallenges(turn.challenges || '');
    setEditCurrentState(turn.current_state || '');
    setEditKnownIssues(turn.known_issues || '');
    setEditNextSteps(turn.next_steps || '');
    setEditBuildState(turn.build_state || 'unknown');
    setEditDuration(String(turn.duration_secs || 0));
    setIsEditing(true);
  }

  function cancelEdit() {
    if (!turn) return;
    setEditObjective(turn.objective || '');
    setEditBuilt(turn.what_was_built || '');
    setEditKeyDecisions(turn.key_decisions || '');
    setEditChallenges(turn.challenges || '');
    setEditCurrentState(turn.current_state || '');
    setEditKnownIssues(turn.known_issues || '');
    setEditNextSteps(turn.next_steps || '');
    setEditBuildState(turn.build_state || 'unknown');
    setEditDuration(String(turn.duration_secs || 0));
    setIsEditing(false);
  }

  async function saveTurn() {
    if (!turn) return;
    setSaving(true);

    var duration = Number(editDuration);
    if (!Number.isFinite(duration) || duration < 0) {
      duration = 0;
    }

    var payload = {
      objective: editObjective,
      what_was_built: editBuilt,
      key_decisions: editKeyDecisions,
      challenges: editChallenges,
      current_state: editCurrentState,
      known_issues: editKnownIssues,
      next_steps: editNextSteps,
      build_state: editBuildState,
      duration_secs: Math.round(duration),
    };

    try {
      var updated = await apiCall(base + '/turns/' + encodeURIComponent(String(turn.id)), 'PUT', payload);
      dispatch({
        type: 'SET',
        payload: {
          turns: state.turns.map(function (item) {
            return item.id === updated.id ? updated : item;
          }),
        },
      });
      setIsEditing(false);
      toast('Log entry updated', 'success');
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to save log: ' + (err.message || err), 'error');
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <div style={{ width: '100%', height: '100%', display: 'flex', flexDirection: 'column', animation: 'fadeIn 0.2s ease' }}>
      <SectionHeader
        count={1}
        action={(
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            {isEditing ? (
              <>
                <button
                  onClick={cancelEdit}
                  disabled={saving}
                  style={{
                    background: 'transparent', color: 'var(--text-3)', border: '1px solid var(--border)',
                    borderRadius: 4, padding: '4px 8px', fontFamily: "'JetBrains Mono', monospace",
                    fontSize: 10, cursor: saving ? 'not-allowed' : 'pointer',
                  }}
                >Cancel</button>
                <button
                  onClick={saveTurn}
                  disabled={saving}
                  style={{
                    background: saving ? 'var(--bg-3)' : 'var(--green)', color: '#000',
                    border: '1px solid transparent', borderRadius: 4, padding: '4px 8px',
                    fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                    fontWeight: 600, cursor: saving ? 'not-allowed' : 'pointer',
                  }}
                >{saving ? 'Saving...' : 'Save'}</button>
              </>
            ) : (
              <button
                onClick={beginEdit}
                style={{
                  background: 'transparent', color: 'var(--accent)', border: '1px solid var(--accent)40',
                  borderRadius: 4, padding: '4px 8px', fontFamily: "'JetBrains Mono', monospace",
                  fontSize: 10, cursor: 'pointer',
                }}
              >Edit</button>
            )}
          </div>
        )}
      >
        {'Turn #' + turn.id}
      </SectionHeader>

      <div style={{ flex: 1, overflow: 'auto', padding: '0 0 16px 0' }}>
        <div style={{ padding: '12px 16px' }}>
          {isEditing ? (
            <div style={{ display: 'grid', gap: 10 }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                <div>
                  <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Build State</label>
                  <select value={editBuildState} onChange={function (event) { setEditBuildState(event.target.value); }} style={selectStyle}>
                    {buildStateOptions.map(function (status) {
                      return <option key={status} value={status}>{status}</option>;
                    })}
                  </select>
                </div>
                <div>
                  <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Duration (s)</label>
                  <input
                    type="number"
                    min="0"
                    value={editDuration}
                    onChange={function (event) { setEditDuration(event.target.value); }}
                    style={inputStyle}
                  />
                </div>
              </div>

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Objective (Markdown)</label>
              <textarea
                value={editObjective}
                onChange={function (event) { setEditObjective(event.target.value); }}
                style={textareaStyle}
                rows={10}
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>What Was Built (Markdown)</label>
              <textarea
                value={editBuilt}
                onChange={function (event) { setEditBuilt(event.target.value); }}
                style={textareaStyle}
                rows={10}
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Key Decisions (Markdown)</label>
              <textarea
                value={editKeyDecisions}
                onChange={function (event) { setEditKeyDecisions(event.target.value); }}
                style={textareaStyle}
                rows={9}
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Challenges (Markdown)</label>
              <textarea
                value={editChallenges}
                onChange={function (event) { setEditChallenges(event.target.value); }}
                style={textareaStyle}
                rows={8}
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Current State (Markdown)</label>
              <textarea
                value={editCurrentState}
                onChange={function (event) { setEditCurrentState(event.target.value); }}
                style={textareaStyle}
                rows={8}
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Known Issues (Markdown)</label>
              <textarea
                value={editKnownIssues}
                onChange={function (event) { setEditKnownIssues(event.target.value); }}
                style={textareaStyle}
                rows={8}
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Next Steps (Markdown)</label>
              <textarea
                value={editNextSteps}
                onChange={function (event) { setEditNextSteps(event.target.value); }}
                style={textareaStyle}
                rows={10}
              />
            </div>
          ) : (
            <MarkdownContent text={buildMarkdown(turn)} style={markdownContentStyle} />
          )}
        </div>
      </div>
    </div>
  );
}

var inputStyle = {
  width: '100%',
  padding: '7px 10px',
  borderRadius: 6,
  border: '1px solid var(--border)',
  background: 'var(--bg-3)',
  color: 'var(--text-0)',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 12,
  outline: 'none',
};

var selectStyle = {
  width: '100%',
  padding: '7px 10px',
  borderRadius: 6,
  border: '1px solid var(--border)',
  background: 'var(--bg-3)',
  color: 'var(--text-0)',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 12,
  outline: 'none',
};

var textareaStyle = {
  width: '100%',
  padding: '10px',
  borderRadius: 6,
  border: '1px solid var(--border)',
  background: 'var(--bg-0)',
  color: 'var(--text-0)',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 12,
  lineHeight: 1.5,
  resize: 'vertical',
  minHeight: 140,
};

var markdownContentStyle = {
  width: '100%',
  margin: 0,
  padding: 0,
};
