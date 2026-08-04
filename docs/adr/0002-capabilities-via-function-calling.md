# Capabilities via function calling, not slash commands

BeanBot takes real actions in the guild — creating Guild Events, generating Images — by declaring each Capability to Gemini as a callable tool and letting the model decide what to invoke from natural language. It registers no Discord slash commands.

## Considered Options

Slash commands are the conventional answer, and a Discord bot with none is surprising enough to warrant this record. They give typed arguments, native permission scoping and no possibility of hallucinated input. We rejected them because BeanBot's whole premise is being *talked to* in character; `/event name:… when:…` is a different product. Requiring members to learn a command surface undoes the reason for putting a language model in front of it.

Registering both was rejected as two surfaces to maintain over the same Go functions before we know anyone wants the second.

## Consequences

- Authorization cannot be delegated to Discord's command permissions, because there are no commands. Every Capability carries its own Gate, checked in Go against the requesting member's permissions. **The Gate must never live in the Backstory** — a system instruction is advisory, and a language model can be talked out of it.
- Arguments arrive from a model rather than a validated form, so each Capability validates its own input. Timestamps in particular are checked for parseability and for being in the future.
- The model has no clock, so the current time and the Server Timezone are injected into every request. Without this it confabulates dates for anything relative.
- A turn is a loop (call → execute → feed result → call again), which is what lets Capabilities compose. It is bounded at 5 round-trips and at most one guild-mutating Capability per turn, so neither API spend nor the guild's event list is left to the model's discretion.
- Adding a Capability is one new type in the registry — no command registration, no Discord-side deployment step.
