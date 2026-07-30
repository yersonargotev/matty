# Emit no Matty-owned telemetry or background network

Matty v1 emits no telemetry, analytics, usage metrics, automatic crash reports,
update probes, or other background network requests. Startup, `/matty status`,
and `/matty doctor` are fully local and perform no live provider probes.

Network requests occur only as User-Directed Network Operations attributable to
a visible user or workflow action. These include normal parent and child Pi
model requests, Web Capability calls, reviewer read-only GitHub inspection, and
worker project-local dependency installation. Pi separately owns its package
lifecycle network behavior.

External providers retain their own data-processing contracts during those
operations. Matty neither duplicates their reporting nor adds a separate
request. Introducing Matty-owned telemetry later requires an explicit product
decision covering consent, disclosure, data ownership, retention, and opt-out
behavior.
