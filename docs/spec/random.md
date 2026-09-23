# `$RANDOM`, and what a seed makes it answer

`RANDOM` is a produced parameter: reading it yields a number in `0`..`32767`,
and assigning to it seeds the generator, so a script that assigns gets a
sequence that is a function of the seed alone. Three shells in the panel have
it — bash, ksh93 and zsh — and **dash and BusyBox ash have no such parameter**,
where `$RANDOM` is an ordinary unset variable and `RANDOM=42` an ordinary
assignment.

That much was already recorded (`semantics.md`, and #2827, which made a seeded
sequence reproducible here). What this file adds is the **sequence itself**,
because reproducible is not the same as right: until #4240 a seeded script here
answered the same two numbers every time and they were not the two numbers the
shell it claims to be answers.

## Why this can be written down at all

`CLEANROOM.md`'s green list covers **observed behavior of real binaries** and,
separately, **ideas, procedures and methods of operation** — the second being
17 U.S.C. §102(b), under which an algorithm is not the subject of copyright.
Everything below was obtained by **running the shells and fitting their
output**: no implementation's source was read, and the fit was then confirmed
against seeds chosen after it was made.

The method is worth stating, because it is the reason this is evidence rather
than recall:

1. Ask each shell for the first number after each of a few hundred consecutive
   seeds, and for a long run from one seed.
2. Read the recurrence off the small seeds. `RANDOM=1` answers `16807` in bash
   and zsh, and `16807` is `bash`'s first number for seed 1 because the state
   after one step *is* `16807 × 1`; `RANDOM=2` answering `846` is `33614`
   with its top bits taken off. That is one multiplication, so the family is a
   multiplicative congruential generator and the multiplier is on the page.
3. Fit the modulus and the output map to the seeds where they separate — the
   first seed whose state exceeds 16 bits is seed 4, and it is the one that
   tells the three shells apart.
4. Solve for the state behind a sequence the seed does not give directly (the
   zero seed) by enumerating the states consistent with its first number and
   discarding those whose next forty numbers disagree.
5. **Confirm on seeds chosen afterwards**: one hundred fresh pseudo-random
   seeds per shell, three draws each, plus a three-thousand-draw run.

## The recurrence, which all three share

    state ← 16807 × state  (mod 2^31 − 1)

The Lehmer generator, with the multiplier and modulus that make it the
"minimal standard" — 16807 is a primitive root of 2^31 − 1, so the state walks
all 2^31 − 2 nonzero residues before it repeats. Each read of `RANDOM`
advances the state once and then converts it to a number; the conversion is
**not** shared, and that is what makes three shells with one recurrence answer
three different sequences.

Zero is the recurrence's fixed point, so no implementation can let the state
sit there. All three replace it with the same constant:

    restart = 123459876

and the rule is one rule in all three: **a state of exactly zero at the start
of a draw is replaced by `restart` before the multiplication.** That covers
both the shell that reduces its seed into `1`..`2^31 − 2` and finds a zero at
seed time, and the shell that does not and finds one after a draw has already
answered `0`.

## What each shell answers, and where it differs

Two questions per shell, and the answers were measured rather than assumed:
how the value a script assigned becomes a state, and which bits of the state
come back out.

| | seed → state | state → number |
| --- | --- | --- |
| bash 5.3.20 | `(v mod 2^32) mod (2^31 − 1)` | `((s >> 16) XOR (s AND 0xffff)) AND 0x7fff` |
| zsh 5.9.2 | `v mod 2^32` | `s AND 0x7fff` |
| ksh93u+ | `v mod 2^15` | `(s >> 3) AND 0x7fff` |

Seed 4 is the row that separates them, and seed 1 the row that does not:

    RANDOM=1; echo $RANDOM       bash 16807   zsh 16807   ksh93 2100
    RANDOM=4; echo $RANDOM       bash  1693   zsh  1692   ksh93 8403

`16807` three ways: bash and zsh agree because the state `16807` has nothing
above bit 15 to fold or mask away, and ksh93 does not because it shifts. At
seed 4 the state is `67228`, which has a bit above 16 — bash folds it back in
and answers one more than zsh, which drops it.

Each reduction was pinned by the seeds that overflow it, not by ordinary ones:

- **bash** reduces modulo `2^31 − 1`, so `RANDOM=2147483647` is the zero seed
  and answers `20814`, the same as `RANDOM=0`, and `RANDOM=2147483648` is
  seed 1. A 32-bit wrap comes first: `RANDOM=4294967296` is seed 0, where a
  plain `mod 2^31 − 1` of 2^32 would be 2.
- **zsh** does *not* reduce modulo the modulus, which is visible because it
  leaves a state the recurrence sees as zero: `RANDOM=-2` — `2^32 − 2`, which
  is twice the modulus — answers `0` and *then* `20034`, the first number of
  its zero-seed sequence. bash answers `20814` immediately for the same
  arithmetic value of `-2`, because it has reduced first.
- **ksh93** keeps only 15 bits, which is why `RANDOM=-1` and
  `RANDOM=2147483647` answer alike (`26571`, the number for state `32767`)
  while `RANDOM=2147483648` and `RANDOM=0` answer alike (`6600`).

## An assignment to `RANDOM` is arithmetic

All three evaluate the value as an arithmetic expression, which is what the
integer attribute each of them lists the name with means. Measured 2026-09-22
under `env -i PATH=/usr/bin:/bin LC_ALL=C`:

    RANDOM=3+4          seeds 7 in all three
    abc=5; RANDOM=abc   seeds 5 in all three
    RANDOM=0x10         seeds 16 in bash
    RANDOM=2#101        seeds 5 in bash
    RANDOM=nope         seeds 0 — an unset name is zero in arithmetic

So "a value that is not a number is a zero seed", which is how #2827 recorded
it, is the *consequence* of arithmetic rather than a rule of its own: `abc` is
a bare name, and a bare name is its value. The distinction is not academic —
with `abc=5` set, the two readings disagree about the whole sequence.

An expression that will not evaluate is complained about and **changes
nothing**: on bash 5.3.20, `RANDOM=nope; RANDOM=1/0; echo $RANDOM` writes
`1/0: division by 0` and then the *second* number of the zero-seed sequence, so
the seed and the draw count both survived the failure. Note that this is not
what an ordinary integer-attributed name does — `declare -i v; v=1/0` ends the
script — so in bash the produced parameter is the gentler of the two, measured
on both.

**One row here is knowingly short.** zsh and ksh93 *end the script* over the
same failing seed, where bash carries on and this implementation carries on:

    RANDOM=1/0; echo "after $RANDOM"; echo tail

    bash 5.3.20   division by 0, then `after …` and `tail`
    zsh 5.9.2     `division by zero`, and nothing after it
    ksh93u+       `divide by zero`, and nothing after it
    here          each shell's own complaint, then `after …` and `tail`

So the complaint and its wording are each shell's own and right, and whether it
is fatal is bash's answer given to all three. That is a disagreement between
real shells over identical syntax, which means it wants a **semantics axis**
rather than an inline conditional, and an axis is not something to add in
passing — it needs the `unpinned` verdicts for the two shells that have no such
parameter at all and a probe for `make axis-grade`. It is recorded here rather
than guessed at. Before #4240 the row was worse in both directions: no shell
complained, and all three silently seeded zero.

## Where this lives in the code

`interp.MinimalStandardState` holds the recurrence, the restart and the closed
form for "the state after *n* draws", because a recurrence names no shell and
three dialects needed the same one. Each dialect holds its own two answers —
`dialect/bash/random.go`, `dialect/zsh/random.go`, `dialect/ksh/random.go` —
and registers them through `interp.Runner.SetSeededRandoms`.

An **unseeded** `RANDOM` is deliberately not specified here. It is a different
number every run in every shell that has it, which is the whole point of the
parameter, and nothing can be graded against it.

## Citation

Oracle runs on this machine, 2026-09-22, all under
`env -i PATH=/usr/bin:/bin LC_ALL=C`:

- `/opt/homebrew/bin/bash` 5.3.20 — seeds 0…300, a 3000-draw run from 12345,
  seventeen out-of-range and negative seeds, and 124 fresh pseudo-random seeds
  chosen after the fit.
- `/opt/homebrew/bin/zsh` 5.9.2 — seeds 0…200, a 200-draw run, eleven edge
  seeds, 100 fresh seeds.
- `/bin/ksh` (Version AJM 93u+ 2012-08-01) — seeds 0…400, a 300-draw run,
  seventeen edge seeds, 100 fresh seeds.

Every one of those matched the table above with no exceptions. A long run must
be taken with `while` rather than `for i in $(seq …)`: ksh93 reseeds `RANDOM`
across the fork a command substitution makes, so the substitution disturbs the
sequence being measured — which first read as the model failing on the very
first draw.
