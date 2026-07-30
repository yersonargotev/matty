# Inject Matty Rules without writing AGENTS files

Matty stores versioned rules inside its package and injects them through Pi's
`before_agent_start` extension event into both parent and child system prompts.
The block is delimited by `<!-- matty:rules -->` and
`<!-- /matty:rules -->`, and Matty deduplicates it before injection. The rules
translate imported host-specific concepts such as `Agent` calls and parallel
task batching, while tool-specific `promptGuidelines` document the exact
`subagent` schema. Matty never writes global or project `AGENTS.md` files for
this purpose. Matty Rules govern only Matty workflow invariants; external
`AGENTS.md` files govern project conventions and policy. Project instructions
may extend but not silently redefine a Matty invariant. A detected direct
conflict blocks and diagnoses only the affected workflow path.
