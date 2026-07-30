# Certify one exact Pi version and target for v1

Matty v1 certifies only Pi `0.83.0` on macOS Apple Silicon, the version and
target selected for the initial real-process acceptance suite. Other Pi
versions or targets leave Matty degraded until that exact combination passes
the complete suite. Supporting additional Pi lines, Linux, and Windows remains
a later goal, allowing the MVP to prove the intended GPT-5.6, ChatGPT/Codex
OAuth, delegation, and Web Capability path before expanding the compatibility
matrix.

The exact v1 Reference Model Path is
`openai-codex/gpt-5.6-sol` with ChatGPT/Codex subscription authentication, not
a core activation allowlist. Phase 0 validated that identifier in a real Pi
`0.83.0` process that loaded the packed Matty artifact alongside the separately
installed, exact `pi-web-access@0.15.0` extension. In that isolated process, the
web extension returned a current, cited result through the subscription-backed
OpenAI search path.

Matty inherits the active provider, model, authentication, and reasoning
settings from Pi. A different model is reported as unverified in status and
doctor but does not degrade core activation. Only failure of a concrete
workflow capability preflight may block the affected path.
