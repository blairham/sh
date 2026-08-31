# The semantics vector

Where shells disagree about **identical syntax**. These are conflicts,
not subsets: no amount of adding features to a smaller language produces
them, and no amount of removing features avoids them. Each one is a
named field on the semantics vector, set by a dialect preset.

Measured 2026-08-29, macOS arm64. Panel and method: `oracle.md`.

## The measured axes

| axis | dash | bash 5.3 | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| unquoted `$var` field-splits | yes | yes | yes | **no** |
| globs the *result* of an expansion | yes | yes | yes | **no** |
| `&>` is one redirection operator | **no** | yes | *build* | yes |
| assignment prefix persists on a special builtin | **yes** | no | **yes** | no |
| brace group needs a terminator before `}` | yes | yes | yes | **no** |
| `${#@}` is the count of parameters | **no** | yes | yes | yes |
| `[^abc]` negates | **no** | yes | yes | yes |
| a leading zero means octal | yes | yes | yes | **no** |
| a fatal error exits | **2** | 1 | 1 | 1 |
| a name-shaped value is re-evaluated | **no** | yes | **no** | yes |
| an invalid octal digit is an error | yes | yes | **no** | *n/a* |
| `${!x}` is the name, not the value | *n/a* | no | **yes** | *n/a* |
| `=cmd` expands to a path | *n/a* | no | no | **yes** |
| an EXIT trap set in a function fires there | no | no | no | **yes** |
| a signal handler sees the earlier `$?` | no | no | no | **yes** |
| an unset positional survives `set -u` | no | no | **yes** | no |
| `set +x` traces itself | yes | yes | **no** | yes |
| each assignment gets its own trace line | no | **yes** | **yes** | no |
| brace expansion happens | **no** | yes | yes | yes |
| arithmetic does floating point | no | no | **yes** | **yes** |
| quoting a `=~` regex makes it literal | *n/a* | **yes** | no | no |
| array index base | *n/a* | 0 | 0 | **1** |
| `echo` expands backslashes | **yes** | no | no | **yes** |
| glob with no match | passes pattern | passes pattern | passes pattern | **error** |
| last pipeline element runs in | subshell | subshell | **current shell** | **current shell** |
| `$0` inside a function | shell name | shell name | shell name | **function name** |
| `local` builtin | yes | yes | **absent** | yes |
| readonly reassignment | fatal | **continues** | fatal | fatal |
| `shift` past the end | fatal | **survives** | fatal | **survives** |
| unparseable text in a special builtin | **fatal** | survives | survives | survives |
| `.` cannot open its file | **fatal** | survives | **fatal** | survives |
| `.` with no operand | **not an error** | 2, survives | 2, fatal | 1, survives |
| `.` passes positional parameters | **no** | yes | yes | yes |
| `.` falls back to the current directory | no | **yes** | no | no |
| `source` as a synonym for `.` | **absent** | yes | yes | yes |

Probes, for reproduction:

    unquoted split   x="a b"; set -- $x; echo $#      → 2 2 2 1
    glob expansion   cd /; x="et*"; set -- $x         → etc etc etc et*
    same axis, again p="a*"; [[ abc == $p ]]          → n/a match match no-match
    &> operator      echo hi &>b; cat b               → hi+empty, [hi], hi+empty, [hi]
    array base       a=(x y); echo "${a[1]}"          → -  y y x
    arith error      echo $((1/0)); echo $?            → 2 1 1 1, and $? never prints
    echo backslash   echo 'a\tb'                      → expanded, literal, literal, expanded
    glob no match    echo /zzz_no_such*               → pattern, pattern, pattern, "no match" error
    pipeline last    echo x | read v; echo "[$v]"     → [] [] [x] [x]
    $0 in function   f() { echo "$0"; }; f            → shell, shell, shell, f
    readonly         readonly r=1; r=2; echo survived → fatal, CONTINUES, fatal, fatal
    shift past end   shift 5; echo survived           → fatal, survives, fatal, survives
    eval unparseable eval "if"; echo REACHED          → fatal, 2, 3, 1 — only dash stops
    dot missing file . /nope; echo REACHED            → fatal, 1, fatal, 127
    dot no operand   . ; echo "st=$?"                 → st=0, 2, fatal 2, 1
    dot arguments    . f.sh ARG   (f.sh echoes $1)    → OUTER, ARG, ARG, ARG
    dot cwd fallback PATH=/bin; . f.sh               → not found, FOUND, not found, not found
    source synonym   source f.sh                     → not found, works, works, works

The readonly probe must be a **plain assignment in a script file**.
Writing `r=2 2>/dev/null` makes it a command with a prefix and takes a
different path, which produces the opposite answer. This is the
contaminated-probe trap `oracle.md` warns about; it was hit while
producing this table.

## Why this is a vector and not a dialect level

Group the shells by which side of each axis they fall on:

    unquoted split       {zsh}
    globs expansions     {zsh}
    `&>` unsupported     {dash, ksh93≤93u+}  — {dash} alone on ksh93u+m
    prefix persists      {dash, ksh93}
    `${#@}` is a count   {dash}
    `[^…]` negates       {dash}
    brace expansion      {dash}
    function EXIT trap   {zsh}
    handler sees $?      {zsh}
    unset positional -u  {ksh93}
    set +x traces itself {ksh93}
    assignment per line  {bash, ksh93}
    `=cmd` expands       {zsh}
    leading zero octal   {zsh}
    fatal error status   {dash}
    `${!x}` is the name  {ksh93}
    name value recurses  {dash, ksh93}
    bad octal digit      {ksh93}
    arithmetic floats    {ksh93, zsh}
    quoted regex literal {bash}
    brace needs `;`      {zsh}
    array base           {zsh}
    echo backslash       {dash, zsh}
    glob no match        {zsh}
    pipeline last elem   {ksh93, zsh}
    $0 in function       {zsh}
    local absent         {ksh93}
    readonly continues   {bash}
    shift survives       {bash, zsh}

Eight distinct groupings across thirty axes: `{zsh}`, `{dash,zsh}`,
`{ksh93,zsh}`, `{ksh93}`, `{bash}`, `{bash,zsh}`, `{dash,ksh93}` and
`{dash}` — the last of which `${#@}` now produces on its own, where
previously it appeared only as the modern-ksh reading of the `&>` axis.

Neither new axis adds a grouping: the name-value one lands on
`{dash, ksh93}` and the octal-digit one on `{ksh93}`, both already
present. Twenty-one axes, still eight groupings.

The nineteenth axis, the exit status of a fatal error, was found by
a *test* rather than by the panel sweep: the interpreter had hardcoded 2,
the conformance run against bash disagreed, and changing the constant to
bash's 1 would have written one shell's policy into the core. It joins
`{dash}`, adding no grouping.

Its probe then found something the axis does not cover. All four shells
**abandon the rest of the script**: none of them reach the `echo $?`, so
the second command in that probe never runs anywhere. Fatality is
therefore *core* behavior that the substrate can simply implement, and
only the status is contested — the useful reminder being that a probe
written to measure one axis reported on two, and the universal half was
the half the interpreter had wrong.

It was then measured a second time and generalised. A readonly
reassignment and a `shift` past the end are unrelated to arithmetic and
to each other, and every shell gives all three failures the *same*
status — dash 2, the rest 1. The split belongs to the shell, not to the
error, so the axis is `FatalErrorStatusIsOne` rather than three
per-error axes. *Which* errors are fatal stays per-error, and there the
shells genuinely disagree: bash survives a readonly reassignment,
bash and zsh survive a long `shift`.

The twentieth and twenty-first axes were both already known and both
recorded as comments in the interpreter rather than as measurements.
`x=abc; echo $((x+1))` is 1 in bash and zsh, which re-evaluate a
name-shaped value, and an error in dash and ksh93; the code said "following
the two that agree", which is the written form of a guess.

That last row is worth its own note: **a panel member is not one thing.**
ksh93 AJM 93u+ (2012, macOS) and ksh93u+m 1.0.8 (2024, Debian) disagree
about `&>`, twelve years apart under the same name. An axis keyed on a
shell's *name* is therefore not implementable; it has to be keyed on a
configured value, which is what a semantics vector is.

**No ordering of these shells explains the data.** dash sides with zsh on
`echo` and against it on splitting. bash sides with zsh on `shift` and
against it on everything else. ksh93 sides with zsh on pipelines, is
alone on `local`, and pairs with dash on `&>` — a grouping no other axis
produces.

A "sh → bash → zsh" ladder is therefore not merely a simplification, it
is **contradicted by measurement**. Dialect is a point in a
multi-dimensional space, and the only structure that represents it
faithfully is a vector of independent switches.

This is the central architectural claim of this repository, and it is
the one thing that cannot be retrofitted cheaply. Every axis above is a
named field, set by a preset, read at the one place the behavior
happens.

## Not every difference announces itself

`&>` is the axis worth singling out, because it is the only one measured
so far where the divergent shells **do not fail**. dash and ksh93 have no
such operator, so `echo hi &>b` tokenizes as `echo hi &` — a background
command — followed by `>b`, which truncates the file. The command runs,
its output goes elsewhere, and the file is emptied, with no diagnostic.

An axis whose wrong answer is an error is self-limiting. An axis whose
wrong answer is a different working program is not, and it is the case a
dialect system exists to get right. See `grammar/tokenization.md`.

## One axis, two places

The glob-expansion row governs more than pathname expansion. The same
zsh rule — do not treat the *result* of an expansion as a pattern — decides
whether `p="a*"; [[ abc == $p ]]` matches, which it does in bash and ksh93
and does not in zsh.

That is worth stating because the obvious reading of the measurements is
two separate quirks. It is one behavior observed twice, and an
implementation with two switches for it will eventually set them
inconsistently.

## One rule in the standard, two axes here

The previous section is about one behavior that looked like two. This is
the reverse: one *rule* that looked like one axis and measured as two.

POSIX says a special builtin's failure is fatal to a non-interactive
shell. `eval` and `.` are both special builtins, so the obvious model is a
single `SpecialBuiltinFailureIsFatal` switch. The panel disagrees:

| failure | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `eval "if"` — unparseable text | fatal | survives | survives | survives |
| `. missing.sh` — cannot open | fatal | survives | **fatal** | survives |

ksh93 kept half the rule. One switch would have to give it either bash's
answer for the file or dash's answer for the eval, and both are wrong. So
there are two axes, `BuiltinSyntaxErrorFatal` and `DotMissingFileFatal`,
and dash answers yes to both, bash and zsh no to both, and ksh93 one of
each.

The same failure also splits three ways on *status*, which is a
`Diagnostics` question rather than a semantics one, and one of those
answers depends on where the text was read from: zsh reports 1 for
unparseable text from `-c` and 126 for the same text in a file `.` opened.
That is `SourcedSyntaxErrorStatus`, and it exists so zsh can hold both
answers at once.

Three questions, then, about what reads in the standard as one sentence:
is it fatal, what status does it carry, and how is it worded. The rule for
adding an axis at the end of this file is what forced them apart.

## One option, six divergences

`set -x` produced more disagreement than any other single feature
measured, and all of it is decoration. The structure is unanimous — every
simple command to stderr, expanded, before it runs, and compound commands
not traced — and then the four shells differ on the prefix, on whether an
expanded field with a space in it is quoted, on which quoting a embedded
quote gets, on whether `a=1 b=2` is one line or two, on whether `set +x`
prints itself, and on whether a `for` header is printed at all.

Four are implemented. Two are recorded and deliberately not: bash and zsh
print a compound command's header once per iteration, and ksh93 prints
pipeline elements last-first, which follows from its running the last one
in the current shell. Both are visible only in a debugging aid, and
reproducing them costs more than the fidelity is worth — which is a
judgement, and is written here so it can be revisited rather than
rediscovered.

The count matters more than any one of them. An option nobody would call
contentious carries six divergences, which is the strongest evidence yet
for the claim this document opens with: dialect is not a ladder, and the
disagreements are not where anyone expects them.

## Timing is not a divergence

Signals arrive asynchronously, and the first implementation recorded them
on a goroutine and read them between commands. That left a window: `kill
-INT $$` returned before the arrival had been recorded, so the handler
fired one command late — sometimes. Two dialects looked like they
disagreed with the others, and neither did; it was the same code being
timed differently.

Draining the channel where the handler runs, rather than in a collector,
closes it. The lesson is about the instrument: a corpus records what a
shell does, and it cannot record a *sometimes*. Anything measured has to
be deterministic first, or the measurement is of the machine rather than
of the shell.

## A second axis that is an ordering

`exit` is not equally fussy about what it is given:

    exit -1     dash → error 2   bash → 255   ksh93, zsh → 255
    exit abc    dash → error 2   bash → 2     ksh93, zsh → 0

dash refuses both, bash refuses only the one that is not a number, and
ksh93 and zsh take either. Three behaviors on a line rather than two
sides, so `ExitArgument` is a policy with three values — the shape
`UnterminatedBracket` established, used a second time without argument.

The status those two refusals carry is *not* the fatal-error axis. bash
exits 1 for a fatal error and 2 for this, and dash exits 2 for both; a
usage error is its own thing, and unanimous where it happens at all.

## A prediction that measurement contradicted

`set -e` was expected to produce axes. Its exemptions are where shells are
said to differ, and the survey that chose it as the next piece said so.

Twenty-four probes across dash, bash, ksh93 and zsh: **no divergence at
all**, byte for byte, including the two that implementations usually get
wrong. The exemption for a tested status is inherited into functions and
all the way down anything they call, and an assignment reports what its
command substitution reported. Both unanimous.

So `set -e` is core behavior and got no axis. Worth recording because the
method is supposed to cut both ways: measuring is what stops a divergence
being invented as readily as it stops one being missed.

## A divergence measured and not implemented

zsh writes to *every* redirection target where the others write only to the
last:

    echo x >a >b        dash, bash, ksh93 → b only
                        zsh              → both

Nothing is reported either way, so it is the `&>` shape again. It is
recorded as `redir/multios-is-zsh-only` and deliberately not implemented:
writing to several targets at once is a feature rather than an answer, and
adding it unasked would be inventing behavior for three of the four.

It also demonstrates the blind spot recorded above, on a case chosen for
something else. The two answers differ in *output* and agree on the exit
status, so the behavioral score counts them as agreeing and every other
view calls it wording. A shell that writes to the wrong file is not a
wording difference.

## Wording is a third kind of answer

`Diagnostics` began as one number and now carries what a shell *says*. The
messages are formats, and empty means the substrate's own — so a dialect
states only where it differs, exactly as a semantics preset does.

Only failures the panel words differently *for the same diagnosis* are
here. Where a shell reaches a different diagnosis no wording can close the
gap: dash calls `[[ ( x ) ]]` "word unexpected (expecting \")\")" where
we say the paren is unexpected, and matching that would mean imitating
dash's parser rather than its vocabulary. Those cases stay open and are
listed as such.

Two things that look like wording are not:

- A shell running a script names the **script** in `$0` and in every
  diagnostic, not itself. ksh93 also changes how it names the line —
  nothing for `-c`, "line 2" for a file — which is the only place in the
  panel where the two forms differ.
- `Wording` lets a format ignore the arguments it is given. dash's `shift`
  message names no count where ksh93's does, and passing the count to both
  is simpler than deciding per dialect which to pass — provided the unused
  one does not become `%!(EXTRA int=5)`, which is what it did first.

## A value in a preset is not an implementation either

`LastPipelineElementInCurrentShell` had a value in all five presets and
nothing read it. `runPipeline` carried a comment saying so — "this takes
the majority until the interpreter carries a dialect" — written before the
interpreter carried one, and left behind when it did.

Implementing it raised a question the other axes do not. The answer is
unobservable through an external command: `echo x | cat` behaves the same
either way, and refusing every pipeline for want of a dialect would make
the core useless. So the axis is asked only when the last element is a
builtin, a function, or a group — something that can touch the shell.
That is the rule `BracketCaretNegates` already uses: ask about the
construct in front of you, not about every construct sharing a code path.

Which builtins a shell *has* is not on this list and should not be. It is
neither grammar nor a conflict of meaning: ksh93 simply lacks `local` and
reports it as a command that was not found. A dialect says so through the
extension seam — `dialect/ksh` calls `Unregister("local")` — which is the
same mechanism anything built on the substrate would use, the difference
being that this one takes something away.

## A row in this table is not an implementation

`[^abc]` negates was measured, written into the table above, and never
wired: the matcher treated `^` as negation unconditionally, with a comment
saying dash does not have it and that this was "accepted here because the
core excludes dash". The row and the code disagreed, and the conformance
run could not see it, because dash and the core give the same *exit
status* for a `case` that takes a different branch.

It was found by reading the wording bucket rather than the behavioral
one. Three of the entries there were not diagnostics at all:

    echo {1..3}                    dash prints it literally; we expanded it
    case d in [^abc])              dash does not negate; we did
    >b with no command             creates the file; we created nothing

None of the three changes an exit status, so all three were counted as
agreements by the behavioral score and as wording by everything else. A
score that compares only statuses cannot see a construct that silently
produces the wrong output, which is the failure mode this project exists
to be honest about — so the wording bucket is worth reading, not just
counting.

## An axis that is not binary

`${!x}` does not fit the table above, and forcing it in would misreport it.
With `x=y` and `y=V`:

    bash   →  V      indirection
    dash   →  error
    zsh    →  error
    ksh93  →  x      neither, and not an error

Three answers rather than two, and the framing "which side is each shell
on" cannot hold them.

It is now expressed, and the way it was expressed is the argument for the
whole grammar/semantics split rather than a workaround for one construct.
The three answers are not three answers to one question. They are two
answers to two questions:

    does `${!x}` parse?        bash yes, ksh93 yes, dash no, zsh no
    does it yield the name?    ksh93 yes, bash no

The first is grammar and additive — a construct parses or it does not —
so it is `Dialect.ParamIndirection`. The second is semantics and a
conflict, so it is `IndirectionYieldsName`. Each half is binary. Nothing
needed a third state; what was needed was noticing that one field was
being asked two questions, which is the same fault as
`ArithLeadingZeroIsOctal` and was found the same way.

It is also the third measured instance of a divergence that does not
announce itself, after `&>` and `[[ ]]`. A fourth was in
`grammar/arithmetic.md`: `x=abc; $((x+1))` errors in dash and in ksh93 —
for different reasons — and yields 1 in bash and zsh, which re-evaluate
the value as an expression and reach an unset name. That one is binary
and is now in the table above as `ArithNameValueRecurses`; it sat in the
interpreter as a comment saying "following the two that agree" until
something made it fail out loud.

An **unterminated bracket expression** is the one genuinely non-binary
question left, and it is load-bearing rather than exotic,
because `[` is the name of the test builtin:

    case "[" in [) echo hit;; *) echo miss;; esac

    bash   →  hit           a literal `[`
    ksh93  →  hit           a literal `[`
    dash   →  miss          a class that matches nothing
    zsh    →  error         "bad pattern: ["

Three answers again. Against the *filesystem* the panel agrees a lone `[`
is literal — which is what lets `[ a = a ]` run at all — and only zsh
rejects `[a`. The core implements the agreed half; the `case` half is
recorded in the corpus as `pat/unterminated-bracket` and is not yet
answered, because answering it needs the non-binary shape this section
describes rather than another bool.

A second candidate appeared and turned out to belong elsewhere: **the exit
status of a syntax error** is 2 in dash and bash, 3 in ksh93 and 1 in zsh.
That is a value, not a side, and it went to `Diagnostics` — a shell that
owns its wording owns its status.

## The prediction about Answer was wrong

This document argued that `Answer` would have to grow a third state before
a third non-binary axis arrived. Writing the bracket policy showed that
would have been worse.

`UnterminatedBracket` has its own type, `BracketPolicy`, with its own four
values. A wider `Answer` would have let `BracketBadPattern` be assigned to
any of the twenty-three genuinely binary axes and still compile, and the
type would have stopped saying what it says now: *this axis has two sides
and the shells picked different ones*. An axis with three answers gets a
type with three values; the binary ones keep the type that says so.

The rule that survived is the other half of the note above: **one field,
one question**. `${!x}` was one field asked two questions and split into a
grammar flag and a binary axis. `ArithLeadingZeroIsOctal` was one field
asked two questions and split into two axes. The bracket policy is one
field asked one question that happens to have three answers, and the fix
is a wider *field*, not a wider `Answer`.

The most dangerous of them all is in that document too, and it is binary,
so it is in the table above: **a leading zero means octal everywhere but
zsh**, where `$((0100))` is one hundred rather than sixty-four. Nothing
warns, both answers are plausible numbers, and file modes are written that
way.

## A measured non-conflict, recorded so it is not over-generalised

**zsh splits an unquoted command substitution**, exactly like the other
three, even though it does not split a parameter expansion:

    x='a b'; set -- $x;              echo $#   → 2 2 2 **1**
    set -- $(printf 'a b');          echo $#   → 2 2 2 **2**

"zsh does not word-split" is the usual summary and it is too broad. The
splitting axis covers parameter expansion only, and modeling it as one
switch over the whole of field splitting gives the wrong answer for
`$(...)`. See `grammar/word-splitting.md`.

## A third vector

Some differences are neither a construct that parses nor a behavior with
two sides. They are *values*: what a shell prints when it refuses, and
which number it exits with. `Diagnostics` holds those.

The first member is the exit status of a syntax error, measured across
eight distinct ones — a stray `}`, `echo (`, an unterminated `if`,
`case`, `for` — and stable within each shell:

    dash 2    bash 2    ksh93 3    zsh 1

It had been hardcoded to 2, under a comment reading "a syntax error is 2
in every shell in the panel". That is true of half of them.

`Diagnostics` differs from this file's vector in one deliberate way: its
zero value means *the substrate's own*, not *unset*. A semantics axis
with no answer is refused, because answering it would claim some shell's
behavior. A status makes no such claim — the process must exit with some
number, and refusing is not one of the options. `sh` is itself a shell,
so where no dialect is chosen it answers for itself, with 2.

The seam reaches both places a script is parsed. A command substitution
re-parses at expansion time, and a parse error there is fatal in all four
shells; this used to report it and carry on, which is the same shape as
the redirect, arithmetic and glob failures recorded above. Our status
there is the dialect's; bash's own is 127 rather than its usual 2,
because bash parses the whole input at once and never reaches expansion,
which is an architectural difference rather than an axis.

## Rules for adding an axis

1. **It must be measured**, with the probe recorded here. An axis added
   from a manual and not from a run is a guess.
2. **It must be named for the behavior, not for the dialect that wants
   it.** `WordSplitUnquoted`, not `ZshMode`. A field named after a
   dialect will collect unrelated behavior and become impossible to
   reason about — which is exactly what a monolithic `posix` flag
   becomes.
3. **It is read at the site of the behavior, once.** Never
   `if dialect == zsh` scattered across call sites. The whole point is
   that a second dialect must not add a second condition to every
   existing one.
4. **Non-conflicts do not get an axis.** `${x:-y}`, `$((u+1))` with `u`
   unset, and field-count of an unset variable were measured and agree
   across the panel. They are core behavior, and adding a switch for
   them would be inventing a difference.
