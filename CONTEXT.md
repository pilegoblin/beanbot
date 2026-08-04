# BeanBot

A Discord bot with a persona, driven by Gemini. Members talk to it in a channel; it answers in character and can take real actions in the guild on request.

## Language

**Trigger**:
The single Discord message that wakes BeanBot — by naming it, @mentioning it, or replying to something it said. BeanBot never speaks uninvited.
_Avoid_: Command, invocation, prompt

**Backlog**:
The recent messages of the channel a Trigger arrived in, fetched from Discord at request time. BeanBot holds no conversation state of its own — Discord is the memory.
_Avoid_: History, context window, session, transcript

**Backstory**:
The persona instruction that defines who BeanBot is. Supplied per-deployment, not authored in the repo.
_Avoid_: System prompt, personality, character

**Capability**:
Something BeanBot can do beyond talking — creating a Guild Event, generating an Image. Declared to the model as a callable tool and executed in Go.
_Avoid_: Function, action, command, skill

**Gate**:
The Go-side permission check a Capability passes before it executes, derived from the Discord permissions of the member who sent the Trigger. Lives in code, never in the Backstory, so it cannot be talked around.
_Avoid_: Guard, auth check, permission

**Guild Event**:
A Discord scheduled event BeanBot creates on request. Gated on the requesting member holding `MANAGE_EVENTS`.
_Avoid_: Meeting, calendar entry, party

**Nano Banana**:
Gemini 2.5 Flash Image, the model BeanBot uses to generate and edit images. Explicitly *not* Nano Banana Pro (Gemini 3 Pro Image), which is a different, costlier model.
_Avoid_: The image model, Imagen

**Server Timezone**:
The single configured IANA timezone every relative time in a Trigger is resolved against. BeanBot has no per-member timezone.
_Avoid_: Local time, user timezone
