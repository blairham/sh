# sh

A shell parser and interpreter in Go.

**Status: specification phase.** No implementation code has been written
yet, by design — see `CLEANROOM.md`.

## What makes this different

The core language is the **common denominator of real shells** — not
strict POSIX, and not bash. Dialects (`posix`, `bash`, `zsh`, `ksh`) are
**presets over a semantics vector**, not layers, translations or forks.

That distinction is not stylistic. Measuring nine behavioural axes across
dash, bash, ksh93 and zsh produces six different groupings of those
shells — dash sides with zsh on `echo`, bash sides with zsh on `shift`,
ksh93 sides with zsh on pipelines. **No "sh → bash → zsh" ladder explains
the data.** Dialect is a point in a multi-dimensional space, so the model
is a vector of independent switches. See `docs/spec/semantics.md`.

Execution is **gateable and observable from the inside**. Sandbox
policy, agent permission prompts, AI context and audit trails are
interpreter-internal seams rather than wrappers, because `eval` and
`source` re-parse inside the interpreter and a wrapper never reaches
them. See `docs/design.md`.

## Independence

This is an independent implementation. It is not a fork or a port of any
existing project, and no file here derives from one. Behaviour is learned
from POSIX, from vendor manuals, and from running real shell binaries as
oracles; it is written down in `docs/spec/`; code is written from the
spec. `CLEANROOM.md` is the binding rule set.

## Layout

    CLEANROOM.md              the rules that keep this independent
    docs/design.md            architecture
    docs/lessons-from-obi.md  starting constraints, learned the hard way
    docs/spec/                behavioural specs — the wall
      oracle.md               how behaviour is learned from real binaries
      shell-matrix.md         measured: which constructs exist where
      semantics.md            measured: where shells conflict
      core.md                 the core language boundary

## Licence

MIT.
