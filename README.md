# Matty

**Bounded delegation for [Pi](https://github.com/earendil-works/pi).**

Matty gives Pi a dependable runtime for delegating engineering work to isolated child processes. It ships five least-privilege roles, capability preflight, bounded concurrency, web research, CodeGraph tools, and local diagnostics. Your workflows and project policy remain yours.

## Install

Matty currently certifies this exact host:

- Pi `0.83.0`
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

A call accepts one to eight tasks and runs at most four child processes concurrently. Required groups are atomic: one failure cancels pending work. Optional fallback is available only for inspection groups and explicitly reports skipped work.

### Roles

| Role | Purpose | Authority |
| --- | --- | --- |
| `explorer` | Explore code and trace behavior | Local inspection |
| `designer` | Assess module boundaries and design options | Local inspection |
| `reviewer` | Review code and GitHub context | Local inspection plus read-only `gh` after preflight |
| `researcher` | Gather cited web evidence | Four web tools plus `research_file`; temporary workspace files and one approved Markdown report |
| `worker` | Implement a bounded change | Trusted working tree and validated temporary paths |

At most one `worker` may be active per repository. The parent agent reviews and integrates delegated changes.

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

## CodeGraph

Matty adds CodeGraph search, navigation, caller/callee, impact, and exploration tools to the parent agent. On session start, Matty uses the nearest existing CodeGraph index or creates `.codegraph/` by default in the current project. It refuses unsafe roots such as your home directory or the filesystem root; initialization failure leaves the rest of Matty available.

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
