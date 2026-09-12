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

All of those are implemented, process substitution last: `<(cmd)` needs
a path the child can *open*, which is plumbing rather than grammar. It
is a named pipe, after `/dev/fd` was tried and rejected — that needs the
descriptor to survive `exec`, and clearing Go's close-on-exec flag leaks
it into every later command, so a `sleep` after the substitution holds
the pipe open and the reader never sees end-of-file. Every item here is
a statement about the code and not only about the matrix — which it was
not, for a while: `$'...'` recorded its quoting and never decoded the
escapes, `+=` was read as a command name, C-style `for` did not parse,
and the four were found by *running scripts* rather than by the corpus,
because nothing in the corpus used them.

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
    driver/           the front end: -c, a script file, argv[0], exit status
    dialect/bash/     one package per shell: Dialect, Semantics, Diagnostics, Style
    dialect/zsh/      …
    dialect/ksh/      …
    dialect/dash/     …
    dialect/ash/      …
    cmd/sh            the substrate's driver, with -dialect
    cmd/bash cmd/zsh  dialect binaries, as proof the library goal holds
    cmd/dash cmd/ksh  …
    cmd/ash           the fifth, and the one no instrument grades — there is
                      no BusyBox ash on a macOS machine, so it has no oracle
                      column; docs/spec/ash.md says what that costs
    cmd/shfmt         the formatter: the first thing that consumes the
                      parser as a product rather than as test scaffolding
    internal/cmd/     the instruments: not shipped, and under internal/
                      so nothing outside the module can import them
      oracle          records what real shells do
      smoke           drives a session through a terminal, per-feature
      wild            parses the shell scripts installed on this machine
      corpusguard     fails when the corpus has lost a case
      fmtwild         lays out every script on the machine and checks
                      that nothing but the layout changed
      acpcheck        speaks the Agent Client Protocol to the shipped
                      binary, as a client, over a pipe

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

1. **Choose the vectors.** `syntax.Dialect`, `interp.Semantics`,
   `interp.Diagnostics` and `syntax.Style` are values. "Which shell am I" is
   data, not a branch in the code — see any package under `dialect/` for the
   whole of one. `Style` is the fourth and the thinnest: it answers what a
   *formatter* needs and a parser never asks, and it earns its place on one
   question — zsh's brace-spelled bodies parse to the tree the keyword
   spelling parses to, so only a stated preference can say which spelling is
   written back. `docs/spec/style.md` holds the measurements.
2. **Register a builtin.** Only for what shell cannot express: `cd` must
   change the runner's directory, `read` must set a variable in the calling
   shell. Reach for Go when shell genuinely cannot say the thing.
3. **Source a prelude.** Everything else. Functions shadow builtins and
   external commands alike, so a dialect can define, replace or wrap
   anything the core provides without touching it — portably, testably, and
   without any way to break the substrate. A function it defines *is* the
   shell: what it reports is located where the script called it and named
   after it, and `diagnose` is how it raises a complaint of its own. See
   `interp/prelude.go` — before that, a prelude builtin's refusals were
   silently mislabeled, and a test that pasted the prelude on the front of
   its snippet could not see it (#603).

`interp/extend_test.go` builds a miniature dialect with all three, as a
working example rather than as prose.

## The core is a library, so it may not touch the process

**Nothing under `interp/` may change process-wide state.** Not the working
directory, not the environment, and not the process image. A Runner is
embedded in other programs, and two of them in one program must not fight
over one cwd — which is why the core's `cd` sets `r.Dir` and never calls
`os.Chdir`, and why `$PWD` is read from `r.Vars` rather than from the
environment.

`exec` is the sharpest case: replacing the process is exactly right for a
shell and catastrophic for a library, so `Runner.ReplaceProcess` is a hook
that `interp` never fills in. `driver` supplies `syscall.Exec`, because a
binary that *is* a shell is the one place the call is correct. If a feature
seems to need `os.Chdir`, `os.Setenv` or `syscall.Exec`, it belongs behind a
hook and the implementation belongs in `driver`.

`kill` is the second one, arrived at from the other side. A signal a script
aims at the shell itself is delivered to the shell's own traps and never
leaves the process: routing it through the kernel would let a line of script
fire the *embedder's* signal handlers, and it bought nothing, because a shell
that has just sent a signal already knows it sent one. Asking `os/signal` to
confirm it is asking a question we hold the answer to — the PATH rule again —
and the answer came back on a goroutine, which made a trapped signal's
delivery a matter of scheduling and lost it outright under load. An untrapped
*fatal* signal is the same split as `exec`: the core stops the script and
`Runner.DieBySignal` — nil in a library, filled in by `driver` — ends the
process.

The corollary caught a real bug: **where a real shell relies on process
boundaries, we have to reconstruct the boundary by hand.** Our subshells are
cloned Runners in one process, so `exec` inside one must not call execve —
it would take the parent shell with it. `docs/spec/semantics.md` has the
long version.

**Nor may it ask the process a question it holds the answer to.** `PATH` is the
worked example: `os/exec`'s `LookPath` reads the *process's* `PATH` and resolves
relative entries against the *process's* directory, and a Runner holds both
itself. Delegating to it produced a shell where `PATH=/tmp/mine; mycmd` found
nothing and `PATH=; anything` still found everything on the developer's machine.
`interp/lookpath.go` does the search against `r.Vars` and `r.Dir` instead.

The tell is the same every time: if a package under `interp/` reaches for
`os.Getenv`, `os.Getwd`, or anything in `os/exec` that consults them, it is
about to borrow state the Runner already owns.

## One driver, four dialects

**Nothing outside `driver/` implements how a shell is invoked.** Reading
`-c` or a script file, naming the shell from `argv[0]`, naming a *script*
by its path in a diagnostic, handing the script its positional parameters,
wording a parse failure the dialect's way and exiting with the dialect's
status — that is one front end taking a `driver.Shell`, which is the three
vectors plus the two extension points.

That includes **deciding to prompt**. `-i`, and no operands with a
terminal on standard input, mean a person rather than a script — read
in the same place as everything else about an invocation, so a dialect
binary has a prompt by existing. It was in one binary's main once, and
`./bash -i` answered `unknown option "-i"`.

Which operand becomes `$0` is the front end's to know and differs by route:
a script's path is `$0` and the operands after it are `$1` onward, `-c`
takes the *first* operand as `$0` and the rest as parameters, and standard
input leaves `$0` as the shell. All four shells agree on all three. The
interpreter had positional parameters long before anything gave it any,
which is a reminder that a feature is not reachable until the front end
reaches it — `$#` was 0 however the shell was invoked, and most real
scripts do nothing useful with no arguments.

It is public rather than under `internal/` on purpose, and this is the one
exception to "packages start under `internal/`". A dialect built outside
this repository needs a front end as much as it needs a semantics vector;
if this were internal, copying the file would be the only way to ship a
dialect binary, and `cmd/bash`'s reason for existing — proving the core is
a library rather than a program with options — would be false.

That rule exists because it was broken. Every binary had its own copy;
`cmd/sh` learned to run a script file and the dialect binaries never did,
so `make conformance-dialects` graded the *drivers* and reported bash at
178/198 when the core scored 198/198. Fourteen of the twenty failures were
literally `bash: -c is required`. Adding a front-end feature to one
binary, or writing a fifth copy for a new dialect, brings that straight
back.

Each `cmd/<shell>` still builds its own `driver.Shell` from its own
dialect package rather than looking one up in a shared registry. That
duplication is deliberate: a central registry of every dialect is exactly
what a binary meant to be liftable into its own repository must not need.

## Make targets

`build`, `test`, `fmt`, `vet`, `tidy`, `clean`, `check`, `install`,
`uninstall`.

`check` runs `fmt vet test corpus-guard oracle-check`. **That is your
gate — the only one.**

**There is no `lint` target, and you must never run `golangci-lint`
yourself.** Not `go tool golangci-lint run`, not wrapped in a script, not
"just once to check before I commit". In this repository it is **not** a
pre-commit hook — `.pre-commit-config.yaml` records the measurement that
took it out — so it runs in exactly one place: the **`Lint` job in CI**,
against the merge base. A run you start by hand tells you nothing the pull
request would not tell you minutes later.

The target was deleted rather than documented-as-discouraged because
discouragement did not work: the sentence it replaced said "when in doubt
run `make check` *and* `make lint`", and three agents briefed off this file
did exactly that at once. golangci-lint defaults its concurrency to
`NumCPU` and takes gigabytes of RSS per run, so several agents each
starting their own is what has repeatedly driven this 18-core machine past
load 20 and starved the work it was running. **Agent count was never the
lever; the per-agent footprint was.** If lint is too heavy, lower
`run.concurrency` in `.golangci.yml` — do not run fewer agents, and never
run lint locally to get it over with early.

**When CI's `Lint` job reports a failure, fix it and push again.** That is
the only lint output anybody should be reading.

The report targets are `conformance`, `conformance-gated`,
`conformance-dialects`, `wild`, `wild-run`, `wild-run-contained`, `smoke`,
`acp`, `acp-wire` and `sandbox`. Each is described below. Only `sandbox`
fails on what it finds, for the reason given there: a conformance number is
meant to be low and climbing, and a boundary is meant to hold.

## Installing, and the name collision

`make install` builds all six binaries and puts them in
`$(PREFIX)/libexec/sh`, keeping their plain names. `docs/install.md` is
the user-facing half; the rule to hold on to is this one:

**The binaries are called `bash`, `zsh`, `ksh`, `dash`, `ash` and `sh`, and
the name collision is both the point and the hazard.** A shebang, `chsh` and
`login` all name a shell — by path, or by an `argv[0]` of `-bash` that
`driver.LoginShell` reads — so renaming them to `sh-bash` would break the
login route, `$0` in diagnostics, and every shebang. What moves instead
is the *directory*: `libexec` is off `PATH`, so nothing on the machine
resolves one of these by accident. `make install` refuses a `SHELLDIR`
that is on `PATH` unless `ALLOW_PATH_SHADOW=1` is passed with it, and
`GOBIN` is never consulted — which is also why `go install ./cmd/...` is
the one Go command not to run in this repository.

The Homebrew formula in `.goreleaser.yaml` makes the same choice: it
installs into the keg's `libexec` and links nothing into `bin`. Anything
that would put these names on a `PATH` is a change to argue for, not a
tidy-up.

## Release

GoReleaser on a `v*` tag, per the parent tree: green CI on `main` → tag →
`.github/workflows/release.yml` → `blairham/homebrew-tap` updated. One
archive per platform carrying all six binaries, Linux and macOS only —
every route into this program is a POSIX one. The first tag is `v0.0.0`.

`cmd/sh`'s `version` is a var rather than a const so the tag can be
stamped over it with `-X main.version=`; it is what `-acp` reports to a
client, and a checkout says `0.0.0-dev`.

### What v0.0.0 waits for, and how to count it

The bar is **countable, not a judgement**: open `P1` = 0 **and** open
`daily-driver` = 0. Nothing else gates the first tag.

That makes the labels load-bearing, so the *counting method* is part of the
definition. Three ways the obvious query has lied here, each of which has
been believed at least once:

- **`gh issue list` returns 30 rows by default.** A board of exactly 30 does
  not look truncated, it looks like a count. Always pass `--limit` well above
  the expected total and take the length with `jq`, never by eye.
- **A label that does not exist returns 0**, identically to a label with no
  issues. `gh issue list --label utter-nonsense` is `0`, and **exits 0**.
  Confirm the label is in `gh label list` before reporting its count as met.

  Note that cross-checking with a second instrument does **not** catch this
  one: `gh search issues --label utter-nonsense` also answers `0`. Two
  instruments agreeing is good evidence against a *transport* fault — a
  truncated page, a stale index right after a label edit — and no evidence
  at all against a *question* fault, where both are faithfully answering
  something other than what was asked. Only `gh label list` distinguishes
  "no issues have this label" from "nothing has ever had this label".
- **An issue with no `P1`/`P2`/`P3` is invisible to the gate.** This is the
  one that keeps happening: the agent best placed to label an issue is the one
  least likely to, because filing is its last act. In a single session the
  unprioritised count drifted to 5, then 2, then 6 — each time from issues
  filed by an agent as it finished, and twice it made the bar read lower than
  it was.

So **re-run this before quoting either leg**, every time:

    gh issue list --state open --limit 400 --json number,labels \
      --jq '[.[]|select([.labels[].name]|any(.=="P1" or .=="P2" or .=="P3")|not)]|length'

It must be `0`, or the bar is measuring a subset of the board.

**And a label count is not a working shell.** "Bar met" was once reported
from two green counts while `go run ./cmd/zsh` still wrote 48 lines to
stderr. The second half of the check is the driver itself, against a real
`~/.zshrc`, compared with the real shell run identically:

    go build -o /tmp/zq ./cmd/zsh && /tmp/zq -i -c exit 2>&1 \
      | sed -E 's/[0-9]+/N/g' | sort | uniq -c | sort -rn

Note that the real shell's own line count is **not** a stable constant — it
varies with whether the caller has a job-control terminal — so compare *which*
diagnostics appear, not how many. And a scripted `-i -c exit` is not an
interactive session: a plugin manager's deferred loads run past the first
precmd, and a shell can pass this and still never reach a prompt. Where a
fault shows only in a real terminal, use the pty harness and say so.

## Conventions

- **Formatter**: gofumpt, pinned in `go.mod`'s `tool` block, run as
  `go tool gofumpt` — by `make fmt`, and at commit time by a pre-commit
  hook that runs the same pinned binary and *reports* rather than
  rewrites (`-l -d`, never `-w`).
- **Linter**: golangci-lint v2, also `go tool`-pinned, config in
  `.golangci.yml` — the same file in every Go repository here. **It is
  invoked by CI's `Lint` job and by nothing else** — not by a pre-commit
  hook in this repository, and never by you; see *Make targets* above for
  why there is no `lint` target and why you must not start one. Two settings in that file matter. `run.concurrency`
  bounds the footprint, and is the dial to turn if lint is too heavy.
  `run.allow-parallel-runners` clears the `$TMPDIR` file lock that
  otherwise makes the second concurrent run *refuse* — which used to fail
  a commit for a reason unrelated to the commit, since several worktrees
  of this module commit at once. The lock is not in the cache directory,
  so a per-worktree `GOLANGCI_LINT_CACHE` does not move it; the cache is
  deliberately left shared, since its keys and the positions it stores are
  module-relative and a warm shared cache lints this repository in under a
  second against 37 seconds for a cold one.
- **Linters follow dependencies.** The shared config is the base. When a
  repository adopts a technology, the linter that understands it is added
  **in the same change as the dependency**, not later and not by someone
  noticing: protobuf → `protogetter`, `database/sql` → `sqlclosecheck` and
  `rowserrcheck`, testify → `testifylint`, prometheus → `promlinter`,
  `log/slog` → `sloglint`, OpenTelemetry → `spancheck`. A dependency
  arriving without its linter is an incomplete change. The reverse holds
  too: when the last use of a dependency goes, its linter goes with it.
- **US English, everywhere.** Comments, documentation, commit messages and
  identifiers. Enforced rather than agreed, and enforced in **two** places,
  because one was never enough: `misspell` with `locale: US` in
  `.golangci.yml` reaches Go and only Go, so every `.md` under `docs/` —
  which is most of the prose this rule was written for — sat outside the
  rule it was cited for, and British spellings accumulated in the spec with
  the linter green (#541). A `misspell` pre-commit hook covers `.md` and the
  YAML configs; it reports and does not rewrite, because unlike gofumpt it
  can reach inside a string literal. Two words are exempt and neither is
  prose: `colour`, one shell's parameter **name**, and `cancelled`, the
  Agent Client Protocol's spelling of a `stopReason`.
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
hygiene, secrets, license headers, `go mod tidy`, the toolchain-pin
invariant, the conflict-marker scan, gofumpt, and misspell over prose.
golangci-lint is **not** among them, on purpose; the measurement that took
it out is recorded in `.pre-commit-config.yaml`. Seconds, not minutes, and
it is the only feedback that arrives before the code leaves the machine.

**The formatter is back, and this file used to be wrong about it twice
over.** The hook that applied gofumpt and goimports went with golangci-lint
in #1495, on the understanding that `go-fumpt-repo` would carry them — and
that hook had never been in this repository's config, so for the year that
followed there was no formatter on the commit path at all while two files
said there was. #1903 put one there. It is `my-cmd-repo` running `go tool
gofumpt`, not `go-fumpt-repo`, because that hook wants a `gofumpt` on PATH
and this repository deliberately has none: the pin in `go.mod` is what makes
the hook, `make fmt` and CI the same formatter. Neither new hook rewrites
anything — both report — so neither reopens the window #1267 is about.

**On a pull request.** The gate, in two tiers.

`Pre-commit` runs first and runs *always*, draft or not, and everything
else waits on it. It is the only job that looks at every file rather than
at Go — trailing whitespace, license headers, secrets, the toolchain pin —
none of which is worth detecting changes for, and a secret committed to a
draft is committed. A hook can be skipped and a contributor may never have
installed one, which is why it runs here as well as there.

Linting happens in a **job of its own**, `Lint`, and only there — it does
not belong at `git commit` time, for the cost recorded above.

It lints the **whole tree**, on a pull request and on a push to `main`
alike. It used to diff from the merge base, so that a branch was graded on
its own change and not on debt it inherited, and that had a cost that was
invisible in the worst direction: on `main` the merge base *is* HEAD, the
diff is empty, and the run examines nothing — so a finding that arrives
through a merge is never reported by anything again. Five accumulated that
way and were found by a hand-run linter (#1640), and the same scoping is
why `make lint` used to fail on a clean checkout for reasons that were
nobody's, which is what teaches people to discount a linter (#1302).

A diff-scoped run also fails **green**. golangci-lint prints `0 issues` and
exits 0 when the base ref does not resolve — a shallow clone, a renamed
base — so the strictest check is the one that passes when it is broken. A
full-tree run has no such mode. It also no longer needs `fetch-depth: 0`,
since nothing computes a merge base.

The trade is that inherited debt now blocks a pull request, which is
affordable only while the tree is clean. Keeping it clean is the point: if
a finding lands, fix it rather than narrowing the run again.

Only once pre-commit passes is it worth asking the expensive question.
`Detect changed files` gates build and test with `-race` on Linux and
macOS. Those stand down for a **draft**: push freely, and marking it ready
starts them.

The hook environments are cached, and that is not an optimisation to skip.
pre-commit builds an environment for a hook even when `SKIP` tells it not
to run one, which cost two and a half minutes a build installing a linter
this job then declines to use.

**After a merge.** The same full set, on `main`. `ci.yml` fires on `push`
to `main` as well as on pull requests, and every job runs — a merge is not
narrowed by diff, because the thing a post-merge run is protecting against
is an earlier merge nobody has exercised since. `main`'s concurrency group
is keyed on the commit, so merges do not cancel each other (#1900). This
was a genuine gap for a long time: `main-canary.yml` was deleted in #53
(`280f8394`) and nothing replaced it until #1905 put the push trigger on
`ci.yml`, and `Lint` was still blind on `main` until #1640 made it a
full-tree run. `drift.yml` is a different question again: weekly,
deliberately non-gating, and watching the oracle *panel* rather than this
tree.

`go vet` is deliberately not a step of its own: `govet` is one of the
linters `.golangci.yml` enables, so the lint job already runs it over the
same code.

`main` is protected, and these three must pass before a merge:

    Build and test (ubuntu-latest)
    Build and test (macos-latest)
    Pre-commit

**A branch does not have to be up to date with `main` first.**
`required_status_checks.strict` is **`false`** — read off the API on
2026-09-11, after this document had asserted the opposite for long enough
that an agent rebased onto a `main` moving every few minutes to satisfy a
rule that was not there. So a `BLOCKED` pull request is waiting on a
**check**, never on a rebase. Rebase when you want the newer tree, not to
be allowed to merge.

`Lint` and `Oracle (report only)` run on every pull request but are **not**
required contexts. Read them anyway: `Lint` is where golangci-lint lives,
and it catches real defects a green build does not.

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
because what it checks — whitespace, YAML, secrets, license headers —
applies to every file.

The gating is inside the jobs, not a `paths:` filter on the workflow, and
that is not a style choice: **a required check that never runs reports as
pending forever, not as passed**, so a paths-filtered required check makes
a docs-only pull request permanently unmergeable. The jobs always run and
report; only the expensive steps are skipped.

**Why the run on `main` is not redundant.** The argument for testing only
pull requests was that a branch has to be up to date before it merges, so a
squash lands a tree that was already tested. That premise is false here:
`strict` is `false`, so a branch may merge from behind, and a pull request
is tested against its **merge base** rather than against the tree the squash
produces. A defect arising from the *interaction* of two separately-green
pull requests has nothing else to catch it. Races make the same point
without needing two branches — a test can pass on a branch and fail on
`main` with the same tree, and that has happened here.

That is also why `main`'s concurrency group is keyed on the commit rather
than on the ref. Canceling a superseded pull-request run discards a stale
answer; canceling a `main` run discards the only answer that commit will
ever get. Measured 2026-09-11, before #1900 fixed it: **nine of the last ten
`main` runs were `cancelled`**, one succeeded.

**So `conclusion: cancelled` is not `success`.** Before saying `main` is
green, check that the run completed — `gh run list --branch main --json
status,conclusion`.

`make conformance` grades our own `sh` against a panel shell over the whole
corpus. It is the point of having built the harness: the corpus already
records what six real shells do, so pointing it at our binary turns every
case into a conformance test with no new expectations to maintain. Add
`ARGS=-v` to list what does not match — the passing set is a number and the
failing set is the work.

`make conformance-gated` runs the same corpus twice through the same binary
— once plain, once under a sandbox policy — and reports the cases that
answer differently. It is how the boundary gets exercised by fourteen
hundred cases written for other reasons entirely, rather than only by unit
tests written by somebody who already knew where the boundary was.

The policy confines *writes* to the scratch directory each case is given
and leaves reads and execs alone, because an allowed program is outside
the boundary the moment it starts and a policy refusing reads while
permitting programs is telling itself a story. A stricter one is a
`-policy` flag away; deny-everything moves a third of the corpus and is a
wall rather than a signal. `docs/design/sandboxing.md` has the numbers and
what each difference is.

`make wild-run-contained` is `wild-run` with the same posture: the shell
under test may write only in the directory its run was given. That is what
turns the sweep's containment from a statement about the *arrangement*
into a statement about the *shell* — and the first run of it found the old
argument was not true. "A directory of its own" is where a script is
*started*, not where it can write: three of the 252 scripts on this
machine write to a temporary file they name themselves, and the policy is
what catches them.

`make conformance-dialects` grades all four dialect binaries against the
shells they claim to be. Each scores exactly what `make conformance` scores
for the same dialect, because both routes run the same core through the same
front end. **If the two ever disagree, the difference is a driver bug and not
a dialect one**; that is the whole reason the front end is shared, and
comparing the two numbers is the cheapest way to notice.

The numbers themselves are not written down here. The corpus grows with
almost every change, so a figure in this file is stale within a day and reads
as a target; the invariant — that the two routes agree, per dialect — is what
stays true. Run the targets for the current scores.

It is deliberately **not** a gate. The number is meant to be low and to
climb; failing CI on it would only mean failing CI on unfinished work.

`make axis-sweep` moves every axis in `interp.Semantics` to each of its
other legal values, runs the graded corpus against the shell each dialect
claims to be, and reports the axes **nothing objected to**.

An axis records a measured disagreement between real shells, so one that can
be flipped with nothing failing is not an untested code path — it asserts a
fact nobody checked, and reads exactly like one that was measured. Four were
found vacuous in a single day, each by accident, and `make check` was green
for all four because it does not run the grader.

The enumeration is derived from the struct by reflection and from the
constants the source declares, never from a list: a field whose type the
sweep cannot move is an **error**, because a silent skip reports as "nothing
objected" and files a bug against an axis nobody touched. That is the third
instance here of a guarantee that was *checked* rather than *enumerated*, and
so was only as wide as the set it walked.

It exits nonzero when it finds something, which it is expected to. It is
**not** in `make check` and must not be — thousands of processes against
three thousand rows — and `docs/spec/semantics.md` has the four-way triage
its output needs: an unpinned axis is a missing row, an axis the corpus
cannot reach, **a disagreement that is not there**, or an axis whose *type*
is too narrow for the panel — and only re-measuring the shells tells you
which. `ARGS='-only <Field>'` sweeps one axis while triaging it.

`ARGS=-presets` is the cheap half and takes about a second: it starts no
shell at all and asks the four dialect vectors what they hold, which finds
the one shape the flip test structurally cannot — an axis whose two values
both reach code, so a flip fails something, while one of them stands for a
reading no shell has (#2029). It reports the axes every dialect answers
alike and the legal values no dialect holds. Both are lists to re-measure,
not verdicts: a preset is not the whole of a dialect, and zsh's `localtraps`
reaches an answer no preset holds.

`make startup` times process start to a first prompt, and the `-c` path a
script's every subshell pays, against the real shells on the same machine in
the same minute. `internal/startupcost` is the harness; it uses a
pseudo-terminal, because a prompt is exactly the thing a shell declines to
draw without one.

Not a gate, and no threshold is committed: a startup time is a fact about
the machine as much as about the code, so what is in the tree is the method
and `docs/design/startup.md` records one run of it. Two measurement traps are
written down there — a benchmark that timed a shell's *teardown* as part of
its startup and reported a reference shell twenty-five times slower than it
is, and the system-wide rc files — Ubuntu's for bash, macOS's for zsh — that
silently replaced the prompt the harness watches for.

`make wild` parses the shell scripts installed on the machine and reports
the ones this parser cannot read. It reads and never runs — the reference
shell is consulted with `-n` — which is what makes it safe to point at
/usr/bin, and a file the reference also refuses is not counted, because a
Tcl program with a `#!/bin/sh` shebang is not evidence about us.

It exists because **the corpus cannot find this class of bug**. Every case
in it is a snippet written to pin one behavior down, so it proves that what
it asks about is right and says nothing about what nobody thought to ask.
Positional parameters were missing entirely while the corpus stood at 100%:
it invokes everything with `-c` and no operands, so `$#` is legitimately 0
in every case it has. Incremental execution and a here-document bug that
silently ate apostrophes were found the same way. Scripts in the wild ask
different questions, in bulk, and were written without knowing this
implementation exists.

Five things about how it looks are worth knowing, because each was a blind
spot before it was a flag.

**It sweeps two populations and names both.** `-dirs` holds the installed
programs — `/bin`, `/etc`, the package manager's trees — and `SH_WILD_DIRS`
holds the plugin and framework trees a shell *sources at startup*, separated
like `PATH`. The second has no default and never will: the path is one
machine's, one plugin manager's and one person's, and baking this machine's
layout into a binary is an argument already lost elsewhere in this tree. An
absent root is skipped, so CI sweeps exactly what it swept before. The split
exists because `make wild` reported **0 failures** on a day a real plugin tree
held 22 parse failures over three bugs — a regression guard over one
population being quoted as coverage of the other. A population that was not
swept is now printed saying so, because the one nobody mentions is the one a
reader assumes was included.

**It reports causes, not failures.** Failures come out grouped by what went
wrong — the kind, and the token where the token is one the grammar knows —
ranked by how many scripts each accounts for, with the offending line of one
of them underneath. A list of sixty paths cannot tell a reader whether that
is sixty problems or three, and the sweep is the only thing that can. `-v`
still lists every path under its cause.

**It descends, and it follows links.** On a machine with a package manager
almost everything worth sweeping is behind one: `/opt/homebrew/bin` is links
to files and `/opt/homebrew/opt` is links to directories, and git keeps two
dozen shell helpers below the second kind. `-depth` bounds the walk, and a
file reachable by two links is counted once, by its resolved path.

**`-dialect zsh` sweeps zsh instead**, which changes three things at once:
the grammar, the shebangs that count, and the reference shell. They move
together — grading zsh on `#!/bin/sh` files, or letting bash decide whether a
zsh script is a shell script, answers a different question.

**A zsh function has no shebang.** Zsh reads its functions rather than
execing them, so they say what they are with `#compdef` or `#autoload` on
the first line. Looking only for `#!` found eleven zsh scripts on a machine
holding several dozen, and the ones it missed — every completion a package
installs — are the most interesting zsh on the machine. A framework file
declares nothing at all: it is *sourced* by a line in somebody's startup file,
so the name is the only evidence, and `.sh`, `.bash`, `.zsh` and `.zsh-theme`
are read as what they say when the first line is silent. Without that, a real
plugin tree yielded 41 files while holding 123 `.zsh` — a root added and the
population still missing. A shebang still wins where there is one, since the
kernel obeys it and a `.sh` file starting `#!/usr/bin/perl` is perl. `.bats`
is deliberately excluded: it is a suite in a language that is bash with a
`@test` header, and it is neither ours to read nor ours to parse.

The deny-list draws one further line, and it is a fine one.
`/opt/homebrew/Cellar/zsh/5.9.2/…` and `/usr/share/zsh/5.9/functions` are
zsh's own code, which CLEANROOM.md's red list covers as squarely as the C
source; `/opt/homebrew/share/zsh/site-functions` has the same name and holds
git's completion, brew's and docker's, which are third-party programs that
happen to be written in zsh. The tell is what sits beside the name: a
package root above it (`Cellar/zsh`, `opt/bash`) or a version below it
(`zsh/5.9`) means the distribution; neither means a place the shell looks.

`make wild-run` goes further and **runs** each script that parses, under
both shells with the same arguments, comparing what they produce. It is the
half a reader cannot do, and it is where the last several gaps came from:
`set -f` was invisible to the parse sweep and only turned up because
/usr/bin/man parsed and then failed on it.

It is opt-in, and the reason is worth stating plainly: **it executes third
party code.** A `--help` is read-only by convention and not by guarantee.
The containment is that each script runs under *both* shells with the same
arguments, in a directory of its own, with no standard input and a timeout —
so whatever a script does, it does the same twice, and the comparison is
between two runs rather than between a run and an expectation.

It is also slow — two shells times two probes times every script, with a
timeout on each — so a whole-machine run takes minutes. `ARGS='-dirs
/usr/bin -timeout 3s'` narrows it while working on one cause.

`-list` prints the scripts the sweep would read and stops, which is what
makes a *chosen* run possible: read the scripts first, link the ones that are
plainly read-only into a directory of their own, and point `-dirs` at that.
Running only what someone has actually read is a stronger containment than
the probe convention, and nothing else printed the population to choose from.

Not a gate either, and for a stronger reason than conformance: the answer
depends on what happens to be installed, so it cannot be the same twice on
two machines.

`make smoke` starts each dialect binary on a real pseudo-terminal with a
scratch `HOME` and drives a session a person would have: an rc file with an
alias, a function and a `PS1` in it, a prompt, Tab, up-arrow, `C-r`, a
pipeline, a mistyped variable name, a background job, `^Z` and `fg`, and
`exit`. It answers **one row per feature** and never one boolean, because a
shell whose rc file is never
read fails four rows for one reason and a suite that stops at the first of
them reports a shell about which nothing else is known. `internal/smoke`
holds it and `internal/cmd/smoke` prints the table.

It exists because the interactive surface has no other test. The corpus
invokes everything with `-c`, `make wild` reads scripts and `make wild-run`
runs them — none of which involves a terminal, an editor, a prompt or a
keystroke, so every gap on the `daily-driver` label was found by somebody
hand-driving a pty for ten minutes. That is a fine way to find the first ten
and a hopeless way to learn whether they are fixed.

Two rules make it worth reading. **A mark is never text that is typed**: the
terminal echoes keystrokes, so a wait on a mark the line contains passes for
a shell that draws the line back and runs nothing — every line is written as
an expression, `echo alias-$((6 * 7))-ok` in and `alias-42-ok` out, and a
test holds the invariant. A third pitfall belongs beside it, learned on
#1124: **counting prompts is not a readiness signal for a shell with a line
editor**, which redraws its prompt on every keystroke, so a wait on "one more
prompt than last time" returns before anything has run. `session.atPrompt`
is safe because it waits on the *transition* and clears its flag on every
keystroke; a hand-rolled counter is not, and reported a shell as surviving a
failure it had merely not reached. And **the assertions are written against what bash
and zsh do rather than against what this tree does today**: rows known to be
missing carry the issue that owns them, so a known gap is quiet, an unowned
one is an exit status, and a gap that closes announces itself. Weakening one
until it passes would produce a green table about an unusable shell, which
is the exact failure the suite exists to prevent.

Not a gate, for the reasons `make wild-run` is not one and one more: it
asserts on a job actually resuming, which is timing. `go test` covers the
instrument's own machinery — the wait discipline, the scratch home, the
grading — and the session it drives is a target you run.

`make acp` drives the Agent Client Protocol front end the way an editor
does — `build/sh -acp` as a subprocess, JSON-RPC on a pipe — and prints
three things: every property `docs/design/acp.md` claims, graded; what a
turn costs against the process-per-command arrangement it replaces; and
what an ordinary pipe would have seen of the same script. `internal/acpcheck`
holds it and `internal/cmd/acpcheck` prints the tables.

It exists because **a protocol tested only from inside is not tested.**
`internal/acp`'s own tests stand an agent up in the test process and speak
to it through a pipe the test made: they prove the two halves of one
package agree, which is worth proving and is not the question anyone asks
about a wire format. The question is whether the binary we ship says what
somebody else's client will understand, and the only way to ask it is to
be somebody else's client. So `internal/acpcheck` imports nothing from
`internal/acp` and writes every message shape out again by hand — if it
shared the types, both sides could misname a field together and the table
would still be green.

Three rules make the rows worth reading, and each is a way the suite could
have lied instead.

**A row that can only pass is not a row.** Where the claim is "this is
refused", the row runs the allowed case too and requires the two to
*differ*: reject the write and the file must not exist, allow it and it
must. A check that sees a refusal without ever having seen the same action
succeed cannot tell a working gate from a shell that fell over.

**A row asserts on the side effect and not on the message.** "The file is
not there afterwards" is enforcement; "the agent said refused" is a string,
and a shell that printed it while writing the file anyway would pass.

**The purity row is checked by construction.** The protocol says an agent
must write nothing but messages on standard output, and one stray print
anywhere under the driver breaks every client at once — so every line the
agent writes is parsed, and anything that is not a message is recorded.
Nothing else in this tree would notice.

**Both directions are graded.** The rows above drive `sh -acp`, which is
what an editor does. The rest drive `sh -acp-connect`, where this shell is
the *client* and something else is the agent — and the claim there is the
stronger one: a policy on the shell reaches the agent it is running,
because the file the agent reads is a file we open and the command it runs
is a command we start. Proving that needs a second process that speaks the
protocol and asks for things, and a real coding agent needs an account, a
network and a model — which is exactly where #729 has been stuck since the
route was written. So the instrument re-executes itself as a scripted
agent: it asks for one file and one command line, records what it was
given or refused, and stops. The same agent runs twice, one `-deny` apart,
and the two answers have to differ.

`make acp-wire` prints one real session message by message, with a line of
English beside each, refusing the first thing it is asked and allowing the
second. It is the artifact for somebody who does not read Go: the shell
announcing a write that is inside an `eval`, asking, being told no, and the
file not existing afterwards.

Not a gate, for the reason `make smoke` is not one: it launches real
binaries, starts real children and times them. `go test ./internal/acpcheck`
covers the instrument's own machinery — the framing, the correlation, the
purity detection, the median — against a scripted agent that is not a shell,
including one that answers nothing at all.

`make acp-bench` says those same costs in `ns/op`, so one release can be
diffed against the release before it. The table `make acp` prints is the
better read — units a person uses, an argument attached — but its sample
count is a constant in the source and its output is prose, so two runs a
month apart can only be held against each other by eye, and nothing in the
tree would have noticed the turn path getting slower. The benchmarks sit in
`internal/acpcheck/bench_test.go` and drive the same client, so the two
instruments are timing one thing and can be checked against each other.

The spawn rows are in that file rather than a table of their own **because
the claim is a ratio**, and a ratio whose halves were measured in different
runs is not one — this machine is rarely quiet. For the same reason a
connection is retired every 64 timed operations: the agent holds a live
shell for every session opened on it and the client keeps every update it
was sent, so a benchmark left on one connection drifts into measuring the
accumulation rather than the operation.

**Writing the measured command out twice is what made both copies wrong.**
The gated turn and the spawn rows each named `/bin/true`, which on macOS is
`/usr/bin/true`, and the spawn rows discarded the error with `_ =` — so all
three were timing a shell that started, failed the lookup and exited 127.
The table read 18x where the truth is 2.9x, and it survived for as long as
it did because a process start dominates either way and the numbers stayed
plausible. `acpcheck.TrueCommand` resolves it once for both callers now,
and the test guarding it **runs** the program rather than checking that a
path came back: the first version of that test *skipped* when the lookup
found nothing, which is precisely the state the broken code was in on every
Mac, so it would have stayed green for the whole life of the bug.

`make sandbox` tries every way a script has of reaching the filesystem —
redirection in each of its forms, `source`, a glob, a probe, an exec, a
signal, and every module builtin that opens or changes a file — against the
binaries that ship, and reports what the boundary stopped.
`internal/sandboxcheck` holds it and `internal/cmd/sandboxcheck` prints the
tables.

**Two tables, because there are two routes to the same shell.** The first is
`sh -dialect X -policy p`, which grades the gate. The second is `bash
--policy p` on each dialect binary, which grades whether the *flag* reaches
that gate on the binary a shebang, `chsh` and `login` name. Only the first
existed until #1826, and the second binary had no flag at all while the
first table was green throughout — the same blind spot as the `-c` drift
that had `make conformance-dialects` grading the drivers rather than the
dialects. Deleting the flag reading today turns the second table to 116
OVERBLOCKED and leaves the first untouched, which is that blind spot in one
picture.

It exists because **the gate's unit tests are written by somebody who
already knows where the boundary is**, and every escape this repository has
had came in somewhere else: `sysopen` and `zsystem flock` opened files with
no gate (#1805), `autoload` read and then *ran* one (#1812), and every
mutating `zsh/files` builtin changed the filesystem with nothing consulted
(#1819). Each was reachable because the code implementing it had no reason
to know a gate existed, and none of them is expressible as a redirection.
So this enumerates the *routes* rather than the rules: a way in that arrives
without a gate is a row rather than a silence.

**Every route runs three times, and that is the whole design.** The failure
mode of a sandbox test is not a false alarm, it is a false calm — sixteen
routes reporting "refused" from a shell that never started looks exactly
like a sandbox that works, and that has happened here more than once. So no
row is decided by one run:

    ungated   no policy. The route must WORK, or it is measuring nothing
              and grades inert rather than passing.
    denied    a policy granting only the workspace. It must FAIL.
    allowed   a policy granting the whole tree. It must WORK again, or the
              gate is refusing what it was told to permit.

A row is `contained` only when all three agree, so a shell that does nothing
scores zero rather than perfect and a gate that refuses everything scores
zero too. Each run gets a fresh fixture, because the ungated one succeeds by
design and a second run against what it left would be measuring the
leftovers.

**`inert` is a ledger, not a skip.** It is a route this shell cannot take
*yet* — `zsh/mapfile` is not implemented, bash's `history -w` still falls
through to a refused exec — which is the state #1808 calls "safe by
accident". Those are the rows that become escapes the day the feature lands,
so they are printed rather than hidden, and each moves to `contained` or
`ESCAPED` on its own when it starts working.

The instrument was checked against a shell known to be broken, which is the
only way to know a green table means anything: pointed at the commit before
#1821 it reports the eleven escapes that change fixed, and pointed at the
commit after it reports none. `go test ./internal/sandboxcheck` covers the
grader's own machinery — the verdict rule, the fixture isolation, the
placeholder substitution, and that the denied policy grants exactly one
thing.

`make bash-suite` fetches **bash's own `tests/`** and runs every file of it
through real bash and through `cmd/bash`, comparing output and status.
`internal/suite` holds it, `internal/cmd/suitecheck` prints the report, and
`make suite-panel` lists the columns. It is report-only and never a gate.

It exists because the two corpora we have are both ours. `make conformance`
grades snippets this project wrote, so it proves that what we thought to ask
about is right; `make wild` sweeps the scripts installed on a machine, which
is real code but is not *about* the shell. A shell's own suite is thousands
of deliberate questions about that shell, written by the people who built it,
and every one of them was written without knowing this implementation exists.

**Nothing fetched is ever committed, and three separate things keep it out.**
bash's suite is GPLv3, this tree is Apache-2.0, and `NOTICE` says there is no
third-party code here — committing one would relicense the repository by
accident, which is a larger loss than any score is a gain. So it lands under
the gitignored build directory; it lands in a tree named for the distribution,
which is what `make wild` refuses to open (the carve-out that exempted this
repository's own working tree from that guard — and so exempted the one
directory a fetch writes to — was #1039); and only the suite is unpacked, so
the shell's own source streams past without reaching the disk at all.

**The report is counts and our own diagnostics, and nothing else.** A suite is
another project's expression and CLEANROOM.md's red list covers it; the
carve-out this stands on is a carve-out for *running* a suite, not for reading
one. So `suite.Result` has no path field and no text field — the rule is the
struct's shape rather than a habit — and a cause in the ranking is
`wild.Reason`'s wording, which is this lexer's own words with the position and
any word from the file removed. `make wild` refuses considerably less than
this and prints why: a path in a report is an invitation to go look.

**Never the shipped `.right` files.** The oracle is the reference shell on the
machine. Partly for measurement — the expected output was generated by a
different build of a different version and disagrees with the binary installed
here — and mostly because it is the most expression-like part of a suite. The
extraction excludes it outright, so that rule is a property of the tree rather
than a promise about the code.

**Three numbers, because each of them lies alone.** *Strict* — the whole file
byte-identical, status included — reads as catastrophe, because a suite file
is hundreds of assertions and one disagreement forfeits all of them. *Line
agreement* reads as triumph for the mirror-image reason, since most lines of
most files are text a shell echoed back; it is a **longest common
subsequence**, not a positional comparison, because one extra line near the
top must not zero a file that agrees on everything after it. *Parsed* explains
the other two: this parser reads a file whole before running any of it, so a
single refused construct forfeits a file that might have agreed line for line,
and the distance between parsed and strict is the size of that effect.

**The ranked list of file-stopping constructs is the actionable output**, and
it comes free with the run. A cause there is a gap costed by what it takes
away — a whole file — rather than by how often it appears.

**The suite's C helpers are built, or the whole thing scores zero for reasons
that have nothing to do with us.** Without `recho`, `zecho` and `printenv` the
calls fail under both shells and the two failures match, so the run reads as
*agreement on an error message*. They are compiled, never read.

**A truncated file is reported and never scored, and who was truncated is the
finding.** Each file gets a generous per-file timeout and is killed by process
*group*, because a suite file backgrounds jobs that hold the output pipe open
and killing the shell at the top leaves the harness waiting on a grandchild
nobody named. A killed dialect binary is **a hang we published**; a killed
oracle is **a harness fault**, since the shell that wrote the file does not
hang on it. Rolling them into one number would turn a loaded laptop into a bug
report and hide a real hang inside it. A file the oracle does not produce
twice is *unstable* — a pid, a clock, a scheduling order — and is counted
rather than dropped, because "we cannot tell" deserves its own number.

**Per-dialect from the first commit**, and that is the point rather than an
aspiration: an instrument that could only ever grade bash would quietly bend
the substrate toward bash, which is what this project's premise forbids. The
columns that are not built are **rows with reasons**, printed on every run —
zsh's files are `.ztst`, a format its own driver interprets rather than one a
shell runs; the maintained ksh93 suite is ksh93u+m's while the only ksh93 on a
Mac is AT&T's 2012 build, so that run would measure the fork; dash ships no
suite of its own. A missing column that says nothing reads as a column that
passed.

Every reason it cannot run prints in full and exits 0 — no network, no C
compiler, no reference shell. An instrument that failed a build because a
download did not arrive would be switched off within the week, and one that
exited quietly would be worse, because zero files compared and zero
disagreements look identical in a log.

`make corpus-guard` fails when the corpus has *lost* a case. It runs in
`make check` and as a step of the required `Build and test (ubuntu-latest)`
job, comparing the case IDs in the tree against the case IDs at the merge
base with `main`.

It exists because the corpus is a **set** written down as a Go slice
literal, and git merges it as text. A merge or a rebase can resolve
`case.go` by taking one side, and the campaign convention — keep *both*
sides — is a convention rather than a check. One `gh pr update-branch` took
the corpus from 1088 to 1084 IDs with a clean auto-merge and no conflict,
and only a hand count noticed. A dropped case is lost coverage that reads
as a passing build: nothing fails, the conformance total shifts by a few,
and that is indistinguishable from the ordinary movement.

Two choices in it are worth knowing. It compares **sets and not a count**,
because a branch that adds two cases while merging two away leaves the
count alone and one that adds five while losing four makes it rise. And the
baseline is **git history rather than a committed file**, because a
committed list is another text file in the same tree, merged by the same
merge that drops the case — a baseline the guarded event can edit is not a
baseline. History also needs no maintenance, so it cannot go stale. No case
ID has ever left `main` in the whole history of the file; retiring one on
purpose means naming it in `corpusguard.Retired`, which is reviewable and
fails safe if it is itself lost.

`make oracle-check` asks two questions, and they are enforced in different
places because only one of them has the same answer on every machine.

**Do the committed files agree with each other?** The corpus in `case.go`,
the golden record and `docs/spec/measurements.md` are three views of one
thing, and the document is a *pure function* of the other two — so
re-rendering it and comparing every byte asks the question completely.
That needs no shells and cannot differ by machine, so it **blocks**: it is
`TestTheCommittedRecordAndDocumentAgree` in `internal/oracle`, which runs
in `make check` and in the required `Build and test` job.

With exactly one exception, found by this check passing on macOS and
failing on a Linux runner for a byte-identical tree: the *word* beside a
signal number comes from `syscall.Signal.String()`, which is the reader's
kernel, while the number is the recording machine's. Signal 30 is SIGUSR1
on macOS and SIGPWR on Linux, so the same measurement is spelled two ways.
The word is computed from the number and carries no fact the number does
not, so the comparison drops it from *both* sides — the document keeps its
words for a reader, and the check stops treating them as evidence.

It exists because `oracle-check` compared *behavior* and a `Why` is prose.
Editing one after `make oracle` had run left `measurements.md` carrying a
sentence its own source no longer held, and nothing failed — twice in one
day, caught by eye both times. A wrong explanation attached to a correct
measurement is worse than a wrong number, because the numbers are checked
and the prose was not. The same comparison catches a case added, removed
or renamed without regenerating, and a change to the renderer shipped
without one.

**Does the panel still behave as recorded?** This one is **report-only in
CI**, and the reason is measured rather than assumed: on `ubuntu-latest`,
against a record made on macOS, **380 of 1122 cases differ** and bash 3.2
cannot be installed at all. A blocking job would fail every build. `make
check` is the gate locally, where the panel is the one that produced the
record.

So a stale record fails for its author on the half that can be pinned, and
the half that cannot is still reported — with a per-shell tally, because
four hundred lines of drift is not a report anyone reads and the shape is
the part worth seeing: concentrated in one column is a shell that moved,
spread evenly is a record that did.

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

## Every commit must be signed, and check before you push

`main` requires signatures. An unsigned commit does not fail loudly: the
pull request shows **every check green** and simply refuses to merge, with
`mergeStateStatus: BLOCKED` and nothing on the page saying why. That has
cost real time twice — once to a commit that signed *badly*, once to four
that did not sign at all.

Before pushing:

    git log --format='%h %G? %s' origin/main..HEAD

`G` or `U` is fine; `U` means a good signature that is untrusted in the
local keyring, which is normal here. `N` is unsigned and `B` is bad, and
either one blocks the merge.

To fix it, re-sign in place and force-push the *branch*:

    git rebase --exec 'git commit --amend --no-edit -S' <last-good-sha>
    git diff <old-head> HEAD          # must be empty — content is unchanged
    git push --force-with-lease

Check the tree diff is empty before pushing and that GitHub agrees after:

    gh api repos/blairham/sh/commits/<sha> -q '.commit.verification'

This is the one case where a force-push is the right answer. It is only
ever a feature branch, never `main`.

## Testing

Compatibility is proven by **differential testing against real shell
binaries**, never by importing another project's suite. A case is a
snippet plus the observed output and exit status of a real shell. The
snippet is ours; the output is a fact.

Third-party suites that must be run (bash's own `tests/`, GPLv3) are
fetched at test time and never committed. `make bash-suite` is the one
thing that does it — see the instrument above for the three separate
mechanisms keeping the fetch out of a commit, and for what a report over
somebody else's files may and may not say.
