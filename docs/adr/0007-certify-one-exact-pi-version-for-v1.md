# Certify one exact Pi version and target for v1

Matty certifies only Pi `0.84.2` on macOS Apple Silicon. Other Pi versions or
targets leave Matty degraded until that exact combination passes the complete
packed-artifact suite. Supporting additional Pi lines, Linux, and Windows
remains a later goal, allowing Matty to prove the intended GPT-5.6,
ChatGPT/Codex OAuth, delegation, and Web Capability path before expanding the
compatibility matrix.

The initial v1 decision certified Pi `0.83.0`. The certification subsequently
migrated to exact Pi `0.84.2`, whose release commit is
`914cf1472e715297caa30db4b9535d534a9eb718`. This migration replaces the
current peer, lock tree, runtime provenance, acceptance pins, and release
evidence; it does not rewrite the historical `0.83.0` adaptation and proposal
evidence.

Production host detection is launcher-derived and fail-closed. Matty
canonicalizes `process.argv[1]`, finds the nearest `package.json` whose name is
`@earendil-works/pi-coding-agent`, and uses only its valid string version.
Matty does not treat its own locally resolved peer as proof of the launcher Pi
version. Missing or malformed launcher metadata therefore leaves activation
degraded.

The exact Reference Model Path is `openai-codex/gpt-5.6-sol` with
ChatGPT/Codex subscription authentication, not a core activation allowlist.
Phase 0 originally validated that identifier in a real Pi `0.83.0` process
that loaded the packed Matty artifact alongside the separately installed,
exact `pi-web-access@0.15.0` extension. In that isolated process, the web
extension returned a current, cited result through the subscription-backed
OpenAI search path. Current release certification re-runs the live gates
against exact Pi `0.84.2`.

Matty inherits the active provider, model, authentication, and reasoning
settings from Pi. A different model is reported as unverified in status and
doctor but does not degrade core activation. Only failure of a concrete
workflow capability preflight may block the affected path.
