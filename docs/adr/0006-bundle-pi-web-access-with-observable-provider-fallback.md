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

Version `0.15.0` is the candidate pin for Matty `0.1.0`. Phase 0 must validate
it with the Certified Pi Version, Certified Target, Reference Model Path,
authentication, and packed-package combination. Matty may replace the candidate
before publishing `0.1.0` if that validation fails. After publication, changing
the bundled version requires a new Matty minor release and the same complete
validation.

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
