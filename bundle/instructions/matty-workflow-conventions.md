# Matty workflow conventions

Apply these Matty-owned conventions when the installed engineering skills interact with project planning artifacts.

## Specs and tickets

Use **Specs and tickets** as the workflow vocabulary. A spec defines the accepted behavior; tickets are the implementation slices derived from it.

## Local ticket layout

When the configured issue tracker is local Markdown, keep its spec and all ticket artifacts under `.scratch/<feature-slug>/`; never write local tickets at the repository root.

## Tracker-defined wayfinding

Use the tracker-defined wayfinding operations documented by the configured issue tracker for map identity, ticket identity, blocking edges, claiming, frontier discovery, and resolution. Do not assume that GitHub labels or assignments exist when the configured tracker defines different operations.
