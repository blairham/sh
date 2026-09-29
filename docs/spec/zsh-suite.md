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

The whole-suite run reports 436 lines over 64 files, because it marks
`V06parameter` unstable and does not score it; run alone that file is stable
and carries 51. 436 + 51 = 487, and real results are 21 either way. A per-file
sum that does not reconcile with the whole-suite run is a fault in one of
them — see `instruments.md` §11 for the one it caught here.

## Real results (21)

These carry a result rather than an agreed refusal.

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

## Failing (31), ranked by differing lines

The front is the **first chunk the reference's own driver reports failing**,
measured the same day. It is not the only thing wrong with the file: closing
one front routinely reveals the next, and `A04redirect` took ten.

| lines | file | today's front |
|---:|---|---|
| 2 | `V07pcre` | (no Was-testing line; ref says: Testing PCRE multibyte with locale en_US.UTF-8) |
| 5 | `C02cond` | -v cond |
| 6 | `C01arith` | error using unset variable as index |
| 6 | `V02zregexparse` | empty |
| 8 | `V10private` | typeset still works with zsh/param/private module loaded |
| 9 | `A02alias` | POSIX_ALIASES option |
| 9 | `B03print` | out of range argument specifier |
| 9 | `D07multibyte` | Subscript searching with multibyte characters |
| 9 | `D09brace` | Numeric range expansion, stepping and padding (1) |
| 10 | `D06subscript` | Scalar pattern subscripts with wildcards |
| 10 | `V14system` | zsystem flock invalid time arguments |
| 11 | `V09datetime` | basic format specifiers |
| 12 | `A01grammar` | how arguments to `exec -a` are handled (chunk paraphrased; the file's own title uses a spelling the hook rejects) |
| 12 | `C04funcdef` | Command not found handler, success |
| 12 | `C05debug` | Skip line from DEBUG trap |
| 13 | `E01options` | BRACE_CCL option |
| 14 | `V04features` | Failed to add parameter if local parameter present |
| 15 | `A09zwc` | workers/54571: Malformed .zwc with implausible npats does not crash the shell |
| 15 | `C03traps` | Nested TRAPEXIT |
| 15 | `D01prompt` | `%_' prompt escape |
| 18 | `B02typeset` | Left justification of floating point |
| 18 | `E03posix` | Parameter hiding and tagging, printing types and values |
| 19 | `D04parameter` | interactive shell returns to top level on ${...?...} error |
| 19 | `V13zformat` | basic zformat test |
| 20 | `X04zlehighlight` | region highlight - standout overlapping on other region_highlight entry |
| 21 | `A05execution` | Bug regression: piping a shell construct to an external process may hang |
| 21 | `B07emulate` | Sticky emulation not triggered if sticky emulation unchanged |
| 21 | `W01history` | History line numbering |
| 24 | `V12zparseopts` | special characters in option names |
| 51 | `V06parameter` | *unmeasured* |
| 53 | `E02xtrace` | < Was testing: a function that redefines itself preserves tracing |

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

So the honest statement of the target is **21 of 51 reachable today**, where
51 is 65 less the two declines and less the twelve refusal-agreements — with
the twelve re-examined individually rather than assumed unreachable, since
some may become reachable if the module they want is built. 58 is not a
target anybody measured; it is the count of files that are not obviously
excluded.
