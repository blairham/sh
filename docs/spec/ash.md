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
they are listed under *What could not be said* below. It is a claim about
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
re-litigated. An ash column, when it lands, **widens** the non-dash
agreement rather than narrowing it.

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
hides the next question along it. Sixteen are answered in
`dialect/ash/ash.go`, each against a BusyBox probe; the remaining three
are items 4, 5 and 6 under *What could not be said*.

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

What it does do is make the #2272 shape impossible to ship quietly, which
was the specific failure: an axis added elsewhere, unanswered here,
refusing at run time with the test suite green.

An axis this dialect genuinely cannot answer is recorded where the
omission is, as a line in `ash.go`:

    // unanswered DollarSingleNulTruncates: a *third* reading (#2276). …

which the coverage report prints under the entry it answers. So the check
states what is unmeasured rather than being switched off — and, the other
way round, a value quietly appearing for one of the three items under
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

## What could not be said

Six measured behaviors have no value on any existing axis. They are
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
the one of the three that costs real corpus rows, and it is now
measurable: the ash column is what the widening would be graded against.
It is deliberately not part of #2263 — a 14-file substrate edit folded
into the change that builds the instrument would be graded by the same
commit that invented the grade.

**2. `line 0` under `-c`.** On the command-string route this shell
numbers a builtin's line from zero — `ash -c 'unset "a[0]"'` is `line 0`
and a second line is `line 1` — while a script file and standard input
both number from one. No axis carries a per-route line origin.

**3. `divide by zero`.** The reason word for a division by zero is the
substrate's own string (`division by zero`); this shell writes `divide by
zero`. There is no `Diagnostics` field for it, and adding one for a
single word was not worth a substrate edit.

**4. A NUL inside `$'…'`.** `x=$'a\0b'` leaves `ab` at length 2 — the
byte is neither the end of the span (bash and ksh93, length 1) nor a
character of it (zsh, length 3) but **dropped**, and the octal and hex
spellings agree. `Semantics.DollarSingleNulTruncates` is an `Answer` and
has no room for a third reading, so the axis is left unanswered here
rather than set to one of the two wrong values. #2276.

**5. `readonly -a`.** `readonly: illegal option -a`, and there is no
`typeset` at all. Our binary accepts the letter because `readonly`'s
option set is fixed in the interpreter rather than taken from the vector
the way `ReadOptions` and `EchoOptions` are, and then walks into
`ReadonlyRecordsTheCompoundAttribute`, which this shell cannot answer
because it cannot be asked. ksh93 has the same hole today and for the
same reason. #2277.

**6. `ulimit -a`.** Measured in full — fifteen rows, `core file size
(blocks)         (-c) unlimited` and its fellows, the letter in its own
parenthesis at the end. Five of them (`-e`, `-i`, `-q`, `-r`, `-x`) name
resources no `interp.Resource` constant does, and they were measured on
Linux, where those limits exist; what a macOS build of this shell should
print for them is a second measurement and not this one. #2278.

A seventh is a difference in kind rather than in wording: **`[[ ]]` here is
a builtin, not a keyword.** `type '[['` answers `[[ is a shell builtin`,
and it does not suppress word splitting — `v="a b"; [[ $v == "a b" ]]` is
`b: unknown operand`. The parser still has to know the construct, because
`&&` and `||` inside it are not the shell's operators, so the dialect
sets `DoubleBracket` and accepts that our `[[` is the keyword kind. It is
the largest single family of remaining corpus disagreements (28 rows).

## Vector summary

`dialect/ash/ash.go` carries the evidence for each answer beside the
answer. The shape of the file is the point worth repeating here: it is
one `syntax.Dialect`, one `interp.Semantics`, one `interp.Diagnostics`, a
one-line prelude, and an `Apply` that unregisters nine builtins this
shell does not have. `Prelude` sets `BB_ASH_VERSION`, which is the only
variable this shell names itself in — there is no `$ASH_VERSION` and no
`${.sh.version}` — so it is what a script testing for this shell tests
for.
