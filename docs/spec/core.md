# The core language

The core is the **common denominator of real shells**, not strict POSIX
and not bash.

## The boundary

**In the core** — POSIX XCU in full, plus everything the current
non-dash shells — bash 5.3, ksh93 and zsh — agree on: arrays,
`[[ ... ]]`, `$'...'`, `+=` append, `${x:off:len}`, `${x/pat/rep}`,
C-style `for`, the `function` keyword, `select`, herestrings, process
substitution, and `local`.

**Not in the core** — anything only one shell in the panel provides.
`${x^^}` is the worked example: bash 4+ only, absent from bash 3.2,
ksh93 and zsh. It belongs to the bash dialect, not the core.

**Never in the core** — any construct on a conflict axis resolved by
fiat. Conflicts get a vector field (`semantics.md`), not a core answer.

**The boundary is about the language, not about where code lives.** It
governs what parses and what identical syntax means. It says nothing
about which Go file registers a builtin: `mapfile`, `compgen` and
`enable` are bash's alone and are registered in the core builtin map,
with each dialect that lacks them calling `Unregister`. That is the
extension seam doing its job, not a bash-only command sneaking into the
core language — a name is in the core language only if every dialect
keeps it. `semantics.md` records the criterion and the measured
membership.

## bash 3.2 is evidence, not a veto

Two shells in the panel cannot vote a construct out of the core, for two
different reasons. dash is excluded by decision — that is the whole
story of `shell-matrix.md`. bash 3.2 is subtler: it is in the panel to
*date* constructs, not to gate them.

The rule, stated the way the decisions actually operate: **the core is
what bash 5.3, ksh93 and zsh agree on.** bash 3.2's column records how
old a construct is and what macOS's `/bin/sh` can run — evidence worth
keeping, because "works everywhere except macOS's `/bin/sh`" is a fact
scripts trip over — but a refusal there does not remove a construct the
three current shells share.

The recorded decisions already work this way; this section makes the
written rule match them:

- `;&` fallthrough is core (`grammar/commands.md`), and bash 3.2
  refuses it — it is a bash 4 feature (`shell-matrix.md`).
- Negative subscripts are core (`grammar/parameter-expansion.md`), and
  bash 3.2 predates the form and refuses it, fatally for a write.
- `$'\uHHHH'` decodes in the core (`grammar/tokenization.md`), and
  bash 3.2 keeps the four characters as written.

Reading "the non-dash panel" as including bash 3.2 would contradict all
three, and none of them was decided by accident. A construct bash 3.2
lacks is core when the other three agree — recorded alongside the
version that introduced it, so a lint targeting old bash has the fact it
needs.

## The holdout rule is about membership, not about meaning

The section above is about **which constructs are in the language**, and
a sole holdout does not keep one out. It says nothing about what a
construct in the language *means*, and the two must not be run together:
a difference about meaning is a conflict, and a conflict gets a vector
field rather than a core answer — which is the rule two sections up, in
"Never in the core".

`[^abc]` is the worked example, and it was filed as a contradiction
between the two rules before it was read as this (#489). Every panel
shell parses it; six of the seven read the caret as negating the class
and dash reads it as an ordinary character. So it is not a construct dash
lacks — the pattern runs there and **matches different things** — and the
holdout rule has nothing to say about it. `Semantics.BracketCaretNegates`
is the field, and `semantics.md` has the measurement and the POSIX text
that declines to settle it.

The tell is what a wrong answer costs. A construct held out of the core
because one shell refuses it would be a **parse error** for the scripts
that use it, which announces itself; the wrong reading of `[^abc]` is a
pattern that still matches, silently, just not the intended things.

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
