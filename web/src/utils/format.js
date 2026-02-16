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
