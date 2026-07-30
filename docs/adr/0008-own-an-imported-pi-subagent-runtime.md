# Own an imported Pi subagent runtime

Matty imports the process-launch and JSONL-result behavior of the official
subagent extension example from Pi `0.83.0` at commit
`845d6ff1f6643aba440341cce877ce1c43ebbc39`, then treats the adaptation as
Matty-owned source. The upstream source is MIT licensed.

The first imported slice owns one child Pi lifecycle. It preserves the example's
separate Pi process while adding invariants the example does not provide:

- an explicit parent provider, model, reasoning level, canonical working
  directory, and child-safe authentication environment;
- a Matty-generated run ID confirmed by Pi's JSONL session header alongside the
  real child PID;
- structured success, failure, and cancellation outcomes with ordered progress;
- SIGTERM followed by SIGKILL only while the child remains open; and
- strict JSONL/session validation instead of silently dropping malformed data.

The executable process remains local-substitutable inside the runtime so focused
tests can use a controlled child executable, while packed-package acceptance
must run the real Pi `0.83.0` binary. The validating throwaway prototype is
recorded at Matty commit `1093dd1` on
`prototype/t03-independent-subagent-runtime`.

Matty does not treat the upstream example as a stable runtime API and accepts
responsibility for maintaining this implementation. Child processes inherit the
active session's provider, model, authentication, and reasoning configuration;
Matty roles may later vary prompts and allowed tools but do not silently switch
models or require a second provider account.

Bounded Concurrency remains a later layer: eight tasks accepted per call and four
child processes active simultaneously, with overflow reported as queued. Agent
discovery, trust, diagnostics, Delegation Contracts, and Single Writer
enforcement also remain outside this one-child runtime.
