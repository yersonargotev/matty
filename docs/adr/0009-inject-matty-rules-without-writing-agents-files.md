# Inject Matty Guidance and Rules without writing AGENTS files

Matty stores two distinct versioned prompt blocks inside its package and injects
them through Pi's `before_agent_start` extension event independently in both
parent and every child process.

Matty Guidance contains overridable defaults, including the packaged Argote
engineering and Neutral Spanish guidance. It is delimited by
`<!-- matty:guidance -->` and `<!-- /matty:guidance -->`. Matty Rules contains
Core workflow invariants and is delimited by `<!-- matty:rules -->` and
`<!-- /matty:rules -->`. Matty removes stale, duplicate, inline, and unmatched
owned markers before deterministic injection.

The resulting order is host and project instructions, Matty Guidance, then Matty
Rules. Guidance retains its explicit precedence language: more-specific user or
project instructions override its defaults, while higher-priority authority and
safety instructions prevail. Rules are last because their Core invariants may
not be silently relaxed. Rules conflict detection excludes content inside both
Matty-owned blocks, so stale package content cannot block a workflow while a
conflicting external project instruction still can.

The rules translate imported host-specific concepts such as `Agent` calls and
parallel task batching, while tool-specific `promptGuidelines` document the
exact `subagent` schema. Matty never writes global or project `AGENTS.md` files
for either block. External `AGENTS.md` files continue to govern project
conventions and policy. A detected direct Rules conflict blocks and diagnoses
only the affected workflow path.
