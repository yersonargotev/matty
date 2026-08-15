# Manage Delegations within Matty

Matty will implement delegation observation and control natively over its owned Subagent Runtime rather than use `pi-subagents` as an execution backend. This preserves process isolation, Capability Contracts, atomic groups, Matty Roles, and Single Writer while allowing Matty to adopt useful management concepts incrementally; interoperability with `pi-subagents` remains an optional future experiment, not a Core dependency.
