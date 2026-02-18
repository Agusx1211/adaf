export function parseTimestamp(value) {
  if (value == null || value === '') return 0;
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  var ts = new Date(value).getTime();
  if (!Number.isFinite(ts)) return 0;
  return ts;
}

export function formatDuration(ms) {
  var s = Math.floor(ms / 1000);
  var m = Math.floor(s / 60);
  var h = Math.floor(m / 60);
  if (h > 0) return h + 'h ' + (m % 60) + 'm';
  if (m > 0) return m + 'm ' + (s % 60) + 's';
  return s + 's';
}

export function formatElapsed(start, end) {
  var startMS = parseTimestamp(start);
  if (!startMS) return '--';
  var endMS = end ? parseTimestamp(end) : Date.now();
  if (!endMS || endMS < startMS) endMS = Date.now();
  return formatDuration(endMS - startMS);
}

export function formatTime(ts) {
  var d = new Date(ts);
  return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
}

export function timeAgo(ts) {
  var parsed = parseTimestamp(ts);
  if (!parsed) return '';
  var diff = Math.max(0, Date.now() - parsed);
  var sec = Math.floor(diff / 1000);
  if (sec < 60) return 'just now';
  var min = Math.floor(sec / 60);
  if (min < 60) return min + 'm ago';
  var hour = Math.floor(min / 60);
  if (hour < 24) return hour + 'h ago';
  var day = Math.floor(hour / 24);
  return day + 'd ago';
}

export function formatNumber(value) {
  var num = Number(value || 0);
  if (!Number.isFinite(num)) return '0';
  return num.toLocaleString();
}

export function cropText(input, limit) {
  var max = Number(limit || 120000);
  var text = String(input || '');
  if (text.length <= max) return text;
  return text.slice(0, max - 1) + '\u2026';
}

export function normalizeStatus(value) {
  return String(value || 'unknown').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_');
}

export function arrayOrEmpty(value) {
  return Array.isArray(value) ? value : [];
}

export function numberOr(current) {
  for (var i = 1; i < arguments.length; i++) {
    var next = Number(arguments[i]);
    if (Number.isFinite(next)) return next;
  }
  return Number(current) || 0;
}

export function safeJSONString(value) {
  if (value == null) return '';
  if (typeof value === 'string') return value;
  try { return JSON.stringify(value); } catch (_) { return String(value); }
}

export function stringifyToolPayload(value) {
  if (value == null) return '';
  if (typeof value === 'string') return value;
  try { return JSON.stringify(value); } catch (_) { return String(value); }
}

var ANSI_SGR_PATTERN = /\u001b\[([0-9;]*)m/g;
var ANSI_16_COLOR_MAP = {
  30: 'var(--text-3)',
  31: 'var(--red)',
  32: 'var(--green)',
  33: 'var(--accent)',
  34: 'var(--blue)',
  35: 'var(--purple)',
  36: 'var(--blue)',
  37: 'var(--text-0)',
  90: 'var(--text-3)',
  91: 'var(--red)',
  92: 'var(--green)',
  93: 'var(--accent)',
  94: 'var(--blue)',
  95: 'var(--purple)',
  96: 'var(--blue)',
  97: 'var(--text-0)',
};

var ANSI_16_BG_COLOR_MAP = {
  40: 'rgba(92, 98, 112, 0.30)',
  41: 'rgba(255, 75, 75, 0.28)',
  42: 'rgba(74, 230, 138, 0.28)',
  43: 'rgba(232, 168, 56, 0.28)',
  44: 'rgba(91, 206, 252, 0.28)',
  45: 'rgba(123, 140, 255, 0.28)',
  46: 'rgba(91, 206, 252, 0.28)',
  47: 'rgba(244, 245, 247, 0.20)',
  100: 'rgba(92, 98, 112, 0.36)',
  101: 'rgba(255, 75, 75, 0.36)',
  102: 'rgba(74, 230, 138, 0.36)',
  103: 'rgba(232, 168, 56, 0.36)',
  104: 'rgba(91, 206, 252, 0.36)',
  105: 'rgba(123, 140, 255, 0.36)',
  106: 'rgba(91, 206, 252, 0.36)',
  107: 'rgba(244, 245, 247, 0.28)',
};

function newAnsiState() {
  return {
    fg: '',
    bg: '',
    bold: false,
    dim: false,
    italic: false,
    underline: false,
    strike: false,
  };
}

function resetAnsiState(state) {
  state.fg = '';
  state.bg = '';
  state.bold = false;
  state.dim = false;
  state.italic = false;
  state.underline = false;
  state.strike = false;
}

function ansiStateKey(state) {
  return [
    state.fg || '',
    state.bg || '',
    state.bold ? '1' : '0',
    state.dim ? '1' : '0',
    state.italic ? '1' : '0',
    state.underline ? '1' : '0',
    state.strike ? '1' : '0',
  ].join('|');
}

function ansiStateToStyle(state) {
  var style = {};
  if (state.fg) style.color = state.fg;
  if (state.bg) style.backgroundColor = state.bg;
  if (state.bold) style.fontWeight = 700;
  if (state.dim) style.opacity = 0.8;
  if (state.italic) style.fontStyle = 'italic';
  if (state.underline || state.strike) {
    var lines = [];
    if (state.underline) lines.push('underline');
    if (state.strike) lines.push('line-through');
    style.textDecorationLine = lines.join(' ');
  }
  return style;
}

function ansi256ToColor(index) {
  var n = Number(index);
  if (!Number.isFinite(n)) return '';
  if (n < 0) n = 0;
  if (n > 255) n = 255;

  var preset = {
    0: '#000000',
    1: '#800000',
    2: '#008000',
    3: '#808000',
    4: '#000080',
    5: '#800080',
    6: '#008080',
    7: '#c0c0c0',
    8: '#808080',
    9: '#ff0000',
    10: '#00ff00',
    11: '#ffff00',
    12: '#0000ff',
    13: '#ff00ff',
    14: '#00ffff',
    15: '#ffffff',
  };
  if (Object.prototype.hasOwnProperty.call(preset, n)) return preset[n];

  if (n >= 16 && n <= 231) {
    var k = n - 16;
    var rIndex = Math.floor(k / 36);
    var gIndex = Math.floor((k % 36) / 6);
    var bIndex = k % 6;
    var levels = [0, 95, 135, 175, 215, 255];
    return 'rgb(' + levels[rIndex] + ', ' + levels[gIndex] + ', ' + levels[bIndex] + ')';
  }

  var gray = 8 + (n - 232) * 10;
  return 'rgb(' + gray + ', ' + gray + ', ' + gray + ')';
}

function applyAnsiColorCode(state, codes, i, isBackground) {
  var mode = Number(codes[i + 1]);
  if (!Number.isFinite(mode)) return i;

  if (mode === 5) {
    var paletteIndex = Number(codes[i + 2]);
    var color = ansi256ToColor(paletteIndex);
    if (color) {
      if (isBackground) state.bg = color;
      else state.fg = color;
    }
    return i + 2;
  }

  if (mode === 2) {
    var r = Number(codes[i + 2]);
    var g = Number(codes[i + 3]);
    var b = Number(codes[i + 4]);
    if (Number.isFinite(r) && Number.isFinite(g) && Number.isFinite(b)) {
      if (r < 0) r = 0;
      if (r > 255) r = 255;
      if (g < 0) g = 0;
      if (g > 255) g = 255;
      if (b < 0) b = 0;
      if (b > 255) b = 255;
      var rgb = 'rgb(' + r + ', ' + g + ', ' + b + ')';
      if (isBackground) state.bg = rgb;
      else state.fg = rgb;
    }
    return i + 4;
  }

  return i;
}

function applySgrCodes(state, rawCodes) {
  var codes = String(rawCodes || '').split(';').map(function (part) {
    if (part === '') return 0;
    var n = Number(part);
    if (!Number.isFinite(n)) return 0;
    return n;
  });
  if (!codes.length) codes = [0];

  for (var i = 0; i < codes.length; i++) {
    var code = codes[i];
    if (code === 0) {
      resetAnsiState(state);
      continue;
    }
    if (code === 1) {
      state.bold = true;
      continue;
    }
    if (code === 2) {
      state.dim = true;
      continue;
    }
    if (code === 3) {
      state.italic = true;
      continue;
    }
    if (code === 4) {
      state.underline = true;
      continue;
    }
    if (code === 9) {
      state.strike = true;
      continue;
    }
    if (code === 22) {
      state.bold = false;
      state.dim = false;
      continue;
    }
    if (code === 23) {
      state.italic = false;
      continue;
    }
    if (code === 24) {
      state.underline = false;
      continue;
    }
    if (code === 29) {
      state.strike = false;
      continue;
    }
    if (code === 39) {
      state.fg = '';
      continue;
    }
    if (code === 49) {
      state.bg = '';
      continue;
    }
    if (Object.prototype.hasOwnProperty.call(ANSI_16_COLOR_MAP, code)) {
      state.fg = ANSI_16_COLOR_MAP[code];
      continue;
    }
    if (Object.prototype.hasOwnProperty.call(ANSI_16_BG_COLOR_MAP, code)) {
      state.bg = ANSI_16_BG_COLOR_MAP[code];
      continue;
    }
    if (code === 38) {
      i = applyAnsiColorCode(state, codes, i, false);
      continue;
    }
    if (code === 48) {
      i = applyAnsiColorCode(state, codes, i, true);
      continue;
    }
  }
}

export function stripAnsi(value) {
  return String(value || '').replace(ANSI_SGR_PATTERN, '');
}

export function ansiToStyledRuns(value) {
  var input = String(value || '');
  if (!input) return [];

  var runs = [];
  var state = newAnsiState();
  var offset = 0;
  var match;

  function pushText(text) {
    if (!text) return;
    var styleKey = ansiStateKey(state);
    var last = runs.length > 0 ? runs[runs.length - 1] : null;
    if (last && last._styleKey === styleKey) {
      last.text += text;
      return;
    }
    runs.push({
      text: text,
      style: ansiStateToStyle(state),
      _styleKey: styleKey,
    });
  }

  ANSI_SGR_PATTERN.lastIndex = 0;
  while ((match = ANSI_SGR_PATTERN.exec(input)) !== null) {
    if (match.index > offset) {
      pushText(input.slice(offset, match.index));
    }
    applySgrCodes(state, match[1]);
    offset = ANSI_SGR_PATTERN.lastIndex;
  }
  if (offset < input.length) {
    pushText(input.slice(offset));
  }

  return runs.map(function (run) {
    return { text: run.text, style: run.style };
  });
}

export function withAlpha(color, alpha) {
  var value = String(color || '').trim();
  var a = Number(alpha);
  if (!Number.isFinite(a)) a = 1;
  if (a < 0) a = 0;
  if (a > 1) a = 1;
  if (/^#([a-fA-F0-9]{6})$/.test(value)) {
    var hex = value.slice(1);
    var r = parseInt(hex.slice(0, 2), 16);
    var g = parseInt(hex.slice(2, 4), 16);
    var b = parseInt(hex.slice(4, 6), 16);
    return 'rgba(' + r + ', ' + g + ', ' + b + ', ' + a + ')';
  }
  return value;
}
