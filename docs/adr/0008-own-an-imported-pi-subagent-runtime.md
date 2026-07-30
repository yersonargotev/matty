# Own an imported Pi subagent runtime

Matty initially copies the official subagent extension example from Pi `0.83.0`
at commit `845d6ff1f6643aba440341cce877ce1c43ebbc39`, then treats it as Matty-owned
source. The Subagent Runtime preserves the example's separate Pi processes,
parallel execution, cancellation, and structured results while adapting agent
discovery, trust, diagnostics, and Delegation Contract enforcement. Matty does
not treat the upstream example as a stable runtime API and accepts responsibility
for maintaining the imported implementation. Child processes inherit the
active session's provider, model, authentication, and reasoning configuration;
Matty roles vary prompts and allowed tools but do not silently switch models or
require a second provider account. Matty also retains the imported runtime's
Bounded Concurrency of eight tasks accepted per call and four child processes
active simultaneously. Tasks beyond the active limit are reported as queued,
not as concurrent execution.
