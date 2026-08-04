# BeanBot

A Discord bot with a persona, driven by Gemini. Members talk to it in a channel; it answers in character and can take real actions in the guild on request.

## Language

**Trigger**:
The single Discord message that wakes BeanBot — by naming it, @mentioning it, or replying to something it said. BeanBot never speaks uninvited.
_Avoid_: Command, invocation, prompt

**Alias**:
Another name a Person is known by, recorded alongside their canonical name so that whatever the channel actually calls them still finds them.
_Avoid_: Nickname, aka, synonym, handle

**Backlog**:
The recent messages of the channel a Trigger arrived in, fetched from Discord at request time. BeanBot keeps no conversation state of its own; what it retains beyond the Backlog is Memory, and that holds facts rather than conversation.
_Avoid_: History, context window, session, transcript

**Backstory**:
The persona instruction that defines who BeanBot is. Supplied per-deployment, not authored in the repo.
_Avoid_: System prompt, personality, character

**Capability**:
Something BeanBot can do beyond talking — creating a Guild Event, generating an Image, speaking a Clip, recording a Memory. Declared to the model as a callable tool and executed in Go.
_Avoid_: Function, action, command, skill

**Clip**:
A short piece of spoken audio BeanBot generates and posts. Made by Gemini 2.5 Flash TTS — the cheapest of the three text-to-speech models, explicitly *not* the 3.1 Flash or 2.5 Pro ones, which both cost double. The model returns headerless PCM; what reaches Discord is a WAV.
_Avoid_: Audio, sound file, voice note, TTS, recording

**Compaction**:
The rewriting of an oversized Memory into a shorter one that keeps the same facts. Performed by the model without the Backstory, so the result is faithful rather than in character. It never touches the Roster: a rewrite that shortens is a rewrite that forgets.
_Avoid_: Summarising, pruning, garbage collection

**Configured Timezone**:
The single IANA timezone every relative time in a Trigger is resolved against, set once for the whole deployment. BeanBot has no per-member or per-Guild timezone.
_Avoid_: Server timezone, local time, user timezone

**Gate**:
The Go-side permission check a Capability passes before it executes, derived from the Discord permissions of the member who sent the Trigger. Lives in code, never in the Backstory, so it cannot be talked around.
_Avoid_: Guard, auth check, permission

**Guild Event**:
A Discord scheduled event BeanBot creates on request. Gated on the requesting member holding `MANAGE_EVENTS`.
_Avoid_: Meeting, calendar entry, party

**Guild-mutating**:
Of a Capability: that it changes something visible in the Guild. At most one runs per Trigger. Recording a Memory is a change but not a Guild-mutating one.
_Avoid_: Mutating, write, side effect

**In Play**:
Of a Person: named in the Backlog of the current Trigger, or present in it as a speaker, and therefore carried into the prompt in full rather than by name alone.
_Avoid_: Relevant, active, loaded, retrieved

**Medium**:
The kind of file a Capability produces — an Image or a Clip. At most one Capability of each Medium runs per Trigger, which is what bounds spending: they are the only Capabilities that cost money per call. Independent of Guild-mutating, so "draw the poster and make the event" spends one of each rather than making them compete.
_Avoid_: Format, file type, quota

**Memory**:
What BeanBot knows about one Guild for longer than a conversation — a markdown document, one per Guild, that it writes itself and reads back on every Trigger. It holds attributed facts about the Guild alongside the Roster of the People it has heard of.
_Avoid_: Notes, knowledge base, long-term context, profile

**Merge**:
The folding of one Person into another once both turn out to be the same human. Every fact moves across and the absorbed name survives as an Alias, so a Merge loses nothing.
_Avoid_: Dedupe, link, combine

**Name Index**:
The bare list of the People who are not In Play, carried into every Trigger alongside those who are. It is what lets BeanBot know who he knows when the details are not in front of him.
_Avoid_: Table of contents, directory, summary

**Nano Banana**:
Gemini 2.5 Flash Image, the model BeanBot uses to generate and edit images. Explicitly *not* Nano Banana Pro (Gemini 3 Pro Image), which is a different, costlier model.
_Avoid_: The image model, Imagen

**Person**:
A human BeanBot has learned something about — a member of the Guild, or someone who is only ever talked about. Created the first time there is a fact to attach, never by contact alone.
_Avoid_: Profile, contact, user, entity

**Roster**:
Every Person in one Guild. The part of Memory BeanBot never discards from, never Compacts, and reads back selectively rather than whole.
_Avoid_: List, directory, address book, people section

**Voice**:
One of six prebuilt Gemini voices a Clip may be spoken in, each carrying the characteristic that says what it sounds like. Chosen per Clip by the model, not per deployment: BeanBot does impressions rather than having a voice of its own. Six of the thirty Gemini offers, because the choice is declared on every Trigger whether or not anyone wants a Clip.
_Avoid_: Speaker, accent, tone, persona
