# Treat upstream skills as manually imported source

Matty initially copies `skills/engineering` and `skills/productivity` verbatim
from one pinned commit of `mattpocock/skills`. After that Skill Import, the files
are ordinary Matty-owned source and may change independently. Upstream changes
are incorporated only when a maintainer deliberately chooses to review and
import or reconcile them in a new Matty release; Matty performs no automatic
tracking, merging, or runtime synchronization. During Upstream Reconciliation,
Matty's changes are preserved by default and upstream differences are accepted
skill by skill; replacing a Matty change must be an explicit review decision.
