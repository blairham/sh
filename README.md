<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.svg">
    <img src="assets/logo.svg" alt="sh" width="400">
  </picture>
</p>

A shell parser and interpreter in Go.

**Status: implemented and measured.** The core parser and interpreter are
in place, and all four dialect binaries grade against the live shell
panel — `make conformance`. Behavior still lands spec-first, per
`CLEANROOM.md`.

## What makes this different

The core language is the **common denominator of real shells** — not
strict POSIX, and not bash. Dialects (`posix`, `bash`, `zsh`, `ksh`) are
**presets over a semantics vector**, not layers, translations or forks.

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

## Using it

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
    docs/design.md            architecture
    docs/lessons-from-obi.md  starting constraints, learned the hard way
    docs/spec/                behavioral specs — the wall
      oracle.md               how behavior is learned from real binaries
      shell-matrix.md         measured: which constructs exist where
      semantics.md            measured: where shells conflict
      core.md                 the core language boundary
      grammar/                per-construct specs

## Licence

Apache-2.0. See `LICENSE` and `NOTICE`.

Contributions require a signed CLA — see `CONTRIBUTING.md` and `CLA.md`.
The CLA exists so the project retains the option to offer different
terms later; you keep your copyright.
