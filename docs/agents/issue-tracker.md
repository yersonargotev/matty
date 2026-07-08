# Issue tracker: Local Markdown

Specs and tickets for active planning can live as markdown files in `.scratch/`.
Durable product and architecture decisions should be migrated to `docs/` before
temporary planning folders are removed.

## Conventions

- One feature per directory: `.scratch/<feature-slug>/`
- The spec is `.scratch/<feature-slug>/spec.md`
- Implementation tickets are `.scratch/<feature-slug>/tickets/<NN>-<slug>.md`, numbered from `01`
- Triage state is recorded as a `Status:` line near the top of each ticket file
- Comments and conversation history append to the bottom of the file under a `## Comments` heading

## Archiving completed planning

When a `.scratch/<feature-slug>/` effort is complete, keep durable information
in repo docs instead of relying on scratch files forever:

- Product scope and user-facing decisions go under `docs/product/`.
- Architecture decisions go under `docs/adr/`.
- Future work and unresolved fog go in `docs/roadmap.md` or a focused planning doc.
- After migration, the completed `.scratch/<feature-slug>/` directory can be deleted.

## When a skill says "publish to the issue tracker"

Create a new `spec.md`, `tickets.md`, or ticket file under `.scratch/<feature-slug>/` (creating the directory if needed).

## When a skill says "fetch the relevant ticket"

Read the file at the referenced path. The user will normally pass the path directly.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a file with one **child** file per ticket.

- **Map**: `.scratch/<effort>/map.md` — the Destination / Notes / Decisions-so-far / Not-yet-specified / Out-of-scope body.
- **Child ticket**: `.scratch/<effort>/tickets/NN-<slug>.md`, numbered from `01`, with the question in the body. A `Type:` line records the ticket type (`research`/`prototype`/`grilling`/`task`); a `Status:` line records `claimed`/`resolved`.
- **Blocking**: a `Blocked by: NN, NN` line near the top. A ticket is unblocked when every file it lists is `resolved`.
- **Frontier**: scan `.scratch/<effort>/tickets/` for files that are open, unblocked, and unclaimed; first by number wins.
- **Claim**: set `Status: claimed` and save before any work.
- **Resolve**: append the answer under an `## Answer` heading, set `Status: resolved`, then append a context pointer (gist + link) to the map's Decisions-so-far in `map.md`.
