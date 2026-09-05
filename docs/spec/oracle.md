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

That name is applied on **every route** — `-c`, a script file, and a
case's own argv alike — and the rule is written down because it was once
broken on one of them. The script route did not apply it, so the column
headed `bash-as-sh` held plain bash for every case that runs from a file,
and 18 rows said the wrong shell's answer under the right shell's
heading. Nothing about such a row looks wrong: they agree with each other
and disagree only with the column they sit in, which is the mislabeled
column `MustReport` exists to prevent, arriving by the one door
`MustReport` cannot watch — it checks that the *binary* is the one the
entry names, and a binary invoked under the wrong name is still that
binary.

What it cost is the measure of why the rule is worth stating. Called
`sh`, the same bash makes a failed special builtin fatal, so a readonly
reassignment stops the script rather than printing what came after it;
expands aliases in a non-interactive script, so a command that was "not
found" is found; prints `INT` where bash prints `SIGINT`; and holds
`trap` to its POSIX usage. Thirteen of the eighteen rows had the wrong
exit status, which is the part a reader would have trusted. **A column is
named by argv[0] and by nothing else** — so a route that cannot say what
argv[0] it used cannot say which shell it measured.

## Producing a fact

A case is a snippet plus, for each shell in the panel, what it wrote to
standard output, what it wrote to standard error, how it ended, and
whether it ended at all. All of it is recorded; a construct that prints
the same thing with a different status is still a difference.

"How it ended" is a status *or* a signal, never both. A process a signal
killed has no exit status — the two are alternatives in one wait status —
so an exit status of -1 means only "there was none", and without the
signal beside it a shell killed by SIGINT, a shell killed by SIGKILL and
a shell that hung until the harness gave up were all one row. Recording
the signal is what makes a shell's exit-on-signal discipline observable:
the convention that a shell with no trap for a fatal signal re-raises it
at itself, so its caller is told which signal killed it rather than a
number the shell computed. Nesting a `sh -c 'kill …'` child and reading
`$?` measures the *parent's* arithmetic instead, and every case that
wanted to see a signal had to do that.

Rules that keep the facts honest:

- **The two streams are facts about each other.** A diagnostic is part
  of the behavior, so which stream carried it is measured rather than
  merged away. `type` on a name that is nothing prints to standard
  output in dash and zsh; a harness that captured one merged stream
  could not see a shell that printed the same words to the other one,
  and could not see an implementation that got it backwards.
- **Order across the two streams is not a fact.** They are captured on
  separate pipes, so a row says what each stream carried and not how the
  two interleaved. That was never worth recording: a shell block-buffers
  standard output into a pipe and writes standard error unbuffered, so a
  merged capture recorded the order the buffers flushed rather than the
  order the shell wrote. A case that means to pin ordering across the
  streams says so the only reliable way, by redirecting one of them
  where the snippet itself can see it.

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
`oracle.ArgSnippet` is replaced by the snippet, and `oracle.ArgScript` by
the path of a file holding it — the same file `Script` writes, normalized
to `<script>` in the output. At most one may appear. A case with neither
never hands the shell its snippet, which is how an invocation that must
fail before it reads anything is written.

A placeholder is replaced **wherever it appears**, not only as a whole
word — the same rule `Stdin` has always followed. `-c` takes its command
string as a separate word only when it is written that way, and
`sh -c'echo hi'` attaches it to the letter, so an invocation that needs
the snippet inside a larger word is spelled `"-c" + ArgSnippet`.

Requiring a whole word meant such a case had to write the text twice,
once as its `Snippet` and once literally in its argv, with nothing
checking that the two stayed in step: an edit to either made the case
test something other than what it recorded, silently. Interpolating
removes the second copy rather than guarding it, and the corpus test now
rejects an argv that spells the snippet out, since there is no longer a
reason to.

Guarding it was the cheaper-sounding option and would have been wrong.
Of the sixteen cases whose `Args` name no placeholder, eleven hand the
program over through `Stdin` and four withhold it deliberately — a script
path that does not exist, `-c` with nothing after it. A rule that a
no-placeholder argv must contain the snippet verbatim would have fired on
fifteen of the sixteen, and suppressing it would have taken a new field
to say which cases mean it.

This exists because the invocation surface was graded by nothing else.
The corpus started every case the same way, so the front end was pinned
only by its own unit tests — the shape of the incident where the drivers
scored 178/198 against a core scoring 198/198.

## Cases graded on the refusal rather than on its wording

The shapes most worth pinning about an invocation are the ones a shell
**refuses**: a command string written against its letter, an option that
is not one, `-c` with nothing after it. Every shell refuses in words of
its own, and those words are deliberately different — they are what the
Diagnostics vector exists for, and a dialect is meant to sound like the
shell it names. So an exact comparison scores four shells that agree
about the behavior as four disagreements, and the sharpest of these
cases could not be written down at all.

`Case.GradedOnRefusal` grades such a case on the fact that the shell
declined. Capturing the two streams apart is what makes that a precise
claim rather than an approximation — "it complained, on standard error,
and did not carry on" is three recorded fields and needs no normalizer:

| still compared exactly | forgiven |
| --- | --- |
| the outcome — status, signal, timeout | the text of the diagnostic |
| standard output, byte for byte | |
| that **both sides** refused: nonzero **and** a diagnostic on stderr | |

The last row is the safeguard, and it is why this is not simply
"compare less". The mode adds two requirements the exact comparison
never makes, so it is *stricter* in the direction that matters. Put the
flag on a case the reference does not refuse and the case **fails** — a
misused flag has to be louder than a correct one, not quieter. A shell
that refuses silently fails too, because saying nothing is not a wording
difference. And a timeout is never a refusal: a shell that never
finished declined nothing, and a case that hangs has stopped measuring.

Keeping standard output exact costs something, and the cost is the point.
bash answers `sh -c'echo hi'` by writing its whole `set -o` table to
standard *output* as part of the usage, so the case that pins that shape
reports a real gap against a bash reference until we write the table
too. Forgiving the stream instead would forgive a shell that **ran** the
command string, which is the exact divergence the case was written to
catch. Measured on the four cases as they stand: two pass on the
wording, and two fail on a genuine difference in what the shell did.

**Only the grading is relaxed; the record is not.** `measurements.md`
prints what each shell actually said, and the drift check still compares
every byte, so a shell that changes its wording is still caught. The
rendered tables mark such a row **(refusal)**, in the table and beside
the case's reason, and a conformance report says how many of its passes
needed the relaxation and which they were. That visibility is the
condition the mode is allowed on: a relaxation nobody can enumerate is
indistinguishable from a score that is quietly wrong.

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

### Signal dispositions are part of the run environment

The environment is not the only thing a shell inherits. A signal
disposition of *ignored* survives `exec` — that is the whole of what
`nohup` does — so a harness launched with SIGHUP ignored hands every
shell it measures a SIGHUP that is already ignored. bash reports
`trap -- '' SIGHUP`, and `kill -HUP $$` leaves the shell alive to print
what came after it. Five of the six panel columns change.

That made the headline instrument a function of how the harness was
launched. Measured against the same binary and the same corpus, a
conformance run scored **1110/1116** from a terminal and **1101/1116**
under `nohup`, and `oracle-check` reported drift it does not report in a
terminal. A terminal, a CI runner, `nohup`, a supervisor and a background
shell can each hand the harness a different starting disposition, and two
runs that disagree read as a flaky implementation rather than as a
different question.

So the harness resets them, beside the environment scrub. It cannot be
done to the child — a disposition belongs to the process that forks, and
there is nothing to run between fork and exec — so it is done to the
harness: taking a signal over replaces the ignored disposition with a
handler, and `exec` resets a *handled* signal to its default in the
child. The harness goes on ignoring what its caller asked it to ignore;
only the shells it measures stop.

Four signals are a **named limit** rather than a fix, and the limit is
written here so it cannot quietly stop being true. The Go runtime keeps
an inherited ignore for `TSTP`, `TTIN`, `TTOU` and `CONT` — stopping a
process that was started with stopping turned off would be wrong — and
does not report that it has, so the harness cannot detect the condition.
Across every signal a shell can have an opinion about, those four are the
only ones where the runtime's report and the child disagree.

Taking them over regardless would cost more than it buys, and the cost
cannot be undone: a Go program stops on a `SIGTSTP` raised at itself, and
the same program after a single `signal.Notify` does not, with neither
`Stop` nor `Reset` giving the stop back. So a blind takeover permanently
costs the harness its Ctrl-Z. Asking a shell instead is not portable —
POSIX says a signal ignored on entry cannot be trapped, and only bash
honors it; dash and zsh install the trap anyway. The four are listed with
the rest, so nothing more is needed the day the runtime starts reporting
them, and a test asserts the leak still exists so that day is noticed.
What is left is a hole no launcher opens: `nohup`, CI runners,
supervisors and a non-interactive shell's background job ignore `HUP`,
`INT` or `QUIT`, all of which are covered.

Two other launch-dependent inputs are worth naming as *not* covered. A
signal the caller **blocked** rather than ignored is inherited too, and
the standard library offers no way to clear the mask for a child;
nothing that launches this harness in practice blocks signals, so it is
a known edge rather than a solved one. And `make` is immune to the whole
class by accident: it resets dispositions for its own recipes, so
`nohup make conformance` does not reproduce any of this while
`nohup go run ./cmd/oracle` does. A guard that only works through one of
two entry points is not a guard, which is why the fix is in the harness
and not in the Makefile.

### The corpus may grow and may not shrink

    make corpus-guard   # fail if a case that existed at the merge base is gone

The corpus is a set, and it is written down as a Go slice literal that git
merges as text. Those two facts disagree: a merge or a rebase can resolve
`case.go` by taking one side and drop cases without a conflict, without a
failing test, and without anything in the diff to see — a reviewer reads
what changed, not what silently left. One `gh pr update-branch` took it
from 1088 IDs to 1084, and only a hand count noticed.

What that costs is why it is a check and not a note. A dropped case is lost
coverage that reads as a passing build: the conformance total moves by a
few, and the campaign moves it by a few every day.

The guard compares the case IDs in the tree against the case IDs at the
merge base with `main`. It compares **sets**, because a count is defeated by
the shape it is meant to catch — add two cases while merging two away and
the total is unchanged; add five while losing four and it rises. Its
baseline is **history**, because a committed list of IDs is another file in
the same tree, merged by the same merge, and a baseline the guarded event
can rewrite is not one. Uniqueness is checked in the same place, since a
repeated ID is one case's evidence written over another's in the golden
record.

Retiring a case on purpose means naming it in `corpusguard.Retired` with
the reason. It should be rare: the ID is the key in the golden record, so
dropping one throws away what five real shells were measured doing, and
renaming a case is a retirement plus an addition. No ID has ever left
`main`.

A shell that is absent is reported, not fatal, and its column is omitted
rather than blanked: a table from three shells is a weaker claim than the
same table from five, and the generated file says which it was.

### `LC_ALL=C` is a choice, and it costs something

Pinning the locale is what makes the record reproducible, and it is also
a **limitation of the record**, not a fact about the shells. Every row in
`measurements.md` is what that shell does *in the C locale*. Two effects
are known and measured, and both are cases where a UTF-8 locale gives a
different answer:

- **Collation.** `echo *` over `Apple banana Cherry _under 1digit` is
  byte order in all four shells under `LC_ALL=C`
  (`1digit Apple Cherry _under banana`). Under `en_US.UTF-8`, bash,
  ksh93 and zsh collate (`_under 1digit Apple banana Cherry`) and dash
  keeps byte order. So the panel is unanimous only because the locale
  is pinned, and the disagreement the record cannot show is real. Pinned
  deliberately by `glob/matches-are-in-order`, whose `Why` says so.
- **What counts as a printable character.** bash's `${x@Q}` single-quotes
  `café` in a UTF-8 locale and writes `$'caf\303\251'` under `LC_ALL=C`,
  because no byte of it is character-shaped to a shell reading one byte
  at a time. This implementation has no locale and always takes the UTF-8
  reading, which is why no corpus row pins a multibyte `@Q` — the row
  would grade it against an environment it does not model
  (`grammar/parameter-expansion.md`).

The rule that follows: **a case whose answer is a property of the locale
does not belong in the corpus.** Record the locale dependence in the spec
entry instead, and pin the C-locale answer only where it is the answer
worth having. Both rows above do the second thing on purpose, and say so
in their `Why`.

Two other classes of answer are excluded for the same reason — because
they are not properties of the shell. A probe must not record the clock
or the machine: `${x@P}` on `\t` is the time of day and on `\w` the
working directory, so the row that pins prompt expansion probes `\n` and
`\\` only.

## Third-party suites

bash's own `tests/` directory is a useful denominator and is **GPLv3**.
It may be fetched at test time; it must never be committed. Vendoring it
would relicense this repository by accident.

No other project's test corpus is used at all.
