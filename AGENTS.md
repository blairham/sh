# AGENTS.md — sh

Working agreements for AI coding agents in this repository. The parent
tree's `~/Developer/github.com/blairham/AGENTS.md` applies; this file
layers on top and wins on conflict.

## ⚠️ Read `CLEANROOM.md` first

This is an **independent implementation**. Never read another shell's
source while writing code here — not mvdan.cc/sh, not bash, not dash,
not zsh. Learn behavior from POSIX, from vendor manuals, and from
running real shell binaries as oracles; write the behavior down in
`docs/spec/`; implement from the spec. `CLEANROOM.md` is binding and
non-negotiable, and breaking it destroys the only thing this repo is
for.

## What this is

A shell parser and interpreter in Go, whose **core language is the
common denominator of real shells** rather than either strict POSIX or
bash. Dialects — `posix`, `bash`, `zsh`, `ksh` — are presets over a
**semantics vector**, not layers, translations or forks.

Two claims follow, and both are load-bearing:

1. **Grammar differences are additive.** A dialect adds constructs to
   the core grammar. This is modeled as a variant set on the parser.
2. **Semantic differences are conflicts.** Where dialects disagree about
   identical syntax — word splitting, array base, whether a failing
   special builtin is fatal — there is no subset relationship, only a
   switch. This is modeled as a named field on the semantics vector,
   never an inline conditional.

The second is the reason this repository exists. Retrofitting it into a
bash-first interpreter is harder than designing it in.

## Why the core is the common denominator, not POSIX

Measured across dash, bash 3.2, bash 5.3, ksh93 and zsh (see
`docs/spec/shell-matrix.md`): dash is the sole holdout on 12 of 15
non-POSIX features. Excluding it yields a core containing arrays,
`[[ ]]`, `$'...'`, `+=`, substrings, pattern substitution, C-style
`for`, `function`, herestrings and process substitution.

Strict POSIX would reject all of those, and would also reject `local`,
which every shell in the panel except ksh93 provides. A core that
refuses what every real shell accepts is a core nobody can write against.

## Project structure

    CLEANROOM.md      the rules that keep this independent — read first
    docs/design.md    architecture: the semantics vector and dialects
    docs/spec/        the wall: behavioral specs, in our own words
      oracle.md       how behavior is learned from real binaries
      shell-matrix.md the measured feature matrix that set the core
    syntax/           lexer, grammar, AST — public
    interp/           execution, the semantics vector, the extension points
    dialect/bash/     one package per shell: Dialect, Semantics, Diagnostics
    dialect/zsh/      …
    dialect/ksh/      …
    dialect/dash/     …
    cmd/sh            the substrate's driver, with -dialect
    cmd/bash cmd/zsh  dialect binaries, as proof the library goal holds
    cmd/oracle        records what real shells do

**The core does not know its successors.** `syntax` and `interp` define the
questions — a grammar flag, a semantics axis, a diagnostic value — and each
shell answers them in `dialect/<shell>`. Nothing under `syntax/` or
`interp/` imports a dialect package or names a shell, so adding fish adds a
directory rather than editing the substrate; when a shell is split into its
own repository, its directory is the whole of it.

That rule reaches the tests. A test in `syntax` or `interp` names a *flag*
or an axis and never a shell; a test that asserts what bash does lives in
`dialect/bash`. The `interp` tests are an external `interp_test` package
for the same reason — a dialect package imports `interp`, so an in-package
test could not import a dialect without a cycle.

No preset derives from another shell. Each starts from POSIX — the
standard, which has no successors — and overrides only what was measured.
Deriving zsh from bash once gave zsh bash's answer for whether a readonly
reassignment is fatal, which is the opposite of zsh's own, and nothing
caught it until something exercised it.

**Packages start under `internal/` and are promoted once something has
consumed them.** `syntax` and `interp` are public now: the parser was built
on the first and the interpreter on both, which is the criterion. The oracle
stays internal because it is test infrastructure rather than product.

This is a library others import and extend — a dialect is built *on* the
core, not forked from it — and there are exactly three ways to do that:

1. **Choose the vectors.** `syntax.Dialect`, `interp.Semantics` and
   `interp.Diagnostics` are values. "Which shell am I" is data, not a branch
   in the code — see any package under `dialect/` for the whole of one.
2. **Register a builtin.** Only for what shell cannot express: `cd` must
   change the runner's directory, `read` must set a variable in the calling
   shell. Reach for Go when shell genuinely cannot say the thing.
3. **Source a prelude.** Everything else. Functions shadow builtins and
   external commands alike, so a dialect can define, replace or wrap
   anything the core provides without touching it — portably, testably, and
   without any way to break the substrate.

`interp/extend_test.go` builds a miniature dialect with all three, as a
working example rather than as prose.

## Make targets

`build`, `test`, `fmt`, `vet`, `lint`, `tidy`, `clean`, `check`.

`check` runs `fmt vet test`. Lint runs in CI, not pre-commit — it is too
slow for every commit. When in doubt run `make check` *and* `make lint`.

## Conventions

- **Formatter**: gofumpt, pinned in `go.mod`'s `tool` block, run as
  `go tool gofumpt`.
- **Linter**: golangci-lint v2, also `go tool`-pinned, config in
  `.golangci.yml` — the same file in every Go repository here.
- **Linters follow dependencies.** The shared config is the base. When a
  repository adopts a technology, the linter that understands it is added
  **in the same change as the dependency**, not later and not by someone
  noticing: protobuf → `protogetter`, `database/sql` → `sqlclosecheck` and
  `rowserrcheck`, testify → `testifylint`, prometheus → `promlinter`,
  `log/slog` → `sloglint`, OpenTelemetry → `spancheck`. A dependency
  arriving without its linter is an incomplete change. The reverse holds
  too: when the last use of a dependency goes, its linter goes with it.
- **US English, everywhere.** Comments, documentation, commit messages and
  identifiers. It is enforced rather than agreed — `misspell` is configured
  with `locale: US` in `.golangci.yml`, so British spelling fails the build
  rather than accumulating until someone minds.
- **Toolchain pin**: `go.mod`'s `go` directive and `.tool-versions`'
  `golang` must match exactly; `go.mod` is authoritative.
- **Tests**: `go test -race ./...`. Tests never touch real user state —
  redirect via `t.TempDir()` + `t.Setenv`.
- **Commits carry no AI-attribution trailers.**

## Licensing and file headers

The project is **Apache-2.0** (`LICENSE`, `NOTICE`). Contributions
require a signed CLA — see `CONTRIBUTING.md`.

**Every `.go` file starts with two lines, before the package clause:**

    // SPDX-FileCopyrightText: 2026 Blair Hamilton
    // SPDX-License-Identifier: Apache-2.0

Short-form SPDX, not the Apache appendix boilerplate — the appendix is
recommended rather than required, and the short form is what scanners
and the REUSE spec read.

This is not ceremony. A root `LICENSE` does not travel with a file that
is copied out of the repository; the header does. Per-file headers are
the reason attribution survives wholesale vendoring into somebody else's
tree, which is precisely the case worth defending against.

- Year is `2026` and stays there. Copyright runs from creation; a
  maintained year range is churn.
- Generated files (`// Code generated ... DO NOT EDIT.`) are exempt.
- Markdown and config carry no header; the root `LICENSE` covers them.
- Enforced by the `check-license-headers` hook from
  [blairham/pre-commit-hooks](https://github.com/blairham/pre-commit-hooks),
  pinned in `.pre-commit-config.yaml` and run in CI by the same config —
  one pinned version, not a copy of a script per repo.

## CI and merging

Work is checked in three places, and each does something the others
cannot.

**Local, on every commit.** The hooks in `.pre-commit-config.yaml`:
hygiene, secrets, licence headers, `go mod tidy`, the toolchain-pin
invariant, and golangci-lint in two forms — `golangci-lint-fmt` applies
every formatter the config names, and `golangci-lint` lints *what changed
since HEAD*. Seconds, not minutes, and it is the only feedback that
arrives before the code leaves the machine.

**On a pull request.** The gate, in two tiers.

`Pre-commit` runs first and runs *always*, draft or not, and everything
else waits on it. It is the only job that looks at every file rather than
at Go — trailing whitespace, licence headers, secrets, the toolchain pin —
none of which is worth detecting changes for, and a secret committed to a
draft is committed. A hook can be skipped and a contributor may never have
installed one, which is why it runs here as well as there.

Only once that passes is it worth asking the expensive question. `Detect
changed files` gates build and test with `-race` on Linux and macOS, and
the *whole-repo* lint — which is not the same run as the hook, because
`--new-from-rev` cannot see whole-module linters like `unused`, and
`unused` has caught dead code here that nothing else would have. Those
three stand down for a **draft**: push freely, and marking it ready starts
them.

The hook environments are cached, and that is not an optimisation to skip.
pre-commit builds an environment for a hook even when `SKIP` tells it not
to run one, which cost two and a half minutes a build installing a linter
this job then declines to use.

**After a merge.** One test job, on one platform — see `main-canary.yml`
for why that and nothing else.

`go vet` is deliberately not a step of its own: `govet` is one of the
linters `.golangci.yml` enables, so the lint job already runs it over the
same code.

`main` is protected. All of these must pass before a merge, and branches
must be **up to date** with `main` first:

    Build and test (ubuntu-latest)
    Build and test (macos-latest)
    Lint
    Pre-commit

Merges are **squash only** — linear history is enforced, and the merge and
rebase buttons are turned off so the UI cannot offer what protection would
reject. Force pushes and deletion of `main` are blocked, and commits must
be signed.

**Head branches are deleted automatically on merge**, so `--delete-branch`
is belt-and-braces rather than the thing that does the work. Remove the
sibling worktree when the pull request opens, not when it merges; the
branch will be gone by then either way.

Auto-merge is enabled, so a pull request can be queued to land the moment
its required checks go green.

**Build, test and lint only do work when Go changed.** A pull request
touching only `docs/` runs them as no-ops. Pre-commit always runs in full,
because what it checks — whitespace, YAML, secrets, licence headers —
applies to every file.

The gating is inside the jobs, not a `paths:` filter on the workflow, and
that is not a style choice: **a required check that never runs reports as
pending forever, not as passed**, so a paths-filtered required check makes
a docs-only pull request permanently unmergeable. The jobs always run and
report; only the expensive steps are skipped.

**Checks run on pull requests, not on pushes to `main`.** Because a branch
has to be up to date before merging, a squash merge lands the tree that
was already tested, so re-running deterministic checks afterwards tests
nothing new.

The exception is `main-canary.yml`, which runs the tests once on
`ubuntu-latest` after a merge, and only when the merge touched Go. Determinism is the whole argument above,
and races are not deterministic: a test can pass on a branch and fail on
`main` with the same tree. That has happened before and only a post-merge
run caught it, so one cheap job stays rather than the full matrix.

`make conformance` grades our own `sh` against a panel shell over the whole
corpus. It is the point of having built the harness: the corpus already
records what six real shells do, so pointing it at our binary turns every
case into a conformance test with no new expectations to maintain. Add
`ARGS=-v` to list what does not match — the passing set is a number and the
failing set is the work.

It is deliberately **not** a gate. The number is meant to be low and to
climb; failing CI on it would only mean failing CI on unfinished work.

`make oracle-check` runs in CI in **report-only** mode: the golden record
is generated on one machine and a runner does not have the same builds of
the same shells, so some differences are legitimate. A check that is red
for a legitimate reason is one people learn to ignore. `make check` is the
gate locally, where the panel matches the record.

## Never work on `main`

Every change starts with a sibling worktree, including a one-line fix:

    git worktree add ../sh-<topic> -b <topic> origin/main

The main checkout stays on `main` and stays clean, so it is always there to
compare against, to check whether something reproduces without your change,
and to branch the next piece of work from. Two changes in flight never
share a working tree.

This is not a preference about tidiness. Working directly on `main` is how
a local commit ends up rewritten to recover from a mistake, and how a
half-finished experiment ends up in the same tree as the fix you meant to
send. Remove the worktree when the pull request opens, not when it merges.

## Testing

Compatibility is proven by **differential testing against real shell
binaries**, never by importing another project's suite. A case is a
snippet plus the observed output and exit status of a real shell. The
snippet is ours; the output is a fact.

Third-party suites that must be run (bash's own `tests/`, GPLv3) are
fetched at test time and never committed.
