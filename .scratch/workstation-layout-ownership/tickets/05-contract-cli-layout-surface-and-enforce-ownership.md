Status: ready-for-agent

# Contract the CLI layout surface and enforce ownership

## What to build

Complete the contraction after every production caller uses owner APIs. Replace
CLI test setup with an owner-assembled fixture, delete the shared production
layout model and all obsolete derivation or mapping helpers, and add structural
enforcement that keeps ambient reads and artifact layout with their approved
owners.

## Blocked by

- [Route capability packs through owning layouts](03-route-capability-packs-through-owning-layouts.md)
- [Route setup health through owner observations](04-route-setup-health-through-owner-observations.md)

## Acceptance criteria

- [ ] No production caller depends on the former shared path structure or resolver.
- [ ] The shared path type, resolver, default-source helper, mapping helpers, duplicated CLI derivation, and obsolete compatibility wrappers are deleted.
- [ ] CLI production code contains no state, skill, host, Installed Source, or Engram candidate path policy.
- [ ] Ambient environment, user-home, and current-directory reads are limited to the approved process edge and workstation resolver.
- [ ] CLI end-to-end tests use a test-only aggregate fixture assembled exclusively from owner APIs.
- [ ] The fixture derives no canonical path independently and is unavailable to production code.
- [ ] Obsolete tests protecting the old layout decomposition are removed only after equivalent owner and CLI contracts cover their behavior.
- [ ] Positive owner contracts cover normalization, overrides, state roots, sources, skills, hosts, and executable topology.
- [ ] Focused structural enforcement prevents reintroducing the shared layout model, known CLI artifact derivation, and unauthorized ambient reads.
- [ ] Structural enforcement avoids a fragile repository-wide scan of every path-like literal.
- [ ] Help, version, init, install, update, uninstall, pack, and doctor compatibility remain covered at the CLI seam.
- [ ] No forwarding facade, compatibility wrapper, or dual layout ownership survives.
- [ ] Repository documentation and accepted architecture remain consistent with the final code.
- [ ] The complete repository test suite and diff checks pass with sandboxed Home and XDG configuration.

## Out of scope

- User-visible behavior, path, schema, output, or command changes.
- New abstractions unrelated to enforcing workstation layout ownership.
- Opportunistic cleanup outside the obsolete layout surface.
