# V2 UI Design & UX Review

## Comparison with Goose

After analyzing Goose's chat implementation, several gaps and opportunities for improvement are evident.

---

## Critical Issues

### 1. ~~No Streaming Support~~ ✅ DONE
**Fixed:** Backend now uses `ChatWithStreaming()` which broadcasts `quest.content_delta` WebSocket events.
Frontend handles these in QuestDetail.tsx and updates `streamingContent` state for real-time rendering.

### 2. Tool Activity Not Inline
**Current:** Tool activity shows in a separate section below all messages.
**Goose:** Tool calls are rendered inline within the assistant message that triggered them, with collapsible details.
**Fix:** Tool calls should be embedded in the message flow, associated with the message that initiated them.

### 3. No Error Recovery
**Current:** If a message fails to send, `setSending(false)` is called but no UI feedback.
**Goose:** Has retry functionality, error states on messages, interruption handling.
**Fix:** Add error state to messages, show inline retry button, handle connection loss gracefully.

### 4. Missing Keyboard Navigation
**Current:** Only Enter/Shift+Enter in input.
**Design spec requires:** `j/k` navigation, `g h` to go home, `Esc` to go back, `?` for shortcuts.
**Fix:** Implement keyboard navigation system as specified in design doc.

---

## UX Polish Issues

### 5. Message Animation
**Current:** Messages use CSS `animation: fade-in-up` but it's subtle and may not be applied consistently.
**Goose:** Uses `animate-[appear_150ms_ease-in_forwards]` with initial opacity 0.
**Fix:** Ensure animation applies to each new message, consider staggering for multiple.

### 6. Scroll Behavior
**Current:** `scrollToBottom()` always scrolls on new messages/streaming.
**Design spec:** Should NOT auto-scroll if user has scrolled up. Show "↓ New messages" indicator.
**Goose:** Has sophisticated scroll management.
**Fix:** Track scroll position, only auto-scroll if near bottom, add new messages indicator.

### 7. Input Polish
**Current:** Basic textarea with cursor decoration.
**Goose:** Has file attachments, image paste, voice dictation, mentions, command history.
**Fix (minimum):**
- Add `↑` key to recall last message
- Focus ring on the container, not just textarea
- Better disabled state (dim the whole area)

### 8. Loading States
**Current:** Generic "Loading..." text.
**Goose:** Has `LoadingGoose` component with personality.
**Fix:** Create a minimal loading indicator consistent with design (e.g., `▓░░` block spinner as specified).

### 9. Objectives List
**Current:** Always visible in header, takes space.
**Design spec:** Should be collapsible.
**Fix:** Add collapse/expand toggle, remember state.

### 10. Thinking Indicator
**Current:** Just a blinking cursor in an empty message bubble.
**Goose:** Has visible thinking state, can show chain-of-thought in collapsible.
**Fix:** Consider adding subtle pulsing dots or more visible "thinking" state.

---

## Missing Features from Design Spec

### 11. "New Messages" Scroll Indicator
**Spec:** "If user has scrolled up, don't auto-scroll — show '↓ New messages' indicator"
**Status:** Not implemented.

### 12. Message Failed State
**Spec:** "✗ Failed to send · [Retry]" below failed message content.
**Status:** Not implemented.

### 13. Connection Lost State
**Spec:** Input placeholder changes to "Reconnecting...", send disabled.
**Status:** Not implemented. We have `connected` from useWebSocket but don't use it in QuestDetail.

### 14. Message Copy
**Goose:** Has `MessageCopyLink` component.
**Spec:** Not explicitly required but adds polish.
**Consider:** Add copy button on hover for assistant messages.

### 15. Cancel/Interrupt
**Goose:** Has stop button during generation, interruption handling.
**Spec:** Not explicit but important for long-running operations.
**Fix:** Add stop button when `sending` is true.

---

## Component-Specific Issues

### ChatInput.tsx

```
Issues:
- No command history (↑ to recall)
- No focus management when disabled state changes
- Send button text - consider icon instead for cleaner look
- No visual container focus state
- Placeholder change when disabled is good but could be more graceful
```

### Message.tsx

```
Issues:
- No hover state for copy/actions
- Timestamp always visible - consider showing on hover like Goose
- No distinction between temporary (optimistic) and confirmed messages
- No support for images/attachments in content
```

### ProposedObjective.tsx

```
Issues:
- No loading state on Accept/Reject buttons
- Should show which items are optional vs must-have
- No animation when accepted/rejected
```

### QuestionPrompt.tsx

```
Issues:
- Selected state sends immediately - no confirmation
- Consider keyboard navigation (1/2/3 to select options)
- No custom answer support (design mentions "Can also type a custom response in main input")
```

### ToolActivity.tsx

```
Issues:
- Not associated with specific messages
- Should collapse completed tools after delay
- Spinner is custom CSS but could use design system spinner
```

---

## Style/CSS Issues

### v2.css

```
Issues:
- Scanline opacity (0.02) may be too subtle to notice
- Some hardcoded values instead of CSS variables
- Missing responsive breakpoints
- No dark/light mode toggle (only dark)
- Button hover states use JS instead of CSS (in Button.tsx)
```

### Missing from Design System

```
- Focus ring utility class
- Skeleton loader for lists
- Toast/notification system (for errors)
- Modal component (for confirmations)
```

---

## Recommended Priority Fixes

### P0 - Must Have for Usable Chat
1. ~~Streaming content support~~ ✅
2. Error handling with retry
3. Connection status awareness
4. Stop/cancel button

### P1 - Important for Polish
5. Scroll behavior with new message indicator
6. Tool calls inline with messages
7. Loading states on buttons
8. Message animations

### P2 - Nice to Have
9. Keyboard navigation
10. Command history (↑ key)
11. Collapsible objectives
12. Copy message button

---

## Code Quality Notes

### Good
- Clean component separation
- Consistent use of CSS custom properties
- TypeScript throughout
- Proper cleanup in useEffect

### Needs Improvement
- Inline styles in JSX (move to CSS)
- Some components could be smaller/more focused
- Missing error boundaries
- No loading skeletons
- Tests not present

---

## Action Items

1. [x] Implement streaming content WebSocket handling (quest.content_delta event)
2. [x] Add connection status to chat UI
3. [x] Add error state to messages with retry
4. [x] Add stop button during generation
5. [x] Fix scroll behavior (don't scroll if user scrolled up)
6. [x] Add "new messages" indicator (ScrollIndicator component)
7. [x] Move tool activity inline with messages (Message.tsx toolCalls prop)
8. [x] Add keyboard navigation system (useKeyboardNavigation hook)
9. [x] Polish animations and transitions (added CSS for streaming, error states)
10. [x] Add loading states to all interactive elements (ProposedObjective has accepting/rejecting states)

## Completed Fixes (This Session)

### Components Updated
- **ChatInput.tsx**: Added stop button, command history (↑/↓ keys), connection status awareness, proper placeholder changes
- **Message.tsx**: Added error states, retry button, copy button on hover, inline tool calls support
- **ProposedObjective.tsx**: Added loading states (accepting/rejecting), must-have/optional distinction
- **ScrollIndicator.tsx**: New component for "↓ New messages" indicator
- **KeyboardShortcuts.tsx**: New modal showing all available keyboard shortcuts

### Hooks Added
- **useKeyboardNavigation.ts**: Vim-style keyboard navigation with `g h`, `g i`, `g o`, `j/k`, `?`, `Esc`, `G`, `gg`

### Pages Updated
- **QuestDetail.tsx**: Integrated stop button, connection status, scroll indicator, keyboard shortcuts modal, message error/retry handling, command history
- **Home.tsx**: Added keyboard navigation for quest list selection, keyboard shortcuts modal
- **Inbox.tsx**: Added keyboard navigation with `a` to approve, `r` to reject selected item, keyboard shortcuts modal

### CSS Added
- Message error states (.v2-message--error, .v2-message__error, .v2-message__retry)
- Message streaming state (.v2-message--streaming)
- Message header actions (.v2-message__header-right, .v2-message__action)
- Inline tool calls (.v2-message__tools)
- Chat input enhancements (.v2-chat-input--disconnected, .v2-chat-input__stop)
- Scroll indicator (.v2-scroll-indicator)
- Modal system (.v2-modal-overlay, .v2-modal, .v2-modal__header, etc.)
- Keyboard shortcuts styles (.v2-shortcuts-section, .v2-shortcut, etc.)
- Proposed objective sections (.v2-proposed__section, .v2-proposed__section-label)
- Timestamp dimmed state (.v2-timestamp--dimmed)
- Connection status indicator (.v2-connection-status)

## Remaining Work

### Backend Required
1. ~~Streaming content support~~ ✅ (`quest.content_delta` WebSocket events implemented)
2. Quest session cancel endpoint (POST /quests/:id/cancel)

### Future Enhancements
- Collapsible objectives list in QuestDetail header
- Custom loading indicator with design system spinner
- Toast/notification system for errors
- Responsive breakpoints
- Light mode support
