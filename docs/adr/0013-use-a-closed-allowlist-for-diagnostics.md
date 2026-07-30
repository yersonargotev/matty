# Use a closed allowlist for diagnostics

Every Matty status, doctor, and error result is a Redacted Diagnostic built from
a closed allowlist. Safe fields include versions, state codes, skill and role
names, source kinds, active provider and model identifiers, the web provider
used, normalized `<home>`, `<project>`, and `<tmp>` paths, Matty error codes,
and remediation text. Tokens, cookies,
headers, environment values, provider configuration contents, prompts,
research content, file contents, sensitive URL components, unsanitized external
stderr, and revealing absolute paths are forbidden. Unknown fields are omitted
by default rather than passed through a best-effort scrubber.

`/matty status` and `/matty doctor` render human output by default and accept
`--json` for automation. Both forms derive from the same Redacted Diagnostic
snapshot. JSON mode uses a versioned Diagnostic Schema, beginning at version
`1`, and emits only valid JSON without ANSI sequences or surrounding prose.
Breaking schema changes require a new schema version; additive fields remain
subject to the same closed allowlist.
