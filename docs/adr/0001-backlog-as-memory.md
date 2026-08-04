# Backlog as memory, not accumulating chat sessions

BeanBot originally held a single global `*genai.Chat` shared across every guild, channel and user, guarded by one mutex. We replaced it with no retained conversation state at all: on each Trigger, BeanBot fetches the recent messages of that channel from Discord and sends them as context.

## Considered Options

The obvious fix to the global session was to key it per channel — `map[channelID]*genai.Chat`. We rejected it because it does not solve the actual problem. BeanBot only ever receives messages that address it, so an accumulating session contains *only* BeanBot-directed turns and never the surrounding conversation. "Read previous messages to get more context" is unachievable that way. Fetching the Backlog is the only option that sees conversation BeanBot was not part of.

A hybrid (per-channel session seeded from the Backlog when cold) was rejected for maintaining two histories that can disagree.

## Consequences

- Restarts and redeploys cost nothing — there is no state to lose. Fly.io runs a single machine that restarts on deploy.
- No unbounded memory growth and no cross-channel bleed, both of which the global session had.
- `!bbreset` no longer means anything and was removed. To make BeanBot forget, delete the messages.
- Token cost moves from "grows with session age" to "flat per request, proportional to Backlog size".
- Requires the privileged `MESSAGE CONTENT` intent. Without it Discord blanks the `content` field and the Backlog arrives empty.
- Per-member state (e.g. individual timezones) has nowhere to live. This is why there is a single Server Timezone.

If someone later reintroduces a persistent chat session, they will reintroduce the cross-channel bleed and the amnesia-on-restart that this decision removed.
