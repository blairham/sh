<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.svg">
    <img src="assets/logo.svg" alt="sh" width="400">
  </picture>
</p>

A shell parser and interpreter in Go.

**Status: early — the first tag is `v0.0.0` and means it.** The core parser
and interpreter are in place, and all five dialect binaries grade against a
panel of the real shells they model. Measured on macOS with
`make conformance-dialects`, 2026-09-12, 3670 cases each:

| our binary | graded against | exact | behavioral |
| --- | --- | --- | --- |
| `bash` | bash 5.3.15 | 96% | **100%** |
| `zsh` | zsh 5.9.2 | 98% | 99% |
| `dash` | dash | 98% | **100%** |
| `ksh` | ksh93 AJM 93u+ | 91% | 97% |
| `ash` | BusyBox 1.37 ash | 79% | 97% |

**Exact** is byte-identical stdout, stderr and exit status. **Behavioral**
lets a diagnostic be worded differently so long as the status and the output
agree, and the difference between the two columns is almost entirely message
wording — which is why `ash` reads as the weakest column on the left and is
mid-pack on the right. Behavior lands spec-first, per `CLEANROOM.md`.

## What makes this different

The core language is the **common denominator of real shells** — not
strict POSIX, and not bash. Dialects (`bash`, `zsh`, `ksh`, `dash`, `ash`,
plus `core` and `posix`) are **presets over a semantics vector**, not
layers, translations or forks.

That distinction is not stylistic. Measuring twenty-eight behavioral axes
across dash, bash, ksh93 and zsh produces eight different groupings of
those shells — dash sides with zsh on `echo`, bash sides with zsh on
`shift`, ksh93 sides with zsh on pipelines. **No "sh → bash → zsh" ladder
explains the data.** Dialect is a point in a multi-dimensional space, so
the model is a vector of independent switches. See
`docs/spec/semantics.md`.

Execution is **gateable and observable from the inside**. Sandbox
policy, agent permission prompts, AI context and audit trails are
interpreter-internal seams rather than wrappers, because `eval` and
`source` re-parse inside the interpreter and a wrapper never reaches
them. See `docs/design.md`.

## Independence

This is an independent implementation. It is not a fork or a port of any
existing project, and no file here derives from one. Behavior is learned
from POSIX, from vendor manuals, and from running real shell binaries as
oracles; it is written down in `docs/spec/`; code is written from the
spec. `CLEANROOM.md` is the binding rule set.

## Installing

From the tap:

    brew install blairham/tap/sh

Or from source:

    make install                           # /usr/local/libexec/sh — needs sudo to write
    make install PREFIX="$HOME/.local"     # no sudo

Six binaries: `sh`, and the dialect binaries `bash`, `zsh`, `ksh`, `dash`
and `ash`. They land in `libexec` and **not** in a `bin` directory, because
they are named after the shells they model — a directory ahead of `/bin`
on `PATH` would answer for every program on the machine that resolves a
shell by name. `make install` refuses a `SHELLDIR` that is on `PATH`
unless it is told to go ahead, and the formula links nothing into
Homebrew's `bin` for the same reason.

So you run one by its full path, and `docs/install.md` has the whole of
it — including what `/etc/shells` and `chsh` need to make one of them a
login shell, and how to try one first as a terminal profile's command,
which is a checkbox to revert rather than a rescue.

A session reads everything a real shell of the same name reads, in the
same order: `~/.bashrc`, `~/.zshrc`, the profile files, and the machine's
own file in front of each. `docs/install.md` has the measured grid.

## What is not there yet

The first tag is `v0.0.0` because this list is real, not because the list
is short.

- **A prompt can land on top of unfinished output.** There is no
  `PROMPT_SP`/`PROMPT_CR` handling yet, so a command whose last line has
  no newline gets the next prompt drawn onto it — [#2477][2477].
- **Diagnostic wording is the biggest remaining gap**, and it is most of
  the distance between the two columns above — `ksh` and `ash` agree on
  what happens and still phrase the complaint differently.
- **The sandbox contains the shell, not the process tree.** A policy
  decides what the *shell* opens, runs and signals; a command it was
  allowed to start makes its own accesses and nothing here sees them.
  `docs/design/sandboxing.md` has the scope limit in full.
- **POSIX only.** No Windows target — every route into this program is a
  process group, a controlling terminal, a signal or a `syscall.Exec`,
  and a binary that cannot start is worse than no binary.

[2477]: https://github.com/blairham/sh/issues/2477

## Using it as a library

The core is a library, and it does not know which shells exist: `syntax`
and `interp` define the questions, and each shell answers them in its own
package under `dialect/`.

```go
sem, dial, diag := bash.Semantics(), bash.Dialect(), bash.Diagnostics()
r := &interp.Runner{Semantics: &sem, Dialect: &dial, Diagnostics: &diag}
bash.Apply(r)                      // what this shell adds or removes

r.Register("cd", cd)               // only what shell cannot express

prelude, _ := syntax.Parse(`basename() { printf '%s\n' "${1##*/}"; }`, dial)
r.Run(ctx, prelude)                // everything else
```

Adding a shell adds a directory. Nothing under `syntax/` or `interp/`
names one.

Functions shadow builtins and external commands alike, so most of a dialect
needs no Go at all.

## Layout

    CLEANROOM.md              the rules that keep this independent
    docs/install.md           installing, and running it as a login shell
    docs/design.md            architecture
    docs/lessons-carried-forward.md
                              starting constraints, learned the hard way
    docs/spec/                behavioral specs — the wall
      oracle.md               how behavior is learned from real binaries
      shell-matrix.md         measured: which constructs exist where
      semantics.md            measured: where shells conflict
      core.md                 the core language boundary
      style.md                measured: how source is laid back out
      grammar/                per-construct specs
    cmd/shfmt                 the formatter — every dialect the parser reads

## License

Apache-2.0. See `LICENSE` and `NOTICE`.

Contributions require a signed CLA — see `CONTRIBUTING.md` and `CLA.md`.
The CLA exists so the project retains the option to offer different
terms later; you keep your copyright.
