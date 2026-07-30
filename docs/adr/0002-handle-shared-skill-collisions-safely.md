---
status: superseded by ADR-0022
---

# Handle shared skill collisions safely

When an external skill claims a Matty-reserved name, Matty continues activating
only if it can deterministically reject or ignore the external definition while
reporting the Shared Skill Collision. If Pi cannot guarantee Matty's ownership
of the name, Matty activates none of the shared workflow, leaves Pi usable, and
retains actionable diagnostics. This favors a clearly unavailable workflow
over a partially active or ambiguously overridden one. Phase 0 must prove that
the wrong content cannot reach the model before Matty blocks activation; load
order is not evidence. If Pi `0.83.0` cannot satisfy this Activation Safety
Gate, Matty v1 is not published until Pi gains the necessary capability or the
activation design is deliberately revised.

T02 tested Pi `0.83.0` at commit
`845d6ff1f6643aba440341cce877ce1c43ebbc39`. Pi exposes the winning skill
command and its source information through `pi.getCommands()` once the session
is bound, so direct input can be rejected before `/skill:name` expansion. It
does not expose collision diagnostics or a public skill removal API.

That input gate is not a complete model boundary. An extension command runs
before `input` and can call `pi.sendMessage(..., { triggerTurn: true })`, which
bypasses both `input` and `before_agent_start`. `before_provider_request` is
also not a universal backstop: a custom `ProviderConfig.streamSimple` receives
the unsanitized context and is not required to invoke the optional `onPayload`
callback. Pi's own custom Anthropic provider example follows that path.
Consequently, a loaded external reserved-skill description and location can
reach a model through supported public APIs.

The Activation Safety Gate therefore fails on Pi `0.83.0`. This is a
release-blocking result, not an implementation to approximate in production:
Matty v1 remains unpublished and no partial collision gate is shipped. The
block can be removed only when Pi provides a universal pre-model interception
point, deterministic skill ownership/removal, or the activation design is
deliberately revised with equivalent safety evidence.
