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
| a zero-padded endpoint pads the range | *n/a* | yes | **no** | yes |
| a range step's sign is honored | *n/a* | no | **yes** | no |
| a negative range step reverses the walk | *n/a* | no | no | **yes** |
| arithmetic does floating point | no | no | **yes** | **yes** |
| significant digits in a float | *n/a* | *n/a* | 15 | **17** |
| a whole float keeps its point | *n/a* | *n/a* | no | **yes** |
| an integer-only operator on a float | *n/a* | *n/a* | **refused** | truncated |
| a malformed expression reports | 2 | **1** | **1** | 1 |
| quoting a `=~` regex makes it literal | *n/a* | **yes** | no | no |
| array index base | *n/a* | 0 | 0 | **1** |
| plain `$a` on an array | *n/a* | first element | first element | **all, joined** |
| pipeline-status name | *none* | `PIPESTATUS` | *none* | `pipestatus` |
| `=~` capture name | *n/a* | `BASH_REMATCH` | *none* | *none* |
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
| `time` keyword | **absent** | yes | yes | yes |
| `time` report layout | *n/a* | real/user/sys | real/user/sys | **one line per forked element** |
| `time` decimal places | *n/a* | **3** | 2 | 2 |
| `time -p` | *n/a* | POSIX format | POSIX format | **not read: a word of the pipeline** |
| bare `time` reports | *n/a* | a run of nothing | **shell user/sys, no real** | **shell and children lines** |
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
    time keyword     time true 2>&1 | wc -l          → 1 (external's report piped), 0, 0, 0
    time layout      time true 2>err; cat err        → n/a, \n+real/user/sys 0m0.000s, same 0m0.00s, NOTHING (no fork)
    time per element time /usr/bin/true 2>err        → n/a, real/user/sys, real/user/sys, "/usr/bin/true  0.00s user 0.00s system N% cpu 0.001 total"
    time -p          time -p /usr/bin/true           → n/a, "real 0.00\nuser 0.00\nsys 0.00", same, "command not found: -p" and the line still printed
    bare time        false; time; echo $?            → n/a, zeros report st=0, user/sys of the shell st=0, shell+children lines st=0

The `time` report always lands on the **shell's** stderr: the layout rows
above were measured with the redirection *outside* the construct
(`{ time true; } 2>err`), and zsh's per-element line carries the element
as written — `true 2>&1  0.00s user …` for `time true 2>&1 | wc -l`, one
line per element, because each element of a multi-element pipeline forks
there. An element that does not fork (a lone builtin, a `{ }` group even
around an external) reports nothing at all in zsh.
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

The groupings repeat across the axes above — `{zsh}`, `{dash,zsh}`,
`{ksh93,zsh}`, `{ksh93}`, `{bash}`, `{bash,ksh93}`, `{bash,zsh}`,
`{dash,ksh93}`, `{dash}` — and axes measured since keep landing on
groupings already in the table: the name-value one on `{dash, ksh93}`,
the octal-digit one on `{ksh93}`, and `${#@}` on `{dash}`, which had
appeared only as the modern-ksh reading of the `&>` axis.

No count of axes or groupings is kept here. The vector in
`interp/semantics.go` has grown far past this table, every count this
file carried went stale, and the argument never needed one: it needs the
*shape* of the data, which has not changed.

A later axis, the exit status of a fatal error, was found by
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

Two more axes were both already known and both
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

## A divergence only the output reveals

zsh writes to *every* redirection target where the others write only to the
last:

    echo x >a >b        dash, bash, ksh93 → b only
                        zsh              → both

Nothing is reported either way, so it is the `&>` shape again. It is
recorded as `redir/multios-is-zsh-only` and implemented as the
`RedirectsWriteToEveryTarget` axis, asked only when one stream is given
several targets — so no script that redirects the ordinary way pays for
zsh's feature. An earlier revision of this paragraph called it deliberately
unbuilt, and the paragraph outlived the decision: the failure mode this
file records about the interpreter's comments applies to its own.

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
recorded in the corpus as `pat/unterminated-bracket` and answered by the
`UnterminatedBracket` axis, whose `BracketPolicy` type carries the three
named answers — the non-binary shape this section describes rather than
another bool, and the subject of the section below on what writing it
showed.

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
any of the genuinely binary axes and still compile, and the
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

The refusal rule has one deliberate exception, recorded here so the
invariant stays checkable against the code. `PrintfOutputPrecedesComplaint`
is read directly rather than asked — `printfWritesThrough` is the one read
of the vector that does not go through `ask` — because the axis decides
only the order in which output and complaint arrive where both streams
meet, nothing else a script can observe, and refusing every `printf` under
the core for want of an answer would cost far more than the majority's
order. Unspecified therefore quietly means No there: the one axis whose
non-answer is not a refusal.

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

## A probe the corpus cannot hold

Rule 1's probes normally live in the corpus, where `make oracle` re-runs
them against the live panel. One cannot: whether `ulimit -f` counts in
POSIX's 512-byte blocks or in 1024-byte ones is invisible until something
writes past the limit, and the kernel answers that with SIGXFSZ. The
probe, measured and re-measured (macOS, 2026-09-04):

    ulimit -f 1
    head -c  600 /dev/zero > f    bash writes all 600; dash, ksh93 and zsh
                                  stop the file at 512
    head -c 1200 /dev/zero > g    bash stops it at 1024

So bash's block is 1024 bytes and the others keep POSIX's 512, which is
the `UlimitBlockIsKilobyte` preset: yes in bash alone. It is recorded as
prose rather than as a case because the output cannot be golden: bash and
ksh93 announce the killed writer by its process id, which is different on
every run.

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

## Listing a declaration back: `-p`

Measured 2026-09-04, macOS arm64: bash 5.3.15, zsh 5.9.2, ksh93u+
2012-08-01, via `typeset -p` and `declare -p` on the same states. `-p` is
how scripts serialize state — the listing is meant to be input the same
shell could read again — and the three shells that have it produce three
different texts for identical state. dash has neither name, so `typeset`
is "not found" there and no question arises.

The same scalar, attribute and array states, side by side:

    state                     bash                          ksh93                     zsh
    v=1                       declare -- v="1"              v=1                       typeset v=1
    v='a b'                   declare -- v="a b"            v='a b'                   typeset v='a b'
    export e=E                declare -x e="E"              typeset -x e=E            export e=E
    readonly r=R              declare -r r="R"              typeset -r r=R            typeset -r r=R
    typeset -i n=5+2          declare -i n="7"              typeset -i n=7            typeset -i n=7
    export n; -i; -r; n=5     declare -irx n="5"            typeset -x -r -i n=5      export -ir n=5
    export v (unset)          declare -x v                  typeset -x v              export v=''
    arr=(x y)                 declare -a arr=([0]="x" [1]="y")   typeset -a arr=(x y)      typeset -a arr=( x y )
    arr=(x); arr[3]=z         declare -a arr=([0]="x" [3]="z")   typeset -a arr=([0]=x [3]=z)   typeset -a arr=( x '' z )
    typeset -A m; m[k]=v      declare -A m=([k]="v" )       typeset -A m=([k]=v)      typeset -A m=( [k]=v )
    typeset -A m (empty)      declare -A m                  typeset -A m=()           typeset -A m=( )

Three forms, not one format with options:

- **bash** opens every line with the word `declare` — even when invoked
  as `typeset` — then one clustered flag word, with `--` standing where
  there is no attribute. Flags cluster in the order `a A i r x`. Values
  are always double-quoted, with `\`, `` ` ``, `$` and `"` escaped, and
  `$'...'` replaces the double quotes when the value holds a control
  character. Array elements always carry their subscript; each element of
  an associative listing is followed by one space, so the text ends
  `"v" )`; an associative array with no elements prints no value at all.
  A key is bare when it is plain, double-quoted otherwise.
- **ksh93** writes `typeset` with each flag a word of its own, in the
  order `x r i` then the kind; a name with **no** attributes is a bare
  `v=1` with no command word. Values quote in ksh93's usual listing
  style (bare when plain, `$'...'` for quotes and control characters).
  Indexed elements carry subscripts only when the array has gaps; an
  empty associative array is `=()`. (An empty *indexed* array lists as
  `typeset -C arr=()` — compound-typing noise this spec deliberately
  does not follow; we keep `-a`.)
- **zsh** writes `typeset`, except an exported *scalar*, which is spelled
  `export` with the `x` dropped from the cluster (an exported array stays
  `typeset -ax`). Flags cluster as in bash; no `--` placeholder. Values
  quote in zsh's alias style (bare when plain, `'\''` for a quote,
  `$'...'` for control characters); keys in its trap style (never
  `$'...'`). Array values are wrapped `( x y )` with padding spaces and
  carry no subscripts — zsh's arrays are dense, so a gap is an empty
  element. An empty associative array is `( )`.

A named variable that does not exist: bash `declare: nosuch: not found`,
zsh `no such variable: nosuch` — both status 1, both after the builtin's
own prefix rules, and both keep going through the remaining names. ksh93
prints **nothing** for a missing name and exits 0, which is an axis, not
a wording.

With no names at all, all three list every variable, environment
included. bash and zsh sort the listing; ksh93's order was not
established. We sort.

Associative keys have **no** promised order: the same three assignments
list in three different orders across the panel, and bash's own order is
its hash table's. We print keys sorted, the same choice `${m[@]}` reading
already made, so a listing is deterministic; a corpus case must therefore
not depend on the panel's key order — use one key, or sort downstream.

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

### What that sweep left

    bash   340/341     zsh  339/341
    ksh93  339/341     dash 341/341

Two cases remained at that sweep and both were features rather than
wordings: `ArithFloat`, which ksh93 and zsh answer yes — since built, as
the next section records — and extended patterns like `@(abc|xyz)`, which
ksh93 matches, zsh parses without matching, and bash and dash reject.

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

## Two more letters the panel does not read alike

The same shape as `-f`, twice over, measured for the `set` option table.

`set -h` is an option letter in three shells and means no one thing. bash
abbreviates hashall with it and ksh93 trackall — the same idea, command
tracking, under two long names — while zsh's `-h` is histignoredups, a
history option that leaves command hashing alone. dash refuses the letter
outright, fatally, as it refuses any `set` letter it does not have:

    set -h; echo st=$?      st=0 in bash, ksh93 and zsh
                            set: Illegal option -h, exit 2, in dash

Two axes: whether the letter exists at all, and which option it abbreviates.
The long names raise no question — `set -o hashall`, `set -o trackall` and
`set -o histignoredups` each belong to the dialects that list them and to no
other.

`set -m` splits the panel three ways, and the split is about the terminal.
Measured with none — which is what a script has, and how the oracle runs:

    set -m; echo st=$?      st=0, silently, in bash and ksh93
                            set: can't access tty; job control turned off,
                              then st=0, in dash — a remark, not a failure,
                              and the option stays off
                            set: can't change option: -m, exit 1, fatally,
                              in zsh — and `set -o monitor` echoes the
                              spelling that asked: can't change option: monitor

With a terminal all four grant it. What granting means in a script is less
than it sounds: even with the option on, no shell in the panel announces a
background job to a script — the notices belong to a prompt — and background
jobs here already run in process groups of their own, which is the half of
the promise that is behavioral. So the axis is whether monitor *needs* the
terminal, and the two refusal shapes are the dialect's wording and status.

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

## A regular expression owns its parentheses

After `=~` the operand is a regular expression, so `(` and `)` are the
regex's and do not end the word. Where a word ends is settled by the lexer
before any parser sees a token, so the lexer has to be told it is reading one
— the parser sets that when it consumes the operator and clears it after the
operand, which is what keeps the rule from leaking: a `(` anywhere else still
opens a subshell.

A **group is taken whole**, balanced, with whatever is inside it. An
alternation in there belongs to the group in all three shells that have
`[[ ]]`, so it needs no dialect. A **bare** `|` outside a group does:

    [[ ab =~ a|b ]]     matches in bash and ksh93
                        parse error in zsh, which ends the word at the `|`

That is `RegexTakesAlternation`, and it is asked only for the bare form.

### Not fixed here

An unterminated or badly terminated `[[ ]]` is worded three different ways by
the three shells that have it — bash says "syntax error in conditional
expression" or "unexpected EOF", ksh93 names either `[[` or the token, zsh
names the token — and we say "expected ]]" to all of them. That gap predates
this change and is only visible through it in one case: where zsh refuses a
bare `|`, we refuse it too and say something else. The behaviour matches; the
wording does not.

## What `=~` captured

A successful match is worth more than its status: the whole match and every
parenthesized group are what the script matched *for*, and reading them back
afterwards is the idiom the operator serves. Every shell with `=~` keeps
them; none agrees with another about **where**. bash fills an array named
`BASH_REMATCH` — the whole match at 0, the groups after it. zsh fills a
scalar `MATCH` and an array `match` holding the groups alone, and leaves the
bash name unset. ksh93 keeps `.sh.match`, whose name is not even a plain
variable name, and leaves the bash name unset too (all measured, oracle run
2026-09-04: bash 5.3.15, zsh 5.9.2, ksh 93u+).

So the core records and a dialect names, the same seam as the
pipeline-status record. Only the bash-shaped array has a name here so far;
the other two shapes are different enough — a scalar plus a groups-only
array, a dotted name — that each would be its own seam when a dialect wants
it, not a second caller of this one.

The rest is measured on bash and pinned by the corpus and the interp tests:

- **A failed match empties the record** rather than leaving the capture
  before last — `declare -p` shows `BASH_REMATCH=()` after a miss, even a
  first miss, so an unchecked status reads nothing instead of stale groups.
- **An optional group that matched nothing is an empty element**, not a gap:
  `[[ abcd =~ b(x)?(c) ]]` gives three dense elements with `[1]` empty, so
  the group after it keeps its number.
- **The evaluation records, before `!` sees the result**: after
  `[[ ! ab =~ a ]]` the record holds `a` while `$?` is 1 — the same order
  the pipeline-status record follows.
- **It is an ordinary stored array, not a produced one**: a script can
  assign over it and read its own value back, and `unset` removes it until
  the next `=~` fills it again.
- A quoted operand made literal still records: `[[ abcd =~ "b" ]]` leaves
  `b` at element 0.

## The names, rather than a value

`${!prefix@}` and `${!prefix*}` yield the *names* of the variables beginning
with the prefix. bash and ksh93 have them; zsh and dash call the whole `${!…}`
family a bad substitution, which is the flag that already governs `${!x}` — so
this needed no new one.

The two spellings differ exactly as `$@` and `$*` do:

    "${!ZQ_@}"    one field per name
    "${!ZQ_*}"    one field, the names joined

The names come back **sorted**, which is not decoration: they are collected
from a map, which has no order to inherit, so without sorting the same script
would print them differently on different runs. They are gathered from
everywhere a lookup would find one — what the shell has set, what it produces,
and what it inherited — and a name `unset` took away is not listed, because it
cannot be read either.

Nothing matching is empty rather than an error, which is what lets a script
ask for a family of variables it may not have been given. That is the form
`brew` uses.

## The sweep is clear

    scripts found: 178   parsed: 175   refused by bash too: 3   failures: 0

Every shell script installed on this machine that is actually a shell script
now parses. The three not counted are a Tcl and two other programs whose first
line names a shell and whose second does not.

That is a floor rather than a finish: the sweep reads and never runs, so it
says nothing about what happens after a script parses — `set -f` was invisible
to it, and was only found because `man` parsed and then failed. A run mode is
the next thing this harness wants.

## `exit` was not one of the things a loop stops for

A loop asks, after each round, whether something has told it to stop. It
knew about `break`, `continue` and `return`. It did not know about `exit`,
and the consequences were not a missing feature but two wrong answers:

    g() { exit 3; }; while :; do g; done      exited **0**
    g() { exit 3; }; until false; do g; done  never returned

Neither looks like the same bug. The loop carried on; the next round found
the shell already refusing to run anything, so the condition produced
nothing; `while` read that as its condition having failed and set the status
to 0 on the way out, and `until` read the very same thing as its condition
still holding and went round again, forever.

A `for` over a list came out right, and that is why this lasted: the list
ends on its own, so the loop stopped without being told to. The shape most
scripts use is the one shape that hid it.

`exit` joins `return` in the answer now — stop, and do not clear it, because
the caller above has to see it too. `break` still counts its loops and
`continue` still starts the next round; the point is that an exit is neither.

It was found by `make wild-run`, in 43 of the 99 disagreements its first run
reported.

## Asking for a command without asking a function

`command name` runs the builtin or the external and never the function of that
name. That is the whole reason it exists: a function may wrap the thing it is
named after — `ls() { command ls --color "$@"; }` — without calling itself.

`command -v name` asks *what* would run rather than running it, and it is how
a portable script tests whether it has a tool. It is unanimous across the
panel in almost every respect: a builtin or a function is answered by the name
as written, an external by the path, and a word of the *grammar* is answered
too — `command -v if` prints `if` in all four, which is not obvious, since
`if` is not a command at all. Nothing found prints nothing, and the silence is
what makes `command -v x >/dev/null` the usual spelling.

The one divergence is the status when the answer is nothing: dash reports 127,
the status of a command that was looked for and run, where the other three
report a plain failure.

### `builtin` is not the same command everywhere

bash and zsh have a `builtin` that runs a builtin and only a builtin. **ksh93
has one of the same name that does something else**: it *registers* builtins,
so `builtin echo hi` there looks for a builtin called `hi` and says so. dash
has none at all.

So the name is a dialect's answer rather than part of the substrate, and
`dialect/ksh` carries ksh93's own meaning: no operands lists the table, and
an operand that is not already a builtin is `not found`, at 1, with the
builtin's own name as the whole prefix. What it does not do is *load*
anything — registering a compiled builtin is a feature with nothing behind
it here, so only the observable surface is built. An earlier revision of
this paragraph had the name taken away instead, and outlived that decision.

One detail of the refusal is worth recording. zsh names the speaking builtin
in a diagnostic's location — `zsh:cd:1:`, `zsh:shift:1:` — and does *not* here:
`zsh:1: no such builtin: ls`. The message is about a name that is not a
builtin, so there is no builtin speaking.

### What this does not close

The 30 disagreements `make wild-run` attributed to `builtin` are the
`/usr/bin` stubs, each of which runs `builtin <its own name>`: `alias`, `bg`,
`fg`, `jobs`, `fc`, `hash`, `type`, `ulimit`, `umask`, `unalias`. Eleven of
those are builtins this shell does not have, and most are interactive. The
error moves from "builtin: command not found" to naming the one that is
missing, which is a better answer to the same unfinished question.

## A bundle of option letters is one word and several options

`read -ra arr` means `read -r -a arr` in every shell there is. The rule is
POSIX's own — the Utility Syntax Guidelines say option letters group behind
one dash — and it is unanimous, which makes it the substrate's answer rather
than an axis. Measured, with `read` because its `-r` is the letter every
shell shares:

    printf 'x\\\ny\n' | $sh -c 'read -rr v; echo "[$v]"'
    dash bash bash3.2 ksh93 zsh    → [x\]    (status 0)

Two `r`s in one word, and all five read the bundle letter by letter and
honor the raw flag twice. dash proves the split from the other side:
`read -rp v` there is `arg count` — it split `-rp`, took `p`, and came up a
variable short — where a shell reading whole words would have refused the
word itself.

This shell refused the bundle wholesale: `read -ra arr` was `-ra is not
implemented yet`, one unknown word, even though `-r` is implemented and `-a`
is the only missing half. `set -euo pipefail` worked all along because `set`
has an option reader of its own, which is what said the splitting existed
and was not shared. Found in the wild — Homebrew's `rustc_wrapper` shim uses
`read -ra` (#347).

### A letter that takes an argument takes the rest of the word, or the next one

Both spellings are the same option everywhere, which `getopts` already
records for programs and holds for builtins too:

    printf 'a:b c\n' | bash -c 'read -rd : v; echo "[$v]"'   → [a]
    printf 'a:b c\n' | bash -c 'read -rd: v; echo "[$v]"'    → [a]
    printf 'x\n'     | dash -c 'read -pfoo v; echo "[$v]"'   → [x]
    printf 'q r\n'   | ksh  -c 'read -rd" " v; echo "[$v]"'  → [q]

So an argument-taking letter ends its bundle: what follows it in the word is
the argument, and when nothing follows, the next word is. An argument that
never arrives — the bundle ends the argument list — is refused in all four,
the way a bad option is: the same status (2, except zsh's 1) and, in the two
shells that follow a bad option with a usage line, the same usage line here.
Measured with `read -d` and `read -p` (2026-09-04):

    bash: read: -d: option requires an argument   (status 2, then usage)
    dash: read: No arg for -p option              (status 2; -d is not dash's)
    ksh:  read: -d: delim argument expected       (status 2, then usage)
    zsh:  argument expected: -d                   (status 1)

`Diagnostics.OptionNeedsArgument` carries the sentence — dash's and zsh's
are modeled, bash's is the fallback the substrate already said — and the
status rides `BuiltinBadOptionStatus`, which is the measurement: no shell
gives the two refusals different numbers. ksh93 names the argument it
wanted per letter (`delim` above), which one string per dialect does not
carry; it keeps the fallback.

### The complaint names the letter, not the bundle

A bad letter in a bundle is named alone, in all four — the shells walk the
word and stop at the letter they cannot use, so the bundle it rode in on is
not in the message:

    $sh -c 'read -rx v </dev/null; echo "st=$?"; echo after'
    dash   dash: 1: read: Illegal option -x            st=2  after
    bash   bash: line 1: read: -x: invalid option      st=2  after  (usage between)
    ksh93  ksh: read: -x: unknown option               st=2  after  (usage between)
    zsh    zsh:read:1: bad option: -x                  st=1  after

All four then carry on — `read` is not a special builtin, so nobody's
fatality rule reaches it.

This measurement sharpens `BadOptionNaming`. ksh93 was recorded as naming
the whole word from `export -Q`, where the word and the letter are the same
thing; the bundle tells them apart, and ksh93 names the letter — `-x` above,
and `-f` then `-Q` for `export -fQ`, one complaint per bad letter, a
divergence noted under `printf` and still not modeled. The whole word
survives only for a word that begins with `--`: `export --foo` is
`--foo: unknown option` there, against bash's `--: invalid option` and zsh's
`bad option: -o` — zsh skipped the dashes, took `f` as one of export's own
letters, and stopped on the `o` it does not know.

## read's options are the dialect's letters

`Semantics.ReadOptions`, in the getopts spelling — a `:` after a letter
whose argument follows it. Measured (oracle runs, 2026-09-04, bash 5.3 and
3.2 agreeing throughout except where 3.2 lacks a letter):

    bash   rsa:d:n:N:p:t:u:    plus -e -E -i, unimplemented here
    ksh93  rspAd:n:N:t:u:      plus -C -S -v and --version, unimplemented
    zsh    rsnpAd:t:u:         plus -e -E -k -q -z -c -l, unimplemented
    dash   rp:

Before any letter, the backslash. Without `-r` it removes the special
meaning of the character after it and is itself removed, and the four
shells agree on the whole matrix (oracle runs, 2026-09-04):

- An ordinary character keeps only itself: `a\tb` — a literal backslash,
  then a t — reads as `atb`.
- An escaped IFS character is data and does not split: `a\ b c` into
  `read x y` is `[a b][c]`, and with `IFS=:`, `a\:b:c` is `[a:b][c]`.
  An escaped *leading* space is not trimmed either — `\ a b` gives
  `[ a][b]` — so the escape has to reach the splitter, not vanish in a
  pre-pass that leaves an escaped space indistinguishable from a
  separating one.
- An escaped backslash is one literal backslash: `a\\b` reads as `a\b`.
- A backslash-newline vanishes whole — the line continuation.
- A backslash the input ends on escapes nothing and is dropped: `a\`
  with no newline assigns `a`, status 1 as any unterminated line.

With `-r` every backslash is ordinary: it stays, and a backslashed
separator still splits (`a\ b c` is `[a\][b c]`).

The letters themselves diverge before the behaviors do:

- **The array.** bash's `-a` takes the array's name as the option's argument
  (`read -aarr` works) and leaves later operands untouched — `x=keep`
  survives `read -a arr x`. ksh93 and zsh spell it `-A` with no argument:
  the first operand names the array, and ksh93 clears the names after it
  (`x=` after `read -A arr x`), where zsh refuses a second one outright
  (not modeled). Fields replace the whole array in all three.
- **The delimiter.** `-d :` stops the read at the colon and makes the
  newline ordinary input; only the argument's first character speaks
  (`-d xy` stops at `x`). An empty argument means NUL in bash and zsh;
  ksh93 reads through a NUL instead (not modeled — it takes a NUL in the
  input to see). Without -r, bash keeps an escaped delimiter as data
  (`a\:b` to `:` is `a:b`) and still folds a backslash-newline away;
  ksh93 and zsh fold the escaped delimiter pair away entirely and keep a
  backslash-newline (both not modeled — the substrate follows bash here).
- **The counts.** `-n N` reads at most N characters, the delimiter still
  ending it early, and the text splits as any read's does. Without -r the
  count is of characters as *delivered* in bash — `a\tbcd` under `-n 3` is
  `atb`, four raw characters read — where ksh93 counts raw bytes and keeps
  the backslash (`a\t`, not modeled — the substrate follows bash here). `-N N` reads
  exactly N: delimiter ordinary, backslash ordinary, the text handed to
  the first name whole and the rest cleared. zsh has neither count: its
  `-N` is a bad option and its `-n` is a bare flag for completion widgets
  that changes nothing here — so a count after it is read *into*, which is
  why `read -n 3 v` leaves v empty there.
- **Ends short of the count.** `printf 'ab' | read -n 3 v` is st=1 in bash
  and st=0 in ksh93, both keeping `ab` — `ReadPartialCountSucceeds`.
  The same input under `-N 5` reports 1 in both, bash keeping `ab` and
  ksh93 assigning nothing — `ReadExactCountKeepsPartial`. Wholly empty
  input is st=1 with nothing assigned everywhere.
- **Silence.** `printf 'x\n' | read -s v` reads x, prints nothing and
  reports 0 in bash, ksh93 and zsh: away from a terminal `-s` is a no-op
  that must still parse. dash refuses it.
- **The prompt, or the coprocess.** `-p` is one letter with two arities
  (#422, measured 2026-09-04). In bash — 5.3 and 3.2 alike — and dash it
  takes a prompt as its argument, written to standard error with no
  newline, and only when the stream being read is a terminal: a pipe or a
  file gets no prompt and reads exactly as though the flag were absent,
  and the test follows `-u` — bash with a terminal on stdin and `-u 5` on
  a file prints nothing. In ksh93 and zsh the same letter is a bare flag
  naming the coprocess as the source; neither's coprocess construct
  (`|&`, zsh's `coproc`) is in this grammar, so the only reachable answer
  is the measured refusal — `read: no query process` in ksh93, `-p: no
  coprocess` in zsh, status 1 in both, and the variables left exactly as
  they were: the read failed before reaching any input, so the
  clear-on-EOF rule never fires. The optstring's shape carries the split
  the way it does for `-n`; the refusal's words are
  `Diagnostics.ReadNoCoprocess`. "Is a terminal" is the substrate's usual
  approximation — a character device, now excepting the null device,
  which every harness-fed child holds and bash measurably does not prompt
  through. A character device that is neither a terminal nor `/dev/null`
  (say `/dev/zero`) is taken for one; real shells ask isatty and are not
  fooled, a difference accepted knowingly.
- **The timeout.** `-t SECS`, fractions allowed; input already waiting is
  read as if the flag were absent. Expiry clears the variables and reports
  142 in bash (128 plus SIGALRM) and 1 in ksh93 and zsh —
  `Diagnostics.ReadTimeoutStatus`. bash's `-t 0` polls for waiting input
  without reading; the substrate reports the timeout there instead, a
  deferred difference. zsh's `-t` may stand alone as a poll; modeled as
  argument-taking. ksh93 reads a non-numeric timeout as none at all (not
  modeled).
- **The descriptor.** `-u FD` reads from the shell's own table — `exec
  5<file; read -u 5 v` — with 0 the standard input. A number nothing is
  open at is status 1 three ways: bash says `9: invalid file descriptor:
  Bad file descriptor`, ksh93 `bad file unit number [Bad file descriptor]`,
  zsh nothing at all — `Diagnostics.ReadBadFileDescriptor`, where an empty
  wording is the answer, not a gap.
- **A word where a number belongs** (`-n bogus`, `-t bogus`, `-u bogus`) is
  refused with one substrate wording and status 1; the panel words it per
  shell per letter (bash's `-n` case matches the substrate's), deferred.

## A write that failed is not a command that worked

`echo hi >&-` closes the descriptor before the builtin writes, so the write
fails with EBADF. Whether the *command* then fails is a disagreement, and
what is said about it is a wording. Measured (oracle run, 2026-09-04):

    echo hi >&-; echo $?
      bash   echo: write error: Bad file descriptor   then 1
      dash   echo: echo: I/O error                    then 1
      ksh93  (silent)                                 then 1
      zsh    (silent)                                 then 0

`printf x >&-`, `pwd >&-` and `type type >&-` answer the same way in bash
and dash, with the failing builtin's own name in the message — so the
wording is per dialect and takes the builtin as a verb, not one string per
builtin. `command echo hi >&-` and `builtin echo hi >&-` name `echo`, the
builtin that wrote, not the wrapper.

The failure is per command, never fatal: `echo a >&-; echo ok` prints `ok`
everywhere. A stream closed for good behaves the same, once per builtin
that writes — `exec >&-; echo a; echo b` complains twice in bash and dash —
and a group redirect is the same again: `{ echo a; echo b; } >&-` fails
each write and the group reports the last one. An external command needs
none of this: `/bin/echo hi >&-` fails in its own process, printing its own
`echo: fflush: Bad file descriptor`, in all four shells alike.

So the split is one axis — does a builtin whose output write failed report
1 — true in bash, dash and ksh93, false in zsh, and POSIX sides with the
three: `echo` and `printf` each promise a status greater than zero when "an
error occurred" (POSIX XCU, EXIT STATUS), and a write that went nowhere is
one. The message is a Diagnostics wording,
`BuiltinWriteError`, empty where nothing is said: ksh93 fails silently, and
zsh does not fail at all. zsh does print `write error: bad file descriptor`
— status still 0 — when the closed stream came from `exec >&-` or a group
redirect rather than from the simple command's own; that wording-without-
failure is measured, recorded here, and not reproduced.

Two corners deliberately not modeled beyond the record: ksh93's `pwd >&-`
reports 0 where its `echo hi >&-` reports 1, so its answer is per builtin
in a way one axis does not carry; and zsh's exec-closed message above. Both
are visible in the corpus if a case ever asks.


## Locale, decided as a policy

The shell has a locale, and locale-sensitive operations consult it — read
the way POSIX ranks the variables, `LC_ALL` over `LC_CTYPE` over `LANG`.
An explicit `C` or `POSIX` narrows "letter" to ASCII; any other value,
and no value at all, is Unicode-aware. The unset half is measured rather
than assumed: bash stripped of every locale variable still uppercases
`café` to `CAFÉ`, so unset is not C.

Decided here once rather than one operator at a time, which is what issue
#367 asked. Case conversion (`${x^^}` and family) follows it now; the
character classes in globs, `[[ a < b ]]` and glob collation follow the
same rule as each is brought to the measured behavior.

The corpus cannot catch this class of difference — `internal/oracle` pins
`LC_ALL=C` for every run so the record does not depend on the developer's
environment — so the pinning lives in unit tests that set the variables
per case.

## Builtins that belong to one shell

Issue #431, from the 2026-09-04 completeness sweep: each shell's own manual
makes a handful of builtins central that no other shell has, and until now
they were simply absent — not built, and not recorded as out of scope, which
is the state a sweep cannot tell from an oversight. This section is the
record; the measurements live beside each implementation and in the corpus
(`setopt/`, `emulate/`, `whence/`, `print/`, `caller/` cases).

What was built, all through the extension seam — registered builtins in each
`dialect/<shell>`, none of them known to `syntax/` or `interp/`:

- **zsh `setopt` / `unsetopt`** (dialect/zsh/setopt.go): zsh's option
  namespace — case-insensitive, underscores ignored, one `no` prefix
  negating — over a measured table that binds each name to the substrate's
  own `set -o` machinery or to a semantics axis (`shwordsplit`, `nomatch`,
  `ksharrays`). The table is an honest subset of zsh's ~180 names: a name it
  holds and cannot move is refused with zsh's own `can't change option`, and
  a name outside it is `no such option`.
- **zsh `emulate`** (dialect/zsh/emulate.go): `sh`/`ksh`/`zsh` switch the
  three measured axes above and reset the option table to the emulation's
  defaults, `-c` runs a string under the emulation and restores everything
  after, and a bare `emulate` names the mode.
- **ksh93 `whence`** (dialect/ksh/whence.go): bare, `-v`, `-p`, `-q` — the
  bare mode delegating to the same lookup `command -v` uses, `-v` to the
  core `type`, whose ksh wording was already `whence`'s, and aliases spoken
  for in the dialect because the core's lookup cannot see them.
- **ksh93 `print`** (dialect/ksh/print.go): the measured escape set with
  `\c` stopping everything, `-r`/`-e`/`-n`, `-u fd` through the runner's
  descriptor table, `-f` delegating to printf, `-s` consumed against a
  history this shell does not keep, and `-p` refused with ksh93's own
  `no query process` — the same shape `read -p` measured.
- **bash `caller`** (dialect/bash/caller.go): the stack the three
  `BASH_*` arrays already name, one step up, `NULL` and `main` where bash
  puts them.

Deliberately **not** built, so the next sweep counts each as scoped rather
than missing:

- zsh `zmodload`: there are no loadable modules here; the name would be a
  table of refusals.
- zsh `autoload` (and `fpath`): function-file loading is an interactive
  startup mechanism; a non-interactive core sources files by name.
- zsh `bindkey`, `vared`, `zle`: the line editor's, and the line editor's
  key handling is the front end's concern, not the interpreter's.
- zsh's own `print` and `whence`: zsh has both — shared ksh ancestry — with
  its own flags and wording (`bad file number: 9` where ksh93 brackets the
  errno; alias values unquoted). This round measured and built ksh93's; the
  zsh pair stays command-not-found, visible in the corpus's `print/` and
  `whence/` cases as the recorded difference.
- zsh `setopt` names beyond the table: accepting an option we do not honor
  would be a promise; the honest subset refuses the rest out loud.
- zsh `emulate -L`: function-local emulation needs a restore-on-return seam
  the runner does not have; refused out loud rather than silently made
  global. `emulate csh` records the mode and changes nothing it could —
  csh's differences are not modeled anywhere else either.
- ksh93 `print -v`/`-C` and `whence -a`/`-f`: value quoting, compound
  output, the all-resolutions walk and the function skip; each is refused
  as not implemented rather than unknown, which would be the worse answer.
- ksh93 `hist`: interactive history editing, and there is no history.
- bash `bind`: readline's, same reasoning as zsh's bindkey.
- bash `history` and `fc`'s editing half: no history file in a
  non-interactive core; `fc` itself already answers with its measured
  empty-history refusal.
- bash `help`: documentation for another implementation's builtins would be
  false advertising; scripts do not branch on it.
- bash `suspend`: sends the shell SIGSTOP, which is an interactive job
  under a job-control parent; a library must not stop its embedder.
- bash `dirs` and `disown`: claimed by the core-builtin sweep (#430)
  alongside the job-spec and directory-stack work they sit on, and recorded
  there rather than twice.

## The declaration long tail: letters, listings, and one letter with an axis inside it

Oracle runs, 2026-09-04, bash 5.3, dash, ksh93u+, zsh 5.9.2. The corpus
rows live under `declare/` and the `set/bare-set-*` and `command/capital-v-*`
ids.

`declare`/`typeset` and `local` read the dialect's letters, in the same
spelling `ReadOptions` uses — `Semantics.DeclareOptions` and
`Semantics.LocalOptions`:

    declare/typeset
      bash   aAfFgilprux   plus -I -n -t, unimplemented here
      zsh    aAfgilprux    plus floats, padding, ties…, unimplemented
      ksh93  aAilprux      plus -f -F -b -n…, unimplemented (see below)
      dash   —             no typeset at all
    local
      bash   aAgilprux     plus -f -F -I -n -t, unimplemented
      zsh    aAilprux
      dash   (none)        `local -r x` declares a name `-r`, then refuses
                           it: `local: -r: bad variable name`, fatal
      ksh93  —             no local at all

What the letters mean, where measured to agree, is implemented once:
`-f` says the functions back and `-F` names them (`declare -f name` per
function bare, the name alone once operands narrow it — and a float's
precision in zsh and ksh93, where the letter is refused as missing);
`-g` declares a global from inside a function; `-l`/`-u` fold what is
assigned to the name, the declaring assignment included. zsh stores the
raw text and folds on *expansion* instead — every read agrees with the
other two shells, and only its `typeset -p` betrays the difference by
listing the raw value, which is deliberately not modeled.

**`-g` has an axis inside it.** With no local in front of the name the
two shells that spell the letter agree: the global is written. With a
`local x` standing there, bash writes the global cell *past* it —
`in=in out=new` — and zsh assigns the local it can see — `in=new
out=out`. `Semantics.DeclareGlobalReachesPastALocal`, asked only where a
local shadows the name.

**ksh93's `typeset -f` prints the source text verbatim** — its own
two-space indentation, `echo two; echo three` still on one line. This
engine keeps a tree, not the text, so the letter is refused as
unimplemented rather than approximated. bash and zsh print from their
trees, each in its own arrangement (`dialect/bash/layout.go`,
`dialect/zsh/layout.go`); the header join differs too — bash gives the
opening brace a line of its own, zsh keeps it on the header's
(`Diagnostics.FunctionListingHeader`).

**typeset is one of ksh93's own special builtins**, so any of its
failures ends the script — a bad option included, usage lines and all:
`Semantics.TypesetBadOptionFatal`.

**A bare `local` writes three different things**
(`Semantics.BareLocalListing`): bash lists the running function's own
locals — the innermost scope only — as clustered declarations
(`declare -i n`, `declare -- x`); dash writes nothing and reports 0; zsh
lists its whole parameter table, special parameters and tied arrays
included, which is a fact about that engine rather than about the
script's variables and is refused as unimplemented.

**A bare `set` is the same shape one step wider**
(`Semantics.SetListing`, `Semantics.SetListingQuoting`): sorted
`name=value` lines, values spelled bare-until-needed with `'\''` in
bash, always-single-quoted with the quote doubled out in dash, and
`$'...'` in ksh93 — the shared listing vocabulary. bash alone follows
the variables with every defined function; zsh lists every parameter and
is refused as the bare `local` is. Arrays keep the shape the engine's
`declare -p` gave them, subscripted and double-quoted in bash, dense and
bare in ksh93.

**`command -V` is POSIX and unanimous in shape**: every shell answers
with its own `type` sentence, so the option costs one wording — the
complaint for a name that is nothing (`Diagnostics.CommandVNotFound`),
where bash and ksh93 blame `command` and dash and zsh keep the shell's
name off the line exactly as their `type` does. Status and prefix rule
are `type`'s own (`TypeNotFoundStatus`, `TypeNotFoundUnprefixed`).

### Out of scope, recorded rather than silent: namerefs

`declare -n` / `typeset -n` — a name that is a reference to another
name — is deferred, not missed. It is a second variable model: every
read and write through the nameref has to resolve to the target,
`unset -n` addresses the reference where `unset` addresses the target,
and ksh93's `nameref` inside functions interacts with its scope rule.
That is its own project with its own measurements. The letters sit in
each dialect's `UnimplementedOptionLetters`, so `declare -n ref=x` is
refused today as "not implemented yet" rather than misread as an
ordinary declaration. Filed as part of #430's scope decision.

## The job and lookup long tail: type's letters, job specs, wait -n, disown, ulimit -a, the directory stack

Oracle runs, 2026-09-04, bash 5.3, dash, ksh93u+, zsh 5.9.2. Corpus rows
under `jobspec/`, `disown/`, `type/`, `ulimit/` and `dirstack/`.

**`type`'s remaining letters** ride `Semantics.TypeOptions` (bash `afpPt`,
ksh93 and zsh `afp`, dash none), with three axes measured inside them:

- `-a` is unanimous in shape — the shell's own answer and then every PATH
  hit, in PATH order, duplicates and all. The file lines are worded
  `name is /path` by all three, *including* the engine whose plain `type`
  calls the same file a tracked alias.
- `-p` splits twice. ksh93 and zsh search PATH past the shell's own answer
  where bash prints nothing at all for a name the shell would answer
  itself (`TypePSearchesPathPastTheShell`); zsh words the hit and the miss
  the way its plain `type` does where bash and ksh93 print the bare path
  and meet a miss with silence and status 1
  (`TypePathAnswerIsASentence`). `-P` — always the PATH search, always
  bare — is bash's alone.
- `-f` leaves functions out of the search in bash and ksh93 — so
  `type -f ls` names /bin/ls with an `ls` function standing right there —
  and *prints* the function in zsh, definition only, no sentence
  (`TypeFSaysTheFunctionBack`).

**Job specs** resolve `%n`, `%%`/`%+`, `%-` everywhere, and `%name` /
`%?text` — by command prefix and substring — everywhere but dash, where
any spec that is not a number is a job that is not there
(`JobSpecsByName`). A name matching two jobs is refused as ambiguous by
bash and taken — the most recent match — by ksh93 and zsh
(`AmbiguousJobNameIsRefused`). `wait` accepts the specs in all four; a
spec that names nothing is bash's `no such job` at 127, dash's at 2,
zsh's at 127 — and ksh93's *silence at 0* (`WaitReportsAMissingJob`).
`kill %9` is its own complaint, not a malformed pid
(`Diagnostics.KillNoSuchJob`) — and real ksh93 dies of it, a segmentation
fault this engine deliberately does not reproduce.

**`wait -n`** is bash's: block until whichever job finishes first, report
its status, 127 in silence with no jobs at all
(`WaitNWaitsForTheNextJob`). dash refuses the option, ksh93 refuses it
with its usage line, and zsh reads it as a job named `-n`.

**`disown`** exists in three shells and splits on what letting go means:
bash and zsh take the job out of the table, ksh93 only shields it from a
HUP this engine never forwards and goes on listing it
(`DisownRemovesTheJob`). Bare `disown` with nothing to let go of is a
worded 1 in bash and zsh and a silent 1 in ksh93
(`Diagnostics.DisownNoCurrentJob`, empty meaning silence). Its sweeping
letters (-a, -h, -r) are unimplemented and refused by name.

**`ulimit -a`** is four tables that share nothing — labels, order, row
sets, units — so each is the dialect's data
(`Diagnostics.UlimitListing`): a row is a literal prefix and a value,
live from the limit or fixed where the row is not a resource limit at
all (pipe and socket buffers, the rows one engine lists as
`not supported`). The `-n` row reports what the Go runtime raised the
soft limit to, not what a child will get — driver/rlimit.go records why
that is not fixable. The locked-memory, resident-set and process-count
limits joined the driver's table by platform header numbers
(driver/rlimit_darwin.go, _linux.go); elsewhere they keep the honest
refusal.

**The directory stack** stays in the prelude — shell over `cd`, the
extension seam working as designed — and became a real stack: `pushd`
pushes and prints the stack (silently, in zsh), a bare `pushd` exchanges
the top entry with the current directory, `popd` pops, and `dirs` prints
everything on one line, current directory first, `$HOME` as `~`, read
from `$PWD` at print time so a plain `cd` never leaves it stale. Two
divergences are deliberate: the empty-stack refusals are bare sentences
(a shell function cannot reach the engine's location machinery), and
`DIRSTACK` holds only the pushed entries where bash's also mirrors the
current directory.

### Out of scope, recorded rather than silent: newgrp

`newgrp` — the one POSIX regular builtin still absent — replaces the
shell with a new one running under a different group id, prompting for a
password on the way. That is process image and credentials: exactly the
two things `interp` may never touch (see "The core is a library"), so an
implementation would be a `driver` hook wrapping setgid/exec, built for a
feature no measured script uses — the panel shells themselves disagree
only about how they fail it in a script. Deferred, with this paragraph
as the record; a shell embedding this engine that needs it can register
the builtin through the extension seam. Filed as part of #430's scope
decision.

## A `wait` a trapped signal cuts short

Oracle runs, 2026-09-04, on macOS: bash 5.3, bash 3.2, dash, ksh93u+,
zsh 5.9.2. Corpus rows `trap/wait-cut-short-by-a-signal`,
`trap/wait-for-a-job-cut-short-by-a-signal` and
`trap/wait-is-not-cut-short-by-an-ignored-signal`.

    trap 'echo T' USR1; (sleep 0.3; kill -USR1 $$) & wait; echo "st=$?"

`wait` is the only builtin that blocks long enough for a signal's
*timing* to be visible, and POSIX has one cut short by a trapped signal
return rather than resume, with a status above 128. The panel agrees
that it returns: measured against a background job outliving the signal
by three seconds, all five builds came back inside the signal's own
200ms rather than at the end of the job. The handler runs first and the
status arrives second — `T` then `st=`, unanimously — which is the
between-commands rule already recorded above, not a special case.

The status splits twice, and the two questions are separate:

- **Bare `wait`** reports what a command killed by that signal reports,
  so it rides `SignalDeathStatusIsTwoFiftySix` rather than a second copy
  of the arithmetic: 158 for USR1 in bash 3.2, bash 5.3, dash and zsh,
  and 286 in ksh93 — 256 + 30, exactly its own encoding for a command
  USR1 killed. Confirmed across INT (258) and USR2 (287) there, so it is
  the encoding and not a special case for one signal.
- **`wait` naming a job** — `wait $!` or `wait %1` — keeps that answer
  in four of the five and drops to a plain **1** in ksh93, which is
  `WaitForAJobFailsWhenInterrupted`. Same shell, two forms, two
  encodings; nothing about how the job is *spelled* changes it.

The signal number is the host's: USR1 is 30 on this machine and 10 on
Linux, so the recorded statuses are facts about the platform as much as
about the shells.

Two boundaries pin what the case is evidence about. An **ignored**
signal — `trap '' USR1` — has no handler to run, does not interrupt, and
leaves `wait` reporting 0 in all five, so the divergence is about a
*trapped* arrival rather than about an arrival. And a wait nobody
interrupted is untouched: bare `wait` still reports 0 however its jobs
exited, and `wait $!` still reports the job's own status.

An interrupted `wait` also does not consume its jobs: a second `wait`
still has them to wait for and still reports 0 once they finish,
measured the same way. `wait; check $?` is the supervisor loop this
whole behavior exists for, and it only works if both halves hold — the
signal reaches the status, and the jobs survive to be waited on again.
