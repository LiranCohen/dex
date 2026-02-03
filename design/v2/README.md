# Dex v2 UI Design

## Design Philosophy

Retro-futuristic. Clean, 70s design sensibilities filtered through a modern lens. Subtle undercurrent of terminal aesthetics — the quiet confidence of someone who knows their way around a command line.

- **Flat** — no gradients, minimal shadows, geometric shapes
- **Restrained** — generous whitespace, nothing competing for attention
- **Muted** — intentional color palette, not colorful, not gray
- **Typography-driven** — type does the heavy lifting
- **Calm** — organized, quietly confident, doesn't try too hard
- **Technical** — hints of the machine underneath, without being loud about it

## Subtle Terminal Aesthetic

Woven in quietly — never the focus, but present for those who notice.

### Typography Cues
- Timestamps, IDs, and technical metadata in monospace
- Activity feed uses mono font — feels like watching logs
- Status labels in uppercase mono: `RUNNING`, `PENDING`
- Subtle use of technical notation: `3/5` not "3 of 5"

### Visual Details
- Blinking cursor in input fields (block cursor, not line)
- Faint scanline texture on backgrounds (barely visible, 2-3% opacity)
- Grid alignment feels intentional, almost like a terminal grid
- Status indicators pulse subtly when active (slow, 2s cycle)
- Activity items appear with a brief typewriter-style fade-in

### Decorative Elements
- Section dividers can use ASCII-inspired patterns: `─ ─ ─` or `━━━`
- Empty states might show: `// no items` or `[ empty ]`
- Loading states: `...` or a minimal block spinner `▓░░`
- Timestamps in 24h format: `14:32:07`

### Color Accents
- Occasional use of terminal green (`#7dad7a`) for "success" or "active"
- Error states hint at red but stay muted — not alarming
- Data/technical elements slightly dimmer than content text

### What This Is Not
- No neon
- No glowing effects
- No matrix rain
- No "hacker movie" clichés
- No animated backgrounds

It's the difference between a movie hacker and someone who actually uses vim daily.

---

## Design System

### Colors

```css
/* Background */
--bg-primary: #1a1a1a;      /* Main background - warm black */
--bg-secondary: #242424;    /* Cards, elevated surfaces */
--bg-tertiary: #2e2e2e;     /* Hover states, subtle emphasis */

/* Text */
--text-primary: #e8e4df;    /* Primary text - warm white */
--text-secondary: #9a9590;  /* Secondary, muted text */
--text-tertiary: #5c5854;   /* Disabled, hints */

/* Accent */
--accent-primary: #d4a574;  /* Warm amber - primary actions */
--accent-muted: #8b7355;    /* Muted amber - secondary */

/* Status */
--status-active: #7dad7a;   /* Muted green - running/active */
--status-pending: #c9b896;  /* Warm tan - waiting */
--status-complete: #6b8f71; /* Sage - done */
--status-error: #c47c6c;    /* Muted coral - error/failed */

/* Borders */
--border-subtle: #333333;   /* Subtle dividers */
--border-emphasis: #444444; /* More visible borders */
```

### Typography

```css
/* Font stack - clean, geometric sans-serif */
--font-sans: 'Inter', 'SF Pro Display', system-ui, sans-serif;
--font-mono: 'JetBrains Mono', 'SF Mono', monospace;

/* Scale */
--text-xs: 0.75rem;    /* 12px - labels, metadata */
--text-sm: 0.875rem;   /* 14px - secondary content */
--text-base: 1rem;     /* 16px - body text */
--text-lg: 1.125rem;   /* 18px - emphasis */
--text-xl: 1.5rem;     /* 24px - section headers */
--text-2xl: 2rem;      /* 32px - page titles */

/* Weight */
--font-normal: 400;
--font-medium: 500;
--font-semibold: 600;
```

### Spacing

```css
/* Base unit: 4px */
--space-1: 0.25rem;   /* 4px */
--space-2: 0.5rem;    /* 8px */
--space-3: 0.75rem;   /* 12px */
--space-4: 1rem;      /* 16px */
--space-6: 1.5rem;    /* 24px */
--space-8: 2rem;      /* 32px */
--space-12: 3rem;     /* 48px */
--space-16: 4rem;     /* 64px */
```

### Components

#### Buttons

```
Primary:   bg-accent-primary, text-bg-primary, no border
Secondary: bg-transparent, text-primary, border-emphasis
Ghost:     bg-transparent, text-secondary, no border, hover: text-primary
```

No rounded corners. Sharp rectangles. Minimal padding.

#### Cards

```
bg-secondary, border-subtle (1px), no shadow, no radius
Padding: space-4 or space-6
```

#### Inputs

```
bg-tertiary, border-subtle, text-primary
No radius. Clean lines.
Focus: border-accent-primary
```

#### Status Indicators

Small rectangles (not circles), 3px × 12px vertical bars:
```
Running:   status-active
Pending:   status-pending
Complete:  status-complete
Error:     status-error
```

---

## Page Layouts

### Global Structure

```
┌─────────────────────────────────────────────────────┐
│ HEADER                                              │
│ [Logo/Name]                    [Inbox] [Objectives] │
├─────────────────────────────────────────────────────┤
│                                                     │
│                                                     │
│                    CONTENT                          │
│                                                     │
│                                                     │
│                                                     │
└─────────────────────────────────────────────────────┘
```

- Header: Fixed, minimal height (48-56px)
- Content: Centered, max-width 960px
- No sidebar

### Home (Quest List)

```
┌─────────────────────────────────────────────────────┐
│ DEX                            [●2 Inbox] [All Obj] │
├─────────────────────────────────────────────────────┤
│                                                     │
│  QUESTS                            [+ New Quest]    │
│                                                     │
│  ┌───────────────────────────────────────────────┐  │
│  │▌ Quest Title                                  │  │
│  │  3/5 objectives · 2 running                   │  │
│  └───────────────────────────────────────────────┘  │
│                                                     │
│  ┌───────────────────────────────────────────────┐  │
│  │▌ Another Quest                                │  │
│  │  1/3 objectives · idle                        │  │
│  └───────────────────────────────────────────────┘  │
│                                                     │
│  ─ ─ ─ ─ ─ ─ ─ COMPLETED ─ ─ ─ ─ ─ ─ ─            │
│                                                     │
│  ┌───────────────────────────────────────────────┐  │
│  │  Finished Quest                               │  │
│  │  5/5 objectives                               │  │
│  └───────────────────────────────────────────────┘  │
│                                                     │
└─────────────────────────────────────────────────────┘
```

- Quest cards: Full width, stacked vertically
- Status bar on left edge indicates activity
- Completed quests visually de-emphasized (lighter text)
- Section divider for completed (subtle, text-based)

### Quest Detail

```
┌─────────────────────────────────────────────────────┐
│ ← Back                         [●2 Inbox] [All Obj] │
├─────────────────────────────────────────────────────┤
│                                                     │
│  QUEST TITLE                                        │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│                                                     │
│  ┌─ OBJECTIVES ─────────────────────────────────┐   │
│  │ ▌ Objective one                    running   │   │
│  │ ▌ Objective two                    complete  │   │
│  │   Objective three                  pending   │   │
│  └──────────────────────────────────────────────┘   │
│                                                     │
│  ┌─ CONVERSATION ───────────────────────────────┐   │
│  │                                              │   │
│  │  You: Can you help me refactor the auth?     │   │
│  │                                              │   │
│  │  Dex: I'll create objectives for this...     │   │
│  │                                              │
│  │  ┌─ PROPOSED OBJECTIVE ──────────────────┐   │   │
│  │  │ Title: Refactor auth middleware       │   │   │
│  │  │ □ Update token validation             │   │   │
│  │  │ □ Add refresh token support           │   │   │
│  │  │                    [Reject] [Accept]  │   │   │
│  │  └───────────────────────────────────────┘   │   │
│  │                                              │   │
│  ├──────────────────────────────────────────────┤   │
│  │ Type a message...                     [Send] │   │
│  └──────────────────────────────────────────────┘   │
│                                                     │
└─────────────────────────────────────────────────────┘
```

- Two sections: Objectives list (collapsible) + Conversation
- Objectives shown inline, clickable to detail
- Proposed objectives appear in conversation flow
- Clean message bubbles, no avatars needed

### Objective Detail

```
┌─────────────────────────────────────────────────────┐
│ ← Quest Name                   [●2 Inbox] [All Obj] │
├─────────────────────────────────────────────────────┤
│                                                     │
│  OBJECTIVE TITLE                          [Pause]   │
│  Status: Running                                    │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│                                                     │
│  ┌─ CHECKLIST ──────────────────────────────────┐   │
│  │ ✓ Set up project structure                   │   │
│  │ ✓ Create database schema                     │   │
│  │ ◯ Implement API endpoints        ← current   │   │
│  │ ◯ Write tests                                │   │
│  │ ◯ Update documentation                       │   │
│  └──────────────────────────────────────────────┘   │
│                                                     │
│  ┌─ ACTIVITY ───────────────────────────────────┐   │
│  │ 14:32  Reading src/api/handlers.go           │   │
│  │ 14:32  Editing src/api/handlers.go           │   │
│  │ 14:31  Running go test ./...                 │   │
│  │ 14:30  Created new file src/api/auth.go      │   │
│  │ ...                                          │   │
│  └──────────────────────────────────────────────┘   │
│                                                     │
└─────────────────────────────────────────────────────┘
```

- Checklist is primary focus — shows progress
- Items check off in real-time
- Activity feed below, scrolls, timestamps on left
- Control button (Pause/Resume/Cancel) top right

### Inbox

```
┌─────────────────────────────────────────────────────┐
│ ← Back                                   [All Obj]  │
├─────────────────────────────────────────────────────┤
│                                                     │
│  INBOX                                              │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│                                                     │
│  ┌───────────────────────────────────────────────┐  │
│  │ APPROVAL · Commit                             │  │
│  │ "Add user authentication middleware"          │  │
│  │ Quest: Auth Refactor → Objective: Auth setup  │  │
│  │                           [Reject] [Approve]  │  │
│  └───────────────────────────────────────────────┘  │
│                                                     │
│  ┌───────────────────────────────────────────────┐  │
│  │ APPROVAL · Pull Request                       │  │
│  │ "feat: implement OAuth2 flow"                 │  │
│  │ Quest: Auth Refactor → Objective: OAuth       │  │
│  │                           [Reject] [Approve]  │  │
│  └───────────────────────────────────────────────┘  │
│                                                     │
│  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─   │
│  Nothing else needs attention                       │
│                                                     │
└─────────────────────────────────────────────────────┘
```

- Card per item
- Type label (APPROVAL, MESSAGE, etc.) prominent
- Context trail: Quest → Objective
- Actions inline on card
- Empty state: simple text, not illustrated

---

## Chat Interface (Quest Conversation)

The chat is the heart of the app. This is where work gets planned. It must feel responsive, intelligent, and effortless.

### Layout

```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│                     CONVERSATION                        │
│                     (scrollable)                        │
│                                                         │
│  ┌────────────────────────────────────────────────┐     │
│  │ You                                   14:32    │     │
│  │ Can you help me refactor the authentication    │     │
│  │ system? It's getting messy.                    │     │
│  └────────────────────────────────────────────────┘     │
│                                                         │
│  ┌────────────────────────────────────────────────┐     │
│  │ Dex                                   14:32    │     │
│  │ I'll look at the current implementation.       │     │
│  │                                                │     │
│  │ ┌──────────────────────────────────────────┐   │     │
│  │ │ ◐ Reading src/auth/middleware.go         │   │     │
│  │ └──────────────────────────────────────────┘   │     │
│  └────────────────────────────────────────────────┘     │
│                                                         │
│  ┌────────────────────────────────────────────────┐     │
│  │ Dex                                   14:33    │     │
│  │ I see a few issues. The token validation is    │     │
│  │ duplicated across three files, and there's no  │     │
│  │ refresh token handling.                        │     │
│  │                                                │     │
│  │ I'd suggest breaking this into objectives:     │     │
│  │                                                │     │
│  │ ┌─ PROPOSED ─────────────────────────────┐     │     │
│  │ │                                        │     │     │
│  │ │ Consolidate token validation           │     │     │
│  │ │ ─────────────────────────────────────  │     │     │
│  │ │ Extract token validation into a single │     │     │
│  │ │ middleware and remove duplicates.      │     │     │
│  │ │                                        │     │     │
│  │ │ ☐ Create TokenValidator middleware     │     │     │
│  │ │ ☐ Update all routes to use it          │     │     │
│  │ │ ☐ Remove duplicate validation code     │     │     │
│  │ │ ☐ Add tests                            │     │     │
│  │ │                                        │     │     │
│  │ │              [Reject]  [Accept]        │     │     │
│  │ └────────────────────────────────────────┘     │     │
│  └────────────────────────────────────────────────┘     │
│                                                         │
│  ┌────────────────────────────────────────────────┐     │
│  │ Dex                                   14:33    │     │
│  │ Before I create the second objective, I need   │     │
│  │ to know:                                       │     │
│  │                                                │     │
│  │ ┌─ QUESTION ─────────────────────────────┐     │     │
│  │ │                                        │     │     │
│  │ │ How should refresh tokens be stored?   │     │     │
│  │ │                                        │     │     │
│  │ │ ┌────────────────────────────────────┐ │     │     │
│  │ │ │ Database                           │ │     │     │
│  │ │ │ Store in PostgreSQL with expiry    │ │     │     │
│  │ │ └────────────────────────────────────┘ │     │     │
│  │ │ ┌────────────────────────────────────┐ │     │     │
│  │ │ │ Redis                              │ │     │     │
│  │ │ │ Fast lookup, automatic expiration  │ │     │     │
│  │ │ └────────────────────────────────────┘ │     │     │
│  │ │ ┌────────────────────────────────────┐ │     │     │
│  │ │ │ HTTP-only cookies                  │ │     │     │
│  │ │ │ Stateless, client-side storage     │ │     │     │
│  │ │ └────────────────────────────────────┘ │     │     │
│  │ │                                        │     │     │
│  │ └────────────────────────────────────────┘     │     │
│  └────────────────────────────────────────────────┘     │
│                                                         │
│  ▊                                                      │
│                                                         │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ▌Type a message...                            [Send]   │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### Message Anatomy

**User Messages**
```
┌────────────────────────────────────────────────────────┐
│ You                                           14:32    │
│                                                        │
│ Message content goes here. Can be multiple lines.      │
│ Supports **markdown** and `code`.                      │
└────────────────────────────────────────────────────────┘
```
- Left-aligned
- Label "You" in text-secondary, mono, uppercase small
- Timestamp right-aligned, text-tertiary, mono
- Content in text-primary
- Background: bg-tertiary (subtle distinction)
- Full width, no max-width constraint
- Spacing: space-4 padding, space-3 gap between messages

**Assistant Messages**
```
┌────────────────────────────────────────────────────────┐
│ Dex                                           14:32    │
│                                                        │
│ Response content. Often longer, may contain embedded   │
│ components like tool activity, proposed objectives,    │
│ or questions.                                          │
└────────────────────────────────────────────────────────┘
```
- Same structure as user
- Background: bg-secondary (or transparent, depending on nesting)
- May contain embedded components (see below)

### Embedded Components

**Tool Activity** (inline, during response)
```
┌──────────────────────────────────────────────────────┐
│ ◐ Reading src/auth/middleware.go                     │
└──────────────────────────────────────────────────────┘
```
- Compact single line
- Spinning indicator (◐ ◓ ◑ ◒) or static when complete (✓ or ·)
- Monospace font
- Background: bg-primary (inset feel)
- Border: border-subtle
- Appears inline within message flow
- Multiple tools stack vertically
- Completed tools can collapse or dim

**Tool Activity States**
```
◐ Reading file...          (running - spinner)
· Read src/auth/main.go    (complete - dimmed)
✗ Failed to read file      (error - status-error color)
```

**Proposed Objective**
```
┌─ PROPOSED ──────────────────────────────────────────┐
│                                                     │
│  Objective Title                                    │
│  ───────────────────────────────────────────────    │
│  Description text explaining what this objective    │
│  will accomplish. Keep it concise.                  │
│                                                     │
│  ☐ Checklist item one                               │
│  ☐ Checklist item two                               │
│  ☐ Checklist item three                             │
│                                                     │
│                        [Reject]  [Accept]           │
│                                                     │
└─────────────────────────────────────────────────────┘
```
- Distinct border treatment: border-accent-muted or double-line
- Label "PROPOSED" in mono, uppercase, accent color
- Title in text-lg, font-medium
- Divider below title (thin line)
- Description in text-secondary
- Checklist items with empty checkbox (☐)
- Buttons right-aligned at bottom
- Accept = primary button, Reject = ghost button
- Once accepted: card transforms to show "✓ ACCEPTED" state, buttons removed
- Once rejected: card dims or collapses with "✗ Rejected" note

**Question Prompt**
```
┌─ QUESTION ──────────────────────────────────────────┐
│                                                     │
│  How should refresh tokens be stored?               │
│                                                     │
│  ┌────────────────────────────────────────────────┐ │
│  │ Database                                       │ │
│  │ Store in PostgreSQL with automatic expiry      │ │
│  └────────────────────────────────────────────────┘ │
│                                                     │
│  ┌────────────────────────────────────────────────┐ │
│  │ Redis                                          │ │
│  │ Fast lookup with TTL-based expiration          │ │
│  └────────────────────────────────────────────────┘ │
│                                                     │
│  ┌────────────────────────────────────────────────┐ │
│  │ HTTP-only cookies                              │ │
│  │ Stateless, stored client-side                  │ │
│  └────────────────────────────────────────────────┘ │
│                                                     │
└─────────────────────────────────────────────────────┘
```
- Label "QUESTION" in mono, uppercase
- Question text prominent
- Options as selectable cards (not radio buttons)
- Option cards: bg-tertiary, border-subtle
- Hover: border-accent-muted
- Selected: border-accent-primary, subtle bg change
- Click sends the response automatically
- Can also type a custom response in main input

### Streaming & Typing

**While Dex is responding:**
```
┌────────────────────────────────────────────────────────┐
│ Dex                                                    │
│                                                        │
│ I'll analyze the current auth implementation to        │
│ understand the structure▊                              │
└────────────────────────────────────────────────────────┘
```
- Text streams in character by character (or chunk by chunk)
- Block cursor (▊) at end, blinking
- Cursor disappears when response complete
- No "typing..." indicator — the streaming text IS the indicator

**While Dex is thinking (before first token):**
```
┌────────────────────────────────────────────────────────┐
│ Dex                                                    │
│                                                        │
│ ▊                                                      │
└────────────────────────────────────────────────────────┘
```
- Just the blinking cursor
- Clean, no "thinking..." text

### Input Area

```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│  ▌Type a message...                            [Send]   │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Anatomy:**
- Block cursor indicator on left edge (▌) — static, decorative
- Placeholder text in text-tertiary
- Input expands vertically as you type (up to ~6 lines, then scroll)
- Send button right-aligned, text-only or minimal icon
- Send enabled only when input has content

**States:**
```
Empty:      ▌Type a message...                    [Send]
                                                  (dimmed)

Typing:     ▌Refactor the auth system to use     [Send]
            JWT tokens with refresh capability    (active)

Disabled:   ▌...                                  [Send]
            (while Dex is responding)             (dimmed)
```

**Keyboard:**
- `Enter` — send message
- `Shift+Enter` — new line
- `↑` (when empty) — edit last message (optional, power user)
- `Esc` — blur input, return to keyboard navigation

### Scroll Behavior

- New messages appear at bottom
- Auto-scroll to bottom when new content arrives (if already near bottom)
- If user has scrolled up, don't auto-scroll — show "↓ New messages" indicator
- Clicking indicator scrolls to bottom and dismisses
- `G` keyboard shortcut jumps to bottom
- `g g` jumps to top
- Smooth scroll, not instant

**New message indicator:**
```
                    ┌─────────────────────┐
                    │  ↓ New messages     │
                    └─────────────────────┘
```
- Fixed position near bottom of chat area
- Small, unobtrusive
- Click or press `G` to dismiss and scroll

### Empty State

When starting a new quest:
```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│                                                         │
│                                                         │
│                                                         │
│           Start by describing what you want             │
│                    to accomplish.                       │
│                                                         │
│                                                         │
│                                                         │
│                                                         │
├─────────────────────────────────────────────────────────┤
│  ▌Type a message...                            [Send]   │
└─────────────────────────────────────────────────────────┘
```
- Centered, text-secondary
- Simple, instructional
- No illustration, no emoji

### Error States

**Message failed to send:**
```
┌────────────────────────────────────────────────────────┐
│ You                                           14:32    │
│                                                        │
│ Can you help me with the auth refactor?                │
│                                                        │
│ ✗ Failed to send · [Retry]                             │
└────────────────────────────────────────────────────────┘
```
- Error note below message content
- Retry link inline
- Message stays in place (not removed)

**Connection lost:**
```
┌─────────────────────────────────────────────────────────┐
│  ▌Reconnecting...                              [Send]   │
│                                                (dimmed) │
└─────────────────────────────────────────────────────────┘
```
- Input placeholder changes
- Send disabled
- Recovers automatically when connection restored

### Micro-interactions

- **Message appear:** Fade in + subtle slide up (100ms)
- **Tool activity spinner:** Smooth rotation, not stepped
- **Button hover:** Background appears (not color change)
- **Option select:** Border color transition (150ms)
- **Cursor blink:** 1s interval, 50% duty cycle
- **Scroll:** Smooth, ~300ms duration
- **Streaming text:** No animation per-character, just appears

### Accessibility

- All interactive elements focusable via Tab
- Enter activates buttons and options
- Screen reader announces new messages
- High contrast between text and backgrounds
- Minimum 44px touch targets for mobile

### Responsive Behavior

**Desktop (>768px):**
- Full layout as shown
- Keyboard shortcuts active

**Tablet/Mobile (<768px):**
- Input area fixed to bottom (keyboard-aware)
- Messages take full width
- Embedded components stack naturally
- Keyboard shortcuts hidden (no physical keyboard)

---

### All Objectives (Secondary)

```
┌─────────────────────────────────────────────────────┐
│ ← Back                                    [Inbox]   │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ALL OBJECTIVES                                     │
│  [All ▼] [Running] [Pending] [Complete] [Failed]   │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│                                                     │
│  Quest: Auth Refactor                               │
│  ┌───────────────────────────────────────────────┐  │
│  │▌ Auth middleware setup                running │  │
│  │▌ OAuth implementation                 pending │  │
│  └───────────────────────────────────────────────┘  │
│                                                     │
│  Quest: Database Migration                          │
│  ┌───────────────────────────────────────────────┐  │
│  │  Schema updates                      complete │  │
│  │  Data migration                      complete │  │
│  └───────────────────────────────────────────────┘  │
│                                                     │
└─────────────────────────────────────────────────────┘
```

- Filter tabs at top (not dropdown)
- Grouped by quest
- Compact rows, status on right
- Click through to objective detail

---

## File Structure

```
frontend/src/
├── v2/
│   ├── App.tsx                 # Router + layout shell
│   ├── styles/
│   │   ├── tokens.css          # CSS custom properties
│   │   └── base.css            # Reset + global styles
│   ├── components/
│   │   ├── Header.tsx
│   │   ├── Button.tsx
│   │   ├── Card.tsx
│   │   ├── StatusBar.tsx
│   │   ├── Input.tsx
│   │   └── ...
│   ├── pages/
│   │   ├── Home.tsx            # Quest list
│   │   ├── QuestDetail.tsx
│   │   ├── ObjectiveDetail.tsx
│   │   ├── Inbox.tsx
│   │   └── AllObjectives.tsx
│   └── hooks/
│       └── (reuse existing)
```

---

## Interaction Patterns

### Navigation
- Back button always present (except Home)
- Breadcrumb-style context in back button ("← Quest Name")
- No nested navigation — flat hierarchy

### Loading States
- Subtle, inline spinners (not full-page)
- Skeleton loaders for lists (simple rectangles)
- No bouncing/pulsing — static or linear animation

### Real-time Updates
- Items slide in/update in place
- Checklist items animate check (subtle fade)
- No toasts — updates happen inline

### Empty States
- Simple text, no illustrations
- "No quests yet" / "Nothing needs attention"
- Primary action button if applicable

---

## Keyboard Navigation (Vim-inspired)

For power users. Not advertised, not documented prominently. Discoverable by those who try.

### Global
```
g h     → go home (quest list)
g i     → go inbox
g o     → go all objectives
?       → show keyboard shortcuts (subtle modal)
/       → focus search/filter (when available)
Esc     → close modal, cancel, go back
```

### Quest List (Home)
```
j / k   → move selection down / up
Enter   → open selected quest
c       → create new quest
```

### Quest Detail
```
j / k   → scroll conversation
g g     → jump to top
G       → jump to bottom
o       → focus objectives list
Tab     → cycle between objectives list and conversation
```

### Objective Detail
```
j / k   → scroll activity feed
g g     → jump to top of activity
G       → jump to latest activity
p       → pause/resume objective
x       → cancel objective (with confirmation)
```

### Inbox
```
j / k   → move between items
a       → approve selected
r       → reject selected
Enter   → view details / expand
```

### Implementation Notes
- No visual indication of current selection until user presses j/k
- Selection shown with subtle left border highlight
- Commands that modify state (a, r, x) require confirmation or are undoable
- Escape always returns to "normal" state
- No insert mode needed — inputs capture naturally on focus

---

## What to Avoid

- Rounded corners (use sharp or barely rounded: 2px max)
- Drop shadows
- Gradients
- Colorful icons
- Bouncy animations
- Friendly/playful copy
- Illustrations or mascots
- Dense information — let it breathe
