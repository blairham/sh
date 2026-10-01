# The zsh suite column, measured

Where `cmd/zsh` stands against zsh's own `Test/` directory, file by file.
**Measured at `37355aae5`, 2026-09-29**, with `-jobs 1`. Every number here
carries that commit for the reason `instruments.md` §10 gives: a table without
a commit is a table nobody can tell is stale. Re-measure in place rather than
filing the table as work.

## The figures

    65 files scored
    34 strict            output and status identical
    13 of those          both shells agree on a refusal, not on a result
    21 real results      <- the number the burndown moves
    487 differing lines

**Since that commit, eleven files have become real results**, each verified
byte-identical in output and status under the reference's own driver:

    C02cond          59 of 59 chunks   82566556f   #5146
    D09brace         28 of 28          81099b07f   #5154
    V12zparseopts    14 of 14          fd4c74161   #5162
    V13zformat        6 of 6           2b9d16b91   #5163
    C05debug          8 of 8           0a2ce6566   #5149
    W01history        7 of 7           1e46d356f   #5165
    A02alias         18 of 18          942a026ef   #5237
    B07emulate       20 of 20          9f49a4f6c   #5257
    V04features      24 of 24          a91b224aa   #5158
    D01prompt        16 of 16          (#5150's closing PR)
    V09datetime      16 of 16          (#5160's closing PR; in the suite's image, see below)

So the count the burndown moves is **32** today. The roll-up figures above are left at the commit they were taken at
rather than adjusted by hand: the whole-suite run is CI's, and a line total
edited in place is a number nobody can reproduce.

The ranking table further down carries a **different commit** from these
figures, `fd4c74161`, and says so in its own heading. Two measurements at two
commits in one document is fine as long as each names its own; what is not
fine is a single figure whose provenance is two runs — see §10.

The whole-suite run reports 436 lines over 64 files, because it marks
`V06parameter` unstable and does not score it; run alone that file is stable
and carries 51. 436 + 51 = 487, and real results are 21 either way. A per-file
sum that does not reconcile with the whole-suite run is a fault in one of
them — see `instruments.md` §11 for the one it caught here.

## Real results (21 at `37355aae5`, 32 today)

These carry a result rather than an agreed refusal. `C02cond`, `D09brace`,
`V12zparseopts`, `V13zformat`, `C05debug`, `W01history`, `A02alias`,
`B07emulate`, `V04features`, `D01prompt` and `V09datetime` are the
twenty-second through thirty-second and are listed here rather than below; every
other entry is as measured at the heading's commit.

`V09datetime` is a real result **in the suite's image and not on a Mac**, and
that is a decision rather than a gap. Its `strftime` extensions are the C
library's, the two builds of zsh 5.9.2 carry two libraries, and the file's own
preparation runs its extension chunks only where the library has them — so the
macOS reference skips two chunks that the Debian one runs. #5160 chose the GNU
library, because the image is what CI grades the zsh column against. Measured
2026-10-01 with `suitecheck -dialect zsh -only V09datetime.ztst -jobs 1` run
inside `ghcr.io/blairham/sh/zsh@sha256:aab8255c…` (`zsh 5.9.2
(aarch64-unknown-linux-gnu)`), graded binary named `zsh`: strict 1/1, line
agreement 100%; the driver's own output byte-identical, 16 of 16 chunks. The
same command against `/opt/homebrew/bin/zsh` reads 0/1 strict with 2 differing
lines, and those are the two skipped chunks.

- `A03quoting`
- `A04redirect`

- `A06assign`
- `A07control`

- `B01cd`
- `B04read`

- `B05eval`
- `B06fc`

- `B08shift`
- `B09hash`

- `B10getopts`
- `B11kill`

- `B12limit`
- `B13whence`

- `C02cond` (closed #5146, strict 1/1 at `82566556f`)

- `D05array`
- `D08cmdsubst`

- `D09brace` (closed #5154, strict 1/1 at `81099b07f`)

- `V03mathfunc`
- `V05styles`

- `V12zparseopts` (closed #5162, strict 1/1 at `fd4c74161`)
- `V13zformat` (closed #5163, strict 1/1 at `2b9d16b91`)
- `C05debug` (closed #5149, strict 1/1 at `0a2ce6566`)
- `W01history` (closed #5165, strict 1/1 at `1e46d356f`)
- `V04features` (closed #5158, strict 1/1 at `a91b224aa`, the branch head
  before the squash; 24 of 24 chunks under the reference's own driver)
- `D01prompt` (closed #5150, strict 1/1 with `TERM=xterm-256color`
  inherited, so the two colour chunks ran rather than skipped; 16 of 16
  chunks under the reference's own driver)

- `V08zpty`
- `W02jobs`

- `W03jobparameters`

## Agreement on a refusal (13)

The reference's own driver abandons these too — the fetch unpacks the suite
and not the distribution, so a file reaching for the function library or a
built module fails the same way under both. Counted, never curated, and they
can reach 0 differing lines without ever carrying a result.

- `D02glob`
- `D03procsubst`

- `P01privileged`
- `V01zmodload`

- `V11db_gdbm`
- `X02zlevi`

- `X03zlebindkey`
- `Y01completion`

- `Y02compmatch`
- `Y03arguments`

- `Z01is-at-least`
- `Z02zmathfunc`

- `Z03run-help`

## Failing (20), ranked by chunks unreached

**Measured at `d3a708b3d`, 2026-09-30**, one file per driver run — every row
still in the table. The `ref` column of seven rows was corrected afterwards at
`fbf3ce78e` from the same runs, once #5209 showed those files were not
reference failures; the section on that says which, and by how much. A file that has closed since leaves the table for the
real-results list above and carries **its own commit** there, rather than
being updated in place under this heading's: a figure whose provenance is two
runs is the staleness §10 warns about.

Every row was **re-measured** rather than carried forward. Twice running, no
front had moved; this time **four did** — `V04features` by two chunks (#5158's
work), `E02xtrace` by two, `A05execution` and `B02typeset` by one each. The
last three moved without anybody aiming at them, which is the case for
re-measuring rather than carrying forward: a figure carried forward is
indistinguishable from a figure that was checked, and only one of them can be
relied on. The cost is one driver run per file.

The `lines` column was **not** re-measured this round and still carries
`fd4c74161`, except `V04features` (8, down from 14). It is named here rather
than left to be inferred, because that is the whole of what §10 asks: a
figure's provenance is either written down or it is a guess.

`ref` is how many chunks the **reference's own driver** gets through
successfully in this layout; `ours` is how many this shell does. `unreached`
is the difference, and it is the sort key. `lines` is the differing-line
count, kept as a column because the burndown quotes it — it is **not** the
order.

### How to reproduce this table

Nothing in the tree computes these two columns; they are a driver run per file
per shell, read by hand. Before this section the method was not written down
anywhere, which made the table exactly the thing line 29 of this page calls
out — a number nobody can reproduce. It is:

```
cd build/suite/zsh-zsh-5.9.2/Test
ln -sf <the shell being counted> ../Src/zsh
ZTST_verbose=2 ZTST_exe=<same shell> <same shell> +Z -f ./ztst.zsh ./<file>.ztst
```

and the count is the `Running test` lines, **less one if the run printed
`: test failed.`** — the chunk it stopped on was attempted, not gotten
through. Three ways to get it wrong, each of which produces a plausible
number rather than an error:

- **Run it from anywhere but `Test/`.** `ZTST_testdir` comes out as the cwd,
  so the suite's own `$ZTST_testdir/../Src/zsh` misses and the *reference*
  fails its own file. `D04parameter`'s ref reads **11** instead of 246 that
  way, and six of the failing rows come back at **0 unreached**, which reads
  as nothing to do rather than as a dead instrument.
- **Run the files in parallel.** They interfere. At four at a time the
  reference disagrees with itself: `B02typeset` at **62** and `E01options` at
  **50**, against 80 and 94 run one at a time. A regression and a concurrent
  sweep are indistinguishable in this column, and the sweep is the faster
  thing to suspect.
- **Count attempted instead of completed.** Off by exactly one on every row
  that fails, which is every row in the table, and the two numbers are both
  plausible.

`ZTST_verbose=2` is needed for the `Running test` lines and does not change
how far either shell gets (checked at 0 and 2 on four files). The harness
itself never sets it.

### A cause proposed from the shape of the numbers, and what it cost

Run the reference the way the block above says and its `ref` comes out
**higher** on eight files than the table recorded — and the excess was, row
for row, the `ungradeable` column those files carried:

| file | old `ref` | reference alone | delta | old `ungradeable` |
|---|---:|---:|---:|---:|
| `E03posix` | 10 | 18 | +8 | 8 |
| `D07multibyte` | 51 | 53 | +2 | 2 |
| `V14system` | 14 | 16 | +2 | 2 |
| `V09datetime` | 14 | 16 | +2 | 2 |
| `D04parameter` | 244 | 246 | +2 | 2 |
| `D06subscript` | 36 | 37 | +1 | 1 |
| `D01prompt` | 15 | 16 | +1 | 1 |
| `E02xtrace` | 5 | 6 | +1 | 1 |

Nine for nine with `V06parameter`, which is 0 either way, and **zero delta on
the other fifteen files** — a correspondence that good is an explanation
rather than a coincidence, and the table above now carries the corrected
`ref`.

**The first reading of it was that a resolvable module path recovered the
twenty, and that was wrong.** Locale had been excluded properly — identical
counts at `LC_ALL=C` and with `LANG`, `LC_ALL` and `LC_CTYPE` unset — and
then a cause was asserted anyway, on no better evidence than that it would
explain the table. It does not survive the harness: `E03posix` reaches 18 of
18 under the harness's own environment, `env -i` with `PATH` lacking
`/opt/homebrew/bin`, `LC_ALL=C LANG=C TERM=dumb TZ=UTC` and `HOME`/`TMPDIR`
in the run directory, and in a minimal layout with only the `.ztst` and
`ztst.zsh` beside it.

**Excluding one suspect is not establishing the next.** The evidence above
narrows what the cause can be and says nothing about what it is, and the gap
between those two is where a plausible sentence gets written down as a
finding. What settled it was a counter that could see a failing file, which
is #5203 and the ungradeable section below.

### Why not differing lines

The previous version of this table sorted by `lines`, and that ordering is
wrong in both directions. The two columns are close to uncorrelated:

- `C01arith` is **6** lines and **55** chunks unreached — second-cheapest by
  one measure and among the most expensive by the other.
- `E02xtrace` is **53** lines and **3** chunks unreached: the most expensive
  row in the old table and one of the cheapest here.
- `A09zwc` was **15** lines and **1** chunk unreached — the sharpest of the
  four, and it is no longer in the table at all: measured, it turned out to
  be a decline (#5141). By lines it looked mid-table; by chunks it looked
  joint-cheapest; it was neither.
- `D04parameter` is **19** lines and **234** chunks unreached, by far the
  largest piece of work on the page and unremarkable in the old order.

The reason is the one `instruments.md` §3 gives: a file's differing-line count
mostly measures **how verbose its first failure's report is**, because the
driver abandons a file at that chunk. A silent divergence and a fifty-line
diff can be the same one bug, and the same bug reported twice as loudly
doubles the number.

**This page warned about that and then sorted by it**, which is worth saying
plainly: a document can carry a true caution one level up and a false
ordering one level down, and the caution makes the ordering *more*
convincing rather than less. It is the shape `instruments.md` §8 describes
for a comment — true about the world, false about the code — happening in
prose about a table.

Counting chunks is not a cost estimate either — it counts what is
*unreached*, not how many root causes are in the way. `C02cond` took **six**
roots to clear 59 chunks, and `D09brace` took **three** to clear its last
one: the ranking put it joint-cheapest at a single unreached chunk, and that
chunk held an escape rendering, an undecodable-body decline and an inert
option, in three changes. So a one-chunk row is the *shortest* piece of work
on the page and not necessarily a small one.

`W01history` is the sharpest of the three, because it says something the other
two do not: it sat at **6** chunks unreached and took **five** roots, and
**the front this table named was the first of the five.** The column says what
a file stops on, and what it stops on is one root — the other four were behind
it and unnameable until it closed. In order: the default history file was the
substrate's for every dialect (#5195), a substitution's replacement unescapes
twice (#5196), `:h` and `:t` take a count (#5197), `:P` resolves as far as it
resolves (#5198), and `fc -p` reads into the new list while `fc -l` skips only
its own line (#5199). Two of those are not history expansion at all.

So the ranking is a sort key and not an estimate, and the `today's front`
column is a **label on the first root**, not a description of the work.

And **a row can be unreachable rather than expensive**, which the count
cannot show either. `A09zwc` was joint-cheapest here at a single unreached
chunk and is now a decline: its chunk needs a `.zwc` carrying valid wordcode
magic, and this shell writes text by a decision recorded in
`dialect/zsh/declined.go`. One unreached chunk, and no amount of work on this
shell closes it.

And **a row can be gated on another row**, in which case the count is not
merely blurred but *inverted*: a file that delegates to a large file reports a
small number precisely because it stops early. `V10private` is the case, and
it has its own paragraph under the table.

So four distinct ways the chunk count misleads about cost, all measured on
this page rather than argued: it is not a root-cause count, a one-chunk row
can be three roots, a cheap row can be unreachable, and a cheap row can be
gated on the most expensive one.

The fourth was in the prose under the table before it was in this list, and
that is the whole reason it is here now: a front was chosen off the sort and
the paragraph five lines below it — the one saying that row's number is the
most misleading in the table — went unread. **A caution that is not in the
enumeration a reader sorts by is a caution that gets sorted past.** The same
correction this page made to itself once already, one level down.

It is still a better proxy than lines, because it is monotone: closing a
front can only move chunks up.

| unreached | ref | ours | lines | file | today's front |
|---:|---:|---:|---:|---|---|
| — | 0 | 1 | 51 | `V06parameter` | *excluded — the reference fails its own first chunk* |
| 2 | 25 | 23 | 8 | `V10private` | typeset still works with zsh/param/private module loaded |
| 3 | 5 | 2 | 53 | `E02xtrace` | xtrace with and without redirection |
| 12 | 12 | 0 | 2 | `V07pcre` | nothing runs; the reference's own first chunk is `Testing PCRE multibyte with locale en_US.UTF-8` |
| 12 | 12 | 0 | 20 | `X04zlehighlight` | region highlight - standout overlapping on other region_highlight entry |
| 14 | 39 | 25 | 21 | `A05execution` | Bug regression: piping a shell construct to an external process may hang |
| 15 | 16 | 1 | 10 | `V14system` | zsystem flock invalid time arguments |
| 16 | 16 | 0 | 11 | `V09datetime` | basic format specifiers |
| 18 | 18 | 0 | 18 | `E03posix` | Parameter hiding and tagging, printing types and values |
| 36 | 37 | 1 | 10 | `D06subscript` | Scalar pattern subscripts with wildcards |
| 38 | 66 | 28 | 9 | `B03print` | out of range argument specifier |
| 41 | 52 | 11 | 12 | `C04funcdef` | Command not found handler, success |
| 50 | 53 | 3 | 9 | `D07multibyte` | Subscript searching with multibyte characters |
| 51 | 79 | 28 | 18 | `B02typeset` | Left justification of floating point |
| 55 | 73 | 18 | 6 | `C01arith` | error using unset variable as index |
| 61 | 61 | 0 | 6 | `V02zregexparse` | empty |
| 74 | 75 | 1 | 15 | `C03traps` | Nested TRAPEXIT |
| 87 | 109 | 22 | 12 | `A01grammar` | how arguments to `exec -a` are handled (paraphrased: the chunk's own title uses a spelling the commit hook rejects) |
| 87 | 94 | 7 | 13 | `E01options` | BRACE_CCL option starting from NUL |
| 236 | 246 | 10 | 19 | `D04parameter` | interactive shell returns to top level on ${...?...} error |

### Three things the table cannot say on its own

**The `ref` column is what the reference's own driver gets through
*successfully*, not what the file contains.** Nine of these thirty files have
a reference that fails its own chunks here, which is a large enough fact to
have its own section below — read it before treating `ref` as a target.

**`V10private`'s last chunk is an entire other file.** It re-runs all 79
chunks of `B02typeset.ztst` under `zsh/param/private`, and this shell is at
28 of 79 there. So its `unreached` of **2** is the most misleading number in
the table: one of those two chunks is `B02typeset`'s whole 51-chunk gap plus
whatever the private module adds. Any file whose chunk re-runs another file
needs that said beside it, because the count cannot show it.

**And two rows are settled declines rather than work**: `V07pcre` (#4737) and
`V02zregexparse` (#4761). They are in the table because they are in the
denominator, not because anybody should pick them up. `A09zwc` was the third
and is no longer a row at all — see the declines above and #5141.

## Part of this suite is ungradeable here, and it is two files

**Among the twenty-seven failing files, two have a reference that fails its
own chunks in this layout**: `V06parameter` and `E02xtrace`.

### This section said nine, and nine came from a counter that could not fire

The figure stood at nine files and twenty chunks from `fd4c74161` until
#5203. It was read off `StrictOnFailure`, whose denominator is the **strict**
files — and a file whose reference failed is by that fact a file the two
shells rarely match byte for byte, so it is never strict, so it is never
counted. Asked per file the pair returns `0/0` on `V06parameter`, whose
reference stops at its own first chunk, exactly as readily as on a file with
nothing wrong. **A vacuous denominator reads like a clean result.**

`RefDriverFailed` (#5209) asks the same question over every scored file, and
it was checked against `V06parameter` — **1/1** — before any of its zeroes
were believed. Over the nine:

| file | reference's driver failed |
|---|---|
| `V06parameter` | **yes** |
| `E02xtrace` | **yes** |
| `E03posix`, `D07multibyte`, `V14system`, `V09datetime`, `D04parameter`, `D06subscript`, `D01prompt` | no |

Confirmed from outside the harness too, one driver run per file: those seven
pass every chunk they have — `E03posix` 18 of 18 under the harness's own
environment (`env -i`, `PATH` without `/opt/homebrew/bin`, `LC_ALL=C LANG=C
TERM=dumb TZ=UTC`, `HOME` and `TMPDIR` in the run directory) and in a minimal
layout. `E02xtrace` is the one file the two methods part on: it fails in the
harness and passes outside it, which is one file's worth of environment and
not a class.

**The seven rows' targets went up accordingly** and the table above carries
the corrected `ref`: `E03posix` 10 → **18**, `D04parameter` 244 → **246**,
`D07multibyte` 51 → **53**, `V14system` and `V09datetime` 14 → **16**,
`D01prompt` 15 → **16**, `D06subscript` 36 → **37**. `E03posix` moves from
10 unreached to **18**, which is most of a row's worth of work that was being
held out of the denominator.

### Why the direction matters

**Every other correction on this page took a number that flattered this shell
and made it worse. This one was flattering us.** Eighteen chunks were held out
of the target as things the reference cannot run here, and they are things it
runs. A target built from what was not *obviously* excluded is how the old bar
of 58 came about — this is that error with the sign reversed, and it survived
longer precisely because nobody interrogates a number that makes the gap look
smaller.

### What is left, and it is real

**`V06parameter` is the sharpest case and the only one where the sign flips.**
Its reference passes **0 of 1**: it stops at its own first chunk because
`zsh/parameter` will not `dlopen` from `./Modules` in this layout. So this
shell gets *further* than the reference does, its `unreached` is **negative**,
and the 51-line figure it used to carry measured nothing at all. That is not
instability in our column — it is the driver.

**`E02xtrace` is the second**, and it is the one to re-check rather than
trust: it fails the reference inside the harness and passes outside it, so
whichever of the two environments is the odd one out, the row's `ref` of 5 is
resting on the harness's answer alone.

**The cause of both is the fetch**: it unpacks `Test/` and not the
distribution, so a file reaching for a built module, the function library, or
`./Modules` fails under *both* shells. Where that takes the whole file, it
lands in the thirteen refusal-agreements. What would move it, and neither is
work on this shell: unpack the distribution rather than `Test/`, or build the
modules these files load.

**Only the failing files were swept for this** — a count of the sweep and not
of the board. The strict files and the thirteen refusal-agreements have not
been checked the same way, so the figure is two *of those*, and the true
number across all sixty-five is unmeasured. Stated rather than extrapolated,
because a number produced by assuming the rest are clean is the shape this
section is about.

## What the reachable target is

The bar this column was given in September was **58 of 65**, set on the first
measured run and never revisited. It is wrong in both directions now:

- **Three files are settled declines**, not work: `V07pcre` (#4737),
  `V02zregexparse` (#4761) and `A09zwc` (#5141). They cannot become real
  results and should never have been in the denominator. The first two are
  declined because there is no specification on the green list to implement
  from; the third is a different reason and a distinct category — the
  artifact its chunk requires cannot be produced without reading the format.
  All three are recorded in `dialect/zsh/declined.go`, each with its cost,
  its reason and what would change the answer.
- **Twelve files agree on a refusal** because the fetch unpacks `Test/` and
  not the distribution. Several need a module this shell does not build, and
  whether they are reachable at all is a separate question from whether their
  front is fixable.

So the honest statement of the target is **29 of 50 reachable today** — 21 at
the figures' commit plus `C02cond`, `D09brace`, `V12zparseopts`, `V13zformat`,
`C05debug`, `W01history`, `A02alias` and `B07emulate` — where 50
is 65 less the three declines and
less the twelve refusal-agreements, with the twelve re-examined individually
rather than assumed unreachable since some may become reachable if the module
they want is built. 58 is not a target anybody measured; it is the count of
files that are not obviously excluded.

**The denominator went down by one without any work being done**, when
`A09zwc` was measured and declined. That is legitimate and worth saying
plainly: a file that cannot carry a real result on current terms was being
counted as though it could, so the old figure was the optimistic one. A
decline moves the board by removing a row nobody could have finished, not by
lowering a bar.

`V06parameter` is a third exclusion on top of those two, for a different
reason: the declines are files that cannot carry a result, and this one is a
file this layout cannot **grade** — see the ranking table's notes. It is left
in the 51 rather than taken out of it, because taking it out would change a
denominator on the strength of a driver problem that may be fixable.
