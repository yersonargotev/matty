# Publish an install-safe artifact

`@yargote/matty` is published as a prebuilt Install-Safe Artifact. It declares
no `preinstall`, `install`, `postinstall`, `prepare`, or equivalent installation
lifecycle script. Building, validation, and artifact generation happen before
publication; installation only places the reviewed package contents through
Pi's package lifecycle.

Release CI inventories lifecycle scripts in the exact resolved production
dependency tree. A new, changed, or unreviewed dependency script blocks
publication until maintainers record its package, exact version, command,
effects, and justification. This does not assign dependency behavior to Matty,
but it prevents the package from silently acquiring install-time execution
through a transitive update.
