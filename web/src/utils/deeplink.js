// Deep link utilities: bidirectional mapping between app state and URL hash.
// Hash format: #/view[/selection...]
// Coexists with #token=xxx auth flow (different prefix).

var VALID_VIEWS = ['loops', 'standalone', 'issues', 'wiki', 'plan', 'logs', 'config'];

/**
 * Converts current navigation state to a hash string.
 * Returns '' (empty) for the default state (loops view, no selection).
 */
export function stateToHash(state) {
  var view = state.leftView || 'loops';
  var parts = [view];

  switch (view) {
    case 'loops':
      if (state.selectedScope) {
        parts.push(encodeURIComponent(state.selectedScope));
      }
      break;

    case 'standalone':
      if (state.standaloneChatID) {
        parts.push(encodeURIComponent(state.standaloneChatID));
      }
      break;

    case 'issues':
      if (state.selectedIssue != null) {
        parts.push(String(state.selectedIssue));
      }
      break;

    case 'wiki':
      if (state.selectedWiki != null) {
        parts.push(encodeURIComponent(String(state.selectedWiki)));
      }
      break;

    case 'plan':
      if (state.selectedPlan != null) {
        parts.push(encodeURIComponent(String(state.selectedPlan)));
      }
      break;

    case 'logs':
      if (state.selectedTurn != null) {
        parts.push(String(state.selectedTurn));
      }
      break;

    case 'config':
      if (state.configSelection && state.configSelection.type && state.configSelection.name) {
        parts.push(encodeURIComponent(state.configSelection.type));
        parts.push(encodeURIComponent(state.configSelection.name));
      }
      break;
  }

  return '#/' + parts.join('/');
}

/**
 * Parses a hash string and returns an array of dispatch actions to restore that state.
 * Returns empty array for unrecognized or empty hashes (including #token=xxx).
 */
export function hashToActions(hash) {
  if (!hash || hash.indexOf('#/') !== 0) return [];

  var path = hash.slice(2); // strip '#/'
  var segments = path.split('/').map(function (s) { return decodeURIComponent(s); });
  var view = segments[0] || '';

  if (VALID_VIEWS.indexOf(view) === -1) return [];

  var actions = [{ type: 'SET_LEFT_VIEW', payload: view }];

  switch (view) {
    case 'loops':
      if (segments[1]) {
        actions.push({ type: 'SET_SELECTED_SCOPE', payload: segments[1] });
      }
      break;

    case 'standalone':
      if (segments[1]) {
        actions.push({ type: 'SET_STANDALONE_CHAT_ID', payload: segments[1] });
      }
      break;

    case 'issues': {
      var issueID = parseInt(segments[1], 10);
      if (!Number.isNaN(issueID)) {
        actions.push({ type: 'SET_SELECTED_ISSUE', payload: issueID });
      }
      break;
    }

    case 'wiki':
      if (segments[1]) {
        actions.push({ type: 'SET_SELECTED_WIKI', payload: segments[1] });
      }
      break;

    case 'plan':
      if (segments[1]) {
        actions.push({ type: 'SET_SELECTED_PLAN', payload: segments[1] });
      }
      break;

    case 'logs': {
      var turnID = parseInt(segments[1], 10);
      if (!Number.isNaN(turnID)) {
        actions.push({ type: 'SET_SELECTED_TURN', payload: turnID });
      }
      break;
    }

    case 'config':
      if (segments[1] && segments[2]) {
        actions.push({ type: 'SET_CONFIG_SELECTION', payload: { type: segments[1], name: segments[2] } });
      }
      break;
  }

  return actions;
}
