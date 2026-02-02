# Quest Chat UI/UX Implementation

Research-backed plan for implementing a high-quality chat interface for Quest conversations.

## Overview

Quest chat is a **planning/research conversation** interface where users interact with Dex to:
- Discuss project goals and requirements
- Research approaches (via web search, file exploration)
- Break work into objectives (tasks)
- Monitor and manage spawned objectives

Unlike task execution (which has its own activity stream), Quest chat is conversational with lightweight tool use for research.

## Current Backend Contracts

### Data Models

**QuestMessage** (`internal/db/models.go:383-390`)
```go
type QuestMessage struct {
    ID        string
    QuestID   string
    Role      string           // "user" | "assistant"
    Content   string           // Markdown content
    ToolCalls []QuestToolCall  // Tool calls made during this message
    CreatedAt time.Time
}

type QuestToolCall struct {
    ToolName   string         `json:"tool_name"`
    Input      map[string]any `json:"input"`
    Output     string         `json:"output"`
    IsError    bool           `json:"is_error"`
    DurationMs int64          `json:"duration_ms"`
}
```

### WebSocket Events

| Event Type | Payload | When |
|------------|---------|------|
| `quest.message` | `{quest_id, message: {id, role, content, tool_calls, created_at}}` | New message (user or assistant) |
| `quest.tool_call` | `{quest_id, tool_name, status: "running"}` | Tool execution starts |
| `quest.tool_result` | `{quest_id, tool_name, output, is_error, duration_ms}` | Tool execution completes |
| `quest.objective_draft` | `{quest_id, draft: ObjectiveDraft}` | Assistant proposes an objective |
| `quest.question` | `{quest_id, question: Question}` | Assistant asks clarifying question |
| `quest.ready` | `{quest_id, drafts, summary}` | All objectives ready for user review |
| `task.created` | `{task_id, quest_id, title, auto_start}` | Objective accepted and created |

### Structured Signals (parsed from assistant response)

**ObjectiveDraft** (`internal/quest/handler.go:29-40`)
```go
type ObjectiveDraft struct {
    DraftID             string    `json:"draft_id"`
    Title               string    `json:"title"`
    Description         string    `json:"description"`
    Hat                 string    `json:"hat"`
    Checklist           Checklist `json:"checklist"`
    BlockedBy           []string  `json:"blocked_by,omitempty"`
    AutoStart           bool      `json:"auto_start"`
    Complexity          string    `json:"complexity,omitempty"`
    EstimatedIterations int       `json:"estimated_iterations,omitempty"`
    EstimatedBudget     float64   `json:"estimated_budget,omitempty"`
}
```

**Question** (`internal/quest/handler.go:48-52`)
```go
type Question struct {
    DraftID  string   `json:"draft_id,omitempty"`  // Links to a draft if asking about it
    Question string   `json:"question"`
    Options  []string `json:"options,omitempty"`   // Clickable choices
}
```

### Available Tools (research-only)

From `internal/tools/sets.go` - ReadOnlyTools:
- `read_file` - Read file contents
- `list_files` - List directory contents
- `glob` - Find files by pattern
- `grep` - Search file contents
- `git_status` / `git_diff` / `git_log` - Git operations
- `web_search` - Search the web
- `web_fetch` - Fetch and read a URL
- `list_runtimes` - List available language runtimes

Quest-specific tools (`internal/quest/tools.go`):
- `list_objectives` - List objectives for this quest
- `get_objective_details` - Get detailed objective info
- `cancel_objective` - Cancel a stuck/failed objective

---

## What We're Borrowing

### From assistant-ui (Architecture Patterns)

**Why:** Clean composable primitives, Radix-based accessibility, battle-tested patterns.

| Pattern | Source | Adaptation |
|---------|--------|------------|
| Auto-resize input | `ComposerPrimitiveInput` | Use `react-textarea-autosize` |
| Submit on Enter | `submitOnEnter` prop | Default true |
| Cancel on Escape | `cancelOnEscape` prop | Clear draft |
| Scroll anchoring | `ThreadViewport` | Anchor to last user message when assistant responds |
| Hover state tracking | `MessageRoot.useIsHoveringRef` | For action buttons |
| Focus management | `unstable_focusOnRunStart` | Auto-focus after assistant finishes |

**Key Libraries:**
- `react-textarea-autosize` - Growing input
- `@radix-ui/react-*` - Accessibility primitives (if using React)

### From Goose (Implementation Details)

**Why:** Production-tested markdown rendering, good performance patterns.

| Component | Source File | What to Borrow |
|-----------|-------------|----------------|
| Markdown rendering | `MarkdownContent.tsx` | Memoization, code block handling |
| Syntax highlighting | `MarkdownContent.tsx` | Prism setup with custom theme |
| Copy button UX | `CodeBlock` component | 2-second feedback, disabled while streaming |
| Thinking display | `GooseMessage.tsx` | Collapsible `<details>` for chain-of-thought |
| Tool call grouping | `toolCallChaining.ts` | Hide timestamps for consecutive tools |
| Large code detection | `CodeBlock` | Log warning at 10KB threshold |

**Key Libraries:**
- `react-markdown` + `remark-gfm` - Markdown parsing
- `react-syntax-highlighter` + `prism` - Code highlighting
- `rehype-katex` + `remark-math` - Math rendering (optional)

### From Goose (Elicitation/Decision UI)

**Why:** Quest needs inline decision points for clarifying questions.

The `Question` signal with `options` array should render as clickable choices:

```
┌─────────────────────────────────────────────┐
│ Which database approach do you prefer?      │
│                                             │
│  ┌─────────────┐  ┌─────────────┐          │
│  │ PostgreSQL  │  │   SQLite    │          │
│  └─────────────┘  └─────────────┘          │
│  ┌─────────────┐                           │
│  │    Other    │  [________________]       │
│  └─────────────┘                           │
└─────────────────────────────────────────────┘
```

User selection becomes the next user message.

---

## Component Architecture

### Core Components

```
QuestChat/
├── QuestChatContainer.tsx     # Main container, WebSocket subscription
├── MessageList.tsx            # Scrollable message list with anchoring
├── MessageBubble.tsx          # Single message (user or assistant)
├── MarkdownContent.tsx        # Markdown + code block rendering
├── ChatInput.tsx              # Auto-resize input with submit
├── ToolActivity.tsx           # Tool call/result inline display
├── QuestionPrompt.tsx         # Inline decision/elicitation UI
├── ObjectiveDraftCard.tsx     # Proposed objective display
└── QuestHeader.tsx            # Title, model indicator, status
```

### Component Specifications

#### 1. MessageList

**Props:**
```typescript
interface MessageListProps {
  messages: QuestMessage[];
  isStreaming: boolean;
  onScrollToBottom: () => void;
}
```

**Behavior:**
- Auto-scroll when new message arrives (if already at bottom)
- Scroll anchor: when assistant message arrives, anchor to the user message above it
- Load older messages on scroll-to-top (if paginated)
- Show "Thinking..." indicator when `isStreaming` and no content yet

#### 2. MessageBubble

**Props:**
```typescript
interface MessageBubbleProps {
  message: QuestMessage;
  isStreaming?: boolean;
  onRetry?: () => void;  // For assistant messages
}
```

**Rendering:**
- User messages: Right-aligned, simple styling
- Assistant messages: Left-aligned, includes:
  - MarkdownContent for `content`
  - ToolActivity blocks for each `tool_call`
  - Copy button (appears on hover, disabled while streaming)
  - Timestamp (hidden if consecutive assistant messages)

#### 3. ToolActivity

**Props:**
```typescript
interface ToolActivityProps {
  toolCall: QuestToolCall;
  status: 'running' | 'complete' | 'error';
}
```

**Display by tool type:**

| Tool | Running State | Complete State |
|------|---------------|----------------|
| `web_search` | "Searching: *{query}*..." | Collapsible results preview |
| `web_fetch` | "Reading: *{url}*..." | Collapsible content preview |
| `read_file` | "Reading: `{path}`..." | Collapsible file preview |
| `glob` | "Finding files: *{pattern}*..." | File count + list |
| `grep` | "Searching for: *{pattern}*..." | Match count + preview |
| `list_objectives` | "Checking objectives..." | Objective summary |
| `get_objective_details` | "Loading objective..." | Status + progress |
| `cancel_objective` | "Cancelling..." | Confirmation |

**Layout:**
```
┌─ web_search ──────────────────────────────┐
│ 🔍 Searching: "AI chat UI best practices" │
│                                           │
│ ▸ 10 results found (click to expand)      │
│   └─ First 3 results preview...           │
└───────────────────────────────────────────┘
```

#### 4. QuestionPrompt

**Props:**
```typescript
interface QuestionPromptProps {
  question: Question;
  onAnswer: (answer: string) => void;
}
```

**Behavior:**
- If `options` provided: Render as clickable buttons
- Always include "Other" option with text input
- On selection: Call `onAnswer` which sends as user message
- Disable after selection (show selected state)

#### 5. ObjectiveDraftCard

**Props:**
```typescript
interface ObjectiveDraftCardProps {
  draft: ObjectiveDraft;
  onAccept: (selectedOptional: number[]) => void;
  onReject: () => void;
  onAskQuestion: (question: string) => void;
}
```

**Display:**
```
┌─────────────────────────────────────────────────┐
│ 📋 Proposed Objective                           │
├─────────────────────────────────────────────────┤
│ **Implement user authentication**               │
│                                                 │
│ Add JWT-based authentication with login/logout  │
│ endpoints and session management.               │
│                                                 │
│ Hat: creator │ Complexity: complex │ ~$0.50     │
├─────────────────────────────────────────────────┤
│ Must-have:                                      │
│ ☑ Create auth middleware                        │
│ ☑ Add login endpoint                            │
│ ☑ Add logout endpoint                           │
│                                                 │
│ Optional:                                       │
│ ☐ Add password reset flow                       │
│ ☐ Add OAuth integration                         │
├─────────────────────────────────────────────────┤
│ [Accept]  [Reject]  [Ask Question]              │
└─────────────────────────────────────────────────┘
```

#### 6. ChatInput

**Props:**
```typescript
interface ChatInputProps {
  onSubmit: (content: string) => void;
  disabled?: boolean;
  placeholder?: string;
}
```

**Behavior:**
- Auto-resize up to max height
- Enter to submit (Shift+Enter for newline)
- Escape to clear
- Disabled while assistant is responding
- Focus restored after assistant response

---

## State Management

### WebSocket Subscription

```typescript
interface QuestChatState {
  messages: QuestMessage[];
  pendingToolCalls: Map<string, {tool_name: string, status: string}>;
  drafts: ObjectiveDraft[];
  activeQuestion: Question | null;
  isStreaming: boolean;
  error: string | null;
}

// WebSocket event handlers
function handleQuestEvent(event: WebSocketMessage) {
  switch (event.type) {
    case 'quest.message':
      // Add message to list
      // If assistant, mark streaming=false
      break;
    case 'quest.tool_call':
      // Add to pendingToolCalls with status='running'
      break;
    case 'quest.tool_result':
      // Update pendingToolCalls with result
      break;
    case 'quest.objective_draft':
      // Add to drafts array
      break;
    case 'quest.question':
      // Set activeQuestion (renders QuestionPrompt)
      break;
    case 'quest.ready':
      // All drafts ready for review
      break;
  }
}
```

### Optimistic Updates

- User message: Add immediately, show pending state
- Tool calls: Show "running" state immediately from `quest.tool_call`
- Question answers: Add as user message immediately

---

## Performance Considerations

### From Goose

1. **Memoize expensive renders:**
   ```typescript
   const MemoizedCodeBlock = memo(CodeBlock);
   const MemoizedMarkdown = memo(MarkdownContent);
   ```

2. **Large code block detection:**
   ```typescript
   if (content.length > 10000) {
     console.log(`Large code block: ${content.length} chars`);
     // Consider virtualization or lazy loading
   }
   ```

3. **Disable copy while streaming:**
   ```typescript
   <CopyButton disabled={isStreaming} />
   ```

### Scroll Performance

- Use `react-virtualized` or `@tanstack/virtual` if message list grows large
- Debounce scroll-to-bottom checks
- Use `IntersectionObserver` for visibility detection

---

## Accessibility

Following assistant-ui patterns:

- **Keyboard navigation:** Tab through messages, Enter to expand tool results
- **Screen reader:** ARIA live regions for new messages
- **Focus management:** Return focus to input after action
- **High contrast:** Ensure sufficient contrast for all states

---

## Implementation Phases

### Phase 1: Core Chat (MVP)
- [x] MessageList with basic scrolling
- [x] MessageBubble for user/assistant
- [x] MarkdownContent with code highlighting
- [x] ChatInput with auto-resize
- [x] WebSocket subscription for `quest.message`

### Phase 2: Tool Activity
- [x] ToolActivity component
- [x] Handle `quest.tool_call` / `quest.tool_result`
- [x] Tool-specific visualizations (web_search, read_file, etc.)
- [x] Collapsible tool results

### Phase 3: Decision UI
- [x] QuestionPrompt component
- [x] Handle `quest.question` events
- [x] Option selection → user message flow

### Phase 4: Objective Management
- [x] ObjectiveDraftCard component (existing, kept in sidebar)
- [x] Handle `quest.objective_draft` / `quest.ready`
- [x] Accept/reject flow with optional item selection

### Phase 5: Polish
- [x] Scroll anchoring (MessageList auto-scroll)
- [x] Focus management (ChatInput auto-focus)
- [x] Copy button UX (disabled while streaming, 2s feedback)
- [x] Loading/error states (error banner, streaming indicators)
- [x] Keyboard shortcuts (Enter to send, Shift+Enter for newline, Escape to clear)

---

## Dependencies

```json
{
  "dependencies": {
    "react-markdown": "^9.0.0",
    "remark-gfm": "^4.0.0",
    "react-syntax-highlighter": "^15.5.0",
    "react-textarea-autosize": "^8.5.0"
  },
  "optionalDependencies": {
    "@radix-ui/react-primitive": "^1.0.0",
    "remark-math": "^6.0.0",
    "rehype-katex": "^7.0.0"
  }
}
```

---

## References

- [assistant-ui](https://github.com/assistant-ui/assistant-ui) - Composable React primitives
- [block/goose](https://github.com/block/goose) - MarkdownContent, CodeBlock patterns
- [Streamdown](https://streamdown.ai/docs/code-blocks) - Code block UX
- [DEV: Integrating AI Agents](https://dev.to/yahav10/integrating-ai-agents-into-modern-uis-m8) - Response normalization patterns
