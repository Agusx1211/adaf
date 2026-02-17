export var AGENT_TYPES = {
  claude: { color: '#E8A838', icon: '\u26A1', bg: 'rgba(232,168,56,0.08)' },
  codex: { color: '#4AE68A', icon: '\u25C6', bg: 'rgba(74,230,138,0.08)' },
  gemini: { color: '#7B8CFF', icon: '\u2726', bg: 'rgba(123,140,255,0.08)' },
  vibe: { color: '#FF6B9D', icon: '\u25C8', bg: 'rgba(255,107,157,0.08)' },
  opencode: { color: '#5BCEFC', icon: '\u25C9', bg: 'rgba(91,206,252,0.08)' },
  generic: { color: '#9CA3AF', icon: '\u25CB', bg: 'rgba(156,163,175,0.08)' },
};

export var STATUSES = {
  running: '#4AE68A',
  completed: '#7B8CFF',
  failed: '#FF4B4B',
  waiting: '#E8A838',
  waiting_for_spawns: '#E8A838',
  spawning: '#FF6B9D',
};

export var STATUS_RUNNING = {
  starting: true,
  running: true,
  active: true,
  in_progress: true,
};

export var SCOPE_COLOR_PALETTE = [
  '#89b4fa', '#94e2d5', '#cba6f7', '#fab387', '#a6e3a1',
  '#b4befe', '#89dceb', '#f2cdcd', '#74c7ec',
];

export function statusColor(status) {
  var key = String(status || 'unknown').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_');
  var map = {
    running: '#f9e2af', starting: '#f9e2af',
    waiting: '#f9e2af', waiting_for_spawns: '#f9e2af',
    awaiting_input: '#89b4fa',
    completed: '#a6e3a1', complete: '#a6e3a1', merged: '#a6e3a1',
    passing: '#a6e3a1', resolved: '#a6e3a1', done: '#a6e3a1',
    failed: '#f38ba8', failing: '#f38ba8',
    canceled: '#f38ba8', cancelled: '#f38ba8',
    rejected: '#f38ba8', blocked: '#f38ba8',
    stopped: '#6c7086', not_started: '#6c7086',
    open: '#f9e2af', in_progress: '#f9e2af',
    critical: '#f38ba8', high: '#fab387', medium: '#f9e2af', low: '#6c7086',
    active: '#89b4fa',
  };
  return map[key] || '#a6adc8';
}

export function statusIcon(status) {
  var key = String(status || 'unknown').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_');
  var map = {
    running: '\u25C9', starting: '\u25C9',
    waiting: '\u25CE', waiting_for_spawns: '\u25CE',
    awaiting_input: '\u25CE',
    completed: '\u2713', complete: '\u2713', merged: '\u2295',
    passing: '\u2713', failed: '\u2717', failing: '\u2717',
    canceled: '\u2298', cancelled: '\u2298', rejected: '\u2297',
    blocked: '\u2717', stopped: '\u25A0', resolved: '\u2713',
    in_progress: '\u25C9', open: '\u25C9',
  };
  return map[key] || '\u25CB';
}

export function scopeColor(scope) {
  var key = String(scope || 'session-0');
  var mainParsed = key.match(/^session-main-(\d+)$/);
  if (mainParsed) {
    var mainIdx = parseInt(mainParsed[1], 10);
    if (!Number.isNaN(mainIdx)) return SCOPE_COLOR_PALETTE[mainIdx % SCOPE_COLOR_PALETTE.length];
    return '#7f849c';
  }
  var parsed = key.match(/(session|spawn)-(\d+)/);
  if (!parsed) return '#7f849c';
  var idx = parseInt(parsed[2], 10);
  if (Number.isNaN(idx)) return '#7f849c';
  return SCOPE_COLOR_PALETTE[idx % SCOPE_COLOR_PALETTE.length];
}

export function scopeShortLabel(scope) {
  var value = String(scope || 'session-0');
  if (value.indexOf('session-main-') === 0) return 's' + value.slice(13);
  if (value.indexOf('session-') === 0) return 's' + value.slice(8);
  if (value.indexOf('spawn-') === 0) return 'sp' + value.slice(6);
  return value;
}

export function agentInfo(agent) {
  return AGENT_TYPES[agent] || AGENT_TYPES.generic;
}
