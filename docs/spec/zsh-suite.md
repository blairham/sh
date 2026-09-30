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

**Since that commit, four files have become real results**, each verified
byte-identical in output and status under the reference's own driver:

    C02cond          59 of 59 chunks   82566556f   #5146
    D09brace         28 of 28          81099b07f   #5154
    V12zparseopts    14 of 14          fd4c74161   #5162
    V13zformat        6 of 6           2b9d16b91   #5163

So the count the burndown moves is **25** today. The roll-up figures above are left at the commit they were taken at
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

## Real results (21 at `37355aae5`, 25 today)

These carry a result rather than an agreed refusal. `C02cond`, `D09brace`,
`V12zparseopts` and `V13zformat` are the twenty-second through
twenty-fifth and are listed here rather than below; every other entry is as
measured at the heading's commit.

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

## Failing (26), ranked by chunks unreached

**Measured at `fd4c74161`, 2026-09-30**, one file per driver run, except
`C05debug`, re-measured at `1c7625a42` after #5187 and carrying its own
figures — 8 ref, 6 ours, and a front that has moved to the next chunk. A row
updated in place without its own commit beside it is the staleness §10 warns
about, so it is named rather than absorbed.

Every row was **re-measured** rather than carried forward, and for the second
time in a row **no front had moved** across the merges in between. That
negative result is what makes the table worth reading: a figure carried
forward is indistinguishable from a figure that was checked, and only one of
them can be relied on. The cost is one driver run per file.

`ref` is how many chunks the **reference's own driver** gets through
successfully in this layout; `ours` is how many this shell does. `unreached`
is the difference, and it is the sort key. `lines` is the differing-line
count, kept as a column because the burndown quotes it — it is **not** the
order.

### Why not differing lines

The previous version of this table sorted by `lines`, and that ordering is
wrong in both directions. The two columns are close to uncorrelated:

- `C01arith` is **6** lines and **55** chunks unreached — second-cheapest by
  one measure and among the most expensive by the other.
- `E02xtrace` is **53** lines and **5** chunks unreached: the most expensive
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
| — | 0 | 1 | 51 | `V06parameter` | *excluded — see below* |
| 2 | 25 | 23 | 8 | `V10private` | typeset still works with zsh/param/private module loaded |
| 2 | 8 | 6 | 23 | `C05debug` | ZSH_DEBUG_CMD in debug traps |
| 5 | 5 | 0 | 53 | `E02xtrace` | xtrace with and without redirection |
| 6 | 7 | 1 | 21 | `W01history` | History line numbering |
| 9 | 24 | 15 | 14 | `V04features` | Failed to add parameter if local parameter present |
| 10 | 10 | 0 | 18 | `E03posix` | Parameter hiding and tagging, printing types and values |
| 12 | 12 | 0 | 2 | `V07pcre` | nothing runs; the reference's own first chunk is `Testing PCRE multibyte with locale en_US.UTF-8` |
| 12 | 12 | 0 | 20 | `X04zlehighlight` | region highlight - standout overlapping on other region_highlight entry |
| 13 | 20 | 7 | 21 | `B07emulate` | Sticky emulation not triggered if sticky emulation unchanged |
| 13 | 15 | 2 | 15 | `D01prompt` | `` `%_' `` prompt escape |
| 13 | 14 | 1 | 10 | `V14system` | zsystem flock invalid time arguments |
| 14 | 18 | 4 | 9 | `A02alias` | POSIX_ALIASES option |
| 14 | 14 | 0 | 11 | `V09datetime` | basic format specifiers |
| 15 | 39 | 24 | 21 | `A05execution` | Bug regression: piping a shell construct to an external process may hang |
| 35 | 36 | 1 | 10 | `D06subscript` | Scalar pattern subscripts with wildcards |
| 38 | 66 | 28 | 9 | `B03print` | out of range argument specifier |
| 41 | 52 | 11 | 12 | `C04funcdef` | Command not found handler, success |
| 48 | 51 | 3 | 9 | `D07multibyte` | Subscript searching with multibyte characters |
| 52 | 79 | 27 | 18 | `B02typeset` | Left justification of floating point |
| 55 | 73 | 18 | 6 | `C01arith` | error using unset variable as index |
| 61 | 61 | 0 | 6 | `V02zregexparse` | empty |
| 74 | 75 | 1 | 15 | `C03traps` | Nested TRAPEXIT |
| 87 | 109 | 22 | 12 | `A01grammar` | how arguments to `exec -a` are handled (paraphrased: the chunk's own title uses a spelling the commit hook rejects) |
| 87 | 94 | 7 | 13 | `E01options` | BRACE_CCL option starting from NUL |
| 234 | 244 | 10 | 19 | `D04parameter` | interactive shell returns to top level on ${...?...} error |

### Three things the table cannot say on its own

**The `ref` column is what the reference's own driver gets through
*successfully*, not what the file contains.** Nine of these thirty files have
a reference that fails its own chunks here, which is a large enough fact to
have its own section below — read it before treating `ref` as a target.

**`V10private`'s last chunk is an entire other file.** It re-runs all 79
chunks of `B02typeset.ztst` under `zsh/param/private`, and this shell is at
27 of 79 there. So its `unreached` of **2** is the most misleading number in
the table: one of those two chunks is `B02typeset`'s whole 52-chunk gap plus
whatever the private module adds. Any file whose chunk re-runs another file
needs that said beside it, because the count cannot show it.

**And two rows are settled declines rather than work**: `V07pcre` (#4737) and
`V02zregexparse` (#4761). They are in the table because they are in the
denominator, not because anybody should pick them up. `A09zwc` was the third
and is no longer a row at all — see the declines above and #5141.

## Part of this suite is ungradeable here, and it is nine files

`V06parameter`'s `dlopen` failure is not a special case. It is the visible end
of a class, and the class is larger than one file: **among the twenty-seven
failing files, nine have a reference that fails its own chunks in this
layout.** Measured at `fd4c74161`, the same runs the table above comes from.

| file | reference passes | of | ungradeable |
|---|---:|---:|---:|
| `E03posix` | 10 | 18 | **8** |
| `D07multibyte` | 51 | 53 | 2 |
| `V14system` | 14 | 16 | 2 |
| `V09datetime` | 14 | 16 | 2 |
| `D04parameter` | 244 | 246 | 2 |
| `D06subscript` | 36 | 37 | 1 |
| `D01prompt` | 15 | 16 | 1 |
| `E02xtrace` | 5 | 6 | 1 |
| `V06parameter` | 0 | 1 | 1 |

Twenty chunks of 1189, and eight of the twenty are `E03posix` alone. The
reference runs 1189 chunks across these files and passes 1169 of them.

**Why it matters more than twenty chunks sounds like.** A `ref` column read as
"what zsh can do" over-counts the target, and that is precisely how the old
bar of 58 came about: a number taken from what was not *obviously* excluded
rather than from what had been measured. The same reading applied per file
would put work on rows where no amount of work on this shell changes the
answer.

**`V06parameter` is the sharpest case and the only one where the sign flips.**
Its reference passes **0 of 1**: the reference stops at its own first chunk
because `zsh/parameter` will not `dlopen` from `./Modules` in this layout. So
this shell gets *further* than the reference does, its `unreached` is
**negative**, and the 51-line figure it used to carry measured nothing at all.
That is not instability in our column — it is the driver.

**The cause is the same one the refusal-agreements have**, and it is worth
saying in one place: the fetch unpacks `Test/` and not the distribution, so a
file reaching for a built module, the function library, or `./Modules` fails
under *both* shells. Where that takes the whole file, it lands in the thirteen
refusal-agreements. Where it takes some chunks and not others — `E03posix`,
eight of eighteen — the file still scores, and the part that cannot be graded
hides inside a number that looks like a measurement.

**What would move it**, and neither is work on this shell: unpack the
distribution rather than `Test/`, or build the modules these files load. Until
one of those happens, `ref` is the honest denominator and the file's own chunk
count is not.

**Only the failing files were swept for this**, the twenty-seven failing at
that commit — a count of the sweep and not of the board, which has moved
since and will again. The strict files
and the thirteen refusal-agreements have not been checked the same way, so
the count is nine *of those* and the true figure across all
sixty-five is unmeasured. Stated rather than extrapolated, because a number
produced by assuming the rest are clean is the shape this section is about.

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

So the honest statement of the target is **25 of 50 reachable today** — 21 at
the figures' commit plus `C02cond`, `D09brace`, `V12zparseopts` and
`V13zformat` — where 50
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
