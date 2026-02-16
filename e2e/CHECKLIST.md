# E2E Master Checklist (Real Flows, No Mocks)

Status legend:
- `[ ]` not implemented
- `[x]` implemented and passing
- `[~]` implemented but flaky or blocked

Priority legend:
- `P0` critical user journey
- `P1` important behavior
- `P2` nice-to-have hardening
- `ENV` requires external binaries/credentials/network

## A) Harness, Boot, and Base Surface

- [x] `E2E-001 (P0)` `adaf web` starts successfully in test harness.
- [x] `E2E-002 (P0)` `/` serves SPA shell and app mounts in browser.
- [x] `E2E-003 (P0)` `/api/config` returns valid JSON config.
- [x] `E2E-004 (P0)` `/static/app.js` and `/static/style.css` are served.
- [x] `E2E-005 (P0)` unknown API route returns JSON 404 response.
- [ ] `E2E-006 (P1)` global setup creates isolated HOME and does not touch host user state.
- [x] `E2E-007 (P1)` global teardown kills server process and removes temp dirs.
- [x] `E2E-008 (P1)` page reload keeps app usable with same base URL.
- [ ] `E2E-009 (P1)` no uncaught browser exceptions during initial load.
- [x] `E2E-010 (P1)` no failed network requests on clean boot path.

## B) Project Discovery, Picker, Init, Open, Switching

- [ ] `E2E-011 (P0)` project picker appears when no selected project exists.
- [ ] `E2E-012 (P0)` unresolved `?project=<id>` shows "project not found" picker state.
- [x] `E2E-013 (P0)` init new project via UI creates `.adaf/project.json`.
- [ ] `E2E-014 (P0)` open existing project via path from picker.
- [ ] `E2E-015 (P0)` select already registered project from picker.
- [x] `E2E-016 (P0)` switching project from top bar updates all views to new project data.
- [x] `E2E-017 (P0)` selected project persists in URL query.
- [ ] `E2E-018 (P0)` selected project persists in local storage.
- [ ] `E2E-019 (P1)` recent projects API list is rendered and selectable.
- [ ] `E2E-020 (P1)` invalid open path shows user-visible error.
- [ ] `E2E-021 (P1)` init with duplicate existing project path shows expected error.
- [x] `E2E-022 (P1)` project browser modal opens and closes correctly.
- [ ] `E2E-023 (P1)` project browser path navigation works from filesystem root.
- [x] `E2E-024 (P1)` project dashboard `/api/projects/dashboard` includes all registered projects.
- [x] `E2E-025 (P1)` project-scoped routes only return data for selected project.
- [ ] `E2E-026 (P2)` auto-register project by URL project ID works on first request.

## C) Top Bar and Global Navigation

- [x] `E2E-027 (P0)` switching between `Loops/Standalone/Issues/Docs/Plan/Logs/Config` works.
- [x] `E2E-028 (P0)` selected left view updates URL hash and survives refresh.
- [ ] `E2E-029 (P1)` running counter updates when session status changes.
- [ ] `E2E-030 (P1)` running sessions dropdown lists active sessions with stop controls.
- [ ] `E2E-031 (P1)` websocket live/offline indicator reflects socket state.
- [ ] `E2E-032 (P1)` usage limits pill opens details dropdown.
- [ ] `E2E-033 (P2)` top bar loop spinner appears during active loop runs.

## D) Loop Tree, Scope Selection, and Agent Pane

- [x] `E2E-034 (P0)` loop runs are listed with grouped turns.
- [ ] `E2E-035 (P0)` completed loop group can be expanded/collapsed.
- [x] `E2E-036 (P0)` selecting a turn updates center panel agent output.
- [ ] `E2E-037 (P0)` selecting all/main/spawn scope updates rendered stream correctly.
- [ ] `E2E-038 (P1)` standalone sessions (no loop) render in standalone section.
- [ ] `E2E-039 (P1)` elapsed timers render non-empty values.
- [ ] `E2E-040 (P1)` status dots/colors reflect running/completed/failed states.
- [ ] `E2E-041 (P1)` agent info bar updates model/profile/agent/status for selected scope.
- [ ] `E2E-042 (P2)` parent/spawn sidebar count is correct for multi-spawn runs.

## E) Historical Replay and Stream Rendering

- [x] `E2E-043 (P0)` completed sessions load historical events from `/sessions/{id}/events`.
- [ ] `E2E-044 (P0)` initial prompt block is rendered from metadata.
- [x] `E2E-045 (P0)` assistant text blocks render from recorded stream events.
- [ ] `E2E-046 (P0)` thinking blocks render and can be expanded/collapsed.
- [ ] `E2E-047 (P0)` tool call blocks render name + arguments.
- [ ] `E2E-048 (P0)` tool result blocks render success and error states.
- [x] `E2E-049 (P0)` provider fixture replay renders expected completion token per provider.
- [ ] `E2E-050 (P1)` source labels show per-agent/per-spawn attribution in all-agents view.
- [ ] `E2E-051 (P1)` markdown rendering works for multiline assistant text.
- [ ] `E2E-052 (P1)` file change lines render as badges when present.
- [ ] `E2E-053 (P1)` invalid JSON lines do not crash UI and show fallback text.
- [ ] `E2E-054 (P1)` unknown event types trigger graceful fallback and continue rendering.
- [ ] `E2E-055 (P1)` auto-scroll follows stream when pinned to bottom.
- [ ] `E2E-056 (P1)` "Jump to latest" appears when user scrolls up.
- [ ] `E2E-057 (P2)` inspect modal opens for agent messages with prompt/event payload.

## F) Session and Loop Actions

- [ ] `E2E-058 (P0)` start ask session from UI creates session and shows running state.
- [ ] `E2E-059 (P0)` start loop session from UI creates loop run and associated session.
- [ ] `E2E-060 (P0)` stopping a running session updates status to non-running.
- [ ] `E2E-061 (P0)` stop loop run endpoint updates loop status and UI state.
- [ ] `E2E-062 (P1)` send session message endpoint persists message and UI confirms.
- [ ] `E2E-063 (P1)` send loop message endpoint persists and appears in loop message stream.
- [ ] `E2E-064 (P1)` retrying stop on already stopped session shows non-crashing behavior.
- [ ] `E2E-065 (P1)` selecting running session opens websocket automatically.
- [ ] `E2E-066 (P1)` closing websocket flips live indicator to offline.
- [ ] `E2E-067 (P1)` websocket reconnect logic recovers after temporary disconnect.
- [ ] `E2E-068 (P2)` loop done event updates UI without manual refresh.

## G) Standalone Chat Instances

- [x] `E2E-069 (P0)` create standalone chat with profile only.
- [ ] `E2E-070 (P0)` create standalone chat with profile + team.
- [ ] `E2E-071 (P0)` create standalone chat with selected skills.
- [ ] `E2E-072 (P0)` chat list updates immediately after creation.
- [ ] `E2E-073 (P0)` selecting a chat loads message history.
- [x] `E2E-074 (P0)` delete chat removes it from list and clears selection if active.
- [ ] `E2E-075 (P1)` quick-pick recent profile/team combinations in new chat modal.
- [ ] `E2E-076 (P1)` chat status badges (`thinking`/`responding`) display while active.
- [ ] `E2E-077 (P1)` saving assistant response endpoint persists events + content.
- [ ] `E2E-078 (P1)` patch chat title/team/skills updates row rendering.
- [ ] `E2E-079 (P2)` opening standalone view with zero chats shows empty-state CTA.

## H) Issues View and CRUD

- [ ] `E2E-080 (P0)` create issue from UI and verify persisted in list.
- [x] `E2E-081 (P0)` update issue fields (title/description/status/priority).
- [ ] `E2E-082 (P0)` delete issue removes it from list and backend store.
- [ ] `E2E-083 (P1)` issue filter by status works.
- [ ] `E2E-084 (P1)` issue filtering by plan via query is reflected in UI.
- [x] `E2E-085 (P1)` issue detail panel loads selected issue correctly.
- [ ] `E2E-086 (P2)` switching projects while in issues view clears stale selection.

## I) Plans View and CRUD

- [ ] `E2E-087 (P0)` create plan from UI.
- [x] `E2E-088 (P0)` update plan title/description/status.
- [ ] `E2E-089 (P0)` activate plan and verify `active_plan_id` changes.
- [ ] `E2E-090 (P0)` delete plan removes it and updates selection safely.
- [ ] `E2E-091 (P1)` create and edit plan phases.
- [ ] `E2E-092 (P1)` update individual phase status and details.
- [ ] `E2E-093 (P1)` phase dependency fields persist and reload.
- [ ] `E2E-094 (P2)` plan detail view renders after reload with deep link state.

## J) Docs View and CRUD

- [ ] `E2E-095 (P0)` create doc from UI.
- [x] `E2E-096 (P0)` edit doc title/content and persist.
- [ ] `E2E-097 (P0)` delete doc and verify list update.
- [ ] `E2E-098 (P1)` markdown-like content round-trips without truncation.
- [ ] `E2E-099 (P1)` docs filtered by plan ID work when active plan changes.

## K) Logs View, Turn Details, and Recordings

- [x] `E2E-100 (P0)` logs view lists turns from backend.
- [x] `E2E-101 (P0)` selecting turn loads detail panel fields.
- [x] `E2E-102 (P1)` turn detail updates persist through `PUT /turns/{id}`.
- [ ] `E2E-103 (P1)` turn recording endpoint `/turns/{id}/events` renders in UI.
- [ ] `E2E-104 (P1)` fallback to session events works when turn recording missing.
- [ ] `E2E-105 (P2)` logs view with empty turns shows clean empty state.

## L) Config View: Profiles, Loops, Teams, Roles, Skills, Rules, Pushover, Agents

- [x] `E2E-106 (P0)` list profiles and open profile detail panel.
- [x] `E2E-107 (P0)` create profile and verify list refresh.
- [x] `E2E-108 (P0)` update profile and verify persisted change.
- [x] `E2E-109 (P0)` delete profile and verify removal.
- [x] `E2E-110 (P0)` create loop definition and verify list/detail.
- [ ] `E2E-111 (P0)` update loop steps and verify step summary rendering.
- [x] `E2E-112 (P0)` delete loop definition.
- [ ] `E2E-113 (P0)` create team with delegation profiles.
- [ ] `E2E-114 (P0)` update team delegation and verify sub-agent count.
- [x] `E2E-115 (P0)` delete team.
- [ ] `E2E-116 (P0)` create role and update role flags.
- [x] `E2E-117 (P0)` delete role.
- [x] `E2E-118 (P0)` create skill and verify appears in standalone chat modal.
- [ ] `E2E-119 (P0)` update skill metadata and prompt/instructions.
- [x] `E2E-120 (P0)` delete skill.
- [ ] `E2E-121 (P1)` create rule and delete rule.
- [ ] `E2E-122 (P1)` list recent combinations and use in quick-pick.
- [ ] `E2E-123 (P1)` update pushover config and re-fetch persisted values.
- [ ] `E2E-124 (P1)` list detected agents endpoint returns expected shape.
- [ ] `E2E-125 (P1)` trigger agent detection endpoint and show result/refresh.
- [ ] `E2E-126 (P2)` config section collapse/expand state works visually.

## M) Filesystem Endpoints and Browser Flows

- [x] `E2E-127 (P0)` browse filesystem root via `/api/fs/browse`.
- [ ] `E2E-128 (P0)` browse nested folder and parent navigation works.
- [x] `E2E-129 (P0)` create folder via `/api/fs/mkdir`.
- [ ] `E2E-130 (P1)` invalid mkdir path returns user-visible error.
- [ ] `E2E-131 (P1)` path traversal attempts are blocked by allowed root.
- [ ] `E2E-132 (P1)` browsing outside allowed root is denied.
- [ ] `E2E-133 (P2)` symlink edge cases do not escape allowed root.

## N) Authentication, Authorization, and Rate Limit

- [ ] `E2E-134 (P0)` with auth token enabled, unauthenticated UI gets auth-required state.
- [ ] `E2E-135 (P0)` auth modal accepts valid bearer token and unlocks API calls.
- [ ] `E2E-136 (P0)` invalid token keeps API blocked and displays failure.
- [ ] `E2E-137 (P1)` token in URL hash (`#token=...`) is consumed and persisted.
- [ ] `E2E-138 (P1)` clear stored token removes authorization and re-locks protected paths.
- [ ] `E2E-139 (P1)` websocket auth token query path works for session socket.
- [ ] `E2E-140 (P1)` rate limit >0 returns 429 under burst traffic.
- [ ] `E2E-141 (P2)` rate limit disabled (`0`) allows burst traffic without 429.

## O) TLS, Expose Mode, and Web Daemon Lifecycle

- [ ] `E2E-142 (P1)` HTTPS self-signed mode starts and UI is reachable with ignored cert errors.
- [ ] `E2E-143 (P1)` custom cert/key mode starts and serves TLS endpoints.
- [ ] `E2E-144 (P1)` expose mode generates auth token when not provided.
- [ ] `E2E-145 (P1)` daemon mode start writes runtime files and status command reports running.
- [ ] `E2E-146 (P1)` daemon mode stop command terminates server and clears runtime files.
- [ ] `E2E-147 (P2)` `adaf web status` reports not running after stop.

## P) WebSocket Session Stream Semantics

- [ ] `E2E-148 (P0)` snapshot envelope is consumed and recent events replayed.
- [ ] `E2E-149 (P0)` prompt envelope renders initial prompt block.
- [ ] `E2E-150 (P0)` assistant event envelope renders text/thinking/tool blocks.
- [ ] `E2E-151 (P0)` user tool_result event envelope renders tool result block.
- [ ] `E2E-152 (P1)` raw envelope renders cropped raw text.
- [ ] `E2E-153 (P1)` spawn envelope updates tree/sidebar spawn state.
- [ ] `E2E-154 (P1)` error envelope displays user-visible error text.
- [ ] `E2E-155 (P1)` unknown envelope type does not crash UI and reports missing sample.

## Q) Terminal WebSocket

- [ ] `E2E-156 (P1)` terminal websocket connects and receives shell output.
- [ ] `E2E-157 (P1)` terminal input executes command and output returns.
- [ ] `E2E-158 (P1)` terminal resize event is accepted and does not break session.
- [ ] `E2E-159 (P2)` terminal socket close is handled gracefully in UI.

## R) Missing UI Sample Reporting Pipeline

- [ ] `E2E-160 (P1)` unknown parsed event triggers `/ui/missing-samples` POST.
- [ ] `E2E-161 (P1)` malformed historical event triggers missing sample report.
- [ ] `E2E-162 (P2)` missing sample payload includes project/scope/session metadata.

## S) Usage and Stats

- [ ] `E2E-163 (P1)` `/api/usage` data renders snapshots in usage dropdown.
- [ ] `E2E-164 (P1)` usage dropdown handles API error payload and displays degraded state.
- [ ] `E2E-165 (P1)` profile stats endpoint contributes to top-bar aggregate usage.
- [ ] `E2E-166 (P1)` loop stats endpoint data is reachable and non-breaking.

## T) Deep Linking, Browser History, and Persistence

- [ ] `E2E-167 (P0)` selected issue/doc/plan/turn/scope deep links restore correctly on refresh.
- [ ] `E2E-168 (P0)` back/forward browser navigation restores prior view state.
- [ ] `E2E-169 (P1)` switching project resets incompatible selections and avoids stale detail panels.
- [ ] `E2E-170 (P1)` chat selection deep link survives refresh and loads same chat.

## U) Error and Empty-State Resilience

- [ ] `E2E-171 (P0)` empty loops view shows actionable empty state.
- [ ] `E2E-172 (P0)` empty standalone chats view shows new-chat CTA.
- [ ] `E2E-173 (P1)` API 500 on view load does not crash app.
- [ ] `E2E-174 (P1)` API 404 on optional resources is handled as empty state.
- [ ] `E2E-175 (P1)` auth-required errors open auth modal only when token missing.
- [ ] `E2E-176 (P2)` large event payloads render without layout breakage.

## V) Real Agent CLI Runs (No Replay, Full End-to-End)

- [ ] `E2E-177 (ENV,P0)` start ask session with real `codex` profile and verify streamed response.
- [ ] `E2E-178 (ENV,P0)` start ask session with real `claude` profile and verify streamed response.
- [ ] `E2E-179 (ENV,P1)` start ask session with real `gemini` profile and verify streamed response.
- [ ] `E2E-180 (ENV,P1)` start ask session with real `opencode` profile and verify streamed response.
- [ ] `E2E-181 (ENV,P1)` start ask session with real `vibe` profile and verify streamed response.
- [ ] `E2E-182 (ENV,P1)` run real loop with 2+ steps and verify loop transitions in UI.
- [ ] `E2E-183 (ENV,P1)` run team/delegation loop and verify spawn lifecycle and summaries.
- [ ] `E2E-184 (ENV,P2)` run standalone chat with real profile and verify message persistence.

## W) Security and Boundary Cases

- [ ] `E2E-185 (P1)` API rejects malformed IDs (`/sessions/abc`, `/issues/0`) with proper errors.
- [ ] `E2E-186 (P1)` project-scoped endpoint denies access to session from other project.
- [ ] `E2E-187 (P1)` CORS headers are present and correct for web app usage.
- [ ] `E2E-188 (P2)` invalid websocket payload from server is safely handled in client.

## X) Performance and Stability Soak

- [ ] `E2E-189 (P2)` rapid view switching 100x does not leak errors or freeze UI.
- [ ] `E2E-190 (P2)` replaying large multi-provider event histories remains responsive.
- [ ] `E2E-191 (P2)` repeated create/delete chat cycles do not leave stale entries.
- [ ] `E2E-192 (P2)` repeated project switch cycles do not cross-contaminate data.
- [ ] `E2E-193 (P2)` websocket reconnect loop under intermittent failures remains stable.

## Y) Responsive and UX Baseline

- [ ] `E2E-194 (P1)` app is usable at desktop viewport.
- [ ] `E2E-195 (P1)` app is usable at mobile viewport.
- [ ] `E2E-196 (P2)` no critical text overlap/truncation regressions in top bar and left panel.
- [ ] `E2E-197 (P2)` modal dialogs are keyboard dismissible and do not trap incorrectly.

## Z) Final Regression Gates

- [ ] `E2E-198 (P0)` full core suite passes on clean machine with no external agent CLIs.
- [ ] `E2E-199 (ENV,P1)` full live-agent suite passes in environment with configured CLIs.
- [ ] `E2E-200 (P0)` CI artifacts include traces/videos for failing tests only.

