# Page Structure

## Home

**Function:**
- See all quests (active, then completed)
- Create new quest
- Know if there are items in inbox needing attention
- Access inbox
- Access all objectives (secondary)

**API Endpoints:**
- `GET /api/v1/projects/:id/quests` - list quests for a project
- `POST /api/v1/projects/:id/quests` - create new quest
- `GET /api/v1/approvals` - get count of pending approvals (for inbox indicator)

**Websocket Events:**
- `quest.created` - new quest added
- `quest.completed` - quest status changed
- `quest.deleted` - quest removed
- `approval.required` - new item in inbox
- `approval.resolved` - item handled

**Gaps/Notes:**
- Uses default project (`proj_default`) - single-project mode ✓
- May want a lightweight "inbox count" endpoint rather than fetching all approvals

---

## Quest Detail

**Function:**
- Chat with AI to plan work
- See objectives proposed, accept or reject them
- See all objectives belonging to this quest and their status
- Navigate to any objective

**API Endpoints:**
- `GET /api/v1/quests/:id` - get quest details and conversation history
- `POST /api/v1/quests/:id/messages` - send message in conversation
- `POST /api/v1/quests/:id/objectives` - accept/create objective from draft
- `POST /api/v1/quests/:id/objectives/batch` - batch accept multiple drafts
- `GET /api/v1/quests/:id/tasks` - get objectives created from this quest
- `POST /api/v1/quests/:id/complete` - mark quest complete
- `POST /api/v1/quests/:id/reopen` - reopen quest
- `DELETE /api/v1/quests/:id` - delete quest

**Websocket Events:**
- `quest.message` - new message (user or assistant)
- `quest.objective_draft` - AI proposes an objective
- `quest.question` - AI asks a clarifying question
- `quest.ready` - quest finished planning
- `quest.tool_call` - AI is using a tool
- `quest.tool_result` - tool execution finished
- `task.created` - objective was created from this quest
- `task.updated` - objective status changed

**Gaps/Notes:**
- None identified - well covered

---

## Objective Detail

**Function:**
- See objective info (title, description, status)
- Live checklist that updates as items complete
- Activity stream showing what's happening
- Control execution (start, pause, cancel)
- See results when done
- Navigate back to parent quest

**API Endpoints:**
- `GET /api/v1/tasks/:id` - get objective details
- `GET /api/v1/tasks/:id/checklist` - get checklist items
- `GET /api/v1/tasks/:id/activity` - get activity history
- `POST /api/v1/tasks/:id/start` - start execution
- `POST /api/v1/tasks/:id/pause` - pause execution
- `POST /api/v1/tasks/:id/resume` - resume execution
- `POST /api/v1/tasks/:id/cancel` - cancel execution

**Websocket Events:**
- `task.updated` - status changed
- `task.paused` - execution paused
- `task.resumed` - execution resumed
- `task.cancelled` - execution cancelled
- `task.completed` - execution finished
- `checklist.updated` - checklist item status changed
- `session.started` - execution session began
- `session.iteration` - progress update (tokens, iteration count)
- `session.completed` - session finished

**Gaps/Notes:**
- Task has `quest_id` field - can navigate back to parent quest ✓
- Activity endpoint exists, session websocket events provide real-time updates ✓

---

## Inbox

**Function:**
- See all items needing attention (approvals now, other types later)
- Each item shows what it is, what it relates to
- Take action on items (approve/reject for approvals)
- Navigate to related quest or objective for context

**API Endpoints:**
- `GET /api/v1/approvals` - list pending approvals
- `GET /api/v1/approvals/:id` - get approval details
- `POST /api/v1/approvals/:id/approve` - approve
- `POST /api/v1/approvals/:id/reject` - reject

**Websocket Events:**
- `approval.required` - new approval needed
- `approval.resolved` - approval was handled

**Gaps/Notes:**
- Approvals have `task_id`, tasks have `quest_id` - can trace to quest context ✓
- Future: will need additional endpoints for messages and other inbox item types

---

## All Objectives (secondary)

**Function:**
- See every objective across all quests
- Filter by status
- See which quest each belongs to
- Navigate to any objective

**API Endpoints:**
- `GET /api/v1/tasks` - list all tasks (supports filtering)

**Websocket Events:**
- `task.created` - new objective
- `task.updated` - status changed
- `task.completed` - objective done

**Gaps/Notes:**
- Tasks have `quest_id` field for grouping/display ✓
- Status filtering supported via `?status=` query param ✓
- Project filtering supported via `?project_id=` query param ✓
