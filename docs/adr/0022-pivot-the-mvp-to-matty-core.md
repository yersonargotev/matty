# Pivot the MVP to Matty Core

Matty `0.1` will ship Core runtime capabilities rather than a shared workflow
catalog. The MVP includes the owned Subagent Runtime, five Matty Roles and their
guards, Matty Rules, the pinned Web Capability, status and doctor, exact host
certification, Zero Telemetry, an Install-Safe Artifact, and public publication
with provenance and OIDC trusted publishing.

The Shared Skill Catalog, `ask-matt`, skill import and collision handling,
Repository Preparation, and per-skill delegation or web contracts move outside
the MVP. Pi `0.83.0` cannot give an installed extension the universal
pre-model ownership seam required by the catalog contract. Continuing to make
catalog activation the release gate would block otherwise useful, independently
testable runtime capabilities. The narrower Core boundary makes the supported
behavior explicit and preserves a path for future workflow distribution under
new evidence and decisions.

This decision supersedes ADRs 0001, 0002, 0003, 0004, and 0021. It reformulates
ADRs 0005, 0006, and 0014 around Core runtime policy, Core web availability, and
global Core installation.

ADRs 0008, 0009, 0010, 0012, and 0015 remain active for their runtime, rule,
role, Single Writer, and no-user-configuration decisions. References in those
ADRs to imported workflows, per-skill contracts, or Repository Preparation are
superseded for `0.1` by this ADR. All other ADRs continue to apply.
