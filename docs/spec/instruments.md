# How the instruments lie

`oracle.md` says how behavior is learned from real binaries. This page says
how the **measurement of our own agreement with them** goes wrong — the
harnesses, grids, probes and mutation runs a burndown is steered by, and the
specific ways each of them reports something that is not true.

**Why this is a document and not an issue.** Every rule below was found the
same way: a measurement was believed, acted on, and turned out to have been
answering a different question. They accumulated on issue #4436, which was an
epic whose bar was a multi-week campaign and so could never close. A rule with
no close condition does not belong on a board; it belongs here, where the next
person meets it before repeating the afternoon that produced it. **If it can
never be closed, it is a `docs/spec/` page, not an issue.**

The governing rule is in `AGENTS.md` and is not repeated here: *before you
trust a null result, prove the instrument can produce a positive.* What
follows is the catalog of shapes that rule does not by itself catch.

---

## 1. The four nothings a mutation harness must be able to say

A mutation row reads `KILLED` or `SURVIVED`, and **four different kinds of
nothing all render as one of those two**. Each needs a column, because each
one's absence wears the face of a result.

| what happened | what the row says without the column | the column |
| --- | --- | --- |
| **nothing built** — the mutant does not compile | `SURVIVED`, F=0 | BUILD |
| **nothing ran** — the `-run` pattern matched no test | `SURVIVED`, F=0 | RAN |
| **nothing failed** — the mutant is genuinely alive | `SURVIVED`, F=0 | FAILED |
| **nothing changed** — the source pattern did not match | `SURVIVED`, F=0 | APPLIED |

All four print `F=0`. Only the third is a finding.

A harness must therefore report **how much it looked at**, not only what it
found. A baseline row — the unmutated tree, which must read `RAN>0` and `F=0`
— is what proves the other three columns mean anything.

### The APPLIED column and formatters

The fourth was found last: a mutant's pattern was written against source as
it had been typed, and `gofumpt` realigned a struct literal before the
harness read it — `get:  func` became `get: func`, one space, and an
exact-match pattern stopped matching. BUILD said ok, RAN said 18, FAILED said
0.

> **Any harness whose patterns are written against source a formatter will
> touch is one `gofumpt -w` away from silently testing nothing.**

`gofumpt` runs as a pre-commit hook in this repository, so the window between
"I wrote this pattern" and "the file looks like that" is one commit wide. Two
remedies: write patterns against post-format source, or keep a column that
fails loudly when a pattern misses. Prefer the second — it does not depend on
remembering.

**Re-verify every pattern applies after any refactor**, including one of your
own. A refactor silently invalidates the patterns a harness was built on.

---

## 2. The four readings of `SURVIVED`

A surviving mutant has four causes and **the correct response differs in
every one**, including two where the code is wrong to touch and two where it
is wrong to leave.

| the cause | the code | the mutant |
| --- | --- | --- |
| the harness did nothing | untouched | fix the pattern |
| the tests are too weak | untouched | **add a row** |
| the code is unreachable | **delete it** | goes with it |
| the distinction is unobservable | **keep it** | **remove it** |

**The tests are too weak** is the ordinary case. Example: a mutant broke only
an option's *reporting* — `setopt NAME` kept working while `[[ -o NAME ]]`
answered false — because every behavior row went through the setter and none
through the getter.

**The code is unreachable** means no test could have killed it. Example: a
`n < 0` guard after an `atoi` that accepts no sign. Deleting removed nothing,
because nothing could reach it.

A second shape of the same cause: **a caller duplicates the test**. Two
mutants survived in the same place, one after the other — the second being
the replacement written for the first — because both call sites tested
`x != nil` before asking a helper that tested it again. The usual response,
a sharper row, could not have worked.

> **One equivalent mutant is a fact about that line. Two in a row, in the
> same place, is a fact about the shape.**

The repair there was neither deletion nor a comment: the *question moved*, so
the helper answered the whole of it and the call sites stopped half-answering
first. The check then became reachable by every call, and deleting it became
a nil dereference rather than a no-op. **A mutant that can only be survived
by code that is correct is the outcome to aim for.**

**The distinction is unobservable** is the one most easily got wrong in both
directions. Example: a kind test — `PrecommandTransparent` versus "any
precommand" — where the branch runs and is correct, and only the *difference*
between the narrow and wide forms cannot be observed, because every other
value is caught earlier or never reaches the scan. The narrow test **stays**,
because it states which kind the branch is for and the next value registered
should not inherit it silently; the mutant **goes**, because a permanent
survivor trains the reader to skim the column and costs the next person the
same investigation.

---

## 3. A total is not a per-row check, in any direction

A roll-up over many files moves for reasons that have nothing to do with the
change under test. All three directions have produced a wrong conclusion:

- **A falling total hid a rising row.** A suite row went 13 → 10 differing
  lines while the one file the change was aimed at went 1 → 2.
- **A static total hid a closed row.** A file's score was identical before and
  after a change that took one of its chunks from disagreeing to
  byte-identical, because a later stopping point truncated everything after
  it.
- **A rising total hid three closed rows.** A driver that reports only the
  *first* failing chunk produces a *longer* failure report as you get further
  into the file.

> **When a change is aimed at a particular case, re-diff that case.** The
> total is the right number for the board and the wrong one for "did my change
> do what I said".

On a file run by a chunk-oriented driver, the per-chunk view is the
instrument and the file-level number mostly measures *where the stopping
point is*.

---

## 4. The grid that cannot fire

A grid where no row can disagree produces **exactly the output of a grid
where no row did**. This is distinct from the empty-run fault above, and the
row-count and harness-error columns do not catch it, because every number in
it is real.

Observed shapes:

- 90 rows over six columns, 0 harness errors, before-vs-before clean, **0
  changed everywhere** — and not one row set the option under test.
- 48 rows built from `command` cases, for a change that deliberately altered
  only `builtin` and `noglob`.
- A runtime grid, 36 rows, 0 changed, for a change that only affects how a
  function is *listed*.

The common cause: **a grid built from the rows you were thinking about rather
than the rows the change can move.** The two sets diverge exactly when the
change is narrower than the topic.

> **Before reading any grid, ask: which row here would change if I were
> wrong? Be able to point at it.**

### The negative control is the other half

The strongest form is to sort the rows *before* reading them, into the ones
that can discriminate and the ones that cannot. A worked example, for an
option that narrows `noclobber` to files of size zero:

```
                        noclobber   both      moves?
regular empty file      refused     0         *** yes ***
/dev/null               0           0         no
a fifo                  0           0         no
a directory             refused     refused   no
```

Three of four rows are allowed by `noclobber` **alone**, so nothing built
from them can tell the narrowing working from the narrowing missing. They
belong in the tests labeled as controls. Exactly one row is evidence, and
knowing that is what makes the others meaningful.

---

## 5. Probes that the subject reaches

- **Do not report through a channel the command under test can move.** A probe
  wrote its observed value to standard error while testing `exec 2>out`; the
  row came back **empty rather than wrong**. Read the value back afterwards,
  from a file the command has not touched, outside any subshell it ran in.
- **A here-document body is newline-delimited text.** A literal `\n` in a
  fixture — from a shell variable that did not expand — makes a here-document
  with *no body*, which is a valid construct that does not error. Write
  fixtures with `printf '%b'` and real tabs.
- **Platform flags.** A panel piped every shell's output through `cat -A`,
  which is GNU-only; every row on every column came back empty and "agreed",
  and the only tell was the usage text printed beside them. Prefer `sed -n l`.
- **The test runner's `PATH` is a temp directory.** A fixture that reaches for
  `cat` fails at 127 for a reason unrelated to the rule. Use the shell's own
  `$(<file)` or a builtin.

---

## 6. Which binary is being measured

- **`driver.PosixNamed` reads the exact basename `sh`** (one leading dash
  removed) and a shell invoked under that name starts in POSIX mode — in
  *every* dialect, not only `cmd/sh`. The same `cmd/bash` bytes named `sh`
  end a script where named `bash` they do not.

  > **Never let a binary's basename cross the `sh` boundary between the two
  > sides of a comparison, in either direction.**

  Renaming the `sh` column to `prior-sh` turns POSIX mode off on the before
  side; naming a scratch binary `sh` turns it on. Everything else is safe:
  `bash` → `prior-bash` and so on never cross it.

  The rule that is immune whether or not anyone remembers the mechanism:
  **a before-binary is not scratch, it is a column.** It carries the column's
  own name, in its own directory — `before/{zsh,bash,ksh,dash,ash,sh}` — the
  way `build/shells/` does for the after side.

- **`go version -m` cannot identify a build in a linked worktree here.** Go
  does not resolve a worktree's `.git` *file* and walks up, so every binary
  built in a worktree under this tree carries the enclosing repository's
  revision — a hash that does not exist in this repository. Identify a
  before-binary by the commit you checked out to build it, not by the stamp.

- **A reference must be a different program from the thing under test.**
  Perfect agreement on every row is the signature of one program, not two.

- **If you invoke a suite driver yourself, you are responsible for everything
  the harness does first** — the per-run copy, the symlink it places, the
  environment it sets. The cheap guard is to run the reference against itself
  and require an empty diff.

---

## 7. A guard against itself

A reference-against-itself check proves the instrument is not firing
**spuriously**. It cannot prove it can fire **at all** — one fault kills the
measurement and its guard together. A harness that failed to place any shell
reported *0 changed* on every row, and its before-vs-before guard reported
*0 changed* for the same broken reason, and read as a pass.

The two guards answer different questions and both are needed:

1. before-vs-before must be **0 changed on every row** (no spurious firing);
2. a known-positive pair must move on **exactly** the rows it should (it can
   fire at all).

---

## 8. Comments that are true about the world and false about the code

A comment stating something true about a reference shell, while the code
beside it no longer does that, reads as a **guarantee** rather than an
observation. Four routes to one, all observed:

| route | example |
| --- | --- |
| **outlived by its subject** | a note describing a reordering a later change removed |
| **argued instead of measured** | an axis doc explaining a disagreeing row away |
| **conditions in prose the script never enforced** | an option named in a comment and absent from the fixture |
| **true of the reference, false of the code** | a comment claiming a row is implemented when it describes what was measured |

Only the last has a mechanical guard available, and only where somebody made
the claim **derived rather than stated**: a count computed from a table fails
a test the moment the table and the prose part, where a number written in
prose drifts silently.

> **When a change moves something from recorded to real, the claim is part of
> the change.**

Every claim in a comment that could be derived instead is a guard nobody has
written yet.

---

## 9. One column's measurement generalized to a panel

A rule measured on one shell and applied to all of them agrees with the
column it came from and is wrong about the others in ways nothing has a row
for.

The worked example is a printer that had one unconditional rule for a
redirection's descriptor, measured on bash — and it was not even bash's,
because bash **adds** the default descriptor to a duplication and **drops**
it from a file redirection, and the tree had only the first half. It printed
`1>&2` for the column that drops it and `1> out` for the column that does
not. The third column came out right by accident of the same mixture.

> **When a column's output differs, check whether it went through the code
> you are about to change.** A panel row is not evidence about a code path the
> column never enters.

Three columns disagreeing is not automatically three values: in one case the
third column wrote its source text back verbatim, so its answer was the
*absence* of a normalization rather than a third one, and a bool was the right
shape.

And when a grid over a panel agrees everywhere, ask what the rule is **keyed
on** and whether the grid varies *that*. Four columns agreeing on an answer
while disagreeing on the reason is the hardest form: every row correct, the
rule underneath still wrong.

---

## 10. A register that cannot close goes stale faster than anyone re-checks it

Issue #4436 carried, at its best, a table of 55 suite files with *the one chunk
that stops each one*, and named 33 distinct issues as those chunks. When the
epic was decomposed, all 33 were checked: **every one was closed**, and every
still-failing file had moved to a new stopping point. Filing from that table
would have produced thirty-odd issues against fronts that no longer existed.

The structural point is not that the table was wrong when written. It is that
**nothing in the shape of a register forces a re-check**, so measurements
accumulate faster than anyone revisits them, and a table that was true in
September reads exactly like work to do in January.

> **A stale front reads exactly like work to do**, which is the same shape as
> a stale reference reading exactly like a real difference — one level up,
> about the board rather than about a run.

Two consequences, both cheap:

- **Every measured number carries the commit it was measured at.** A table
  without a commit is a table nobody can tell is stale. Every figure on this
  page and in any issue filed from a sweep states its `sha` and date.
- **A measurement with no close condition is a document, not an issue.** It
  gets re-measured in place rather than accumulating as a backlog nobody can
  burn down.

## 11. A harness needs a column for its own parsing

A per-file ranking script extracted four numbers per file with `grep -oE`,
and defaulted a missing match to `0`. Two files failed to parse — and because
the default was zero, they sorted to the **top of the ranking as the two
cheapest files in the suite**, at 0 differing lines each. One of them was in
fact a refusal-agreement and the other carried 51 differing lines.

The fault was caught only because the per-file sum did not reconcile with the
whole-suite run — the check that exists for exactly this. But the lesson is
narrower and worth stating on its own:

> **A parse failure that defaults to a plausible value is indistinguishable
> from a measurement.** Default to a value that cannot be mistaken for one, or
> carry a column that counts the rows that did not parse.

This is the same family as section 1: the harness must report how much it
successfully *read*, not only what it found. An extraction script is a
harness.

## 12. Where these came from

Each rule above cost at least one wrong conclusion that was acted on. They
were collected during the `zsh-suite` burndown between September 2026 and the
close of issue #4436, and the worked examples are real rows from that
campaign. Adding to this page is cheaper than rediscovering an entry.
