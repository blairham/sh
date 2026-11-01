# Security Policy

## What sh does and does not protect

sh is a shell. Running the code it is given, with the privileges of the user
who started it, is its job — a script that deletes files when run is not a
vulnerability. What it promises is narrower, and these are the promises a
report can hold it to:

- **Parsing is not running.** `sh -n`, `shfmt` and the `syntax` package read
  a script without executing any of it: no command, no substitution, no
  expansion with side effects.
- **The sandbox contains the shell, not the process tree.** A policy
  (`-deny`, a policy file — see `docs/design/sandboxing.md`) decides what the
  *shell itself* opens, runs and signals, including inside `eval` and
  `source`. A command it was allowed to start makes its own system calls, and
  nothing here sees them; `allow exec-unconfined` says so in its name.
  Containing children needs an OS sandbox beside this one.
- **A policy that cannot be fully read is refused.** An unknown directive, a
  relative pattern or a missing `version 1` fails the load; nothing is
  silently dropped.
- **The ACP front end** (`--acp`, `docs/design/acp.md`) runs commands for the
  client on the other end of its stdio, through the same gate. It opens no
  socket; whoever holds its stdio is its client.

## Supported versions

sh is pre-stable (`v0.0.x`) and ships continuously. Only the latest release
receives fixes.

## Verifying a release

Releases after `v0.0.22` are signed with [cosign](https://github.com/sigstore/cosign)
keyless signing: the signature is tied to the GitHub Actions workflow that
built the release, not to a key someone could leak. Earlier releases are
unsigned.

Releases are built by `.github/workflows/release.yml`, which runs the shared
release workflow in [blairham/.github](https://github.com/blairham/.github)
(`.github/workflows/go-release.yml`). The signing identity is that shared
workflow; the certificate also names this repository and the commit the
release was built from.

**Downloads.** `checksums.txt` is signed; it lists the digest of every archive.
Verify the signature, then the archives against it. A release is built by the
Release Please run on the push to `main` that merged its release pull request,
not by a tag push, so the certificate names `refs/heads/main` rather than the
tag. Pin it to the commit the tag points to instead:

```sh
VERSION=v0.0.39
# Or `git rev-parse "$VERSION^{commit}"` in a clone.
SHA=$(gh api "repos/blairham/sh/commits/$VERSION" --jq .sha)
cosign verify-blob \
  --certificate-identity-regexp '^https://github\.com/blairham/\.github/\.github/workflows/go-release\.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-github-workflow-repository blairham/sh \
  --certificate-github-workflow-sha "$SHA" \
  --bundle checksums.txt.sigstore.json checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

This holds for every release since the move to blairham/.github (`v0.0.31`
on): the ones before release-please were built by a push of the tag itself, at
the same commit. One exception: `v0.0.37`'s release was re-run by hand from
`main` at `e6c28a6`, so verify it with
`SHA=e6c28a619ce128527426bdd2bd9426c76752d7e5`. A release re-run that way is
signed from the commit it was dispatched at, which that run's page shows.

**Build provenance.** The same releases carry SLSA build provenance for every
archive — which workflow run, commit and tag produced it. It is in GitHub's
attestation store and attached to the release as `sh-$VERSION.intoto.jsonl`,
for checking offline with `gh attestation verify --bundle`:

```sh
gh attestation verify sh_Linux_x86_64.tar.gz --repo blairham/sh \
  --signer-workflow blairham/.github/.github/workflows/go-release.yml
```

The macOS binaries are additionally Developer ID signed and notarized.

**Tags released before the move to blairham/.github** (`v0.0.23` through
`v0.0.30`) were signed by this repository's own `release.yml`. Verify those
with `--certificate-identity "https://github.com/blairham/sh/.github/workflows/release.yml@refs/tags/$VERSION"`
in place of the identity flags above, without `--signer-workflow`; their provenance asset is `sh.intoto.jsonl`.

## Reporting a vulnerability

**Do not open a public issue.** Report it privately through GitHub:
[Security → Report a vulnerability](https://github.com/blairham/sh/security/advisories/new).

Please include the affected version or commit, what an attacker can do, and
the steps to reproduce. You should receive a response within a week.

In scope, among others:

- `sh -n`, `shfmt` or the `syntax` package executing anything, or panicking
  or hanging on input — a formatter run over an untrusted file must be safe
- a sandbox policy failing to refuse something the *shell itself* does —
  through `eval`, `source`, a redirection, a builtin, an alias, completion or
  any other route
- a policy file that loads while dropping or misreading a rule
- the ACP front end running anything its gate refused
- the history file or the block store being created readable by other
  users (both are `0600`), or written outside the paths configured for them

Out of scope: what a child process does after a policy allowed it to start
(see above), behavior that matches the real shell sh models for that dialect,
and anything that requires already controlling the user's rc files,
environment or `$PATH`.
