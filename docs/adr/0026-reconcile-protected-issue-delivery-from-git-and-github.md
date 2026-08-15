# Reconcile protected Issue Delivery from Git and GitHub

Issue Delivery derives authoritative state from Git and GitHub rather than a
Matty database or Pi transcript. One explicit `/matty deliver <issue>`
invocation binds an exact ready issue and trusted Prepared Repository to a
stable Delivery Identity; repeating it reconciles and continues from issue,
branch, candidate SHA, PR, checks, reviews, integration, closure, and
owned-resource markers. Matty v0.2 certifies GitHub only and permits one
nonterminal delivery per repository.

The workflow preserves unrelated work through isolation and permits Current
Work Adoption only after structured human confirmation and fresh evidence.
Verification, Manual Validation, and independent review bind to the candidate
SHA; candidate changes invalidate affected evidence. Repair cycles, CI retries,
and delegated writing are bounded. Published history is append-only,
integration uses the repository's protected route without bypass, cancellation
stops future effects while preserving recoverable work, and ambiguous ownership
or post-crash effects block with an Exception Brief instead of being inferred
or replayed blindly.
