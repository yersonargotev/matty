# Matty

**Bounded delegation for [Pi](https://github.com/earendil-works/pi).**

Matty gives Pi a dependable runtime for delegating engineering work to isolated child processes. It ships five least-privilege roles, capability preflight, bounded concurrency, web research, and local diagnostics. Your workflows and project policy remain yours.

## Install

Matty currently certifies this exact host:

- Pi `0.84.2`
- Node.js `>=22.19.0`
- macOS on Apple Silicon (`darwin/arm64`)
- Reference model path: `openai-codex/gpt-5.6-sol` with ChatGPT/Codex subscription authentication

Other active models are reported as unverified and do not by themselves disable Core activation; delegation still requires Pi-compatible authentication. Other Pi versions and targets are reported as uncertified.

Install Matty globally through Pi:

```bash
pi install npm:@yargote/matty
```

Then start Pi from a trusted repository:

```bash
cd /path/to/repository
pi
```

Check the installation inside Pi:

```text
/matty status
/matty doctor
```

Both commands support machine-readable output with `--json`.

## Theme

The package includes the `matty-catppuccin-mocha` theme for Pi 0.84.2. Select it from `/settings`, or use it for one run without changing your saved setting:

```bash
pi --use-theme matty-catppuccin-mocha
```

## Delegate work

Ask Pi to delegate a bounded task, for example:

> Delegate an explorer to map the authentication flow and report the relevant files.

Each task launches an independent Pi child with the parent's active model and authentication, so delegated work incurs normal model usage.

Matty exposes one `subagent` tool with this contract:

```json
{
  "requirement": "required",
  "tasks": [
    {
      "role": "explorer",
      "task": "Map the authentication flow and report the relevant files"
    }
  ]
}
```

A call accepts one to eight tasks and runs at most four child processes concurrently. Required groups are atomic: one failure cancels pending work. Optional fallback is available only for inspection groups and explicitly reports skipped work. Task declarations are role-specific: `explorer`, `designer`, and `worker` accept only `role` and `task`; `reviewer` requires a closed `reviewScope`; `researcher` requires `web` and may receive `report`. `web` and `report` are rejected for every other role.

### Roles

| Role | Purpose | Authority |
| --- | --- | --- |
| `explorer` | Explore code and trace behavior | Local inspection |
| `designer` | Assess module boundaries and design options | Local inspection |
| `reviewer` | Review code and GitHub context | Local inspection plus read-only `gh` after preflight |
| `researcher` | Gather cited web evidence | Four web tools plus `research_file`; temporary workspace files and one approved Markdown report |
| `worker` | Implement a bounded change | Trusted working tree and validated temporary paths |

Every `reviewer` task must include a closed `reviewScope` with exactly `schemaVersion`, `issue` (`repository`, `number`, `reference`), `requirements`, `outOfScope` (`reference`, `reason`), `baseSha`, `candidateSha`, and `axes`. Reviewer findings must bind to that candidate and one exact listed requirement; dependent and out-of-scope references remain excluded.

At most one `worker` may be active per repository. A worker returns a structured completion report; its process success and checks are supporting evidence only. The parent inspects the diff and independently runs the repository-authoritative full gate before claiming success or integrating.

For cited research, declare web access and an approved report path:

```json
{
  "requirement": "required",
  "tasks": [
    {
      "role": "researcher",
      "task": "Compare the current upstream APIs using primary sources",
      "web": "required",
      "report": "docs/research/upstream-apis.md"
    }
  ]
}
```

## Manage delegations

Use `/matty delegations` to inspect current and recent delegations; add `--json` for structured output. In the TUI, browse a task by its short display ID with `/matty task T-<8-hex-digits>`.

Headless commands require the task's exact UUID:

```text
/matty task <exact-task-UUID>
/matty task <exact-task-UUID> transcript
/matty steer <exact-task-UUID> <message>
/matty follow-up <exact-task-UUID> <message>
```

The first task command reads registry metadata. The explicit `transcript` suffix reads the private Child Session presentation, which can contain prompts, reasoning, tool arguments, and results; unlike delegation summaries and task metadata, it is not a safe summary surface.

## Safety and privacy

Matty roles use least-privilege tool surfaces and capability preflight. The Inspection Guard and Worker Guard are best-effort command policies, **not security sandboxes**. Treat Matty and delegated children as trusted code.

The parent retains commits, pushes, pull requests, reviews, merges, releases, user-configuration changes, and other external-state mutation. Matty emits no telemetry, analytics, update probes, or background network requests. Network access occurs only through visible operations such as model calls, web research, read-only GitHub inspection, or project-local dependency installation.

## Development

```bash
npm install
npm run check
```

`npm run check` runs type checking, tests, the build, artifact inspection, release-chain verification, and acceptance tests.

## License

[MIT](LICENSE) · [Issues](https://github.com/yersonargotev/matty/issues) · [npm](https://www.npmjs.com/package/@yargote/matty)
