# Tickets: Matty core lifecycle deepening

Tracer-bullet slices that move classic install, update, and uninstall behavior
behind the deep Matty core lifecycle module without changing user-visible
behavior. Source: [specification](spec.md) and ADR 0003.

Work the **frontier**: any ticket whose blockers are all done. Clear context
between tickets and use `/implement` for one frontier ticket at a time.

| # | Ticket | Blocked by | Status |
| --- | --- | --- | --- |
| 01 | [Move classic state behind lifecycle ownership](tickets/01-move-classic-state-behind-lifecycle-ownership.md) | None | resolved |
| 02 | [Route install through the lifecycle facade](tickets/02-route-install-through-lifecycle-facade.md) | 01 | resolved |
| 03 | [Route update through the lifecycle facade](tickets/03-route-update-through-lifecycle-facade.md) | 02 | resolved |
| 04 | [Route uninstall through the lifecycle facade](tickets/04-route-uninstall-through-lifecycle-facade.md) | 01 | ready-for-agent |
| 05 | [Contract the CLI lifecycle and verify the architecture](tickets/05-contract-cli-lifecycle-and-verify.md) | 02, 03, 04 | ready-for-agent |
