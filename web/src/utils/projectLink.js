var PROJECT_QUERY_KEY = 'project';

export function readProjectIDFromURL() {
  try {
    var params = new URLSearchParams(window.location.search || '');
    return String(params.get(PROJECT_QUERY_KEY) || '').trim();
  } catch (_) {
    return '';
  }
}

export function syncProjectIDToURL(projectID) {
  try {
    var id = String(projectID || '').trim();
    var url = new URL(window.location.href);
    if (id) {
      url.searchParams.set(PROJECT_QUERY_KEY, id);
    } else {
      url.searchParams.delete(PROJECT_QUERY_KEY);
    }
    history.replaceState(null, '', url.pathname + url.search + url.hash);
  } catch (_) {}
}

export function persistProjectSelection(projectID) {
  var id = String(projectID || '').trim();
  try {
    if (id) {
      localStorage.setItem('adaf_project_id', id);
    } else {
      localStorage.removeItem('adaf_project_id');
    }
  } catch (_) {}
  syncProjectIDToURL(id);
}
