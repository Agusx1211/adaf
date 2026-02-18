import { useState, useMemo } from 'react';
import { ansiToStyledRuns } from '../../utils/format.js';

/* ── Helpers ────────────────────────────────────────────────────── */

export function cleanResponse(text) {
  return text
    .split('\n')
    .filter(function (line) {
      if (line.indexOf('[stderr]') !== -1) return false;
      if (/^File changes:\s/i.test(line)) return false;
      return true;
    })
    .join('\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

function renderMarkdown(text) {
  if (!text) return '';
  if (typeof window.marked === 'undefined') {
    return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\n/g, '<br/>');
  }
  try {
    var renderer = new window.marked.Renderer();
    return window.marked.parse(text, {
      renderer: renderer,
      gfm: true,
      breaks: true,
      highlight: function (code, lang) {
        if (typeof window.hljs !== 'undefined' && lang && window.hljs.getLanguage(lang)) {
          try { return window.hljs.highlight(code, { language: lang }).value; } catch (_) {}
        }
        if (typeof window.hljs !== 'undefined') {
          try { return window.hljs.highlightAuto(code).value; } catch (_) {}
        }
        return '';
      },
    });
  } catch (_) {
    return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\n/g, '<br/>');
  }
}

function stringify(val) {
  if (val == null) return '';
  if (typeof val === 'string') return val;
  try { return JSON.stringify(val, null, 2); } catch (_) { return String(val); }
}

function formatToolInput(tool, input) {
  if (typeof input === 'object' && input) {
    var lc = (tool || '').toLowerCase();
    if ((lc === 'bash' || lc === 'run_shell_command' || lc === 'shell') && input.command) return input.command;
    if ((lc === 'read' || lc === 'write' || lc === 'edit' || lc === 'glob' || lc === 'grep' ||
         lc === 'read_file' || lc === 'write_file' || lc === 'edit_file') && input.file_path) return input.file_path;
  }
  return stringify(input);
}

function truncate(str, max) {
  if (!str) return '';
  return str.length > max ? str.slice(0, max) + '\u2026' : str;
}

var WAIT_FOR_SPAWNS_TEXT = '[system] waiting for spawns (parent turn suspended)';
var WAIT_FOR_SPAWNS_NORMALIZED = WAIT_FOR_SPAWNS_TEXT.toLowerCase().replace(/\s+/g, ' ').trim();

function normalizeWaitForSpawnsText(value) {
  return String(value || '').toLowerCase().replace(/\s+/g, ' ').trim();
}

function isWaitForSpawnsMessage(value) {
  var normalized = normalizeWaitForSpawnsText(value);
  if (!normalized) return false;
  if (normalized === WAIT_FOR_SPAWNS_NORMALIZED) return true;
  return normalized.indexOf('waiting for spawns') >= 0 && normalized.indexOf('parent turn suspended') >= 0;
}

function isShellTool(tool) {
  var lc = String(tool || '').toLowerCase();
  return lc === 'bash' || lc === 'shell' || lc === 'run_shell_command' || lc === 'run_shell';
}

function extractShellCommand(tool, input) {
  if (!isShellTool(tool)) return '';
  if (typeof input === 'string') return input;
  if (!input || typeof input !== 'object') return '';
  if (typeof input.command === 'string') return input.command;
  if (typeof input.cmd === 'string') return input.cmd;
  return '';
}

function trimQuotePair(text) {
  var out = String(text || '').trim();
  if (!out) return '';
  if ((out[0] === '"' && out[out.length - 1] === '"') || (out[0] === '\'' && out[out.length - 1] === '\'')) {
    return out.slice(1, -1).trim();
  }
  return out;
}

function normalizeLoopControlText(text) {
  var out = trimQuotePair(text);
  if (!out) return '';
  if (out.endsWith('\'') && !out.startsWith('\'')) out = out.slice(0, -1).trim();
  if (out.endsWith('"') && !out.startsWith('"')) out = out.slice(0, -1).trim();
  return trimQuotePair(out);
}

function detectLoopControl(tool, input) {
  var rawCommand = String(extractShellCommand(tool, input) || '').trim();
  if (!rawCommand) return null;

  var normalized = rawCommand.toLowerCase();
  var callIdx = normalized.indexOf('adaf loop call-supervisor');
  var stopIdx = normalized.indexOf('adaf loop stop');
  if (callIdx < 0 && stopIdx < 0) return null;

  var kind = '';
  var idx = 0;
  if (callIdx >= 0 && (stopIdx < 0 || callIdx <= stopIdx)) {
    kind = 'call_supervisor';
    idx = callIdx;
  } else {
    kind = 'stop_loop';
    idx = stopIdx;
  }

  var command = normalizeLoopControlText(rawCommand.slice(idx));
  var detail = command.replace(/^adaf\s+loop\s+(?:call-supervisor|stop)\b/i, '').trim();
  detail = normalizeLoopControlText(detail);
  return { kind: kind, command: command, detail: detail };
}

/* ── Components ─────────────────────────────────────────────────── */

export function MarkdownContent({ text, style }) {
  var html = useMemo(function () { return renderMarkdown(text); }, [text]);
  return <div className="md-content" style={style || {}} dangerouslySetInnerHTML={{ __html: html }} />;
}

export function ThinkingBlock({ content }) {
  var [expanded, setExpanded] = useState(false);
  return (
    <div
      style={{
        margin: '6px 0', padding: '6px 10px',
        background: 'var(--bg-2)', borderRadius: 4,
        borderLeft: '2px solid #9399b240', cursor: 'pointer',
      }}
      onClick={function () { setExpanded(!expanded); }}
    >
      <div style={{
        display: 'flex', alignItems: 'center', gap: 6,
        fontSize: 10, color: '#9399b2',
        fontFamily: "'JetBrains Mono', monospace", fontWeight: 600,
      }}>
        <span style={{ fontSize: 8 }}>{expanded ? '\u25BE' : '\u25B8'}</span>
        <span>THINKING</span>
      </div>
      {expanded ? (
        <div style={{
          marginTop: 6, fontSize: 12, color: 'var(--text-2)',
          lineHeight: 1.5, whiteSpace: 'pre-wrap', wordBreak: 'break-word',
        }}>{content}</div>
      ) : (
        <div style={{
          marginTop: 2, fontSize: 11, color: 'var(--text-3)',
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{truncate(content, 120)}</div>
      )}
    </div>
  );
}

export function PromptBlock({ content }) {
  var [expanded, setExpanded] = useState(false);
  var text = String(content || '');
  if (!text) return null;
  return (
    <div
      style={{
        margin: '6px 0', padding: '6px 10px',
        background: 'var(--accent)10', borderRadius: 4,
        borderLeft: '2px solid var(--accent)', cursor: 'pointer',
      }}
      onClick={function () { setExpanded(!expanded); }}
    >
      <div style={{
        display: 'flex', alignItems: 'center', gap: 6,
        fontSize: 10, color: 'var(--accent)',
        fontFamily: "'JetBrains Mono', monospace", fontWeight: 600,
      }}>
        <span style={{ fontSize: 8 }}>{expanded ? '\u25BE' : '\u25B8'}</span>
        <span>PROMPT</span>
      </div>
      {expanded ? (
        <div style={{
          marginTop: 6, fontSize: 11, color: 'var(--text-2)',
          lineHeight: 1.5, whiteSpace: 'pre-wrap', wordBreak: 'break-word',
        }}>{text}</div>
      ) : (
        <div style={{
          marginTop: 2, fontSize: 11, color: 'var(--text-3)',
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{truncate(text, 140)}</div>
      )}
    </div>
  );
}

export function ToolCallBlock({ tool, input }) {
  var [expanded, setExpanded] = useState(false);
  var displayInput = formatToolInput(tool, input);
  var hasDetails = displayInput.length > 80;
  return (
    <div
      style={{
        margin: '6px 0', padding: '6px 10px',
        background: 'var(--bg-2)', borderRadius: 4,
        borderLeft: '2px solid #f9e2af',
        cursor: hasDetails ? 'pointer' : 'default',
      }}
      onClick={function () { if (hasDetails) setExpanded(!expanded); }}
    >
      <div style={{
        display: 'flex', alignItems: 'center', gap: 6,
        fontSize: 10, fontFamily: "'JetBrains Mono', monospace",
      }}>
        <span style={{ color: '#f9e2af' }}>{'\u2699'}</span>
        <span style={{ color: '#f9e2af', fontWeight: 600 }}>{tool}</span>
        {!hasDetails && displayInput && (
          <span style={{ color: 'var(--text-2)', fontSize: 11, marginLeft: 4 }}>
            {truncate(displayInput, 80)}
          </span>
        )}
        {hasDetails && (
          <span style={{ color: 'var(--text-3)', fontSize: 8 }}>{expanded ? '\u25BE' : '\u25B8'}</span>
        )}
      </div>
      {hasDetails && (
        <pre style={{
          marginTop: expanded ? 6 : 2, fontSize: 11,
          color: 'var(--text-2)', whiteSpace: 'pre-wrap', wordBreak: 'break-word',
          overflow: 'hidden', maxHeight: expanded ? 300 : 20, lineHeight: 1.4,
          background: 'transparent', padding: 0, border: 'none',
        }}>
          {displayInput}
        </pre>
      )}
    </div>
  );
}

export function ToolResultBlock({ tool, result, isError }) {
  var [expanded, setExpanded] = useState(false);
  var resultStr = stringify(result);
  var hasDetails = resultStr.length > 120;
  var color = isError ? 'var(--red)' : '#a6e3a1';
  var displayText = expanded ? resultStr : truncate(resultStr, 200);
  var styledRuns = useMemo(function () {
    return ansiToStyledRuns(displayText);
  }, [displayText]);
  return (
    <div
      style={{
        margin: '6px 0', padding: '6px 10px',
        background: 'var(--bg-2)', borderRadius: 4,
        borderLeft: '2px solid ' + color,
        cursor: hasDetails ? 'pointer' : 'default',
      }}
      onClick={function () { if (hasDetails) setExpanded(!expanded); }}
    >
      <div style={{
        display: 'flex', alignItems: 'center', gap: 6,
        fontSize: 10, fontFamily: "'JetBrains Mono', monospace",
      }}>
        <span style={{ color: color }}>{isError ? '\u2717' : '\u2713'}</span>
        <span style={{ color: color, fontWeight: 600 }}>{tool || 'RESULT'}</span>
        {hasDetails && (
          <span style={{ color: 'var(--text-3)', fontSize: 8 }}>{expanded ? '\u25BE' : '\u25B8'}</span>
        )}
      </div>
      <div style={{
        marginTop: 4, fontSize: 11, color: 'var(--text-2)',
        whiteSpace: 'pre-wrap', wordBreak: 'break-word',
        maxHeight: expanded ? 'none' : 60, overflow: 'hidden', lineHeight: 1.4,
      }}>
        {styledRuns.map(function (run, idx) {
          return <span key={idx} style={run.style}>{run.text}</span>;
        })}
      </div>
    </div>
  );
}

export function FileChangeBadges({ changes }) {
  return (
    <div style={{ margin: '6px 0', display: 'flex', flexWrap: 'wrap', gap: 4 }}>
      {changes.map(function (fc, i) {
        var opColor = fc.operation === 'delete' ? 'var(--red)' : fc.operation === 'create' ? 'var(--green)' : 'var(--accent)';
        var opIcon = fc.operation === 'delete' ? '\u2717' : fc.operation === 'create' ? '+' : '~';
        var shortPath = fc.path.split('/').pop();
        return (
          <span key={i} title={fc.path} style={{
            display: 'inline-flex', alignItems: 'center', gap: 4,
            padding: '2px 8px', borderRadius: 3,
            background: opColor + '15', border: '1px solid ' + opColor + '30',
            fontSize: 10, color: opColor,
            fontFamily: "'JetBrains Mono', monospace",
          }}>
            <span>{opIcon}</span>
            <span>{shortPath}</span>
          </span>
        );
      })}
    </div>
  );
}

export function WaitForSpawnsBlock({ content }) {
  var detail = String(content || WAIT_FOR_SPAWNS_TEXT).trim();
  return (
    <div style={{
      margin: '6px 0',
      padding: '8px 10px',
      background: 'rgba(249, 226, 175, 0.08)',
      borderRadius: 4,
      borderLeft: '2px solid rgb(249, 226, 175)',
      border: '1px solid rgba(249, 226, 175, 0.22)',
    }}>
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: 6,
        fontSize: 10,
        fontFamily: "'JetBrains Mono', monospace",
      }}>
        <span style={{ color: 'rgb(249, 226, 175)' }}>{'\u29D6'}</span>
        <span style={{ color: 'rgb(249, 226, 175)', fontWeight: 700, letterSpacing: '0.04em' }}>WAITING FOR SPAWNS</span>
      </div>
      <div style={{
        marginTop: 4,
        fontSize: 11,
        color: 'var(--text-2)',
        lineHeight: 1.4,
      }}>
        Parent turn is paused until spawned agents return results.
      </div>
      <div style={{
        marginTop: 4,
        fontSize: 10,
        color: 'var(--text-3)',
        fontFamily: "'JetBrains Mono', monospace",
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
      }}>
        {detail}
      </div>
    </div>
  );
}

export function LoopControlBlock({ control }) {
  var c = control && typeof control === 'object' ? control : {};
  var isStop = c.kind === 'stop_loop';
  var accent = isStop ? 'rgb(243, 139, 168)' : 'rgb(249, 226, 175)';
  var title = isStop ? 'STOP LOOP' : 'CALL SUPERVISOR';
  var subtitle = isStop
    ? 'Supervisor issued a loop stop signal for this run.'
    : 'Manager escalated this turn to the supervisor.';

  return (
    <div style={{
      margin: '6px 0',
      padding: '8px 10px',
      background: accent + '14',
      borderRadius: 4,
      borderLeft: '2px solid ' + accent,
      border: '1px solid ' + accent + '38',
    }}>
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: 6,
        fontSize: 10,
        fontFamily: "'JetBrains Mono', monospace",
      }}>
        <span style={{ color: accent }}>{isStop ? '\u25A0' : '\u21A5'}</span>
        <span style={{ color: accent, fontWeight: 700, letterSpacing: '0.04em' }}>{title}</span>
      </div>
      <div style={{
        marginTop: 4,
        fontSize: 11,
        color: 'var(--text-2)',
        lineHeight: 1.4,
      }}>
        {subtitle}
      </div>
      {c.detail ? (
        <div style={{
          marginTop: 4,
          fontSize: 10,
          color: 'var(--text-1)',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
        }}>
          {c.detail}
        </div>
      ) : null}
      {c.command ? (
        <div style={{
          marginTop: 4,
          fontSize: 10,
          color: 'var(--text-3)',
          fontFamily: "'JetBrains Mono', monospace",
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
        }}>
          {c.command}
        </div>
      ) : null}
    </div>
  );
}

/* ── EventBlockList: renders an array of typed events ──────────── */

export function EventBlockList({ events }) {
  if (!events || !events.length) return null;

  // Group events for rendering:
  // - Merge consecutive text into one markdown block
  // - Detect "File changes:" patterns in text
  // - Keep thinking, tool_use, tool_result as separate items
  var groups = [];
  var currentText = '';
  var currentFileChanges = [];

  function flushText() {
    var trimmed = currentText.replace(/\n+$/, '');
    if (trimmed) groups.push({ type: 'text', content: trimmed });
    currentText = '';
  }
  function flushFileChanges() {
    if (currentFileChanges.length) {
      groups.push({ type: 'file_changes', changes: currentFileChanges.slice() });
      currentFileChanges = [];
    }
  }

  events.forEach(function (evt) {
    if (evt.type === 'wait_for_spawns') {
      flushText();
      flushFileChanges();
      groups.push({ type: 'wait_for_spawns', content: evt.content || evt.text || WAIT_FOR_SPAWNS_TEXT });
      return;
    }
    if (evt.type === 'tool_use') {
      var loopControl = detectLoopControl(evt.tool, evt.input);
      if (loopControl) {
        flushText();
        flushFileChanges();
        groups.push({ type: 'loop_control', control: loopControl });
        return;
      }
    }
    if (evt.type === 'text') {
      var text = evt.content || evt.text || '';
      if (isWaitForSpawnsMessage(text)) {
        flushText();
        flushFileChanges();
        groups.push({ type: 'wait_for_spawns', content: text });
        return;
      }
      var lines = text.split('\n');
      for (var li = 0; li < lines.length; li++) {
        var line = lines[li];
        var m = line.match(/^File changes:\s*(create|update|delete|rename)\s+(.+)/i);
        if (m) {
          flushText();
          currentFileChanges.push({ operation: m[1].toLowerCase(), path: m[2].trim() });
        } else {
          flushFileChanges();
          currentText += line;
          if (li < lines.length - 1) currentText += '\n';
        }
      }
    } else {
      flushText();
      flushFileChanges();
      groups.push(evt);
    }
  });
  flushText();
  flushFileChanges();

  return (
    <div>
      {groups.map(function (g, i) {
        if (g.type === 'text') return <MarkdownContent key={i} text={g.content} />;
        if (g.type === 'initial_prompt') return <PromptBlock key={i} content={g.content} />;
        if (g.type === 'thinking') return <ThinkingBlock key={i} content={g.content} />;
        if (g.type === 'tool_use') return <ToolCallBlock key={i} tool={g.tool} input={g.input} />;
        if (g.type === 'tool_result') return <ToolResultBlock key={i} tool={g.tool} result={g.result} isError={g.isError} />;
        if (g.type === 'file_changes') return <FileChangeBadges key={i} changes={g.changes} />;
        if (g.type === 'wait_for_spawns') return <WaitForSpawnsBlock key={i} content={g.content} />;
        if (g.type === 'loop_control') return <LoopControlBlock key={i} control={g.control} />;
        return null;
      })}
    </div>
  );
}

/* ── Convert global state events to block events ───────────────── */

export function stateEventsToBlocks(stateEvents) {
  if (!stateEvents || !stateEvents.length) return [];
  return stateEvents.map(function (evt) {
    var eventTurnID = Number(evt && (evt.turn_id || evt.turnID || evt._turnID) || 0) || 0;
    var meta = {
      _sourceLabel: evt._sourceLabel || '',
      _sourceColor: evt._sourceColor || '',
      _spawnID: evt._spawnID || 0,
      _scope: evt.scope || evt._scope || '',
      _ts: Number(evt && evt.ts) || 0,
      _turnID: eventTurnID,
    };
    if (evt.type === 'initial_prompt') {
      return Object.assign({
        type: 'initial_prompt',
        content: evt.text || evt.content || '',
        _turnHexID: evt.turn_hex_id || evt.turnHexID || '',
        _turnID: eventTurnID,
        _isResume: !!(evt.is_resume || evt.isResume),
      }, meta);
    }
    if (evt.type === 'tool_use') {
      var parsedInput = evt.input;
      if (typeof parsedInput === 'string' && parsedInput) {
        try { parsedInput = JSON.parse(parsedInput); } catch (_) {}
      }
      return Object.assign({ type: 'tool_use', tool: evt.tool || 'tool', input: parsedInput || '' }, meta);
    }
    if (evt.type === 'tool_result') {
      return Object.assign({ type: 'tool_result', tool: evt.tool || '', result: evt.result || evt.text || evt.content || '', isError: !!evt.isError }, meta);
    }
    if (evt.type === 'thinking') {
      return Object.assign({ type: 'thinking', content: evt.text || evt.content || '' }, meta);
    }
    var fallbackContent = evt.text || evt.content || '';
    if ((evt.type === 'text' || !evt.type) && isWaitForSpawnsMessage(fallbackContent)) {
      return Object.assign({ type: 'wait_for_spawns', content: fallbackContent }, meta);
    }
    return Object.assign({ type: evt.type || 'text', content: fallbackContent }, meta);
  });
}

/* ── Styles ──────────────────────────────────────────────────────── */

var stylesInjected = false;
export function injectEventBlockStyles() {
  if (stylesInjected) return;
  stylesInjected = true;
  var style = document.createElement('style');
  style.textContent = [
    '@keyframes slideIn { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: translateY(0); } }',
    '@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }',
    '.md-content { margin: 0; padding: 0; font-size: 13px; line-height: 1.7; color: var(--text-0); word-break: break-word; }',
    '.md-content p { margin: 0 0 8px 0; }',
    '.md-content p:last-child { margin-bottom: 0; }',
    '.md-content h1, .md-content h2, .md-content h3, .md-content h4 { margin: 16px 0 8px 0; color: var(--text-0); }',
    '.md-content h1 { font-size: 18px; }',
    '.md-content h2 { font-size: 15px; }',
    '.md-content h3 { font-size: 14px; }',
    '.md-content h4 { font-size: 13px; }',
    '.md-content ul, .md-content ol { margin: 4px 0 8px 0; padding-left: 20px; }',
    '.md-content li { margin-bottom: 4px; }',
    '.md-content code { font-family: "JetBrains Mono", monospace; font-size: 12px; background: var(--bg-3); padding: 1px 5px; border-radius: 3px; }',
    '.md-content pre { margin: 8px 0; padding: 12px; background: var(--bg-3); border-radius: 6px; overflow-x: auto; border: 1px solid var(--border); }',
    '.md-content pre code { background: none; padding: 0; font-size: 12px; }',
    '.md-content blockquote { margin: 8px 0; padding: 4px 12px; border-left: 3px solid var(--accent); color: var(--text-2); background: var(--bg-2); border-radius: 0 4px 4px 0; }',
    '.md-content a { color: var(--accent); text-decoration: none; }',
    '.md-content a:hover { text-decoration: underline; }',
    '.md-content table { border-collapse: collapse; margin: 8px 0; width: 100%; }',
    '.md-content th, .md-content td { border: 1px solid var(--border); padding: 6px 10px; text-align: left; font-size: 12px; }',
    '.md-content th { background: var(--bg-3); font-weight: 600; }',
    '.md-content hr { border: none; border-top: 1px solid var(--border); margin: 12px 0; }',
    '.md-content strong { color: var(--text-0); font-weight: 600; }',
    '.md-content img { max-width: 100%; border-radius: 4px; }',
  ].join('\n');
  document.head.appendChild(style);
}
