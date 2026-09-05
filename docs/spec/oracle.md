# The oracle method

Behavior is learned by **running real shell binaries and recording what
they do**. This is the primary input to every spec entry, and it is
clean-room safe: a snippet is ours, and the output of a binary is a fact
about that binary, not an expression of anyone's authorship.

It is also better evidence than reading an implementation, because it
measures the shell people actually run rather than what some third party
believed it did.

## The reference panel

| shell | role |
| --- | --- |
| `dash` | the minimal `/bin/sh`; Debian and Ubuntu ship it as `sh` |
| `bash` 5.x | the dominant scripting target |
| `bash` 3.2 | what macOS still ships; the oldest target that matters |
| `ksh93` | the other ksh-family lineage |
| `zsh` | the interactive incumbent, and the most divergent semantics |

`bash` is run **both as `bash` and as `sh`**, because argv[0] changes the
language: bash 3.2 has process substitution when invoked as `bash` and
rejects it as `sh`. Any claim about "bash" that does not say how it was
invoked is incomplete.

## Producing a fact

A case is a snippet plus, for each shell in the panel, the combined
output and exit status observed. Both halves are recorded; a construct
that prints the same thing with a different status is still a difference.

Rules that keep the facts honest:

- **Compare against the binary, never against a belief.** If the panel
  disagrees with the POSIX text, record what the binaries did and note
  the divergence. The specification says what should happen; the panel
  says what does.
- **Say which build produced the row.** Versions change answers —
  `${x^^}` is a bash 4 feature and bash 3.2 rejects it.
- **A redirect on an assignment is not an assignment.** `r=2 2>/dev/null`
  is a command with a prefix, and it takes a different path through the
  shell than a plain `r=2`. Probes must isolate the construct they claim
  to measure; a contaminated probe produces a confident wrong fact.
- **Prefer a script file to `-c`** where the construct interacts with
  how input is read, and say which was used.
- **Spell the whole invocation out where the invocation is the fact.**
  `-e` with a script, a bundle of option letters, `-o name`, which
  operand becomes `$0` — these are decided before a line of the snippet
  runs, and a case that only ever arrives through `-c` cannot ask about
  them.

## Cases that choose their own argv

`Case.Args` is the argv, in place of the harness's own `-c` and snippet.
Both sides of a comparison are handed the same words, after the flags
that say which shell each of them is, so an option the harness has never
heard of is still the same option in both runs.

The snippet still has to reach the shell, and `Args` says where it goes:
`oracle.ArgSnippet` is replaced by the snippet as one word, and
`oracle.ArgScript` by the path of a file holding it — the same file
`Script` writes, normalized to `<script>` in the output. At most one may
appear. A case with neither never hands the shell its snippet, which is
how an invocation that must fail before it reads anything is written.

This exists because the invocation surface was graded by nothing else.
The corpus started every case the same way, so the front end was pinned
only by its own unit tests — the shape of the incident where the drivers
scored 178/198 against a core scoring 198/198.

## Cases that supply the shell's standard input

`Case.Stdin` is what the shell finds on its own standard input, handed to
both sides byte for byte. Empty means closed, which is what every other
case gets, so the record cannot depend on what the harness itself was
started with.

Where the *program* comes from is a separate question, and the two
combine three ways. With the snippet arriving the usual way, `Stdin` is
data — the line `read` consumes, the choice `select` is answered with;
those are otherwise reachable only through a here-string or a pipe
written inside the snippet, and neither is the shell's own input. With
`Args` naming no placeholder, the program itself arrives on standard
input: the `sh < script` and `echo … | sh` route, where `$0` stays the
shell rather than becoming a path, and where `-s` makes every operand a
positional parameter. With a script file, the file is the program and the
input is still data, which is the ordinary shape of a script that reads.

`ArgSnippet` and `ArgScript` are honored in `Stdin` as well, wherever
they appear rather than only as the whole string, so a case whose program
arrives on standard input writes it once as its `Snippet` and the
rendered table shows what actually ran.

## The harness

`internal/oracle` implements this, and `cmd/oracle` drives it:

    make oracle         # re-measure, rewrite measurements.md and the golden record
    make oracle-check   # fail if the panel no longer behaves as recorded

The corpus is checked-in Go data, one entry per behavior a spec entry
asserts. Each carries a `Why` explaining what it pins down — without
that, a case that changes later gets "fixed" by updating the golden
record, which is how a regression becomes a feature.

`make oracle-check` runs in `make check` and in CI. Drift is deliberately
**not** described as a failure of the code: a shell was upgraded, or a
case was edited, and the recorded behavior is no longer what the panel
does. The response is to work out which, update the affected spec
entries, and re-record.

The run environment is fixed — an empty `PATH` of system directories,
`HOME` pointed at a scratch directory, `LC_ALL=C`, and no standard input
unless the case supplies its own — because a record that depends on
whose machine produced it is not evidence. Shell
and script paths are normalized out of diagnostics for the same reason.

A shell that is absent is reported, not fatal, and its column is omitted
rather than blanked: a table from three shells is a weaker claim than the
same table from five, and the generated file says which it was.

## Third-party suites

bash's own `tests/` directory is a useful denominator and is **GPLv3**.
It may be fetched at test time; it must never be committed. Vendoring it
would relicense this repository by accident.

No other project's test corpus is used at all.
