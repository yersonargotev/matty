# Publish publicly with OIDC and provenance

Matty uses a Public Distribution: the public canonical source repository
`https://github.com/yersonargotev/matty` and the public scoped npm package
`@yargote/matty`. Package metadata explicitly sets public access and identifies
that case-exact publishing repository.

Routine releases use a Staged Release from the canonical GitHub Actions
workflow on a GitHub-hosted runner. The npm trusted publisher uses OIDC, holds
only `npm stage publish` authority, and has no long-lived write token or direct
`npm publish` permission. Each functional release carries npm provenance
linking its package artifact to the public source and build instructions.
Release CI must satisfy npm's current trusted-publishing toolchain and
workflow-permission requirements.

For every release after `0.1.0`, CI submits the exact validated artifact to npm
staging. A maintainer downloads and inspects that staged tarball, then approves
it interactively with 2FA or rejects it. The release record retains the staged
identifier, artifact digest, inspection result, approver, and final registry
result. CI cannot make the package public by itself.

npm currently requires a package to exist before its trusted publisher can be
configured. The initial registry registration is therefore a distinct,
explicitly reviewed Bootstrap Publication. The canonical GitHub Actions
workflow publishes `@yargote/matty@0.1.0` with
`npm publish --provenance --access public`, using a granular npm token with the
minimum available scope, permissions, and expiration. The token exists only as
a protected workflow secret and must not appear in logs.

Immediately after publication, maintainers verify `0.1.0`'s public provenance,
configure that workflow as the stage-only npm OIDC trusted publisher, disallow
token-based publication, revoke the bootstrap token, and remove its workflow
secret. The release record captures evidence of every step. Matty publishes no
placeholder version, and no later release may use this bootstrap path.
