# Memory as a per-Guild markdown file on a Fly volume

BeanBot keeps a markdown document per Guild on a mounted Fly volume, writes to it through a `remember` Capability the model calls, and injects the whole document into every Trigger in that Guild. This partly reverses ADR 0001: BeanBot now holds state of its own, and it is state that a restart can lose.

## Considered Options

**Where the bytes live.** Discord itself was the obvious candidate and the one consistent with ADR 0001 — a pinned message the bot edits, or a re-uploaded `.md` attachment. Both survive losing the machine entirely and can be read and corrected by a member from their phone, which the volume cannot. They were rejected for the 2000-character message cap and for putting a CDN round-trip in front of every read. A git-backed file was rejected for putting a network write in the path of every change.

The volume's costs are accepted knowingly: Memory is bound to one machine in one region, there is no backup without snapshots, and because a volume binds to a single machine, deploys are now brief downtime rather than rolling.

**How writes happen.** The model submits a *change* — section, entry, and optionally the entry it supersedes — rather than a rewritten document. Returning the whole document is simpler in Go, but it retypes every fact through the model on every write, so paraphrase drift stops being an occasional cost of Compaction and becomes a certainty of each write. It also overwrites from a snapshot read seconds earlier, silently erasing anything a concurrent Trigger recorded. Go merges the change against the file as it is now, under a per-Guild lock.

**What happens at the ceiling.** Refusing the write once the document is full was rejected in favour of Compaction, accepting that a model rewrite can quietly reword a fact. Compaction runs *without* the Backstory — pointing a persona whose job is a distinctive voice at the one operation whose success criterion is fidelity produces a Memory rewritten to be funnier.

Nobody waits on Compaction, and that requires two things rather than one. It is detached from the Trigger, because `respond` defers stopping the typing indicator and would otherwise hold it up through a model call the member has already been answered by. It also takes the Guild's lock *twice* — once to read, once to write — rather than holding it across the model call, because a held lock would stall any member who asked BeanBot to remember something meanwhile. The consequence is that the document may change while the model is thinking, so the write re-reads and abandons a compaction that has gone stale; the next Trigger finds the Memory still oversized and tries again.

**Removal.** There is no `forget` Capability. Corrections happen through `remember`'s supersede argument, and everything else is left to Compaction.

## Consequences

- Memory is ungated: any member can cause a write, because a Memory only moderators may teach is a mod-maintained wiki rather than BeanBot learning from the server. Since supersede can overwrite, deletion is effectively ungated too.
- That makes the document member-authored text injected into every later prompt — attacker-writable Backstory. It is therefore rendered in the *user* content, fenced and labelled as fallible notes, never in `SystemInstruction`. It cannot reach a Gate, which reads live Discord permissions in Go and never consults the model. The residual exposure is that BeanBot can be taught to say false things, and that `generate_image` is ungated, so a memory entry is a durable way to make it draw — a per-turn image budget is the fix, and is not part of this decision — tracked as issue #1.
- Every entry carries the date and the member who prompted it, which is the only thing that makes the file auditable given ungated writes.
- `Mutating()` now means *Guild-mutating* specifically. Memory writes are excluded from the one-mutation-per-Trigger budget: they cost no API call and change nothing in Discord, and sharing the budget would break "make the event and remember we do this weekly".
- `BEANBOT_MEMORY_DIR` unset disables the feature entirely and `remember` is not declared to the model. Set but unmounted is fatal at startup, and BeanBot deliberately does not create the directory: `MkdirAll` would succeed against the container's ephemeral layer when the volume failed to attach, producing Memory that works perfectly until the next deploy empties it.
- Compaction fires rarely by design, which makes it the least-exercised path in the system despite being the one that rewrites the only copy. `BEANBOT_MEMORY_LIMIT` exists so it can be forced to run locally.
