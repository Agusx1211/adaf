import { arrayOrEmpty, numberOr } from './format.js';

export function parseScope(scope) {
  var raw = String(scope || '');
  if (raw.indexOf('turn-main-') === 0) {
    var mainTurnID = parseInt(raw.slice(10), 10);
    if (!Number.isNaN(mainTurnID) && mainTurnID > 0) {
      return {
        kind: 'turn_main',
        id: mainTurnID,
        scope: 'turn-main-' + mainTurnID,
        turnScope: 'turn-' + mainTurnID,
      };
    }
  }
  if (raw.indexOf('turn-') === 0) {
    var turnID = parseInt(raw.slice(5), 10);
    if (!Number.isNaN(turnID) && turnID > 0) {
      return { kind: 'turn', id: turnID, scope: 'turn-' + turnID };
    }
  }
  if (raw.indexOf('session-main-') === 0) {
    var mainSessionID = parseInt(raw.slice(13), 10);
    if (!Number.isNaN(mainSessionID) && mainSessionID > 0) {
      return {
        kind: 'session_main',
        id: mainSessionID,
        scope: 'session-main-' + mainSessionID,
        sessionScope: 'session-' + mainSessionID,
      };
    }
  }
  if (raw.indexOf('session-') === 0) {
    var sessionID = parseInt(raw.slice(8), 10);
    if (!Number.isNaN(sessionID) && sessionID > 0) {
      return { kind: 'session', id: sessionID, scope: 'session-' + sessionID };
    }
  }
  if (raw.indexOf('spawn-') === 0) {
    var spawnID = parseInt(raw.slice(6), 10);
    if (!Number.isNaN(spawnID) && spawnID > 0) {
      return { kind: 'spawn', id: spawnID, scope: 'spawn-' + spawnID };
    }
  }
  return { kind: 'unknown', id: 0, scope: raw };
}

export function buildTurnToSessionMap(loopRuns) {
  var turnToSession = {};
  arrayOrEmpty(loopRuns).forEach(function (run) {
    var daemonSessionID = numberOr(0, run && run.daemon_session_id);
    if (daemonSessionID <= 0) return;
    arrayOrEmpty(run && run.turn_ids).forEach(function (turnID) {
      var tid = numberOr(0, turnID);
      if (tid > 0) turnToSession[tid] = daemonSessionID;
    });
  });
  return turnToSession;
}

export function buildSpawnScopeMaps(spawns, loopRuns) {
  var list = arrayOrEmpty(spawns);
  var turnToSession = buildTurnToSessionMap(loopRuns);
  var spawnToSession = {};
  var sessionToSpawnIDs = {};
  var rootSpawnIDsByTurn = {};
  var childrenByParent = {};

  list.forEach(function (spawn) {
    if (!spawn || spawn.id <= 0) return;
    var parentSpawnID = numberOr(0, spawn.parent_spawn_id);
    if (parentSpawnID > 0) {
      if (!childrenByParent[parentSpawnID]) childrenByParent[parentSpawnID] = [];
      childrenByParent[parentSpawnID].push(spawn.id);
      return;
    }
    var parentTurnID = numberOr(0, spawn.parent_turn_id);
    if (parentTurnID > 0) {
      if (!rootSpawnIDsByTurn[parentTurnID]) rootSpawnIDsByTurn[parentTurnID] = [];
      rootSpawnIDsByTurn[parentTurnID].push(spawn.id);
    }
  });

  list.forEach(function (spawn) {
    if (!spawn || spawn.id <= 0 || spawn.parent_spawn_id > 0) return;
    var sid = numberOr(0, spawn.parent_daemon_session_id);
    if (sid <= 0 && spawn.parent_turn_id > 0) sid = numberOr(0, turnToSession[spawn.parent_turn_id]);
    if (sid <= 0 && spawn.child_turn_id > 0) sid = numberOr(0, turnToSession[spawn.child_turn_id]);
    if (sid > 0) spawnToSession[spawn.id] = sid;
  });

  // Propagate root daemon session IDs down the spawn tree.
  for (var pass = 0; pass < list.length; pass++) {
    var changed = false;
    list.forEach(function (spawn) {
      if (!spawn || spawn.id <= 0 || spawnToSession[spawn.id] > 0) return;
      if (spawn.parent_spawn_id <= 0) return;
      var parentSID = numberOr(0, spawnToSession[spawn.parent_spawn_id]);
      if (parentSID > 0) {
        spawnToSession[spawn.id] = parentSID;
        changed = true;
      }
    });
    if (!changed) break;
  }

  // Last-resort inference for non-root spawns if parent resolution failed.
  list.forEach(function (spawn) {
    if (!spawn || spawn.id <= 0 || spawnToSession[spawn.id] > 0) return;
    var sid = numberOr(0, spawn.parent_daemon_session_id);
    if (sid <= 0 && spawn.parent_turn_id > 0) sid = numberOr(0, turnToSession[spawn.parent_turn_id]);
    if (sid <= 0 && spawn.child_turn_id > 0) sid = numberOr(0, turnToSession[spawn.child_turn_id]);
    if (sid > 0) spawnToSession[spawn.id] = sid;
  });

  Object.keys(spawnToSession).forEach(function (spawnKey) {
    var spawnID = parseInt(spawnKey, 10);
    var sid = numberOr(0, spawnToSession[spawnKey]);
    if (spawnID <= 0 || sid <= 0) return;
    if (!sessionToSpawnIDs[sid]) sessionToSpawnIDs[sid] = [];
    sessionToSpawnIDs[sid].push(spawnID);
  });

  Object.keys(sessionToSpawnIDs).forEach(function (sidKey) {
    sessionToSpawnIDs[sidKey].sort(function (a, b) { return a - b; });
  });

  var turnToSpawnIDs = {};
  Object.keys(rootSpawnIDsByTurn).forEach(function (turnKey) {
    var turnID = parseInt(turnKey, 10);
    if (Number.isNaN(turnID) || turnID <= 0) return;

    var seen = {};
    var stack = rootSpawnIDsByTurn[turnKey].slice();
    while (stack.length > 0) {
      var spawnID = numberOr(0, stack.pop());
      if (spawnID <= 0 || seen[spawnID]) continue;
      seen[spawnID] = true;
      var childIDs = childrenByParent[spawnID] || [];
      childIDs.forEach(function (childID) { stack.push(childID); });
    }

    var ids = Object.keys(seen).map(function (id) { return parseInt(id, 10); })
      .filter(function (id) { return !Number.isNaN(id) && id > 0; })
      .sort(function (a, b) { return a - b; });
    turnToSpawnIDs[turnID] = ids;
  });

  return {
    turnToSession: turnToSession,
    spawnToSession: spawnToSession,
    sessionToSpawnIDs: sessionToSpawnIDs,
    turnToSpawnIDs: turnToSpawnIDs,
  };
}

export function resolveScopeSessionID(scope, sessions, spawns, loopRuns) {
  var parsed = parseScope(scope);
  if (parsed.kind === 'session' || parsed.kind === 'session_main') return parsed.id;
  if (parsed.kind === 'spawn' || parsed.kind === 'turn' || parsed.kind === 'turn_main') {
    var maps = buildSpawnScopeMaps(spawns, loopRuns);
    if (parsed.kind === 'turn' || parsed.kind === 'turn_main') {
      var turnSID = numberOr(0, maps.turnToSession[parsed.id]);
      if (turnSID > 0) return turnSID;
      var sessionListFromTurn = arrayOrEmpty(sessions);
      return sessionListFromTurn.length ? numberOr(0, sessionListFromTurn[0] && sessionListFromTurn[0].id) : 0;
    }
    var sid = numberOr(0, maps.spawnToSession[parsed.id]);
    if (sid > 0) return sid;
  }
  var sessionList = arrayOrEmpty(sessions);
  return sessionList.length ? numberOr(0, sessionList[0] && sessionList[0].id) : 0;
}
