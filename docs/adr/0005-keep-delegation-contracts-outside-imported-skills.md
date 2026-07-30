# Keep capability contracts in Matty Core

Matty Core owns Capability Contracts as versioned runtime policy. Each contract
defines the Matty Role, tool surface, write authority, web requirement,
cardinality, concurrency, independence, and failure behavior for one Core
operation without depending on workflow content.

Matty `0.1` does not classify individual skills or maintain per-skill
contracts. Capability Preflight validates the requested execution before it
produces effects. Unsupported or unavailable required execution fails
explicitly rather than being replaced by inline or sequential work.
Workflow-specific policy remains outside the MVP.
