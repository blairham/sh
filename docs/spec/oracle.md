# The oracle method

Behaviour is learned by **running real shell binaries and recording what
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

## Third-party suites

bash's own `tests/` directory is a useful denominator and is **GPLv3**.
It may be fetched at test time; it must never be committed. Vendoring it
would relicense this repository by accident.

No other project's test corpus is used at all.
