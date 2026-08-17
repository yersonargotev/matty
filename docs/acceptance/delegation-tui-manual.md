# Manual Delegation TUI validation

- Artifact: `yargote-matty-0.2.0.tgz`
- SHA-256: `338cb1ac57525af1cac7cc759fed5966c4f90be24c57f2a601d8ba5fbb7e9a5f`
- Pi: `0.84.2`
- Target: `darwin/arm64`
- Date: `2026-08-16T05:37:10.413Z`

## Subjective observations (complete after exiting Pi)

- [x] No duplicated, stale, or corrupted console frames during live rerenders.
- [x] Cancellation controls and confirmation were visually unambiguous.
- [x] Cursor and editor focus were correct after closing with `q` and with `Esc`.
- [x] `/matty status` accepted input immediately after each close.

Operator/result notes:

- Result: passed.
- The operator reported clean live rendering, clear cancellation controls, correct focus restoration, and immediate `/matty status` input after closing the console.

## Continuation validation for the next packed candidate

- [ ] A closed persistent Child Session shows HEAD and working-tree drift before confirmation.
- [ ] Confirming Continue creates fresh linked Delegation and Delegated Task IDs without changing the source result.
- [ ] A missing or ephemeral transcript reports that Continuation is unavailable.
- [ ] A Reviewer with unavailable required commits is blocked by fresh preflight.
- [ ] A continuation Worker visibly waits for Single Writer and proceeds only after authority is acquired.
- [ ] Headless commands do not expose Continue.

Operator/result notes: pending the next packed candidate.
