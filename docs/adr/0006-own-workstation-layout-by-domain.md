# ADR 0006: Owning modules derive their workstation layout

## Status

Accepted.

## Context

`internal/cli.Paths` combines normalized environment facts with derived paths
owned by unrelated domains. This makes the CLI aware of state files, skill
locations, host projections, and executable locations even though it should
only compose the modules that own those artifacts.

## Decision

`internal/workstation` exclusively owns normalization of the minimum workstation facts supplied
by the process, including the home directory, configuration home, executable
search path, and Homebrew prefix. Each owning module will derive the paths and
layout policy for its own artifacts from those normalized facts. Modules must
not read the operator's ambient environment directly; callers retain the
ability to supply sandboxed facts.

The workstation module validates the home directory, normalizes the
configuration home, derives Matty Home, and creates the immutable snapshot. It
does not know state filenames, skill paths, host projections, or executable
locations.

`internal/cli` remains the composition root. It passes normalized facts and
explicit dependencies to owning modules, but does not derive or transport a
shared catalog of state, skill, host-projection, or executable paths.

This decision refines the temporary path-composition responsibility retained by
`internal/cli` in ADR 0003 and the shared-workstation exclusion in ADR 0004.

Skill-source selection is the deliberate exception to independent derivation.
`skillbundle` applies the precedence between an explicit operator override, a
repository checkout, and the Installed Source once, returning one resolved
Skill Source value with its origin and missing-source guidance. The CLI passes
that value to consumers; no consumer repeats the selection policy.

`bootstrap` owns an immutable Installed Source descriptor containing the
checkout root and its bundle location. It derives the descriptor from the
workstation snapshot or an explicit `--source-root`; `skillbundle` uses it as a
fallback candidate and core lifecycle reuses it for validation. The CLI adapts
the flag but does not normalize or derive the Installed Source layout.

The process edge captures ambient inputs once, including the home directory,
configuration-home input, executable search path, Homebrew prefix, and current
working directory. The resolver normalizes these into an immutable workstation
fact set. Owning modules receive that set or a narrower derived value and must
not call `os.Getenv`, `os.UserHomeDir`, or `os.Getwd` to rediscover layout
inputs.

Resolution is lazy and occurs at most once for one command invocation. Commands
that do not need workstation access, including help and version output, do not
require a valid home directory. Every module participating in an invocation
observes the same immutable snapshot.

Snapshot construction accepts an explicit Home override for commands such as
`matty init --home`. Without the override, configuration home follows the
existing XDG normalization. With the override, configuration home is derived
from that Home and does not inherit ambient `XDG_CONFIG_HOME`. The CLI only
translates the flag into resolver input; it does not retain a second layout
implementation.

Host layout follows the same single-owner rule. The Codex and OpenCode modules
derive their canonical host paths from normalized workstation facts and expose
narrow layout values to both core lifecycle and capability-pack consumers.
Lifecycle modules decide when a host projection participates; the host module
decides where that projection lives.

`skillbundle` owns the global skill-installation layout. It derives the target
directory and owns skill-name projection rules; core lifecycle and capability
pack decide which skills participate in their reconciliation, while setup
health consumes the skill owner's read-only observation.

`engrambin` owns Engram executable topology: candidate locations, precedence,
resolution, and read-only observation. It receives only the required
workstation facts. Lifecycle, capability-pack, and setup-health consumers use
its resolver or observer rather than receiving PATH, Homebrew, or local-bin
candidate paths from the CLI.

The resolver also derives one shared Matty Home root. Core lifecycle derives
its classic state file beneath that root, while capability pack derives its
separate pack-state file. Sharing the namespace root does not merge ownership
of the files below it or justify another global path catalog.

Read-only consumers do not reacquire layout knowledge. In particular,
`setuphealth` consumes observations supplied by the modules that own lifecycle
state, skills, host configuration, and Engram behavior. It receives directly
only facts intrinsic to diagnosis, such as executable-search and report
context, rather than a union of every observed artifact path.

## Consequences

- Workstation layout policy becomes local to the module that owns each artifact.
- Tests preserve sandbox leverage without granting modules ambient environment access.
- The shared 20-field `internal/cli.Paths` interface can be removed once all owners expose their narrower construction seams.
- Source selection remains a separate policy owned by the skill-source modules rather than becoming CLI path logic.
- Core lifecycle, capability packs, and setup health observe the same resolved Skill Source.
- Bootstrap, source selection, and lifecycle validation share one Installed Source descriptor.
- Codex and OpenCode path conventions cannot diverge between lifecycle owners.
- Core and pack lifecycle share one global skill-installation layout without sharing reconciliation policy.
- Engram executable-location policy has one owner and is hidden from all consumers.
- Matty's state namespace remains consistent while classic and pack state retain separate owners.
- Moving an owned artifact does not require setup-health path changes.
- One command observes a consistent, explicitly substitutable workstation context.
- Help and version behavior remain independent from workstation resolution.

## Compatibility

This is a behavior-preserving architectural change. Existing global paths,
Skill Source precedence and diagnostics, error text, CLI output, state schemas,
filesystem effects, and sandboxed `HOME`/`XDG_CONFIG_HOME` behavior remain
unchanged. In particular, a missing `HOME` remains an error, a relative
`XDG_CONFIG_HOME` still falls back to `$HOME/.config`, and no state moves to a
new XDG location. State migration, path changes, and revised diagnostics require
separate decisions.

## Enforcement

Each owner maintains positive contract tests for its derived layout, overrides,
and sandbox behavior. A focused structural architecture test prevents
`internal/cli` from reintroducing the shared `Paths`/`ResolvePaths` surface,
known artifact-path derivation, or unauthorized ambient reads. Ambient calls
such as `os.Getenv`, `os.UserHomeDir`, and `os.Getwd` are limited to the allowed
process edge and workstation resolver. The structural check targets concrete
ownership violations rather than scanning every path-like literal globally.

CLI end-to-end tests may use a test-only aggregate fixture for setup and
assertions. That fixture must be assembled from owner APIs, derive no paths of
its own, and remain unavailable to production code. Semantic path tests remain
with each owner; CLI tests cover wiring, rendering, and integrated effects.

## Migration

Migration is incremental but deletion-complete. First introduce and contract
the workstation snapshot, then add owner layouts and migrate vertical
consumers, route setup health through owner observations, and replace CLI test
fixtures. Finally delete `internal/cli.Paths`, `ResolvePaths`, their mapping
helpers, and all obsolete CLI derivation before enabling structural
enforcement. Transitional wiring may exist within the migration series, but no
forwarding facade, compatibility wrapper, or dual layout ownership remains at
closure.
