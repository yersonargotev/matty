# Ship v1 without Matty-owned configuration

Matty v1 does not create or read user- or project-owned configuration of its
own. Matty Rules, compatibility manifests, role definitions, Delegation
Contracts, and Web Contracts are immutable Package Contract Data inside each
complete release snapshot.

Pi remains authoritative for host settings and project trust,
`pi-web-access` remains authoritative for provider settings and state, and
explicit Repository Preparation creates project-owned workflow policy rather
than Matty configuration. Matty therefore assigns no meaning to paths such as
`~/.matty`, `.mattyrc`, or a project `.matty` file.

Matty may create validated session-scoped Research Workspaces and explicit
project-owned Research Reports, but it persists no activation, first-run,
compatibility, or diagnostic state. A future configuration surface requires an
explicit minor release that defines precedence, ownership, migration, and
removal behavior.
