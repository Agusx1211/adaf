import { apiCall, apiBase } from './client.js';
import { cropText, safeJSONString } from '../utils/format.js';

var seenKeys = Object.create(null);
var seenOrder = [];
var maxSeenKeys = 800;
var maxText = 12000;

function normalizeString(value, maxLen) {
  var text = String(value || '').trim();
  if (!text) return '';
  var max = Number(maxLen || 256);
  if (text.length <= max) return text;
  return text.slice(0, max - 3) + '...';
}

function normalizePayload(payload) {
  if (payload == null) return null;
  if (typeof payload === 'string') return cropText(payload, maxText);

  var raw = safeJSONString(payload);
  if (raw.length <= maxText) return payload;
  return {
    truncated: true,
    original_length: raw.length,
    preview_json: raw.slice(0, maxText - 3) + '...',
  };
}

function sampleKey(projectID, sample) {
  return [
    String(projectID || ''),
    sample.source || '',
    sample.reason || '',
    sample.scope || '',
    String(sample.session_id || ''),
    String(sample.spawn_id || ''),
    sample.event_type || '',
    sample.agent || '',
    sample.model || '',
    normalizeString(sample.fallback_text || '', 120),
    normalizeString(safeJSONString(sample.payload || ''), 200),
  ].join('|');
}

function rememberKey(key) {
  if (!key || seenKeys[key]) return;
  seenKeys[key] = true;
  seenOrder.push(key);
  if (seenOrder.length <= maxSeenKeys) return;
  var oldest = seenOrder.shift();
  if (oldest) delete seenKeys[oldest];
}

export function reportMissingUISample(projectID, sample) {
  if (!sample || typeof sample !== 'object') return;

  var payload = {
    source: normalizeString(sample.source, 120),
    reason: normalizeString(sample.reason, 120),
    scope: normalizeString(sample.scope, 256),
    session_id: Number(sample.session_id) || 0,
    turn_id: Number(sample.turn_id) || 0,
    spawn_id: Number(sample.spawn_id) || 0,
    event_type: normalizeString(sample.event_type, 120),
    agent: normalizeString(sample.agent, 120),
    model: normalizeString(sample.model, 180),
    provider: normalizeString(sample.provider, 120),
    fallback_text: normalizeString(sample.fallback_text, 2000),
    payload: normalizePayload(sample.payload),
  };

  if (!payload.source || !payload.reason) return;

  var key = sampleKey(projectID, payload);
  if (seenKeys[key]) return;
  rememberKey(key);

  apiCall(apiBase(projectID) + '/ui/missing-samples', 'POST', payload).catch(function () {});
}
