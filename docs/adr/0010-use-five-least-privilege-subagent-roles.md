# Use five least-privilege subagent roles

Matty v1 provides five curated Matty Roles: `explorer` for read-only code
discovery, `reviewer` for review with local and GitHub inspection,
`designer` for independent design alternatives, `researcher` for web-backed
investigation and bounded report production, and `worker` for implementation
with execution and write access. Matty Rules map upstream terms such as
`Explore`, `background agent`, and `general-purpose` to the concrete role
declared by the Delegation Contract. Matty does not provide an unrestricted
general-purpose role because it would obscure the permissions each workflow
actually needs.

Only `researcher` receives the certified `web_search`, `source_check`,
`fetch_content`, and `get_search_content` tools. The parent agent retains the
same Web Capability; the other four roles must delegate external research to
the researcher or return it to the parent.

The `explorer`, `designer`, and `reviewer` receive `read`, `grep`, `find`, `ls`,
and `bash`. Explorer and designer shell access supports local Git history
and diagnostic inspection. Reviewer shell access also supports
remote inspection through inherited `gh` authentication. A shared, role-aware
Inspection Guard blocks recognized local and remote mutation command families
and blocks `gh` entirely for explorer and designer. The guard is explicitly
best-effort rather than an isolation boundary; the main agent retains sole
responsibility for comments, approvals, merges, and other GitHub mutations.

The `worker` receives `read`, `write`, `edit`, `grep`, `find`, `ls`, and `bash`.
It may edit the trusted working tree, use validated temporary paths, install
project-local dependencies, and run project checks. A Worker Guard blocks
recognized `gh`, Git index and reference mutation, global-installation,
external-path, and real user-configuration writes. This guard is also
best-effort rather than an isolation boundary; the main agent owns Git
integration and all external-state mutation.
