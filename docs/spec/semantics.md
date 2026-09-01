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
| `}` as an ordinary argument | yes | yes | yes | **reserved** |
| an unterminated construct is reported on | last line | **the line after** | last line | last line |
| `${#@}` is the count of parameters | **no** | yes | yes | yes |
| `[^abc]` negates | **no** | yes | yes | yes |
| `@(a|b)` in a pattern | no | **in `[[ ]]` only** | yes | no |
| a bare `(a|b)` in a pattern | no | no | no | **yes** |
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
| `set -f` turns off globbing | yes | yes | yes | **no** |
| each assignment gets its own trace line | no | **yes** | **yes** | no |
| brace expansion happens | **no** | yes | yes | yes |
| arithmetic does floating point | no | no | **yes** | **yes** |
| significant digits in a float | *n/a* | *n/a* | 15 | **17** |
| a whole float keeps its point | *n/a* | *n/a* | no | **yes** |
| an integer-only operator on a float | *n/a* | *n/a* | **refused** | truncated |
| a malformed expression reports | 2 | **1** | **1** | 1 |
| quoting a `=~` regex makes it literal | *n/a* | **yes** | no | no |
| array index base | *n/a* | 0 | 0 | **1** |
| plain `$a` on an array | *n/a* | first element | first element | **all, joined** |
| pipeline-status name | *none* | `PIPESTATUS` | *none* | `pipestatus` |
| a bare assignment updates it | *n/a* | **yes** | *n/a* | no |
| `unset` ends it | *n/a* | no | *n/a* | **yes** |
| `echo` expands backslashes | **yes** | no | no | **yes** |
| glob with no match | passes pattern | passes pattern | passes pattern | **error** |
| last pipeline element runs in | subshell | subshell | **current shell** | **current shell** |
| `$0` inside a function | shell name | shell name | shell name | **function name** |
| `local` builtin | yes | yes | **absent** | yes |
| `select` menu layout | *n/a* | vertical, then tabs | vertical | **columns** |
| `select` prompt | *n/a* | `#? ` | `#? ` *(terminal only)* | **`?# `** |
| input ending a `select` | *n/a* | 1, newline on stdout | 1 | **0**, newline on stderr |
| `typeset` needs a `function`-defined function to declare a local | *n/a* | no | **yes** | no |
| a name declared without a value counts as set | no | no | no | **yes** |
| readonly reassignment | fatal | **continues** | fatal | fatal |
| `shift` past the end | fatal | **survives** | fatal | **survives** |
| unparseable text in a special builtin | **fatal** | survives | survives | survives |
| `.` cannot open its file | **fatal** | survives | **fatal** | survives |
| `.` with no operand | **not an error** | 2, survives | 2, fatal | 1, survives |
| `.` passes positional parameters | **no** | yes | yes | yes |
| `.` falls back to the current directory | no | **yes** | no | no |
| `source` as a synonym for `.` | **absent** | yes | yes | yes |
| an empty PATH means the cwd | yes | yes | **no** | yes |
| `times` layout | self/children | self/children | **user/sys, labeled** | self/children |
| `times` decimal places | **6** | **3** | 2 | 2 |
| `times` with an argument | ignored | ignored | **syntax error** | **refused** |
| a diagnostic names the builtin | no | no | no | **in the location** |
| a failed `exec` runs the EXIT trap | yes | yes | **no** | **no** |
| `exec` reads options of its own | **no** | yes | yes | yes |
| a failed `exec` names | the operand | **the resolved path** | the operand | the operand |
| `exec` on a directory says | permission | is-a-directory | is-a-directory | permission |

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
    empty PATH       PATH=; ./x-in-cwd-by-name       → runs, runs, NOT FOUND, runs
    times shape      times | sed -E 's/[0-9]+/N/g'    → 2x2, 2x2, labeled user/sys, 2x2
    times argument   times foo; echo $?              → 0, 0, syntax error 3, refused 1
    builtin in prefix shift 5                        → sh: 1: shift: …, …, …, zsh:shift:1: …
    exec trap        trap T EXIT; exec nosuch        → T, T, silent, silent
    exec options     exec -a n sh -c 'echo $0'       → not found, n, n, n
    exec names       cd /tmp; exec ./noexec.sh       → ./noexec.sh, /tmp/noexec.sh, ./…, ./…
    exec a directory exec /tmp                       → Permission denied, Is a directory, Is a…, permission…

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

## A divergence that is not the shells' but ours

Every other row in this file is a disagreement between real shells. This one
is a disagreement between what a shell must do and what a *library* may do,
and it is recorded here because it changed the design rather than a value.

`exec cmd` replaces the process. A shell calls execve and becomes the
command: same pid, same signal dispositions, nothing of the shell left.
`interp` is a library, so doing that unconditionally means a program which
embeds a Runner to interpret a script gets replaced by whatever that script
named — not a shell feature but a way to lose a program.

So `Runner.ReplaceProcess` is a hook with no default implementation. Nil
runs the command as a child and stops the script with its status;
`driver` sets it, because a binary that *is* a shell is the one place the
call is correct. Everything the conformance harness can observe is the same
either way — the output, the status, and that nothing after it runs. What
differs is the pid, the signal dispositions, and which process the parent
waits for.

The measurement that made this more than a precaution: **a subshell must
never replace the process at all.** `( exec echo hi ); echo after` prints
both lines in every shell in the panel, because there a subshell is a
separate process. Here it is a cloned Runner inside the same process, so
execve in one takes the parent shell with it — and did, in all four dialect
binaries, until the guard existed. A pipeline element is a subshell by
another name and had the same hole.

That is the general shape of the hazard, and it is worth stating once:
**where a real shell relies on process boundaries, an implementation that
does not have them has to reconstruct the boundary explicitly.** The EXIT
trap is the same problem in miniature — no shell runs one after a
successful `exec`, not as a rule but because the trap died with the process
— so standing in a child means clearing it by hand or printing a handler
nothing else prints.

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
narrows it. It does not close it, and this section said it did for
several months.

The window that remained was one level down. `os/signal` catches the
signal in the runtime and forwards it to a channel from a goroutine of
its own, so *any* drain that reads a channel is asking a question whose
answer depends on the scheduler. Removing our collector removed one
goroutine and left the other one, which is why the symptom got rare
enough to look like a flaky test rather than a defect. Measured on a
loaded machine: three failures in three hundred runs of one corpus case,
against none at all in three hundred runs on an idle one — one of them
running the handler a command late, and two never running it at all,
because the script ended while the forwarding goroutine was still
waiting to be scheduled. The shell was losing trapped signals.

What closes it is not a better drain. It is noticing that the shell was
asking the kernel to tell it something it already knew: `kill -INT $$` is
the shell signaling *itself*, and the only reason the answer had to come
back through the runtime is that `kill` was not a builtin. Making it one
— see `interp/killbuiltin.go` — means the arrival is recorded at the
point of sending, and a signal a script aims at the shell never leaves
the shell at all.

That last part is a design decision and not just an optimization. A
Runner is embedded in other programs, and routing a script's `kill -INT
$$` through the process would let a line of shell fire the *host's*
signal handlers. The core does not do that, for the same reason its `cd`
does not call `os.Chdir`. What a script sends the shell is delivered to
the shell's traps; what it sends anywhere else is a real signal to a real
process.

An untrapped fatal signal is the mirror image and needed the same
treatment for a sharper reason: the kernel runs the default action on
whichever thread it likes, and a shell has several, so `kill -INT $$`
with no trap printed the *next* command's output before the process went
away. Nothing can make the dying thread win that race. So the shell stops
the script, runs the EXIT trap where the dialect says it should, and
hands the death to `driver` — the same split `exec` uses, and for the
same reason.

The lesson is about the instrument: a corpus records what a shell does,
and it cannot record a *sometimes*. Anything measured has to be
deterministic first, or the measurement is of the machine rather than of
the shell. It is also about how a *sometimes* hides. This one survived
because the external `/bin/kill` it used to run took about a millisecond
to fork and exec, which was enough for the forwarding goroutine to win
almost every time. Making `kill` a builtin removed that accidental
padding, and the same case with the recording mutated out fails three
hundred times in three hundred rather than three.

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

## One axis in three places

**dash reads no SIG-prefixed signal name.** The prefix is simply not part
of a signal's name there, and the same refusal surfaces three times with
three different sentences:

    trap 'x' SIGINT     trap: SIGINT: bad trap
    kill -SIGINT $$     kill: Illegal option -S
    kill -s SIGINT $$   kill: invalid signal number or name: SIGINT

bash, ksh93 and zsh take all three. It is one axis rather than one per
builtin because it is a property of how the shell *reads a signal name*,
and the shell that refuses it refuses it everywhere — which is also the
argument for asking it in `canonicalSignal` and in `kill`'s spec parser
rather than at each of the places a name arrives.

Two details of it are worth keeping, because each was a wrong answer
before it was measured. dash names only the **first character** of an
illegal option — `-SIGCONT` is `Illegal option -S`, and `-99` is
`Illegal option -9` — because it stopped reading there; the wording takes
that as a second verb. And zsh names an unknown signal with exactly
**one** prefix however many the operand arrived with: `Q` is `SIGQ` and
`SIGNOPE` is `SIGNOPE`, where a format that added one to the operand as
written produced `SIGSIGNOPE`.

Measuring it also turned up something that was not an axis at all. The
trappable set was nine names, so `trap 'x' CONT` was refused as a signal
this shell cannot catch — in a shell where all four panel members catch
it, along with CHLD, WINCH, TSTP, URG, IO, SYS, TRAP and XCPU. The set is
now derived: everything except KILL and STOP. Refusing those two remains
a deliberate divergence, since all four accept `trap … KILL` and then
never fire it, and it keeps its own wording rather than borrowing the
dialect's complaint about a word that names nothing.

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

## An axis that is only about one of two names

`typeset` and `local` do the same thing and do not have the same rule.

Every shell that has `local` — all but ksh93, which does not — makes it
local in a function defined either way. `typeset` is where ksh93 differs,
and it differs on the *definition syntax* rather than on the declaration:

    x=outer; f() { typeset x=inner; }; f; echo $x
      bash inner? no — outer.  ksh93 **inner**.  zsh outer.

    x=outer; function f { typeset x=inner; }; f; echo $x
      outer everywhere, ksh93 included.

Only a function defined with the `function` word has a scope for ksh93 to
declare into. So the axis is `TypesetLocalNeedsKeywordFunction` and not
`LocalNeedsKeywordFunction`: naming it after `local` would have made the
bare core refuse a construct on which the shells do not actually disagree,
and `f() { local x=1; }` is far too common to refuse for a difference that
is not there. It was named that way first, and the core stopped running
`local` at all — which is how the distinction was found.

It is asked only where the two answers differ. Inside a keyword-defined
function they do not, so that case needs no dialect.

## A declaration is an assignment, and expands like one

`export`, `readonly`, `local` and `typeset` are declaration utilities:
their `name=value` arguments are assignments, so the value is subject to
tilde, parameter, command and arithmetic expansion and **not** to field
splitting or pathname expansion.

This is not an axis — every shell in the panel agrees — but it is worth a
section because getting it wrong is quiet. Sending an assignment's value
through the ordinary word pipeline produced three wrong answers at once:

    n=*                → the directory listing, not `*`
    IFS=:; y=a:b; x=$y → `a b`, because the fields were split and
                          rejoined on a space
    typeset -i n=3*3   → a refusal, because `3*3` matched no file

The first two are ordinary assignments and had been wrong from the start.
Nothing in the corpus used a value that looked like a pattern, so nothing
noticed until a case about the integer attribute needed one.

Array elements are not this: `a=(*.txt)` does glob, because each element
is an ordinary word. The rule is about an assignment's *value*, not about
the `=`.

Which names are declaration utilities is a dialect's answer rather than an
axis, for the same reason `source` is: bash and zsh add `declare`, ksh93
has only `typeset`, and dash has neither — and in dash `declare x=*` is an
ordinary command with an ordinary globbed argument, so applying the rule
to the name everywhere would be wrong in the shell that lacks it.

## The widest presentation difference in the panel

`select` prints a menu, and no two shells draw it the same way. The same
twelve items, at `COLUMNS=80`:

    bash    1) 1<TAB> 3) 3<TAB> 5) 5<TAB> 7) 7<TAB> 9) 9<TAB>11) 11
            2) 2<TAB> 4) 4<TAB> 6) 6<TAB> 8) 8<TAB>10) 10<TAB>12) 12
    ksh93    1) 1
             2) 2
            … one per line, all twelve
    zsh     1) 1    3) 3    5) 5    7) 7    9) 9    11) 11
            2) 2    4) 4    6) 6    8) 8    10) 10  12) 12

Three engines, not two answers with a tie-break, which is why `SelectLayout`
is a named type rather than a flag:

- **vertical** — one per line, numbers right-aligned so `9)` and `10)` line
  up their parentheses. ksh93's.
- **vertical, then columns** — bash's, and the rule is the opposite way
  round from how it sounds: one item per line *while the whole list would
  fit on one line*, and tab-separated columns once it would not. Widening
  the terminal to 200 columns puts twelve items back on twelve lines.
- **columns** — zsh's, always packed, so even three items share a line.
  Every cell is padded to the same width, the last one included, so a row
  ends in trailing spaces.

All three fill **column-major**: reading *down* the first column gives 1, 2,
3. And all three put the menu and the prompt on standard error, so a
script's own output can be redirected without taking the menu with it.

Around the layout sit four more answers, each its own question:

- the prompt when `PS3` is unset, which is `#? ` twice and `?# ` once;
- whether the prompt is printed at all — ksh93 withholds it unless the
  input is a terminal, which is why a ksh93 transcript has menus and no
  prompts in it;
- what an unset `COLUMNS` means: 80 to bash, and no limit at all to zsh,
  which puts forty items on one line rather than wrapping;
- and what the input running out does, which is three questions and no
  shell answers them alike — bash reports 1 and writes a newline to
  standard *output*, ksh93 reports 1 and writes nothing, zsh reports 0 and
  closes the prompt line on standard error.

Everything else about the loop is unanimous and is not an axis: a reply
naming no item leaves the variable empty, keeps the raw line in `REPLY` and
runs the body anyway; a blank line reprints the menu without running it; the
menu is printed once and the prompt every iteration; `PS3` is read fresh each
time; and an empty menu does not prompt at all — the loop never runs and the
status is 0, which is the difference between a menu with nothing in it and a
menu nobody answered.

### Two things measured here and not built

**ksh93 columnizes on height, not width.** Its menu goes to columns when the
list is longer than the terminal, and the threshold is `LINES` — 30 items at
`LINES=24` is two columns, and 12 items at `LINES=10` is two columns as well,
where 12 at `LINES=24` is twelve lines. It is not built because a menu long
enough to reach it cannot be graded by the corpus, whose cases do not control
`LINES`, and a layout with no case behind it is a layout nobody can re-check.

**zsh's cell width narrows in a way twelve samples did not explain.** With
`COLUMNS` unset, or at 72 and above, its cell is the longest entry plus two,
which is what is implemented and what matches. Below that it is sometimes one
wider — 9 rather than 8 at `COLUMNS=40` and at 20, but 8 again at 36 — and
the rule behind that is not derived here. A narrow terminal is the one place
this implementation's menu differs from zsh's, and it is written down rather
than approximated.

### The harness was the contaminated probe this time

Splitting the two EOF newlines between the streams needs care, and the first
attempt got it wrong in a way worth recording. Measuring with
`2>&1 >/dev/null` from an interactive **zsh** reports both newlines on
standard error for every shell — because zsh's MULTIOS option sends the
output to both destinations rather than to the last one. The probe was
contaminated by the shell running it rather than by the shell under test,
which is the same trap `oracle.md` records from the other direction. Writing
each stream to its own file is what settled it.

## The record `$?` cannot hold

`$?` reports a pipeline's *last* command. `false | true` therefore succeeds,
and without something else a script cannot discover that the first half
failed — which is the entire reason two of the four shells keep the rest.

The values are unanimous where they exist. What is not is the **name**:
bash's `PIPESTATUS`, zsh's `pipestatus`, and no name at all in ksh93 or
dash. So the core keeps the record and a dialect names it through `Apply`,
the same seam that gives ksh93 `source` and takes `local` away. With no name
the record is unreadable, and — the part that matters — the axes below are
never asked, so the bare core does not refuse an `x=1` over a difference
nothing in that shell could observe.

Despite the name it is not only for pipelines. A command on its own records
one element, and so does a compound one: after `if false | true; then :; fi`
the record holds the `if`'s own status, because the `if` is the command that
just ran and the pipeline inside it is over. `!` does not reach it either —
after `! false | true` the record is `1 0` and `$?` is 1, so the record is
taken before the inversion.

Two things about it are axes:

- **a bare assignment counts as a command.** bash says yes, so
  `false | true; x=1` leaves one element holding 0; zsh says no and leaves
  the pipeline's two. Every other shape of command updates it in both, so
  the question is asked only for an assignment with no command name.
- **`unset` is permanent.** In zsh the name never fills again; in bash the
  producer outlives it and the next pipeline fills it. This is the *opposite*
  of what a produced scalar does — `unset RANDOM` leaves an ordinary empty
  name in every shell measured — which is why it is asked here rather than
  inherited from the rule `Dynamic` already follows.

### Two bugs it uncovered, both about arrays and neither about pipelines

Writing the cases for it found two things that had been wrong since arrays
were added, and that nothing in the corpus had touched:

**A plain `$a` on an array is not the first element everywhere.** zsh gives
every element joined by a space where bash and ksh93 give the first alone.
It is now an axis, and it is asked lazily — only when a scalar is actually
read, and only when there is more than one element. Asking it in `setArray`
instead made *building* an array require a dialect, which is a question the
input does not depend on: `a=(x y z)` says nothing about how `$a` would be
flattened.

**dash rejects a subscript, and accepted one here.** `${a[@]}` is a bad
substitution in dash whether or not `a` was ever an array, and ours read it
as an array of one. It is a grammar flag now, and separate from the one that
enables `a=(x y)`: the two halves are separately reachable, since a subscript
can be written for a variable that was never an array — which is exactly the
case that was wrong.

## One rule, and the half of it that gives it away

`{ echo hi }` runs in zsh and is a syntax error in the other three. The
obvious reading is that zsh's brace groups do not need a terminator, and
that reading is wrong in a way that matters for where the rule goes.

The other half is `echo }`, which prints a brace in three shells and is a
**parse error** in zsh. So the rule is not about brace groups at all: in zsh
`}` is reserved wherever a word may stand, not only where a command may
begin. A `}` cannot be an argument there, so the only thing it can be doing
after `echo hi` is closing the group — and after `echo` on its own it is
closing nothing.

Modelled as `CloseBraceAlwaysReserved`, a grammar flag, it gets both halves.
Modelled as "brace groups may omit their terminator" it would have got the
first and left `echo }` printing a brace. This one was in the axes table as
measured and had never been built: the parser rejected `{ echo hi }`, which
is a valid zsh script.

## A reserved word inside a brace group

`{ echo a; do :; done; }` fails everywhere, and every shell in the panel
names the word it stopped on. Ours said `expected } — a brace group needs a
terminator before it`, which named neither the token nor any of the four
ways the shells say it, so it now goes through the ordinary
unexpected-token failure and each dialect words it.

dash alone also says what it expected, and only sometimes:

    { echo a; esac; }  →  "esac" unexpected (expecting "}")
    { esac; }          →  "esac" unexpected

The expectation rides along when the group had something in it and not when
it was empty — an empty group has nothing to be in the middle of.

## The front end was not using the shared answer

Where an unterminated construct is reported is a dialect's answer: bash puts
it on the line *after* the input's last when the text does not end in one,
and the other three on the last line itself. `Diagnostics` knew that, and
`eval` and `.` asked it. The front end read the error's own position
instead, so the same failure came out on different lines depending on which
route reached it.

This is the bug `driver` exists to prevent, one layer down: sharing a front
end is no help if the front end does not use the shared answer. The line is
now `ParseFailureLine`, exported beside `ParseFailure` for the same reason —
it is as much a part of the dialect's answer as the wording — and a test
asserts the two routes agree.

## What the conformance number does not cover

`make conformance` skips every case marked `SyntaxError`, alongside the ones
whose reference races. The two exclusions are not alike: a racing reference
cannot grade anything, but a syntax error is perfectly deterministic and is
exactly what the `Diagnostics` vector exists for.

The effect is that parse-error wording — a whole vector — is ungraded, and a
report of 100% is silent about it. Measured by including those cases:

    bash   332/337     zsh  335/337
    ksh93  335/337     dash 337/337

Five gaps in bash, two each in zsh and ksh93, none in dash. The bash five
are one thing: it names the input in the location (`bash: -c: line 1:`) and
echoes the offending source line after an unexpected-token error, and we do
neither. None of that is fixed here; it is written down so the number is not
read as coverage it does not have.

## Closing the hole in the number

`make conformance` used to skip every case marked `SyntaxError`, alongside
the ones whose reference races. The two exclusions were not alike. A racing
reference cannot grade anything — the score would move without the
implementation having changed. A *rejection*, though, is as deterministic as
an acceptance, and its wording is exactly what the `Diagnostics` vector
exists for. Skipping those cases left a whole vector ungraded and a report of
100% silent about it.

Grading them found nine gaps behind a number that read as perfect. Seven
were bash's and were two rules:

**bash names where the script came from, for a parse failure only.** It
writes `bash: -c: line 1:` when it read the script from `-c`, and plain
`bash: line 1:` for a *runtime* diagnostic on the same input — so
`echo ${x!}` carries no `-c`. A script names itself instead, and standard
input names neither.

**bash echoes the offending source line** after the message, as a second
line of the same shape:

    bash: -c: line 1: syntax error near unexpected token `fi'
    bash: -c: line 1: `{ fi; }'

Only after a token the grammar did not want. Input that simply ran out gets
no echo — there is no offending line to point at.

### A failure one shell finds later than we do

Adding the origin to the location immediately broke a case that had been
passing, and the reason is worth keeping. `for 1x in a; do :; done` is a
parse failure here and a *runtime* one in bash: bash parses it and complains
when it reaches it, so the complaint carries the status of a failed command
and none of a parse failure's decoration.

`Diagnostics` already knew this — `ForNameStatus` existed to give that
failure its own status — but the knowledge was spelled out only for the
status. Now one predicate answers it, and the status, the named origin and
the echoed line all follow from it, so there is one list rather than three
that could drift.

The whole diagnostic is built in one call, `ParseDiagnostic`, for the same
reason the wording lives in `interp`: the three decisions are not
independent, they all turn on the same error, and `eval` and `.` must say
the same thing about the same failure as the front end does.

### What is left

    bash   340/341     zsh  339/341
    ksh93  339/341     dash 341/341

Two cases remain and both are features rather than wordings: `ArithFloat`,
which ksh93 and zsh answer yes and which is measured and not built, and
extended patterns like `@(abc|xyz)`, which ksh93 matches, zsh parses without
matching, and bash and dash reject.

## Floating point, and the bug under it

ksh93 and zsh evaluate arithmetic in floating point; bash and dash do not.
The axis had been recorded in two places at once — a `Dialect.ArithFloat`
and a `Semantics.ArithFloat`, neither of them read by anything — which is
one question with two answers, exactly what the vectors exist to prevent.

It is the **dialect's**, because it decides what parses: `1.5` is one
literal where the shell has floats and, where it does not, a `1` followed by
text that could not be an operator. That is why bash blames the `.5` rather
than the `1.5` it was part of. The interpreter reads the same flag for the
half the parser cannot answer — a float arriving in a *variable* — so there
is one field consulted in two places rather than two fields that could
disagree.

An expression is integer until a float enters it. `3/2` is 1 in all four
shells and `3.0/2` is 1.5 in the two with floats, so values carry which kind
they are and each operation promotes rather than everything being a float
from the start.

Three things about it diverge, and one does not:

- **precision.** Fifteen significant digits in ksh93 and seventeen in zsh,
  so `0.1+0.2` is `0.3` in one and `0.30000000000000004` in the other from
  identical arithmetic.
- **whether a whole float keeps its point.** zsh writes `4.` where ksh93
  writes `4`, which is what keeps a float visible as one.
- **what an integer-only operator does with a float.** ksh93 refuses;
  zsh truncates for the bitwise operators and takes a *floating* remainder,
  so `7%2.5` is `2.` there and an error in ksh93. A remainder is not the
  same question as a bitwise and, which the implementation had to learn: the
  first attempt asked only about the left operand, and `7%2.5` has a whole
  number there.
- **a comparison does not.** `1.5 < 2` is `1` and not `1.` in either, so
  comparisons answer an integer whatever they compared.

Dividing a float by zero is an infinity rather than the error integer
division gives — there is no integer to hand back — and each shell spells
the infinity and the NaN its own way.

### A malformed expression is a failed command, not a failed parse

bash and ksh93 report `1` for `$((1.5))`, the status of a command that
failed, because they find the failure while *expanding*. We find it while
parsing, so without saying otherwise it would carry the status of a syntax
error and the decoration one gets. It goes through the same
`runtimeRefusal` predicate `for` with a bad name already used, which is what
keeps that one list rather than several.

bash also words two leftovers apart: `arithmetic syntax error in expression`
when an operand stands where an operator belonged, and `invalid arithmetic
operator` when the text could be neither. The parser distinguishes them,
because it is a question about what the text could have been. And a third
case is neither: `$((.5))` without floats is a missing *operand*, which both
shells without floats word as such — that site had been reporting a leftover
operator.

### The bug the feature uncovered

`echo $((2-7))` printed an empty line. In every dialect.

The integer writer was hand-rolled and looped while `n > 0`, so a negative
number produced no digits at all and every negative arithmetic result
expanded to nothing — silently, with status 0. Nothing in the corpus had a
negative result in it, and no *other* caller of that writer could ever pass
one: the lengths, exit statuses and process ids it also serves are never
below zero. It went unnoticed because the one caller that could reach it was
the one nothing tested.

It was found by writing a test for `~1.5`, whose expected answer is `-2`.

## The same text, read by two different rules

`@(abc|xyz)` matches `abc` in ksh93 and matches `@abc` in zsh. Both parse it,
neither is wrong, and they are not reading the same thing:

- **ksh93** has *extended patterns*: a quantifier — `@`, `?`, `+`, `*`, `!` —
  in front of a group. `@(abc|xyz)` is "exactly one of these".
- **zsh** has *alternation*: a bare group anywhere inside a pattern word, so
  `a(b|c)` matches `ab`. The `@` in `@(abc|xyz)` is then just a literal `@`,
  and the group follows it.

So the two are separate flags rather than one, and a shell can have either
without the other. Reading them as one feature would have made `@abc` match
in ksh93 and `abc` match in zsh, which is wrong in both.

**bash has the first, and only inside `[[ ]]`.** `[[ abc == @(abc|xyz) ]]`
matches while `case abc in @(abc|xyz))` is a syntax error, so *where* a group
is allowed is a separate question from whether the shell has one, and it is
its own field. The lexer is what has to know, because whether `(` ends the
word is settled before any parser sees a token — so the parser tells it when
a condition opens and when it closes.

### Two things a bare group must not swallow

Allowing `(` inside a word is a wide rule, and it broke two narrower ones
before the exceptions were found. Both are measured against the one shell
that has bare groups, and both are that shell's own answers:

    f() { echo hi; }     an empty `()` is a function definition. That shell
                         rejects `a()` as a pattern outright, so nothing is
                         lost by leaving an empty group alone.

    a=(x y)              a `(` straight after `=` opens an array literal,
                         never a group: `a=(b|c)` is a parse error there
                         rather than a pattern, so the assignment always wins.

The first was found by the dialect's own prelude failing to parse. The second
by five array cases in the corpus turning `(x y)` into a literal.

### And one a single pass cannot express

Whether an expansion's text is re-read for groups follows the axis that
already decides whether `p="a*"` globs, and three of the four leave the
parentheses alone:

    p="(b)"; case b in $p)      ksh93 matches; the others do not

ksh93 also matches the subject `(b)` against that same pattern, so it accepts
both readings of the text at once. Ours takes the group reading and therefore
answers the second half differently. It is one case, it needs the matcher to
try a pattern two ways, and it is written down rather than approximated.

## A shell runs what it has read

`echo one` on line 1 runs before line 3 fails to parse. Every shell in the
panel does this for a script, and it is the difference between a shell and a
compiler: a script that ends badly still does what its good lines said.

The **line** is the unit, not the statement. With `echo one; { fi; }` on one
line, neither half runs — the whole line is parsed before any of it is. And
the unit stretches past a newline while a construct is open, or a multi-line
loop could never be read at all:

    echo one            runs
    for i in 1 2        one unit, all five lines
    do
      echo $i
    done
    { fi; }             fails here

The EXIT trap still fires afterwards, which is why the failure returns
through the teardown rather than around it. That is unanimous too.

One thing about it is an axis. **zsh reads a `-c` command string whole**
before running any of it, so `zsh -c 'echo one⏎{ fi; }'` prints nothing where
the other three print `one`. A *script* is read a line at a time in all four,
which is why the axis asks only about the command string.

This shaped the interfaces rather than just the front end. `Parser.NextLine`
returns one logical line, and `Parse` is now a loop over it, so nothing
depends on which a caller chose. `Runner.RunPart` runs a chunk and leaves the
shell open; `Runner.Finish` does the teardown once, however many chunks ran.

### A line named twice

Reading a script a line at a time made ksh93's parse failures visible for the
first time, and they were being prefixed with a line the message already
carried: `line 3: syntax error at line 3`. Its wording names the line itself,
so the location must not. Only the parse failure — a runtime diagnostic in a
script is still prefixed there, `line 2: nosuchcmd: not found`, which is why
this is not the script location simply being absent.

ksh93 sometimes prefixes a *different* line: the one it had reached rather
than the one that failed, as in `line 2: syntax error at line 3`. When it does
so is not derived here — six inputs did not settle it — and we do not track
"the line reached" to print it with. Recorded rather than guessed at.

## A leniency measured and deliberately not built

zsh accepts a `case` arm the other three reject:

    case a in (a)) echo y;; esac      zsh prints y; bash, ksh93 and dash
                                      all call it a syntax error

It looks like tolerance of a redundant `)`, and it is not. The leading `(`
is a *pattern group* — the same alternation described above — rather than the
optional paren a `case` arm may carry:

    case ab in (a|b)b)   matches. The pattern is the group `(a|b)` followed
                         by a literal `b`, which is `ab`.
    case "b)b" in (a|b)b)  does not match, so the `)` is not literal either.
    case a in (a|b)|c)   matches. `(a|b)` and `c` are two arm patterns, so
                         the `|` between them is the arm's and the one inside
                         the group is the group's.

But the ordinary form still works there too:

    case a in (a) echo y;; esac       zsh prints y

and under the group reading that arm never closes — the `(a)` is the whole
pattern and the `)` that would end the arm has been used up. So zsh accepts
*both* readings of a leading `(`, which needs the parser to try one and fall
back to the other.

**Not built.** Backtracking a case arm would mean re-reading tokens whose
lexing has side effects — a here-document body is consumed at the newline,
not at the operator — and it buys a form nobody writes: `(a))` is a typo that
one shell happens to forgive. The half that scripts do use, a group inside a
pattern, is implemented and tested.

A second thing shows here and is worth writing down beside it. `((` at the
start of a `case` arm is read as an arithmetic command, because that is what
`((` means wherever a command may begin. zsh reads `case a in ((a))` as the
arm's paren followed by the group `(a)`, so it prints y where we report an
arithmetic error. Telling the two apart needs the lexer to know it is in a
case arm, which is the same lookahead problem in a different place.

## A here-document body is not a word

`don't` came out as `dont`. No error, status 0 — the worst way to be wrong,
and it survived a corpus at 100% because nothing in the corpus had an
apostrophe in a here-document.

The body of an *unquoted* here-document was being run through the word lexer,
so shell quoting applied to it. It is not a word. It is closest to a
double-quoted string, and the difference is exactly the quote:

| in the body | means |
| --- | --- |
| `'` `"` | ordinary characters. There is nothing for a quote to quote. |
| `$name` `${ }` `$( )` `$(( ))` `` ` `` | expand |
| `\$` `` \` `` `\\` | the character, without the backslash |
| `\` before anything else | both characters, kept |
| `\` before a newline | the lines joined, with nothing between |
| `~` `*` | ordinary. No tilde expansion and no globbing. |

All four shells agree on every row, so none of it is an axis.

`syntax.HeredocSpans` scans it now — a sibling of the double-quote scanner
with the quote taken out of the escape set and no terminator to look for. The
one thing it must not reuse is the *word* path, which is what this replaced.

A quoted delimiter is a different question and was already right: that body
is literal throughout and never reaches the scanner.

### And a second bug hiding behind the first

With the quoting fixed, every backslash came out doubled. The span expander
marks a literal's metacharacters for the glob stage, and a here-document has
no glob stage — its text is input, not a pattern. The old code unescaped them
at the end and the replacement had to as well. It is the kind of thing that
only shows once the layer above it is correct.

### The lexer cannot read a here-document on its own

Adding a case with an apostrophe in a body broke `TestCorpusLexes`, which
lexes every snippet standalone. That test had been passing for here-documents
by luck: where a body's end is decided by a delimiter the *parser* registers,
so a lexer with no parser above it reads the body as ordinary words — the same
mistake as the one above, one layer down, and invisible while every recorded
body happened to be valid word syntax.

`Tokens` queues the here-document itself now, from the same two tokens the
parser uses. It is the only caller that needs to: the parser does its own
registering, and everything else goes through the parser.

## An expression is not known until it is expanded

`$(( $x$y ))` with `x=1+` and `y=2` is **3** in every shell in the panel. Not
because `$x` and `$y` are operands slotted into a tree, but because the text
inside `$(( ))` is substituted *first* and the result is then read as an
expression. Two halves of an operator can come from two variables.

That is a fact about ordering, and it decides where the work goes:

    parse time   an expression containing a `$` or a backtick has no tree.
                 Building one would be building it from text that is not the
                 program.
    run time     the text is expanded — parameters, command substitutions,
                 nested arithmetic — and *then* read.

So `parseArith` returns nil for such an expression and keeps the raw text
beside it, and the interpreter reads it when it evaluates. The same applies to
`(( ))` as a command and to each of the three parts of `for (( ; ; ))`, where
the condition and the step are re-read every time round because what they
expand to can change between iterations.

An expression with no `$` in it is untouched by this and is still parsed once,
which matters: `$(( x + 1 ))` resolves the *name* through the evaluator, under
the axis that decides whether a name-shaped value is evaluated again. The two
rules give different answers for a value that is not a number, and keeping
them apart is the point.

### What it fixed

`/usr/bin/man` — and `apropos`, `manpath`, `whatis` and `sampleproc`, all of
which use `$(( $# ))` or `$(( $2-2 ))`. Five of the seven parse failures
`make wild` reported on this machine were this one cause. It was not on the
list before the sweep existed; it was found by pointing the sweep at /usr/bin.

## Two spellings of an option, and only one of them is portable

`set -o noglob` means the same thing in all four shells. `set -f` is its short
spelling in three of them, and in zsh it is a different option entirely —
about startup files — which leaves globbing alone:

    set -f; echo *.txt      *.txt in bash, dash and ksh93
                            a.txt b.txt in zsh

So the long name needs no dialect and the short one does. The axis is asked
only where `-f` is written, which means a script using `set -o noglob` never
raises the question.

What the option switches off is the *filesystem* half and nothing else. A
pattern in a `case` arm or after `==` still matches under it, because that is
matching rather than expansion — the same split that keeps the pattern
matcher free of the rules that belong to pathname expansion.

### What it unblocked

`/usr/bin/man`. With `$` in arithmetic fixed it parsed, and then failed at
run time on `set -f` — which the sweep could not have told us, because the
sweep reads and never runs. `man -w ls`, `manpath`, `whatis` and `apropos`
all produce byte-identical output to bash now.

That is the second time a fix has revealed the *next* thing behind it, and
the argument for giving the sweep a run mode: a parse-only check finds what
cannot be read, and says nothing about what cannot be done.

## Two routes to the same element

`${a[1]}` is substituted into an expression before it is read. `a[1]` is read
as part of it. Same element, two routes, and both have to count from the same
base — the dialect's, which is 0 in bash and ksh93 and 1 in zsh, so the same
text is 4 in two shells and 3 in the third.

The subscript is an expression of its own, so `a[i+1]` works and the name in
it needs no `$` for the same reason the array's does not. Reading past the end
is zero, and so is a subscript on a name that was never an array — which is
the form a version check uses before it knows whether the shell set one:

    (( BASH_VERSINFO[0] < 3 ))     zero, not an error, where there is no such
                                   array

A subscript on an assignment's target belongs to the target, so `(( a[1] = 9 ))`
writes an element rather than evaluating one and discarding it.

Whether a dialect has subscripts at all is the flag that already admits
`${a[1]}` — one question asked in the two places that need it. Where it is
off, `a[0]` inside an expression is a name followed by text that cannot be an
operator, and dash reports it as exactly that.

### What it fixed

`bats`, the last of the two arithmetic causes the sweep found. The sweep is
now down to one failure: `brew`, which uses a regex group in `[[ =~ ]]`.
