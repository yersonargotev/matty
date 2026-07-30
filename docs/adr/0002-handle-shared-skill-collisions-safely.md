# Handle shared skill collisions safely

When an external skill claims a Matty-reserved name, Matty continues activating
only if it can deterministically reject or ignore the external definition while
reporting the Shared Skill Collision. If Pi cannot guarantee Matty's ownership
of the name, Matty activates none of the shared workflow, leaves Pi usable, and
retains actionable diagnostics. This favors a clearly unavailable workflow
over a partially active or ambiguously overridden one. Phase 0 must prove that
the wrong content cannot reach the model before Matty blocks activation; load
order is not evidence. If Pi `0.83.0` cannot satisfy this Activation Safety
Gate, Matty v1 is not published until Pi gains the necessary capability or the
activation design is deliberately revised.
