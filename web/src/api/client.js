var _authToken = '';

export function loadAuthToken() {
  var hash = window.location.hash || '';
  if (hash.indexOf('#token=') === 0) {
    saveAuthToken(hash.slice(7));
    history.replaceState(null, '', window.location.pathname + window.location.search);
    return;
  }
  try {
    _authToken = localStorage.getItem('adaf_token') || '';
  } catch (_) {
    _authToken = '';
  }
}

export function saveAuthToken(token) {
  _authToken = String(token || '').trim();
  try { localStorage.setItem('adaf_token', _authToken); } catch (_) {}
}

export function clearAuthToken() {
  _authToken = '';
  try { localStorage.removeItem('adaf_token'); } catch (_) {}
}

export function getAuthToken() {
  return _authToken;
}

export function hasAuthToken() {
  return !!_authToken;
}

export async function apiCall(path, method, body, options) {
  var headers = { Accept: 'application/json' };
  if (_authToken) headers.Authorization = 'Bearer ' + _authToken;

  var request = { method: method || 'GET', headers: headers };

  if (body != null) {
    headers['Content-Type'] = 'application/json';
    request.body = JSON.stringify(body);
  }

  var response = await fetch(path, request);

  if (response.ok) {
    if (response.status === 204) return null;
    var text = await response.text();
    if (!text) return null;
    try { return JSON.parse(text); } catch (_) { return text; }
  }

  if (response.status === 401) {
    var authErr = new Error('Auth required');
    authErr.authRequired = true;
    throw authErr;
  }

  if (response.status === 404 && options && options.allow404) {
    return null;
  }

  if (response.status === 204) return null;

  var message = response.status + ' ' + response.statusText;
  try {
    var payload = await response.json();
    if (payload && payload.error) message = payload.error;
  } catch (_) {}

  throw new Error(message);
}

export function buildWSURL(path) {
  var proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  var url = proto + '//' + window.location.host + path;
  if (_authToken) {
    url += (path.indexOf('?') >= 0 ? '&' : '?') + 'token=' + encodeURIComponent(_authToken);
  }
  return url;
}

export function apiBase(projectID) {
  if (projectID) {
    return '/api/projects/' + encodeURIComponent(projectID);
  }
  return '/api';
}
