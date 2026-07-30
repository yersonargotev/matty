# Keep delegation contracts outside imported skills

Matty records delegation obligations in a Matty-owned Delegation Contract
rather than modifying the initially verbatim imported `SKILL.md` files. The
contract classifies each skill or scenario as `required`, `optional`, or `none`
and records cardinality, parallelism, independence, role, and failure behavior.
This preserves source provenance while giving Matty a machine-checkable
interface for adapting the workflow to Pi's subagent surface. Matty evaluates
the applicable contract through a Delegation Preflight before a skill or
conditional branch produces effects. An unmet required contract blocks only
that path; the skill remains discoverable and unrelated workflows remain
available. An explicit upstream instruction to delegate, execute in parallel,
or isolate context maps to `required`; `optional` is used only when the skill
itself permits an inline execution with equivalent semantics; absence of a
delegation instruction maps to `none`. Ambiguous cases require maintainer
review rather than an invented fallback. A required multi-agent Delegation
Group is atomic: failure of any member fails the group, cancels work that can no
longer satisfy the contract, and exposes partial output only as diagnostics.
Matty neither retries the whole group invisibly nor substitutes inline or
sequential execution.
