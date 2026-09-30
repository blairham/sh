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

## 1. The four nothings a mutation harness must be able to say, and one that is not

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

### And one thing that is not nothing: the binary died

A fifth outcome reads as `SURVIVED` and is the opposite of one — the mutant
was detected, and detected as loudly as possible. **The test binary
crashed.**

A mutant that dropped a `-0x80` from a recursive call made the recursion
unbounded. `go test` printed a stack trace, exited non-zero, and printed
`FAIL` for the package — but **no `--- FAIL:` line**, because no test got far
enough to fail. A harness that greps for `--- FAIL` scores that as a
survivor, which is exactly backwards: nothing could be more killed.

> **`FAILED` and `KILLED` are not the same column.** A package that exits
> non-zero with no `--- FAIL:` line died rather than failed; count it as a
> kill and say which it was, because "a test caught this" and "this brought
> the process down" call for different repairs.

The distinction matters beyond the bookkeeping. A crash means the mutant
found a **reachable unbounded path**, so the thing the original code was
doing to prevent it — here subtracting the offset before recursing — is
load-bearing in a way no assertion states. That is worth a comment, and it
is worth knowing it was a crash rather than a diff.

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

## 2. The five readings of `SURVIVED`

A surviving mutant has five causes and **the correct response differs in
every one**, including two where the code is wrong to touch, two where it is
wrong to leave, and one where the code is simply wrong.

| the cause | the code | the mutant |
| --- | --- | --- |
| the harness did nothing | untouched | fix the pattern |
| the tests are too weak | untouched | **add a row** |
| the code is unreachable | **delete it** | goes with it |
| the distinction is unobservable | **keep it** | **remove it** |
| **the rule is wrong** | **change it** | **keep it, as the guard** |

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

**The rule is wrong** is the fifth, and it is the one that does not feel like
a mutation result at all: the tests are not too weak, the code is reachable,
the distinction is observable — and the mutant survives because the original
rule is no more correct than the mutant over everything that was measured.

The worked example is a brace range's zero-padding. The code took the widest
of the two endpoints and the written step; a mutant replacing that maximum
with the step's own width survived. The tests were **complete for the file
the work was aimed at** — every row of `D09brace` passed under either
reading — and widening the grid at exactly that spot showed the reference
does neither: a padded *endpoint* settles the width outright and the step is
consulted only when neither endpoint carries zeros. `{01..3..0005}` is `01`
and not `0001`.

> **A survivor can be evidence about the code rather than about the tests.**
> Before adding a row to kill it, ask whether the two readings differ
> anywhere the *reference* has an opinion — because a mutant as correct as
> the original is saying the original is not correct either.

Two things make this hard to see. The mutant is *equivalent over the measured
inputs*, which is also the signature of an unobservable distinction — so the
tempting reading is "keep the code, delete the mutant", and that ships the
bug. And the suite row the work was aimed at could never have caught it: the
two readings agree wherever the step is no wider than the endpoints, which
was every case in the file. **Completeness against the target is not
completeness about the rule** — the file bounded what had to be measured, and
the rule reaches past it.

The repair is the ordinary one for a wrong rule — change the code — plus one
step: the mutant that restores the wrong version is **kept**, so the next
person to reach for a maximum there does so loudly.

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

### A one-element case can separate two mechanisms a grid cannot

The cheapest discriminating probe is often **smaller** than the case under
test rather than a wider grid of it.

A brace character range over unprintable characters came back as their
printable forms — `{$'\x01'..$'\x02'}` yields `^A ^B`. Two mechanisms fit
that: the *range* renders them, or `print` does. A wider range cannot tell
those apart, because both produce the same thing for every multi-element
case. The pair that settles it is one row each and one of them has a single
element:

	$'\x01'                  the raw byte          -> not print
	{$'\x7f'..$'\x7f'}      ^?                    -> the range renders

A range of one element still goes through the range code and produces one
word, so it isolates the rendering from everything a range does with more
than one endpoint.

> **Before widening a grid, ask whether a smaller case separates the
> hypotheses.** Breadth distinguishes rules that disagree about *inputs*; a
> degenerate case distinguishes rules that disagree about *which stage* is
> responsible.

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
  Section 18 is the case where two different programs were handed over and
  one of them was never invoked.

- **If you invoke a suite driver yourself, you are responsible for everything
  the harness does first** — the per-run copy, the symlink it places, the
  environment it sets. The cheap guard is to run the reference against itself
  and require an empty diff. Section 17 is what it costs when this bullet is
  read and then not enumerated.

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

## 12. A counting tool that dies on its input reads as a count of nothing

Section 11 is about a harness mis-parsing a row. This is its neighbor and
the tool is not the harness: it is `grep`, `awk`, `wc` — whatever counts the
harness's output afterwards.

A driver's transcript held raw `\x80` bytes, because the case under test was
a range of 8-bit characters. `grep -c 'Running test'` on that file printed
**nothing at all** — not zero, nothing — and `awk` died with a multibyte
conversion error on standard error, which was not being read. The counts came
back empty and were interpreted as "the file ran no chunks", which was the
opposite of true: it ran all 28. `LC_ALL=C` in front of both fixed it.

> **A counting tool that cannot read its input looks exactly like a count of
> zero.** Where the thing being measured can emit arbitrary bytes — and a
> shell under test always can — count in the C locale and check the counter's
> own exit status, not only its output.

Two details that made it worse, both worth expecting:

- The failure was **silent in the shape that matters**. `grep` exited
  non-zero with no message; the loop around it substituted an empty string
  into a formatted line, and the line printed as `chunks run:   ok:`, which
  reads as a formatting slip rather than a broken measurement.
- It only appeared **once the work succeeded**. The bytes were in the
  transcript because the front had moved to the 8-bit chunk; every earlier
  run of the same command on the same file had been fine. An instrument can
  be correct for a campaign and fail on the row that closes it.

This is section 1 again from a third direction: the tool must distinguish "I
read the input and found none" from "I could not read the input". Neither
`grep -c` nor `awk` distinguishes those in its output, so the distinction has
to come from the exit status or from removing the failure mode.

### The variant that does not fail at all

A filter that **tidies** its input is as dangerous as one that chokes on it,
and harder to notice, because the output still looks like data.

The same 8-bit range was first read through `od -c | tr -s ' '`. Nothing
failed: `od` rendered every byte, `tr` squeezed the runs of spaces, and the
result was a tidy line of tokens. But `od -c` separates its own output with
spaces *and* renders a literal space byte as a lone space — so squeezing them
made the real separator indistinguishable from the formatting. A three-word
result read as one word for several minutes, which inverted the conclusion:
the question was whether a brace range had expanded at all.

> **Ask what your filter normalizes, and whether the thing under test is made
> of it.** Spacing, case, trailing newlines, quoting, color: each is noise in
> most measurements and the subject in some. `od -An -tx1` fixed this one by
> not having a format whose separator collides with the data.

The general form is worse than a crash because a crash is reported. Here the
pipeline exited 0, printed something plausible, and the loss happened in the
one stage nobody thinks of as part of the instrument.

## 13. A mutant that survives where a comment says it should not

Section 2 lists five readings of `SURVIVED`, all of them about code and
tests. There is a sixth subject a survivor can be evidence about: **a
comment.**

A byte-range helper had a shape test that counted bytes, with a comment
saying it had to — that a four-rune body and a four-byte body would otherwise
take whichever test the caller happened to reach. A mutant replacing the byte
length test with a rune one **survived**, which said the comment was a claim
nothing could support.

> **A mutant that survives where a comment says it should not is the comment
> being checked.** There are two outcomes — the comment is wrong, or the grid
> is missing the case — and they are told apart **by going to the reference,
> not by weakening the comment.**

Here the comment was right and the grid was short. Under `no_multibyte` a
two-byte sequence is **two characters**, so `{$'\xc3\xa9'..a}` is five
characters and four runes: a rune-counting length test takes it for a range
and reads its endpoints from the wrong positions. Three rows, and the mutant
dies.

The failure mode to name is the easy resolution. Softening the comment — "the
byte count is used here" instead of "it must be" — makes the survivor
consistent with the code and leaves the bug, and it looks like diligence
because a claim was made more modest. **A comment that asserts a distinction
is a testable claim**, and a surviving mutant is the test reporting on it.

The converse is worth stating too, because it is the commoner case: where the
comment turns out to be wrong, the repair is the comment *and* the code
simplified to match, not a row added to defend a distinction that does not
exist.

## 14. A check that normalizes its input cannot see the thing under test

Section 12 is about a filter that tidies. This is the same fault inside a
single function call, and it is the sharpest example the campaign produced,
because the production code and the test made **the same mistake from
opposite ends.**

The subject: a brace range whose body holds a byte the encoding cannot
decode. The reference leaves such a word alone.

**In the code**, the body was converted with `[]rune` before anything was
decided. Go maps each undecodable byte to U+FFFD, and the length test that
followed then *passed* — so `{$'\x80'..$'\x81'}` became a one-element range
of U+FFFD and came back as that character. Converting first invented the
thing the check was about.

**In the test**, the assertion that the result holds no U+FFFD was written
`strings.ContainsRune(out, utf8.RuneError)` — and it **failed on correct
output.** That function iterates runes, and Go decodes an undecodable byte
*as* `utf8.RuneError`, so it answers true for the very input the row is built
from.

> **"Could not be decoded" and "contains the replacement character" are
> different facts, and every rune-level reading collapses them.** Ask the
> question at the byte level: `utf8.ValidString` for the first,
> `strings.Contains(s, "\uFFFD")` for the second.

The control that separates them is the one to keep: **a U+FFFD the script
wrote.** It is perfectly decodable, both shells expand a range of it, and
every implementation that checks *after* converting declines it. So it is
both a test row and the row that kills a mutant written as that
implementation — the wrong version is kept as the guard, the way section 2's
fifth reading asks.

The general shape: a normalizing step between the input and the check is part
of the instrument, whether it is a shell pipeline stage, a library function,
or a type conversion. `[]rune`, `strings.ToLower`, `filepath.Clean`,
`strings.TrimSpace` and a `%v` format are all lossy about something, and the
question is only ever whether they are lossy about *this*.

## 15. A uniqueness claim is scoped to the thing it was measured over

"No X can contain this" is a claim about a set, and the set is whichever one
somebody had in mind. Outside it the claim is not weaker — it is **absent**,
and the code that relied on it goes on relying.

The worked example is the sharpest fault this campaign found. A backslash
that arrives in a shell **value** is encoded as a sentinel byte, and the
comment defending the choice says NUL is *the only byte no value can hold*.
That is true, carefully argued, and load-bearing. It is a claim about
**values**.

The sentinel then met `IFS`, whose zsh default is space, tab, newline and
**NUL**. A value's backslash matched `IFS` as itself and split a field that
nobody asked to split. The byte was unique among values and perfectly
ordinary among separators, and nothing in the comment was wrong.

> **Write the domain into the claim.** "No value can hold NUL" invites a
> reader to treat the byte as globally free. "NUL is unavailable to values
> and ordinary everywhere else" would have made the collision visible at the
> line that caused it.

The general shape, because sentinels are where it recurs: a sentinel is safe
only in the alphabet it was chosen against. Ask what *other* alphabets the
value travels through — a separator set, a pattern, a filename, an
environment block, a wire format — because each has its own answer and the
choice was made against one of them.

Two tells that this is what you are looking at:

- The comment is **right** and the code is wrong, so reading harder does not
  help. Section 8 is the reverse case, where the comment is false about the
  code; here it is true about its own subject and silent about the caller's.
- The fault appears in **one dialect or one configuration** and not the
  others. That reads like a dialect bug and is really the alphabet changing
  underneath a shared encoding — four shells here were already correct
  because their `IFS` does not contain the byte, so the collision could not
  arise in them.

### Half a question answered leaves the other half standing

The same site had already been repaired once. An earlier change fixed the
**false negative** — a backslash *written into* `IFS` failing to separate,
because the walk tested the sentinel instead of the character it stands for —
and left the **false positive**, a sentinel that separates because `IFS`
happens to contain its byte. Both are the same sentence misapplied, in
opposite directions, at one position.

It is a relative of the fault where a fix is written into a new helper and
omitted from the old one, but harder to see. There the two halves are in two
places and a reader who finds one can look for the other. Here the question
is **half answered at a single site**: there is one place to look, it has
already been visited, a comment above it says the position was thought
about, and the file that comment points at carries measurements. Everything
about it reads as settled.

> **When a repair resolves an asymmetry, ask what the other direction
> answers.** A helper that returns "not a separator" where the character is
> not in the set has said nothing about what happens when the *sentinel* is,
> and a caller that falls through to the raw byte test will answer that
> question by accident.

The check is one row: hold the position fixed and vary the set the byte is
tested against. Here that is `IFS='\\'` against zsh's default, and the two
rows pull opposite ways — which is also how the reading was chosen, because
the first attempt satisfied one and broke the other and an existing test said
so.

## 16. A found instrument can see more than a purpose-built one

Every other entry here is about an instrument that could not see what it
claimed. This is the opposite case, and it is worth knowing because the
response is different: **a suite file written by people who already knew
which shapes discriminate can be a better instrument than the grid you built
to answer your own question.**

The worked example is `xtrace` routing. Three models of it were written, each
fitting every row of a hand-made grid, and `E02xtrace.ztst` refuted two of
them — because the suite runs each chunk through an `eval` and the chunk
redirects deliberately, a composition no probe in the grid had. The file was
not a target being approached; it was the only instrument in the room that
held the case apart.

> **When a suite file resists a model, read what its chunk asserts as a
> specification rather than as a target.** The expectations were written by
> somebody who knew where the seams are, and the lines they chose to assert
> are a list of the distinctions that matter.

Doing that paid immediately. The chunk's own expectation separates two
questions that had been merged in every model: a **simple command**'s trace
with `2>file` goes to standard error, while a **subshell**'s inner trace with
the same redirection goes to the file. So "before the construct's own
redirections" was already the rule for one shape, and borrowed text was a
second question layered on it. None of the three models was written knowing
that, and no grid built from the front alone would have said so — the front
is one line of a chunk whose other twenty lines are the discriminators.

The practical order, once a file has refused a model twice:

1. Read the chunk's assertions — the `>` and `?` lines — as the specification.
2. Ask which distinction each line exists to make; a suite rarely asserts
   something that only one reading produces.
3. Only then write the next probe, and write it for the distinction the file
   named rather than the one the failure showed.

This does not make a file a *substitute* for a grid. A grid varies one thing
at a time and a suite file does not, so a file tells you *that* a model is
wrong and a grid tells you *which* part. The mistake is treating the file
only as the thing to be satisfied.

### A report written through a shell can lose a phrase without saying so

Adjacent, and about the instrument that carries the findings rather than the
one that makes them.

A write-up posted through a shell heredoc lost three phrases to command
substitution: backticked text — the ordinary way to quote a filename or a
construct in prose — reached a shell and was executed. What arrived read as
**prose with words missing**, not as an error, and two sentences changed
meaning without any sign that anything had happened.

> **Write reports from a file, with something that does not interpolate.**
> A shell is a bad courier for text about shells, because the characters you
> are most likely to quote are the ones it is most likely to read.

Leaving the damaged version in place with a correction beside it is better
than editing it away: the failure mode is invisible by construction, so the
only record that it happened is the one you keep on purpose.

## 17. The environment is an input, and a named field is not a guard

**What caught this is the part to lead with: grading the reference through the
same apparatus and watching it fail identically.** Everything else here
follows from that one step being cheap and nearly skipped.

A row that had just been advanced from 3 chunks run to 7 was re-measured on
merged `main` with a hand-rolled ztst run. It came back **2 run, 1
successful** — a clean, specific, plausible regression, with a named chunk to
blame. The apparatus was missing one symlink.

zsh's suite files do not reach the shell under test only through
`$ZTST_exe`. Many of them spell it `$ZTST_testdir/../Src/zsh`, so without
something at that path every such chunk fails on `no such file or
directory` — **under both shells**. A hand-run that omits it does not error;
it produces a smaller number.

> **Before believing a hand-rolled measurement, run the reference through the
> same apparatus.** A deficient environment and a real regression produce the
> same shape, and only the reference's own row tells them apart.

### The sharp part: the hazard was already documented, on this page

`Suite.ShellAt` in `internal/suite/suite.go` describes this precisely, down to
naming it "the worst shape a harness fault can take", because the two runs
agree and the column reports a healthy number for cases neither shell ran.
Section 6 above carries it as a bullet: *if you invoke a suite driver
yourself, you are responsible for everything the harness does first.* Both
were read before the run, and the run reproduced the fault anyway.

> **A documented hazard that a hand-run silently reproduces is worse than an
> undocumented one, because the reader has met the warning and believes they
> are clear of it.**

That is why this is a section rather than another bullet under section 6. A
statement of responsibility is not a checklist: "everything the harness does
first" cannot be complied with from the sentence, only from the code. The
enumeration is the guard, and it is short enough to write down — for this
suite, a per-run copy of the driver and the file, a `Src/zsh` symlink to the
shell under test, `ZTST_exe`, and the driver invoked as section 18 describes.

**The environment's deficiency need not be local to be silent.** The same
failure has been seen from a container image carrying three locales instead of
several hundred: the *reference* answered `cannot change locale` and every
line after it diverged, reading as a regression of ten lines. Pin what CI
pins, install what CI installs, and state the digest and the reference's own
`--version` beside any number worth acting on.

## 18. The shell a variable names and the shell that runs the driver

Same session, second apparatus fault, and this one produced a **pass**.

With the symlink fixed, both columns came back **8 chunks run, 8 successful**,
on a file that has a chunk we demonstrably fail. The tails were
byte-identical. The cause: zsh's invocation model is

```
ZTST_exe=<shell> <shell> +Z -f ztst.zsh <file>.ztst
```

— the shell under test runs the driver *and* is named in the variable. Setting
only `ZTST_exe` and invoking the driver with the reference grades real zsh
twice. What settled it was `go version -m` reading the **module path** — ours
says `github.com/blairham/sh/cmd/zsh` — not the revision stamp, which section 6
explains is unreliable for anything built in a linked worktree. The honest
numbers are 7/6 against 8/8.

> **Byte-identical output from two shells is the signature of one shell.**

This page already states that a reference must be a different program from the
thing under test (section 6). The reason that bullet did not catch this is
worth the paragraph: **the check was passed, against the wrong fact.** The two
paths in `ZTST_exe` really were two different programs, and verifying them
would have confirmed it. The process that executed the driver was never part
of the comparison, so nothing in the apparatus was lying — one true fact was
checked and a second, unexamined one decided the result.

So this is the third form the echo has taken here, and the forms are not
variations on carelessness:

1. **The reference was our own build** — `build/shells/bash` is `cmd/bash`,
   named `bash` so the suite's `$THIS_SH` re-entry works. The slot was wrong.
2. **A before-binary named across the `sh` boundary** — the same bytes, a
   different POSIX mode, and the two sides were the same program configured
   differently. The name was wrong.
3. **The driver run by the wrong shell with the right variable set** — two
   correct binaries, one of them never invoked. The *invocation* was wrong.

> **Ask which process actually executed the thing you are grading, not only
> which path you handed it.** A shell selected by a variable, by `argv[0]`, by
> a symlink and by the command word are four claims, and a run can satisfy
> three of them while a fourth decides the output.

The general guard is cheap and it is the same one as section 17's: require the
two columns to **disagree somewhere you already know they disagree.** A row
with a known-failing chunk is a positive control, and an apparatus that
reports it passing has told you about itself rather than about the shell.

## 19. Where these came from

Each rule above cost at least one wrong conclusion that was acted on. They
were collected during the `zsh-suite` burndown between September 2026 and the
close of issue #4436, and the worked examples are real rows from that
campaign. Adding to this page is cheaper than rediscovering an entry.
