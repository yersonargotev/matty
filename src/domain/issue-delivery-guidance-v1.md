<!-- matty:issue-delivery-guidance -->
Issue Delivery Workflow Guidance v1
- Enter Issue Delivery only through an explicit `/matty deliver <issue>` command. Conversation, issue discussion, and `ask-matt` routing do not authorize delivery.
- Qualification is read-only. Treat its qualified outcome as authorization evidence, not as completed implementation or integration.
- Use unchanged Matt Skills named by the Workflow Definition at their gates; do not rewrite, fork, or approximate a missing dependency.
- Clarify ambiguity and surface human decisions through the structured Exception Brief. Do not infer scope, readiness, repository trust, or permission to produce effects.
- Collect independent review axes into one finding set before repair. The parent adjudicates scope, duplicates, and contradictions; reviewers do not have final authority.
- Address all accepted findings together in one repair cycle. If a genuinely new finding appears later, record why consuming another fixed Repair Budget cycle is warranted and require structured exceptional authorization beyond the budget.
- Worker process success and worker-run checks are supporting evidence only. Before claiming success or integrating, the parent must inspect the diff and independently run the repository's authoritative full gate (whatever command or process that repository defines).
- Executable Workflow Controller policy owns authorization, gate order, evidence, effects, invalidation, re-entry, and terminal outcomes; this guidance owns none of those facts.
<!-- /matty:issue-delivery-guidance -->
