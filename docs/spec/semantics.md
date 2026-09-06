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
| `$!` before any job is `0` | no | no | no | **yes** |
| `$!` before any job is unset for `set -u` | yes | yes | **no** | **no** |
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
| failed redirection on a special builtin | **fatal** | survives *(fatal under `set -o posix`)* | **fatal** | survives *(fatal under `emulate sh`)* |
| duplication target wider than one digit | **refused, fatal** | read as a number | read as a number | read as a number |
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

## Handing the death to the driver is not the same as dying

Measured 2026-09-05 against bash 5.3, bash 3.2, dash, ksh93 and zsh, and
against our four dialect binaries on macOS 15 and Linux. Corpus rows
`signal-death/*`.

Every shell in the panel dies *by* an untrapped fatal signal: HUP, INT,
ILL, TRAP, ABRT, EMT, FPE, BUS, SEGV, SYS, PIPE, ALRM, TERM, USR1, USR2,
XCPU, XFSZ, VTALRM and PROF all end the shell with `WIFSIGNALED` true and
that signal in the status, and none of them writes a byte first. The
default-ignored signals — CHLD, CONT, URG, WINCH, INFO — do nothing and
the script carries on, unanimously.

Ours did not, and the reason is worth writing down because it is a
property of the language this is written in rather than of shells. The Go
runtime installs a handler for nearly every signal at startup and decides
for itself what to do with one nothing is listening for. It classifies
them three ways, and only the first is what a shell wants:

| the runtime's class | signals | what it did |
| --- | --- | --- |
| killing | HUP, INT, TERM | ran the default action — correct |
| throwing | QUIT, ABRT, ILL, TRAP, SYS, and the fault signals | wrote a full goroutine dump to standard error and exited 2 |
| everything else | USR1, USR2, ALRM, PIPE, XCPU, XFSZ, VTALRM, PROF | **discarded it**, so the shell never died at all |

The third row was a hang, not a wrong answer: the shell had already
stopped the script and was parked waiting to be killed, and nothing was
going to kill it. It cost ten seconds of harness timeout per case.

`signal.Reset` does not fix either half. It undoes a previous `Notify` or
`Ignore`, and an untrapped signal was passed to neither, so there is
nothing to undo and the runtime's handler stays installed. The
disposition has to be put back to the kernel's default by asking the
kernel — `driver/die_darwin.go` and `driver/die_linux.go`, one small
platform file each — and only then raised. All nineteen signals above
then behave as the panel does, in under ten milliseconds and in silence.

Two general points fall out. The first is that **a hook that hands work
to another layer has not been tested until that layer has been watched
doing it**: `interp` was correct throughout, the seam was correct, and
the shell still would not die. The second is that recording *which*
signal killed a shell is what made any of this visible — a timeout and a
death were the same row until the oracle kept the signal, and one of
these cases was scoring as a match against zsh's real SIGUSR1 death while
our shell was hanging.

## The one fatal signal that is not fatal

    kill -QUIT $$; echo after

    bash 5.3  → after, exit 0        dash   → killed by SIGQUIT
    bash 3.2  → killed by SIGQUIT    ksh93  → killed by SIGQUIT
    zsh       → after, exit 0

bash 5.3 and zsh take SIGQUIT's default action away and put nothing in
its place; dash, ksh93 and bash 3.2 leave it alone. `Semantics.
QuitIgnoredWhenNotInteractive`.

Three things pin it down as an axis rather than an accident. It holds for
a signal sent from *another process*, so it is a disposition and not a
deferral. It disappears with `-i`: all five ignore an untrapped QUIT in an
interactive shell, which is unanimous and therefore not an axis — the
question is asked only of a shell that is not interactive. And a trap
overrides it everywhere, so this is about the absence of a handler.

The bash column is a reminder that a shell is a version as well as a
name: 3.2 and 5.3 disagree here under one word, which is why the corpus
records both builds and why `Semantics` fields are never named after a
shell.

Removing the handler again is a further question, and the two shells that
ignore QUIT answer it differently: after `trap 'x' QUIT; trap - QUIT`,
bash 5.3 still ignores the signal and zsh dies by it. Not modeled — POSIX
says `trap -` restores the disposition the shell *inherited*, which makes
both defensible, and nothing in the corpus asks yet.

## A subshell that kills the shell, and where the process went

    (kill -TERM $$; echo inner); echo outer

    bash 5.3  → inner, killed by SIGTERM     dash   → inner, killed by SIGTERM
    bash 3.2  → inner, killed by SIGTERM     ksh93  → nothing, killed by SIGTERM
    bash as sh → inner, killed by SIGTERM    zsh    → inner, killed by SIGTERM

`Semantics.SubshellRunsOnAfterSignalingTheShell` — bash yes · dash yes ·
ksh93 no · zsh yes, preset yes.

**The shell ends either way, and that half is unanimous.** All six are
killed by the signal and `outer` prints in none of them. What splits is
the one echo between the `kill` and the end of the subshell.

**It is not a race.** The same answer on twenty-five runs of each with the
machine under load, and a `sleep 0.3` between the kill and the echo does
not change it either — nor does a second echo: `(kill -TERM $$; echo one;
echo two)` prints both in the five and neither in ksh93.

### The reason is the opposite of the obvious one

The obvious reading is that ksh93 forks a real subshell, so `$$` names
the parent and the whole process goes. Measured, that is backwards.

Start a child inside the subshell and read its parent process id — with a
command after the subshell, so no shell can exec its last command in
place:

    echo "shell=$$"; ( /bin/sh -c 'echo "in=$PPID"'; : ); echo tail

    bash 5.3   in ≠ shell     dash   in ≠ shell
    bash 3.2   in ≠ shell     ksh93  in = shell
    bash as sh in ≠ shell     zsh    in ≠ shell

**bash, dash and zsh give the subshell a process of its own. ksh93 does
not.** So the five that keep going are the five that forked: the signal
was aimed at the parent, the child never received it, and it finished its
body while the parent died. ksh93 runs the subshell in the shell's own
process, so `kill -TERM $$` is a self-signal landing on the very process
that was about to run `echo inner`.

`$$` itself is not the difference. It is the parent's pid inside a
subshell in all six — `echo "$$"; (echo "$$")` prints the same number
twice everywhere — which is what POSIX requires and what makes `$$`
usable as a lock name.

### Only a subshell

The same signal at the top level, in a brace group, in a function body and
in a `while` body stops all six at once and prints nothing. A command
substitution is silent everywhere too, for its own reason: what the child
wrote went into the assignment rather than to the output. So there is no
question to ask anywhere but `( )`, and the axis is about being a separate
*process* rather than about being a separate scope.

### Why it is an axis here rather than a consequence

Nothing in this implementation forks for a subshell — `docs/spec/core.md`
and AGENTS.md both say so — which is structurally ksh93's arrangement. So
the majority answer is not something the architecture hands us; it is a
choice to behave like the shells that fork, and the minority answer is a
choice to behave like the one that does not. That is exactly what a
semantics field is for.

The preset says yes: POSIX has `( )` execute "in a subshell environment"
and describes that environment as a copy, which is the forking reading,
and it is five of the six.

Read with `ask` rather than read plainly, unlike the startup axes: this is
reached while the script is running, and only by a `kill` at the shell's
own pid from inside a subshell, so an unanswered preset refuses there and
nowhere else.


## The other fatal signal, which is fatal without being a death

    kill -HUP $$; echo after

    bash 5.3  → killed by SIGHUP, 129    dash   → killed by SIGHUP, 129
    bash 3.2  → killed by SIGHUP, 129    ksh93  → killed by SIGHUP, 129
    zsh       → exit 1

`Semantics.HangupIsAnOrderlyExit`. Nothing prints after it anywhere, so
the script stops in all five and the disagreement is about *how* it
stopped. The status is the first sign: 1 is not 128 plus anything.

Three further measurements say it is an exit rather than a differently
numbered death, and the second of them is the one that settles it.

- The shell's own caller sees an ordinary exit rather than a process
  killed by SIGHUP — so nothing was re-raised.
- The EXIT trap runs. `trap 'echo bye' EXIT; kill -TERM $$` prints
  nothing in zsh, because that shell answers *no* to
  `ExitTrapRunsOnSignalDeath`; `trap 'echo bye' EXIT; kill -HUP $$`
  prints `bye` in the same shell. Both can only be true if SIGHUP
  produced no death for the first question to be asked about.
- An `exit 5` inside that trap takes the status, exactly as it would
  after any other ending.

The status is a constant and not something carried over: `(exit 7); kill
-HUP $$` is still 1.

It is one signal, and the sweep is worth recording because it bounds the
family. Across the nineteen signals whose default action ends a process —
HUP, INT, QUIT, ILL, TRAP, ABRT, FPE, BUS, SEGV, SYS, PIPE, ALRM, TERM,
USR1, USR2, XCPU, XFSZ, VTALRM and PROF — the panel is unanimous on
seventeen. QUIT is one exception and HUP is the other; there is no third.

An external SIGHUP is answered the same way, so this is a disposition and
not something `kill` does on its way past, and a trap overrides it as it
overrides everything here.

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

## A third axis that is an ordering, and the boundary it is asked at

What a subshell sees of the jobs the shell around it started, with
`sleep 1 &` already running and commands after the probe so that nothing
is the last thing a script does:

    (jobs -p); echo T          ksh93 → the pid   bash, dash, zsh → nothing
    jobs -p | cat; echo T      bash, ksh93 → the pid   dash, zsh → nothing

Two rows and two different pairs, so no yes-or-no holds both. `Semantics.
SubshellJobTable` is a policy with three values: cleared everywhere (dash,
zsh), kept everywhere (ksh93), and kept where the subshell was made for a
simple command or a substitution while cleared where it was made for a
compound (bash).

The `; echo T` is load-bearing, and leaving it off is how the question
gets the wrong answer. `(jobs -p)` alone prints the pid in dash, because a
subshell that is the last thing a script does need not be a subshell at
all; with anything after it, dash prints nothing. A probe for a fact about
subshells has to make sure it has one.

The boundaries, measured one at a time:

| boundary | bash | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `( … )` | none | none | the job | none |
| a simple command as a pipeline element | the job | none | the job | none |
| a compound command as a pipeline element | none | none | the job | none |
| `$( … )` and `` ` ` `` | the job | none | the job | none |
| `<( … )` | the job | — | the job | none |
| a `&` job | none | none | none | none |

The last row is unanimous and is therefore *not* asked about: a background
job sees nothing anywhere, and an axis consulted where nothing disagrees is
an axis that can be answered wrongly.

Two rows are measured and deliberately not modeled. bash also clears the
table for a **function** and for **`eval`** used as a pipeline element,
both of which are simple commands as far as any syntax can tell — `f(){
jobs -p; }; f | cat` prints nothing there where `jobs -p | cat` prints the
pid. Modeling that would mean a boundary changing kind partway through
running the command it was made for, which is a worse thing to own than
two rows of a table.

This is the clearest case so far of the corollary that **where a real
shell relies on a process boundary, we have to reconstruct the boundary by
hand**. A fork gives the child whatever the parent's table was, or nothing,
because the shell decides on the far side of it. Our subshells are cloned
Runners in one process, so the table arrived by simply being copied along
with the variables — not a decision anyone made, and invisible until a
script piped `jobs` somewhere.

It is a different question from the one `trapContext` answers, and the two
split the same clones differently: `( … )` and `$( … )` are one boundary
for a trap listing and two for a job table. Two names rather than one with
two meanings.

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

## A probe the corpus could hold after all

Rule 1's probes live in the corpus, where `make oracle` re-runs them
against the live panel. One was written down here instead, as prose,
because its output was thought not to be goldenable — whether `ulimit -f`
counts in POSIX's 512-byte blocks or in 1024-byte ones is invisible until
something writes past the limit, the kernel answers that with SIGXFSZ,
and the shell that reaps the killed writer announces it with a **process
id**, which is different on every run.

The fact is right and the reason was the probe's rather than the panel's.
A process id only reaches the output if the announcement does, and the
announcement is written by the *parent* when it reaps — so it is caught
by a redirection on the group around the subshell rather than on the
subshell itself:

    { ( ulimit -f 1; printf "%0600d" 0 > f ); } 2>/dev/null
    ls -l f                       600 where a block is 1024
                                  512 where it is 512

That is `ulimit/the-file-size-block`, stable over repeated runs and now
re-measured on every `make oracle` like everything else. Re-measuring it
also widened the answer: the 1024-byte block is bash's **as `bash`**, and
the same bash 5.3 called `sh` uses POSIX's 512, alongside dash, ksh93 and
zsh. So `UlimitBlockIsKilobyte` is yes for the bash dialect and no for
every other preset including a POSIX one, which is what the argv[0]
column is in the panel to catch.

The general lesson is the one `oracle.md` states about contaminated
probes, in the other direction: a probe that cannot be recorded is worth
re-examining before its fact is copied into prose, because prose is where
`oracle-check` stops looking.

## A conversion the corpus cannot ask for twice

`printf '%(fmt)T'` writes an epoch through a date format, with the format
inside the conversion. Oracle runs, 2026-09-05, and it is **bash 5.3's
alone** in the panel: dash and zsh call `%(` a directive they do not have
(status 2 and 1, two wordings), bash 3.2 an invalid format character, and
ksh93 has a `%T` under the same letter that is not this one at all.

    printf '%(%Y)T\n' 1000000000        bash  2001
    printf '%()T\n'    1000000000       bash  01:46:40 — the C locale's
                                               time of day
    printf '[%12(%Y)T]\n' 1000000000    bash  [        2001] — the width
                                               is the result's, not the
                                               date's

The operand is seconds since the epoch, and **two numbers are not times**:
`-1` is now and `-2` is when the shell started. A missing operand is now as
well. That is why the corpus pins a *fixed* epoch and nothing else — a case
that asked for the current year would record the year it was recorded in —
and why the engine grew `Runner.Clock`: a hook, nil meaning the wall clock,
so the two forms the corpus cannot ask about are pinned in
`interp/printftime_test.go` instead. It is the same shape `RANDOM` has,
and for the same reason: a value the shell *produces* has to be sayable
from outside or nothing that depends on it can be tested twice.

The zone is **the Runner's `$TZ`**, not the process's. Measured: `TZ=UTC`
without an export changes the answer, so an exported-only lookup would be
wrong as well as ambient — the `PATH` rule, in a second place. An unset TZ
is the machine's zone; an empty one, and a name no zone database has, are
both UTC.

`strftime` is ours, written from the POSIX conversion specifications and
checked against a live shell in the C locale, because Go has none. Two
things about it are the platform's rather than the shell's, and are
recorded rather than pinned: a conversion no strftime has keeps its letter
and loses the `%` on the system this was measured on, and glibc has
letters this does not.

`ksh93`'s `%T` is deferred rather than missed. Its operand is a date
*string* — `now`, `tomorrow`, a date written out — and a number earns
`printf: warning: invalid argument of type T` and the time it is now.
Reading a date the way ksh93 reads one is its own feature with its own
grammar, so `PrintfTimeConversion` is No there and four corpus rows show
the ksh93 column diverging, which is the honest record of an unbuilt
feature rather than a silent one.

### The C length modifiers

A conversion may carry a C length modifier between its precision and its
verb, and the panel splits three ways over which ones. Measured with
`/bin/bash` 3.2.57, `/opt/homebrew/bin/bash` 5.3.15, `/bin/ksh` 93u+
2012-08-01, `/opt/homebrew/bin/zsh` 5.9.2 and `/bin/dash`:

    printf '[%ld]'  42        bash 42   ksh93 42   zsh 42   dash %l: invalid directive
    printf '[%Lf]'  1.5       bash ok   ksh93 ok   zsh ok   dash %L: invalid directive
    printf '[%lld]' 42        bash 42   ksh93 42   zsh %ll: invalid directive
    printf '[%zX]'  255       bash FF   ksh93 FF   zsh %z:  invalid directive
    printf '[%jd]'  42        bash 42   ksh93 42   zsh %j:  invalid directive
    printf '[%llld]' 42       bash 42   ksh93 42   zsh %ll: invalid directive
    printf '[%hld]' 42        bash 42   ksh93 42   zsh %hl: invalid directive

So: dash has none; zsh has `h`, `l` and `L` and exactly one of them,
which is C89's set — the refusals are precisely C99's additions; bash and
ksh93 have all eight and skip a *run* of the letters rather than a list
of spellings, which is why `%llld` and `%hld` are accepted.

Every one of them is read and thrown away. `printf '%hhd' 300` is 300 and
not 44 in all four that take it, and `printf '%lld'` with the largest
signed 64-bit value is that value — a shell's arithmetic is one width and
the modifier cannot change it. This is about what a format may *say* and
never about what it means, which is what makes one set-valued axis enough.

The diagnostic follows from the parse rather than being its own rule. A
conversion nobody has is named two ways, and only two:

    printf '%v]xY' 1     bash    `v': invalid format character
                         ksh93   v: unknown format specifier
                         zsh     %v: invalid directive
                         dash    %v: invalid directive

bash and ksh93 name the conversion character alone — neither names the
`]` after it — and zsh and dash name the whole directive as written. That
is why bash names `T` in `printf '%T'` and `]` in `printf '%z]'`: `z` is
a modifier there, so the conversion it could not read is the `]`. It was
recorded here as "the character *after* `z`", which was the wrong reading
of a right observation, and it survived because nothing in the corpus put
a tail after a bad conversion.

Two things ksh93 does here are **not** modeled, and neither belongs to
this axis:

- ksh93 will read a width *after* a modifier — `printf '%l5d' 42` is a
  padded 42 there and an error in bash and zsh. That belongs to the shape
  of its conversion prefix, below, and has nothing to do with modifiers:
  `printf '%5-d'` and `printf '%5 d'` work in ksh93 too, with no modifier
  in either of them.
- ksh93 appends a newline to any output holding a byte above the ASCII
  range, in a format and inside `$'…'` alike.

### ksh93's conversion prefix, and why it is not one axis

The note above once read that ksh93's prefix is **free-order** — that it
loops over flags, width and precision until it finds a verb, where the
rest of the panel walks the four parts once. That is half right, and the
half that is wrong is the half that would have to be implemented.

What holds. A flag after the width is read and acted on, and a width after
a length modifier likewise:

    printf '[%5-d]' 42    ksh93 [42   ]   bash `-': invalid format character
    printf '[%5 d]' 42    ksh93 [   42]   bash ` ': invalid format character
    printf '[%5+d]' 42    ksh93 [  +42]   bash `+': invalid format character
    printf '[%l5d]' 42    ksh93 [   42]   bash `5': invalid format character

What does not. The second `.` is **not** a second precision that a
free-order reader would fold into the first. It is ksh93's output *base*,
which is a conversion feature of its own and not a question about order:

    printf '[%..36d]'  1295   ksh93 [zz]     1295 is zz in base 36
    printf '[%..2d]'   5      ksh93 [101]    and 101 in base 2
    printf '[%.3.16d]' 255    ksh93 [0ff]    base 16, padded to a precision of 3
    printf '[%.0.8d]'  64     ksh93 [100]    base 8
    printf '[%5.2.3d]' 42     ksh93 [ 1120]  base 3, precision 2, width 5

`[ 1120]` was what made the prefix look like it was being read twice. It
is 42 written in base 3.

And the order is not free even among the parts that are reordered: the
same precision and the same flag give two different results depending on
which side of each other they are written.

    printf '[%-.3d]' 42   ksh93 [042]   the precision survives
    printf '[%.3-d]' 42   ksh93 [42]    written after it, the minus loses it

So "does this shell read the prefix in any order" is the wrong shape for
the question, the way "does this shell add 128" was the wrong shape for
the pipefail one. Implementing it needs ksh93's output base as a feature
and a rule for what a flag does to a precision already read, neither of
which is an axis over prefix order. It stays unmodeled, and now with the
measurement that says why rather than a claim that was never checked.

### A format that ends before its conversion character

`printf 'a%'` has no conversion character to complain about, and the panel
answers it four ways — none of which is the ordinary bad-conversion
complaint with an empty name:

    printf 'a%'    bash   a, and `%': missing format character     st=1
                   zsh    a, and %: invalid directive              st=1
                   dash   a, and missing format character          st=2
                   ksh93  a%                                       st=0
    printf 'a%5'   bash   `%5': missing format character
                   zsh    %5: invalid directive
                   dash   missing format character
                   ksh93  a%
    printf 'a%ll'  bash   `%ll': missing format character
                   zsh    %ll: invalid directive
                   dash   %l: invalid directive
                   ksh93  a%

Three things are in there, and each is modeled separately.

- bash has a **second wording**, `missing format character`, and it names
  the *whole directive* in it where its ordinary bad-conversion complaint
  names the conversion character alone. That is `PrintfMissingVerb`, a
  diagnostic beside `PrintfBadVerb` rather than the same one spelled with
  an empty name. It takes one verb, the directive, because there is no
  character in it to name.
- dash names nothing at all for `%`, `%5` and `%.` — its wording simply
  takes no verb — and reports 2 where the others report 1, which is
  `PrintfMissingVerbStatus`. Its `%ll` line is not this case: dash has no
  length modifiers, so nothing ran out there and the `l` is an ordinary
  conversion it could not read.
- ksh93 does not complain. It writes a bare `%` for the whole unfinished
  conversion — `a%5` and `a%ll` are both `a%`, so the prefix it scanned is
  dropped rather than written back — and reports success. That is
  `PrintfUnfinishedConversionIsAPercent`, asked only where a format
  actually ends inside a conversion.

Only the *last* conversion can be the unfinished one, so a doubled percent
earlier in the format is unrelated: `printf 'a%%b%'` writes `a%b` in all
four before any of this applies.

### The hexadecimal escape in a format

`printf` reads `\xHH` in its *format*, and the panel splits four ways over
how. Measured with `/bin/bash` 3.2.57, `/opt/homebrew/bin/bash` 5.3.15,
`/bin/ksh` 93u+ 2012-08-01, `/opt/homebrew/bin/zsh` 5.9.2 and `/bin/dash`,
reading the bytes with `od -An -tx1` rather than the display — which is
the only way to ask this question at all. The two bash builds agree
throughout, so they are one column here:

    printf 'a\x41Z'    bash 61 41 5a   ksh93 61 41 5a   zsh 61 41 5a
                       dash 61 5c 78 34 31 5a
    printf 'a\x80Z'    bash 61 80 5a   ksh93 61 80 5a   zsh 61 80 5a
                       dash 61 5c 78 38 30 5a
    printf 'a\x1Z'     bash 61 01 5a   ksh93 61 01 5a   zsh 61 01 5a
    printf '[\xff]'    bash 5b ff 5d   ksh93 5b ff 5d   zsh 5b ff 5d
    printf '[\x0ff]'   bash 5b 0f 66 5d   zsh 5b 0f 66 5d
                       ksh93 5b c3 bf 5d
    printf '[\x0041]'  bash 5b 00 34 31 5d   zsh 5b 00 34 31 5d
                       ksh93 5b 41 5d
    printf 'a\xZ'      bash 61 5c 78 5a and `printf: missing hex digit for \x`
                       ksh93 61 00 5a   zsh 61 00 5a
                       dash 61 5c 78 5a

Three separate details, and each splits the panel in a different place,
which is why `PrintfHexEscape` is one enumeration rather than a bool:

- **Whether the escape is there.** dash has no `\x`, so all six characters
  of `a\x41Z` come out as written. It is the sole holdout, as it is on
  most of this file.
- **How wide the digit run is, and what the value means.** bash and zsh
  stop at two digits and the value is a *byte*. ksh93 takes every digit
  that follows, and once there are more than two of them the value is a
  *code point* written in UTF-8 — which is why `\xff` is one byte there
  and `\x0ff` is two. The switch is on the number of digits and not on the
  value: `\x0041` is an `A` and `\x0080` is `c2 80`.
- **What an empty digit run means.** bash leaves `\x` standing and writes
  `printf: missing hex digit for \x` on standard error, with a status that
  is still 0 — a warning rather than a failure. ksh93 and zsh read the
  empty run as a zero and write a NUL.

One thing measured here is **not** modeled. ksh93's code point may run
past the last one there is, and it writes a nonstandard encoding for it:
`printf '[\x123456789abc]'` is `5b fd 96 9e 89 aa bc 5d`, six bytes for a
value forty times larger than U+10FFFF. This writes nothing for a run past
the last code point, which is what ksh93 itself does once the value stops
fitting at all — `printf '[\xffffffffffffffffffffff]'` is `5b 5d` there.
Every value Unicode has agrees.

The escape is a question about the **format**. A `%b` argument asks the
same four readings at its own site, through `PrintfBHexEscape`, and ksh93
answers the two differently: it reads `\x41` in a format and leaves it as
written in `printf '%b' 'a\x41Z'`, which is `61 5c 78 34 31 5a`. bash and
zsh have it at both sites and dash at neither, so ksh93 alone shows that
the site matters — and `PrintfHexEscape` is put to a format and never to a
`%b` argument. The next section is the rest of that table.

### The escapes a `%b` argument expands

`%b` and a format are two escape tables, not one table read twice, and
every shell in the panel means it. Measured with `/opt/homebrew/bin/bash`
5.3.15 (as `bash` and as `sh` — the corpus's `bash` and `bash-as-sh`),
`/bin/bash` 3.2.57 (`bash32`), `/bin/ksh` 93u+ 2012-08-01,
`/opt/homebrew/bin/zsh` 5.9.2 and `/bin/dash`, reading bytes with
`od -An -tx1` (macOS, 2026-09-05). All three bash columns agree throughout
except where noted, so they are one column here:

    printf '%b' 'a\0101Z'  bash 61 41 5a  ksh93 61 41 5a  zsh 61 41 5a
                           dash 61 41 5a
    printf 'a\0101Z'       all six  61 08 31 5a

The octal is the sharpest of these: a `%b` reads `\0` and up to three
octal digits **after** it, which is the XSI escape `echo` expands, where a
format reads up to three digits with the leading zero optional. So the same
four characters are an `A` at one site and a backspace and a `1` at the
other, unanimously, and reading a `%b` with the format's reader produced
neither answer — `61 08 31 5a` from a shell that should have written
`61 41 5a` (#798). Both sites truncate to a byte rather than encoding a
code point: `\0300` is `c0` and `\0400` is `00`.

A `%b` and an `echo` argument are close but are not the same table either,
and the panel says so twice over. bash 3.2 writes `61 1b 5a` for a `%b`
argument's `\e` and `61 5c 65 5a` for an `echo` argument's — that is
`/bin/bash` 3.2.57, the corpus's `bash32` column, and not `bash-as-sh`,
which is the 5.3 build under another name — so one shell answers the two
sites differently; and the bare octal below splits the two
sites for every bash. The four dialects modeled here happen to give the
same answer at both sites for `\e`, `\E` and `\x` — bash 3.2 is a version
and not a dialect — but they are separate questions and are asked
separately.

The rest of the table splits, and each split falls in a different place:

    printf '%b' 'a\101Z'   bash 61 41 5a   dash 61 41 5a
                           ksh93 61 5c 31 30 31 5a   zsh 61 5c 31 30 31 5a
    printf '%b' 'a\eZ'     bash 61 1b 5a   zsh 61 1b 5a
                           dash 61 5c 65 5a   ksh93 61 5c 65 5a
    printf '%b' 'a\EZ'     bash 61 1b 5a   ksh93 61 1b 5a
                           dash 61 5c 45 5a   zsh 61 5c 45 5a
    printf '%b' 'a\x41Z'   bash 61 41 5a   zsh 61 41 5a
                           dash 61 5c 78 34 31 5a   ksh93 61 5c 78 34 31 5a
    printf '%b' 'a\cbZ'    all six  61

- **Whether the octal needs its `\0`.** `PrintfBOctalWithoutZero`. bash
  and dash read a bare `\101`; ksh93 and zsh write the four characters.
  This is **not** `echo`'s answer at the other site: an `echo` argument's
  `\101` is `61 41 5a` in dash alone and `61 5c 31 30 31 5a` in the other
  five, `-e` or not. So the two sites needed two axes rather than one
  shared reading, and bash is the shell that separates them.
- **The two spellings of the escape character are two questions.**
  `PrintfBEscEscape` and `PrintfBCapitalEscEscape`. The two shells that
  split them split them in **opposite** directions: ksh93 has `\E` and not
  `\e`, zsh has `\e` and not `\E`. bash has both, dash neither. A single
  answer for both letters is wrong for half the panel, which is why one
  `Echo…` axis could not be reused for the pair. `echo`'s own site has the
  same asymmetry and was split the same way in #908 — the four dialects
  give the same pair of answers at both sites, and the two sites are still
  asked separately.
- **`\x`.** `PrintfBHexEscape`, the section above.
- **`\c` asks nothing.** All six end the output there, where the same two
  characters in a *format* are literal in bash and dash, control-X in ksh93
  and a full stop in zsh (`PrintfBackslashC`). The one entry where the
  format's table is the divided one and the `%b`'s is unanimous. What it
  stops is the whole `printf` and not the one conversion — the rest of the
  format is abandoned, the operands after it go unused, and the format is
  not run again: `printf '[%b][%s]' 'a\cb' x` is `[a` in all six.

Unanimous and asking nothing: the eight one-letter XSI escapes `\a \b \f
\n \r \t \v \\`, which are the same bytes at every site that expands
escapes at all; a backslash the argument ends on, which is a backslash; and
a backslash before a character no entry claims, which keeps its backslash —
`printf '%b' 'a\qZ'` is `61 5c 71 5a` everywhere.

Two things measured here and **not** modeled.

`\uHHHH` and `\UHHHHHHHH` are escapes in bash 5.3 and zsh at both sites,
and in ksh93's *format* only. `printf '%b' 'a\u0041Z'` is `61 41 5a` in
bash 5.3 — as `bash` and as `sh` alike, so this is a bash *version* and
not a posix-mode question — and in zsh, and it is the characters as
written in bash 3.2, dash and ksh93; `printf 'a\u0041Z'` moves ksh93 into
the first group. Nothing here decodes them at either site, which is a gap
the change for #798 left exactly where it found it.

One more divergence lives in the corner where the stop meets a conversion's
field, and it is `PrintfBStopIsPadded`. Five of the six put the text a `\c`
cut short through the field exactly as they would any other; ksh93 alone
writes it as it stands:

    printf '[%5b]'   'a\cb'    five  [    a      ksh93  [a
    printf '[%-5b]'  'a\cb'    five  [a          ksh93  [a
    printf '[%.1b]'  'ab\cc'   five  [a          ksh93  [ab
    printf '[%5.1b]' 'ab\cc'   five  [    a      ksh93  [ab

It is a property of the **stop** and not of the conversion: with nothing
stopping it ksh93 pads and truncates like the rest — `printf '[%5b]' 'ab'`
is `[   ab` in all six, and `printf '[%.1b]' 'abc'` is `[a` — which is the
control row `printf/a-b-escape-a-field-with-nothing-stopping-it` exists to
hold. Without it the axis reads as "ksh93 has no field for `%b`", which is
not what it does.

Asked only where a `\c` actually stopped a `%b` *and* the field would change
the text, so `printf '%b' 'a\cb'` and `printf '[%1b]' 'a\cb'` need no
dialect.

`echo`'s own `\e`/`\E` split is the same asymmetry the `%b` site has —
`echo -e 'a\EZ'` is `61 1b 5a` in ksh93 and `61 5c 45 5a` in zsh, where
`echo -e 'a\eZ'` is the other way round — and it was one axis for both
letters until #908 split it. See `EchoExpandsCapitalEscEscape`.

### A format is a byte string, in every direction

A shell word is a string of bytes and so is a `printf` format, and the
byte a format decodes has to reach the output as that byte. Three routes
into `printf` were spelling it as the text UTF-8 gives the *code point* of
the same number instead, so `printf 'a\300Z'` wrote `61 c3 80 5a` where
all five shells write `61 c0 5a`:

    printf 'a<0xc0>Z'    a literal byte walked over by the format loop
    printf 'a\300Z'      an octal escape
    printf '%c' '<0xc0>' the first byte of a `%c` operand

None of them is about how a word is *read*. The same byte written into a
variable, or produced by `$'\xc0'`, or handed to `%s`, came through
untouched in every dialect — `printf '%s' 'a<0xc0>Z'` was already
`61 c0 5a` — and the octal case has no byte above ASCII in its source at
all. The three sites are the format's own, and the cause in each is the
same one: Go's `string(x)` on an integer is a *rune* conversion, so a byte
of 0xc0 becomes the two bytes U+00C0 is spelled with. A one-byte slice is
the conversion that means what a shell means.

## A capability the corpus cannot hand a case

The corpus can give a case its own argv (`Case.Args`) and its own standard
input (`Case.Stdin`), and it still cannot give one a *fourth* descriptor.
Every case is run by one harness that builds one `exec.Cmd`, and nothing
in a Case says "and open this on 3" — so the whole of what a shell does
with a descriptor its caller opened is outside what `make oracle` can
re-run. The obvious workaround makes it worse: a snippet could invoke
`"$0"` recursively with a redirection, but one panel column is deliberately
invoked with `argv[0]` of `sh`, and `"$0"` there is whatever `/bin/sh`
happens to be on the machine — bash on macOS, dash on Debian. That is the
mislabeled-column trap `MustReport` exists to prevent, reintroduced by a
snippet.

So it is prose, measured and re-measured against bash 5.3, bash 3.2, dash,
ksh93 and zsh (macOS, 2026-09-05). The invocation throughout is
`echo hello | <shell> 3<&0 case.sh`, which opens descriptor 3 on the pipe
before the shell starts:

    exec <&3; read x; echo "got:$x"        got:hello — unanimous
    read x <&3; echo "got:$x"              got:hello — unanimous
    exec 4<&3; read x <&4                  got:hello — unanimous
    exec 3<&3; read x <&3                  got:hello — unanimous
    read x <&7                             7: bad file descriptor — unanimous
    exec 3<&-; read x <&3                  3: bad file descriptor — unanimous
    exec 3>&-; read x <&3                  3: bad file descriptor — unanimous,
                                           and the close reports 0 whichever
                                           direction it is written
    exec 3<&-; exec 3<&-                   0 — closing a closed one is not an
                                           error, as closing an unopened one
                                           is not
    sh -c 'read y <&3'                     the child reads it — unanimous
    exec 3<&-; sh -c 'read y <&3'          the child finds it closed — unanimous
    exec 9<&3 3<&-; sh -c '… >&3'          the same, with the descriptor moved
                                           rather than dropped
    exec sh -c 'read y <&3'                the replacement reads it — unanimous

Nothing splits the panel, which is the finding: an inherited descriptor is
an ordinary member of the table from the moment the shell starts, and every
question that has an answer for `exec 3<file` has the same answer for one
the caller opened. There is no axis here, so no preset gains a field — see
"Rules for adding an axis": a unanimous answer is a *behavior*, and asking
about it would refuse the construct in the core over a question that
decides nothing.

Two things about it are ours rather than the panel's, because a shell
written in Go has to rebuild what fork and exec give a shell in C. The
descriptors have to be found, and close-on-exec is the discriminator: the
Go runtime opens every descriptor of its own close-on-exec, so one that
would survive an exec is one the process was handed. And they have to be
found by the *binary* rather than by the interpreter — `interp` is a
library, and a Runner embedded in some other program would otherwise
publish that program's own files to a script it was asked to interpret.
`interp.Runner.InheritedFiles` is therefore a fact the front end hands in,
laid out exactly as the outbound table is: entry i is descriptor 3+i, and
a gap is a number nothing arrived on.

### The table a replacement inherits

`exec cmd` is the third side of that boundary and the only one with no
fork in it. An external child is renumbered by `os/exec` on the way
across; a replacement has nothing to do the renumbering, so the
*process's* own descriptors are what the command inherits — and a
Runner's descriptor 3 is not the process's 3, because `exec 3>h` opens a
file on whatever number was free and the script's number exists only in
the interpreter's table. Clearing close-on-exec is therefore half the
answer: each file has to be duplicated onto its number as well, which
clears the flag as a side effect.

Measured on macOS, 2026-09-05, across bash 5.3, bash 3.2, dash, ksh93 and
zsh — the first two are pinned in the corpus as
`redir/exec-descriptor-reaches-a-replacement` and
`redir/a-replacements-descriptor-numbers-keep-their-gaps`, which need no
descriptor from the harness because the script opens its own:

    exec 3>f; exec sh -c '… >&3'           the replacement writes — all but
                                           ksh93, which alone keeps it
    exec 5>f; exec sh -c '… >&5; … >&3'    5 carries and 3 is closed there, so
                                           the numbers are the shell's and a
                                           gap stays a hole
    exec 3<&-; exec sh -c 'read y <&3'     closed for the replacement too, and
                                           here ksh93 agrees — unanimous
    coproc …; exec sh -c '… >&${C[1]}'     nothing open on either near end,
                                           nor on a 3 duplicated from one,
                                           though the shell still writes
                                           through that 3 (bash)

which is the outbound table an external child is given, entry for entry:
the same numbering, the same holes and the same exclusions. ksh93's
divergence is the one already recorded for a child and is the same
divergence rather than a second one — and it is narrow, because the row
about a *closed inherited* descriptor is unanimous. What ksh93 withholds
is what the script opened, not what the shell was handed.

The closing row is the one that is not about the flag at all. A
descriptor the process was started with is still open here and is *not*
close-on-exec — that is how it was recognized as inherited — so
`exec 3<&-; exec cmd` hands 3 to the command through the kernel, behind
the table's back, unless the number is reached and closed. It is closed
only where it would otherwise survive, which is what keeps the same walk
from closing the Go runtime's own files on the way past.

Placing the table is the *binary's*, for the reason the exec itself is:
rewriting the process's descriptors is process-wide state, and a Runner
embedded in another program may not touch it. So `interp` decides which
descriptors are the script's to hand out and hands that slice to
`Runner.ReplaceProcess`.

### The named streams cross in the same slice

They are the half with no second route. A descriptor above 2 is placed
because Go opened it close-on-exec; standard output has to be placed
because after `exec >log` the file is on whatever number Go had free and
the process's own 1 is still the caller's. An external child never shows
this, because `os/exec` builds its 0, 1 and 2 separately and will copy
bytes through a pipe for a stream that is not a file at all — there is no
separately here, and nothing to copy with.

Measured on macOS, 2026-09-05, across bash 5.3, bash 3.2, dash, ksh93 and
zsh, and pinned in the corpus as `redir/a-replacement-keeps-a-redirected-…`
and `redir/a-closed-std…-is-closed-for-a-replacement`:

    exec >log; exec cmd                    the replacement writes to the file
                                           — unanimous, ksh93 included
    exec cmd >log                          the same, with the redirection on
                                           the `exec` itself
    exec <data; exec cat                   the replacement reads the file
                                           rather than the shell's own input
    exec >log 2>&1; exec cmd               both streams reach the file: `2>&1`
                                           is one file under two numbers
                                           rather than two targets
    exec >&-; exec cmd                     the command finds it closed and
                                           fails — unanimous
    exec <&-; exec cmd                     the same on the reading side

So the slice a replacement is given is the table extended down to zero —
entry i is descriptor i — rather than the ExtraFiles layout the other two
halves use, and that is the whole of the difference between them. The
exclusion the dissenting shell makes for what `exec` opened stops above 2
in that shell too: its own `exec >log; exec cmd` writes to the file.

Both rules the table already stated carry over, and the second is a
decision rather than a consequence:

- **Only a real file can cross.** A Runner embedded in another program may
  have a caller's buffer behind standard output, and there is no number to
  hand a replacement for one of those.
- **A nil is a number that must not be open**, for a named stream as for
  the rest of the table, rather than "leave the process's own". The
  measured rows above decide it: `exec >&-; exec cmd` leaves the command a
  closed descriptor everywhere, and it is the same nil. The alternative
  would also be the one way for an embedder's terminal to reach a command
  through a stream the script had redirected away from, which is the
  borrowing of process state this library exists not to do.

One case is neither reading, and it is the one the two rules together got
wrong: one dialect writes to *every* target of a repeated redirection, so
`exec >a >b` leaves the runner a writer over two files and no single number
to place. Read as "not a file, therefore nil, therefore closed", the command
a replacement ran found standard output closed and failed —
`echo: fflush: Bad file descriptor` — where that shell writes into both.

The two rules are still right; what was missing was a third, ahead of them
and narrower than either:

- **A replacement is declined for a stream the shell built out of several
  targets**, and for nothing else. It then stands in for the replacement
  with the child route it already has for a subshell, which `os/exec` gives
  a pipe and copies from, so both files get the bytes.

The narrowness is the point. The test is a mark the shell puts on the stream
when it combines the targets, not "this is not an `*os.File`" — that wider
reading would have swept in an embedder's buffer, whose closed number is
measured and deliberate and which a child route could not improve on
anyway. A stream closed on purpose still crosses as a closed number, which
is the row the panel is unanimous about.

What that costs is the child route's own difference, written down where it
is: the pid, the signal dispositions, and being the process the parent waits
for. Measured, the shell being reproduced spends a process on this too — it
forks a copier and keeps its own pid for the command, where we keep the pid
for the shell and give the command a new one — so the number of processes
agrees and which one is the command does not. Everything the corpus can see
agrees.

**Only the named streams.** A *numbered* descriptor with several targets —
`exec 3>a 3>b` — is a different gap and an older one: it is not modeled at
all here, replacement or not, and the last target simply wins. No route
through `os/exec` would carry it either, because `ExtraFiles` is files.

### Whether a descriptor `exec` parked is handed over at all

`ExecOpenedFdReachesACommand`. Four of the five hand it over and ksh93
keeps it, which is the shape of a conflict rather than a subset: there is
no reading under which one answer contains the other, so it is a field on
the vector rather than a core answer with a dialect apologising for it.

**POSIX decides nothing.** The Shell Command Language says which of the
standard descriptors a utility is entered with and is silent about the
rest, so both answers conform and there is no standard to defer to. The
dissenting shell's manual states its rule outright — a file descriptor
number greater than 2 opened by `exec`'s redirection list is closed when
it invokes another program — so this is a documented language decision
rather than a build's accident, which is what a recorded divergence would
have implied.

The rule is narrower than "that shell hands nothing over", and the whole
of the boundary was measured (macOS, 2026-09-05):

    exec 3>f; sh -c '… >&3'          four write; ksh93's child finds 3
                                     closed
    exec {v}>f; sh -c '… >&$v'       the same split — a number the shell
                                     picked is no different
    exec 9<&3; sh -c 'read <&9'      with 3 inherited: four read it, ksh93
                                     finds 9 closed — and its *3* still
                                     reads, so a duplicate carries the mark
                                     and the original does not
    sh -c 'read <&3' (3 inherited)   unanimous: the caller's descriptor
                                     crosses everywhere
    sh -c '… >&3' 3>f                unanimous: a command's own redirection
                                     crosses everywhere
    exec 3>f; sh -c '… >&3' 3>&3     unanimous: naming the number again on
                                     the command hands it over even there
    exec 2>f; sh -c '… >&2'          unanimous: the rule starts above 2

The last three are why the axis is asked about a *mark* rather than about
the table. A descriptor is `exec`'s when `exec`'s own redirection list
opened it; the mark travels with a duplication, and a command redirecting
the same number takes it off for that command and no longer. The corpus
records the two unanimous rows as
`redir/a-commands-own-redirection-crosses` and
`redir/restating-the-number-hands-an-exec-descriptor-over`, because
"everyone crosses here" is the half that says what the axis is not about.

It is read where the outbound table is built, once, so an external child
and a process replacement get the same answer — and they were measured to
give the same one, which makes this one divergence rather than two. It is
asked only when the table actually holds such a descriptor: an axis
consulted on the common path would refuse every external command in a
Runner that had not chosen a dialect, over a question that decides nothing
for a script with no parked descriptors.

A fifth panel member does not change it. The question is whether the
dissenter gets its own answer, not how large the majority is, so a shell
that agreed with the four would leave the axis exactly as it is and one
that agreed with ksh93 would answer `No` beside it. What a fifth member
could change is a *core* answer that rested on a bare majority, and this
one does not: the core follows four shells and the standard's silence.

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

That last sentence is also a trap, because **a rule can decide late**. The
body of a function is refused for not being compound only once the body has
been *read*, so by the time the refusal is raised the parser is looking at
whatever follows it. Recording that as a bare syntax failure loses two
things at once: the echo, which is only offered to an unexpected-token
failure, and the token itself. bash names the word, the assignment or the
redirection operator the body began with:

    f() echo hi; f     syntax error near unexpected token `echo'
    f() x=1; f         syntax error near unexpected token `x=1'
    f() >out; f        syntax error near unexpected token `>'

So the token is saved before the body is parsed and the failure is raised
against *it*, which also puts the echo on the body's own line rather than on
the line the parser stopped at (measured: `cmd/function-body-simple-command`,
`cmd/function-body-an-assignment`, `cmd/function-body-a-redirection`).

`f() ;` is the same shape with no body for any grammar, refusing or
permissive, and all four shells name the token rather than describing the
function (measured: `cmd/function-with-no-body-at-all`).

### End of input with nothing open

`f()` runs out of input with the parens already closed, so there is no
construct left to name — and two of the shells whose end-of-input sentence
names one say something shorter rather than leaving a hole in it:

    dash   Syntax error: end of file unexpected
           (against `Syntax error: end of file unexpected (expecting "fi")`)
    bash   syntax error: unexpected end of file
           (against `… from `if' command on line 1`)
    ksh93  syntax error at line 1: `end of file' unexpected
           (against ``if' unmatched`)

That is `Diagnostics.UnterminatedNoConstruct`, used when the failure carries
no construct and left empty by a dialect whose one sentence never mentioned
one (measured: `cmd/function-parens-then-end-of-input`).

### A bad descriptor is an errno like any other

`echo hi >&6` with nothing on 6 reports `Bad file descriptor`, which is the
C strerror string and goes through the same `LowercaseReason` axis as every
other reason a redirection quotes. Writing it out lowercase at the places
that raise it — Go's own spelling of the errno — matched the one dialect
that lowercases everything and nobody else (measured:
`redir/a-dup-prefix-is-that-commands-alone`).

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
The long names are the dialect's membership rather than an axis — but the
membership is not the clean three-way split the letters suggest, and this
file said it was. Re-measured 2026-09-05 by asking each shell for every
name in both directions:

| long name | bash 5.3 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `hashall` | yes | no | no | **yes** |
| `trackall` | no | no | yes | **yes** |
| `histignoredups` | no | no | no | yes |

**zsh has both spellings of command tracking.** The sentence that stood
here — each name belonging to the dialects that list them "and to no
other" — read as though `trackall` were ksh93's alone, and
`dialect/zsh` had been written to match: `set +o trackall` succeeded in
real zsh and was refused by ours. Corrected in both places, and pinned by
`opt/command-tracking-has-two-long-names` so the next reading of the
table is against the panel rather than against the sentence.

## The long `set -o` names, and why turning one *off* always works

Three questions live behind one builtin, and conflating them is what made
`set +o posix` — the thirteenth line of Homebrew's own `brew` script —
stop the shell dead with `invalid option name`:

1. **Does this shell have the name at all?** The dialect's answer.
2. **Do we implement it?** Ours, and for most of them the answer is no.
3. **If not, which state are we already in?** Also ours, and it decides
   whether a request is a lie or a no-op.

The third is the point, and it yields one policy: **a request to turn off
something this shell was never doing has been granted.** `set +o posix`
in a shell with no posix mode leaves it exactly where it was asked to be,
so it succeeds silently. Turning *on* something we do not do would be a
promise we cannot keep, so it is refused out loud rather than accepted
quietly. The corpus row is
`opt/turning-off-a-name-a-shell-does-not-implement`.

The policy stops at names the dialect does not have at all. A name outside
the table is refused in both directions, because a name that does not
exist is not a state anything is already in — every shell in the panel
agrees, in four wordings and at two statuses, and three of them end the
script over it (`opt/an-unknown-long-name-is-refused`).

**Fourteen names are unanimous** and belong to the core, needing no
dialect to declare them: `errexit`, `nounset`, `xtrace`, `noclobber`,
`noglob`, `allexport`, `noexec`, `verbose`, `monitor`, `notify`,
`ignoreeof`, `nolog`, `vi`, `emacs`. Measured by asking each shell to turn
each one off; all four accept all fourteen. (ksh93's own `set -o` listing
prints the *positive* spellings — `unset`, `glob`, `clobber`, `exec`,
`log` — which is why the membership was established by asking rather than
by reading the listing.)

The rest are declared by each dialect that has them:

| name | bash 5.3 | ksh93 | zsh |
| --- | --- | --- | --- |
| `braceexpand` | yes | yes | yes |
| `histexpand` | yes | yes | yes |
| `pipefail` | yes | yes | yes |
| `privileged` | yes | yes | yes |
| `keyword` | yes | yes | no |
| `hashall` | yes | no | yes |
| `onecmd` | yes | no | yes |
| `physical` | yes | no | yes |
| `trackall` | no | yes | yes |
| `histignoredups` | no | no | yes |
| `errtrace` | yes | no | no |
| `functrace` | yes | no | no |
| `history` | yes | no | no |
| `interactive-comments` | yes | no | no |
| `posix` | yes | no | no |

dash declares none of them: it has the fourteen and nothing else.

Of these, four are real here — `pipefail` (the pipeline code reads it),
`hashall`/`trackall` (one state behind both names: permission to cache
rather than a promise to), and `histignoredups` (kept truthfully over a
history this shell does not keep). The rest are recorded with the state we
are already in, so that turning them off succeeds honestly:
`braceexpand` and `interactive-comments` are **on**, because we do expand
braces and do honor comments wherever they are written; everything else is
**off**. That is not a claim about what any other shell defaults to —
bash has `hashall` on and we do not hash at all, so ours is off and a
script turning it off gets what it asked for.

The table is a subset of what these shells actually have — ksh93's own
listing runs to `bgnice`, `globstar`, `letoctal`, `markdirs` and a dozen
more. Names outside it are refused by name rather than accepted and
ignored: under `set -o`, accepting an option we do not honor would be a
promise. zsh's `setopt` answers the same question differently and on
purpose — see **zsh's option names** below — because the names a zsh rc
file writes are overwhelmingly about features this shell does not have at
all, where recording a request promises nothing.

## zsh's option names

zsh 5.9.2 has 185 options and 12 further spellings borrowed from sh and
ksh, and `setopt`/`unsetopt` recognize all 197. The set was derived by
probing the binary: `set -o` lists every option in the spelling that is off
by default, `zmodload zsh/parameter` then exposes `$options`, whose keys are
the canonical names, and each compat spelling was identified by flipping it
alone and reading which canonical entry moved with it.

**Recognizing, recording and implementing are three claims, and only the
first is unanimous across the table.** Every name is one of five kinds:

| kind | how many | what `setopt NAME` does |
| --- | --- | --- |
| substrate-backed | 11 | moves a real `set -o` switch: `setopt err_exit` **is** `set -e` |
| axis- or matcher-backed | 6 | moves a semantics axis (`shwordsplit`, `nomatch`, `ksharrays`) or a pattern-matcher option (`nullglob`, `globdots`, `caseglob`) |
| fixed | 18 | refuses to move, in zsh's own words: `can't change option: NAME`, status 1. Asking for the state it already holds is granted |
| store-backed, read by the front end | 1 | `histignorespace`: kept where a recorded name is kept, and read by the line editor before it records a line |
| **recorded** | 149 | succeeds, is remembered, and is reported by `setopt`/`unsetopt` — and changes nothing about what the shell does |

**Two names moved out of "recorded" when the history knobs were built**
(#571). `histignorespace` is the fifth row above: its state has nowhere
better to live, because the substrate has no `set -o` name for it, but an
interactive session reads it through this namespace every time it accepts
a line. `histignoredups` was already substrate-backed — zsh's `set -h`
abbreviates it — and was a switch whose state nothing consulted; it is now
consulted too. Both decide what a session writes to its history file, so
neither is recorded any more. Nothing else about the split moved: 149 of
185 is still most of the table, and the count above is the one produced by
counting the constructors in `dialect/zsh/setopt.go`.

The recorded kind is the change of position, and it is deliberate. A real
`~/.zshrc` opens with a dozen `setopt` lines about completion, correction,
menu selection and history files — none of which exist here — and answering
each with `no such option` sprayed sixteen complaints at every interactive
start over nothing this shell was ever going to do. Recording stops the
complaint and reports the request back faithfully. **It promises nothing
further**: `setopt auto_cd` succeeds and a bare directory name still does not
change directory; `setopt extended_glob` succeeds and the glob syntax does
not change. A reader wanting to know which is which reads the table in
`dialect/zsh/setopt.go`, where the recorded ones say `recorded(…)` and
nothing else does.

The five that refuse to move are the five about being interactive —
`interactive`, `monitor`, `shinstdin`, `singlecommand`, `zle` — which is
measured rather than chosen: asking a real non-interactive zsh for all 197
names in both directions refused exactly those five (plus their two compat
spellings) and granted every other one. The other thirteen fixed names are
this shell's own — `aliases`, `banghist`, `chaselinks`, `emacs`,
`functionargzero`, `hashdirs`, `ignorebraces`, `ignoreeof`,
`interactivecomments`, `notify`, `privileged`, `shglob` and `vi` — each of
which reads its state through the substrate or holds a constant and cannot
move it, so asking it to move is refused rather than granted falsely. Real
zsh grants all thirteen; that divergence is the price of not lying about
symbolic links, brace expansion or a history that is not kept.

**The listings.** Every option has one printed spelling — the one that is
off by default, so `noclobber` for an option that defaults on. A bare
`setopt` prints the spellings that are on and a bare `unsetopt` prints the
ones that are off, both ordered by canonical name and both naming the
canonical option rather than the compat spelling that may have set it. In a
shell that has changed nothing that is 1 line and 184.

**A known inaccuracy, inherited rather than introduced.** The table records
zsh's default for each name, which is what the listings compare against.
Four entries hold this shell's own state there instead — `banghist`,
`emacs`, `hashcmds` and `interactivecomments` are all measured the other way
round in real zsh — which silences four deviations the listing exists to
show. Correcting them would make the bare `unsetopt` listing byte-identical
to zsh's 184 lines and would move the same four lines of divergence onto the
bare `setopt` listing, because the underlying fact is that this shell's
state genuinely differs from zsh's for those four. It is a trade rather than
a fix, and it is left where it was found.

Two names are one-way. `noexec` ignores being turned back off in all four
shells — and with it on, the command that would do so never runs anyway.
`monitor` is the one request in the table a dialect can refuse, which is
the `set -m` split above.

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

### `--help` is an option one shell answers and the rest refuse

It looks like a courtesy and it is the most-run builtin call there is.
macOS ships fifteen commands in `/usr/bin` as the same stub —
`builtin $(basename $0) "$@"` — so `/usr/bin/alias --help` *is* a shell
builtin call, and it is how a person or a script pokes at one. Every
disagreement the real-script run sweep found was this: 22 of 48 runs
across 15 scripts (#815, #825).

Measured 2026-09-05, panel and machine as `../spec/oracle.md`:

| probe | dash | bash 3.2 | bash 5 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `alias --help` | `--help not found`, 1 | `alias: --: invalid option`, 2 | **synopsis on stdout**, 2 | `Usage: alias …` on stderr, 2 | `bad option: -h`, 1 |
| `bg --help` | `Illegal option --`, 2 | `bg: no job control`, 1 | **synopsis on stdout**, 2 | `Usage: bg …` on stderr, 2 | `no job control …`, 1 |
| `true --help` | 0, silent | 0, silent | 0, silent | 0, silent | 0, silent |
| `echo --help` | prints `--help` | prints `--help` | prints `--help` | prints `--help` | prints `--help` |
| `alias --help=x` | takes the assignment | `--: invalid option`, 2 | `--: invalid option`, 2 | its whole option list | `bad option: -h`, 1 |
| `alias -- --help` | `-- not found` | `--help: not found`, 1 | `--help: not found`, 1 | 0 | 1 |

Four facts, and each is a separate way to get it wrong:

1. **The answer goes to standard output.** Every refusal here goes to
   standard error, so the stream is what a script uses to tell an answer
   from a complaint — and it is why this is not simply another wording of
   the bad-option refusal it sits beside.
2. **It ends the builtin with 2 all the same.** Printing help is not
   success in bash: `alias --help` exits 2, the same status a bad option
   earns, which is why a script cannot tell the two apart by status.
3. **It is answered before anything the builtin cannot do.** `bg --help`
   answers in a shell with no job control at all, so the option is read
   before the precondition rather than after it.
4. **It has to be the exact word, standing where an option stands.** An
   abbreviation is not it, `--help=x` is not it, and a `--help` past the
   `--` that ends the options is an operand. Nor is one that a letter has
   already claimed: `read -d --help` hands `--help` to `-d` as its
   delimiter and reads on. That last one is why the shared option reader
   answers this rather than a scan of the argument list — only the reader
   knows which letters take an argument.

Diagnostics: `BuiltinHelp`, a map from builtin name to the answer, and
`BuiltinHelpStatus`. A name with no entry has no answer and `--help` is
then the ordinary option nobody has, which is what three of the five do
for every builtin — so the map says *whether* as well as *what*, and no
separate axis is needed. `BuiltinHelpStatus` defaults to 2 rather than to
0 on purpose: the option reader tells its callers "this builtin is
finished" with a nonzero code and has no other way to say it, so a help
status of zero would print the answer and then run the builtin anyway.

**What is implemented is the synopsis and not the block.** bash follows
the synopsis with a paragraph of description, a list of its options and
an "Exit Status" note. That is documentation prose rather than shell
behavior, and this tree carries no other project's text — see
`CLEANROOM.md` — so it is deliberately not reproduced. What a script can
act on is implemented: that the option is recognized, that the answer is
on standard output, that it comes first, and that the status is 2. The
corpus cases are written as a first line and a status for the same
reason. ksh93's answer is a third shape again — a short usage on standard
error — and is not modeled.

The synopsis is not written twice. Measured over every builtin bash has:
the first line of `help NAME` is exactly the usage line a bad option to
NAME earns, with the `usage: ` taken out — `alias: alias [-p]
[name[=value] ... ]` against `alias: usage: alias [-p] [name[=value]
... ]` — so `dialect/bash` derives one from the other.

### The usage line after a bad option is missing from no builtin

The same measurement found the neighboring gap. bash and ksh93 print a
usage line under a bad-option complaint for *every* builtin, and six of
ours printed the complaint with nothing under it: `alias`, `unalias`,
`cd`, `fc`, `hash` and `ulimit`. Five of those reach the shared option
reader, which looks the line up in `BuiltinUsage` and found no entry;
`ulimit` refuses on its own, because its letters are resource names
rather than a fixed set, and was not asking for the line at all.

`umask` is the third shape and was wrong in a different way: it spelled
the offending word out itself, so it was the one builtin in the shell
that answered `umask: --version: invalid option` where every other one
answers `umask: --: invalid option`. Which part of a `--` word a
complaint names is `BadOptionNaming`, a rule about the dialect and not
about the builtin, and it now goes through the same reader as the rest.

Corpus: `help/a-builtin-answers-the-help-option`,
`help/the-help-option-comes-before-what-the-builtin-cannot-do`,
`help/a-builtin-with-nothing-to-say-takes-it-as-a-word`,
`help/the-help-option-is-the-whole-word-and-stands-where-an-option-stands`,
`help/a-bad-option-is-followed-by-the-builtins-usage`,
`help/a-bad-option-to-umask-is-named-the-way-the-dialect-names-one`.

## read's options are the dialect's letters

`Semantics.ReadOptions`, in the getopts spelling — a `:` after a letter
whose argument follows it. Measured (oracle runs, 2026-09-04, bash 5.3 and
3.2 agreeing throughout except where 3.2 lacks a letter):

    bash   rsa:d:i:n:N:p:t:u:  plus -e -E, unimplemented here
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
- **The seed.** `-i text` is the text a *line editor* opens with, so it
  has an effect only where there is a terminal with an editor on it.
  bash's own answer with no terminal is to take the option, consume its
  argument, and read the line as though the letter were not there:

      printf "x\n"  | { l=keep; read -i pre -r l; ... }   bash  st=0 l=[x]
      printf "\n"   | { l=keep; read -i pre -r l; ... }   bash  st=0 l=[]
      read -i                                            bash  `-i: option
                                                               requires an
                                                               argument`, st=2

  The second line is the reading that looks right and is not: the seed is
  **not** a default for an empty line, in bash or in anything else. bash
  3.2, bash as `sh`, dash, ksh93 and zsh have no such letter at all and
  refuse it in four wordings at two statuses — bash 3.2 and dash and
  ksh93 at 2, zsh alone at 1 — which the shared bad-option reader already
  produces from each dialect's own words.

  So the letter is implemented here as bash's no-terminal behavior, and
  what stops that from being an option that lies is the *other* letter:
  `-e`, which opens the editor a seed would go into, is still refused by
  name as unimplemented. There is never an editor here for the seed to
  reach (#761).
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
- **A timeout of zero is not a timeout.** `-t 0` is a question about the
  state of the stream, and every dialect with the letter treats it as one.
  Measured three ways (oracle runs, 2026-09-05): a whole line already
  waiting, nothing waiting at all, and `ab` waiting with the rest of the
  line half a second behind it.

        shell   line waiting     nothing   `ab` waiting
        bash    0, nothing read  1         0, nothing read
        ksh93   0, line read     1         1, nothing kept
        zsh     0, line read     1         0, `abc` read
        dash    has no -t at all

  The first two columns are the same everywhere, which is why this looked
  like one behavior. The middle column is the same in what it *reports* and
  not in what it leaves: `v=old; read -t 0 v` with nothing waiting is
  `v=[old]` in all three, because giving up is running out of time and
  running out of time touches no name — see "An expired `read -t` is not an
  end of input" below. An already-ended stream is not that case and empties
  the variable everywhere. The third column separates them, and so does what happens
  next: after bash's `-t 0` the following `read` still finds the first
  line, and after ksh93's and zsh's it finds the *second*. That is the
  difference that matters to a script — a shell that polls can be asked
  the same question in a loop, and a shell that reads eats what it was
  watching for one line at a time.

  It is `Semantics.ReadZeroTimeout`, three named answers plus the refusal,
  asked only where the timeout is written as zero. The end of a stream is
  *ready* to the shell that polls — a read there returns at once, with
  nothing — so an empty file gives status 0 there and 1 in the two that
  read (measured: `read/a-zero-timeout-and-what-it-does-to-the-stream`,
  `read/a-zero-timeout-at-the-end-of-the-input`).

  The status for "nothing waiting" is 1 and not the number an expired
  `-t` reports, which is the second reason this is not a timeout: the
  shell that polls answers 1 here and 142 for a deadline that ran out.

  Answering it needs the one question a shell asks a stream without
  touching it — whether a read would return at once — and a read is
  exactly what would destroy the thing being asked about. `interp`
  asks the descriptor with a zero-length wait and never reads
  (`inputready.go`); anything a Runner holds in memory answers
  immediately by construction and counts as ready, which is also the
  answer where a descriptor cannot be asked.
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
  `Diagnostics.ReadNoCoprocess`. **"Is a terminal" is the ioctl here too**
  (#525). It was `interp`'s own approximation until then — a character
  device, excepting the null device — because the exact answer lived in
  `repl` and `repl` imports `interp`, so the dependency only ran one way.
  It is `internal/tty` now, which both import, and the exception for the
  null device is *gone* rather than moved: the ioctl answers ENOTTY there
  without being told the path. Measured 2026-09-06 on the device that made
  the point, `read -p 'PROMPT-42 ' v < /dev/urandom` — bash 5.3.15, bash
  3.2.57 and bash-as-sh all read a line, exit 0 and print **no prompt**,
  where this shell printed one. `/dev/random` rather than `/dev/null` is
  the case that says the question is about terminals: the read succeeds
  there, so there is every reason to have prompted. `select`'s menu had
  the same fault under the ksh93 axis, drawing its `#? ` where real ksh93
  draws none.
- **The timeout.** `-t SECS`, fractions allowed; input already waiting is
  read as if the flag were absent. Expiry reports 142 in bash (128 plus
  SIGALRM) and 1 in ksh93 and zsh — `Diagnostics.ReadTimeoutStatus` — and
  what it leaves in the variables is `Semantics.ReadTimeoutKeepsWhatArrived`,
  measured below. bash 3.2 has the letter and takes **whole seconds only**:
  `read -t 0.2` there is `invalid timeout specification`, status 1, not
  modeled. bash's `-t 0` polls for waiting input
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

## An expired `read -t` is not an end of input

Measured 2026-09-05 against bash 5.3, bash 3.2, ksh93 and zsh; dash has no
`-t`. The probe is a stream that delivers half a line and then stalls,
because the usual one — nothing arriving at all — cannot tell the answers
apart:

    { printf part; sleep 0.5; printf 'ial\n'; } |
      sh -c 'v=old; read -t 0.2 v; echo "$? [$v]"'

    bash 5.3  142 [part]      ksh93  1 [old]      zsh  0 [partial]

    { sleep 0.5; printf 'late\n'; } |
      sh -c 'v=old; read -t 0.2 v; echo "$? [$v]"'

    bash 5.3  142 []          ksh93  1 [old]      zsh  1 [old]

Three things, and only the first is `ReadTimeoutKeepsWhatArrived`.

**bash does not clear the variable, it assigns a short read.** The second
probe is the one everybody writes, and it makes bash look like it clears —
which is what this implementation copied, unconditionally, for all three
dialects. The first probe says otherwise: what lands in the variable is
whatever arrived before the deadline, and an empty assignment is only that
rule with nothing to assign. The distinction matters because the code that
was wrong was wrong in *both* directions at once — it cleared where ksh93
leaves the name alone, and it discarded a partial line bash keeps.

**A timeout is not an end of input.** At end of input all four assign,
including the partial with no delimiter: `printf tail | read v` leaves
`tail` everywhere. The comment on that arm explains why it has to — a
`while read -r l` loop that left `l` behind would read as the last line
rather than as nothing — and the rule does not carry over to a deadline,
which two of the three treat as no read at all.

**zsh's `-t` is not the same kind of timeout**, and this is a separate
divergence rather than this axis. It bounds the wait for the stream to
become *readable* and nothing after that: once a byte has arrived zsh reads
the line to its end however long that takes and reports 0. Measured with a
byte dripping every 0.1 s under `-t 0.25`, zsh returned 0 with the whole
six-byte line after 0.6 s where bash returned 142 with the first three
characters; with a stream that stalls mid-line for three seconds and never
closes, zsh waited all three and succeeded. Ours is a whole-read deadline
in every dialect, so the zsh preset answers this axis "leave the name
alone" — right for every timeout it can actually reach, since a zsh timeout
only happens with nothing to assign — and is still wrong about *when* it
times out. Not modeled here; it is its own question and its own measurement.

There is no corpus row for any of this. Every probe above needs a stream
that is slow on purpose, and a case whose answer depends on which of two
timers wins is the kind of *sometimes* `oracle.md` says a record cannot
hold. It is pinned by Go tests instead, against a reader that hands over a
fixed prefix and then blocks — the same situation with no clock in it.

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

## Where a one-shell builtin's code lives: register in the core, take it away

Two placements are available for a builtin only some shells have, and the
choice is made per builtin rather than by a rule that covers all of them.
It is worth stating because the two look contradictory from outside and
`core.md`'s boundary reads as forbidding the first.

**Registered in the dialect.** `dialect/zsh/setopt.go`,
`dialect/zsh/whence.go`, `dialect/ksh/whence.go`, `dialect/ksh/print.go`,
`dialect/bash/caller.go`. Two of those are the *same spelling* in two
shells and two separate files, which is the placement earning its keep: a
shared `whence` would have to hold both option sets, both statuses and
both streams, and the shells agree about none of them.
The command is that shell's own idea — its option namespace, its escape
set, its stack format — and nothing in the core would have a use for it.

**Registered in the core and `Unregister`ed by the dialects without it.**
`mapfile`, `readarray`, `compgen`, `complete`, `enable`, and the same
shape for `local` (taken away by ksh93), `let`, `fc`, `typeset`, `disown`
and `builtin` (taken away by dash).

The criterion is not which shells have it today but **whether the command
is the substrate's kind of thing**: does it act on state the core already
owns, in a way a second dialect would plausibly want? `mapfile` reads a
stream into an array; `compgen` answers from the builtin and function
tables; `enable` turns a builtin off. All three are questions about the
interpreter rather than about bash, so the code belongs where the state
is, and membership is expressed by taking the name away.

Measured 2026-09-05 with `command -v` under an empty PATH, which is how
the Unregister lists were checked rather than assumed:

| name | bash 5.3 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `mapfile`, `readarray` | yes | no | no | no |
| `compgen`, `complete` | yes | no | no | no |
| `enable` | yes | no | no | **yes** |
| `let`, `fc`, `typeset`, `disown`, `builtin` | yes | no | yes | yes |
| `local` | yes | yes | **no** | yes |

`enable` is the row that shows the lists are measured: zsh has it, so
`dialect/zsh` does not take it away, while `dialect/ksh` and
`dialect/dash` both do. `builtin` is a further wrinkle — ksh93 has a
command of that name that does something else entirely, so ksh does not
unregister it but replaces it.

**This is not a hole in `core.md`'s boundary.** That boundary is about the
*language* — what parses, and what identical syntax means. Which builtins
a shell has is neither: ksh93 simply lacks `local` and reports a command
that was not found. Where a Go function is registered is an implementation
detail of the extension seam, and a name registered in the core map is not
thereby in the core language; it is in the core language only if every
dialect keeps it.

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
  negating — over the **whole** measured name set. See "zsh's option names"
  below for what each name costs and what it buys; the short version is that
  all 185 options and all 12 compat spellings are recognized, a name outside
  them is `no such option`, and one of the four kinds a name can be — the
  recorded kind — is remembered without being acted on.
- **zsh `emulate`** (dialect/zsh/emulate.go): `sh`/`ksh`/`zsh` switch the
  three measured axes above and reset the option table to the emulation's
  defaults, `-c` runs a string under the emulation and restores everything
  after, and a bare `emulate` names the mode.
- **ksh93 `whence`** (dialect/ksh/whence.go): bare, `-v`, `-p`, `-q`, `-a` —
  the bare mode delegating to the same lookup `command -v` uses, `-v` to the
  core `type`, whose ksh wording was already `whence`'s, and aliases spoken
  for in the dialect because the core's lookup cannot see them.

  `-a` is every resolution rather than the first, always in `-v`'s
  sentences, in this order:

      alias, if any            N is an alias for V
      keyword, if any          N is a keyword
      function, if any         N is a function
      builtin, if any and no   N is a shell builtin
        function shadows it
      every PATH hit           N is <path>, or `N is a tracked alias for
                               <path>` when the PATH hit is the only line
      the FPATH candidate      N is an undefined function

  A defined function hides the builtin of the same name — which is what the
  shell would run — and an alias hides nothing.

  The last line is what #633 recorded as needing FPATH machinery this
  substrate does not have, and the measurement says it does not. It appears
  exactly when the name had a **builtin or function** resolution *and* a
  **PATH hit**, and it appears with FPATH unset, set to an empty directory,
  or exported: `whence -a whence` and `whence -a typeset` — builtins with no
  file on PATH — do not print it, while `whence -a alias` and a function
  named after a PATH command do. So it is a PATH search, which this
  substrate has.
- **zsh `whence`, `which` and `where`** (dialect/zsh/whence.go): bare, `-v`,
  `-c`, `-a`, `-p`, `-w`, `-f`, with `which` as `whence -c` and `where` as
  `whence -ca` under names of their own. Each name **stops offering the
  letters its preset already decided**, which is a rule and not a
  coincidence — measured letter by letter, zsh 5.9.2, 2026-09-05:

      whence  -v -p -c -a -w -f      and -m -s -x -S
      which      -p    -a -w         and -m -s -x -S   (-c -v -f refused)
      where      -p       -w         and -m -s -x -S   (-a -c -v -f refused)

  `-c` is already on in both, so it cannot be asked for; `-a` is already on
  in `where`; and `-v` and `-f` are the two shapes `-c` displaces. `where`
  used to refuse *every* dash word, which is right for `-v` and wrong for
  `-p` and `-w`, both of which this shell answers.

  `which` is registered even though /usr/bin/which exists: a builtin
  shadowing a PATH command is what this shell does, and the five shells
  without the builtin reach the external, which knows only PATH — the same
  word asking two different questions, which is the corpus row
  `whence/which-is-whence-with-c`. #572 left it out on the grounds that
  shadowing was not what it had been asked for; #633 is that decision going
  the other way.

  **Not ksh93's builtin under the same spelling** — it is measured separately and differs in every part that
  could differ, which is the whole reason it is a second implementation
  rather than a registration of the first:

  |  | zsh | ksh93 |
  | --- | --- | --- |
  | a miss goes to | standard output | standard error |
  | an unknown letter | `bad option: -z`, status 1 | `unknown option` plus a usage line, status 2 |
  | no operand at all | silence, status 1 | the usage line, status 2 |
  | letters it has | `-c -m -w -f -s -x` | `-q` |
  | a second name | `where` | — |

  The four output shapes, each measured on an alias, a function, a reserved
  word, a builtin, a file and a name that is nothing:

  | shape | alias | function | reserved | builtin | file | nothing |
  | --- | --- | --- | --- | --- | --- | --- |
  | bare | the value | the name | the name | the name | the path | silence |
  | `-v` | `N is an alias for V` | the `type` sentence | the `type` sentence | the `type` sentence | the `type` sentence | `N not found` |
  | `-c` | `N: aliased to V` | the body | `N: shell reserved word` | `N: shell built-in command` | the path | `N not found` |
  | `-w` | `N: alias` | `N: function` | `N: reserved` | `N: builtin` | `N: command` | `N: none` |

  `-v` *is* this shell's `type`, measured, so its sentences come from the
  dialect's Diagnostics rather than from a second copy of them. The
  resolution order is alias, then whatever the shell would run, and the
  alias part is the one thing the core cannot answer: whether a word
  expands as an alias is the parser's fact rather than the runner's.

  The lookup itself is the core's, through `interp.Runner.ResolveName` —
  which is new for this, and generic on purpose. Every shell resolves a
  name the same way and none of them says so in the same words, so what a
  dialect with a builtin of its own needs is the resolution and not a
  sentence; a resolution redone in the dialect is one that can disagree
  with the shell's own.
- **ksh93 `print`** (dialect/ksh/print.go): the measured escape set with
  `\c` stopping everything, `-r`/`-e`/`-n`, `-u fd` through the runner's
  descriptor table, `-f` delegating to printf, `-s` consumed against a
  history this shell does not keep, and `-p` refused with ksh93's own
  `no query process` — the same shape `read -p` measured.
- **zsh `print -P`** (dialect/zsh/print.go): the prompt escapes over each
  operand, and it is the **same expansion** `${(%)…}` is rather than a
  second one — `interp.Runner.PromptExpand`, which is the `%` flag's own
  code under an exported name. Two tables of prompt escapes is how two
  answers to one question drift apart, and with four escapes carried the
  drift would stay invisible until a script wrote the one they disagreed
  about.

  Two passes and their order, measured both ways round because one row
  alone is satisfied by doing them backwards: the backslash escapes run
  **first** and the prompt escapes over their result, so `print -P
  '\045\045'` is one `%` — `\045` made a `%` each, and the prompt pass
  read the pair. Neither pass reads its own result: `print -P '%%%%'` is
  `%%`, not `%`. `-r` suppresses the backslash pass and leaves this one,
  so the two letters are about different passes and folding them into one
  `raw` flag would be wrong. `-f` wins over `-P` outright, measured:
  `print -Pf '%s|%n\n' A` treats the format as printf's.

  A `%` with nothing after it is **dropped**, not written — `print -P 'x%'`
  is `x` and `${(%)v}` on `x%` is `x`. This was written through, which was
  the one place the expansion said more than the shell it copies, and
  fixing it fixed both spellings at once because there is only one.

  What is carried is what the flag carried: `%%`, `%x`, `%N` and `%n`.
  Everything else is refused **by name** — `the %q prompt escape is not
  implemented` — where zsh drops an escape it does not know. Naming it is
  the choice: a prompt quietly short of a field is the kind of wrong
  answer nobody reports. The builtin's refusal is a status the script goes
  on from and writes nothing at all, because a `print` that put out the
  operands it managed and then complained would leave a script holding a
  line it could not tell apart from a whole one; the expansion's ends the
  script, which is the expansion's own rule for a word it could not
  produce. Corpus: `print/prompt-escapes-*`, `print/a-trailing-percent-is-
  dropped`, `print/a-prompt-escape-this-shell-has-not`,
  `print/the-raw-letter-leaves-the-prompt-pass-alone`.

  Note that `repl.PromptStyle` is a *second* table of these escapes, the
  one the prompt drawer reads, and it is far larger — colors, visual
  attributes, the clock. So this shell already answers `%F{196}` when
  drawing a prompt and refuses it when a script writes `print -P
  '%F{196}…'`. The two are not unified because the visual entries are the
  *dialect's* measured byte sequences and `interp` cannot read `repl`;
  hard-coding ANSI here would trade a missing answer for a wrong one. See
  #1090.
- **bash `caller`** (dialect/bash/caller.go): the stack the three
  `BASH_*` arrays already name, one step up, `NULL` and `main` where bash
  puts them.
- **zsh `zstyle`** (dialect/zsh/zstyle.go): the styles database, whole —
  set, `-e`, `-d`, `-g`, `-L`, the bare listing, and the `-s`/`-a`/`-b`/
  `-t`/`-T`/`-m` retrievals — with nothing reading it yet, because the
  completion system that is its largest reader is #801 and much larger.
  Storing what it is given is the point: a real rc file sets twenty styles
  in its first twenty lines, and a database that refuses them fails every
  one of those lines rather than the one feature behind them.

  The order is the part worth getting right, because it is what "the most
  specific pattern wins" means and both the listing and every lookup read
  it. Measured by setting the same patterns in several orders: **more
  colon-separated components first** (`:a:b:c:d:*` before `:m:n:*` before
  `:q:*`, and the one-component `abcdefgh*` after the three-component
  `:a:*`, so it is the component count and not the length of the literal
  part), then **a pattern with no metacharacter before one with**, then
  **the order they were set in**. The second key is subordinate to the
  first rather than above it: `:x:y:z:*` still precedes the exact
  `:a:b:c`. A lookup walks that order and takes the first pattern that
  matches, so `*` — one component — is the fallback it is written to be.

  Two sets of characters are involved and they are not the same set, which
  is measured one punctuation character at a time: `*?[(|#^<` make a
  pattern non-exact for the ordering and `~` does not, while `~` and `=`
  are among those `-L` quotes.
- **zsh `zmodload`** (dialect/zsh/zmodload.go): the module loader, which
  answers **per module** and never simulates one. This shell cannot load a
  compiled module and never will, so the useful question is not how to load
  one but what to say about one — and `command not found: zmodload` was the
  wrong answer to a real rc file's

      zmodload zsh/zutil || { print -P "…required, aborting"; return 1; }

  because "no such command" is not something that line can act on.

  Every zsh module is a set of named **features**, measured one module at a
  time with `zmodload -lF` after loading it: `+b:zparseopts` is a builtin,
  `+p:functions` a parameter, and `+c:`, `+f:` and `+a:` a condition, a
  function and a math function. A module loads here exactly when this shell
  already has every feature it names — the features being the ones it
  implements anyway, under their own names — so the answer is mechanical
  rather than a claim, and nothing is stubbed. `zsh/zutil` is refused today
  because three of its four builtins are missing, and the day `zparseopts`,
  `zformat` and `zregexparse` exist it will load with no change to the
  builtin: the table says what the module *is* and the shell answers whether
  it has it.

  **Not a silent success**, which is the failure this builtin is most able to
  cause: a script told `zsh/zutil` loaded and then calling `zparseopts` fails
  several hundred lines later, in a function whose caller has gone, about a
  command nobody wrote. So a refusal names the module and the features it is
  short of — `zformat, zparseopts and zregexparse are not implemented yet` —
  up to six of them, beyond which the count speaks (`33 of its 33 features`),
  because thirty-three parameter names on one line is not something a person
  reads. The reason clause is the only part that is not zsh's: zsh's is a
  dlopen error naming the module directory of the running build, which is a
  fact about a machine rather than about a shell.

  What is measured and matched exactly: a fresh shell has `zsh/main` alone
  loaded and nothing else, not `zsh/complete` and `zsh/zle` — those are
  linked into the binary and `zmodload -e` says 1 for both. The bare listing
  is one name per line and `-L` is the same set as `zmodload <name>` lines.
  `-e` asks rather than loads, silently, and every module named must be
  loaded for 0. **`-u`'s `no such module` means "not loaded", not "no such
  name"**: `zmodload -u zsh/mathfunc` is `no such module zsh/mathfunc` and 1
  for a module zsh certainly ships, and silence and 0 once it is loaded — so
  a list of the modules zsh ships, which a first version of this carried to
  tell "absent here" from "no such thing", answered no question the builtin
  asks and is gone. A load attempts every module named and does not stop at
  the first failure. `-s` silences the complaint and keeps the 1, which is
  the whole of what a script can act on. `-lF` on a loaded module with no
  features is `does not support features`, its own sentence and not the `is
  not yet loaded` an absent one gets.

  Where the two locations part: `bad option: -q` carries the builtin's name
  — `zsh:zmodload:1:` — and a load failure does not, `zsh:1:`. Measured, and
  it is one command writing both, so `interp.Runner.DiagnoseAsTheShellf`
  exists for the second kind.

  Refused by name: `-a` with `-b`/`-c`/`-f`/`-p` (autoloaded builtins,
  conditions, functions and parameters), `-A` and `-R` (module aliases), `-d`
  (the dependency table), `-m` (pattern arguments), `-I`, `-P`, and `-F`
  without `-l` — a feature here is a builtin or a parameter the shell either
  has or has not, and `+zparseopts` cannot conjure one. The twenty-two
  letters zsh's `zmodload` does not have at all are `bad option: -q` and 1,
  which is this builtin's wording and `bindkey`'s, not `zstyle`'s `invalid
  option`. Corpus: `zmodload/*`.
- **zsh `bindkey`** (dialect/zsh/bindkey.go): the line editor's key table.
  See below for why this moved out of the not-built list.

Deliberately **not** built, so the next sweep counts each as scoped rather
than missing:

- zsh `zmodload`: there are no loadable modules here; the name would be a
  table of refusals.
- zsh `autoload` (and `fpath`): function-file loading is an interactive
  startup mechanism; a non-interactive core sources files by name.
- zsh `vared` and `zle`: the line editor's, and the line editor's key
  handling is the front end's concern, not the interpreter's. `bindkey` was
  on this list for the same reason and has come off it, because the reason
  stopped being true: the front end now *has* a key table worth binding to
  (#811, #835), so `bindkey` is built, and it is built the way the rule
  says rather than against it. `repl` names the *actions* — a Widget
  vocabulary of what the editor does — and `dialect/zsh` maps this shell's
  names onto them, which it has to, because the two shells with a line
  editor disagree: the key that walks history back is `up-line-or-history`
  here and `previous-history` there. Nothing under `repl/` names a widget
  the way a shell spells it.

  What reaches the editor is an **override layer** and not a keymap: only
  what somebody rebound, with every other key going to the editor's own
  dispatch. A full keymap in `repl` would be a second copy of the editor's
  key handling, and two places to fix a key with one of them silently
  ahead of the other. It is read through a function rather than copied in
  at the start of a session, because `bindkey` is a command a person runs
  at the prompt as much as one an rc file runs.

  Three measured facts shaped the builtin. **An unknown widget is not an
  error**: `bindkey '^X^T' no-such-widget` is status 0 and silence, the
  binding is stored, and `bindkey '^X^T'` says it back — under a terminal
  with the editor loaded as much as under `-c`. So a config binding a
  plugin's widget is not a config that fails, and refusing it here would be
  stricter than the shell being modeled. **`^?` is DEL** rather than the
  0x1f that clearing the top three bits of `?` would give, which is the one
  exception to what a caret means. And **the listing is sorted by the bytes
  a key sends**, not by the caret spelling of them, which is why `^[^?`
  comes after `^[f`.

  What it does not do: the default keymaps are **this editor's keys and not
  zsh's 117**. Listing `vi-match-bracket` for `^X^B` because zsh does would
  name a widget nothing here performs, and `bindkey` is the question a
  person asks to find out what a key does — so what it answers is what this
  editor will actually do. The visible consequence is small and real:
  `bindkey '^[[3~'` says `delete-char` here and `undefined-key` in zsh,
  which reaches its Delete key through terminfo rather than through the
  keymap. `-p`, `-R`, `-N`, `-A`, `-D` and `-d` — prefix bindings, key
  ranges, and making, aliasing or destroying a keymap — are refused as not
  implemented, the distinction `whence` draws between a gap and a typo.
- zsh's own `print`: zsh has one — shared ksh ancestry — with its own flags
  and wording (`bad file number: 9` where ksh93 brackets the errno). Its
  `whence` is built now, above; `print` stays command-not-found, visible in
  the corpus's `print/` cases as the recorded difference.
- zsh `whence -m`, `-s`, `-S` and `-x`, under all three of its names: `-m`
  reads the operands as *patterns* and matches them against every name the
  shell could run, PATH included — the answer on the measuring machine was
  sixty-four lines of /usr/bin, and nothing here walks PATH; `-s` resolves a
  symlink, which would be the bare answer for every name that is not one and
  silently wrong for one that is, and `-S` is `-s` reporting every step of
  the chain rather than the last; `-x` sets the tab width of a printed body.
  Each is refused as not implemented rather than as unknown, the same
  distinction `compgen` draws between an action a shell lacks and a typo.
- zsh `setopt` names of the **recorded** kind: 149 of the 185 are recognized,
  remembered and reported without being acted on. See "zsh's option names".
  (This line read 157 while the table above read 150; neither was the count
  the table produces. It is now counted from the constructors.)
- zsh `emulate -L`: function-local emulation needs a restore-on-return seam
  the runner does not have; refused out loud rather than silently made
  global. `emulate csh` records the mode and changes nothing it could —
  csh's differences are not modeled anywhere else either.
- ksh93 `print -v`/`-C` and `whence -f`: value quoting, compound output and
  the function skip; each is refused as not implemented rather than unknown,
  which would be the worse answer. `whence -a` was on this list and is
  built now, above.

  Two divergences it inherits rather than introduces, both visible at
  `whence -v` and `whence` already: this shell's builtin set is not ksh93's
  (`sleep` is a builtin there and a PATH command here, so `whence -a sleep`
  is one line rather than three), and `type`/`whence -v` says `is a shell
  builtin` where ksh93 says `is a special shell builtin` for the special
  ones. Neither is `-a`'s to fix.
- ksh93 `hist`: interactive history editing, and there is no history.
- bash `bind`: readline's. The seam zsh's `bindkey` reaches the editor
  through is the core's rather than zsh's, so this is now a matter of
  measuring readline's names and wordings rather than of architecture.
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

## `compgen`, and how much of a completion generator a shell owes

Issue #476: `compgen` was registered in the core builtin map with its
measurements written into the file's comments and nowhere else — no spec
text, no rows in `measurements.md`, no corpus cases. That is the shape
`CLEANROOM.md` rules out, because a claim nothing checks is a claim that
rots quietly, and this one had. The record is here now and the corpus
rows are under `compgen/`.

Measured 2026-09-05, panel and machine as `oracle.md` (bash 5.3.15,
bash 3.2.57, bash-as-sh, dash, ksh 93u+ 2012, zsh 5.9.2).

**One shell has it.** `type compgen` answers `compgen is a shell builtin`
in bash 5.3, bash 3.2 and bash invoked as `sh`; dash, ksh93 and zsh have
no such name, and running it there is a command that is not found at 127.
So it is registered in the core and taken away by the three without it,
the way `enable` is — the same shape `semantics.md` records above for
builtins that belong to one shell.

**What is generated, and why only this much.** `compgen` answers from
the shell's own knowledge in two places and nowhere else:

| spelling | generates |
| --- | --- |
| `-A builtin`, `-b` | the names of this shell's builtins |
| `-A function` | the names of the functions defined so far |

Those are the two questions the interpreter already holds the answer to.
Homebrew's `brew` asks the first of them, `compgen -A builtin`, to check
that none of the shell's own commands has been shadowed, which is why the
builtin exists at all.

The surrounding behavior is measured and matches bash exactly:

    compgen -A builtin retu       →  return             status 0
    compgen -A builtin zzzznosuch →  nothing            status 1
    compgen foo                   →  nothing            status 0
    compgen -A nosuchaction x     →  invalid action name, status 2

The two statuses are the pair worth stating together: **1 means asked and
empty, 0 means never asked.** An empty completion is a failure because
the question a completer puts is whether there is anything to offer, and
that reads backwards from the usual meaning of a command that printed
nothing without complaining.

**A wrong claim the corpus would have caught.** The letter table said
`-u` was short for the function action. It is not: bash's short letters
are `abcdefgjksuv` — its own usage line — and **none of them is
`function`**, whose only spelling is the long `-A function`. `compgen -u`
in bash lists *user* names, so `compgen -u f1` with a function `f1`
defined printed `f1` here and nothing at 1 in bash. That is the invisible
direction of wrong: an answer rather than an error, from a builtin whose
whole job is to answer. Fixed with this section, and pinned by
`compgen/there-is-no-short-letter-for-function`.

The word rule was wrong the same way and is fixed with it: the **first**
non-option word is the one matched against and the rest are ignored, so
`compgen -A builtin ret re` answers for `ret`. Both orders are measured,
because one alone cannot tell first-wins from last-wins.

**Out of scope, recorded rather than silent.** Everything bash's
`compgen` does beyond the two generators above is refused out loud, and
the refusal is deliberate on the rule the `set -o` table follows — an
action we cannot generate is a promise we cannot keep:

- **The actions bash has and this shell does not generate** — `alias`,
  `arrayvar`, `binding`, `command`, `directory`, `disabled`, `enabled`,
  `export`, `file`, `group`, `helptopic`, `hostname`, `job`, `keyword`,
  `running`, `service`, `setopt`, `shopt`, `signal`, `stopped`, `user`,
  `variable`. Each is `not implemented` at 2, and an action bash does
  not have either is `invalid action name` at 2. The distinction matters
  to a script: the first is a shell that is missing something and the
  second is a typo. Measured divergence, recorded in
  `compgen/an-action-this-shell-does-not-generate`: bash *generates*
  these and answers 1 for no match where this refuses at 2.
- **The letters that go with them** — the same split. `-u` and the rest
  of `abcdefgjksuv` are `not implemented`; a letter outside that set is
  `invalid option`, which is bash's own wording. bash also prints its
  usage line after the complaint and this does not, which is a
  presentation difference and not a behavioral one.
- **The generators that run something** — `-F function`, `-C command`,
  `-G globpat`, `-W wordlist`. Each is a hook for producing words from
  outside the shell's own tables, and each would need the completion
  machinery this core does not have.
- **The filters and decorations** — `-X filterpat`, `-P prefix`,
  `-S suffix`, `-o option`, `-V varname`. These shape a word list rather
  than generate one, and there is nothing yet for them to shape.
- **A bundle naming more than one action** — `compgen -bu` unions two
  generators in bash. Here the letters are read in order and the last one
  wins, which is only ever reachable with a letter that is refused
  anyway, since `b` is the only one implemented. Recorded so a second
  implemented letter does not arrive without the union arriving with it.
- **`complete` and `compopt`.** The rest of programmable completion, and
  the reason `compgen` is the interesting third of it: `compgen` answers
  a question, where the other two register and adjust completion
  specifications for an interactive line editor this core does not own.

## `mapfile`, and a delimiter that is not a character

Issue #488: `mapfile` — and `readarray`, the same command under its other
name — reads a stream into an indexed array, one element per delimiter.
bash alone has it; dash, ksh93 and zsh answer command-not-found, and so
does bash 3.2, which predates it. Measured 2026-09-05 against bash 5.3.15,
bash 3.2.57, dash, ksh93u+ 2012-08-01 and zsh 5.9.2; the corpus rows are
`mapfile/`, `readarray/`.

It earns a builtin rather than a prelude function for the reason `read`
does: `printf … | while read` runs its loop in a subshell and the variable
it set is gone at the other end of the pipe, and an array is the case where
that hurts most. `mapfile -t arr < file` puts the array in the shell that
asked for it.

The letters, each measured:

- **`-t`** strips the delimiter from every element. Without it the
  delimiter is kept: `printf 'a\n' | mapfile x` leaves `${#x[0]}` at 2,
  and `printf 'a:' | mapfile -d : y` leaves `${#y[0]}` at 2 likewise.
- **`-d`** renames the delimiter, and only its argument's **first byte**
  speaks — `-d xy` splits on `x`. An **empty** argument is not "no
  delimiter" but **NUL**, which is the whole point of the letter:
  `find -print0 | mapfile -d '' -t` is the one file-name-safe read a shell
  has. It is a genuine special case rather than a first-byte reading of the
  empty string, so the code cannot express it as `word[0]`.
- **NUL is stripped from the element whether or not `-t` was given.**
  `printf 'a\0' | mapfile -d '' x` leaves `${#x[0]}` at 1, against the 2 a
  `:` delimiter gives — a value cannot carry a NUL, so keeping it was never
  on offer.
- **`-n`** caps how many elements arrive, and **`0` is no cap** rather than
  none, so an absent `-n` and `-n 0` land in the same place. The cap is
  measured on the *stream*, not only the array: `printf '1\n2\n3\n' |
  { mapfile -t -n 1 a; read rest; }` leaves `rest` at `2`, so the read
  stops at the cap instead of draining and discarding. This
  implementation reads a byte at a time for that reason.
- **`-s`** throws away that many elements before the first one kept.
- **`-O`** writes from a given subscript into whatever the array already
  holds; without it the array is replaced outright.
- **`-u`** reads a descriptor from the shell's own table rather than
  standard input — `exec 3<f; mapfile -u 3 -t arr` — which is how the
  command is used without a pipe putting it in a subshell. A number nothing
  is open at is `mapfile: 9: invalid file descriptor: Bad file
  descriptor`, status 1.

The operand is the array name, `MAPFILE` with none, and operands after the
first are ignored. A name that is not an identifier is `` `bad name':
not a valid identifier `` and a name already declared associative is
`h: not an indexed array`, both status 1. An unknown letter is the
dialect's usage refusal at status 2, carrying whichever of the two names
the script used — `readarray -q` says `readarray`.

**`-C` and `-c` are deferred**, not silently accepted: the callback letters
run shell code every *quantum* elements, which is a second evaluation
context inside a read, and the dialect's letter table refuses them by name.
Parsing and ignoring them would be the wrong answer, because a script that
passes `-C` is asking for something to happen.

## The declaration long tail: letters, listings, and one letter with an axis inside it

Oracle runs, 2026-09-04, bash 5.3, dash, ksh93u+, zsh 5.9.2. The corpus
rows live under `declare/` and the `set/bare-set-*` and `command/capital-v-*`
ids.

`declare`/`typeset` and `local` read the dialect's letters, in the same
spelling `ReadOptions` uses — `Semantics.DeclareOptions` and
`Semantics.LocalOptions`:

    declare/typeset
      bash   aAfFgilprux   plus -I -n -t, unimplemented here
      zsh    aAfFgHilpruUx plus floats, padding, ties…, unimplemented;
                           -F taken in silence, see below
      ksh93  aAilprux      plus -f -F -b -n… and its own -H, unimplemented
      dash   —             no typeset at all
    local
      bash   aAgilprux     plus -f -F -I -n -t, unimplemented
      zsh    aAHilpruUx
      dash   (none)        `local -r x` declares a name `-r`, then refuses
                           it: `local: -r: bad variable name`, fatal
      ksh93  —             no local at all

What the letters mean, where measured to agree, is implemented once:
`-f` says the functions back and `-F` names them (`declare -f name` per
function bare, the name alone once operands narrow it — and a float's
precision in zsh and ksh93, where zsh's is taken in silence and ksh93's
is refused as missing; see "A letter taken in silence" below);
`-g` declares a global from inside a function; `-l`/`-u` fold what is
assigned to the name, the declaring assignment included. zsh stores the
raw text and folds on *expansion* instead — every read agrees with the
other two shells, and only its `typeset -p` betrays the difference by
listing the raw value, which is deliberately not modeled.

**A letter taken in silence: zsh's `-F`** (#1037). The letter is a
float's *precision* there rather than bash's function listing, and this
engine has no float attribute to record it in. Measured 2026-09-06: real
zsh answers a bare `declare -F` with **0 and not one byte**, on either
stream, and answers the same for `typeset -F`, for `declare -F f` where
`f` is a function, and for `declare -F nosuch`. Only `-f nosuch` is 1,
which is the letter we already had.

We refused it at 2, and that is the wrong answer in the direction that
matters: `declare -F` is how a state capture asks what functions exist,
so a caller reading the status recorded a shell with no functions, and a
caller reading the stream got a complaint from a shell that has nothing
to say to it. `Semantics.DeclareOptionsWithoutEffect` is the quiet
counterpart of `Diagnostics.UnimplementedOptionLetters`, and the choice
between them is which lie is smaller: a refusal puts a sentence into
output a caller shows a person and ends a script under `set -e`, where
silence changes only how a value is printed back.

What it costs is written down rather than hidden: `typeset -F v=1.5`
leaves `1.5` here where that shell prints `1.5000000000`, since the
precision is the whole of what the letter does. It is still better than
the refusal it replaced, which set the name to nothing at all. And the
silence is silence, not indifference — a declaration carrying only the
letter is still a declaration, so it does not fall through to the bare
word's listing of every parameter, which would be the one answer worse
than refusing.

zsh's `-E` — the same float family, spelled for scientific notation —
stays refused. Nothing measured asks for it, and the case for silence
rests on `-F` being the letter a *function* listing is spelled with
somewhere else, which is what makes refusing it a wrong answer to a
question callers really ask. ksh93's `-F` stays refused for the same
reason: its `declare` does not exist at all, so no caller reaches the
letter there.

**`-H` is one letter and two attributes, and only one of them is
built.** Measured 2026-09-06. In zsh it hides a name's *value* from every
listing that would write one: `typeset -H h=hid` leaves `$h` reading
`hid`, and `typeset -p h` answers `typeset h` with no `=hid` after it —
the attributes still speak, the value does not. `+H` gives it back. It
reaches the other listings too: `export -p` writes `export ex`, bare
`export` writes `ex`, and a bare `set` writes `zzh` where an ordinary
name on the same listing is `zzv=plain`. So it is a listing attribute
and nothing else, and it is implemented as one — recorded against the
name and consulted where a value would be written, not accepted and
dropped.

ksh93 has the same letter for a different attribute: file name mapping,
which does *not* hide. `typeset -H h=hid` there lists back as
`typeset -H h=hid` — the value **and** the flag — and a bare listing
calls the attribute `filename`. That letter stays refused by name; it is
not the zsh one under another spelling, and modelling the two as one
attribute with two renderings would be inventing a shared thing that is
not there. bash refuses `-H` outright, under `declare` and `typeset`
alike, with its usage line and 2; dash has no such builtin. There is
therefore no axis — only a letter one dialect has, in
`Semantics.DeclareOptions` and `LocalOptions`. Corpus:
`declare/hide-attribute-*`, `set/bare-set-and-a-hidden-value`.

**`-U` keeps the first occurrence of each element, and it is one shell's
letter.** Measured 2026-09-06 against zsh 5.9.2. `typeset -U a=(1 1 2 2 3
1)` reads back `1 2 3`, and because the attribute belongs to the *name*
rather than to that assignment, `a+=(2 4 4)` reads back `1 2 3 4` — the
append is deduped against what is already there, which is the whole
reason `path=( new "${path[@]}" )` in an rc file does not grow a copy of
`new` every time it runs. `+U` takes the attribute away and leaves the
elements standing, so a later `c+=(1)` keeps its duplicate: the dedupe
happens when a value is written, not when one is read.

Which of two equal elements survives is measurable, and it is the
earlier one, where it stands: `b=(1 2 3); b=(3 "${b[@]}")` is `3 1 2`,
not `1 2 3`. Applying the letter reaches a value the name already holds
— `b=(1 1 2); typeset -U b` is `1 2` — the same rule the integer and
case letters follow. A scalar takes the letter and nothing happens to
it: `typeset -U s=a:b:a` is `a:b:a` unchanged, which matters because the
letter is used almost entirely on `path` and `fpath`, whose scalar
halves *are* colon lists. And the array is deduped in the reading the
dialect gives it and stored back dense, which is what a gap shows:
`typeset -U f=(1 2); f[5]=1` leaves three elements — `1`, `2` and one
empty, the two the gap made having deduped to one and the trailing
duplicate gone — where the same two lines without the letter leave five.

There is no axis. bash refuses `-U` under `declare` and `typeset` alike
with its usage line and 2, and ksh93u+ has no such letter at all:
`typeset: -U: unknown option`, and because its `typeset` is special the
refusal ends the script. Its own lower-case `-u` is a different
attribute and stays what it was. So the letter lives in
`Semantics.DeclareOptions` and `LocalOptions` — `local -U u=(1 1 2)`
dedupes too — and the attribute is recorded against the name and
consulted at the one place an array goes back into the store. It is said
back on a listing last of all the letters, after export: `typeset -arxU
A1=( 1 2 )`, `export -iU n1=5`. Corpus:
`declare/unique-attribute-*`.

**An attribute added to a name that already holds a value keeps it, and
re-reads it.** A separate rule from the one above, and the one that
makes `-H` safe: `typeset -H h` on an existing `h` must hide the value,
not destroy it. What the name holds survives in all four shells that
spell the builtin — and in zsh and ksh93 it is read back through the
attribute that has just arrived, so `v=5+2; typeset -i v` is 7 and
`d=MiXeD; typeset -u d` is `MIXED`, where bash leaves both alone. A
compound value is not re-read anywhere: `arr=(a b); typeset -u arr`
stays `a b`. This is not an assignment, so a readonly name meets no
refusal — `typeset -r r=1; typeset -i r` is 1 with status 0. Corpus:
`declare/integer-attribute-added-to-a-name-with-a-value`,
`declare/case-attribute-added-to-a-name-with-a-value`.

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
name off the line exactly as their `type` does. Status, prefix rule and
stream are `type`'s own (`TypeNotFoundStatus`, `TypeNotFoundUnprefixed`,
`TypeNotFoundOnStdout`).

**A name `type` could not account for goes to a different stream in each
half of the panel** (`Diagnostics.TypeNotFoundOnStdout`). bash and ksh93
write it to standard error; dash and zsh write it to standard output.

    $ type nope 2>/dev/null      $ type nope 1>/dev/null
    bash   (nothing)             bash   bash: line 1: type: nope: not found
    dash   nope: not found       dash   (nothing)
    ksh93  (nothing)             ksh93  ksh: whence: nope: not found
    zsh    nope not found        zsh    (nothing)

Two shells treat the line as an *answer* — part of what the reader asked
`type` for — and two treat it as a complaint about the request. It is a
question of its own: the status is settled separately
(`TypeNotFoundStatus`, 127 in dash and 1 elsewhere) and so is the prefix
(`TypeNotFoundUnprefixed`), and nothing about either predicts the stream.

The consequences are the ones a wording difference never has.
`p=$(type -p nope)` captures the line where the shells report it and
captures nothing where they complain; `type nope 2>/dev/null` shows it in
one half and hides it in the other. It is also why a multi-name
invocation reads in order under the reporting shells: dash has no option
letters on `type` at all, so `type -t f cd if ls` reads five names and
prints five lines — two misses and three answers — in one stream, in the
order they were asked for. Splitting them across two streams leaves a
reader to interleave them, and under a pipe leaves them unordered.

`command -V` shares the answer, being `type`'s question under another
name. ksh93's `whence`, which is the builtin its `type` is spelled from,
complains on standard error to match.

Oracle runs, 2026-09-05, bash 5.3.15, dash, ksh93u+ 2012-08-01, zsh
5.9.2, each stream redirected separately. Corpus rows
`type/a-name-that-is-nothing`, `type/p-on-a-name-that-is-nothing`,
`type/capital-p-on-a-name-that-is-nothing`, `type/dash-t-names-the-kind`,
`type/dash-t-on-nothing-is-silent-failure`, `type/a-lists-a-keyword`,
`type/f-skips-or-prints-the-function`,
`type/several-names-and-a-double-dash` and
`command/capital-v-a-name-that-is-nothing`. The rows were scoring as
agreement until #462 stopped merging the two captures before grading, and
they are the reason it does not.

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

**`jobs`' option letters** were read and thrown away until #469. No case
in the corpus passed the builtin an option, so `jobs -p` printed the
whole listing and `kill $(jobs -p)` killed nothing — the failure mode an
ignored option always is, and a sharper one here than elsewhere: a name
`IsBuiltin` recognizes never reaches the exec seam, so a builtin that
misreads its options *shadows* the program on the machine rather than
falling through to it.

Oracle runs, 2026-09-05. The letter sets are not nested, so they are
`Semantics.JobsOptions` rather than one string in the engine:

| letter | dash | bash 5.3 / 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `-l` | listing + id | listing + id | listing + id | listing + id |
| `-p` | ids alone | ids alone | ids alone | listing + group id |
| `-r` | illegal option | running only | unknown option | running only |
| `-s` | illegal option | stopped only | unknown option | stopped only |
| `-n` | illegal option | changed since | since last notice | bad option |
| `-x` | illegal option | run a command | unknown option | bad option |
| `-d` `-z` `-Z` | illegal option | invalid option | unknown option | zsh's own |
| a jobspec | yes | yes | yes | yes |

Implemented: `-l`, `-p`, `-r`, `-s`, and jobspec operands. Three things
came out of the measurement that the issue did not have:

- **`-l` and `-p` are exclusive and the last one given wins** —
  `jobs -pl` is the long listing and `jobs -lp` the ids, in dash, bash
  and ksh93 alike. Unanimous, so not an axis.
- **Operands settle their own order and keep their own numbers.**
  `jobs %2 %1` lists 2 then 1 in all five, including the two whose bare
  listing starts from the newest — so the newest-first rotation applies
  only to a listing nobody asked particular jobs for. And each row
  carries the *job's* number: `jobs %2` printed `[1]` here before this,
  because a listing of one job counted from the start of the slice it
  had been handed.
- **A bad spec is reported after the rows written before it**, which was
  the other way around.

Two axes, asked where the panel splits:

- `JobsPidsOnlyOption` — whether `-p` is the process ids and nothing
  else. dash, bash and ksh93 yes; zsh reads the same letter as the job's
  process *group* and prints its ordinary rows, which is why
  `kill $(jobs -p)` is a bash idiom rather than a portable one.
- `JobsStateFiltersAccumulate` — `jobs -rs`, both filters at once: zsh
  lists a job in either state, bash lets the last letter decide. Asked
  only when both letters arrive, because one alone means the same thing
  in both, and the two dialects without the letters cannot reach it.

And one rule that is neither: **a listing that was not a listing of
states does not finish with a job.** Measured in bash, `jobs -p` and
`jobs -r` both leave a job that has ended for the next bare `jobs` to
report, where `jobs` and `jobs -l` consume it.

Deliberately out of scope, refused by name through
`UnimplementedOptionLetters` rather than accepted and dropped:

- **`-n`** (bash, ksh93) needs a record of what the shell has already
  reported, *and* the two shells do not agree on what it means: a job
  that has only just started is a change in bash and is not one in
  ksh93. Two features behind one letter.
- **`-x`** (bash) is not a listing at all — it runs a command with the
  job specs among its arguments replaced by process ids.
- **`-d`, `-z`, `-Z`** (zsh) are the job's directory and the process
  title, neither of which this engine holds.

Two divergences are recorded rather than modeled. A job with no process
of its own — a builtin or a compound command, which runs on a cloned
Runner here where a real shell forks — is left out of a `-p` listing
entirely, because that listing is written to be *used* and a `0` in it
would send `kill` at the whole process group. And ksh93 alone finishes
with a job after `jobs -p`, where dash and bash keep it for the next
listing; the engine follows the two that agree.

### A background job and the end of the shell

**A job whose remaining work is this process's does not survive this
process ending.** #519, decided rather than fixed, and the decision is
here because it is a limit of the engine's shape rather than a bug with
a patch.

Measured 2026-09-06, each line run as `-c` and the file read a second
later:

| the job | bash 5.3 · 3.2 · dash · ksh93 · zsh | this shell |
| --- | --- | --- |
| `sh -c '…' &` — one external command | survives | **survives** |
| `echo … > f &` — a builtin, finished at once | survives | **survives** |
| `( … ) &` — a subshell | survives | **lost** |
| `a && b &` — an and-list | survives | lost |
| `{ …; } &` — a brace group | survives | lost |
| `f &` — a function | survives | lost |
| any of them with `wait` before the end | survives | **survives** |

A real shell forks, so a job is a process that outlives its parent by
construction. Here a job is a goroutine on a cloned Runner in *this*
process, so the parts of it that are the shell's own work end when the
process does. A job that is one external command is already a real
child and already survives — which is why the row above it and the row
below it are the pair that matters: the difference is about the shell
logic in a job, not about `&`.

**The failure is half-done rather than absent, which is the sharp
version.** Measured: after `(sleep 3; echo LATE > out) &` and the shell
exiting, the `sleep` is reparented to init, runs its full three seconds
and exits — and the `echo` after it never runs. A process is left
behind *and* the work is lost.

**The shell does not wait, and that is right.** Measured, both this
shell and bash return in 0.02s from `-c '(sleep 0.4; …) &'`. Waiting for
outstanding jobs at `Finish` would stop losing the work and is cheap,
and it is what the issue lists first — but no shell in the panel waits,
and a shell that took four seconds to exit after `sleep 4 &` would be
wrong in a way a person notices every time.

**Detaching the external process is already done.** That is the issue's
second option, and measuring it is what removed it from the list: the
common `cmd &` shape needs nothing, because it is already a process of
its own that the shell neither waits for nor kills.

So what is left is the third option, taken here: say so. Fixing it
needs a job to *be* a process, which needs a fork this runtime cannot
safely do or a re-exec of a shell binary — and `interp` is a library,
so there is no binary it may assume it is. That is a front end's change
if it is anyone's, and it is not one to make blind.

**A related bug found while measuring this, and fixed here.** A job's process
id was its *latest* process rather than its first: a job running more than one
external command reached the assignment again, while the shell that started it
had already read the field — `background` releases on `<-job.ready`, which the
first write closes, and every later write was unsynchronized. `go test -race`
reports it. The first process wins now, which is also the better answer: this
shell has no subshell process to name, so the id is an approximation either
way, and one that stays put is worth more to `kill %1` and `jobs -l` than one
that tracks whichever command the job has reached.

**And one it did not fix, filed as #1003 and fixed there.** `&` did not return
at all when the job blocked *before* starting any external command —
`(read x < a-fifo; :) & echo NOW` printed nothing here and prints the marker at
once in all six shells in the panel, which fork before they open anything.
Every shape of job did it, a lone external command included, because the
blocking was in the *redirection* and a redirection is opened before the shell
knows whether the command is a builtin, a function or a program.

The settling now has a third trigger beside a process having started and the
job having ended: **a background job is settled without a process id when it is
about to open something that may never open** — a named pipe with no peer,
which is the one shape whose open waits by construction. Nothing has started there
and while the job stands at that open nothing will, so zero is the truthful
answer and not a lost one; it is the same zero this shell already reports for a
background builtin or compound command.

Only where the open can really wait, and the stat is the whole reason: settling
before *every* redirection would cost a real answer, since `sleep 0.3 > log &`
reports the sleep's process id here and a latch that fired on an ordinary file
would report zero. A character device was in the test for one draft, on the
reasoning that a terminal's open can wait, and it was a regression rather than
a completeness — `/dev/null` is a character device, so `sleep 1 > /dev/null &`
began reporting no process id at all. The devices whose open really does wait
are a terminal held by another process group and a serial line with no carrier,
and a background job here is a goroutine in the shell's own process group, so
neither is reachable. The trade in the other direction is that a job settled at a
blocking open keeps its zero even if a writer arrives later and the job goes on
to run a program — a pid that arrives after the shell has stopped waiting is
dropped rather than recorded, because writing the field and closing the channel
that publishes it are now one `sync.Once` and not two. That is what makes every
reader's `<-job.ready` a synchronization point rather than a hope. `$!` for a
blocked job is therefore `0` where the panel answers with the pid of the fork it
made, which is the same knowing difference as the row below: there is no fork
here to name.

Five corpus rows ask the question, where none did before:
`jobs/a-background-job-outlives-the-shell` records the difference,
`jobs/a-background-external-command-outlives-the-shell` records the
half that already works,
`jobs/wait-brings-a-background-job-back-before-the-shell-ends` records
the workaround a script has, and
`jobs/an-ampersand-returns-before-the-job-opens-a-fifo` and
`jobs/an-ampersand-returns-when-an-external-commands-redirection-blocks`
record that `&` comes back — which is a row that timed out before.

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
divergence is deliberate: `DIRSTACK` holds only the pushed entries where
bash's also mirrors the current directory.

**Rotation and `dirs`' letters** are the half that landed behind that
single success-path pin and were therefore never exercised (#468).
Oracle runs, 2026-09-05, and the first thing they settled is that
**bash and zsh agree about rotation exactly** — twelve rotations
compared, entry for entry and standing directory for standing
directory, so there is no axis here at all:

- `pushd +N` counts the *current* directory as entry 0 and turns the
  stack until entry N is the one the shell stands in. `pushd -N` counts
  from the other end, so `-0` is the oldest entry.
- `popd +N` takes an entry out where it stands and leaves the shell
  where it is; only `+0` — which is what a bare `popd` means — moves it.
- `dirs -c` empties the stack, `-l` writes `$HOME` out in full where the
  plain listing abbreviates it, `-p` writes one entry to a line and `-v`
  numbers them.

The wordings and the option parsers do not agree, and both dialects have
their own prelude, so those are written twice rather than switched on:

| | bash 5.3 | zsh |
| --- | --- | --- |
| index out of range | `pushd: +9: directory stack index out of range` | `pushd: no such entry in dir stack` |
| nothing pushed | `pushd: directory stack empty` | the same sentence as above |
| `dirs` out of range | `dirs: 9: …` — the sign dropped | no such form |
| a letter it has not | `-q: invalid number` + usage, at 2 | `bad option: -q`, at 1 |
| `dirs` letters | one to a word: `-lv` is a malformed index | they bundle |
| `-v` numbering | right-aligned in two columns, two spaces | bare, then a tab |
| bare `pushd`, nothing pushed | `pushd: no other directory`, at 1 | goes to `$HOME` and pushes, at 0 |
| a `dirs` operand | an index into the stack | a *new* stack |

dash and ksh93 have no `pushd`, `popd` or `dirs` at all — three names
that resolve to nothing and exit 127. Recorded as absence rather than as
a divergence, which is what the corpus rows show in those two columns.

Deliberately out of scope, and refused by name rather than read as a
directory called `-n`: **`pushd -n` and `popd -n`**, which do the stack
work and stay where they are. Recorded and not implemented: zsh's
`pushd old new`, the substitution form; zsh's `dirs -c` in company with
a printing letter, which that engine measures as doing nothing at all;
and bash's `DIRSTACK` as an assignable variable.

**How a prelude function says where it is** (#603). The rotation work
made this visible rather than causing it: a `pushd` whose directory does
not exist used to report `cd`'s complaint, located inside the prelude,
where both shells that have the builtin say
`pushd: /nope: No such file or directory` at the caller's line. The
status and the untouched stack were right and the sentence was the
function's, so five corpus rows pinned the status and discarded the text.

Two rules close it, and they are one fact seen from two sides.

- **A function the prelude defined is the shell speaking.** While one
  runs, every diagnostic raised inside it is located where the *script*
  called it and named after it — whatever raised it. So `cd`'s refusal
  arrives as `pushd`'s, which is what both shells print, and the name is
  the one the script wrote: a prelude helper another prelude function
  calls does not take it over. Remembered by declaration rather than by
  name, so a script redefining `pushd` gets the ordinary treatment of a
  function that shadows a builtin — `cd`, at the line in its own body.
- **`diagnose` is how it raises one of its own.** One line in the
  prelude, `diagnose "directory stack empty"`, renders as
  `sh: line 3: popd: directory stack empty` under one dialect's location
  style and `sh:popd:3: directory stack empty` under the other's, with
  the shell text saying neither. It is the only command in `interp` that
  exists for a prelude rather than for a script, and the lookup answers
  it only while a prelude function is on the stack — a script running the
  word gets the `command not found` the dialect it is written for would
  give it, so a dialect gains no builtin by having a prelude.

A second line of a refusal is still a plain `echo`, which is measured
rather than a shortcut: bash locates only the first line, so
`dirs: usage: dirs [-clpv] [+N] [-N]` follows the located complaint with
no prefix of its own.

What this does not do is make such a function a builtin in any other
respect. `type pushd` still answers `function`, because it is one; the
question the seam answers is whose diagnostic it is, which is the
question a location already asks.

**Whose function a listing is asking about** (#1035). The same fact is
what a *listing* needs, and it was needed second: `declare -F` against
this dialect named `__dirs_rotate`, `dirs`, `popd` and `pushd` beside
the two functions the snippet had defined. That is not a cosmetic
surplus. A listing is how a caller captures a shell's state — an agent
harness starts a login shell once, writes the functions, the shell
options and the aliases to a file, and sources that file ahead of every
later command — so a leaked name is recorded as the *person's*, handed
to every later shell as though somebody had written it, and `pushd` is
redefined on top of the prelude's on every command. Real bash has the
three as builtins and lists none of them.

The rule is the one above, asked by name instead of by declaration:

- **A listing of the functions is the script's own.** `declare -f`,
  `declare -F`, `typeset -f` and the function half of a bare `set`
  leave out what the prelude defined. Deliberately no second record of
  prelude-ness: the declaration is what is compared, so a script that
  writes its own `pushd` is in the listing from that moment — its
  function is its own, by the same rule that moves the diagnostic's
  voice back to it. An empty listing is 0, not the 1 a name that is no
  function answers.
- **A name asked for is still answered.** `declare -F pushd` writes the
  name and `declare -f pushd` writes the body, because there is a
  function there and `type pushd` already says so. Real bash refuses
  both with 1, having a builtin instead, and this is the divergence
  taken knowingly: the alternative makes the prelude's implementation
  unreachable by name and gives the shell two answers to whether
  `pushd` is a function. One notion of prelude-ness, one answer.

**Whose function a removal is asking about** (#1082). The third caller,
and the one that lost something: `unset -f pushd` deleted the
declaration and took the directory stack away for the rest of the
session, at a silent 0, with the failure surfacing later and elsewhere
as `command not found`. `unset -f name` ahead of defining one is an
ordinary defensive line in an rc file — a widely used zsh plugin
manager writes exactly it — and `unset -f` over a list of names is what
a state capture's cleanup does.

Measured across the panel, and the part that matters is unanimous:
`dirs`, `popd` and `pushd` are builtins in the two shells that have
them, so there is no function of that name to remove and **the name
still works afterwards**. `unset -f pushd; pushd /tmp` pushes in bash
and in zsh. What the two disagree about is only whether the *unset*
says anything, and that was already asked — bash, bash 3.2, bash-as-sh,
dash and ksh93 are silent at 0, and zsh writes the same
`no such hash table element: pushd` it writes for any name it does not
hold, at 1.

So the rule needs no new field and no new record of prelude-ness:

- **A name the prelude declared is not the script's to remove.** It is
  the "not defined" case, answered by
  `Semantics.UnsetFunctionReportsMissing` and worded by
  `Diagnostics.UnsetFunctionNotFound` — a name this shell provides is
  exactly a name the script never defined. The declaration is what is
  compared, which is the whole of it.
- **Removing a redefinition gives the name back to the shell.**
  `pushd() { echo mine; }; unset -f pushd; pushd /tmp` pushes in both
  shells that have the builtin, because the function shadowing it went
  and uncovered it. A dialect written as shell has one function table
  where they have a builtin table and a function table, so the boundary
  is reconstructed by hand: the prelude's declaration stands again. It
  is silent at 0 in all six, zsh included, because the name really was
  a function — and a second `unset -f` is then answered exactly as the
  first row is, which is what separates "gave the name back" from
  "kept the script's" and from "deleted it for good".

This is the same call "a name asked for is still answered" made, seen
from the other side. Refusing the unset outright would give the shell
two answers to one question; answering it as a name the script does not
have is one answer, and it is the one every shell in the panel behaves
as though it had given.

**Whose function a completion generator is offered** (#1081). The last
of the four, and the one that took longest to find, because it does not
walk the runner's table: `compgen -A function` generated through the public
`Runner.FuncNames`, which is also what the line editor completes from.
Narrowing *that* would cost `pushd` its completion at a prompt, since
`BuiltinNames` does not name it either.

Measured against real bash 5.3:

| snippet | real bash | ours before |
| --- | --- | --- |
| `f(){ :; }; compgen -A function` | `f`, 0 | four more names, 0 |
| `compgen -A function pu` | nothing, 1 | `pushd`, 0 |
| `compgen -A function pushd` | nothing, 1 | `pushd`, 0 |
| `pushd(){ :; }; compgen -A function pu` | `pushd`, 0 | `pushd`, 0 |

The middle two are the silent kind: a completer asks whether there is
anything to offer, and it was told yes and handed a name the person
never defined.

- **A generated function listing is the script's own.** The answer is
  two sets and one predicate, not a filter added to one accessor:
  `Runner.FuncNames` stays every function *callable*, which is what an
  embedder's line editor wants, and every *listing* — `declare -f`,
  `declare -F`, the function half of a bare `set`, and now
  `compgen -A function` — asks the script's own. Both come from
  `speaksForTheShell`, so there is still one notion of whose a function
  is. The last row is the whole of why: the word is a prefix filter over
  the listing, so a redefinition is offered again, and a fix that
  suppressed the three names outright would pass the second row and fail
  the fourth.
- **The word is a filter and not a named operand.** `compgen -A function
  pushd` answers nothing where `declare -f pushd` answers the body, and
  real bash draws the same line from the other side — it refuses
  `declare -f pushd` with 1 *and* answers the `compgen` with nothing,
  both meaning "no function called pushd". "A name asked for is still
  answered" is about an operand naming one thing, not about a prefix
  that happens to spell one.

Measured and deliberately **not** changed: `compgen -A builtin pushd`
answers `pushd` at 0 in real bash and nothing at 1 here, and
`compgen -A builtin` names `dirs`, `popd` and `pushd` there and none of
them here. Adding them would match bash on that row and give this shell
two answers to whether `pushd` is a builtin, since `type pushd` says
`function` — which is the one thing the rule above exists to prevent. A
miss at 1 is `compgen`'s own "nothing to offer" rather than a fabricated
name, and the divergence is the same one `type` already carries. Whether
a dialect should instead hold a table of the names it *presents* as
builtins — which would move `type` too, and is therefore not a local
change — is left open.

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

## Asking about one slot in the jobs table, without reading a listing

The jobs table's slot lifecycle produced no corpus case for a long time
(#500, #783), and the reason is that **a listing is not stable enough to
grade**. `jobs` with no operands prints a `Done` row for a reaped job on
some runs of the same binary and not others, so the same snippet gives
two different bodies from one shell. A family of such rows is a family
nobody can use.

The probe that works asks about **one slot at a time, through a status**:

    jobs %2 >/dev/null 2>&1; echo "two=$?"

Whether a slot is *occupied* is the whole of the allocation question, and
a status has no text to race. Two things had to be true first.

### The number has to be right, and it was not

Measured 2026-09-05 on `jobs %9`, which is the one of the three job
builtins that reaches this in a script — `fg` and `bg` refuse for want of
job control first in bash and zsh:

| shell | status | what it says |
| --- | --- | --- |
| bash 5.3.15 · 3.2.57 · as `sh` | 1 | `jobs: %9: no such job` |
| dash | **2** | `jobs: No such job: %9` |
| ksh93u+ | 1 | `jobs: no such job` — **no spec at all** |
| zsh 5.9.2 | **127** | `jobs: %9: no such job` |

Four statuses across the panel, where the field for this said "all four
report 1". `Diagnostics.NoSuchJobStatus` carries the two that differ, and
zsh's 127 is the same number its `wait` gives a job that is not there — a
command that is not there. Until those were right the probe graded
nothing: every dialect answered 1 and every "not there" looked alike.

ksh93's wording uses neither verb, which is the shell and not a
truncation: `%nope` produces the same line.

### The timing has to be gone, not merely small

A job that is *certainly still running* is one with seconds left, killed
at the end of the case. A job that has *certainly finished* is one a
`wait` has returned from. Neither is a guess about the scheduler, which
is what a `sleep 0.2` raced against something else is.

The corpus rows are `jobs/slot-a-running-job-occupies-its-slot`, its
control `jobs/slot-a-status-query-does-not-consume-it` — a *listing*
consumes what it reports, and a status query does not — and
`jobs/slot-an-empty-table-has-no-first-slot`, which reads the four
statuses with nothing else in the script that could have produced them.
Each ends in `:` so the case is about slots rather than about what `kill
%n` reports, which dash alone answers 1.

`jobs/slot-the-last-background-pid-outlives-a-bare-wait` asks the `$!`
half as a yes/no, because the pid is different every run.

### What is measurable this way and still not taken

Two lifecycle questions the probe reaches and this spec does not answer,
recorded so the next attempt does not re-measure them. Both are stable
across runs; neither is a race.

**Whether a bare `wait` frees the slots it waited for.** `sleep 0 & wait;
jobs %1` — bash 1, dash 0, ksh93 0, zsh 127. So bash and zsh free the
slot and dash and ksh93 keep the finished job. It is not taken because it
does not hold still under the neighbouring probes: **bash 5.3.15 and bash
3.2.57 disagree with each other** on the same question without a `wait`
(`sleep 0 & sleep 1; jobs %1` is 0 in 5.3 and 1 in 3.2), and ksh93 answers
`wait` and `wait %1` differently — keeping the slot for the first and
freeing it for the second.

**Where the next job number goes when there is a hole.** bash allocates
*after the highest occupied slot* rather than refilling; this
implementation refills. The other three cannot be asked the same way,
because they do not free the slot in the first place. And the obvious
probe faults ksh93u+: `sleep 0 & wait; sleep 5 &; jobs %1; jobs %2` exits
139 there, reproducibly — a segmentation fault, not an answer, so no
corpus row can hold it.

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

### `wait` is not the only builtin that blocks

The sentence above — that `wait` is the only one where a signal's timing
is visible — held only because nothing had given `read` something slow to
read. A named pipe does, and the answer there is not `wait`'s. Measured
2026-09-05, `read/interrupted-by-a-trapped-signal`:

    mkfifo p; exec 3<>p; trap "echo T" INT
    (sleep 0.3; kill -INT $$; sleep 0.5; echo late >p) &
    read -r l <&3; echo "st=$? l=[$l]"

| shell | after the handler runs |
| --- | --- |
| bash 5.3, bash 3.2, zsh | the read **resumes**: `st=0 l=[late]` |
| bash 5.3 as `sh` | abandoned, `st=130`, nothing assigned |
| dash | abandoned, `st=1` |
| ksh93 | abandoned, `st=258` |

Four answers, and the sharpest is that two of them come from the same
binary: bash resumes a read a trapped signal interrupted and, called
`sh`, does not. So this is argv[0]'s question and not the build's, which
is the second time in this sweep the panel's bash-as-`sh` column carried
the difference (`ulimit/the-file-size-block` is the other).

Prior work of our own recorded `$?` after an interrupt as 130 and had it
down as a possible bash 3.2 against 5.3 split. Neither holds: the two
bash builds agree with each other here, 130 belongs to one *invocation*
of one of them, and `sh -c 'kill -INT $$'` — the shape the 130 was
measured with — is 130 in every shell in the panel that survives it,
which makes it a fact about the child's death rather than about `$?`.

The late write is what keeps the case measuring. Without it the three
resuming shells wait for a line that never comes and the row records a
timeout, which is a case that has stopped asking anything.

**A related shape is a hang and is not in the corpus.** zsh's `read -t 1`
on a fifo already holding unterminated bytes never returns at all — the
timeout covers waiting for input and not completing a line — measured
eight times out of eight. `read/a-timeout-that-expires` uses an empty
pipe for that reason.

## A script that will not open is not a usage error

Oracle runs, 2026-09-05, on macOS: bash 5.3, dash, ksh93u+, zsh 5.9.2.
Corpus rows `invoke/a-script-that-is-not-there`,
`invoke/a-script-under-a-directory-that-is-not-there` and
`invoke/an-empty-script-is-a-success`.

A shell handed a script path it cannot read has failed to reach a
program, not failed to understand its argument vector, and the panel
numbers it that way. Six ways of failing were measured — a missing
path, a path whose parent is missing, a dangling symlink, a mode-000
file, a directory, and an empty-but-readable file as the control:

| operand | bash 5.3 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| missing path | 127 | 2 | 127 | 127 |
| missing parent | 127 | 2 | 127 | 127 |
| dangling symlink | 127 | 2 | 127 | 127 |
| mode 000 | 126 | 2 | 126 | 127 |
| a directory | 126 | **0** | 126 | 127 |
| empty file | 0 | 0 | 0 | 0 |

The first three rows are one failure — the operating system says
`ENOENT` to all of them, and no shell in the panel distinguishes a
missing leaf from a missing parent or from a symlink pointing at
nothing. The next two are the second failure, `EACCES` and `EISDIR`,
which bash and ksh93 treat alike.

So there are two statuses and the panel gives three answers about them.
bash and ksh93 split 127 for a path that names nothing against 126 for
one that is there and will not open — a missing command's number and an
unrunnable command's, which is what a script operand is. zsh knows the
difference and declines to use it: 127 for all five. dash answers 2 for
all five, the number it gives a usage error.

The wordings split further, and none of the four shares one:

    bash   <shell>: nosuch.sh: No such file or directory
    dash   <shell>: 0: cannot open nosuch.sh: No such file
    ksh93  <shell>: nosuch.sh: not found
    zsh    <shell>: can't open input file: nosuch.sh

    bash   <shell>: unread.sh: Permission denied
    dash   <shell>: 0: cannot open unread.sh: Permission denied
    ksh93  <shell>: unread.sh: cannot open [Permission denied]
    zsh    <shell>: can't open input file: unread.sh

Three things in there are each shell's habit rather than anything about
this failure. dash writes the line it has not reached — `0:`, from a
shell that counts from one — and spells `ENOENT` its own way, both of
which it already does elsewhere. ksh93 brackets the reason, as its `cd`
does. zsh puts the reason first everywhere else and here leaves it out
altogether. Every shell names *itself* rather than the script, which
follows from nothing having been read: there is no `$0` yet.

This is `Diagnostics.ScriptNotFound` and `ScriptNotReadable` with a
status each, plus `InvocationNamesTheUnreadLine` for dash's nought. It
was one path returning any `os.ReadFile` error into the front end's
generic input error, which exits 2 — dash's answer given to all four,
and the reason `sh script-that-is-not-there` looked like a shell that
had been invoked wrongly rather than one that could not find a program.

### Two divergences recorded rather than reproduced, both about a directory

**dash exits 0.** Given a directory it opens it, reads nothing, and
succeeds in silence. Reproducing that would mean a shell that quietly
does nothing when pointed at the wrong path, which is the failure mode
the whole change exists to remove; this dialect reports it as a script
that would not open, at dash's 2.

**bash names the script twice.** `bash /tmp/d` on a directory prints
`/tmp/d: /tmp/d: Is a directory` — the operand where the shell's own
name goes. bash has taken the operand for its name by the time the read
fails, which is an artifact of the order it does things in rather than a
wording a dialect vector could hold. The status, 126, is reproduced.

Neither is about a file whose *contents* are not a script; that is a
separate question and has an issue of its own.

## A subscript that will not read

Issue #649. A subscript is an arithmetic expression, so one that does not
read is the failure `$((b c))` is — and every shell in the panel with
arrays reports it in exactly the words it reports that one, gives up on
the word, and exits non-zero. Measured 2026-09-05 across bash 5.3.15,
bash 3.2.57, ksh93u+ 2012-08-01, zsh 5.9.2 and dash.

    a=(x y z); echo "[${a[b c]}]"; echo after

    bash 5.3   b c: arithmetic syntax error in expression (error token is "c")
    bash 3.2   b c: syntax error in expression (error token is "c")
    ksh93      b c: arithmetic syntax error
    zsh        bad math expression: operator expected at `c'

`after` is never printed. This expanded to nothing at status 0 and the
script ran on, which is the worst shape a wrong answer takes here: an
empty string is a plausible value for a real element, so nothing after it
could tell. The evaluator's error was being returned and dropped at every
one of the five places a subscript is read — the element, its length, an
operator that reaches one, `${a[i]:=v}`, and a substring's offset and
length, which are the same reading under another spelling.

The **status is the ordinary fatal one** rather than the failed-expansion
one. bash draws that line itself: `-c 'echo "${a[b c]}"'` exits 1 where
`-c 'echo "${x@QQ}"'` from the same invocation exits 127
(`ExpansionFailureStatusFromCommandString`, recorded under #577). A bad
expression is not a word that could not be read.

**Writing through one is unanimous and `unset` is not.** `a[1+]=v` ends
the script in all four; `unset a[1+]` ends it only in bash, and ksh93 and
zsh leave a failed builtin behind — `BadSubscriptToUnsetFatal`, in the
catalog below.

### What a substring's range is blamed on

Three shapes for one failure, which is why it is two fields rather than
one flag on `ArithError`:

    x=abcdef; echo "${x:1+:2}"

    bash 5.3   x: 1+: arithmetic syntax error: operand expected …
    ksh93      1+:2: more tokens expected
    zsh        bad math expression: operand expected at end of string

bash puts the **parameter** in front of the sentence — the name and its
subscript, so `${a[@]:1+}` is blamed on `a[@]` — where the same shell
blames a bad *subscript* on the expression alone.
`Diagnostics.SubstringRangeError` carries it. ksh93 blames the offset
together with everything written after it in the range
(`SubstringErrorNamesTheWholeRange`); a failing **length** has nothing
after it and is named alone, which is the pair that shows this is a
wording rather than a reading. Extending the text before *evaluating* it
instead invents a second failure — `${x:2:1+}` reported that `2:1+` would
not parse and then that `1+` would not, where the shell reports one.

### A substring range is a modifier list where the letter decides

Issue #662. `${x:…}` is a substring in three of the panel and is *also*
zsh's history-modifier syntax, applied to a parameter. The two spellings
share every byte of their punctuation, so which one a range is has to be
decided before either is evaluated — and the rule, measured, is the first
byte and nothing else:

    x=abcdef; i=2; echo "${x:i:2}"

    bash 5.3, bash 3.2, ksh93   cd
    zsh                         unrecognized modifier `i'   (status 1)

A range segment beginning with an **unquoted letter** is a modifier list.
Everything else is the arithmetic it looks like, which is why each of
these is a substring in all four:

    ${x:_q:2}    an underscore is not a letter
    ${x: i:2}    a leading space puts the letter second
    ${x:(i):2}   as does a parenthesis
    ${x:$i:2}    the expansion happened before the reading
    ${x:"h"}     the letter was quoted

`SubstringRangeReadsModifiers` is the axis, asked only where a segment
does begin with a letter, so `${x:1:2}` needs no answer from anyone. It
is a *reading* and not a refusal: `${x:h}` is the head of a path there
and the substring from the offset `h` holds everywhere else.

A range is segments, split on its colons, and the two readings are
segments of one range rather than alternatives. Segments are an offset
and then a length while they do not begin with a letter; from the first
that does, each is one modifier applied in order to what is left:

    ${x:h}       the head
    ${x:h:t}     the head, then the tail of it
    ${x:h:t:r}   three, left to right
    ${x:2:t}     the substring from 2, then the tail of it
    ${x:h:2}     refused — `2` is a segment where a modifier belonged

A segment is one modifier and the letter is the whole of it, so the
refusal has two shapes, measured: an unknown *first* letter is named and
a leftover after a known one is not.

    ${x:i}     unrecognized modifier `i'
    ${x:ha}    unrecognized modifier          `h' is one and `a' is left over

`Diagnostics.UnrecognizedModifier` and `UnrecognizedModifierAlone` are the
two, a second field rather than an empty verb because the sentences do not
differ by a substitution — one ends in a quoted name and the other ends.

The letters this shell accepts, measured one at a time against zsh 5.9.2:
`a c e h l q r s t u A P Q`. Every other letter is refused, named.

#### Recorded rather than reproduced: seven of the thirteen

Six are performed here, and they are the six that are a pure function of
the string:

    :h  everything before the last slash, trailing slashes off first,
        `.` where there is none — `/a/b//` → `/a`, `a/` → `.`, `/` → `/`
    :t  everything after it — `/a/b//` → `b`, `/` → nothing
    :r  the value with its suffix taken off, counting a leading dot:
        `.hidden` → nothing, `x/.hidden` → `x/`, `a/b.c/d` unchanged
    :e  what `:r` takes off, without its dot
    :l  lowercase        :u  uppercase

The other seven are recognized and **refused out loud** rather than
guessed at, because each needs something a string does not carry: `:a`
the working directory, `:A` and `:P` the disk, `:c` the command search,
`:q` and `:Q` that shell's quoting table — `a b*c` quotes to `a\ b\*c` and
a newline to `$'\n'` — and `:s` a pattern rather than a letter. Refusing
is the choice `set` makes for an option letter it has and does not do:
passing the value through would be promising one that was never computed.

Two shapes are also left: `${x:1:5:t}`, a modifier after both an offset
and a length, and `${x:h2}`, where a digit after a modifier is swallowed
where `${x:h:2}` is refused.

### What "operand expected" means — two failures, not one

Issue #661. Two of the panel word the *reason* by whether the expression
ran out or found something it could not use. Measured 2026-09-05 across
bash 5.3.15, bash 3.2.57, ksh93u+ 2012-08-01, zsh 5.9.2 and dash:

    $((1+))    ksh93  more tokens expected        zsh  operand expected at end of string
    $((~))     ksh93  more tokens expected        zsh  operand expected at end of string
    $((%))     ksh93  arithmetic syntax error     zsh  operand expected at `%'
    $((1+&2))  ksh93  arithmetic syntax error     zsh  operand expected at `&2'
    $((@))     ksh93  arithmetic syntax error     zsh  illegal character: @

bash words all of them identically —
`arithmetic syntax error: operand expected (error token is "…")`, and bash
3.2 the same without the leading `arithmetic` — and dash names the whole
expression and says `expecting primary` whatever happened. That is why one
field served for years: bash is the column a conformance number is usually
read against, and it cannot see the difference.

The parser is the only place the two can be told apart, and it already
separated them without knowing it. Reading a value returns nothing at the
end of the text *without* reporting anything, so the frame that wanted the
operand names the operator it was left holding — `ErrArithOperandEnd`,
token `+`. Where there is text that cannot begin a value, the reading
reports it itself, naming everything from the refused byte to the end of
the expression — `ErrArithOperand`, token `&2`. The two tokens differ
because the two shells that name anything name different things.

`Diagnostics.ArithOperandExpected` is the found wording and
`ArithExpressionRanOut` the other; an empty second field means "the same
as the first", which is what bash and dash want.

**A dialect that blames past the expression read past it.** ksh93 reports
a failing substring offset together with everything after it in the range
— `${x:1+:2}` is `1+:2` — because it reads `offset:length` as one string.
So what ran out for this parser did not run out for that shell: it found
a `:`. Naming the whole range and reading the whole range are one fact,
so the blamed text is enough to know it and there is no second flag to
keep in step:

    ${x:1+}      ksh93  1+: more tokens expected
    ${x:2:1+}    ksh93  1+: more tokens expected      a length has nothing after it
    ${x:1+:2}    ksh93  1+:2: arithmetic syntax error

#### Still recorded rather than reproduced: zsh's third wording

zsh has a third sentence for a byte that is not part of any arithmetic
token — `illegal character: @` — and it is not modeled. It is not a
question of *which* byte alone; it is also where the byte stands:

    $((@))       illegal character: @          nothing read yet
    $((1 @))     illegal character: @          where an operator belonged
    $((1+@))     operand expected at `@'       where an operand belonged
    $((+@))      operand expected at `@'       after a unary operator too
    $((1 :))     operand expected at end of string    `:` *is* a math token
    $((1 2))     operator expected at `2'      a value where an operator belonged

The refused set measured is `@ { } ; '`; every other byte tried is either
a math token or can begin a value. Reproducing the sentence therefore
needs that shell's lexical table *and* a rule about position, which is a
lexer's internals rather than a grammar question, and this repository
learns behavior by running binaries. `arith/an-operand-a-lexer-refuses-outright`
records it: three of the four columns pass and the zsh column is the work.

## A subscript before the first element

Issue #617. An assignment whose subscript lands before the array's first
element is refused, the element is not written, and **the refusal ends the
script at 1** — in every shell on the panel that has arrays. This reported
in a wording of our own at status 0 and ran on, so the next command read
an array the assignment had not touched.

Which subscript reaches it is `ArrayBaseIsZero` and nothing else, and that
is why one rule needs two spellings to show it. Measured 2026-09-05 across
bash 5.3.15, bash 3.2.57, ksh93u+ 2012-08-01, zsh 5.9.2 and dash.

    a=(p q); a[0]=x     bash, ksh93 → [x][q], the first element
                        zsh         → refused, script ends at 1

    a[-1]=x             bash, ksh93 → refused, script ends at 1
                        zsh         → [x], the array's first element

Where the first element is 1, `a[0]` is below it. Where the first element
is 0, no non-negative subscript can be, and the boundary is reached only
by a negative subscript counting back past the start. Neither shell has
both spellings, so the two columns look like different behaviors and are
one.

`+=` is the same assignment and reaches the same refusal: nothing joins a
position that does not exist.

### What the refusal names — three answers

    bash 5.3   a[x-2]: bad array subscript
    ksh93      a: subscript out of range
    zsh        a: assignment to invalid subscript range

bash quotes the subscript back **as it was written** — `a[x-2]`, not the
`-1` it evaluated to — which is why the text is carried as far as the
store rather than only the number. ksh93 and zsh name the array alone.
`Diagnostics.BadArraySubscript`.

Through an array literal two of the three word it differently again, so it
is a field of its own (`BadArrayLiteralSubscript`, falling back to the
plain one):

    a=(p q [-5]=x)   bash 5.3  [-5]=x: bad array subscript
    a=([0]=p)        zsh       bad subscript for direct array assignment: 0

bash names the element as it stands between the parentheses, with no array
name in front of it; zsh names the subscript and the kind of assignment.
ksh93 does not reach it, because a subscript inside a literal is a key
there (`ArrayLiteralSubscriptIsAKey`).

**bash 3.2 differs on the literal alone**: it prints the same sentence and
does *not* end the script, where 5.3 does. The plain form is fatal in both.
The preset follows 5.3.

### The same boundary reached from `unset`

Issue #700. `unset a[i]` reaches the identical boundary and was silent at
status 0 there, so a script that asked to remove something out of reach
was told it had. Measured 2026-09-05 across the same five binaries, on
`a=(x y z)`:

    unset "a[0]"     bash 5.3, bash 3.2, ksh93 → [y z], the first element
                     zsh                       → refused, st=1, [x y z]

    unset "a[-4]"    bash 5.3  unset: [-4]: bad array subscript, st=1
                     ksh93     unset: a: subscript out of range,  st=1
                     zsh                       → silent, st=0, [x y z]

Two things about it are the same as the assignment's and one is not.

**Same:** which spelling reaches the boundary is `ArrayBaseIsZero` and
nothing else. Where the first element is 1, `a[0]` is below it; where it is
0, only a negative subscript counting back past the start can be.

**Same:** that the blanking shell is silent on the negative spelling is not
a second rule and not a new axis. `UnsetArraySpan` already says that
blanking replaces a span *that is there* — the reading `unset a[-2]` is
recorded under, where the array comes back whole — and a subscript past the
start names no span at all. The removing answers reach only the negative
spelling, their first element being 0; the blanking answer reaches only the
non-negative one. One rule, one axis apiece, no third.

**Not the same:** the ending. The assignment stops the script; `unset`
leaves a **failed builtin** behind and the next command runs, in all three.
That is a shape a script can test, so the status is returned rather than
thrown, and reusing the assignment's fatal path here would have been a new
bug rather than a fix.

The wording is a field of its own, `UnsetSubscriptBeforeTheFirstElement`,
because two of the three word this route differently from the assignment:
bash drops the array's name and keeps the bare subscript — still **as it
was written**, `unset "a[x-9]"` naming `[x-9]` — and both bash and ksh93
put the builtin's name in front of a sentence neither prefixes for an
assignment. zsh says the same sentence by both routes, and does **not**
put the builtin in the location, where it does for messages of its own.

Under the blanking answer a scalar is the single element it is — a span of
one is what `unset a[i]` means there — so `a=v; unset "a[0]"` reaches the
boundary and is refused. The removing answers read a scalar without
looking at the subscript at all, which is `UnsetNotAnArray`'s question. A
name holding nothing at all has no first element for a subscript to be
before, and is left alone without a word everywhere.

**bash 3.2 has no negative array subscripts at all.** `unset "a[-1]"` on a
three-element array is `[-1]: bad array subscript` there — with no `unset:`
in front of it — where 5.3 removes the last element; `-4` is refused by
both, for different reasons. It is an absence of the spelling rather than a
wording, and it is the same absence `array/removing-an-element-from-the-end`
and `array/appending-to-an-element` already record. The preset follows 5.3,
so bash 3.2 is a column in the golden record and not a dialect.

### Not fixed here, recorded rather than reproduced

- *Reading* through a subscript past the start is its own shape and its
  own three answers — `${a[-5]}` is empty at 0 in bash and zsh and fatal
  in ksh93. It is neither an assignment nor an `unset`.
- ksh93's `a=()` is a **compound variable**, `typeset -C a=()`, and not an
  empty indexed array: `${#a[@]}` is 1 for it and `unset "a[-1]"` says
  nothing. We model `a=()` as an empty array, so our ksh refuses that one
  spelling where ksh93 does not. The difference is what `a=()` builds
  rather than where the boundary is.
- zsh blanks a **scalar** through an in-range subscript — `a=v; unset
  "a[1]"` leaves `a` empty at 0 — where we leave the value standing. It is
  the blanking reading applied to a name that is no array, and a separate
  gap from this boundary.
- zsh refuses **any** negative subscript inside a literal — `a=(p q
  [-1]=x)` is `bad subscript for direct array assignment: -1` — where bash
  reads it end-relative and places it. So a literal's subscript is not
  end-relative there, which is a second rule about literals rather than
  about this boundary.

## The axis catalog

Issue #487: 160 of the fields on `interp.Semantics` were named nowhere in
`docs/spec/`. None of them was dark — every one is exercised by the corpus
or by a unit test, and each carries a field comment that already reads
like a spec entry — but this file's own rule is that **a probe is recorded
here**, and for those 160 it was not. The rest of this document is the
record, and with it every field on the vector is named somewhere a reader
looks. (One of the 160, `RedirectTargetIsAnOrdinaryWord`, gained a
section of its own under field splitting while this was being written;
its entry below is the short form.)

**What an entry says.** The bold name is the field on `interp.Semantics`.
The line beside it is the answer each preset gives; `unspecified` is a
real answer and means the axis is *refused* rather than guessed, which is
what the core does wherever the panel genuinely disagrees. Then the
behavior, in the terms the measurement was taken in.

**Where the answers come from.** Each was measured against bash 5.3.15,
dash, ksh93u+ 2012-08-01 and zsh 5.9.2, in the fixed environment
`oracle.md` describes. Two things follow, and both matter for reading the
catalog:

- **`bash` here means bash 5.3.** A preset is one shell, and bash 3.2's
  column dates a construct rather than voting on it (`core.md`). Where
  bash 3.2 differs the entry says so; where it is silent, it was not the
  question being asked.
- **The answers are this implementation's, and they are claims about the
  panel.** Re-measuring is how the claim is checked, and it is worth
  doing: writing this catalog re-measured every cluster below against the
  live panel, and the `set -o` long names — audited in the same sweep —
  turned out to have a wrong answer in both the code and the prose.

**Where an axis is asked.** Almost every one is asked *narrowly*: only
where the construct that raises it is actually present. `echo hi` needs
no dialect, and `echo -e "a\tb"` does. That is not an optimization but
the thing that lets the bare core run useful scripts while refusing the
handful of questions no boolean can answer for it — the rule stated under
"Rules for adding an axis" above, seen from the other end.

**Multi-valued axes name a policy type** rather than yes-or-no, and the
type's own values are documented beside it in `interp/semantics.go`:
`ListingQuotingStyle`, `PrintfQuoteStyle`, `NameOperands`,
`ExitArgumentPolicy`, `TrapBodyLineStyle`, `SelectMenuLayout`,
`DeclarationListingForm`, `KillStatusStyle`, `BracketPolicy`,
`DollarSingleControlPolicy`, `DollarSingleUnknownPolicy`,
`UnsetArraySpanPolicy`. Where an entry below says "see X", X is one of
those.

**This catalog is not the whole of the vector.** The axes with their own
sections earlier in this document — word splitting, array base, the
special-builtin fatality rules, `$'…'` decoding, `read`'s letters and the
rest — are covered there in more depth and are not repeated here.

### `alias` and `unalias`

**`AliasHasPrintOption`** — bash yes · dash no · ksh93 yes · zsh no

Gives `alias` a `-p`, which prints the listing with `alias ` in front of
every line. bash and ksh93 have it — and it is what bash's plain listing
already looks like, so it is only visible in ksh93. dash parses no
options for `alias` at all, so `-p` is a *name* there and the answer is
"not found"; zsh has options and refuses it.

**`AliasNotFoundStatusCounts`** — bash no · dash no · ksh93 yes · zsh no

Makes `alias` report how many names it could not find rather than a
plain 1: `alias n1 n2 n3` is 3 in ksh93 and 1 in the other three.

About `alias` alone — ksh93's own `unalias` answers 1 however many were
missing — so it is asked where the count is known and not where the
complaint is printed.

**`AliasParsesOptions`** — bash yes · dash no · ksh93 yes · zsh yes

Lets `alias` read leading `-` words as options. True in bash, ksh93 and
zsh; dash reads none, so `alias -p` is a name there and the answer is
"-p not found" rather than a refusal.

**`AliasQuoting`** — bash ListingQuoteAlwaysEscaped · dash ListingQuoteAlwaysDoubled · ksh93 ListingQuoteWhenNeededDollar · zsh ListingQuoteWhenNeededEscaped

Is how a value is spelled in a listing — four engines, no two alike. See
ListingQuotingStyle.

**`AliasReportsNotFound`** — bash yes · dash yes · ksh93 yes · zsh no

Says something when `alias` is given a name the table does not hold.
True in bash, dash and ksh93; zsh reports 1 and prints nothing.

**`UnaliasAllRefusesOperands`** — bash no · dash no · ksh93 no · zsh yes

Makes `unalias -a name` an error that clears nothing. zsh alone: "-a:
too many arguments", status 1, table intact. The other three take the
`-a`, ignore the names and empty the table.

**`UnaliasReportsNotFound`** — bash yes · dash yes · ksh93 no · zsh yes

Is that question for `unalias`, and the panel does not pair the two:
ksh93 complains about `alias nope` and is silent about `unalias nope`,
and zsh does exactly the reverse. One field could not say that.


### `echo`

**`EchoExpandsEscEscape`** — bash yes · dash no · ksh93 no · zsh yes

Admits `\e` for the escape character in an `echo` argument.

**`EchoExpandsCapitalEscEscape`** — bash yes · dash no · ksh93 yes · zsh no

Admits `\E`, and it is a second axis because the two shells that split the
letters split them in **opposite** directions. Measured with `od -An -tx1`
(macOS, 2026-09-06):

    echo -e 'a\eZ'   bash 5.3 61 1b 5a   zsh 61 1b 5a
                     bash 3.2 61 5c 65 5a   ksh93 61 5c 65 5a
                     dash (no -e) 61 5c 65 5a
    echo -e 'a\EZ'   bash 5.3 61 1b 5a   ksh93 61 1b 5a
                     bash 3.2 61 5c 45 5a   zsh 61 5c 45 5a
                     dash (no -e) 61 5c 45 5a

ksh93 has `\E` and not `\e`; zsh has `\e` and not `\E`. bash 5.3 has both
and bash 3.2 neither, which is a version line rather than a dialect one.
A single answer for the pair — which is what this was until #908 — is wrong
for half the panel: it gave ksh93 an `\e` it does not have and zsh an `\E` it
does not have.

It is the same asymmetry the `%b` site has, asked separately there
(`PrintfBEscEscape`, `PrintfBCapitalEscEscape`), and the four dialects give
the same pair of answers at both sites. Each letter is asked only where its
own spelling appears, so `echo -e 'a\EZ'` needs no answer about `\e`.

Corpus: `echo/the-two-spellings-of-the-escape-character`.

Re-measured for this catalog, bash 3.2 prints `\e` as written where bash
5.3 writes ESC, so this too is a bash 4 addition — dating rather than
vetoing, per `core.md`.

**`EchoExpandsHexEscapes`** — bash yes · dash no · ksh93 no · zsh yes

Admits `\xHH` alongside the XSI set: bash and zsh do, dash and ksh93
print it as written.

**`EchoInterpretsEscapes`** — bash no · dash yes · ksh93 no · zsh yes

Expands backslash escapes in `echo` without -e. True in dash and zsh,
false in bash and ksh93 — a grouping no other axis produces.

**`EchoLastEscapeFlagWins`** — bash yes · dash unspecified · ksh93 unspecified · zsh no

Decides `echo -e -E`: bash lets the last flag win and prints the
backslashes, zsh lets -e win whatever the order. Reached only when -e
came first — the other order agrees everywhere — and only in a dialect
whose EchoOptions has both letters.

**`EchoOptions`** — bash neE · dash n · ksh93 ne · zsh neE

Is the set of letters `echo` reads as options: `n` for every shell
measured, `e` everywhere but dash, `E` in bash and zsh alone. A word
carrying any other letter is not an option at all — the whole word
becomes an operand, which is unanimous and is why `echo -nq hi` prints
`-nq hi` in all four. Empty means `n`.


### `cd`, `CDPATH` and command lookup

**`CdDashPrintsTheDirectory`** — bash yes · dash yes · ksh93 yes · zsh no

Writes the new directory when `cd -` moves. True in bash, dash and
ksh93; zsh alone is silent.

**`CdLastPathOptionWins`** — bash yes · dash yes · ksh93 yes · zsh no

Lets the last of `cd -L` and `cd -P` decide. True in bash, dash and
ksh93 — `cd -P -L` is logical there. zsh gives `-P` the answer wherever
it appears, so both orders resolve.

Asked only when both were given, because that is the only time the two
rules differ.

**`CdRefusesUnknownOption`** — bash yes · dash yes · ksh93 yes · zsh no

Refuses a letter `cd` does not have rather than reading the word as a
directory. True in bash, dash and ksh93; zsh looks for somewhere called
`-Q` instead, because its `cd` takes two operands — `cd old new` — and a
leading dash word is the first of them there.

Only about an *unknown* letter. `-L` and `-P` are options in all four
and are not asked about.

**`CdWithoutHomeIsAnError`** — bash yes · dash no · ksh93 yes · zsh no

Makes `cd` with no operand and no HOME a failure. True in bash and
ksh93; dash and zsh stay where they are and report success, which is the
quieter answer and the surprising one. The same axis answers `cd -` with
no OLDPWD.

**`CdpathAnnouncesTheDirectory`** — bash yes · dash yes · ksh93 yes · zsh no

Prints where CDPATH sent a `cd`, when the winning entry was not a plain
dot — three of the four; zsh moves in silence.

**`CommandNotFoundStatusIsNotFound`** — bash no · dash yes · ksh93 no · zsh no

Makes `command -v` answer 127 for a name that is nothing, rather than a
plain 1. dash alone says yes; the other three report a failure and leave
127 to mean a command that was looked for and run.

**`DirectoryOnPathIsACandidate`** — bash no · dash yes · ksh93 yes · zsh yes

Keeps a directory the PATH search found as the failed candidate when no
later entry runs, so the report names the directory rather than saying
the command was never found.

Every shell measured continues the search past the directory — that is
unanimous, and is what makes a shim directory early on PATH work at all.
They part ways only when nothing later matches: bash reports the name as
not found at all (status 127), where dash, ksh93 and zsh report the
directory they could not run. dash alone keeps 127 for the status even
then, which is DirectoryOnPathStatus's question.

**`EmptyPathIsTheCurrentDirectory`** — bash yes · dash yes · ksh93 no · zsh yes

Searches the current directory when PATH is set and empty.

True in dash, bash and zsh; false in ksh93. `PATH=` reads like "nowhere"
and is not: an empty PATH is one *empty element*, and an empty element
means the current directory, so three of the four will still run a
command sitting next to the script. Measured with the command in the
current directory, which is the only arrangement that tells the two
answers apart — with it anywhere else all four report not-found and the
axis is invisible.

`PATH=:` is not this question. Two empty elements is unanimous: every
shell searches the current directory for it.

**`HashReportsAMissingName`** — bash yes · dash yes · ksh93 no · zsh yes

Has `hash name` complain and answer 1 when the name resolves to nothing.
bash, dash and zsh do; ksh93 — whose hash is an alias for `alias -t` —
says nothing and reports success.

**`HashSearchesPathAlone`** — bash no · dash no · ksh93 no · zsh yes

Counts only what PATH holds: zsh answers `hash shift` with "no such
command" where the other three accept a builtin or a function as
hashable. Measured with `shift`, which no PATH carries — `cd` was the
contaminated probe, macOS ships /usr/bin/cd. Recorded as
`hash/a-builtin-counts-except-in-zsh`.


### `umask`, and the symbolic form

**`SymbolicMaskSetsWithoutAWho`** — bash yes · dash yes · ksh93 yes · zsh no

Takes `umask -- =w`, where `=` has no who before it and means all three
groups. Three of the four do; zsh wants one, and names a character that
is not in the input when it does not get one.

**`SymbolicMaskTakesMoreThanOneOperator`** — bash yes · dash yes · ksh93 yes · zsh no

Lets one `umask` clause turn on several: `umask u+rw-x` is 0122 from 022
in three of the four. zsh takes a single operator per clause and names
the second one.

Re-measured for this catalog, bash 3.2 is on zsh's side —
`` umask: `-': invalid symbolic mode character `` — so the multi-operator
form is a bash 4 addition. Dating rather than vetoing, per `core.md`.

**`SymbolicMaskTakesTheSetuidLetter`** — bash yes · dash yes · ksh93 yes · zsh no

Accepts `s` in a clause, which changes no bits — a umask has no setuid
bit to deny — and is accepted by three of the four all the same. zsh
refuses it.

**`SymbolicMaskTakesTheStickyLetter`** — bash yes · dash no · ksh93 yes · zsh no

Is the same question about `t`, and a different set of shells: bash and
ksh93 take it, dash and zsh do not. Two fields because the two letters
are not answered together.

**`SymbolicMaskWhoAloneSetsIt`** — bash no · dash no · ksh93 yes · zsh no

Reads `umask g` as `umask g=`, denying that group everything. ksh93
alone. bash and dash refuse it, and zsh answers it with the complaint it
gives a number it could not read.

**`UmaskPrintsFourDigits`** — bash yes · dash yes · ksh93 yes · zsh no

Writes the mask as four digits, always — `0022` against zsh's `022`.
True in bash, dash and ksh93.

False is not "three digits". zsh writes a C octal literal with a minimum
of three, so the leading zero comes back as soon as the owner group
denies anything: `022` and `077`, but `0333` and `0777`. Reading this as
a flat three printed `333` where zsh prints `0333`.

Only about printing: all four read `022` and `0022` alike, and the
symbolic form `umask -S` is identical in every one of them.

**`UmaskSetWithSPrints`** — bash yes · dash no · ksh93 no · zsh no

Echoes the new mask when `umask -S mask` both sets and is asked for the
symbolic form. True only in bash, which prints `u=rwx,g=,o=` after
setting; the other three set and say nothing.

Only for that combination: `umask mask` is silent in all four, and
`umask -S` with no mask prints in all four.


### `trap`: options, listings and what survives a subshell

**`BackgroundJobKeepsTrapListing`** — bash yes · dash no · ksh93 no · zsh no

Asks it of `… &`. bash alone: the other three list nothing the parent
had there.

**`KeptTrapListingIncludesExit`** — bash yes · dash unspecified · ksh93 yes · zsh no

Says a kept listing shows the parent's EXIT trap alongside the signals.
bash and ksh93 list it; zsh keeps a pipeline element's listing and still
drops EXIT from it. Unanswerable where nothing is kept, so dash never
reaches the question.

**`PipelineElementKeepsTrapListing`** — bash yes · dash no · ksh93 no · zsh yes

Is the same question asked of a pipeline element that runs in a subshell
environment, and the panel pairs off the other way: bash and zsh keep
the listing there, ksh93 and dash do not. `trap 'echo x' USR1; trap |
cat` prints the trap in bash and zsh and nothing in the other two — the
shape issue #339 measured.

**`SubshellHidesInheritedIgnoredTraps`** — bash no · dash no · ksh93 no · zsh yes

Drops an *inherited* ignore from the child's listing while the signal
stays ignored in fact: zsh, where `trap '' INT; (trap)` prints nothing
and `(kill -INT $$; echo alive)` still prints alive. The other three
list what POSIX says is still a current trap. An ignore the child sets
itself is listed everywhere.

**`SubshellKeepsTrapListing`** — bash yes · dash no · ksh93 yes · zsh no

Makes `trap` inside `( … )` or `$( … )` still list the traps the parent
had, though a handled one no longer fires — the save=$(trap) idiom POSIX
carves out, extended to the compound. bash and ksh93; dash and zsh list
only what survived the entry.

**`TrapActionIsParsedWhenSet`** — bash no · dash no · ksh93 no · zsh yes

Reads a trap's action when the trap is set rather than when it fires,
and refuses a trap whose action will not parse.

zsh alone. The other three store the text: `trap "if" EXIT` is taken and
complains at the end, and `trap "if" INT` is taken and never complains
at all, because the trap never fires.

**`TrapBodyLine`** — bash TrapBodyLineWithin · dash TrapBodyLineWithin · ksh93 TrapBodyLineOffsetFromWhereItFired · zsh TrapBodyLineWhereItFired

Is which lines a diagnostic from inside a trap's body names. See
TrapBodyLineStyle.

**`TrapBodyRunsWhatParsed`** — bash yes · dash yes · ksh93 no · zsh no

Runs each line of a trap's body as it parses, so the part before a
syntax error has already run by the time the error is reported.

bash and dash do — `trap "echo a if" EXIT` prints `a` and then
complains. ksh93 reads the whole body first and prints nothing. zsh
answers no by construction rather than by measurement: it reads the
action when the trap is set, so by the time a trap fires the whole body
has parsed and there is no partial run to have. The two answers cannot
be told apart there.

**`TrapHasDebugCondition`** — bash yes · dash no · ksh93 yes · zsh yes

Makes `trap … DEBUG` run the action before each simple command. dash
alone refuses the name.

**`TrapHasErrCondition`** — bash yes · dash no · ksh93 yes · zsh yes

Makes `trap … ERR` a condition rather than a misspelled signal: the
action runs after every command that fails where `set -e` would judge it
— with or without `set -e` on, which is measured rather than assumed.
dash alone refuses the name, with the same words it refuses any other
word that names no signal.

**`TrapHasReturnCondition`** — bash yes · dash no · ksh93 no · zsh no

Makes `trap … RETURN` a condition that fires when a sourced file
finishes, and when a function whose own body set the trap returns. bash
alone; the other three refuse the name the way they refuse any word that
names no signal.

**`TrapListsSignalsWithL`** — bash yes · dash no · ksh93 no · zsh unspecified

Makes `trap -l` list the signal names, the way `kill -l` does. bash only
— ksh93 refuses the letter.

**`TrapOneArgumentIsACondition`** — bash yes · dash yes · ksh93 no · zsh yes

Reads `trap EXIT` as "put EXIT back" rather than as an action with no
condition to attach it to.

Three of the four do, which makes `trap EXIT` the short spelling of
`trap - EXIT`. ksh93 refuses the form and the refusal ends the script.

**`TrapParseFailureNamesWhereItFired`** — bash no · dash no · ksh93 yes · zsh no

Puts the runtime location in front of a trap body's parse failure —
where the trap fired — rather than the line the parse gave out on.

ksh93 alone, and the two are different numbers: a body set on line 2 and
fired from line 5 reports `w5.sh: line 5: syntax error at line 6`. bash
and dash name the parse position in both places. zsh is not asked,
because it reads the action when the trap is set and never reaches a
parse failure at fire time.

**`TrapParsesOptions`** — bash yes · dash yes · ksh93 yes · zsh no

Reads a leading `-` word as an option rather than as the action to run.

Three of the four do. zsh does not, so `trap -p` sets a trap whose
action is the word `-p` and the failure surfaces later, when it fires —
which is what this shell did for every dialect before there were options
here at all.

Asked of the letters a dialect knows as much as of the ones it does not,
because zsh takes `-p` as the action just as it takes `-Q`. A lone `-`
is trap's own word for "put it back" and is never an option, and `--`
ends them in all four.

**`TrapPrintsBareWithConditions`** — bash no · dash no · ksh93 yes · zsh unspecified

Makes `trap -p condition ...` write the action alone rather than the
whole `trap -- action condition` line.

ksh93 only, and it is why `-p` and bash's `-P` are two questions and not
one: ksh93 reaches bash's `-P` output through `-p` with an operand, and
has no `-P` at all.

**`TrapPrintsBareWithP`** — bash yes · dash no · ksh93 no · zsh unspecified

Makes `trap -P condition ...` write the action alone, with no `trap --`
around it. bash only, and it is the one option that insists on an
operand: printing all of them is `-p`'s job.

**`TrapPrintsWithP`** — bash yes · dash no · ksh93 yes · zsh unspecified

Makes `trap -p` write the traps currently set, and `trap -p condition
...` only the named ones. bash and ksh93 have it; dash rejects the
letter along with every other.

**`TrapQuoting`** — bash ListingQuoteAlwaysEscaped · dash ListingQuoteAlwaysDoubled · ksh93 ListingQuoteWhenNeededDollar · zsh ListingQuoteWhenNeededPlain

Is that same question asked of `trap`, and it is a separate field
because one dialect answers the two differently: zsh writes an alias
holding a tab as `$'a\tb'` and a trap holding one as a plainly quoted
`'a<tab>b'`.

**`TrapReportsAnUnknownSingleCondition`** — bash yes · dash yes · ksh93 unspecified · zsh no

Complains when that one word turns out not to name a condition. zsh says
nothing — and does complain about `trap : foo`, so this is the single-
word form's own answer rather than zsh declining to check at all.

**`TrapSingleUnknownConditionIsUsage`** — bash yes · dash no · ksh93 unspecified · zsh unspecified

Prints the usage line rather than naming the word. bash does: with one
word it cannot tell a misspelled condition from an action someone forgot
to give a condition to. dash names the word the same way it does
anywhere else.


### the ERR, DEBUG, RETURN and EXIT conditions

**`DebugTrapRunsInSubshells`** — bash no · dash unspecified · ksh93 yes · zsh yes

Fires the DEBUG trap inside a subshell or a command substitution. ksh93
and zsh do — a command substitution there captures the handler's output
into the variable — and bash does not, which is a grouping
ErrTrapRunsInSubshells does not have: ksh93 carries DEBUG into the child
and not ERR.

**`DebugTrapRunsInsideCalls`** — bash no · dash unspecified · ksh93 yes · zsh yes

Fires the DEBUG trap before commands inside a function or a sourced file
the trap was not set in. bash does not; ksh93 and zsh do. Not the ERR
axis under another name, and not only because bash controls the two with
different options: a sourced file bounds DEBUG there and does not bound
ERR — measured, with a top-level trap of each, `false` inside a dotted
file fires ERR and the commands of the same file fire no DEBUG.

**`ErrTrapRunsInSubshells`** — bash no · dash unspecified · ksh93 no · zsh yes

Fires the ERR trap for a failure inside a subshell or a command
substitution. zsh alone: `trap 'echo E' ERR; x=$(false; echo hi)`
captures an E there and nowhere else. bash and ksh93 reset the trap on
the way into the child, the way they reset every trap that is not
ignored.

**`ErrTrapRunsInsideFunctions`** — bash no · dash unspecified · ksh93 yes · zsh yes

Fires the ERR trap for a failure inside a function the trap was not set
in. bash does not — there a function does not inherit the ERR trap, so
only the call itself is judged where the trap can see it. ksh93 and zsh
fire it inside too.

The suppression is per *frame*, not per depth, which is measured: a trap
set inside a function fires in that function and at the top level after
it returns, and does not fire inside a sibling function entered
afterwards, though the sibling's own failing call still does.

**`ExitTrapFiresPastTheEnd`** — bash unspecified · dash unspecified · ksh93 no · zsh yes

Counts the EXIT trap as having fired on the line after the script's
last, rather than on its first.

Only asked by a dialect whose TrapBodyLine needs a firing line at all,
and only for EXIT, which has no line of its own. zsh says yes: its EXIT
trap reports the line the parser stopped at. ksh93 says no, which makes
an EXIT body read like a small script of its own.

**`ExitTrapIsFunctionLocal`** — bash no · dash no · ksh93 no · zsh yes

Fires an EXIT trap set inside a function when that function returns,
rather than when the script ends. zsh alone; a trap set at the top level
behaves the same everywhere.

**`ExitTrapRunsOnSignalDeath`** — bash yes · dash no · ksh93 yes · zsh no

Fires the EXIT trap when the shell is ending because a signal it had no
handler for killed it, rather than because it reached the end or ran
`exit`.

    trap 'echo bye' EXIT; kill -INT $$

prints bye in bash and ksh93 and prints nothing in dash and zsh, and all
four report 130. A two-two split on whether dying counts as exiting.

**`ReturnOutsideAFunctionIsRefused`** — bash yes · dash no · ksh93 no · zsh no

Reports a `return` that has nothing to return from and carries on,
instead of ending the script with the status it was given. True in bash
alone.

Asked only where there is nothing to return from. Inside a function and
inside a sourced file all four obey it, so the question is about the one
case they split on.


### jobs, `kill` and background work

**`AnnouncesBackgroundJob`** — bash yes · dash no · ksh93 yes · zsh yes

Prints the job number and the process id when a job is backgrounded,
before the next prompt. True in bash, ksh93 and zsh; dash says nothing
at all.

Only ever at a prompt: no shell announces one to a script.

**`JobControlAbsenceIsReportedFirst`** — bash yes · dash no · ksh93 no · zsh yes

Refuses `bg` and `fg` before reading the operand when there is no job
control — bash and zsh; dash and ksh93 read their operands and options
first and complain about those.

**`JobsListFinishedJobs`** — bash yes · dash yes · ksh93 yes · zsh no

Includes a job that has already ended in a `jobs` listing, once, before
forgetting it. True in bash, dash and ksh93; zsh drops a finished job
without ever mentioning it.

The forgetting is not the axis and is not optional: every shell in the
panel reports a finished job at most once, so a second `jobs` shows
nothing. A shell that kept them would grow a listing for the length of
the session.

**`JobsListNewestFirst`** — bash no · dash yes · ksh93 yes · zsh no

Puts the most recent job at the top of a `jobs` listing. True in dash
and ksh93; bash and zsh list oldest first.

A two-two split, which is the usual shape here and the reason this is a
field rather than a choice: there is no ordering of the shells that
explains it.

**`JobsShowBackgroundCommand`** — bash yes · dash no · ksh93 no · zsh yes

Puts the command of a `&` job in a `jobs` listing. True in bash and zsh;
dash prints an empty column there and ksh93 a placeholder.

Only for a `&` job, which is the whole reason this is not a question
about rendering a command at all: both of the shells that leave it out
here *do* print the command of a job they stopped themselves. They kept
nothing for this kind of job, and the listing is where that shows.

**`KillListAcceptsName`** — bash yes · dash no · ksh93 yes · zsh yes

Lets `kill -l` translate a name into a number, as the reverse of what it
does with one. True in bash, ksh93 and zsh.

dash's `-l` takes an *exit status* rather than a signal, so `kill -l 9`
agrees with everyone by arriving there another way and `kill -l INT` is
an illegal number. One question with two answers rather than a feature
dash is missing, which is why it is an axis and not a gap.

**`KillStatus`** — bash any success · dash any failure · ksh93 any failure · zsh failure count

Is what `kill` reports when it was given several targets and they did
not all agree. Three answers, and no two of them are the majority:

    kill -0 $$ 999999    bash → 0   dash, ksh93 → 1   zsh → 1
    kill 999998 999999   bash → 1   dash, ksh93 → 1   zsh → 2

bash reports success if it signaled anything at all, and zsh reports the
number that failed — which is a status carrying a count rather than a
verdict, and the reason this is a policy rather than a bool.

**`LastBackgroundPidIsZeroBeforeAnyJob`** — bash no · dash no · ksh93 no · zsh yes

Makes `$!` read `0` before a background command has been started. zsh
alone, and it is a number nothing ever had: `sh -c 'echo "[$!]"'` writes
`[0]` there and `[]` in bash 5.3.15, bash 3.2.57, bash 3.2 run as `sh`,
dash and ksh93u+.

Zero is not the same answer as nothing, which is why this is a switch
and not a rendering: a background builtin runs in this process and its
job carries no pid, so a shell really can hold a *recorded* zero, and a
script could not tell that apart from zsh's if the two were spelled
alike.

Read without asking. A preset that has not chosen answers with nothing,
which is what five of the six columns do; refusing a `$!` expansion over
an unanswered field would break the `p=$!` of every script running under
it, including before its first job, where the read is the ordinary one.

**`LastBackgroundPidIsUnsetBeforeAnyJob`** — bash yes · dash yes · ksh93 no · zsh no

Makes `$!` an *unset* parameter before a background command has been
started, so `set -u` is fatal about it.

A different split from the field above and the more useful one — two
against two, and `set -u` is what scripts actually rely on:

| shell | `set -u; echo "[$!]"; echo "st=$?"` |
| --- | --- |
| bash 5.3.15 | `$!: unbound variable`, status 127 |
| bash 3.2.57 | `$!: unbound variable`, status 127 |
| bash 3.2 as `sh` | `$!: unbound variable`, status 127 |
| dash | `!: parameter not set`, status 2 |
| ksh93u+ | `[]` then `st=0` — set, and empty |
| zsh 5.9.2 | `[0]` then `st=0` — set, and zero |

Neither field predicts the other. zsh's zero is a value and ksh93's
empty is a set parameter, so the two quiet columns are quiet for
different reasons, and a single field could say only one of those
things. `TestTheTwoLastBackgroundPidAxesAreIndependent` runs all four
combinations, the one no shell in the panel is included.

Nor is it `UnsetPositionalIsAllowed` reached from another route. ksh93
lets an unset `$1` be empty and lets `$!` be empty too, but zsh refuses
`$1` and does not refuse `$!`, so the pair splits the panel differently.

The wording and the status are the same two Diagnostics fields an unset
positional reads, because measured they are the same two lines: bash
writes the `$` back for `$!` exactly as it does for `$1`, which is
`Diagnostics.UnboundPositional`, and dash's is its ordinary `parameter
not set`. A third field would have held a copy of one of those in all
four presets.

Read without asking, for the reason above. Unanswered means the
parameter is set and empty, which is what this shell did before the axis
existed and what the two shells that carry on do.

One spelling is recorded and not claimed. bash writes `$!` back for the
`$!` spelling and `!` alone for `${!}` — measured, `set -u; echo "${!}"`
is `!: unbound variable` in 5.3.15 where `$!` is `$!: unbound variable`,
and 3.2.57 prints *two* lines for the braced form. This shell says
`$!: unbound variable` for both. A second wording field would hold a
copy of the first in all four presets to carry one character in one
shell in a form nothing writes, and the braced spelling collides with
bash's indirection syntax besides, which is what the two lines from 3.2
are.

Corpus: `jobs/the-last-background-pid-before-any-job` reads the
parameter, `jobs/an-unstarted-last-background-pid-under-set-u` asks
`set -u` about it, and
`jobs/the-last-background-pid-under-set-u-once-a-job-has-run` is the
control that says the refusal is about nothing having been started
rather than about `$!` — with one job behind it the parameter is set in
all six columns. These are the whole `$!` grid's only recordable rows:
everything else about the parameter carries a pid, which is not the same
twice.

**`MonitorNeedsATerminal`** — bash no · dash yes · ksh93 no · zsh yes

Ties turning `set -m` on to having a terminal. Measured in shells run
with none, which is what a script has: bash and ksh93 grant the option
silently; dash remarks `can't access tty; job control turned off` and
reports success with the option left off; zsh refuses it at 1, fatally.
The two refusal shapes are the dialect's own wording and status —
Diagnostics.MonitorDenied and MonitorDeniedStatus. A runner whose front
end gave it a person to report jobs to (JobControl) has a terminal, so
the question is asked only without one. Turning the option *off* is
granted everywhere.

**`InteractiveMonitorNeedsATerminal`** — bash yes · dash yes · ksh93 no · zsh yes

Ties the monitor an *interactive* shell turns on for itself to having a
terminal. A different question from the one above: that is a script
asking with `set -m`, and this is nobody asking at all.

**The rule it qualifies is unanimous.** `-i script.sh` through a
pseudo-terminal turns the monitor on in all four — bash 5.3.15, dash,
ksh93u+ and zsh 5.9.2 all put `m` in `$-`, and the first three list
`monitor on`. What splits is the same invocation with no terminal
anywhere: ksh93 still reports `monitor on` and `imBE`, and the other
three leave it off. See docs/spec/invocation.md for the grid.

**It is not `MonitorNeedsATerminal` read twice**, and bash separates them
in one binary: `bash -c 'set -m'` with no terminal turns the monitor on,
and `bash -i script.sh` with no terminal leaves it off.

The terminal that counts is one on **any of the three standard streams**,
measured — a controlling terminal with all three redirected elsewhere is
not enough, and a pseudo-terminal on any one of the three alone is.

The preset says a terminal *is* needed, and this is the rarer case where
the text does not decide: XCU enables `-m` by default for interactive
shells and names no terminal in that sentence, but defines job control
throughout in terms of a controlling terminal — so the sentence is silent
about having none rather than permissive about it. Silent text gets the
answer that claims less, which is three of the four as well.

Read rather than `ask`ed, as `InteractiveOptionLetters` is: the answer is
wanted once at startup, so refusing over an unanswered field would put
"the shells disagree here" ahead of every `-i script.sh` under a preset
that has not chosen. Unanswered reads as yes, which leaves the monitor
off — the majority and the quiet answer.


**`InteractiveScriptAnnouncesJobs`** — bash no · dash yes · ksh93 yes · zsh yes

Gives an interactive shell running a **named script file** somebody to
tell about its jobs: the job number and pid as one starts, the `Done` row
as one ends.

Measured 2026-09-05 through a pseudo-terminal, scratch `HOME` and scratch
`HISTFILE`, on `sh -i script.sh` running `sleep 0.3 &` between two echoes:
bash 5.3.15, bash 3.2.57 and bash 3.2 run as `sh` print nothing at all;
dash prints the `Done` row and never the start; ksh93u+ and zsh print
both.

**It is not the monitor read a second time.** The monitor is unanimous on
this route with a terminal and this is not, so a front end that turned
both on together would give bash an announcement no bash makes. That is
why #793 turned the monitor on here and left this alone.

**It is however gated on the monitor.** Measured with no terminal
anywhere: dash and zsh leave the monitor off there and say nothing about
the job either, while ksh93 runs the monitor without one and announces
both ends. So the notice rides on the monitor, and this axis is what the
one dialect that runs a monitor and stays quiet anyway is for.

**It is not about where the commands come from either**, which is the
reading the grid rules out: `bash -i < script`, with the program on a
pipe and no terminal to read commands from, announces both — and so does
`bash -i -c`. bash is silent on exactly one interactive route, the one
whose program is a named file, so the axis names the route.

Separate from `AnnouncesBackgroundJob`, which asks whether the *start* is
announced at all and which dash alone answers no. Both are read here, and
dash is why they cannot be one field: it announces the end of a job on
this route and never the beginning.

The preset says no. XCU has nothing to say about a notice on this route,
and where the text is silent the preset takes the answer that claims less
— a shell that has not been asked for a job report does not write one. It
is the intersection as well: the panel is quiet here only if bash is, and
the core is the intersection rather than the majority.

Read rather than `ask`ed, exactly as `InteractiveMonitorNeedsATerminal`
is and for the same reason: the answer is wanted once at startup, so an
unanswered field would put "the shells disagree here" ahead of every
`-i script.sh` under a preset that has not chosen, including the scripts
that never start a job.

`-i -c` is a different split — bash, ksh93 and zsh announce there and dash
does not — and is therefore a different axis, not yet taken. See
docs/spec/invocation.md for the whole grid.


**`WaitReadsOptions`** — bash yes · dash yes · ksh93 yes · zsh no

Reads a leading `-` word as an option rather than as a job to wait for.
Three of the four do; zsh has none, and answers `wait -x` with the job
it could not find.


### `printf`

**`PrintfAssignsWithV`** — bash yes · dash no · ksh93 no · zsh yes

Makes `printf -v name fmt args` put the formatted text in a variable and
print nothing. True in bash and zsh; dash and ksh93 have no such option
and reject it as an unknown one.

It is how a script formats a value without a command substitution, so
without it the text goes to stdout and the variable stays empty — two
wrongs at once, and both silent.

**`PrintfBackslashC`** — bash literal · dash literal · ksh93 control character · zsh stops the output

Is what `\c` means in a printf format, and it is three different things
rather than a switch:

    printf "a\cbZ"   bash, dash  a\cbZ      two literal characters
                     ksh93       a<0x02>Z   \cX is control-X
                     zsh         a          the output stops there

Measured by the bytes rather than by the display, which is the only way
to tell the middle one from the last: ksh93's output *looks* truncated
next to zsh's until the control character is read as a byte.

**`PrintfEmptyIsNotANumber`** — bash yes · dash no · ksh93 no · zsh no

Complains about a numeric conversion given an operand that is present
and empty. bash alone: `printf '%d' ""` is an error there and a zero in
the other three, all of which print the zero anyway. An argument that is
*missing* is never an error in any of them.

**`PrintfQuote`** — bash backslash · dash absent · ksh93 single quoted · zsh backslash

Is how `%q` quotes, which is three answers and an absence rather than a
switch — see PrintfQuoteStyle.

**`PrintfRejectsUnknownOption`** — bash yes · dash yes · ksh93 yes · zsh no

Treats any leading word starting with `-` as an option and refuses one
it does not know. True in bash, dash and ksh93, where even `printf
"-%s\n" x` is an error because the format itself begins with a dash.

False in zsh, which recognizes the options it has and takes anything
else as the format — so `printf -q x` prints `-q` there and is an error
in the other three.

**`PrintfReportsBadNumber`** — bash yes · dash yes · ksh93 no · zsh no

Complains when a numeric conversion is given something that is not a
number. True in bash and dash, false in ksh93 and zsh — and all four
print the zero either way, so the complaint sits beside the output
rather than instead of it.


### `$_`, `$-` and the option letters

**`BadSetOptionNameFatal`** — bash no · dash yes · ksh93 yes · zsh yes

Ends the script when `set -o` is given a name this shell does not have.
True in dash, ksh93 and zsh.

Not the same question as BadOptionToSpecialBuiltinFatal, and measured
rather than assumed to be: a bad option *letter* to the same builtin is
fatal in only two of them, and zsh does not so much as complain about
`set -Q`. So one shell treats an unknown name as worse than an unknown
letter, which is why this is a field of its own.

**`DefaultOptionLetters`** — bash hB · dash  · ksh93 hB · zsh 569X

Is what `$-` starts with before the script has set anything: the single-
letter options a shell turns on at startup. Measured identical under
`-c`, a script file and standard input — bash and ksh93 report `hB`, zsh
`569X`, dash nothing at all.

The letters that describe the invocation *route* rather than an option a
script could set — `c` for a command string, `s` for standard input —
are not modeled, because the panel disagrees about them and no axis has
been asked yet: measured, ksh93 alone puts `s` in `$-` under `-c`, and
only bash and ksh93 put `c` there at all. `i` is the exception and is
modeled, by Runner.Interactive rather than here: it is unanimous, and it
is a fact about the invocation that the front end carries in rather than
a startup letter of the dialect's.

Nor are the letters a shell turns on only *when* it is interactive: bash
adds `H`, zsh adds `Z`, and ksh93 trades `h` for `mE`. That is a second,
per-dialect vector and nothing has needed it yet.

**`NoglobLetterIsF`** — bash yes · dash yes · ksh93 yes · zsh no

Puts `f` in `$-` while noglob is on, which is the letter POSIX gives it
and what bash, dash and ksh93 report. False in zsh, which reports the
capital: `-F` is the short option that means noglob there, `-f` being
about startup files — the same split SetFTurnsOffGlobbing records, seen
from the reading side.

**`SetFTurnsOffGlobbing`** — bash yes · dash yes · ksh93 yes · zsh no

Makes `set -f` the short spelling of `set -o noglob`. True in bash, dash
and ksh93. zsh spells that option the long way only: there `-f` is about
startup files and leaves globbing alone, so `set -f; echo *.txt` lists
the files.

**`SetHLetterTracksCommands`** — bash yes · dash yes · ksh93 yes · zsh no

Makes `set -h` the short spelling of command tracking — the option bash
lists as hashall and ksh93 as trackall, permission to remember where
commands were found. zsh answers no: its -h abbreviates histignoredups,
a history option, and leaves command hashing alone. Asked only where the
letter is written, like SetFTurnsOffGlobbing: the long names raise no
question.

**`SetHasTheHLetter`** — bash yes · dash no · ksh93 yes · zsh yes

Gives `set` the -h letter at all. Three of the four have it and no two
mean quite the same thing by it — which option it abbreviates is
SetHLetterTracksCommands — while dash refuses the letter outright,
fatally, the way it refuses any letter it does not have.

**`SetHasTraceLetters`** — bash yes · dash no · ksh93 no · zsh no

Gives `set` the -E and -T letters, which carry the ERR trap (and DEBUG
with RETURN) into functions and subshells the dialect otherwise bounds
them out of. bash alone: dash and ksh93 refuse the letters, and zsh
spells different options with them, so only a refusal is honest
elsewhere. Recorded as `opt/set-e-carries-the-err-trap`.

**`UnderscoreTracksTheLastArgument`** — bash yes · dash no · ksh93 no · zsh yes

Moves `$_` to the previous simple command's last expanded argument — the
command word itself when it had none, and empty after a bare assignment.
bash and zsh; dash and ksh93 keep no such parameter at all, so `$_` is an
ordinary unset name there and expands to nothing.

That last sentence used to read "leave it at the shell's own path
forever", in this entry and in the reason of
`special/underscore-follows-the-last-argument`, and the measurement
underneath both has always been an empty cell — the harness writes a
shell's path as `<shell>`, so a path would have been visible. Corrected
from the panel in the sweep of #500. `oracle-check` compares behavior and
not prose, which is why a wrong sentence over a right row can sit in two
places for as long as nobody re-reads it (#706).

One shape the axis does not cover, recorded rather than modeled because
nothing has needed it:

- **A declaration command binds something different in every shell that
  has `$_`.** After `export y=2`, bash 5.3 holds `y=2` — the assignment
  word as written — bash 3.2 holds `y`, and zsh holds `export`, the
  command word (`special/underscore-after-a-declaration-command`). The
  disagreement runs *through* bash, so this is one of the shapes a claim
  recorded against a single build gets wrong.

**`UnderscoreStartsAtTheInvocation`** — bash yes · dash no · ksh93 no · zsh no

Writes argv[0] into `$_` before the first command runs, so a script
reading it at the top finds how the shell was started. bash alone, in
both builds; the same binary reached under an argv[0] of `sh` writes
`sh`, so what is written is the invocation and not the executable, and
not `$0` either — a `-c` shell takes that from its first operand and
still writes the binary here (`special/underscore-at-startup`).

It is a second axis rather than a consequence of the one above, and zsh
is the reason: zsh tracks the last argument and still starts empty, so a
startup write gated on tracking would give zsh a value no zsh has. That
is what the issue proposing this predicted would work, and re-measuring
is what showed it does not.

**`UnderscoreInheritsFromTheEnvironment`** — bash yes · dash yes · ksh93 yes · zsh no

Lets an `_` the shell was handed in its environment show through.
Everywhere but zsh, which discards it and starts empty however it was
invoked (`special/underscore-inherited-from-the-environment`).

It decides what the axis above means: bash writes argv[0] only where the
environment said nothing, so an exported `_` wins over the invocation in
every shell that reads one. This is not a hypothetical shape — some
shells export `_` as the command they are about to run, so it is what a
shell started by another shell actually finds — and it is unreachable
from a snippet, since nothing running inside a shell can put a name in
the environment that shell was started with. The case that pins it
carries one.

Both were measured with `_` scrubbed from the environment and again with
`_=X` in it, on the `-c` route and the script route alike. Reading them
from an ordinary interactive shell measures the *invoking* shell's `_`
instead, which is a probe that answers even when the shell under test
keeps no such parameter.

**A prelude is not a command the script ran.** A dialect's prelude is
shell text, so it moves `$_` exactly as a script would, and bash's ends
in an assignment — which leaves the parameter empty in everything that
tracks it. The front end puts `$_` back after sourcing it
(`interp.Runner.ForgetLastArgument`), for the same reason it holds the
invocation's `set` options back until the prelude has run: tracing the
dialect's own plumbing under `-x`, or stopping on it under `-e`, reports
on machinery nobody wrote. Without that, the startup value is unreachable
through any binary that has a prelude at all, and the fix above would
have looked correct in the library and done nothing in the shell.


### `test` and `[`

**`TestAcceptsDoubleEqual`** — bash yes · dash no · ksh93 yes · zsh yes

Makes `==` a synonym for `=` in `test` and `[`, so `test a == a` is a
string comparison. True in bash, ksh93 and zsh.

False in dash, and false does not mean "compares unequal": it means the
word is not an operator at all, so `test a == b` is three words with no
operator among them and is reported as one. The answer therefore has to
come before the comparison, not after it.

This is only about `test` and `[`. Inside `[[ ]]` the same spelling is a
pattern match, which is a different question entirely.

**`TestIntegerRefusalIsSilent`** — bash no · dash no · ksh93 yes · zsh no

Has `[ a -eq 1 ]` fail with no sentence at status 1 — ksh93; the other
three complain at 2.


### the names a builtin will and will not take

**`BadNameToDeclarationFatal`** — bash no · dash yes · ksh93 yes · zsh yes

Ends the script when `export` or `readonly` is given an operand that is
not a name. True in dash, ksh93 and zsh; bash reports every bad operand,
exports the well-formed ones and carries on with a status of 1.

**`BadNameToUnsetFatal`** — bash no · dash yes · ksh93 no · zsh yes

Is that question again for `unset`, and is a separate field because
ksh93 answers the two differently: `export 1x` ends the script there
where `unset 1x` prints the same kind of complaint, returns 1 and
carries on.

Not a question about `unset` being less special than the other two — a
bad *option* to ksh93's `unset` is fatal, which is what makes the split
about the kind of failure rather than about the builtin.

**`UnsetReadonlyFatal`** — bash no · dash yes · ksh93 no · zsh yes

Ends the script when `unset` is asked to remove a readonly name.

**The refusal is not the axis.** Every shell in the panel refuses, says
so, and leaves the value standing — `readonly x=1; unset x; echo
"${x-gone}"` prints `1` in all six — so all three of those are the core
answer and only what follows the refusal splits. Measured 2026-09-05 with
a scratch `HOME`:

| shell | what it says | status | carries on? |
| --- | --- | --- | --- |
| bash 5.3 | `unset: x: cannot unset: readonly variable` | 1 | yes |
| bash 3.2 | the same, at `line 0` | 1 | yes |
| bash-as-`sh` | the same wording | 1 | **no** |
| dash | `unset: x: is read only` | 2 | **no** |
| ksh93 | `unset: warning: x: is read only` | 1 | yes |
| zsh | `read-only variable: x` | 1 | **no** |

The status needs nothing of its own: the three that stop carry
`FatalErrorStatusIsOne`'s number, which is where dash's 2 comes from, and
the three that carry on all report 1.

**It is not `BadNameToUnsetFatal` read twice**, though the two agree on
all four dialect defaults. bash's POSIX mode moves this one and not that
one: `set -o posix` makes bash 5.3 stop here, and makes no difference to
`unset 1x`, which bash 5.3 accepts in silence whatever the mode. That is
what `FatalErrorStatusIsOne`'s note describes — *which* errors are fatal
stays per-error even when two errors happen to split the panel alike.

**Nor is it `RedirectErrorOnSpecialBuiltinFatal`**, which is the axis it
most looks like. The sets are different and they cross:

| | redirection on a special builtin | `unset` of a readonly |
| --- | --- | --- |
| bash 5.3 | carries on | carries on |
| bash-as-`sh` | **stops** | **stops** |
| dash | **stops** | **stops** |
| ksh93 | **stops** | carries on |
| zsh | carries on | **stops** |

ksh93 and zsh swap sides, so one flag could not answer both. Like that
axis, though, a dialect's field is where the shell *starts* and its own
posix knob moves it — `set -o posix` and `set +o posix` in bash, and
`SetPosixMode` in the core, which is why the runner remembers a saved
answer per axis rather than one for the mode. zsh does **not** move:
`emulate sh`, `emulate ksh` and `emulate zsh` all stop.

bash 3.2 is fatal in neither mode, so what is recorded is bash 5's rule
rather than bash's.

The wording is `Diagnostics.UnsetReadonly`, one verb — the name. Three of
the four dialects name `unset` in the sentence and one of those three
calls the refusal a *warning*; the fourth writes exactly what it writes
for a refused assignment and names no builtin, so its wording must not
reach the location either. An empty value falls back to the default,
which is the wording the three bash columns share.

Two details of *which* name is refused, both unanimous where they can be
seen. A `readonly` name declared with no value is still refused — the
attribute is what is checked, not the value. And a subscripted operand is
refused by the variable the subscript indexes, naming the **base**:
`unset a[0]` against a readonly `a` says `a` and never `a[0]`, so the
refusal stands ahead of the element path and the subscript is never
evaluated.

The refusal is one name's rather than the builtin's. `readonly x=1; y=2;
unset x y` leaves `x` standing and removes `y` in every shell that gets
that far, so a single readonly operand does not save the rest, and the
status is still 1.

Corpus: `unset/a-readonly-name-is-refused`,
`unset/a-readonly-name-refused-alongside-another`,
`unset/a-readonly-name-with-no-value-is-refused-too`,
`unset/a-readonly-name-under-v-is-refused`,
`unset/a-readonly-name-behind-a-subscript-is-refused`.

**`DeclarePrintReportsAMissingName`** — bash yes · dash unspecified · ksh93 no · zsh yes

Makes `typeset -p nosuch` say so and fail. bash and zsh report it (with
their own wording — see Diagnostics.DeclareNoSuchVariable) and answer 1
even when other names listed fine; ksh93 prints nothing for the missing
name and answers 0.

**`PunctuatedFunctionNameIsRefused`** — bash no · dash no · ksh93 yes · zsh no

Stops the script when a function whose name carries `-` or `.` is
defined. ksh93 alone: bash and zsh define and run it, and dash never
parses the definition at all.

**`ReadRequiresAVariableName`** — bash no · dash yes · ksh93 no · zsh no

Refuses a bare `read`: dash's "arg count" at 2, where the other three
read into REPLY.

**`UnsetFunctionChecksTheName`** — bash no · dash no · ksh93 yes · zsh no

Judges the operand `unset -f` was given as a name, and refuses one that
could not be a function name. True in ksh93 alone.

Not the same question as the one below, and measured to be: ksh93
refuses `1x` and is quiet about a well formed name that is not defined,
where zsh is the other way round.


### arrays

**`ArrayBaseIsZero`** — bash yes · dash yes · ksh93 yes · zsh no

Indexes arrays from 0. True in bash and ksh93, false in zsh, which
counts from 1. dash has no arrays at all, which is why the axis is
absent rather than false there.

**`SubscriptCommaIsARange`** — bash no · dash no · ksh93 no · zsh yes

Reads the comma in `${a[1,3]}` as the separator of a range rather than as
the arithmetic comma operator, whose value is its right operand:

    a=(w x y z)
    ${a[1,3]}    bash, ksh93 → z          (element 3, the operator's value)
                 zsh         → w x y      (elements 1 through 3)
    ${a[1,2,3]}  bash, ksh93 → z
                 zsh         → bad substitution, status 1

The same characters with two meanings, which is what makes it an axis
rather than a grammar flag: `${a[1,3]}` is one subscript in every shell
that has subscripts at all, and they disagree about what it says. The
preset answers *no*, which is the standard's reading — POSIX has the
comma operator and no ranges — as well as four of the five shells'.

Asked only where the two readings differ, so `${a[2,2]}` needs no
answer. The endpoints, and the one asymmetry between an array and a
string, are in
`docs/spec/grammar/parameter-expansion.md`.

**`ScalarSubscriptIsACharacter`** — bash no · dash no · ksh93 no · zsh yes

Reads `${s[2]}` on a plain string as its second character rather than as
an element of the one-element array a scalar reads as:

    s=hello
    ${s[0]}  bash, ksh93 → hello      zsh → (empty)
    ${s[2]}  bash, ksh93 → (empty)    zsh → e

Both readings answer, neither reports, and an empty string is a
plausible element as well as a plausible miss — so a script cannot tell
which shell it is on except by the value. That empty rather than an
error is the non-zsh answer is measured and not defaulted: neither bash
nor ksh93 says anything about `${s[2]}`, and both give status 0.

A range and a character go together — `${s[2,4]}` is `ell` in the shell
that reads characters — but they are two axes, because `${a[1,3]}` on an
*array* is a range with no character in it.

**`MultibyteEncodingIsHonored`** — bash yes · dash no · ksh93 yes · zsh yes

Decodes the character encoding the locale names, so that `${#s}`,
`${s:off:len}` and a subscript on a scalar count characters rather than
bytes.

Measured 2026-09-05 on bash 5.3.15, bash 3.2.57, bash as `sh`, ksh93u+,
zsh 5.9.2 and dash:

    s=héllo; echo ${#s}     LC_ALL=C          → 6 in all six
                            LC_ALL=C.UTF-8    → 5 in all but dash, which says 6
    s=日本語; echo ${#s}     LC_ALL=C.UTF-8    → 3 in all but dash, which says 9

So the panel does **not** divide over whether a character is a byte. It
divides over whether the locale is consulted at all: dash has no
multibyte decoder and gives the byte count in every locale there is,
while the other four give the byte count too whenever the locale names a
single-byte encoding. Silent either way — both answers are plausible
numbers, and nothing is reported.

**The locale is not a second axis.** Which encoding is in force is
runtime state, read off the runner's own variables the way `PATH` and
`IFS` are, and it moves inside a running shell: `LC_ALL=C; s=héllo; echo
${#s}` gives 6 in every panel member with nothing exported. An axis keyed
on it would record the machine the measurement was taken on rather than
the rule — the same reasoning `driver/startup.go` writes down for POSIX
mode being read off the runner (#691, #733).

The precedence is measured rather than assumed: a non-empty `LC_ALL`
beats a non-empty `LC_CTYPE` beats `LANG`, and an *empty* one of the
first two is skipped rather than being an answer of its own, so
`LC_ALL= LANG=C.UTF-8` counts characters. A locale name with no codeset
in it — `LC_ALL=UTF-8`, which is not a locale name — is single-byte
here; the panel splits on it, and refusing it is what ksh93, zsh and
dash do.

This implementation decodes UTF-8 and nothing else. The single-byte
encodings are right by that rule (`C`, `POSIX` and `en_US.ISO8859-1` all
measure as bytes across the panel); `eucJP` and the other multibyte
codesets would each be a decoder, and are a known limit rather than an
answer.

Asked only where the two readings differ — a value whose bytes are all
ASCII is the same length and has its positions in the same places under
both — so the core, which refuses nearly every axis, still answers
`${#x}` on ordinary text.

An undecodable byte is one character of one byte and is handed back as
itself, which is unanimous: `s=$(printf 'a\200b')` has length 3 in every
panel member, and zsh's `${s[2]}` is that byte rather than a replacement
character.

**Pattern matching follows the same axis** (#905). `?` consumes one
character, a bracket matches one, and a `*` stops only between them, so
`${s#???}` on `héllo` is `lo` under a UTF-8 locale and `llo` under a
single-byte one — and `case héllo in ?????` matches under the first and
not the second, though the pattern is five ASCII bytes either way. That
is why the callers ask about the *subject* as well as the pattern, and
why pathname expansion asks about the names in the directory.

There is one matcher, and there always was: `case`, `[[ ]]`, `${x#pat}`,
`${x%pat}`, `${x/pat/rep}`, the element-selection operators and pathname
expansion all reach it. #899 read `${s%??}` giving `hél` as evidence
that suffix removal was already rune-aware; the probe cannot
discriminate, because `héllo` ends in two ASCII characters and two bytes
off the end leave the same four. `../spec/grammar/patterns.md` has the
range, the star and the character classes, each of which needed a
measurement of its own.

**`NegativeSubscriptPastTheStartInserts`** — bash no · dash unspecified · ksh93 no · zsh yes

Places a new element in front of every other when a negative subscript
counts back past the first one:

    a=(p q); a[-3]=x    bash, ksh93 → refused, script ends at 1
                        zsh         → [x][p][q], n=3

However far past the start it reached: `-3`, `-4` and `-5` all land in the
same place and the array grows by exactly one.

Asked only for a *negative* subscript that lands before the first element,
which is the only spelling that can. A non-negative one below the base —
`a[0]` where the first element is 1 — is refused by every shell measured,
zsh included, so it needs no answer from anyone.

**`ArrayLiteralSubscriptIsAKey`** — bash no · dash unspecified · ksh93 yes · zsh no

Reads a subscript written inside an array literal as the text between
the brackets rather than as an arithmetic expression, and — because the
two go together — makes such a literal declare a keyed array rather than
an indexed one. In ksh93 `a=([1+1]=c)` stores under the three
characters, `typeset -p a` answers `typeset -A`, and `${a[2]}` finds
nothing; bash and zsh evaluate the subscript and the value lands at 2.

One concept with two consequences, like whether an assignment prefix
survives a special builtin.

Asked only where the two readings differ. A plain decimal numeral
evaluates to itself, so `a=([2]=c)` fills the same slot either way and
never reaches the question — which is what keeps the ordinary way to
build a sparse array available in a core that has chosen no shell. dash
has no array literal at all, so the axis is absent there rather than
false.

**`ArrayLengthWithoutSubscriptIsCount`** — bash no · dash no · ksh93 no · zsh yes

Makes `${#a}` of an array the number of elements, which is zsh's
reading; bash and ksh93 measure the element the bare name yields. Asked
only where the two answers differ.

**`ArrayNameWithoutSubscriptIsTheList`** — bash no · dash unspecified · ksh93 no · zsh yes

Makes an unquoted bare array name the array itself — one field per
element, a slice slicing the list and an element-wise operator applying
to each — exactly as `${a[@]}` is. zsh reads it that way; bash and ksh93
read the bare name as `${a[0]}` and hand over one field, and dash has no
arrays, which is why the axis is absent rather than false there.

The field-count half of what `ArrayScalarIsTheWholeArray` answers for the
value, and separate from it because the same shell answers the two
differently by quoting: `"$a"` is one joined field in zsh as well. So the
divergence is exactly the *unquoted* spelling in a context that splits —
an assignment's value, a `case` subject and a here-document body join it
in every shell, measured.

Asked only where the two readings differ: more than one element, or
exactly one under an operator that still reads the list there — a slice,
whose offset counts elements rather than characters, and the three
element-selecting operators, which can leave the list empty.

**`ArrayScalarIsTheWholeArray`** — bash no · dash unspecified · ksh93 no · zsh yes

Decides what a plain `$a` gives when `a` is an array: zsh says every
element joined by the first character of `IFS`, and bash and ksh93 say
the element at the base position alone. dash has no arrays, which is why
the axis is absent rather than false there.

It decides **whether the bare name is set**, and not only what it holds,
because the two are one question. Where a bare name is the whole list it
is set whenever the array is, an array with no elements included — that
reads as the empty string. Where it is one element it *is* `${a[base]}`:
the element at the base position, and **not the lowest subscript that
happens to be assigned**. A name whose base element was removed, or was
never written, is therefore unset while the array still holds elements.

Measured 2026-09-06, from a script file and through `-c` alike, `env -i`
with a scratch `HOME`. On `a=(x y z); unset "a[0]"`, bash 5.3.15, bash as
`sh`, bash 3.2.57 and ksh93u+ all leave `${a-U}` at `U`, `${a+S}` empty,
`${#a}` at 0, `"$a"` an empty field, `$((a+5))` at 5 and `set -u; $a` an
unbound variable — while `${a[@]}` still yields `y z` and `${a[@]+S}` is
still set, so it is the bare name that is unset and not the array.
ksh93's `set -u` complaint names `a[0]` rather than `a`, which says the
reading out loud. `a[5]=q`, where no base element was ever written, reads
the same way, so it is the position and not the removal that decides.

zsh asks nothing here: it reads the list, and its own `unset "a[1]"`
leaves an empty element in place rather than a gap — see
`UnsetArraySpan`.

Reading the lowest assigned subscript instead answered `y` for the bare
name and `7` for the arithmetic reference, both of them plausible values
at status 0 (#998).

**`ArraysAreSparse`** — bash yes · dash unspecified · ksh93 yes · zsh no

Makes an unassigned subscript no element at all, so `a=(x); a[5]=y` is
an array of two. True in bash and ksh93; zsh reads the whole extent and
finds the gap empty, giving five.

The store is sparse either way — only the reading differs — so this is
asked when an array *has* a gap and never otherwise, which is almost
every array there is.

**`EmptyArrayAtIsOneEmptyField`** — bash no · dash no · ksh93 yes · zsh no

Hands a quoted "${a[@]}" of an empty array one empty field: ksh93 alone,
and the reason careful scripts write "${a[@]+"${a[@]}"}".


### arithmetic

**`ArithBaseAbove36`** — bash yes · dash yes · ksh93 yes · zsh no

Admits `37#…` through `64#…`, whose letters split into cases and whose
last two digits are `@` and `_`. bash and ksh93 take the full 64; zsh
stops at 36 and says so.

**`ArithIntegerOperatorRefusesFloat`** — bash unspecified · dash unspecified · ksh93 yes · zsh no

Rejects a float where only an integer will do — `7 % 2.5`, `1.5 & 1`, a
shift. ksh93 says yes and refuses; zsh says no and truncates. It does
not arise in a shell without floats, which is why bash and dash leave it
unanswered.

**`ArithOverflowSaturates`** — bash no · dash no · ksh93 yes · zsh no

Clamps integer overflow at the edge: ksh93 holds max+1 at the maximum
where the other shells wrap. Asked only when an overflow actually
happened.

**`EmptyArithExpressionIsAnError`** — bash no · dash yes · ksh93 no · zsh no

Refuses `$(( ))`: dash wants a primary and stops the script; the other
three answer zero.

**`ShiftCountIsArithmetic`** — bash no · dash no · ksh93 yes · zsh yes

Reads `shift`'s operand as an expression rather than as a plain number:
`shift 1+1` moves two and `shift n` moves whatever n holds.

ksh93 and zsh do. An unset name is zero in an expression, so `shift abc`
shifts nothing and succeeds there, where bash and dash call it a number
they cannot read.


### `select`

**`SelectAssumesUnboundedWidth`** — bash no · dash unspecified · ksh93 unspecified · zsh yes

Treats an unset COLUMNS as no limit rather than as 80. zsh says yes —
with no terminal to ask it puts forty items on one line — and bash says
no. It does not arise for a menu that is always vertical, which is why
ksh93 leaves it unanswered.

**`SelectEofEndsPromptLine`** — bash no · dash unspecified · ksh93 no · zsh yes

Writes a newline to standard error when the input runs out, closing the
line the prompt left open. zsh alone does; bash closes the line on
standard *output* instead, which is a different question and the axis
`SelectEofPrintsNewline` below.

**`SelectEofIsSuccess`** — bash no · dash unspecified · ksh93 no · zsh yes

Makes the input running out a success. zsh alone says yes; the other two
report 1.

**`SelectEofPrintsNewline`** — bash yes · dash unspecified · ksh93 no · zsh no

Writes a newline to standard *output* when the input runs out — the one
thing this loop prints that does not go to standard error. bash alone
does it.

**`SelectPromptNeedsTerminal`** — bash no · dash unspecified · ksh93 yes · zsh no

Withholds PS3 unless the input is a terminal. ksh93 alone says yes,
which is why a ksh93 script's transcript has the menu in it and no
prompt.

**`SelectTakesUnterminatedReply`** — bash no · dash unspecified · ksh93 no · zsh yes

Counts a final reply that has no trailing newline. zsh alone: `printf 2
| sh -c 'select x in a b; do ...'` picks `b` there, and bash and ksh93
ignore the line and end the loop with 1.

The same question `read` answers, and the opposite outcome — the panel
is unanimous for `read` and split here, so that one is the core's
behavior and this one is an axis. Reachable only from a pipe or a file,
since a terminal ends every line.


### `exec`, `.` and `local`

**`DotFallsBackToCurrentDirectory`** — bash yes · dash no · ksh93 no · zsh no

Looks in the current directory for a `.` operand with no slash in it,
after PATH has missed.

True only in bash. PATH is searched first everywhere, and wins over an
identically named file in the current directory in all four — this is
only about what happens when PATH does not have it.

**`DotPassesArguments`** — bash yes · dash no · ksh93 yes · zsh yes

Gives a sourced file its own positional parameters from the words after
the filename, restoring the caller's afterwards.

False in dash, which ignores them, so `. f.sh ARG` leaves `$1` as the
caller's; true in bash, ksh93 and zsh. With no words after the filename
every shell leaves the parameters alone, so the axis only speaks when
there are some.

**`DotWithNoOperandIsAnError`** — bash yes · dash no · ksh93 yes · zsh yes

Decides whether `.` with no filename is a failure at all. False in dash,
which does nothing and reports success; true in bash, ksh93 and zsh.

Separate from the status and from the fatality because the panel splits
four ways on `.` alone — dash 0, bash 2 surviving, ksh93 2 fatal, zsh 1
surviving — and one field with four answers would have to invent a type
to hold what is really three independent questions.

**`ExecFailureRunsExitTrap`** — bash yes · dash yes · ksh93 no · zsh no

Runs a `trap … EXIT` handler when `exec` could not run the command it
was given. True in dash and bash, false in ksh93 and zsh.

A *successful* exec runs no handler anywhere, and that is not an axis:
the trap died with the process the exec replaced. Only the failure has a
shell left to decide anything, and the panel splits on it.

**`ExecTakesOptions`** — bash yes · dash no · ksh93 yes · zsh yes

Lets `exec` read options of its own, such as `-a name` to choose the
argv[0] the command sees. True in bash, ksh93 and zsh; false in dash,
where a leading `-a` is the name of a command and is reported as not
found.

The answer has to come before the command is looked up, because it
decides which word the command is.

**`LocalOutsideAFunctionIsAnError`** — bash yes · dash yes · ksh93 yes · zsh no

Refuses `local x=2` written where there is no function to be local to.

bash and dash refuse it, zsh takes it and sets a global instead. ksh93
has no `local` at all, so it never reaches the question.

**`LocalOutsideAFunctionIsFatal`** — bash no · dash yes · ksh93 unspecified · zsh unspecified

Ends the script rather than carrying on after that refusal. dash does;
bash says the same thing and runs the next command.


### signals, status and interruption

**`ChildInterruptEndsTheScript`** — bash no · dash no · ksh93 yes · zsh no

Stops the script when a child was ended by an interrupt, instead of
carrying on with the next command. True in ksh93 alone, and for SIGINT
alone — measured across QUIT, TERM, HUP, USR1 and PIPE, every one of
which it carries on from.

It ends the whole script rather than the construct around it: from
inside a loop, the loop and everything after it are abandoned too. The
status is 128 plus the signal, which is not the same shell's answer for
a command killed by one — that is 256 plus it.

**`ExitInTrapReportsEarlierStatus`** — bash yes · dash yes · ksh93 yes · zsh no

Makes a bare `exit` in an EXIT trap report the status the shell had when
the trap began, rather than that of the trap's own last command.

    trap "false; exit" 0; true

is 0 in bash, dash and ksh93 and 1 in zsh. Only the bare form: `exit 7`
is 7 everywhere, and a trap that does not exit at all leaves the
script's status alone in all four.

Found on an installed script — /usr/bin/bzless traps `stty …; exit` on
EXIT, and the `stty` failing made the script exit 1 where every shell
exits 0. A wrong exit status is what a caller branches on, so this is
the quiet kind of difference.

**`ReportsACommandKilledBySignal`** — bash yes · dash yes · ksh93 yes · zsh no

Says out loud that a signal ended a command, rather than leaving the
status to carry it alone. True in bash, dash and ksh93; zsh says nothing
— measured with a terminal as well as without one, so it is not the
prompt-only rule that governs a background job's announcement.

Not asked for the two signals nothing reports. ^C and a broken pipe are
how a command is meant to end, and all four stay quiet about those, so
there is no disagreement there to put to a dialect.

**`ReportsAKilledCommandInACommandSubstitution`** — bash no · dash yes · ksh93 yes · zsh unspecified

Remarks on a command that a signal ended inside `$(…)`.

bash does not, and does remark on the same command inside `( … )`, so
this is not the subshell question in another spelling. dash and ksh93
report it wherever it happened; zsh remarks on none of them and never
reaches this.

Asked only inside a substitution, so the three dialects that answer the
wider question the same way everywhere are not asked twice.

**`ReportsAnyKilledPipelineElement`** — bash no · dash yes · ksh93 no · zsh no

Remarks on a signal that ended an element of a pipeline other than the
last. True in dash alone.

bash and ksh93 report only the element whose status the pipeline takes:
`sh -c 'kill -ABRT $$' | cat` is silent in both, and the same command as
the *last* element is not. dash says the same thing wherever the element
stands.

Unreachable in zsh, which says nothing about a killed command at all, so
the question never arises there.

**`SIGPrefixAccepted`** — bash yes · dash no · ksh93 yes · zsh yes

Reads `SIGINT` as a name for the same signal `INT` names, wherever a
signal can be named.

dash alone says no, and says it in three places for two different
reasons — the prefix is simply not part of a signal's name there:

    trap 'x' SIGINT   trap: SIGINT: bad trap
    kill -SIGINT $$   kill: Illegal option -S
    kill -s SIGINT $$ kill: invalid signal number or name: SIGINT

It is one axis rather than one per builtin because it is a property of
how the shell reads a signal name, and the shell that refuses it refuses
it everywhere. Asked only where the prefix is actually present and
stripping it would name a signal: `trap 'x' INT` needs no answer from
anyone, and neither does `SIGNOPE`, which names nothing either way.

**`SignalHandlerSeesEarlierStatus`** — bash no · dash no · ksh93 no · zsh yes

Shows a signal handler the status from before the command that triggered
it rather than that command's own. zsh alone: after `false; kill -INT
$$`, zsh's handler reads 1 where the others read 0, because `kill`
succeeded.


### descriptor variables and redirection targets

**`FdVariableBadCloseIsAnError`** — bash yes · dash yes · ksh93 no · zsh yes

Refuses `exec {name}>&-` when the name holds no descriptor number. ksh93
says nothing and reports success.

**`FdVariableOutlivesTheCommand`** — bash yes · dash yes · ksh93 no · zsh yes

Keeps a `{name}>f` descriptor open past the simple command that carried
it — two of the three that have the grammar; ksh93 takes it back with
the command's other redirections, so the number the variable holds is
already dead.

**`FdNumberBoundedByOpenFileLimit`** — bash yes · dash no · ksh93 yes · zsh no

Refuses a redirection whose descriptor number is at or above the
process's soft limit on open files.

**No shell in the panel has a ceiling of its own.** There is no language
constant to look for; the bound is the kernel's, and the shells differ
only in whether they hand its refusal back. Measured on macOS,
2026-09-05, by moving the limit rather than by finding the default —
which is what shows it is the limit and not a number somebody chose:

    ulimit -n 20; exec 19>f     silent, status 0, in all five
    ulimit -n 20; exec 20>f     bash: `20: Bad file descriptor`, status 1
    ulimit -n 20; exec 20<f     the same — the direction does not matter
    ulimit -n 20; echo hi 20>f  the same, on a command's own redirection
    ulimit -n 6;  exec 8>f      bash: `8: Bad file descriptor`
                                ksh93: `bad file unit number [Invalid
                                       argument]`, and the shell ends
                                dash, zsh: status 0, and the descriptor
                                       is unusable afterwards
    ulimit -n 20; exec 20>fresh the file is created either way: the open
                                happens and it is the *number* that
                                cannot be had

bash and ksh93 report the errno they were given, and they were given
different ones — EBADF against EINVAL — which is why the wording is a
Diagnostics field (`FdNumberOverLimit`) rather than one sentence with the
number substituted in. dash and zsh do not ask, which is the shape of not
looking rather than of a different answer: the redirection reports
success and then nothing aimed at that number works.

Reached most often through `MultiDigitFdNumber`, since a script that may
write only one digit can only get here under a limit below ten. It is why
`exec 1000000>f` was accepted here and refused by both bash builds, which
is what this axis was opened for.

A number the *shell* picks is checked too, and that is measured rather
than assumed: `ulimit -n 6; exec {v}>f` fails in all three shells that
have the construct, since the number they pick is over the limit like any
other. They word it three ways — `cannot duplicate fd`, `cannot open`,
`cannot move fd 3` — and we say what we say about a number the script
wrote, which is the shape of the failure without the sentence. It is
reachable only under a limit below ten, where the picking starts.

The axis is asked at the disagreement and never on the common path: a
number below the limit is nobody's question, and a Runner with no
`GetRlimit` has no limit to be asked about — a library that was handed no
limits is not the place to invent one. Recorded as
`redir/a-descriptor-number-over-the-open-file-limit`.

**`RedirectTargetIsAnOrdinaryWord`** — bash yes · dash no · ksh93 no · zsh no

Expands a redirection's target the way an argument is expanded — split
into fields and matched as a pattern — and requires the result to be
exactly one word. True in bash alone:

    e="a b"; echo hi > $e      bash refuses; the rest write to `a b`
    e="x*";  echo hi > $e      bash refuses where two files match, and
                               writes to the match where one does; the
                               rest create a file named `x*`

The other three expand it and stop there: no splitting, no matching,
whatever it came to is the name. A tilde expands either way.

Doing bash's expansion and then quietly taking the first field is the
answer no shell gives, and it is the one this had: `> $e` wrote to `a`,
and `> $e` with a pattern truncated whichever file happened to match.


### `getopts`, `shift`, `times`, `type`, `ulimit` and `fc`

**`FcEmptyHistoryIsAnError`** — bash no · dash no · ksh93 no · zsh yes

Has `fc` report the event it cannot find — zsh; bash and dash answer a
script with silence at 0.

**OPTIND is 1 before `getopts` has ever run.** Not an axis — every shell
in the panel initializes it at startup rather than when the builtin first
executes (`getopts/optind-starts-at-one`), so a script that reads it
before entering its option loop, or that is handed no options at all,
still finds a number. Leaving it unset until the first call is invisible
to any case whose loop runs, which is how it survived here until
`getopts/a-function-with-its-own-optind` read it back afterwards.

The startup value is *written* and not merely defaulted: an `OPTIND` in
the environment is overwritten with 1 by all six
(`getopts/optind-ignores-an-inherited-value`), so it cannot be supplied
by falling back to the inherited value when nothing has set the variable.
`OPTIND` is not exported, so the only way a script sees an inherited one
is a caller that exported it on purpose — and the panel still refuses to
start a scan in the middle.

**`GetoptsAssignmentRestartsWord`** — bash yes · dash yes · ksh93 yes · zsh no

Makes assigning OPTIND begin the word again, dropping any position
inside a cluster.

True in bash, dash and ksh93, and it is the *assignment* that does it
rather than the value: `set -- -ab; getopts ab o; OPTIND=1` writes the
number OPTIND already held, and those three still restart and read `a` a
second time where zsh carries on to `b`.

**`GetoptsClearsOptarg`** — bash no · dash no · ksh93 no · zsh yes

Empties OPTARG when `getopts` reports a bad option rather than leaving
it unset. zsh alone, and a script testing `${OPTARG-}` can tell the two
apart.

**`ShiftPastEndFatal`** — bash no · dash yes · ksh93 yes · zsh no

Ends a non-interactive shell when `shift` runs off the end. True in dash
and ksh93.

**`ShiftOptionWords`** — bash none · dash none · ksh93 every dash word · zsh an option unless it is all digits

Which leading-`-` words `shift` reads as options rather than as its
count. Three answers rather than a presence, because the panel splits on
*which* dash words and not on whether there are any:

    shift -x   bash `-x: numeric argument required`   dash `Illegal number: -x`
               ksh93 `-x: unknown option` and a usage line
               zsh `bad option: -x`
    shift -1   bash `-1: shift count out of range`    dash `Illegal number: -1`
               ksh93 `-1: unknown option` and a usage line
               zsh `argument to shift must be non-negative`
    shift -0   bash, dash, zsh  shift nothing, status 0
               ksh93 `-0: unknown option` and a usage line

zsh is what makes it three: it refuses `-x` as an option and reads `-1`
as a count. A bool could say "reads options" or "does not", and neither
describes zsh. `-0` is the sharpest row — ksh93 alone against the rest.

A lone `-` is not a dash word in any of the readings and reaches the
count everywhere here, which is bash's and dash's answer for it; ksh93
and zsh diverge on that one word (`more tokens expected`, and zsh
consumes it and shifts one) and are not modeled.

**`ShiftDoubleDashEndsOptions`** — bash yes · dash no · ksh93 yes · zsh yes

Takes `--` as the end-of-options marker and reads what follows as the
count.

    shift -- 2    bash, ksh93, zsh  shift two
                  dash `Illegal number: --`
    shift --      bash, ksh93, zsh  the count falls back to one
                  dash `Illegal number: --`
    shift -- -1   bash `-1: shift count out of range`
                  ksh93 `-1: bad number`
                  zsh `argument to shift must be non-negative`
                  dash `Illegal number: --`

It is **not** `ShiftOptionWords`: bash reads no dash word as an option
and honors the marker anyway, so one shell answers the two differently
and one axis cannot hold both. What is past the marker is an operand and
not an option, which is where ksh93's two answers to `-1` come from.
Only the *first* `--` is the marker — `shift -- --` complains about the
second in all three that take one.

**`ShiftNegativeIsOutOfRange`** — bash yes · dash no · ksh93 yes · zsh yes

Reads a count below zero as a number that is out of range rather than as
a word that is not a number.

It is the other end of `ShiftTooMany` — one count, out of range in two
directions — so the same `ShiftPastEndFatal` decides whether it ends the
script, and it does: fatal in dash and ksh93, survivable in bash and zsh,
with `$#` untouched either way and status 1 where it is survived. Only
the wording differs by direction, which is why `Diagnostics` has a field
for each: bash is silent above `$#` and says `shift count out of range`
below zero, zsh has a sentence for each end, and ksh93 reuses `bad
number` for both. dash answers no and calls a negative count an illegal
number, which is the same complaint it makes about `-x`.

In ksh93 a negative count is only reachable *after* the marker, because
a bare `-1` is an option there.

A count may also carry a `+`, which is unanimous and asks nothing:
`shift +1` moves one in all six. The count reader had no sign at all
before #779, which is how `shift -1` reached a slice bound and panicked
in the two dialects that evaluate the operand as an expression.

**The `shift` shapes measured in the sweep of #500**, and where they
landed:

- **Past the end with a count**, `set -- a b; shift 5`. Prior work of our
  own had this as "moves nothing and answers 1", measured against bash.
  That is bash and zsh; dash and ksh93 end the script, which is
  `ShiftPastEndFatal` arriving with a count rather than without one. And
  the same bash 5.3 binary is *silent* as `bash` and prints `shift count
  out of range` as `sh`, so the diagnostic belongs to argv[0] and not to
  the build (`shift/past-the-end-with-a-count`).
- **A negative count** divides them on what kind of word it is before it
  divides them on the answer, which is why it took two axes rather than
  the one #779 expected: `ShiftOptionWords` says whether `-1` is even a
  count, and `ShiftNegativeIsOutOfRange` says what a count below zero
  is (`shift/a-negative-count`,
  `shift/a-negative-count-after-the-marker`).
- **`shift -- 2`** shifts two in five of the six; dash calls `--` an
  illegal number, having no option parsing here for it to end. #779 read
  this as belonging to the option axis; bash answers the two differently
  in one binary, so it is its own
  (`shift/a-double-dash-before-the-count`,
  `shift/a-double-dash-alone-still-shifts-one`).

One thing measured here and **not** modeled: a word of nothing but
dashes. `shift ---` is `---: more tokens expected` in ksh93 and
`bad option: --` in zsh, where the shared bundle reader names `----` —
`firstOptionLetter` has no letter to find and hands back the whole word,
which then gets a dash put in front of it. It is the bundle reader's
question rather than `shift`'s and no case pins it.

**`TimesRejectsArguments`** — bash no · dash no · ksh93 no · zsh yes

Makes `times` refuse an argument rather than ignore it. True in zsh,
false in dash and bash.

ksh93 answers neither: `times` is a reserved word there, so `times foo`
is a *syntax* error and no builtin ever runs. That is a grammar question
rather than this one, and it is recorded in the corpus rather than
modeled here.

**`TypeEndsOptionsWithDashDash`** — bash yes · dash no · ksh93 yes · zsh yes

Makes `type -- name` skip the `--`. True in bash, ksh93 and zsh; dash
has no options for it at all, so `--` is a name there and gets answered
as one before the real names are.

**`TypeNamesTheKindWithDashT`** — bash yes · dash no · ksh93 no · zsh no

Gives `type` its `-t`, which answers one bare word per name — keyword,
function, builtin or file — and prints nothing at all for a name it
cannot account for, only the failing status. The scripted form of the
question: a word to compare against rather than a sentence to parse.
True in bash alone; ksh93 and zsh refuse the letter the way they refuse
any option they do not have, and dash reads it as a name like the rest
of its operands.

**`TypePrintsFunctionBody`** — bash yes · dash no · ksh93 no · zsh no

Makes `type name` follow "name is a function" with the function itself,
reformatted. True in bash alone; the other three stop at the sentence.

**`UlimitSetsBothLimits`** — bash yes · dash yes · ksh93 yes · zsh no

Lowers the hard limit along with the soft one when neither -H nor -S was
given — which is what makes `ulimit -t 3600` irreversible. True in bash,
dash and ksh93.

False in zsh, which sets only the soft limit and leaves the hard one
where it was, so the same line there can be undone.

Pinned by `ulimit/setting-with-neither-letter-moves-both`, which compares
the hard limit against the value it set rather than printing it: what a
hard limit starts at is a property of the machine, and a row that
recorded it would record where it was generated.


### declarations, `export` and `readonly`

**`DeclarationNameOperands`** — bash PlainNamesOnly · dash PlainNamesOnly · ksh93 PlainNamesOnly · zsh NamesAndSpecialParameters

Says what may stand where `export` and `readonly` want a name, beyond a
plain name itself.

zsh is the only one that takes anything more: the special parameters are
names to it, which is why `export -` is a complaint in three of the four
and not in the fourth.

**`DeclarationTakesASubscript`** — bash no · dash no · ksh93 yes · zsh no

Accepts `export a[0]` and `readonly a[0]`, naming an element rather than
a variable. ksh93 does; bash and dash refuse it in the words they give
any other bad name.

A separate question from the name strictness above, because the answer
is per builtin: bash refuses it here and takes it for `unset`, and the
two builtins sit on different strictnesses in every shell, so no rule
over that strictness gives all four.

**`DeclareListing`** — bash DeclareListingClustered · dash DeclarationListingUnspecified · ksh93 DeclareListingBareAssignments · zsh DeclareListingExportSpelled

Is the shape of what `declare -p` and `typeset -p` write back. Three
engines rather than two answers — see DeclarationListingForm.

**`DeclareValueQuoting`** — bash ListingQuoteAlwaysDouble · dash ListingQuoteAlwaysEscaped · ksh93 ListingQuoteWhenNeededDollar · zsh ListingQuoteWhenNeededEscaped

Is how a listed declaration spells its value. A field of its own over
the shared vocabulary because it does not follow the dialect's other
listings: the engine that single-quotes its aliases and traps double-
quotes its declarations.

**`DeclaredNameWithoutValueIsEmpty`** — bash no · dash no · ksh93 no · zsh yes

Gives a name a value when it is declared without one: `local u` or
`typeset u`. zsh alone says yes, so `${u-UNSET}` is empty there and
UNSET in bash and ksh93 — the name exists in all three, but only zsh
considers it set.

It is about a name the declaration *creates*, and the cell rather than
the name: a shadow a function's declaration takes is a new cell however
much the caller held, and a second declaration in the same scope is
writing over its own. A name that already holds a value keeps it — see
"an attribute added to a name that already holds a value", above.

**`ExportCarriesFunctions`** — bash yes · dash no · ksh93 no · zsh no

Gives `export` its `-f`, which writes a function into a child's
environment. True in bash alone: the other three have no way to carry a
function at all, and each rejects the option as an option — two of them
fatally.

**`ExportTakesTheAttributeOff`** — bash yes · dash no · ksh93 no · zsh no

Gives `export` its `-n`, which takes the export attribute off a name and
leaves the name itself alone. True in bash alone. What the letter *means*
is not in question anywhere it exists — the name stays set in the shell
and stops reaching a child — so the axis is about availability and there
is no wording beside it: a dialect that says no sends `-n` down the
ordinary unknown-option path and collects its own refusal. Measured
2026-09-05: `dash: 1: export: Illegal option -n` and the script ends,
`ksh: export: -n: unknown option` with a usage line and the script ends,
`zsh:export:1: bad option: -n` with `export` failing at 1 and the script
carrying on. The POSIX preset says no from the text, which spells
`export` with `-p` and nothing else.

**`ExportListing`** — bash DeclareListingClustered · dash DeclareListingCommandWord · ksh93 DeclareListingCommandWord · zsh DeclareListingCommandWord

Is the shape `export -p` writes: bash spells each name as a clustered
declaration (`declare -x V="1"`), and the other three repeat the command
word (`export V='1'`).

**`ReadonlyListing`** — bash DeclareListingClustered · dash DeclareListingCommandWord · ksh93 DeclareListingCommandWord · zsh DeclareListingExportSpelled

Is the same question from `readonly -p`, where zsh parts ways with its
own export listing and writes `typeset -r R=2`.

**`ReadonlyReassignmentByDeclarationFatal`** — bash no · dash yes · ksh93 yes · zsh yes

Ends the script when a declaration utility assigns to a readonly name —
`export x=2`, `typeset x=2`. True in dash, ksh93 and zsh; bash reports
it and carries on.

A different set of shells from the plain assignment above, which is what
makes it a question of its own: bash stops for `x=2` given as an
argument and never stops for this one.

**`ReadonlyReassignmentFatal`** — bash no · dash yes · ksh93 yes · zsh yes

Ends the script when a readonly variable is assigned. True everywhere
but bash, measured with a plain assignment in a script file — adding a
redirect makes it a command and reverses the answer, which is the
contaminated-probe trap docs/spec/oracle.md records.

**`ReadonlyReassignmentFatalFromCommandString`** — bash yes · dash yes · ksh93 yes · zsh yes

Is the same question for a shell whose program came from an argument
rather than from a file.

One dialect answers the two differently: `bash -c 'readonly x=1; x=2;
echo after'` stops and exits 1, and the same three lines in a file print
`after` and exit 0. The other three are fatal either way.

Asked only for an assignment standing as a command of its own. The
dialect that splits is not fatal for `export x=2` or `x=2 cmd` by either
route, so those keep the answer above.

**`ValuelessDeclarationHidesTheOuterValue`** — bash yes · dash no · ksh93 yes · zsh no

Makes `local u` in a function hide any outer `u` — the local exists
unset, so `${u-UNSET}` fires the default even when the caller had a
value. Reached only when DeclaredNameWithoutValueIsEmpty said no: a name
declared *empty* hides the outer value by having one of its own.

bash hides it, and so does ksh93's `typeset` in a keyword function; dash
leaves the caller's value showing through until the first assignment.
This is the shape used to declare a local before assigning it
conditionally, so the difference is silent: the function reads the
caller's value where it expected nothing.

**Asked of the declaration and not of the scope.** The value hidden is an
*outer* one, so a second declaration of a name its own scope has already
declared asks nothing: it stands in front of the local the first one
made, and every shell with the builtin leaves that standing. See the note
under `LocalInheritsTheExportAttribute` for the measurement and for the
`typeset -g` route to the same place (#999).

**`DeclarationAssignmentClearsTheExportAttribute`** — bash no · dash absent · ksh93 yes · zsh no

Takes the export attribute off a name a declaration utility assigns to.

    export FOO=bar; typeset FOO=baz; env | grep '^FOO='

    bash 5.3, bash 3.2, bash as sh, zsh   FOO=baz
    ksh93                                 nothing, now and afterwards

One shell resets it. The name goes on holding `baz` — `typeset -p` says
so, `export -p` no longer lists it — and no child is told about it again
until something names the attribute. That last part is what makes it a
reset rather than a refusal: `export FOO` afterwards puts it back.

**The value on the line is what asks it.** A valueless `typeset FOO`
leaves the attribute alone in every shell, and so do the valueless
declarations that change the value anyway — `typeset -i FOO` stores 0
over a non-numeric value and `typeset -u FOO` folds what is there, and a
child is told about both. `readonly FOO=baz` clears it, because in the
shell that does this `readonly` *is* its `typeset -r`; `export FOO=baz`
does not, because it names the attribute; and a plain `FOO=baz` does not
in any shell, which is the boundary the axis is drawn at.

Asked only where the name was already exported, where the declaration
does not name the attribute itself, and where the declaration did **not**
take a scope. The scoped half is `LocalInheritsTheExportAttribute` below
— the same shell's answer arrived at from the other side — and the two
must not both fire: in a keyword function the attribute comes back on
return, and in a function whose declarations reach the caller it is gone
for good.

    export FOO=bar
    f() { typeset FOO=baz; }             # no scope: gone for good
    function f { typeset FOO=baz; }      # a scope: back on return

dash is absent rather than no: it has no `typeset` at all. It does have
`readonly`, and it keeps the attribute there, which is the one column
that reaches this axis by the other spelling.

The preset is no. POSIX has an exported name keep the attribute for the
life of the shell, and both other shells with the builtin agree.

**`LocalInheritsTheExportAttribute`** — bash yes · dash yes · ksh93 no · zsh no

Gives a local declaration the export attribute of the name it shadows,
so a child sees the local's value under that name. Asked only where the
shadowed name is exported — explicitly or by having been inherited — and
only where a scope was actually taken; a local declared `-x` says so
outright and asks nothing.

Measured through a real child, because what the attribute decides is
what a command is told: `export FOO=bar; f() { local FOO=baz; env; }`
shows the child `FOO=baz` in bash and dash and no `FOO` at all in zsh.
The same split holds for a name that arrived in the environment rather
than being exported by hand, which is the same question by the other
route.

ksh93 has no `local`, so the question reaches it only through `typeset`
in a keyword function — where the child is told nothing, as in zsh. It
gets there from further away, and the axis above is the rest of the road:
that shell's `typeset` takes the export attribute off any name it
assigns, at the top level as well as in a function. Both halves are
modeled, and each is asked where the other is not.

Two neighbors are *not* this axis. What a valueless declaration leaves
visible is `ValuelessDeclarationHidesTheOuterValue`, and the two compose:
`local FOO` over an exported `FOO` shows dash's child the outer value,
because dash hides nothing, and shows zsh's child nothing, because zsh
exports nothing.

bash is the residue, and it is not a third axis. Only a dialect that
answers **both** of the two yes reaches the question at all — dash has
nothing hidden to tell a child about and zsh has nothing exported — so
there is one dialect here and no disagreement for a switch to hold. It
is a value that dialect has to pick, and the two candidates are two
builds of it:

    export FOO=bar; f() { local FOO; env | grep '^FOO='; }; f

    bash 5.3    FOO=bar     the value the local hid
    bash 3.2    (nothing)

**Ours is 5.3's**, and the reason is not that it is newer. Three
measurements decided it:

- POSIX mode is not the variable. The same 5.3 invoked as `sh` — the
  panel's own `bash-as-sh` column, and a distinct member — gives the
  same `FOO=bar`, and `/bin/sh` on this machine, which is 3.2, gives
  nothing. Two of the six columns say the value and one says nothing.
- 3.2 is not a coherent second model. It reports the opposite of what it
  does — `export -p` inside the function lists `declare -x FOO=""` for a
  name it then tells no child about — and it *does* hand a child a value
  by the other route: for a name that arrived in the environment rather
  than being exported by hand, `local TERM` shows the child `TERM=dumb`
  in 3.2 as in 5.3. Choosing 3.2's answer means choosing between two
  behaviors within 3.2 as well.
- 5.3's answer is one rule and reaches further than the shape that
  raised it. Two functions deep it is the *caller's* local that a child
  is told, not the global behind it; `local -x` over an exported name
  hands the value over and over an unexported one hands nothing; `+x`
  takes the attribute off the local outright and the child is still told
  the outer value. So it is the shadowed binding speaking rather than
  the local, and both bash builds agree on the last of those.

Written down as a rule: **an exported name a declaration shadows goes on
reaching a command with the value it had, for as long as the declaration
standing in front of it has none of its own.** What the shell reads and
what a child is told part company there, which is why only a real child
can measure it.

The rule reaches two more shapes, and both are modeled (#999). Both bash
builds agree on both, so neither is a version split, and neither needed a
new axis:

**A local that drops the attribute uncovers the binding behind it.**
`local +x FOO=z` over an exported `FOO` hands a child `bar` and not `z`.
`+x` is the *local's* attribute and says nothing about what it shadows,
so the shadowed binding is still exported, still holds `bar`, and is
still the one a child is told about — while the shell itself reads `z`.

    export FOO=bar; f() { local +x FOO=z; env | grep '^FOO='; }; f

    bash 5.3, bash 3.2, bash as sh   FOO=bar
    ksh93 (typeset, keyword func)    nothing
    zsh                              nothing

Three measurements say it is the shadowed binding speaking rather than a
value the local kept: with nothing exported behind it the same line tells
a child nothing; `export FOO` inside the function puts the attribute on
the local and the child is then told `z`; and two functions deep it is
the caller's local that is handed over. ksh93 and zsh tell a child
nothing because their local never inherits the attribute — this axis's
own `no`, not a second question.

It falls out of the rule above with one word changed: the declaration
standing in front has none of its **exported** value of its own, rather
than none of its own. A local with a value the child cannot be told about
is, from the environment's side, a local with nothing to say. Reading it
the other way left the name in the gap between two lists — the loop over
the tables passed it over because the local is not exported, and the list
of shadowed exports passed it over because the local has a value — and
the child was told nothing at all.

**A redeclaration in the same scope hides nothing.**
`local FOO` after a `local FOO=x` leaves the value standing: `x`, not
UNSET, in bash 5.3, bash 3.2, bash as `sh` and zsh, and `typeset FOO=x;
typeset FOO` in a keyword function reads `x` in all four shells that
spell the builtin, ksh93u+ included. So this is **core** and not this
axis at all — what a valueless declaration hides is an *outer* value, and
a scope that already declared the name has none in front of it. The value
the second declaration would hide is the local the first one made.

    f() { local FOO=x; local FOO; ... }      x       — same scope
    f() { local FOO=x; g; }; g() { local FOO; ... }  UNSET — new scope

The second line is what `ValuelessDeclarationHidesTheOuterValue` is
about, and the two must not be answered together. Asking the *scope*
whether the name was shadowed answered them identically, so a line that
only meant to name the variable again threw away what the function had
just computed — silently, at status 0. The question is whether **this
declaration** took the shadow. `typeset -g FOO` after a `local FOO=x` is
the same shape reached where no shadow is taken at all, and the same
answer.

One neighbor is still recorded rather than modeled: `unset` on such a
local reads as unset in every build and still hands a child the outer
value. That one we match, by the rule above rather than by a case of its
own.


### `unset`

**`BadSubscriptToUnsetFatal`** — bash yes · dash unspecified · ksh93 no · zsh no

Ends the script when an `unset` operand's subscript will not evaluate.
This is the one place a bad subscript does not behave the same way in all
four: everywhere else — reading an element, its length, an operator that
reaches one, an assignment through one, a substring's offset — the word
is abandoned and the script with it, unanimously.

    a=(x y z); unset "a[1+]"; echo "st=$? n=${#a[@]}"

    bash 5.3    1+: arithmetic syntax error: operand expected …  and stops
    ksh93       unset: 1+: more tokens expected                  st=1 n=3
    zsh         bad math expression: operand expected …          st=1 n=3

So bash gives up on the script as it does for any bad expression, and the
other two leave a *failed builtin* behind — which is the shape a script can
test, and the reason this is an axis rather than a wording.

It was silent in all three: the subscript's error came back and nothing
read it, so `unset a[b c]` was a no-op at status 0.

ksh93 also names the builtin in front of the sentence
(`Diagnostics.UnsetBadSubscript`), where it words the identical failure in
an expansion without one. That is a wording and not a second axis.

Asked only for an operand whose subscript actually failed. dash has no
subscript to evaluate — `UnsetTakesASubscript` is no there — so the axis
is absent rather than false.

**`UnsetSubscriptOnAScalarIsAnError`** — bash yes · dash absent · ksh93 no · zsh unreachable

Refuses `unset "a[1]"` where `a` holds a string, rather than leaving the
name alone without a word.

`unset` through a subscript on a name that is no array gets a different
answer from every column, and none of the three is a rule of its own —
they all fall out of what a subscripted name *means* there. Measured on
`a=hello`:

    probe            bash 5.3   bash 3.2   ksh93     zsh
    unset a[0]       unset      refused    unset     refused
    unset a[1]       refused    refused    silent    ello
    unset a[2]       refused    refused    silent    hllo
    unset a[-1]      refused    refused    silent    hell
    unset a[-5]      refused    refused    silent    ello
    unset a[-6]      refused    refused    silent    hello
    unset a[9]       refused    refused    silent    hello

**Where a subscript names a character**
(`ScalarSubscriptIsACharacter` yes) the string loses that character and
nothing else about the name changes. Past the end names none. Every
negative *within reach* acts — which an array under the same shell does
not do, where only `-1` does — so a string here is a character position
and not a one-element array wearing one. And a negative reaching back
past the first character is quiet, where the non-negative below the first
is `array/unsetting-below-the-first-element`'s refusal reached through a
string.

**Where it names an element**, a scalar is the one element at the base.
The subscript that names it takes the whole *name* away — the value, the
attribute and the environment entry with it, which is `unset a` written
the long way round. Every other subscript names nothing, and there the
two element-reading shells part: this axis.

Asked only there. An array with a gap takes the same subscript without a
word everywhere — `a=(x y z); unset "a[9]"` is a success in all four — so
what the refusing shell objects to is the *name* not being an array,
which is what its wording says: `unset: %s: not an array variable`, the
`Diagnostics.UnsetNotAnArray` the whole-array spelling already carries. A
name holding nothing at all has neither an element nor a character for
any subscript to name and is left alone under every reading, which is why
`unset "b[0]"` on an unset `b` is quiet in all four.

An empty string is asked too, and it is where the two readings are
furthest apart from one line: it has the one element a scalar is and no
character at all, so `a=; unset "a[1]"` leaves the name empty where the
base is 1 and takes the name away where it is 0.

The preset is no. POSIX has `unset` remove what it finds and say nothing
about what it does not — `unset nosuchname` succeeds everywhere — and the
quiet reading is that sentence read over a subscript.

zsh is *unreachable* rather than no: its subscript on a string names a
character, so it never gets here. dash is absent — `UnsetTakesASubscript`
is no there, and `unset "a[0]"` is a bad variable name.

bash 3.2 refuses the base subscript too, having read `${a[0]}` as the
whole string moments earlier, so
`array/unsetting-the-subscript-that-names-a-scalar` splits the two bash
columns on purpose. The graded column is 5.3's, for the reason
`LocalInheritsTheExportAttribute` gives at the same fork: the older build
is not a second coherent model of the shape, it is the same build
disagreeing with itself about what `a[0]` names.

**`UnsetArraySpan`** — bash removes every element · dash unspecified · ksh93 a subscript · zsh leaves one empty element

Is what `unset` does to the span of elements a subscript names — `a[@]`
and `a[*]`, which every column answers identically, and `a[3]`, which
names a span of one. Three answers, and the third is not a variation on
the other two, so it is a policy type (`UnsetArraySpanPolicy`) rather
than a switch:

    a=(p q r); unset "a[@]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"

    bash 5.3, bash 3.2   [] n=0
    ksh93                unset: @: arithmetic syntax error   [p][q][r] n=3
    zsh                  [] n=1

bash empties the array. ksh93 has no whole-array reading here at all: the
brackets hold an arithmetic expression as they do everywhere else, `@` is
not one, and the operand is reported as a bad subscript with the array
left standing. zsh replaces what the subscript names with a single empty
element, which is not a rule about `[@]` but this shell's reading of
`unset` on a *span* — `unset a[2]` leaves an empty element in place too,
and `unset a[1,3]` leaves exactly one.

Two consequences follow from that, and they are what tell the two
clearing shells apart:

    a=(p q); unset "a[@]"; a+=(z)    bash → [z] n=1     zsh → [][z] n=2
    a=hello; unset "a[@]"            bash → refused, 1  zsh → [] at 0

A count alone cannot separate an emptied array from one holding a single
empty string; where the next append lands can. And on a name that holds a
scalar the two readings say what they mean: bash has no elements to take
away and refuses at 1 with `unset: %s: not an array variable`
(`Diagnostics.UnsetNotAnArray`, identical in bash 3.2), while zsh treats
the scalar as the single span it is and empties it without a word.

The reading is the *indexed* array's alone. With the keyed attribute on,
`@` is a key like any other and nothing was stored under it, so
`typeset -A m; m[k]=v; m[j]=w; unset "m[@]"` leaves both elements in all
three shells that have the attribute — including the two that clear an
indexed array through the same spelling.

**A subscript written as a range** reaches this axis too, in the one
dialect that reads the comma that way (`SubscriptCommaIsARange`). What
the span *becomes* is the answer above; what a range adds is that a span
can be written down. Measured on zsh 5.9.2 with `a=(x y z)`:

    a[1,2]    [][z]        the span becomes one empty element
    a[1,3]    []           every element, and one is left
    a[0,1]    [][y][z]     a start below the first is the first
    a[3,4]    [x][y][]     an end past the last is the last
    a[4,5]    [x][y][z]    a start past the last names nothing
    a[2,1]    [x][][y][z]  a span with nothing in it is an empty
                           element *inserted* where it would have begun
    a[-1,-1]  [x][y][]     the last element
    a[-2,-1]  [x][y][z]    a negative start other than -1 acts on nothing
    a[0,0]    refused      the whole span is below the first element

Two of those would not have been guessed. The reversed range *inserts*,
which is the strongest evidence anywhere that this reading replaces a
span rather than removing subscripts — an empty span still becomes one
element. And a range's negative start takes the same "only `-1` acts"
rule this shell's single subscript already takes, so `[-2,-1]` leaves the
array whole where `[-1,-1]` blanks the last element.

Over a *string* the same span names characters, and the two halves part
exactly where the single subscript's do: every negative within reach acts
(`a=hello; unset "a[-2,-1]"` is `hel`), and a reversed range is invisible
because an empty character put where the span would have begun leaves the
string as it was.

**The below-the-first-element refusal is a rule about the span**, and
that is what tells `a[0]` from `a[0,1]`. A range that begins out of reach
and ends inside is not refused — the start is the first element — and one
that lies wholly out of reach is. A single subscript is that span with
one end, so `array/unsetting-below-the-first-element` and
`array/unsetting-a-range-below-the-first-element` are one rule seen at
two widths, and `interp.Runner.spanIsBelowTheFirstElement` is the one
place it is written. A negative end never counts as below: it is counted
back from the end and reaches nothing rather than reaching before the
start.

A pair whose ends are the same subscript is that subscript under either
reading, so `unset "a[2,2]"` asks the comma axis nothing and a runner
with no dialect still answers it — the same discipline `${a[2,2]}`
follows.

Two shapes are recorded rather than modeled. zsh reports a range whose
*end* will not evaluate and then acts as though it were 0 — `unset
"a[1,x+]"` on three elements complains and comes back with four — where
the same failure at the *start* is reported and nothing is done; we
report and do nothing in both, which is what the single subscript already
does. And no dialect reads a range while removing rather than blanking, so
the removing answer read over a span — take every element the span names
— is the two answers composed rather than a measured column.

A single subscript is the same axis at a span of one, which is why there
is one field and not two:

    a=(x y z); unset "a[3]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"

    bash 5.3, bash 3.2, ksh93   [x][y][z] n=3
    zsh                         [x][y][] n=3

The base decides which column is being asked. `3` is the last element
where the first is 1 and one past the end where the first is 0, so bash
and ksh93 hold their length because nothing was named and zsh holds its
length because the element it named was blanked rather than removed. Ask
bash and ksh93 for *their* last element and it goes:

    a=(x y z); unset "a[2]"     bash, ksh93 → [x][y] n=2
    a=(x y z); unset "a[2]"     zsh         → [x][][z] n=3

The end is the only place the two readings can be told apart. In the
middle of an array they cannot: a dense reader finds a removed subscript
empty on its own, so `unset a[2]` on `(p q r)` reads back as `[p][][r]`
under both. That is what let the wrong answer stand — the corpus had the
middle case and it passed — and it is why `array/removing-the-last-element`
exists.

Two end-relative wrinkles, measured rather than reasoned. Only `-1` acts
under the blanking reading: `unset a[-2]` on `(x y z)` leaves all three
where the removing shells take the middle one away. And bash 3.2 has no
negative subscripts at all, reporting `[-2]: bad array subscript` where
bash 5.3 removes.

dash has no arrays and answers `UnsetTakesASubscript` with no, so the
operand is a bad name there and the axis is never consulted. The POSIX
preset leaves it unspecified: the panel gives three answers to the
whole-array spelling and no reading of the standard picks one, so that
spelling is refused by name. A *single* subscript is not refused, because
the preset has already committed to removal there — see
`UnsetTakesASubscript`, where POSIX has `unset a[0]` name an element and
take it away.

The spelling used to do nothing at all. `@` is not an arithmetic
expression, so the subscript failed to evaluate and the element nobody
named was quietly not removed: the array came back whole at status 0, in
the shape a script writes to start a list over. It is a different question
from `EmptyArrayAtIsOneEmptyField`, which is about how many *fields* a
quoted `"${a[@]}"` of an empty array makes and is answered after this one
has already decided whether the array is empty.

**`UnsetEndsTheProducedPipelineStatus`** — bash no · dash unspecified · ksh93 unspecified · zsh yes

Makes `unset` permanent. zsh says yes and the name never fills again; in
bash the producer outlives it. It is the opposite of what a produced
*scalar* does, where unset ends it in both — `unset RANDOM` leaves an
ordinary empty name everywhere.

**`UnsetFunctionReportsMissing`** — bash no · dash no · ksh93 no · zsh yes

Complains when `unset -f` names a function that is not defined. True in
zsh alone, which reports it about any name it does not hold, well formed
or not.

Unsetting a function that *is* there is quiet in all four.

**`UnsetNameOperands`** — bash AnythingIsAName · dash PlainNamesOnly · ksh93 PlainNamesOnly · zsh NamesAndPositionals

Is that question for `unset`, and is a separate field because two
dialects answer it differently from the declarations. zsh answers the
two with *disjoint* sets — `export ?` is fine there and `unset ?` is
not, while `unset 12` is fine and `export 12` is not — and bash 5.3
checks a name for `export` and nothing at all for `unset`. One field
could not say either.

**`UnsetPositionalIsAllowed`** — bash no · dash no · ksh93 yes · zsh no

Lets `$1` expand to nothing under `set -u` rather than being an error.
ksh93 alone, and quiet where it differs: a script that reads an argument
it was not given carries on there and stops everywhere else.

**`UnsetTakesASubscript`** — bash yes · dash no · ksh93 yes · zsh yes

Is the same question asked of `unset`, where the answers are not the
same: bash, ksh93 and zsh take it and dash refuses it.


### expansion and tracing

**`BraceExpansion`** — bash yes · dash no · ksh93 yes · zsh yes

Expands `{a,b}` and `{1..3}`. Absent from dash, where the word is a
literal.

It lives here rather than in syntax.Dialect even though it is additive,
because the token stream is identical either way: the parser produces
the same word, and only expansion differs. It is also silent in the `&>`
sense — `echo {1..3}` prints something either way, and nothing reports
that one of them is not what was meant.

**`DollarZeroNamesTheInnermostCall`** — bash no · dash no · ksh93 no · zsh yes

Makes `$0` the innermost thing the shell has been called into rather than the
shell's own name: the function being run, or the file being sourced. One field
because no shell splits the two — zsh has both under a single option and
loses both when it is turned off, and the other four have neither.

Innermost, and measured: a function that sources a file reports the *file*
while that file runs and its own name again afterwards, and a function defined
in a sourced file reports its own name rather than the file it came from. The
file is named as the operand was written, so `. ./inc.sh` reports `./inc.sh`.

Startup files are outside it. They are read by the shell rather than sourced
by a script, and `$0` inside `~/.zshrc` is the path of the zsh binary.

The value `$0` had before any of this is `ZSH_ARGZERO`, which is a wording
rather than an axis: the zsh dialect's prelude sets it and no other dialect
has the name.

**`EqualsExpansion`** — bash no · dash no · ksh93 no · zsh yes

Replaces an unquoted word beginning with `=` by the path of the command
named after it: `echo =ls` prints /bin/ls. zsh alone, and silent in the
`&>` sense — the other three take the word literally and report nothing,
so the same script prints two different things and neither shell
complains.

Its failure is not silent: a name that resolves to nothing is fatal to
the script, like any other failed expansion.

**`LinenoCountsFromTheFunction`** — bash no · dash no · ksh93 no · zsh yes

Numbers `$LINENO` inside a function from the line the function was
written on: zsh; the other three count from the file.

**`TraceAssignmentsSeparately`** — bash yes · dash no · ksh93 yes · zsh no

Gives each assignment of `a=1 b=2` its own trace line. True in bash and
ksh93; dash and zsh put them on one.

**`TraceShowsItsOwnDisabling`** — bash yes · dash yes · ksh93 no · zsh yes

Prints `set +x` before acting on it. True in dash, bash and zsh; ksh93
applies the change first, so the command that stops tracing leaves no
trace of itself.

**`UnquotedListJoinsOnIFS`** — bash yes · dash no · ksh93 no · zsh no

Makes an unquoted list expansion one string first — the elements joined
on the *first character* of `IFS` — and splits that, rather than
splitting each element on its own. bash, bash 3.2 and bash as `sh` join;
zsh, ksh93 and dash do not.

The two readings agree under a whitespace `IFS`, which is what made the
difference easy to miss and what keeps a plain `for f in $@` from
demanding an answer. Under a non-whitespace one they part, and the
divergence has three faces that one answer produces and no rule about
*empty elements* can:

    IFS=:; set -- x "" y    → bash 3 [x][][y]   ksh93/dash 2 [x][y]
    IFS=:; set -- x y ""    → bash 2 [x][y]     ksh93 3 [x][y][]
    IFS=:; set -- "x:" y    → bash 3 [x][][y]   ksh93/dash 2 [x][y]

An empty element *between* others survives, because two separators meet
and the field between them is a field. One at the *end* does not, because
the join puts a trailing separator there and a trailing delimiter is
absorbed. And an element ending in a separator makes an empty field with
nothing empty anywhere. Every one of bash's answers is the scalar split
of the joined string — `x::y`, `x:y:`, `x::y` — which is what says it
joins rather than that it has a second rule.

zsh is what shows the join is real: with its splitting off it answers
`[x:][y]` for the third row, which no arrangement of the splitting answer
reaches, since a join would give one field there and it gives two.

The *quoted* spellings are not this question and must not reach it:
`"$@"` and `"${a[@]}"` are one field per element and `"$*"` and
`"${a[*]}"` are one joined field, in every shell measured — both core.

Asked only at the disagreement. It is not reached when the two readings
give the same fields, when there is nothing to join (one element) or
nothing to join *with* (`IFS` set and empty, where no shell joins:
`IFS=""; set -- x y` is two fields in all four), or when splitting is
off — with no split to undo the join the whole list would be one field,
which no shell does, so `SplitParamExpansion` stands in front of it.


### options, pipelines and status

**`AssignmentUpdatesPipelineStatus`** — bash yes · dash unspecified · ksh93 unspecified · zsh no

Counts a bare assignment as a command for the pipeline-status record.
bash says yes, so `false | true; x=1` replaces the two elements with one
holding 0; zsh says no and leaves them. Every other shape of command
updates it in both.

**`BadOptionToSpecialBuiltinFatal`** — bash no · dash yes · ksh93 yes · zsh no

Ends the script when a special builtin is given an option it does not
have. True in dash and ksh93, which is the POSIX rule that a special
builtin's failure is fatal; bash and zsh report it and carry on.

A different question from BuiltinSyntaxErrorFatal, which is about text
that would not *parse* and is true for dash alone. Measured across
`export`, `readonly` and `unset`.

**`MultiDigitDuplicationTargetIsAnError`** — bash no · dash **yes** · ksh93 no · zsh no

Refuses `>&10`: a duplication whose *target* is written with more than one
digit. dash alone; the other four read the number and fail at run time with
`10: Bad file descriptor` at status 1, carrying on.

**It is not a parse refusal, though the shell that has it words it as one.**
Measured three ways, and each one moves the question out of the grammar:

    sh -n -c 'echo hi >&10'                    accepted, exits 0
    printf 'echo one\necho hi >&10\n' | sh     prints `one`, then stops
    n=10; echo hi >&$n                         refused; n=9 is not

So the grammar takes the construct everywhere and the answer is the semantics
vector's, with the sentence in the dialect's `MultiDigitDuplicationTarget`.
The third line also fixes *where* the check goes: after the target expands.

The **width** is what is refused, not the value — `>&08` names descriptor 8, a
number everyone would otherwise take, and is refused just the same. That is
what keeps it apart from `FdNumberBoundedByOpenFileLimit`, which is about a
number too large for the process.

The refusal ends the script, and that travels with the answer rather than
being an axis of its own: one shell in the panel refuses and that shell stops.
The status is `FatalErrorStatusIsOne`'s, so 2 there.

**The companion question has the opposite dissenter**, which is why this is a
field of its own rather than the same one read from the other end. How many
digits may stand *before* the operator is the grammar's — bash alone reads
`exec 10>f` as a redirection where the other three run a command called `10`
(`Dialect.MultiDigitFdNumber`). One shell adds a width there; a different one
takes one away here.

**Reading the file is what settles the panel.** bash 3.2 prints `hi` for `echo
hi >&10` and reports success, which looks like a fifth answer and is not: that
build parks its own saved streams at descriptor 10, so something really is
open there and the write goes to bash's internal copy of standard output
without crossing any boundary. `cat <&10` in the same build is `Bad file
descriptor`, which is the tell.

One divergence is recorded rather than modeled: dash reports the line its
reader has *reached* rather than the redirection's own. With the redirection
on line 2 of three it says 3, and on the last line it says that line. It is
the same lookahead that makes the message read like a parse error while the
parse has already succeeded, and reproducing it would mean keeping a lexer
position that an evaluated tree does not have. Single-line programs — every
corpus case here — agree.

**`RedirectErrorOnSpecialBuiltinFatal`** — bash no · dash yes · ksh93 yes · zsh no

Ends a non-interactive shell when a redirection written on a *special*
builtin cannot be made. POSIX states it outright, and the reach is one
rule rather than several: `exec 3>/nope/x`, `: 3>/nope/x`,
`eval : 3>/nope/x` and `. /dev/null 3>/nope/x` stop wherever any of them
does, and so does a descriptor number the open-file limit refuses.

The failure is the redirection's, so the builtin never runs and the
status of the shell that stops is `FatalErrorStatusIsOne`'s — dash 2,
the rest 1 — which is why this axis carries no status of its own.

The boundary was measured from both sides and is unanimous. A regular
builtin (`true 3>/nope/x`), an external command, and a *compound*
command's own redirection (`{ echo x; } 3>/nope/x`) stop nothing in any
column. Inside a subshell it ends the subshell and the parent runs on;
inside a function it ends the shell.

**This axis is posix mode, and that is where the answer lives.** The
panel splits three to two — dash, ksh93 and bash-as-`sh` stop, bash and
zsh carry on — and the two disagreeing bash columns are the same binary,
so the difference cannot be attached to a shell. It is reachable at run
time in both shells that have such a mode, which is what turns an
accident of `argv[0]` into a rule:

| probe | answer |
| --- | --- |
| `bash -c 'exec 3>/nope/x; echo after'` | prints `after`, status 0 |
| `bash -c 'set -o posix; exec 3>/nope/x; echo after'` | stops, status 1 |
| `sh -c 'set +o posix; exec 3>/nope/x; echo after'` (bash as `sh`) | prints `after`, status 0 |
| `zsh -c 'emulate sh; exec 3>/nope/x; echo after'` | stops, status 1 |
| `zsh -c 'emulate zsh; exec 3>/nope/x; echo after'` | prints `after`, status 0 |
| `sh -c 'exec 3>/nope/x; echo after'` (zsh as `sh`) | stops, status 1 |

Both bash builds move, seventeen years apart, and so does a second
binary — so a dialect's field here is where the shell *starts* and its
own posix knob is what moves it. `set -o posix` is a real option in the
core's table for that reason, and `emulate sh` and `emulate ksh` carry
it in the zsh dialect beside the three axes they already moved.

Nothing is attached to `argv[0]`. Starting in posix mode because the
shell was called `sh` is a fact about an *invocation*, and the front end
does not read it yet; recording it as the axis would have written the
accident down and lost the rule.

**`BuiltinWriteErrorFailsTheCommand`** — bash yes · dash yes · ksh93 yes · zsh no

Makes a builtin whose output write failed — into a descriptor closed
with `>&-`, most plainly — report status 1. True in bash, dash and
ksh93; zsh keeps the builtin's own status and quietly loses the text.

Whether anything is *said* about it is the dialect's wording —
Diagnostics.BuiltinWriteError — not a second axis: bash and dash
complain, ksh93 fails silently, and zsh has nothing to word because it
does not fail. Asked only when a write has actually failed, so `echo hi`
on an open stream needs no dialect.

**`ErrexitSeesPipefailFailure`** — bash yes · dash unspecified · ksh93 no · zsh yes

Lets `set -e` stop for a failure that only pipefail produced — a
pipeline whose last element succeeded and whose earlier one did not.
True in bash and zsh; false in ksh93, which runs on.

Absent rather than false in dash, which has no pipefail, so the question
cannot arise there and is never asked.

Narrower than it looks: an ordinary failing pipeline — `true | false` —
stops all three, and this is only about the failure the option adds.

**`PipefailSubstitutesTheBareSignal`** — bash no · dash unspecified · ksh93 yes · zsh no

Reports an element `pipefail` chose over the pipeline's last one, and
which died of a signal, as the signal's **number** rather than as the
status a command killed by that signal reports.

It looks like a second reading of `SignalDeathStatusIsTwoFiftySix` and
is not. Measured over the whole family, ksh93 answers *that* one
consistently everywhere — with `/bin/bash` 3.2.57, `/opt/homebrew/bin/bash`
5.3.15, `/bin/ksh` 93u+ 2012-08-01, `/opt/homebrew/bin/zsh` 5.9.2 and
`/bin/dash`, a child raising signal *n*:

    context                                bash/zsh/dash   ksh93
    foreground command                     128+n           256+n
    inside ( … )                           128+n           256+n
    inside $( … )                          128+n           256+n
    `cmd & wait $!`                        128+n           256+n
    the shell itself, as its parent sees   128+n           256+n
    the LAST element of a pipeline         128+n           256+n
    an EARLIER element, through pipefail   128+n           **n**

Measured across HUP, INT, QUIT, ABRT, FPE, BUS, SEGV, PIPE, ALRM, TERM,
USR1 and XCPU for the first row, and with SIGPIPE and SIGTERM for the
last — 13 and 15 in ksh93 where the same deaths are 269 and 271 one row
up. So it is neither "does this shell add 128 at all" (it does add
something, consistently) nor a rule about SIGPIPE (SIGTERM behaves the
same way).

The last two rows are why this belongs to the *substitution*: an element
that fails in the position the pipeline reports anyway keeps the ordinary
encoding, and only the status pipefail went looking for is bare. Both
halves of that were measured in one script —

    { echo "$big"; } | true; a=$?      # substituted
    sleep 5 & p=$!; kill -TERM $p
    wait $p; b=$?                      # waited for

    bash 3.2 / bash 5.3 / zsh   a=141  b=143
    ksh93                       a=13   b=271

An ordinary non-zero exit is substituted unchanged in every shell with
the option — `(exit 42) | true` is 42 in all three — so a signal is the
whole of the difference. Builtin and external, first and middle, in
pipelines of two and of three, all give the same answer, so it is not
about which side of the process boundary the death happened on either.

Absent rather than false in dash, which has no pipefail. Asked only where
a substitution actually happened *and* was a signal death, so every other
pipeline runs in a core with no dialect.

Reaching this needed a status to remember what produced it, since 143 is
`exit 143` as readily as SIGTERM: `Runner.diedOfSig` carries the signal
beside the status, is cleared at the top of every command so it can only
describe the one the status describes, and travels out of a subshell with
the status it belongs to.

**`PipefailOption`** — bash yes · dash no · ksh93 yes · zsh yes

Is whether `set -o pipefail` exists, making a pipeline report its last
failing element rather than its last element. True in bash, ksh93 and
zsh; absent from dash and from POSIX, where a pipeline is defined to
report its last command and nothing offers to change it.

Not a wording difference: where it is absent the name is not an option
at all, so `set -o pipefail` fails and the pipeline goes on reporting
its last element — which is the answer a script guarding against a
failure upstream is specifically trying not to get.

## Suspend and resume: what ^Z, `fg` and `bg` say, and what a stopped job does to an exit

Measured through a pseudo-terminal on 2026-09-05 — bash 5.3.15, zsh
5.9.2, dash and ksh93u+ — because none of it exists anywhere else: a
shell reads its stopped-job state off a terminal it owns, and `-c` and a
script file cannot reach any of it. Each shell was driven in a session of
its own with a scratch `HOME`, the sequence `sleep 40`, ^Z, `jobs`, `fg`,
^Z, `bg`, `exit`, `exit`, waiting on the next prompt between steps.

**^Z prints a notice where the stop happened**, not before the next
prompt. `sleep 5; echo after` suspended with ^Z prints the notice and
*then* `after` in all four, so it belongs to the stop rather than to the
prompt — unlike the notice a job that *ended* gets, which every shell
holds back until the prompt.

Three of the four print the `jobs` listing's own row. zsh prints a
sentence that names itself and no job number at all:

    bash    [1]+  Stopped                    sleep 40
    dash    [1] + Suspended: 18              sleep 40
    ksh93   [1] + Stopped                    sleep 40
    zsh     zsh: suspended  sleep 40

`Diagnostics.JobStoppedNotice` is empty for the first three and zsh's
sentence for the fourth.

**Whether it starts on a line of its own splits two and two.** The
terminal echoed `^Z` where the cursor was and left it there. bash and zsh
write a newline first; dash and ksh93 write the row straight after the
echo, so the screen reads `^Z[1] + Stopped …`. That is
`Diagnostics.JobStoppedNoticeOnANewLine`, and it is the same debt the
prompt pays after a `^C`.

**`fg` names the command alone in three of the four**, and zsh prints a
listing row with a state that appears in no listing:

    bash, dash, ksh93   sleep 40
    zsh                 [1]  + continued  sleep 40

**`bg` differs in all four**, which is why it is a format rather than a
flag:

    bash    [1]+ sleep 40 &
    dash    [1] sleep 40
    ksh93   [1]<tab>sleep 40&
    zsh     [1]  + continued  sleep 40

**A stopped job holds the exit in bash and zsh and not in dash or
ksh93.** The two that hold say so and stay; the attempt has to be made a
second time, and the end of input behaves exactly as `exit` does:

    bash    There are stopped jobs.
    zsh     zsh: you have suspended jobs.

The status afterwards is the sharper half. bash's held `exit` reports 1 —
a builtin that failed, which `echo $?` on the next line shows — and zsh's
reports 0. A held ^D changes no status at all in either, so a session
ended by ^D ^D exits 146, the status the stopped `sleep` left behind.

**What suppresses the second warning is the thing immediately before
it**, and both shells agree:

    ^Z, exit, exit                    warns, then leaves
    ^Z, echo hi, exit                 warns
    ^Z, jobs, exit                    leaves at once
    ^Z, jobs, true, exit              warns
    ^Z, jobs, sleep 41, ^Z, exit      warns
    ^Z, jobs -p, exit                 leaves at once

So a `jobs` listing counts as the shell having shown you the same thing —
in any form, `jobs -p` included — and only for the very next line. A
sticky "has been warned" flag gets the first four rows right and the
fifth and sixth wrong, which is why `Runner` carries what the *previous*
chunk said rather than what has ever been said.

`Semantics.StoppedJobsHoldTheExit` is the axis; the POSIX base leaves,
since the standard describes `exit` as exiting and says nothing about a
job left behind, and bash and zsh override.

## Interrupting what the shell is running itself: ^C and ^Z against a compound command

Measured through a pseudo-terminal on 2026-09-05, bash 5.3.15 and zsh 5.9.2,
with a scratch `HOME` and each line waited on to its next prompt.

The shape that matters is a command the shell executes *itself* — a loop, an
`if`, a function body — rather than a process it started. `while :; do echo
tick; sleep 1; done` typed at a prompt is two different things at two
different moments: while `sleep` runs, the terminal belongs to the sleep's
process group and a ^C goes there; while the shell runs `echo` and the loop
around it, the terminal belongs to the shell and the ^C is a signal at the
shell itself. A shell that answers only the first goes round again either way.

**^C is unanimous and it gives up the whole line.**

    while :; do echo tick; sleep 1; done; echo AFTER    ^C → no AFTER, $? = 130
    while :; do echo tick; done                          ^C → stops, $? = 130
    if sleep 10; then echo YES; fi; echo AFTER-IF        ^C → neither, $? = 130
    f() { sleep 10; echo INFN; }; f; echo AFTER-FN       ^C → neither, $? = 130
    sleep 10; echo AFTER                                 ^C → no AFTER, $? = 130

Loops, conditionals, function bodies and the plain list all end, and so do the
commands written after them on the same line. That is exactly what
`controlAbandon` already meant for a refused assignment to a readonly name, so
it is the same mechanism rather than a second one.

**^Z is not the same rule, and here the two shells part company.**

bash breaks out of the loops the stop was inside and carries on with the line:

    while :; do echo tick; sleep 1; done; echo AFTER-LOOP   ^Z → AFTER-LOOP, $? = 0
    if sleep 10; then echo YES; fi; echo AFTER-IF           ^Z → AFTER-IF, no YES
    f() { sleep 10; echo INFN; }; f; echo AFTER-FN          ^Z → INFN and AFTER-FN
    sleep 10; echo AFTER                                    ^Z → AFTER, $? = 0

So a stop is not an interrupt: the only construct it ends is a loop, and the
`if` above skips its `then` because the stopped condition reported 146 rather
than because anything was abandoned.

zsh instead suspends **the construct itself** as a job. Every one of those
lines stops dead at 146 with nothing after it run, and `jobs` afterwards shows
an extra entry with an empty command beside the stopped `sleep` — the loop or
the function, suspended and resumable. That needs a fork of the shell's own
execution state, which this engine has no way to make: our subshells are
cloned Runners in one process. bash's rule is what is implemented, and zsh
gets it too; the alternative on offer was leaving the loop running, which is
the runaway this section exists to fix.

**A trap changes it and the two disagree again.** With `trap 'echo CAUGHT'
INT` set, bash's ^C on the loop prints nothing, ends the line and reports 130 —
the handler does not run at all — while zsh runs the handler and the loop goes
on ticking. Neither is reproduced here; the engine treats a trapped interrupt
as an untrapped one, which is bash's ending.

**Where the two halves live.** The interrupt that reached the *command* is read
off the wait: `runWatched` sees a child killed by SIGINT and gives up the line.
The interrupt that reached the *shell* can only be heard by the binary —
`interp` installs no signal handlers, because a library that did would be
taking them from the program around it — so `Runner.TakeInterrupt` is a hook
the front end fills in, asked at the top of every command. That is the only
place a loop of builtins can be stopped: nothing in `while :; do echo tick;
done` blocks, waits, or returns anywhere else.
