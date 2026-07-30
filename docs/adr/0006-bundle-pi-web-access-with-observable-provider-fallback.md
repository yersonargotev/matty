# Bundle pi-web-access with observable provider fallback

Matty supplies its Web Capability by bundling exactly
`pi-web-access@0.15.0`, without a semver range. It prefers the OpenAI search
path backed by Pi's ChatGPT/Codex subscription credentials and permits the
dependency's internal Exa MCP fallback when OpenAI is unavailable. The selected
provider remains observable, but credentials, configuration, and stored state
remain owned by Pi and `pi-web-access`.

Matty certifies only `web_search`, `source_check`, `fetch_content`, and
`get_search_content`. The parent and `researcher` receive those tools; the
other four roles do not. Matty does not enable browser-cookie access, write
provider configuration, or expose the internal fallback as a general MCP
surface.

Unavailable or failed web access follows the calling Capability Contract:
required use fails, optional use may continue only with explicit disclosure,
and absent use receives no web tools. Matty `0.1` does not maintain per-skill
Web Contracts; workflow-specific requirements stay outside Matty Core.

Phase 0 validated the exact dependency with Pi `0.83.0`, macOS Apple Silicon,
`openai-codex/gpt-5.6-sol`, ChatGPT/Codex authentication, and the packed Matty
artifact. Changing the bundled version requires a new minor release and the
same complete validation.
