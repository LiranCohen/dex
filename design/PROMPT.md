# Home Page Design Brief

## What is this app?

Dex is a task automation tool. Users chat with an AI to plan work ("Quests"), which generates actionable items ("Objectives") that get executed automatically. When critical actions need human approval, they appear in an "Inbox."

## The Home Page

This is the main entry point. It's quest-centric — quests are the primary way users interact with the system.

### What users need to do here:

- See all their quests at a glance (active ones prominently, completed ones accessible)
- Start a new quest
- Know if anything needs their attention in the inbox (and get there)
- Access a deeper view of all objectives (secondary, not prominent)

### Data available for each quest:

- Title
- Status (active, completed)
- Number of objectives created from it
- How many objectives are complete vs total (e.g., "3/5")
- Whether any objectives are currently running

### Where users go from here:

- Click a quest → Quest Detail (chat interface + objectives list)
- Click "new quest" → New Quest (starts a fresh chat)
- Click inbox indicator → Inbox (list of items needing attention)
- Click "all objectives" → Full objectives list (secondary path)

### Inbox indicator context:

- Shows count of pending items (approvals for now, more types later)
- Should be noticeable but not distracting when there are items
- Should be ignorable when empty

## Design Direction

**Aesthetic:** Retro-futuristic. Think clean, 70s design sensibilities filtered through a modern lens.

**Style:**
- Flat design — no gradients, minimal shadows, simple geometric shapes
- Clean and restrained — generous whitespace, nothing competing for attention
- Muted, intentional color palette — not colorful, not gray, deliberate
- Typography does the heavy lifting

**Feel:**
- Calm, not busy
- Organized, almost clinical
- Quietly confident — the interface doesn't try too hard

**Avoid:**
- Rounded, bubbly, "friendly" SaaS aesthetics
- Shadows and depth effects
- Lots of colors or visual noise
- Skeuomorphism

## Tech context (if helpful)

- React frontend
- Real-time updates via websocket (quests and inbox update live)
- Tailwind CSS available
