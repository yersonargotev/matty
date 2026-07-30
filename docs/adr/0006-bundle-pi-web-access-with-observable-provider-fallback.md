# Bundle pi-web-access with observable provider fallback

Matty supplies its Web Capability by bundling exactly
`pi-web-access@0.15.0`, without a semver range. It prefers the OpenAI search
path backed by Pi's ChatGPT/Codex subscription credentials, while allowing
`pi-web-access` to fall back internally to Exa MCP when OpenAI is unavailable.
This internal transport does not create a general Matty MCP surface. The
provider used must remain observable, and users may pin OpenAI through
provider-owned `pi-web-access` configuration when they do not want fallback.

Matty bundles the complete, unmodified dependency rather than maintaining a
fork. Its v1 support contract covers `web_search`, `source_check`,
`fetch_content`, and `get_search_content`. Additional capabilities such as
GitHub cloning, PDF handling, YouTube, and local video remain
`pi-web-access`-owned behavior outside Matty's specific v1 guarantees. Matty
does not enable browser-cookie access or write provider configuration.

Phase 0 validated version `0.15.0` as the pin to bundle for Matty `0.1.0` with
the Certified Pi Version, Certified Target, Reference Model Path,
ChatGPT/Codex OAuth, and packed-package combination. The proof installed the
exact web extension alongside the packed Matty artifact; adding it to Matty's
package dependency graph remains a separate integration step. The OpenAI
search path resolved Pi's `openai-codex` credential through the extension
context, selected its first compatible internal search model `gpt-5.4`, and called
`https://chatgpt.com/backend-api/codex/responses`. The returned result contained
current source citations. Validation observed the endpoint, model, native
`web_search` tool, authorization presence, and response status without
recording credentials, prompts, queries, or response bodies.

The validation used no separate `OPENAI_API_KEY`, enabled no browser-cookie
access, wrote no provider configuration, and left the operator's Pi credential
file byte-for-byte and metadata-identical. The extension's public result names
the coarse provider `openai`; the exact internal `openai-codex/gpt-5.4`
selection was therefore observed at the sanitized transport seam. After
publication, changing the bundled version requires a new Matty minor release
and the same complete validation.

Matty records each skill or web-dependent branch in a Web Contract outside the
imported skill files, classifying access as `required`, `optional`, or `none`.
A Web Preflight runs before the classified path produces effects. Unavailable
required access blocks only that path and is never silently replaced by model
knowledge. Unavailable optional access may continue only with explicit
disclosure that no web research was performed. The same behavior applies when
access fails after a successful preflight.

The parent agent receives the four certified Web Capability tools. Among
subagents, only `researcher` receives them. Explorer, designer, reviewer, and
worker rely on a researcher or the parent for external research. This boundary
does not reclassify the reviewer's read-only `gh` inspection or the worker's
project-local dependency installation as web research.
