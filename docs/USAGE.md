# Poindexter (dex) Usage Guide

This guide explains how to use Poindexter for AI-assisted development.

## Concepts

### The Hat System

Poindexter uses a "hat" system where AI sessions take on different roles:

| Hat | Purpose | Output |
|-----|---------|--------|
| **Planner** | Breaks down epics into tasks | Task list with dependencies |
| **Architect** | Designs interfaces and patterns | Architecture docs, interfaces |
| **Implementer** | Writes the actual code | Source code changes |
| **Reviewer** | Reviews code quality | Review comments, approvals |
| **Tester** | Writes and runs tests | Test files, coverage reports |
| **Debugger** | Fixes failing tests/bugs | Bug fixes |
| **Documenter** | Writes documentation | README, docs (terminal) |
| **DevOps** | Deploys to cloud | Deployed services (terminal) |
| **Conflict Manager** | Resolves merge conflicts | Clean merges (terminal) |

Sessions automatically transition between hats based on work completed.

### Task States

```
pending → ready → running → completed
            ↓         ↓
         blocked   paused/quarantined
```

- **pending**: Created but waiting for dependencies
- **ready**: All dependencies met, can be started
- **blocked**: Waiting on other tasks
- **running**: Active AI session working on it
- **paused**: User paused the session
- **quarantined**: Hit budget limit, needs approval
- **completed**: Work finished, PR created
- **cancelled**: User cancelled

### Worktrees

Each task gets its own git worktree - an isolated copy of the repo on a dedicated branch. This allows:
- Multiple tasks to run in parallel
- Clean separation of changes
- Easy PR creation per task

## Workflows

### Basic Workflow: Single Task

1. **Create a task**
   ```
   Title: Add user logout button
   Description: Add a logout button to the header that clears the session
   Hat: implementer
   ```

2. **Start the task**
   - Click "Start" or call `POST /api/v1/tasks/{id}/start`
   - A worktree is created on branch `task/task-{id}`
   - An AI session begins with the implementer hat

3. **Monitor progress**
   - Watch real-time updates via WebSocket
   - Check iteration count and token usage
   - View session logs

4. **Review the PR**
   - When complete, a PR is auto-created
   - Review the changes
   - Merge or request changes

### Advanced Workflow: Epic Decomposition

For larger features, use the planner hat:

1. **Create an epic task**
   ```
   Title: User authentication system
   Description: Complete auth with login, logout, sessions, password reset
   Type: epic
   Hat: planner
   ```

2. **The planner creates subtasks**
   - Breaks down the epic into smaller tasks
   - Sets dependencies between tasks
   - Assigns appropriate hats to each

3. **Run subtasks**
   - Tasks with no blockers become "ready"
   - Start them individually or let the scheduler auto-run
   - Each completes and unblocks the next

4. **Merge incrementally**
   - Each subtask creates its own PR
   - Merge as they complete
   - Final integration when all done

### Approval Workflow

Certain actions require your approval:

- **Hat transitions**: When an AI session wants to change roles
- **Budget exceeded**: When token/time limits are hit
- **PR creation**: Before pushing to GitHub
- **Merge conflicts**: When changes conflict with main

Approvals appear in the UI and can be:
- **Approved**: Continue with the action
- **Rejected**: Stop or try alternative

## API Usage

### Authentication

```bash
# 1. Get a challenge
CHALLENGE=$(curl -s http://localhost:8080/api/v1/auth/challenge | jq -r .challenge)

# 2. Sign with your passphrase (done in UI/client)
# Returns JWT token

# 3. Use token for all requests
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/tasks
```

### Task Operations

```bash
# List all tasks
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/tasks

# Create a task
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Add dark mode",
    "description": "Implement dark mode toggle with system preference detection",
    "type": "feature",
    "hat": "architect",
    "priority": 2
  }' \
  http://localhost:8080/api/v1/tasks

# Start a task (creates worktree, begins session)
curl -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/tasks/{id}/start

# Get task status
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/tasks/{id}
```

### WebSocket Events

Connect to `ws://localhost:8080/api/v1/ws` for real-time updates:

```javascript
const ws = new WebSocket('ws://localhost:8080/api/v1/ws');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data.type, data.payload);
};

// Events you'll receive:
// - task:started
// - task:progress
// - task:completed
// - session:iteration
// - approval:required
// - error
```

### Service Status

```bash
# Check all services
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/toolbelt/status

# Test service connections
curl -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/toolbelt/test
```

## Best Practices

### Writing Good Task Descriptions

**Good:**
```
Add a logout button to the main navigation header.
When clicked, it should:
1. Call POST /api/v1/auth/logout
2. Clear the JWT from localStorage
3. Redirect to the login page
4. Show a "Logged out successfully" toast

The button should be visible only when authenticated.
Use the existing Button component from src/components/ui.
```

**Bad:**
```
Add logout
```

### Setting Budgets

Configure budgets to prevent runaway sessions:

| Budget Type | Recommended | Use Case |
|-------------|-------------|----------|
| Tokens | 50,000-100,000 | Most tasks |
| Time (min) | 30-60 | Standard features |
| Dollars | $1-5 | Cost control |
| Iterations | 20-50 | Exploration tasks |

### Choosing the Right Hat

| Task Type | Start With | Why |
|-----------|------------|-----|
| New feature | architect | Design first, then implement |
| Bug fix | debugger | Jump straight to fixing |
| Refactor | architect | Plan the structure changes |
| Documentation | documenter | Direct to docs |
| Deployment | devops | Direct to deployment |
| Large epic | planner | Break it down first |

### Parallel Tasks

You can run up to 25 tasks in parallel. For best results:
- Ensure tasks don't modify the same files
- Use separate worktrees (automatic)
- Set appropriate priorities
- Monitor resource usage

## Monitoring

### Session Logs

View what the AI is doing:
- Real-time in the UI
- Via `GET /api/v1/tasks/{id}/logs`
- In the session checkpoints

### Resource Usage

Track consumption:
- Token count per session
- Estimated cost
- Time spent
- Iteration count

### Health Checks

```bash
# System status
curl http://localhost:8080/api/v1/system/status

# Database health
sqlite3 dex.db "PRAGMA integrity_check;"
```

## Troubleshooting

### Task Stuck in "Running"

1. Check if session hit budget limit (quarantined)
2. Check for pending approvals
3. View session logs for errors
4. Manually pause and resume

### PR Not Created

1. Verify GitHub token has `repo` scope
2. Check if branch was pushed
3. Look for errors in session logs
4. Ensure base branch exists

### Hat Transition Rejected

1. Check the transition is valid (see hat diagram)
2. Review the approval request details
3. Provide feedback to guide the AI

### Merge Conflicts

1. Conflict Manager hat can help
2. Alternatively, resolve manually:
   ```bash
   cd worktrees/project/task-xxx
   git fetch origin
   git merge origin/main
   # resolve conflicts
   git commit
   ```

## Keyboard Shortcuts (UI)

| Shortcut | Action |
|----------|--------|
| `n` | New task |
| `s` | Start selected task |
| `p` | Pause/resume task |
| `a` | Open approvals |
| `/` | Focus search |
| `?` | Show help |

## Tips

1. **Start small**: Test with a simple task before complex epics
2. **Be specific**: Detailed descriptions get better results
3. **Review early**: Check first iterations to catch issues
4. **Use approvals**: Don't auto-approve everything blindly
5. **Set budgets**: Prevent unexpected costs
6. **Clean up**: Delete old worktrees to save disk space
