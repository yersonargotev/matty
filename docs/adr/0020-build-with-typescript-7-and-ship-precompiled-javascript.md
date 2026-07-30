# Build with TypeScript 7 and ship precompiled JavaScript

TypeScript 7 is currently distributed as the native compiler preview
`@typescript/native-preview`, not as a stable `typescript` release. Matty uses
it as a development-only tool pinned to the exact version
`7.0.0-dev.20260707.2`. Upgrades are deliberate compatibility changes and must
pass the complete packed-package suite.

Matty source is organized around three seams:

- the domain diagnostic module owns compatibility facts and redacted status;
- the application module owns activation, command behavior, and the
  single-snapshot invariant;
- the Pi adapter translates the host extension interface into the application
  interface.

Tests use an in-memory adapter at the same application seam. Packed acceptance
uses the real Pi adapter.

`tsgo` compiles strict Node ESM source into `dist/` before packing. The npm
artifact contains precompiled JavaScript and declarations, not TypeScript
source or the compiler. Matty declares no build or installation lifecycle hook;
`pi install` only unpacks and loads the reviewed JavaScript.

This keeps preview compiler risk out of the Matty User's runtime and preserves
an Install-Safe Artifact. It also means a clean source checkout must run the
explicit build before `npm pack`; the acceptance harness enforces that order.
