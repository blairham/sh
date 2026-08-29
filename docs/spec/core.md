# The core language

The core is the **common denominator of real shells**, not strict POSIX
and not bash.

## The boundary

**In the core** — POSIX XCU in full, plus everything the non-dash panel
agrees on: arrays, `[[ ... ]]`, `$'...'`, `+=` append, `${x:off:len}`,
`${x/pat/rep}`, C-style `for`, the `function` keyword, herestrings,
process substitution, and `local`.

**Not in the core** — anything only one shell in the panel provides.
`${x^^}` is the worked example: bash 4+ only, absent from bash 3.2,
ksh93 and zsh. It belongs to the bash dialect, not the core.

**Never in the core** — any construct on a conflict axis resolved by
fiat. Conflicts get a vector field (`semantics.md`), not a core answer.

## Why not strict POSIX

POSIX omits `local`, which dash, bash and zsh all provide. A core that
rejects `local` rejects what essentially every real shell accepts, and
nobody can write against it. Strict POSIX is the right target for a
*portability lint*, not for a runtime.

## Why not bash

bash's long tail is discovered by being bitten, not derived from a spec,
and most of it is bash-only. Adopting it as the core would make every
other dialect a subtraction from bash — which is the architecture this
repository exists to avoid.

## The two directions the variant set is read

- **Runtime: union.** Be permissive. Accept what any enabled dialect
  accepts. A user pasting a bash snippet wants it to run.
- **Lint: intersection.** Given a declared target set, report what is not
  universal — "`[[` is used; dash rejects it." This is where strict-subset
  checking is actually valuable, and it is a reporting mode, never a
  runtime restriction.

The same variant set serves both. They differ only in whether membership
is tested with *any* or *all*.
