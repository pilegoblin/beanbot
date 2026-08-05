# BeanBot has no voice, so it borrows one at random

ADR 0005 had the model choose a Voice per Clip, on the reasoning that the delivery should suit the line. The Voice is now drawn at random in Go and the `voice` parameter is gone from the declaration. BeanBot is a bean computer: it has no gender, no vocal cords and no voice of its own, so there is nothing for a choice to express — every Clip is borrowed from somebody, and which somebody is nobody's decision.

## Considered Options

**Whose choice it is.** ADR 0005 rejected a per-deployment Voice in favour of the model picking, and described the result as BeanBot "doing impressions rather than having a voice of its own". That was the right observation and the wrong conclusion drawn from it. If BeanBot has no voice of its own, then no particular voice is more its own than any other, and a model deliberating over which one to use is deliberating about nothing — it is picking a costume for something with no body. Randomness says the same thing more honestly and is funnier for saying it: the surprise of not knowing who it will sound like *is* the characterisation.

Keeping the model's pick as an option — random by default, overridden when the request calls for a particular sound — was rejected as random in name only. A model handed an optional parameter fills it in, so the default would almost never be reached, and the outcome would be ADR 0005's behaviour with extra machinery.

**What is actually lost.** Less than it looks. The `style` direction is untouched and is prefixed onto the script the speech model reads, so "slowly and menacingly", "barely holding back laughter" and "in a thick Scottish accent" all still work — delivery stays directable, and accents were never the Voice's job in the first place. What can no longer be targeted is timbre: "say it in a deep gravelly voice" now comes back in whatever voice was drawn, possibly a youthful one. That mismatch is the cost, and it is also the joke.

**How wide the catalogue.** All thirty. ADR 0007 had just widened the shortlist from six to twenty-two, dropping eight voices whose characteristics duplicated others — Kore, Orus and Alnilam are all labelled "firm" — on the grounds that a model picking between identically-labelled options is picking blind. That reasoning dies with the model's involvement. A random index does not read labels, and three voices called "firm" do not sound alike merely because one word describes all three. Every voice dropped for being an indistinguishable *option* is a perfectly distinguishable *sound*, and leaving one out now just means nobody ever hears it.

The `Characteristic` field survives the removal of everything that read it, because the live smoke test logs it and a human listening to thirty clips wants to know which one was supposed to be gravelly.

## Consequences

- The `voice` parameter is gone from `generate_speech`, and with it `VoiceMenu`, `VoiceNames` and `KnownVoice` — all three existed only to describe the choice to the model or to validate what came back. A `voice` argument the model sends anyway is ignored rather than refused: there is no wrong answer to a question nobody asked.
- The declaration shrinks by the whole voice menu, which was its largest part. Combined with ADR 0007 this is a tool that is both rarely declared and much smaller when it is.
- **ADR 0007's shortlist figure of 22 is superseded.** Its other conclusion — that Cueing narrowed ADR 0005's token argument for keeping the list short — still stands, and is why widening was on the table at all.
- The smoke test speaks in every voice on the list, so it grows from 22 live TTS calls to 30.
- Nothing is seeded, so Clips are not reproducible. The tests assert that draws are spread across the catalogue and always name a real voice, rather than pinning a sequence.
- A server hears no consistent voice from BeanBot, ever. ADR 0005 already gave this up when it rejected a per-deployment Voice; this makes it total, and deliberate.
