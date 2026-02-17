const fs = require('node:fs');
const path = require('node:path');

const FIXTURE_PROVIDERS = ['claude', 'codex', 'gemini', 'opencode', 'vibe'];
const FIXTURE_LOOP_NAME = 'fixture-replay';
const FIXTURE_PROJECT_NAME = 'E2E Replay Fixtures';
const FIXTURE_PROJECT_MARKER_ID = 'fixture-replay-project-id';
const FIXTURE_SESSION_BASE_ID = 4200;
const WAIT_FIXTURE_SESSION_ID = 4301;
const WAIT_FIXTURE_PROFILE = 'wait-parent-fixture';
const WAIT_FIXTURE_LOOP_NAME = 'turn-scope-fixture';
const WAIT_FIXTURE_LOOP_RUN_ID = 7;
const WAIT_FIXTURE_LOOP_HEX = 'waitloop';
const WAIT_FIXTURE_STEP_HEX = 'waitstep';
const WAIT_FIXTURE_PREV_TURN_ID = 900;
const WAIT_FIXTURE_PREV_TURN_HEX = 'waitturn900';
const WAIT_FIXTURE_TURN_ID = 901;
const WAIT_FIXTURE_TURN_HEX = 'waitturn901';
const WAIT_FIXTURE_PROMPT_TEXT = 'Parent turn prompt without explicit turn_id metadata.';
const WAIT_FIXTURE_OUTPUT_TEXT = 'Parent output survives missing turn_id metadata.';
const ROLE_FIXTURE_SESSION_ID = 4302;
const ROLE_FIXTURE_LOOP_NAME = 'role-control-fixture';
const ROLE_FIXTURE_PROFILE = 'role-control-fixture';
const ROLE_FIXTURE_LOOP_RUN_ID = 8;
const ROLE_FIXTURE_LOOP_HEX = 'roleloop';
const ROLE_FIXTURE_MANAGER_STEP_HEX_C0 = 'role-c0-s0';
const ROLE_FIXTURE_SUPERVISOR_STEP_HEX_C0 = 'role-c0-s1';
const ROLE_FIXTURE_MANAGER_STEP_HEX_C1 = 'role-c1-s0';
const ROLE_FIXTURE_SUPERVISOR_STEP_HEX_C1 = 'role-c1-s1';
const ROLE_FIXTURE_TURN_MANAGER_0_ID = 910;
const ROLE_FIXTURE_TURN_MANAGER_0_HEX = 'roleturn910';
const ROLE_FIXTURE_TURN_MANAGER_1_ID = 911;
const ROLE_FIXTURE_TURN_MANAGER_1_HEX = 'roleturn911';
const ROLE_FIXTURE_TURN_SUPERVISOR_ID = 912;
const ROLE_FIXTURE_TURN_SUPERVISOR_HEX = 'roleturn912';
const ROLE_FIXTURE_CALL_SUPERVISOR_TEXT = 'Need next objective from supervisor.';
const ROLE_FIXTURE_STOP_TEXT = 'All manager and spawn work is complete.';
const SEED_PLAN_ID = 'seed-plan';
const SEED_WIKI_ID = 'seed-wiki';
const SEED_ISSUE_ID = 1;
const SEED_TURN_ID = 1;
const SEED_TURN_HEX = 'seedturn';
const SEED_RESUME_TURN_ID = 2;
const SEED_RESUME_TURN_HEX = 'seedturn2';
const RESUME_PROMPT_TEXT = 'Continue with the next steps after the previous response.';
const RESUME_ASSISTANT_TEXT = 'Resumed turn response for UI replay coverage.';

function prepareFixtureReplayData(repositoryRoot, homeDir, fixtureProjectDir) {
  const fixtures = loadFixtures(repositoryRoot);

  writeFixtureProject(fixtureProjectDir, repositoryRoot, homeDir);
  writeFixtureSessions(homeDir, fixtureProjectDir, fixtures);

  return fixtures.map(function (fixture, index) {
    return {
      provider: fixture.provider,
      session_id: FIXTURE_SESSION_BASE_ID + index + 1,
      profile: fixture.profile,
      expected_output: String(fixture.fixture.result && fixture.fixture.result.output ? fixture.fixture.result.output : '').trim(),
      fixture_file: fixture.filePath,
    };
  });
}

function loadFixtures(repositoryRoot) {
  return FIXTURE_PROVIDERS.map(function (provider) {
    const providerDir = path.join(repositoryRoot, 'internal', 'agent', 'testdata', provider);
    const files = fs.readdirSync(providerDir)
      .filter(function (name) { return name.endsWith('.json'); })
      .sort();

    if (files.length === 0) {
      throw new Error('No fixture files found for provider: ' + provider);
    }

    const filePath = path.join(providerDir, files[0]);
    const fixture = JSON.parse(fs.readFileSync(filePath, 'utf8'));
    return {
      provider: provider,
      profile: provider + '-fixture',
      filePath: filePath,
      fixture: fixture,
    };
  });
}

function writeFixtureProject(fixtureProjectDir, repositoryRoot, homeDir) {
  fs.rmSync(fixtureProjectDir, { recursive: true, force: true });
  fs.mkdirSync(fixtureProjectDir, { recursive: true });

  const projectStoreRoot = path.join(homeDir, '.adaf', 'projects', FIXTURE_PROJECT_MARKER_ID);
  fs.rmSync(projectStoreRoot, { recursive: true, force: true });
  fs.mkdirSync(projectStoreRoot, { recursive: true });

  const nowISO = new Date().toISOString();

  fs.writeFileSync(
    path.join(fixtureProjectDir, '.adaf.json'),
    JSON.stringify({ id: FIXTURE_PROJECT_MARKER_ID }, null, 2) + '\n',
    'utf8',
  );

  const projectConfig = {
    name: FIXTURE_PROJECT_NAME,
    repo_path: repositoryRoot,
    created: nowISO,
    agent_config: {},
    metadata: {
      source: 'playwright-fixture-replay',
    },
    active_plan_id: SEED_PLAN_ID,
  };

  fs.writeFileSync(
    path.join(projectStoreRoot, 'project.json'),
    JSON.stringify(projectConfig, null, 2) + '\n',
    'utf8',
  );

  writeSeedProjectData(projectStoreRoot, nowISO);
}

function writeSeedProjectData(adafDir, nowISO) {
  [
    'plans',
    'wiki',
    'issues',
    'local/turns',
    'local/records',
    'local/spawns',
    'local/messages',
    'local/loopruns',
    'local/stats/profiles',
    'local/stats/loops',
    'waits',
    'interrupts',
  ].forEach(function (dir) {
    fs.mkdirSync(path.join(adafDir, dir), { recursive: true });
  });

  const plan = {
    id: SEED_PLAN_ID,
    title: 'Seed Delivery Plan',
    description: 'Initial plan data seeded for Playwright e2e coverage.',
    status: 'active',
    created: nowISO,
    updated: nowISO,
  };
  fs.writeFileSync(
    path.join(adafDir, 'plans', SEED_PLAN_ID + '.json'),
    JSON.stringify(plan, null, 2) + '\n',
    'utf8',
  );

  const wiki = {
    id: SEED_WIKI_ID,
    plan_id: SEED_PLAN_ID,
    title: 'Seed Wiki',
    content: '# Seed Wiki\n\nThis markdown wiki entry is seeded for real e2e edit flows.',
    created: nowISO,
    updated: nowISO,
    created_by: 'fixture-seed',
    updated_by: 'fixture-seed',
    version: 1,
    history: [{
      version: 1,
      action: 'create',
      by: 'fixture-seed',
      at: nowISO,
    }],
  };
  fs.writeFileSync(
    path.join(adafDir, 'wiki', SEED_WIKI_ID + '.json'),
    JSON.stringify(wiki, null, 2) + '\n',
    'utf8',
  );

  const issue = {
    id: SEED_ISSUE_ID,
    plan_id: SEED_PLAN_ID,
    title: 'Seed issue',
    description: 'Initial issue content used by e2e editing tests.',
    status: 'open',
    priority: 'medium',
    labels: ['seed', 'e2e'],
    session_id: 0,
    created: nowISO,
    updated: nowISO,
  };
  fs.writeFileSync(
    path.join(adafDir, 'issues', String(SEED_ISSUE_ID) + '.json'),
    JSON.stringify(issue, null, 2) + '\n',
    'utf8',
  );

  const turn = {
    id: SEED_TURN_ID,
    hex_id: SEED_TURN_HEX,
    loop_run_hex_id: 'seedloop',
    step_hex_id: 'seedstep',
    plan_id: SEED_PLAN_ID,
    date: nowISO,
    agent: 'codex',
    agent_model: 'seed-model',
    profile_name: 'seed-profile',
    commit_hash: 'abc123',
    objective: 'Seed objective',
    what_was_built: 'Seeded baseline output for logs view.',
    key_decisions: 'Seed decisions',
    challenges: 'Seed challenges',
    current_state: 'Seed current state',
    known_issues: 'Seed known issues',
    next_steps: 'Seed next steps',
    build_state: 'passing',
    duration_secs: 42,
  };
  fs.writeFileSync(
    path.join(adafDir, 'local', 'turns', String(SEED_TURN_ID) + '.json'),
    JSON.stringify(turn, null, 2) + '\n',
    'utf8',
  );

  const resumeTurn = {
    id: SEED_RESUME_TURN_ID,
    hex_id: SEED_RESUME_TURN_HEX,
    loop_run_hex_id: 'seedloop',
    step_hex_id: 'seedstep',
    plan_id: SEED_PLAN_ID,
    date: nowISO,
    agent: 'codex',
    agent_model: 'seed-model',
    profile_name: 'seed-profile',
    commit_hash: 'def456',
    objective: 'Seed objective (continued)',
    what_was_built: 'Follow-up turn seeded for resume marker coverage.',
    key_decisions: 'Seed resume decisions',
    challenges: 'Seed resume challenges',
    current_state: 'Seed resumed state',
    known_issues: 'Seed resumed known issues',
    next_steps: 'Seed resumed next steps',
    build_state: 'passing',
    duration_secs: 8,
  };
  fs.writeFileSync(
    path.join(adafDir, 'local', 'turns', String(SEED_RESUME_TURN_ID) + '.json'),
    JSON.stringify(resumeTurn, null, 2) + '\n',
    'utf8',
  );

  const turnRecordDir = path.join(adafDir, 'local', 'records', String(SEED_TURN_ID));
  fs.mkdirSync(turnRecordDir, { recursive: true });
  const turnEvents = [
    {
      timestamp: nowISO,
      type: 'meta',
      data: JSON.stringify({ prompt: 'Seed turn prompt' }),
    },
    {
      timestamp: nowISO,
      type: 'stdout',
      data: 'Seed turn stdout',
    },
  ];
  fs.writeFileSync(
    path.join(turnRecordDir, 'events.jsonl'),
    turnEvents.map(function (event) { return JSON.stringify(event); }).join('\n') + '\n',
    'utf8',
  );

  writeTurnScopeFixtureData(adafDir, nowISO);
  writeRoleControlFixtureData(adafDir, nowISO);
}

function writeFixtureSessions(homeDir, fixtureProjectDir, fixtures) {
  const sessionsRoot = path.join(homeDir, '.adaf', 'sessions');
  fs.mkdirSync(sessionsRoot, { recursive: true });

  const now = Date.now();
  fixtures.forEach(function (entry, index) {
    const sessionID = FIXTURE_SESSION_BASE_ID + index + 1;
    const sessionDir = path.join(sessionsRoot, String(sessionID));
    fs.mkdirSync(sessionDir, { recursive: true });

    const startedAt = new Date(now - (fixtures.length-index)*90_000);
    const endedAt = new Date(startedAt.getTime() + 15_000);

    const meta = {
      id: sessionID,
      profile_name: entry.profile,
      agent_name: entry.provider,
      loop_name: FIXTURE_LOOP_NAME,
      loop_steps: 1,
      project_dir: fixtureProjectDir,
      project_name: FIXTURE_PROJECT_NAME,
      project_id: '',
      pid: 0,
      status: 'done',
      started_at: startedAt.toISOString(),
      ended_at: endedAt.toISOString(),
      error: '',
    };

    fs.writeFileSync(
      path.join(sessionDir, 'meta.json'),
      JSON.stringify(meta, null, 2) + '\n',
      'utf8',
    );

    const events = buildReplayEvents(entry.provider, entry.fixture);
    const payload = events.map(function (event) {
      return JSON.stringify(event);
    }).join('\n');

    fs.writeFileSync(
      path.join(sessionDir, 'events.jsonl'),
      (payload ? payload + '\n' : ''),
      'utf8',
    );
  });

  writeTurnScopeFixtureSession(sessionsRoot, fixtureProjectDir, now);
  writeRoleControlFixtureSession(sessionsRoot, fixtureProjectDir, now);
}

function writeTurnScopeFixtureData(adafDir, nowISO) {
  const prevTurn = {
    id: WAIT_FIXTURE_PREV_TURN_ID,
    hex_id: WAIT_FIXTURE_PREV_TURN_HEX,
    loop_run_hex_id: WAIT_FIXTURE_LOOP_HEX,
    step_hex_id: WAIT_FIXTURE_STEP_HEX,
    plan_id: SEED_PLAN_ID,
    date: nowISO,
    agent: 'codex',
    agent_model: 'seed-model',
    profile_name: WAIT_FIXTURE_PROFILE,
    objective: 'Turn fixture previous turn',
    what_was_built: 'Prior turn for turn-scope fixture.',
    current_state: 'Prior turn complete.',
    next_steps: 'Start next turn',
    build_state: 'success',
    duration_secs: 5,
  };
  fs.writeFileSync(
    path.join(adafDir, 'local', 'turns', String(WAIT_FIXTURE_PREV_TURN_ID) + '.json'),
    JSON.stringify(prevTurn, null, 2) + '\n',
    'utf8',
  );

  const waitTurn = {
    id: WAIT_FIXTURE_TURN_ID,
    hex_id: WAIT_FIXTURE_TURN_HEX,
    loop_run_hex_id: WAIT_FIXTURE_LOOP_HEX,
    step_hex_id: WAIT_FIXTURE_STEP_HEX,
    plan_id: SEED_PLAN_ID,
    date: nowISO,
    agent: 'codex',
    agent_model: 'seed-model',
    profile_name: WAIT_FIXTURE_PROFILE,
    objective: 'Turn fixture waiting turn',
    what_was_built: 'Waiting-for-spawns turn.',
    current_state: 'Waiting for spawned agents.',
    next_steps: 'Resume when spawns complete',
    build_state: 'waiting_for_spawns',
    duration_secs: 12,
  };
  fs.writeFileSync(
    path.join(adafDir, 'local', 'turns', String(WAIT_FIXTURE_TURN_ID) + '.json'),
    JSON.stringify(waitTurn, null, 2) + '\n',
    'utf8',
  );

  const loopRun = {
    id: WAIT_FIXTURE_LOOP_RUN_ID,
    hex_id: WAIT_FIXTURE_LOOP_HEX,
    loop_name: WAIT_FIXTURE_LOOP_NAME,
    plan_id: SEED_PLAN_ID,
    steps: [
      { profile: WAIT_FIXTURE_PROFILE, role: 'manager', turns: 1 },
    ],
    // Use "active" so the run remains in a live state after store startup cleanup.
    status: 'active',
    cycle: 0,
    step_index: 0,
    started_at: nowISO,
    session_ids: [WAIT_FIXTURE_PREV_TURN_ID, WAIT_FIXTURE_TURN_ID],
    step_last_seen_msg: { 0: 0 },
    pending_handoffs: [],
    step_hex_ids: { '0:0': WAIT_FIXTURE_STEP_HEX },
    daemon_session_id: WAIT_FIXTURE_SESSION_ID,
  };
  fs.writeFileSync(
    path.join(adafDir, 'local', 'loopruns', String(WAIT_FIXTURE_LOOP_RUN_ID) + '.json'),
    JSON.stringify(loopRun, null, 2) + '\n',
    'utf8',
  );
}

function writeTurnScopeFixtureSession(sessionsRoot, fixtureProjectDir, now) {
  const sessionDir = path.join(sessionsRoot, String(WAIT_FIXTURE_SESSION_ID));
  fs.mkdirSync(sessionDir, { recursive: true });

  const startedAt = new Date(now - 45_000);
  const endedAt = new Date(startedAt.getTime() + 10_000);
  const meta = {
    id: WAIT_FIXTURE_SESSION_ID,
    profile_name: WAIT_FIXTURE_PROFILE,
    agent_name: 'codex',
    loop_name: WAIT_FIXTURE_LOOP_NAME,
    loop_steps: 1,
    project_dir: fixtureProjectDir,
    project_name: FIXTURE_PROJECT_NAME,
    project_id: '',
    pid: 0,
    status: 'done',
    started_at: startedAt.toISOString(),
    ended_at: endedAt.toISOString(),
    error: '',
  };
  fs.writeFileSync(
    path.join(sessionDir, 'meta.json'),
    JSON.stringify(meta, null, 2) + '\n',
    'utf8',
  );

  const events = [
    {
      timestamp: startedAt.toISOString(),
      type: 'prompt',
      data: JSON.stringify({
        turn_hex_id: WAIT_FIXTURE_TURN_HEX,
        prompt: WAIT_FIXTURE_PROMPT_TEXT,
      }),
    },
    {
      timestamp: new Date(startedAt.getTime() + 1000).toISOString(),
      type: 'event',
      data: JSON.stringify({
        event: assistantTextEvent(WAIT_FIXTURE_OUTPUT_TEXT),
      }),
    },
    {
      timestamp: new Date(startedAt.getTime() + 2000).toISOString(),
      type: 'finished',
      data: JSON.stringify({
        wait_for_spawns: true,
      }),
    },
  ];
  fs.writeFileSync(
    path.join(sessionDir, 'events.jsonl'),
    events.map(function (event) { return JSON.stringify(event); }).join('\n') + '\n',
    'utf8',
  );
}

function writeRoleControlFixtureData(adafDir, nowISO) {
  const managerTurn0 = {
    id: ROLE_FIXTURE_TURN_MANAGER_0_ID,
    hex_id: ROLE_FIXTURE_TURN_MANAGER_0_HEX,
    loop_run_hex_id: ROLE_FIXTURE_LOOP_HEX,
    step_hex_id: ROLE_FIXTURE_MANAGER_STEP_HEX_C0,
    plan_id: SEED_PLAN_ID,
    date: nowISO,
    agent: 'codex',
    agent_model: 'seed-model',
    profile_name: ROLE_FIXTURE_PROFILE,
    objective: 'Role fixture manager turn 0',
    what_was_built: 'Initial manager work in cycle 0.',
    current_state: 'Manager work complete.',
    next_steps: 'Continue manager responsibilities',
    build_state: 'success',
    duration_secs: 4,
  };
  fs.writeFileSync(
    path.join(adafDir, 'local', 'turns', String(ROLE_FIXTURE_TURN_MANAGER_0_ID) + '.json'),
    JSON.stringify(managerTurn0, null, 2) + '\n',
    'utf8',
  );

  const managerTurn1 = {
    id: ROLE_FIXTURE_TURN_MANAGER_1_ID,
    hex_id: ROLE_FIXTURE_TURN_MANAGER_1_HEX,
    loop_run_hex_id: ROLE_FIXTURE_LOOP_HEX,
    step_hex_id: ROLE_FIXTURE_MANAGER_STEP_HEX_C1,
    plan_id: SEED_PLAN_ID,
    date: nowISO,
    agent: 'codex',
    agent_model: 'seed-model',
    profile_name: ROLE_FIXTURE_PROFILE,
    objective: 'Role fixture manager turn 1',
    what_was_built: 'Manager follow-up work in cycle 1.',
    current_state: 'Manager completed handoff checks.',
    next_steps: 'Escalate to supervisor for next objective',
    build_state: 'success',
    duration_secs: 3,
  };
  fs.writeFileSync(
    path.join(adafDir, 'local', 'turns', String(ROLE_FIXTURE_TURN_MANAGER_1_ID) + '.json'),
    JSON.stringify(managerTurn1, null, 2) + '\n',
    'utf8',
  );

  const supervisorTurn = {
    id: ROLE_FIXTURE_TURN_SUPERVISOR_ID,
    hex_id: ROLE_FIXTURE_TURN_SUPERVISOR_HEX,
    loop_run_hex_id: ROLE_FIXTURE_LOOP_HEX,
    step_hex_id: ROLE_FIXTURE_SUPERVISOR_STEP_HEX_C1,
    plan_id: SEED_PLAN_ID,
    date: nowISO,
    agent: 'codex',
    agent_model: 'seed-model',
    profile_name: ROLE_FIXTURE_PROFILE,
    objective: 'Role fixture supervisor turn',
    what_was_built: 'Supervisor processed escalation and stop decision.',
    current_state: 'Loop stop requested after supervisor review.',
    next_steps: 'Await operator confirmation',
    build_state: 'success',
    duration_secs: 2,
  };
  fs.writeFileSync(
    path.join(adafDir, 'local', 'turns', String(ROLE_FIXTURE_TURN_SUPERVISOR_ID) + '.json'),
    JSON.stringify(supervisorTurn, null, 2) + '\n',
    'utf8',
  );

  const loopRun = {
    id: ROLE_FIXTURE_LOOP_RUN_ID,
    hex_id: ROLE_FIXTURE_LOOP_HEX,
    loop_name: ROLE_FIXTURE_LOOP_NAME,
    plan_id: SEED_PLAN_ID,
    steps: [
      { profile: ROLE_FIXTURE_PROFILE, role: 'manager', position: 'manager', turns: 1 },
      { profile: ROLE_FIXTURE_PROFILE, role: 'supervisor', position: 'supervisor', turns: 1 },
    ],
    status: 'active',
    cycle: 1,
    step_index: 1,
    started_at: nowISO,
    session_ids: [ROLE_FIXTURE_TURN_MANAGER_0_ID, ROLE_FIXTURE_TURN_MANAGER_1_ID, ROLE_FIXTURE_TURN_SUPERVISOR_ID],
    step_last_seen_msg: { 0: 0, 1: 0 },
    pending_handoffs: [],
    step_hex_ids: {
      '0:0': ROLE_FIXTURE_MANAGER_STEP_HEX_C0,
      '0:1': ROLE_FIXTURE_SUPERVISOR_STEP_HEX_C0,
      '1:0': ROLE_FIXTURE_MANAGER_STEP_HEX_C1,
      '1:1': ROLE_FIXTURE_SUPERVISOR_STEP_HEX_C1,
    },
    daemon_session_id: ROLE_FIXTURE_SESSION_ID,
  };
  fs.writeFileSync(
    path.join(adafDir, 'local', 'loopruns', String(ROLE_FIXTURE_LOOP_RUN_ID) + '.json'),
    JSON.stringify(loopRun, null, 2) + '\n',
    'utf8',
  );
}

function writeRoleControlFixtureSession(sessionsRoot, fixtureProjectDir, now) {
  const sessionDir = path.join(sessionsRoot, String(ROLE_FIXTURE_SESSION_ID));
  fs.mkdirSync(sessionDir, { recursive: true });

  const startedAt = new Date(now - 35_000);
  const endedAt = new Date(startedAt.getTime() + 9_000);
  const meta = {
    id: ROLE_FIXTURE_SESSION_ID,
    profile_name: ROLE_FIXTURE_PROFILE,
    agent_name: 'codex',
    loop_name: ROLE_FIXTURE_LOOP_NAME,
    loop_steps: 2,
    project_dir: fixtureProjectDir,
    project_name: FIXTURE_PROJECT_NAME,
    project_id: '',
    pid: 0,
    status: 'done',
    started_at: startedAt.toISOString(),
    ended_at: endedAt.toISOString(),
    error: '',
  };
  fs.writeFileSync(
    path.join(sessionDir, 'meta.json'),
    JSON.stringify(meta, null, 2) + '\n',
    'utf8',
  );

  const callSupervisorCmd = `/bin/bash -lc 'adaf loop call-supervisor \"${ROLE_FIXTURE_CALL_SUPERVISOR_TEXT}\"'`;
  const stopCmd = `/bin/bash -lc 'adaf loop stop \"${ROLE_FIXTURE_STOP_TEXT}\"'`;

  const events = [
    {
      timestamp: startedAt.toISOString(),
      type: 'prompt',
      data: JSON.stringify({
        turn_hex_id: ROLE_FIXTURE_TURN_SUPERVISOR_HEX,
        prompt: 'Supervisor resumes after manager escalation.',
        is_resume: true,
      }),
    },
    {
      timestamp: new Date(startedAt.getTime() + 1000).toISOString(),
      type: 'event',
      data: JSON.stringify({
        event: assistantTextEvent('Processing escalation and loop shutdown request.'),
      }),
    },
    {
      timestamp: new Date(startedAt.getTime() + 2000).toISOString(),
      type: 'event',
      data: JSON.stringify({
        event: assistantToolUseEvent('Bash', 'role-call-supervisor', { command: callSupervisorCmd }),
      }),
    },
    {
      timestamp: new Date(startedAt.getTime() + 3000).toISOString(),
      type: 'event',
      data: JSON.stringify({
        event: userToolResultEvent('role-call-supervisor', 'Bash', 'Supervisor signal queued.', false),
      }),
    },
    {
      timestamp: new Date(startedAt.getTime() + 4000).toISOString(),
      type: 'event',
      data: JSON.stringify({
        event: assistantToolUseEvent('Bash', 'role-stop-loop', { command: stopCmd }),
      }),
    },
    {
      timestamp: new Date(startedAt.getTime() + 5000).toISOString(),
      type: 'event',
      data: JSON.stringify({
        event: userToolResultEvent('role-stop-loop', 'Bash', 'Loop stop signal sent.', false),
      }),
    },
    {
      timestamp: new Date(startedAt.getTime() + 6000).toISOString(),
      type: 'finished',
      data: JSON.stringify({
        turn_hex_id: ROLE_FIXTURE_TURN_SUPERVISOR_HEX,
      }),
    },
  ];
  fs.writeFileSync(
    path.join(sessionDir, 'events.jsonl'),
    events.map(function (event) { return JSON.stringify(event); }).join('\n') + '\n',
    'utf8',
  );
}

function buildReplayEvents(provider, fixture) {
  const capturedAt = Date.parse(fixture.captured_at || '');
  let ts = Number.isFinite(capturedAt) ? capturedAt : Date.now();
  const events = [];

  function push(type, data) {
    events.push({
      timestamp: new Date(ts).toISOString(),
      type: type,
      data: data,
    });
    ts += 1;
  }

  if (fixture.prompt) {
    push('meta', JSON.stringify({ prompt: String(fixture.prompt) }));
  }

  if (Array.isArray(fixture.events)) {
    fixture.events.forEach(function (event) {
      if (!event || typeof event !== 'object') return;
      if (event.type === 'meta' || event.type === 'stdin' || event.type === 'stdout' || event.type === 'stderr') {
        push(event.type, String(event.data == null ? '' : event.data));
      }
    });
  }

  const streamLines = Array.isArray(fixture.stream) ? fixture.stream : [];
  streamLines.forEach(function (line) {
    const rawLine = String(line == null ? '' : line);
    const parsed = parseMaybeJSON(rawLine);
    if (!parsed || typeof parsed !== 'object') {
      if (rawLine.trim()) {
        push('stdout', rawLine);
      }
      return;
    }

    const mapped = mapFixtureStreamEvent(provider, parsed);
    mapped.forEach(function (mappedEvent) {
      push('claude_stream', JSON.stringify(mappedEvent));
    });
  });

  const expectedOutput = String(fixture.result && fixture.result.output ? fixture.result.output : '').trim();
  const expectedToken = expectedOutput.replace(/[.!?]+$/, '');
  if (expectedOutput && expectedToken && !eventsContainText(events, expectedToken)) {
    push('claude_stream', JSON.stringify(assistantTextEvent(expectedOutput)));
  }

  if (provider === 'codex') {
    push('prompt', JSON.stringify({
      session_id: 0,
      turn_hex_id: SEED_RESUME_TURN_HEX,
      prompt: RESUME_PROMPT_TEXT,
      is_resume: true,
    }));
    push('event', JSON.stringify({
      event: assistantTextEvent(RESUME_ASSISTANT_TEXT),
    }));
  }

  return events;
}

function eventsContainText(events, text) {
  return events.some(function (event) {
    return typeof event.data === 'string' && event.data.indexOf(text) >= 0;
  });
}

function mapFixtureStreamEvent(provider, event) {
  switch (provider) {
    case 'claude':
      return mapClaudeStreamEvent(event);
    case 'codex':
      return mapCodexStreamEvent(event);
    case 'gemini':
      return mapGeminiStreamEvent(event);
    case 'opencode':
      return mapOpencodeStreamEvent(event);
    case 'vibe':
      return mapVibeStreamEvent(event);
    default:
      return [];
  }
}

function mapClaudeStreamEvent(event) {
  if (!event || typeof event !== 'object') return [];
  if (!event.type) return [];
  if (event.type !== 'assistant' && event.type !== 'user' && event.type !== 'content_block_delta') {
    return [];
  }
  return [event];
}

function mapCodexStreamEvent(event) {
  if (!event || typeof event !== 'object') return [];
  if (event.type === 'thread.started') {
    return [systemInitEvent(event.thread_id || '', 'codex')];
  }
  if (event.type !== 'item.started' && event.type !== 'item.updated' && event.type !== 'item.completed') {
    return [];
  }

  const item = event.item;
  if (!item || typeof item !== 'object') return [];
  const itemType = String(item.type || '');

  if (itemType === 'reasoning' && item.text) {
    return [assistantThinkingEvent(item.text)];
  }
  if (itemType === 'agent_message' && item.text) {
    return [assistantTextEvent(item.text)];
  }
  if (itemType === 'command_execution') {
    const status = String(item.status || '').toLowerCase();
    const isStart = event.type === 'item.started' || status === '' || status === 'in_progress';
    const commandID = item.id || 'codex-command';
    if (isStart) {
      return [assistantToolUseEvent('Bash', commandID, { command: item.command || '' })];
    }
    const exitCode = Number(item.exit_code);
    const isError = status === 'failed' || status === 'declined' || (Number.isFinite(exitCode) && exitCode !== 0);
    const output = item.aggregated_output || item.command || 'command finished';
    return [userToolResultEvent(commandID, 'Bash', output, isError)];
  }

  if (itemType === 'web_search') {
    if (event.type === 'item.started') {
      return [assistantToolUseEvent('web_search', item.id || 'codex-web-search', { query: item.query || '' })];
    }
    return [userToolResultEvent(item.id || 'codex-web-search', 'web_search', item.query || 'web search completed', false)];
  }

  if (itemType === 'file_change' && Array.isArray(item.changes) && item.changes.length > 0) {
    const summary = item.changes.map(function (change) {
      return 'File changes: ' + String(change.kind || 'update') + ' ' + String(change.path || '');
    }).join('\n');
    return [assistantTextEvent(summary)];
  }

  return [];
}

function mapGeminiStreamEvent(event) {
  if (!event || typeof event !== 'object') return [];

  if (event.type === 'init') {
    return [systemInitEvent(event.session_id || '', event.model || 'gemini')];
  }

  if (event.type === 'message') {
    if (event.role !== 'assistant') return [];
    if (event.delta) {
      return [contentDeltaEvent(event.content || '')];
    }
    return [assistantTextEvent(event.content || '')];
  }

  if (event.type === 'tool_use') {
    return [assistantToolUseEvent(event.tool_name || 'tool', event.tool_id || 'gemini-tool', event.parameters || {})];
  }

  if (event.type === 'tool_result') {
    let output = event.output;
    let isError = event.status === 'error';
    if (isError && event.error && event.error.message) {
      output = event.error.message;
    }
    if (output == null || output === '') {
      output = isError ? 'tool failed' : 'tool completed';
    }
    return [userToolResultEvent(event.tool_id || 'gemini-tool', 'tool_result', output, isError)];
  }

  if (event.type === 'error' && event.message) {
    return [assistantTextEvent('Gemini error: ' + event.message)];
  }

  return [];
}

function mapOpencodeStreamEvent(event) {
  if (!event || typeof event !== 'object') return [];

  if (event.type === 'step_start') {
    return [systemInitEvent(event.sessionID || '', 'opencode')];
  }

  const part = event.part && typeof event.part === 'object' ? event.part : {};

  if (event.type === 'text' && part.text) {
    return [assistantTextEvent(part.text)];
  }
  if (event.type === 'reasoning' && part.text) {
    return [assistantThinkingEvent(part.text)];
  }
  if (event.type === 'tool_use') {
    const toolName = part.tool || 'tool';
    const callID = part.callID || part.id || 'opencode-tool';
    const state = part.state && typeof part.state === 'object' ? part.state : {};
    const input = state.input && typeof state.input === 'object' ? state.input : {};
    const output = state.output || 'tool completed';
    const isError = state.status === 'error';
    return [
      assistantToolUseEvent(toolName, callID, input),
      userToolResultEvent(callID, toolName, output, isError),
    ];
  }

  if (event.type === 'error') {
    const msg = event.error && event.error.name ? String(event.error.name) : 'OpenCode error';
    return [assistantTextEvent(msg)];
  }

  return [];
}

function mapVibeStreamEvent(event) {
  if (!event || typeof event !== 'object') return [];

  if (event.role === 'assistant') {
    const mapped = [];

    if (event.reasoning_content) {
      mapped.push(assistantThinkingEvent(event.reasoning_content));
    }

    const blocks = [];
    const text = String(event.content || '');
    if (text.trim()) {
      blocks.push({ type: 'text', text: text });
    }

    if (Array.isArray(event.tool_calls)) {
      event.tool_calls.forEach(function (toolCall) {
        if (!toolCall || typeof toolCall !== 'object') return;
        const fn = toolCall.function && typeof toolCall.function === 'object' ? toolCall.function : {};
        const args = parseMaybeJSON(String(fn.arguments || ''));
        blocks.push({
          type: 'tool_use',
          id: toolCall.id || '',
          name: fn.name || 'tool',
          input: args || String(fn.arguments || ''),
        });
      });
    }

    if (blocks.length > 0) {
      mapped.push({
        type: 'assistant',
        message: {
          role: 'assistant',
          content: blocks,
        },
      });
    }

    return mapped;
  }

  if (event.role === 'tool') {
    const toolName = event.name || 'tool_result';
    const toolCallID = event.tool_call_id || toolName;
    return [userToolResultEvent(toolCallID, toolName, event.content || '', false)];
  }

  return [];
}

function assistantTextEvent(text) {
  return {
    type: 'assistant',
    message: {
      role: 'assistant',
      content: [
        {
          type: 'text',
          text: String(text || ''),
        },
      ],
    },
  };
}

function assistantThinkingEvent(text) {
  return {
    type: 'assistant',
    message: {
      role: 'assistant',
      content: [
        {
          type: 'thinking',
          text: String(text || ''),
        },
      ],
    },
  };
}

function assistantToolUseEvent(name, id, input) {
  return {
    type: 'assistant',
    message: {
      role: 'assistant',
      content: [
        {
          type: 'tool_use',
          name: String(name || 'tool'),
          id: String(id || 'tool-call'),
          input: input == null ? {} : input,
        },
      ],
    },
  };
}

function userToolResultEvent(toolUseID, name, content, isError) {
  return {
    type: 'user',
    message: {
      role: 'user',
      content: [
        {
          type: 'tool_result',
          tool_use_id: String(toolUseID || ''),
          name: String(name || ''),
          content: content == null ? '' : content,
          is_error: !!isError,
        },
      ],
    },
  };
}

function contentDeltaEvent(text) {
  return {
    type: 'content_block_delta',
    delta: {
      type: 'text_delta',
      text: String(text || ''),
    },
  };
}

function systemInitEvent(sessionID, model) {
  return {
    type: 'system',
    subtype: 'init',
    session_id: String(sessionID || ''),
    model: String(model || ''),
  };
}

function parseMaybeJSON(raw) {
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch (_) {
    return null;
  }
}

module.exports = {
  FIXTURE_PROJECT_NAME,
  prepareFixtureReplayData,
};
