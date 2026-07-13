# Tickets: Matty setup health deepening

An expand-contract sequence that moves base-installation health diagnosis behind
one deep setup health module without changing observable `doctor` behavior.
Source: [specification](spec.md).

Work the **frontier**: any ticket whose blockers are all done. Clear context
between tickets and use `/implement` for one frontier ticket at a time.

| # | Ticket | Blocked by | Status |
| --- | --- | --- | --- |
| 01 | [Contract setup health behavior and record the architecture](tickets/01-contract-setup-health-behavior-and-record-architecture.md) | None | resolved |
| 02 | [Route doctor through the setup health module](tickets/02-route-doctor-through-setup-health-module.md) | 01 | resolved |
| 03 | [Contract the CLI health adapter and verify the architecture](tickets/03-contract-cli-health-adapter-and-verify.md) | 02 | resolved |
