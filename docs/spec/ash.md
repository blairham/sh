# BusyBox ash

The fifth dialect, and the only one in this tree whose behavior **no
instrument grades**. Read this file before trusting anything in
`dialect/ash`.

It exists for two reasons, and the second is the one that decided the
shell. The first is coverage: in the container world `/bin/sh` *is*
BusyBox ash — Alpine, most embedded images, most `FROM scratch`-adjacent
bases — so it is where a gated, sandboxed shell actually gets deployed.
The second is that the repository's central claim had never been tested.
`AGENTS.md` says **"adding fish adds a directory rather than editing the
substrate"**, and every dialect in the tree was written before that
sentence was. A fifth one is the experiment.

## The verdict on the substrate claim

**It held.** Nothing under `syntax/`, `interp/` or `driver/` changed.
`dialect/ash/` and `cmd/ash/` are the whole of the shell; the rest of the
diff is the lists a new binary has to appear in — `Makefile`'s `SHELLS`,
`cmd/sh`'s `-dialect`, `cmd/shfmt`'s `-dialect`, `.goreleaser.yaml`, and
two counts in prose.

That is not a claim that every measured behavior fits. Three do not, and
they are listed under *What could not be said* below. Three more used to
be there and have gone, each a different way: #2276 widened an axis so a
reading that had no value could have one, #2277 moved a refusal from an
axis to the option that raises it, and #2278 — `ulimit -a` — turned out
not to need the second platform it was waiting for, because what a shell
does about a limit its kernel lacks is a rule the shells that *do* run on
both platforms could be measured for. It is a claim about
the shape of the work: every answer this shell needed was a **value on an
existing axis**, and no axis anywhere acquired a branch on a shell's name.

## The prediction that was refuted

The issue this dialect came from predicted that ash — dash's sibling,
both NetBSD ash descendants — would "side with dash on most non-POSIX
omissions", and warned that the day it joined the panel every "dash is
the sole holdout, so the construct is core" argument would stop being a
sole-dissenter argument.

Measured, ash goes the other way on every question that was checked:

| question | dash | ash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `[^x]` negates | **no** | **yes** | yes | yes | yes |
| `echo 'a\tb'` interprets without `-e` | yes | **no** | no | no | yes |
| `echo -e` is a flag | **no** | **yes** | yes | yes | yes |
| valueless `local x` over an outer `x` | `outer` | **unset** | unset | no `local` | empty |
| `$((2**3))` | error | **8** | 8 | 8 | 8 |
| `$((10#08))` | error | **8** | 8 | 8 | 8 |

So the sentence in `interp/semantics.go` about `BracketCaretNegates` —
"dash alone treats `^` as an ordinary character" — survives ash rather
than losing to it, and `docs/spec/core.md`'s boundary is not
re-litigated. The ash column **widens** the non-dash agreement rather
than narrowing it, re-measured 2026-09-18 on BusyBox v1.37.0 through the
digest-pinned image: `case z in [^abc])` matches there as it does in the
five other non-dash columns.

That is the half that could have changed #489's answer and did not — a
sixth agreeing column would have become a grouping if it had gone the
other way. It stays an axis regardless, for a reason the column count
does not reach: the caret is a disagreement about what a pattern
**means** rather than about whether it parses, and core.md's holdout rule
governs membership. `semantics.md` has that argument in full.

The grammar goes the same way. ash takes nine constructs dash refuses:

    $'…'      [[ ]]      <(…)      function f { }      function f() { }
    ${v:1:2}  ${v/a/b}   $((2**3)) $((10#08))

and, beyond the table, `++`, `,` in arithmetic, `&>`, `>|` and the `time`
keyword. It refuses arrays, subscripts, the C-style `for`, `select`,
`<<<`, `(( ))`, `$[…]`, floating-point arithmetic, `${v^^}`, `${!x}`,
`${v@Q}`, `|&`, `;;&`, `$(<f)`, `{fd}>f`, `@(…)`, brace expansion and
`+=`.

## How it was measured

There is **no ash binary on the panel machine and no easy way to get
one**. `brew install busybox` answers `No available formula`; BusyBox is
Linux-centric C that does not build cleanly on macOS; its published
static binaries are Linux ELF. macOS's `/bin/sh` is bash 3.2 in `sh`
mode, which is already the `bash-as-sh` column.

So it was measured in a container, **ad hoc and by hand**, on
2026-09-12, against BusyBox **v1.37.0** — the `/bin/ash` of the
`alpine:3` image, `linux/arm64`, under colima.

Two passes:

1. **The whole corpus.** A throwaway program linked against
   `internal/oracle` was cross-compiled for `linux/arm64`, mounted into
   the container and run over every case in `oracle.Corpus` through
   `oracle.Exec` — the same invocation, the same scrubbed environment and
   the same normalization the golden record's own columns get. 3383 cases
   recorded. The results were then diffed against the golden record's
   `dash` column, which is what produced the divergence list the dialect
   was written from.
2. **Hand probes.** Roughly 180 snippets aimed at the questions the
   corpus does not ask of a shell it has never seen: which location shape
   each route uses, which `set` letters exist, which builtins are absent,
   what `[[` actually is, and the wording of every diagnostic family.

No BusyBox source was read, and none may be: it is GPLv2 and
`CLEANROOM.md`'s red list names it. Everything above is the observed
behavior of a binary, which is a fact rather than anyone's expression.

Every number here is reproducible with:

    docker run --rm alpine:3 /bin/ash -c '<snippet>'

## What checks it

The oracle reaches a panel member by a **route** rather than by a path
(`oracle.Reach`). Every other member's route is a binary on this machine,
found with `exec.LookPath`; ash's is a container of the `alpine` image
**pinned by digest** —
`sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b`,
which is BusyBox v1.37.0. So:

- **`internal/oracle/testdata/golden.json` has an `ash` column**, and
  `docs/spec/measurements.md` prints its build string beside the others'.
- **`make conformance-dialects` grades `cmd/ash`** against it and reports
  a number.
- **`make axis-sweep` has an ash target**, which it could not have while
  the column did not exist: a sweep graded against an absent column
  reports every axis as unpinned, which is the loudest way of saying
  nothing.
- **`make oracle` regenerates the column on a developer's machine**,
  which is the property that decided the shape. The alternative — record
  it in CI only — would have left the committed record un-regenerable
  here, and this repository already has the scar that says a gate inert
  on one platform reads as a pass.

Two costs came with that and are not hidden. The column pins the
behavior of an **image**, not of a binary on this machine, so its
provenance is a digest rather than a package version; and **regenerating
it needs a container runtime**. Where there is none, the column does not
run, and the harness says so in the words it says nothing else in:

    ash          NOT RUN — no container runtime here (docker is not on PATH)

`oracle -check` will not print `no drift: the panel behaves as recorded`
over a panel missing a column the record has. It names what did not run
instead — which also fixed the same dishonesty for `bash32`, absent on
every Linux machine since long before ash existed.

The run is one long-lived container spoken to over a pipe, not one
invocation per case, and the thing inside it is **`oracle.Exec` itself**,
cross-compiled from this tree. Not a `docker run` command line built to
resemble what `Exec` does: the same function, so the environment scrub,
the signal-disposition scrub, the timeout, the wait-status reading and
every normalization rule are one implementation for both routes. Measured
on the panel machine, the column costs about **four seconds** on a `make
oracle` that already takes four minutes.

### The first time that cost something

Not theoretical any more. `Semantics.ReadTimeoutBoundsReadability` landed
(#2252, for #644) within an hour of `dialect/ash` itself, from another
session. Neither change was wrong; the axis simply had no ash value, and
**an unanswered axis refuses at run time** — so the shipped `ash` binary
started rejecting `read -t 1 x`, at status 2, with `go test ./...` green.
Nothing in `make check` could have seen it. That is #2272, and it is the
first concrete evidence that an ungraded dialect is a live risk rather
than a tidiness complaint.

The fix was not one value. Running every `oracle.Corpus` snippet through
the `ash` binary and collecting each `no dialect was chosen` found
**nineteen** unanswered axes reachable in this shell, not one — and
answering them uncovered a twentieth, because a refusal early on a path
hides the next question along it. Sixteen were answered in
`dialect/ash/ash.go`, each against a BusyBox probe, and the remaining
three were left refusing with the measurement written down beside them.
All three have since closed, and none of them closed by picking one of
the answers: #2276 widened an axis, #2277 moved the refusal to the option
that raises it, and #2278 found that the fact it was missing was a
platform's and not a shell's.

That sweep is worth re-running whenever an axis is added, and it is three
lines over `oracle.Corpus` and a built binary. It does not need a
container: the *refusals* are our own, and only the answers need BusyBox.

### What now catches it, and what still does not

`make axis-coverage` (#2340) asks every dialect, of every axis, whether it
has an answer — ash included, and with no shell run at all. It is a test,
so it is in `make check`, and it fails on the commit that adds an axis
rather than in somebody's terminal a week later. `internal/axissweep/`
`testdata/unanswered.txt` records what is unanswered today, because most
of it should be: 113 of the 429 askable axes have no ash value and many
of them are questions BusyBox is never asked.

Two things it deliberately does not do, and both are why the empirical
sweep above is still worth running:

- **It cannot say whether the dialect *reaches* an unanswered axis.** The
  113 include the ones that matter and the ones that never will, and only
  running the corpus through the binary separates them. That run found
  nineteen; a static count cannot.
- **It cannot say whether an answer is right.** A value copied from dash
  to quiet a refusal satisfies it perfectly. That is what an oracle
  column is for, and there is one now (#2263) — `make
  conformance-dialects` is where a wrong answer shows as a number that
  did not move, or moved the wrong way.

  And a number that did not move is a poor signal, which is why `make
  axis-grade` exists (#2441). It reads an axis's value off the *recorded
  cells* of the ash column and compares it with what this preset holds, so
  a wrong answer is named rather than folded into a score — with no shell
  run, since the record is already on disk. Two of this dialect's answers
  have been wrong that way and both were caught by reading the record:
  `QuitIgnoredWhenNotInteractive`, which held dash's value for the whole
  life of the column, and `ShiftPastEndFatal`, which was inherited from
  `PosixSemantics` and never overridden. Note what the two have in common:
  **the failure mode of a preset that starts from POSIX is a value nobody
  ever chose**, and an inherited value is indistinguishable from a measured
  one in the source.

What it does do is make the #2272 shape impossible to ship quietly, which
was the specific failure: an axis added elsewhere, unanswered here,
refusing at run time with the test suite green.

An axis this dialect genuinely cannot answer is recorded where the
omission is, as a line in `ash.go`:

    // unanswered ReadonlyRecordsTheCompoundAttribute: this shell has no
    // letter to ask it with … (#2277)

which the coverage report prints under the entry it answers. So the check
states what is unmeasured rather than being switched off — and, the other
way round, a value quietly appearing for one of the items under
*What could not be said* now **fails** the check while its note still
stands, which is exactly the "copied from a neighboring dialect to make
the message go away" move this file forbids.

### Where it stood when it landed

Graded against the container recording — our `cmd/ash` run over the same
3382 graded cases and compared with `oracle.Verdict`, the same rule
`make conformance` uses — this dialect scored **2671/3382, 79.0%** on the
day it landed. `cmd/dash` scored 97.3% against its own column on the same
run, after a long campaign; 79% is what a first pass looks like.

On the day the column landed, `make conformance-dialects` scored it
**2731/3450, 79%** — 96% behavioral. That number is re-derivable now,
which is the difference this made: it used to be written down here
*because* it could not be.

### The column, shown red and then green on #2272

Not a demonstration arranged afterwards. #2272 was **live in the tree**
while the column was being built, so the first run of the new
`conformance-dialects` row named it — `read/a-timeout-that-expires` and
`read/a-fractional-timeout`, both of which BusyBox answers and `cmd/ash`
was refusing:

    want out "st=0 [hi]" err "" (status 0)
    got  out "st=2 []"   err "`read -t` bounding the wait for the first
                              byte rather than the whole read: the shells
                              disagree here and no dialect was chosen"

`want` is real BusyBox through the container route; `got` is our shipped
binary. Then #2272 landed on `main` — `ReadTimeoutBoundsReadability`
gained its ash value — the branch was rebased onto it, and **both rows
pass**. Red with the answer missing, green with the answer present, on
the same instrument and with nothing else changed.

That is the whole argument for the column in one pair of runs. The defect
was an axis added elsewhere with no ash value; the shipped binary stopped
taking a flag it used to take; and `go test ./...` was green throughout,
because nothing graded ash.

## Borrowed text is named in front of the location

A failure *while* text this shell borrowed is running names the text, and
the name goes between the shell's own and the line:

    ash: ./p.sh: line 2: NOPE: parameter not set
    ./s.sh: eval: line 11: NOPE: parameter not set

That is the **fourth** arrangement of three fields the panel already
shares, and the four are worth reading together, because the shape is one
question with four answers rather than four mechanisms:

| shell | a run-time failure inside a sourced file |
| --- | --- |
| bash, zsh | the file's path where the shell's own name goes |
| dash | the name **after** the location — `dash: 2: ./p.sh:` |
| ksh93 | the whole chain of borrowed texts — `./n.sh[2]: .[2]: .:` |
| BusyBox ash | one name **before** the location — `ash: ./p.sh: line 2:` |

`Diagnostics.BorrowedTextIsNamedAtRunTime` turns the name on and
`SourceFileNaming`/`EvalNaming` say where it goes; nothing new was needed
beyond a second placement (#2520).

The **line** beside it did need a field. This shell's own text carries a
line only from a script file — `Location` is bare and `ScriptLocation` says
`line N` — while text it borrowed carries one on **every** route,
`-c` and standard input included, and for a parse failure as much as for a
run-time one. `Diagnostics.BorrowedLocation` is that second answer. Reading
it off `ScriptLocation` would have given the same string here and would
have been an inference no measurement supports.

Two rows keep the rule from being too wide, and both are in the corpus. A
function frame standing above the borrowed text does not end it — a `.`
inside a function still names the file — while the source **returning**
does: a function defined in a sourced file and called afterwards is
`./s.sh: line 1:` with no name at all. So the rule is over the innermost
borrowed text still being read, which is dash's rule with this shell's
placement.

## The two hexadecimal-escape axes, and the probe that cannot decide them

`$'\x…'` splits the panel twice and this shell answers bash's way both
times. Measured 2026-09-12 in the pinned image:

    printf '[%s]' $'\x414'   [A4]      two digits, and the rest is text
    printf '[%s]' $'\xzz'    [\xzz]    a digitless escape is kept as written
    printf '[%s]' $'\x'      [\x]
    printf '[%s]' $'\uZ'     [\uZ]

where ksh93 takes every digit and reads a code point (`$'\x414'` is
U+0414) and zsh reads a zero byte from a digitless escape.

**The obvious probe cannot decide the first one here**, and that is worth
keeping written down because it is specific to this column: `$'a\x00b'`
is length 2 under *both* readings, since a three-digit run read short
gives a NUL — which this shell **drops** — and read long gives U+000B,
one byte either way. Only a run whose long reading is a *different*
character separates them, which is what `\x414` is for.

Both were left unanswered while there was no binary to ask. The three
corpus rows that reach them already carry an ash column, so the values
are graded by rows that existed before them (#554).

## The operand a short circuit already decided

This shell evaluates the right operand of `&&` and `||` inside `$(( ))`
even when the left one has settled the answer, so an assignment or an
increment written there takes effect. It is the sole holdout in a panel of
seven. Measured 2026-09-13 in the pinned image:

    x=0; : $((0 && (x = 9))); echo "$x"        9        elsewhere 0
    y=0; : $((1 || (y = 8))); echo "$y"        8        elsewhere 0
    x=0; : $((0 && (x++)));   echo "$x"        1        elsewhere 0
    echo "$((0 && (x = 9)))"                   0        the same everywhere
    echo "$((7 || (x = 9)))"                   1        the same everywhere
    echo "$((0 && (1/0)))"                     divide by zero, status 2

The last two lines are what make this one question rather than two. The
value is the operator's under either reading — `0 &&` anything is 0 —
so nothing about the number could ever have found this; the division is
what says the operand is *evaluated* and not merely scanned for stores.

The conditional does **not** do it: `w=5; $((0 ? (w = 1) : 2))` leaves w
at 5, and an `&&` written inside an arm the conditional dropped never
runs either. So the answer is about the two logical operators and not
about a shell that evaluates everything it parses, which is the reading
the `&&` line alone would have supported.

`interp.Semantics.ArithShortCircuitEvaluatesTheRightOperand` is the axis;
`docs/spec/semantics.md` has the seven columns and the bash 3.2 reading
that axis deliberately cannot hold.

### How it was found

By the first run of this dialect's column in `make suite` (#2605, #2608),
on the day the column could run at all. `share/suite/core/arith.tests`
scored 9/10 against BusyBox with both shells at status 0 — a silent wrong
answer, which is the class a pass/fail count is worst at surfacing and a
whole-file diff is best at.

The corpus had the case all along: `arith/short-circuit-is-observable`
records `[0][9]` in this column against `[0][0]` in the other six, and has
since the column existed. What it did not have was a gate — the dialect
conformance number is report-only by design — or a `Why` that said a
column moved. Both are fixed here, and the pairing is the lesson: **the
corpus records, the suite reports.** A divergence can be measured, written
to disk and shipped without anything saying it out loud.

## The refusal that is harsher for a letter than for a name

`set -o nosuchname` reports **1** here and the script carries on;
`set -Z` writes its complaint and ends the script at **2**. No other panel
column does that, and two of the substrate's fields said it could not
happen: `Semantics.BadSetOptionNameFatal` and
`Diagnostics.SetInvalidOptionStatus` each answered both spellings with one
value, because #483 measured them across the six columns that existed
before this one and all six agree.

So this is not a wrong measurement being corrected. It is a measurement
taken when the panel was smaller, and a seventh column splitting a question
that genuinely looked like one — which is the second time this dialect has
done that, the first being the short circuit above. #2629 split both fields
in two.

The value that shipped was `Yes` for both, written among the assignments
recording that a special builtin's failure is fatal here and with no
comment of its own. That is the #2272 shape again in a quieter form: not an
axis with *no* ash answer, but an ash answer inherited from an older
measurement of a smaller panel. It cost three corpus rows, and only one of
them was about `set` — the other two reach it because `set -o posix` is a
name BusyBox does not have, so a test of where a `${ }` ends was ending the
script instead.

**What now catches it** is a probe rather than a number.
`internal/axissweep`'s grade reads both axes off the recorded cells of
`opt/an-unknown-long-name-is-refused` and `opt/an-unknown-letter-is-refused`
and compares them with what each preset holds, so a future value copied
from a neighboring dialect disagrees with BusyBox's own row in `make
check`, with no shell run. Reading a row a shell *owns* would not do it:
`opt/set-o-takes-a-name-only-this-shell-has` uses `autocd`, which zsh has,
so zsh's cell is a silent 0 that says nothing about a refusal — the probes
use a name and a letter no panel member owns, and refuse to read a cell
with an empty standard error at all.

Two things measured alongside it are deliberately **not** folded in: this
shell's `set: line 0:` prefix, which is item 1 under *What could not be
said* below and accounts for a large share of this column's wording-only
mismatches, and a refused `-o` **name at an invocation**, which writes the
complaint, declines to run the command string, and exits **0** (#2639).

## The `set -o` table is this shell's own, and it was dash's

`dialect/ash` held dash's fourteen names, sorted. Three things were wrong
at once, and a count catches none of them, because dash's table has
fourteen names too. Measured 2026-09-17 in the digest-pinned image,
BusyBox v1.37.0, against `cmd/ash` in the same container (#3366):

| probe | BusyBox 1.37.0 | ours, before |
| --- | --- | --- |
| `set -o errtrace` | 0 | `illegal option -o errtrace`, 1 |
| `set -E` | 0 | `illegal option -E`, 2 |
| `set -T` | `illegal option -T`, 2 | the same |
| `set -b` / `set -I` | 0 | `illegal option`, 2 |
| `set -o emacs` | `illegal option -o emacs`, 1 | accepted, 0 |
| `set -o nolog` | `illegal option -o nolog`, 1 | accepted, 0 |
| `set -o pipefail` | 0, **and listed after** | 0, **and not listed** |
| the listing's order | this shell's own table | dash's names, sorted |

BusyBox's listing, in its order:

    errexit noglob ignoreeof monitor noexec xtrace verbose noclobber
    allexport notify nounset errtrace vi pipefail

`pipefail` is the one that costs a script something silently: the option
was taken and then not written, so `set -o pipefail; set +o` did not say
the shell was in it and a script saving state with that output lost it.

Two substrate changes fall out, and both are the same shape as the one
above — a question that looked like one and is two once a seventh column
is asked.

`emacs` and `nolog` left the substrate's *unanimous* table, since this
shell has neither, and are declared by the four dialects that do have
them. "Every shell in the panel" is a measurement, not a definition.

`Semantics.SetHasTraceLetters` became `SetHasTheErrtraceLetter` and
`SetHasTheFunctraceLetter`, because this shell takes `-E` and refuses
`-T`. One answer could only have given it both letters or neither. There
is no ERR trap here for the carriage to be about — what the option moves
is nothing, and what a script can see is the letter, the name and the
listing row.

`-b` and `-I` are declared through `Runner.SetOptionLetterNames`, which
is what that table is for: no other column in the panel spells `notify`
or `ignoreeof` with a letter.

## The letters of `$-` are in this shell's order too

Same family, same session, and a different kind of wrong answer: status
0, the right set of letters, the wrong string. `ash -i -c 'echo "$-"'`
is `ci` there and was `ic` here, which passes `case $- in *i*)` and fails
every comparison against a saved string (#3256).

The order is dash's *discipline* — the reverse of this shell's own
`set -o` table, with the invocation letters inserted where that table has
no row — and not dash's string, since the two tables differ. Measured a
letter at a time and then all at once: `set -EubaCvxI` under `-c` is
`EubaCvxcI`, `-i` puts `i` between `c` and `I`, and a terminal with `-m`
puts `m` there too. The `-s` route puts its letter where `c` stands on
the other one. `n` is the one letter nothing can observe, here as in
dash, because `set -n` stops the `echo` that would read `$-`.

## What could not be said

Three measured behaviors have no value on any existing axis. They are
recorded here rather than approximated in code, because an invented
answer is indistinguishable from a measured one in a file that holds both.

They were deliberately left undone while nothing graded this dialect: a
substrate edit made for it could not be checked. **That reason has
expired** — the column exists, `make conformance-dialects` reports a
number, and a widening can now be shown to move it. What remains is the
size of the edit rather than the impossibility of verifying it, so each
is a change to make and to measure rather than a note to keep.

**1. The builtin location prefix.** A message a *builtin* speaks carries
the builtin's name and a line, separated the ordinary way:

    ash: unset: line 0: a[0]: bad variable name
    s.sh: export: line 2: :: bad variable name

`Diagnostics.NamesBuiltinInLocation` is the axis for "the builtin's name
rides on the shell's name", and it is a `bool` whose one true value joins
them *tight* — zsh's `zsh:shift:1:`. ash wants the same thing spaced.
Closing it means widening that bool into a three-valued enum
(absent / tight / spaced), which is 31 references across 14 files. It is
the one of them that costs real corpus rows, and it is now
measurable: the ash column is what the widening would be graded against.
It is deliberately not part of #2263 — a 14-file substrate edit folded
into the change that builds the instrument would be graded by the same
commit that invented the grade.

**2. `line 0` under `-c`.** On the command-string route this shell
numbers a builtin's line from zero — `ash -c 'unset "a[0]"'` is `line 0`
and a second line is `line 1` — while a script file and standard input
both number from one. No axis carries a per-route line origin.

It reaches further than a builtin's own message. `eval`'s text continues
the caller's lines here, so the origin is added to every line inside it:
`ash -c 'eval nosuchcmd'` is `eval: line 0` where the same line in a script
is `line 1`. That is why the corpus row for the naming above
(`eval/the-borrowed-text-in-the-prefix`) runs from a **script** — over
`-c` the digit would differ from every other column for a reason that has
nothing to do with naming, exactly as it would have in
`eval/where-the-texts-lines-are` (#2462).

**3. `divide by zero`.** The reason word for a division by zero is the
substrate's own string (`division by zero`); this shell writes `divide by
zero`. There is no `Diagnostics` field for it, and adding one for a
single word was not worth a substrate edit.

A seventh is a difference in kind rather than in wording: **`[[ ]]` here is
a builtin, not a keyword.** `type '[['` answers `[[ is a shell builtin`,
and it does not suppress word splitting — `v="a b"; [[ $v == "a b" ]]` is
`b: unknown operand`. It is `test` with a closing `]]`, reading `&&` and
`||` as its connectives (and `-a` and `-o` as nothing), matching `=`, `==`
and `!=` against a pattern however the pattern was quoted, and taking `=~`.
The parser has one thing to know: after an unquoted `[[` word of a simple
command, `&&` and `||` are words until an unquoted `]]` word, which is why
`echo [[ a && b ]]` prints all five. Everything else — `<` and `>` as
redirections, `(` as a syntax error after a word, a newline ending the
command — is what a simple command already does. The dialect used to set
`DoubleBracket` and accept the keyword kind, and it was the largest single
family of corpus disagreements (28 rows); it sets
`DoubleBracketIsACommand` since #3409, and `share/suite/ash/dblbracket.tests`
holds the rows.

## `test` reports as the applet, and parses its two-word form

Every complaint `test`, `[` and `[[` make here opens with the shell's own
name and nothing else, where every other complaint in the same shell opens
`<file>: <builtin>: line N:`. That is the shape `printf` already had in
this column, and `kill` and `read` have at one site each; `[` is the
third builtin in the group (#3278, #3165).

The word named is the second difference and it is not a swap. Measured
2026-09-17 in the digest-pinned image, BusyBox v1.37.0:

| case | BusyBox 1.37.0 | ours, before |
| --- | --- | --- |
| `n=5; [ n -eq 5 ]` | `ash: n: out of range` | `case.sh: [: line N: n: out of range` |
| `[ -Q g.f ]` | `ash: g.f: unknown operand` | `-Q: unknown operand` |
| `[ -N n.f ]` | `ash: n.f: unknown operand` | `-N: unknown operand` |
| `[ a b c ]` | `ash: b: unknown operand` | `case.sh: line N: b: unknown operand` |
| `[ 1 -eq 2` | `ash: missing ]` | `case.sh: [: line N: missing ]` |
| `test -Q g.f` | `ash: g.f: unknown operand` | `-Q: unknown operand` |

The first row is the control that says the two are not simply swapped:
where the complaint really is about the operand, both name the operand.
What parts them is that **the two-word form is parsed here** rather than
sent straight to the unary evaluation, so a word this shell has no
operator for is a *string* and the word after it is one operand too many.
Six of the seven columns send it straight there and name the word in
front — bash `-Q: unary operator expected`, dash `-Q: unexpected
operator`, zsh `unknown condition: -Q` — which is why
`Diagnostics.TestNamesTheWordTheParseStoppedAt` is a field of its
own rather than `TestUnknownLongOperator` reaching one word further down.

### The parse reaches every form, not only two words

That shell's `test` is an expression parser over the words rather than the
argument-count table POSIX describes, and the field above is now named for
that rather than for the form it was first measured in. Measured 2026-09-18
in the same image, one word further along each time:

| case | BusyBox 1.37.0 | what the parse took |
| --- | --- | --- |
| `[ a b c ]` | `ash: b: unknown operand` | `a` |
| `[ -z a b ]` | `ash: b: unknown operand` | `-z a` |
| `[ ! a b ]` | `ash: b: unknown operand` | `! a` |
| `[ -Q x y ]` | `ash: x: unknown operand` | `-Q`, being no operator here |
| `[ -z a b c ]` | `ash: b: unknown operand` | `-z a` |
| `[ a = b = c ]` | `ash: =: unknown operand` | `a = b` |

The fourth row is what says this is a parse and not a count, and the last
two are what separate the **first leftover** word from the last word taken
— which is dash's answer to the same position, `Diagnostics.TestNamesFirst-
Operand`. `[ -z a b c ]` names `b` here and `a` there, over the same words.

### An operator with nothing behind it is missing an argument

The other half, and it is a different sentence rather than the same one
about a different word. A binary operator **this shell has**, standing in
the trailing position, is missing its right operand and names itself; a
connective there is missing its right operand too and names nothing.

| case | BusyBox 1.37.0 |
| --- | --- |
| `[ 1 -eq ]` | `ash: -eq: argument expected` |
| `[ g.f -ot ]` | `ash: -ot: argument expected` |
| `[ a == ]` | `ash: ==: argument expected` |
| `[ a =~ ]` | `ash: =~: unknown operand` |
| `[ x -a ]` | `ash: argument expected` |
| `[ -z a -a ]` | `ash: argument expected` |
| `[ 1 -eq 1 -a ]` | `ash: argument expected` |

The `==` and `=~` rows are the pair that says the set is exactly the
operators the shell has: the first is one of them and names itself, the
second is not and falls back to the word the parse stopped at. dash draws
the same line one operator over — it has `<` and `>` and not `==` — which
is why `Diagnostics.TestTrailingBinaryOperandExpected` is a wording both
columns hold rather than a list either of them carries (#3550).

### A group holding a unary operator alone never closes

The third difference in this builtin, and it is a defect in the shell
rather than a reading: a group whose contents are a unary operator and its
operand, standing as the **whole** expression, is `closing paren expected`
at 2 — where bash, ksh93, zsh and dash all evaluate it and answer 0
(#3419).

| case | BusyBox 1.37.0 | the rest of the panel |
| --- | --- | --- |
| `[ \( -n x \) ]` | `closing paren expected`, 2 | 0 |
| `[ \( -z "" \) ]` | `closing paren expected`, 2 | 0 |
| `[ \( -n \) ]` | `closing paren expected`, 2 | 0 |
| `[ ! \( -n x \) ]` | `closing paren expected`, 2 | 1 |
| `[ \( ! -n x \) ]` | 1 | 1 |
| `[ \( ! x \) ]` | 1 | 1 |
| `[ \( a \) ]` | 0 | 0 |
| `[ \( a = a \) ]` | 0 | 0 |
| `[ \( \( -n x \) \) ]` | 0 | 0 |
| `[ \( -n x \) -a x \]` | 0 | 0 |
| `[ \( -n x -a y \) ]` | 0 | 0 |

The controls are the whole of the finding. A parenthesized string, a
parenthesized comparison, a `!` in front of the operator *inside* the
group, a group inside a group and the same group with anything behind it
are all read — so this is neither "no grouping" nor "no unary in a group",
and the axis says what is observed rather than naming a rule
(`Semantics.TestGroupedUnaryAloneLosesTheClosingParen`). Three words or
four, behind any number of leading `!`s.

### And so does anything else that stops inside an open group

The general case of the same defect, and the paragraph above used to end by
setting it aside. Every expression that gives up while a `(` is still open
is that one sentence, whatever the reading would otherwise have said. The
status is 2 in both columns throughout, so what moves is the sentence
(`Semantics.TestFailureInsideAnUnclosedGroupIsTheParen`, #3665).

Measured 2026-09-18, BusyBox v1.37.0 in the pinned image with `cmd/ash`
cross-compiled and run in the same container:

| case | BusyBox 1.37.0 | here, before |
| --- | --- | --- |
| `[ \( x y \) ]` | `closing paren expected` | `y: unknown operand` |
| `[ \( -Q x \) ]` | the same | `x: unknown operand` |
| `[ \( -a x \) ]` | the same | `x: unknown operand` |
| `[ \( -n x y \) ]` | the same | `argument expected` |
| `[ \( -n x ]` | the same | `-n: unknown operand` |
| `[ \( \) ]` | the same | `): unknown operand` |
| `[ \( \) x ]` | the same | `): unknown operand` |
| `[ \( -a ]` | the same | `argument expected` |
| `[ \( \( x \) ]` | the same | `x: unknown operand` |
| `[ \( \( \) \) ]` | the same | `): unknown operand` |
| `[ \( x y z \) ]` | the same | `argument expected` |
| `[ \( x y \) junk ]` | the same | `argument expected` |
| `[ \( x y \) -a z ]` | the same | `argument expected` |
| `[ x -a \( y ]` | the same | `argument expected` |
| `[ x -a \( y z \) ]` | the same | `argument expected` |
| `[ x -a \( \) ]` | the same | `argument expected` |
| `[ \( x \) -a \( y z \) ]` | the same | `argument expected` |
| `[ \( \) junk ]` | the same | `(: unknown operand` |

**The controls are what make it an open group rather than a parenthesis.**
Once the group has closed, the leftover is named as an ordinary word — and
a second `(` standing there is a word too, not an opener:

| case | BusyBox 1.37.0 |
| --- | --- |
| `[ \( x \) junk ]` | `junk: unknown operand` |
| `[ \( x \) junk more ]` | `junk: unknown operand` |
| `[ \( x \) \( y \) ]` | `(: unknown operand` |
| `[ x \) ]` | `): unknown operand` — nothing was ever opened |

Fourteen more agree before and after: `[ \( x \) ]`, `[ \( x = x \) ]`,
`[ \( 1 -eq 1 \) ]`, `[ \( x \) -a \( y \) ]`, `[ ! \( x \) ]` and
`[ \( ! x \) ]` among them.

**dash is not this rule and has the same wording**, which is why the axis is
one column's. It reads `[ \( -n x \) ]` and `[ \( -n \) ]` at 0, names
the word in `[ \( x y \) ]`, and answers a silent **1** for an empty group
where BusyBox refuses — a status a script can see. See #3687.

## The bare `kill -l` listing is numbered, one per line

A fifth shape, sharing its number column with bash's and its bare name with
ksh93's and being neither: the number right-aligned in two, `) `, then the
name with no `SIG` in front of it, one entry per line
(`Diagnostics.KillListingNumberedPerLine`, #3655).

Measured 2026-09-18 against BusyBox v1.37.0 in the pinned image, `kill -l`
from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`, newlines
written `|`:

```
BusyBox ash   1) HUP| 2) INT| 3) QUIT|…|31) SYS|35) RTMIN|64) RTMAX|
dash          0|HUP|INT|QUIT|…|SYS|32|33|34|RTMIN|…|RTMAX|
here, before  0|HUP|INT|QUIT|…                       (dash's shape)
```

The first three lines as bytes are ` 1) HUP\n 2) INT\n 3) QUIT\n`. It also
**skips the positions it cannot name** — `31) SYS` then `35) RTMIN` then
`64) RTMAX`, with nothing between — which is the empty value of
`Diagnostics.KillListingUnnamedPosition` and was already right here: this
column's table has no name for those numbers either.

This had dash's zero-first shape, which is the one thing about the listing it
does not share with dash.

**Two spellings the applet reads that the table does not name**, measured the
same day in the same container:

| probe | BusyBox 1.37.0 | here, before |
| --- | --- | --- |
| `kill -l POLL` | `29` | `unknown signal 'POLL'` |
| `kill -l IO` | `29` | `29` |
| `kill -l IOT` | `6` | `unknown signal 'IOT'` |
| `kill -l CLD` | `unknown signal 'CLD'` | the same |
| `kill -l 6` | `ABRT` | `ABRT` |
| `kill -l 29` | `POLL` | `IO` |

The **listing** writes `29) POLL` and ` 6) ABRT` — and so does the
translating form, which is what settles what kind of value each is.
`Diagnostics.SignalListingWritesTheAlias` cannot say it, being one answer for
the whole table, but neither can a per-name version of it: `kill -l 29` is
`POLL` here and ksh93's `kill -l 6` is `ABRT` while its listing says `IOT`, so
one column writes its word in both places and the other holds a preference for
its listing alone.

So `POLL` is not an older spelling beside this table's name — it *is* this
shell's name for the signal, and `IO` is the platform's word that it also
reads. `Semantics.SignalNamesTheShellSpellsItsOwnWay` carries the one pair
(`IO=POLL`); it is written wherever a number comes back as a name and it
resolves back to the number, so `kill -s POLL`, `kill -POLL` and
`trap 'x' POLL` all reach signal 29 too. `IOT=ABRT` stays in
`SignalNamesTheShellAlsoReads`, which is the reading-only field it belongs to,
and `SignalListingWritesTheAlias` stays off (#3684).

## An unmatched `[` in a pattern is a character

`case [ in [)` takes the arm here, which is bash's and ksh93's answer and
not the sibling's — the value this dialect had been given by inheritance
and never measured (#3420). It reaches `[[ ]]` too, since that keyword is
`test` in this shell.

| probe | BusyBox 1.37.0 | dash 0.5.12 | bash 5.3 | zsh 5.9.2 |
| --- | --- | --- | --- | --- |
| `case [ in [)` | match | no | match | `bad pattern: [` |
| `case a[ in a[)` | match | no | match | — |
| `v='['; case [ in $v)` | match | no | match | — |
| `[[ "[" == "[" ]]` | 0 | — | — | — |
| `w='[a'; ${w#[}` | `a` | `[a` | `a` | — |
| `w='[a'; ${w#[[:alpha:]}` | (empty) | `[a` | (empty) | — |

The last row is the neighboring axis — a bracket a `[:name:]` inside it
left open — which #3379 recorded as unmeasured for want of a BusyBox to
ask. It is the same literal reading, so both axes are bash's here and
ksh93 is the column that moves between them. dash 0.5.13.1 reads the bare
`[` as a character as well, which is a *version* difference in that shell
rather than a platform one: the panel's dash is 0.5.12 and gives the
no-match answer on both kernels.

## `command local` declares nothing

`command local a=1` inside a function declares nothing, assigns nothing
and reports 0, here and in dash, where bash declares the local. The scope
the prefix puts the declaration in is not the function's (#3370).

| line | BusyBox 1.37.0 | dash 0.5.12 | bash 5.3 |
| --- | --- | --- | --- |
| `command local a=1`, read inside the call | `outer` | `outer` | `1` |
| the same read after the call | `outer` | `outer` | `outer` |
| `local b=2` with no prefix | `2` | `2` | `2` |
| `command local a=1` outside a function | silent, 0 | silent, 0 | — |
| `command local -x c=1` | `local: -x: bad variable name`, 2 | the same | — |

The last two rows are what say the builtin still runs: a bare `local`
outside a function is `not in a function` at 2 in both shells, and the
refusal that is left is the one the builtin gives a name it cannot have —
this shell gives `local` no option letters, so `-x` is a name. So the
operands are read and the declaration goes nowhere, rather than the word
being skipped. `command export a=1` and `command readonly a=1` assign
normally in both, which is what makes it `local` alone. The operand is
still not split there, which is a separate axis (#3341).

## `read`'s complaints are its own, and one of them is the applet's

Four separate differences, measured the same day and in the same way:

| probe | BusyBox 1.37.0 | ours, before |
| --- | --- | --- |
| `read -d` | `no arg for -d option`, 2 | `No arg`, 2 |
| `read -u x v` | `invalid file descriptor`, 2 | `x: invalid number`, 1 |
| `read -n x v` | `invalid count`, 2 | `x: invalid number`, 1 |
| `read -t x v` | `invalid timeout`, 2 | `x: invalid number`, 1 |
| `read 1bad` | `ash: read: '1bad': bad variable name`, 1 | located, 2 |

`read` is the only builtin in this shell that reaches
`OptionNeedsArgument` at all — the five letters above are the whole of the
measurement — so the capitalisation was dash's with nothing else holding
it up. The three numeric complaints are worded by the letter rather than
by one sentence with the operand in it, and the operand appears in none of
them.

The last row is the narrow half of the applet shape: `read` reports as the
applet for a bad *name* and as a located builtin one letter over, in the
same run. `unset`, `export`, `readonly`, `local` and `getopts` refuse the
same operand located and at 2, which is the control that says the
exception is this one complaint's rather than the dialect's.

## An assignment through an expansion is refused in dash's words

Measured 2026-09-20, BusyBox v1.37.0 in the pinned image, each line alone
in a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`:

| probe | BusyBox 1.37.0 | ours, before |
| --- | --- | --- |
| `echo "${@:=abc}"` | `@: bad variable name`, 2 | `not an identifier: @`, 2 |
| `echo "${*:=abc}"` | `*: bad variable name`, 2 | `not an identifier: *`, 2 |
| `echo "x${@:=abc}y"` | `@: bad variable name`, 2 | `not an identifier: @`, 2 |
| `echo "${1:=abc}"` | `1: bad variable name`, 2 | `not an identifier: 1`, 2 |

The sentence is dash's exactly, sigil-less name and all, and the third row
is what says so rather than ksh93's: the parameter is named and the word
the expansion stands in is not.

`Diagnostics.AssignThroughExpansionBadName` was unset in this column, and
that field's fallback is **zsh's** `not an identifier: @` — zsh being the
one shell whose grammar has `${name::=word}`, the operator that reaches
the question on every name. ash has no such operator and reaches the
question only through the conditional `${name:=word}` every dialect has,
so the unset field handed this column the sentence of the shell furthest
from it and nothing failed. An unset `Diagnostics` value is a wording
nobody measured, not a wording nobody needs (#3910).

The status was already right and is not this field's: dash and ash exit 2
where the other five columns exit 1, which is
`Semantics.FatalErrorStatusIsOne`.

## Vector summary

`dialect/ash/ash.go` carries the evidence for each answer beside the
answer. The shape of the file is the point worth repeating here: it is
one `syntax.Dialect`, one `interp.Semantics`, one `interp.Diagnostics`, a
one-line prelude, and an `Apply` that unregisters nine builtins this
shell does not have. `Prelude` sets `BB_ASH_VERSION`, which is the only
variable this shell names itself in — there is no `$ASH_VERSION` and no
`${.sh.version}` — so it is what a script testing for this shell tests
for.
