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

**Since that commit, `C02cond` has become a real result**, verified strict
1/1 at `82566556f` — 59 of 59 chunks, byte-identical output and status under
the reference's own driver (#5146). So the count the burndown moves is **22**
today. The roll-up figures above are left at the commit they were taken at
rather than adjusted by hand: the whole-suite run is CI's, and a line total
edited in place is a number nobody can reproduce.

The ranking table further down carries a **different commit** from these
figures, `ac43cb67a`, and says so in its own heading. Two measurements at two
commits in one document is fine as long as each names its own; what is not
fine is a single figure whose provenance is two runs — see §10.

The whole-suite run reports 436 lines over 64 files, because it marks
`V06parameter` unstable and does not score it; run alone that file is stable
and carries 51. 436 + 51 = 487, and real results are 21 either way. A per-file
sum that does not reconcile with the whole-suite run is a fault in one of
them — see `instruments.md` §11 for the one it caught here.

## Real results (21 at `37355aae5`, 22 today)

These carry a result rather than an agreed refusal. `C02cond` is the
twenty-second and is listed here rather than below; every other entry is as
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

- `V03mathfunc`
- `V05styles`

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

## Failing (30), ranked by chunks unreached

**Measured at `ac43cb67a`, 2026-09-29**, one file per driver run.

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
- `A09zwc` is **15** lines and **1** chunk unreached.
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
*unreached*, not how many root causes are in the way, and `C02cond` took six
roots to clear 59. It is a better proxy because it is monotone: closing a
front can only move chunks up.

| unreached | ref | ours | lines | file | today's front |
|---:|---:|---:|---:|---|---|
| — | 0 | 1 | 51 | `V06parameter` | *excluded — see below* |
| 1 | 2 | 1 | 15 | `A09zwc` | workers/54571: Malformed .zwc with implausible npats does not crash the shell |
| 1 | 28 | 27 | 9 | `D09brace` | range of 8bit chars, multibyte option unset |
| 2 | 25 | 23 | 8 | `V10private` | typeset still works with zsh/param/private module loaded |
| 3 | 14 | 11 | 24 | `V12zparseopts` | special characters in option names |
| 5 | 5 | 0 | 53 | `E02xtrace` | xtrace with and without redirection |
| 6 | 8 | 2 | 12 | `C05debug` | Skip line from DEBUG trap |
| 6 | 6 | 0 | 19 | `V13zformat` | basic zformat test |
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

**Nine files have a reference that fails its own chunks here**, so the
reference's run is not a clean denominator and `ref` is its *successful*
count rather than its total: `D07multibyte` 51 of 53, `V14system` 14 of 16,
`V09datetime` 14 of 16, `D06subscript` 36 of 37, `D01prompt` 15 of 16,
`D04parameter` 244 of 246, `E02xtrace` 5 of 6, `E03posix` **10 of 18**, and
`V06parameter` **0 of 1**. Where the reference fails, the chunk is outside
what this layout can grade at all.

**`V06parameter` is excluded, and the instrument is why.** Its `ref` is 0:
the reference stops at its own first chunk because `zsh/parameter` will not
`dlopen` from `./Modules` in this layout. This shell gets *further* than the
reference does, which is why its `unreached` is negative and why the 51-line
figure it used to carry measured nothing. That is not instability in our
column — it is the driver, and no amount of work on this shell moves it.

**`V10private`'s last chunk is an entire other file.** It re-runs all 79
chunks of `B02typeset.ztst` under `zsh/param/private`, and this shell is at
27 of 79 there. So its `unreached` of **2** is the most misleading number in
the table: one of those two chunks is `B02typeset`'s whole 52-chunk gap plus
whatever the private module adds. Any file whose chunk re-runs another file
needs that said beside it, because the count cannot show it.

**And two rows are settled declines rather than work**: `V07pcre` (#4737) and
`V02zregexparse` (#4761). They are in the table because they are in the
denominator, not because anybody should pick them up.

## What the reachable target is

The bar this column was given in September was **58 of 65**, set on the first
measured run and never revisited. It is wrong in both directions now:

- **Two files are settled declines**, not work: `V07pcre` (#4737) and
  `V02zregexparse` (#4761). They cannot become real results and should never
  have been in the denominator.
- **Twelve files agree on a refusal** because the fetch unpacks `Test/` and
  not the distribution. Several need a module this shell does not build, and
  whether they are reachable at all is a separate question from whether their
  front is fixable.

So the honest statement of the target is **22 of 51 reachable today** — 21 at
the figures' commit plus `C02cond` — where 51 is 65 less the two declines and
less the twelve refusal-agreements, with the twelve re-examined individually
rather than assumed unreachable since some may become reachable if the module
they want is built. 58 is not a target anybody measured; it is the count of
files that are not obviously excluded.

`V06parameter` is a third exclusion on top of those two, for a different
reason: the declines are files that cannot carry a result, and this one is a
file this layout cannot **grade** — see the ranking table's notes. It is left
in the 51 rather than taken out of it, because taking it out would change a
denominator on the strength of a driver problem that may be fixable.
