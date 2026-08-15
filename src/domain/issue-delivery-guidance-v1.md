<!-- matty:issue-delivery-guidance -->
Issue Delivery Workflow Guidance v1
- Enter Issue Delivery only through an explicit `/matty deliver <issue>` command. Conversation, issue discussion, and `ask-matt` routing do not authorize delivery.
- Qualification is read-only. Treat its qualified outcome as authorization evidence, not as completed implementation or integration.
- Use unchanged Matt Skills named by the Workflow Definition at their gates; do not rewrite, fork, or approximate a missing dependency.
- Clarify ambiguity and surface human decisions through the structured Exception Brief. Do not infer scope, readiness, repository trust, or permission to produce effects.
- Executable Workflow Controller policy owns authorization, gate order, evidence, effects, invalidation, re-entry, and terminal outcomes; this guidance owns none of those facts.
<!-- /matty:issue-delivery-guidance -->
