# AGENTS.md — sh

Working agreements for AI coding agents in this repository. The parent
tree's `~/Developer/github.com/blairham/AGENTS.md` applies; this file
layers on top and wins on conflict.

## ⚠️ Read `CLEANROOM.md` first

This is an **independent implementation**. Never read another shell's
source while writing code here — not mvdan.cc/sh, not bash, not dash,
not zsh. Learn behaviour from POSIX, from vendor manuals, and from
running real shell binaries as oracles; write the behaviour down in
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
   the core grammar. This is modelled as a variant set on the parser.
2. **Semantic differences are conflicts.** Where dialects disagree about
   identical syntax — word splitting, array base, whether a failing
   special builtin is fatal — there is no subset relationship, only a
   switch. This is modelled as a named field on the semantics vector,
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
    docs/spec/        the wall: behavioural specs, in our own words
      oracle.md       how behaviour is learned from real binaries
      shell-matrix.md the measured feature matrix that set the core
    syntax/           lexer, grammar, AST  (public API)

Packages are public. This is a library others are meant to depend on,
not an internal detail of one shell.

## Make targets

`build`, `test`, `fmt`, `vet`, `lint`, `tidy`, `clean`, `check`.

`check` runs `fmt vet test`. Lint runs in CI, not pre-commit — it is too
slow for every commit. When in doubt run `make check` *and* `make lint`.

## Conventions

- **Formatter**: gofumpt, pinned in `go.mod`'s `tool` block, run as
  `go tool gofumpt`.
- **Linter**: golangci-lint v2, also `go tool`-pinned, config in
  `.golangci.yml`.
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

`main` is protected. All of these must pass before a merge, and branches
must be **up to date** with `main` first:

    Build, vet and test (ubuntu-latest)
    Build, vet and test (macos-latest)
    Lint
    Pre-commit

Merges are **squash only** — linear history is enforced, and force pushes
and branch deletion are blocked. Commits must be signed.

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

`make oracle-check` runs in CI in **report-only** mode: the golden record
is generated on one machine and a runner does not have the same builds of
the same shells, so some differences are legitimate. A check that is red
for a legitimate reason is one people learn to ignore. `make check` is the
gate locally, where the panel matches the record.

## Testing

Compatibility is proven by **differential testing against real shell
binaries**, never by importing another project's suite. A case is a
snippet plus the observed output and exit status of a real shell. The
snippet is ours; the output is a fact.

Third-party suites that must be run (bash's own `tests/`, GPLv3) are
fetched at test time and never committed.
