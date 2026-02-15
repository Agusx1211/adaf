import { useState, useMemo } from 'react';

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

/* ── Components ─────────────────────────────────────────────────── */

export function MarkdownContent({ text, style }) {
  var html = useMemo(function () { return renderMarkdown(text); }, [text]);
  return <div className="pm-md-content" style={style || {}} dangerouslySetInnerHTML={{ __html: html }} />;
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
        {expanded ? resultStr : truncate(resultStr, 200)}
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
    if (evt.type === 'text') {
      var lines = (evt.content || '').split('\n');
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
        if (g.type === 'thinking') return <ThinkingBlock key={i} content={g.content} />;
        if (g.type === 'tool_use') return <ToolCallBlock key={i} tool={g.tool} input={g.input} />;
        if (g.type === 'tool_result') return <ToolResultBlock key={i} tool={g.tool} result={g.result} isError={g.isError} />;
        if (g.type === 'file_changes') return <FileChangeBadges key={i} changes={g.changes} />;
        return null;
      })}
    </div>
  );
}

/* ── Convert global state events to block events ───────────────── */

export function stateEventsToBlocks(stateEvents) {
  if (!stateEvents || !stateEvents.length) return [];
  return stateEvents.map(function (evt) {
    if (evt.type === 'tool_use') {
      var parsedInput = evt.input;
      if (typeof parsedInput === 'string' && parsedInput) {
        try { parsedInput = JSON.parse(parsedInput); } catch (_) {}
      }
      return { type: 'tool_use', tool: evt.tool || 'tool', input: parsedInput || '' };
    }
    if (evt.type === 'tool_result') {
      return { type: 'tool_result', tool: evt.tool || '', result: evt.result || evt.text || '', isError: false };
    }
    if (evt.type === 'thinking') {
      return { type: 'thinking', content: evt.text || '' };
    }
    return { type: 'text', content: evt.text || '' };
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
    '.pm-md-content { margin: 0; padding: 0; font-size: 13px; line-height: 1.7; color: var(--text-0); word-break: break-word; }',
    '.pm-md-content p { margin: 0 0 8px 0; }',
    '.pm-md-content p:last-child { margin-bottom: 0; }',
    '.pm-md-content h1, .pm-md-content h2, .pm-md-content h3, .pm-md-content h4 { margin: 16px 0 8px 0; color: var(--text-0); }',
    '.pm-md-content h1 { font-size: 18px; }',
    '.pm-md-content h2 { font-size: 15px; }',
    '.pm-md-content h3 { font-size: 14px; }',
    '.pm-md-content h4 { font-size: 13px; }',
    '.pm-md-content ul, .pm-md-content ol { margin: 4px 0 8px 0; padding-left: 20px; }',
    '.pm-md-content li { margin-bottom: 4px; }',
    '.pm-md-content code { font-family: "JetBrains Mono", monospace; font-size: 12px; background: var(--bg-3); padding: 1px 5px; border-radius: 3px; }',
    '.pm-md-content pre { margin: 8px 0; padding: 12px; background: var(--bg-3); border-radius: 6px; overflow-x: auto; border: 1px solid var(--border); }',
    '.pm-md-content pre code { background: none; padding: 0; font-size: 12px; }',
    '.pm-md-content blockquote { margin: 8px 0; padding: 4px 12px; border-left: 3px solid var(--accent); color: var(--text-2); background: var(--bg-2); border-radius: 0 4px 4px 0; }',
    '.pm-md-content a { color: var(--accent); text-decoration: none; }',
    '.pm-md-content a:hover { text-decoration: underline; }',
    '.pm-md-content table { border-collapse: collapse; margin: 8px 0; width: 100%; }',
    '.pm-md-content th, .pm-md-content td { border: 1px solid var(--border); padding: 6px 10px; text-align: left; font-size: 12px; }',
    '.pm-md-content th { background: var(--bg-3); font-weight: 600; }',
    '.pm-md-content hr { border: none; border-top: 1px solid var(--border); margin: 12px 0; }',
    '.pm-md-content strong { color: var(--text-0); font-weight: 600; }',
    '.pm-md-content img { max-width: 100%; border-radius: 4px; }',
  ].join('\n');
  document.head.appendChild(style);
}
