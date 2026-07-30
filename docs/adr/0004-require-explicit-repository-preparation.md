# Require explicit repository preparation

Matty makes its Shared Skill Catalog available immediately after installation,
but engineering flows that depend on repository policy require explicit
Repository Preparation through `setup-matt-pocock-skills`. Matty may detect
missing preparation and direct the user to that skill, but it never performs
the writes during Pi startup or without consent. This preserves the imported
workflow's project assumptions without introducing a separate `/matty setup`
command or hidden project mutation.
