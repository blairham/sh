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
| a refused assignment prefix ends the script | **always** | never | **special builtin or function** | **a command it runs itself** |
| a refused assignment prefix costs the command | — | no | **yes** | **yes** |
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

It was then measured a second time and generalized. A readonly
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

## An assignment prefixed to a frozen name: one unanimous answer and three axes

`readonly x=1` then `x=2 cmd` is two questions: whether the refusal is
reported, which is unanimous, and what it costs, which is three axes.

**Reporting is unanimous.** Every column in the panel complains about the
refused name, on every kind of command, with the sole exception of ksh93
in front of a *regular builtin*, `command` naming one, or an alias that
resolves to one. Ours complained on the builtin path only — the one path
that stored the assignment and so the one that met the refusal — so a
prefix to an external command or to a function was accepted in silence,
status 0, and the external one handed the child the very value it had
refused. That half is implemented: the complaint is written wherever the
command is dispatched, and nothing else about the command moves.

**What the refusal costs is not one answer, and it splits by the kind of
command the prefix is attached to.** Measured 2026-09-07 and re-measured
2026-09-11, script file,
`env -i` with an isolated `HOME`, `ZDOTDIR`, `HISTFILE` and `ENV`; **R**
is a refusal reported, **run** that the command ran, **fatal** that the
script stopped:

| prefix to | bash 5.3 | bash-as-sh | bash 3.2 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| an external command | R, **run** | R, skipped | R, **run** | R, **fatal 2** | R, skipped | R, skipped |
| `command` + external | R, **run** | R, skipped | R, **run** | R, fatal 2 | R, skipped | R, skipped |
| `command` + builtin | R, **run** | R, skipped | R, **run** | R, fatal 2 | **no R**, **run** | R, skipped |
| a regular builtin | R, **run** | R, skipped | R, **run** | R, fatal 2 | **no R**, **run** | R, **fatal 1** |
| an alias for a builtin | R, not found | R, skipped | R, not found | R, fatal 2 | **no R**, **run** | R, **fatal 1** |
| a special builtin | R, **run** | R, **fatal 1** | R, **run** | R, fatal 2 | R, **fatal 1** | R, **fatal 1** |
| a function | R, **run** | R, skipped | R, **run** | R, fatal 2 | R, **fatal 1** | R, **fatal 1** |
| a name that is not found | R, then looks | R, skipped | R, then looks | R, fatal 2 | R, never looks | R, never looks |

Everything not marked fatal reaches the `echo after` on the line below
and reports 0. *skipped* is the command not running while the script
carries on, which the `/bin/echo RAN` spelling is what makes visible: the
row is about whether `RAN` was printed.

**The two lenient shells draw the line in opposite places.** ksh93 stops
the script on a special builtin or a function and carries on for
everything else, and says nothing at all where the word resolves to a
regular builtin — which the alias row is the probe for, since `al` is a
word of its own that resolves to `echo` only after alias expansion. zsh
stops on everything internal and carries on for an external command *and
for `command`*, whatever `command` names. So zsh reads the word that was
written and ksh93 reads the kind it resolves to, which is why one probe
cannot answer for both. bash and dash are uniform: bash 5.3 and bash 3.2
report, run the command and carry on at 0 in every row; dash is fatal 2
in every row.

**bash invoked as `sh` is not bash here**, which the three-bash rows
above are for: it reports and then gives up the rest of the list, so the
command does not run, and it is fatal on a special builtin. With `;`
separators it prints nothing after the refusal and with newlines it
reaches the next line, which is what identifies *the list* as what it
abandons.

**Held fixed, and measured rather than assumed:** the route. The same
square through `-c`, a script file and standard input agrees cell for
cell, with one exception that belongs to the fatality question rather
than to reporting — bash-as-sh answers 127 through `-c` where it answers
1 through a file and through standard input, on a special builtin. How
the name was frozen does not enter into it either: `readonly x`, with no
value, and `typeset -r x=1` refuse exactly as `readonly x=1` does in
every column that has the spelling.

**Four axes, because the table above needs four questions answered.**
Writing one axis from the bash rows would have given ksh93 the opposite
answer for five kinds of nine, which is how this was first filed.

- **`PrefixToARegularBuiltinIsRefused`** — whether it is a refusal at all
  in that one position. No in ksh93 and Yes everywhere else. Asked only
  in front of a regular builtin, which is the only position the panel
  parts at.
- **`PrefixRefusalFatality`** — never (bash), always (dash), on a special
  builtin or a function (ksh93), or on a command this shell runs itself
  (zsh). The last two are the kind-keyed values, and they read *different
  words* to find the kind: `command` is transparent to ksh93 and is not
  to zsh, which is what the `command` pair in the corpus is the probe
  for.
- **`PrefixRefusalCostsTheCommand`** — whether the command is left unrun
  at status 1 where the refusal is not fatal. Yes in ksh93 and zsh, No in
  bash. Unanswered in the standard's preset, where every refusal is fatal
  and the command's fate never arises.
- **`PrefixToAFrozenNameIsCheckedFirst`** — whether the check happens
  before the command's values are expanded and its redirections opened.
  Yes in bash alone. Measured 2026-09-12 with `readonly x=1`:
  `x=$((1/0)) /bin/echo RAN` says `x: readonly variable`, prints `RAN`
  and never mentions the division there, and reports the division and no
  `RAN` in dash, ksh93 and zsh; `x=2 /bin/echo RAN >/nope/f` names the
  name and then the file there, and only the file elsewhere. Two probes
  agreeing on one boundary is what makes it a boundary rather than a
  quirk of arithmetic, and the first says the value is not merely
  reported later but **never evaluated**. Read once a name in the prefix
  is actually frozen, which is where the other three are asked and for
  the same reason (#1943).

The kind is read once, at the dispatch, by resolving the command word the
way the dispatch itself resolves it — a function shadows a builtin and a
builtin shadows an external — so the axes cannot disagree with what runs.

**Two things follow from those and are not axes.** A shell that carries
on names *every* frozen name in the prefix, in written order; the shells
that name only the first are the ones that give the command up at the
first refusal, so the count follows from
`PrefixRefusalCostsTheCommand`. And a name that resolves to nothing is an
external command that fails to run, so the columns that skip the command
report 1 and never write `command not found`.

Corpus: the twenty `roprefix/` rows, one per command kind and one per
variable the construct turned out to depend on — including the control,
an ordinary prefix to a name nothing froze, which complains about nothing
in all six columns.

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

## How far a fatal error reaches: the file, or every file above it

Measured 2026-09-06. The previous section is about *whether* a failure is
fatal. This one is about what "fatal" costs, and it is a different
question with a different split.

The probe has to use a failure every shell in the panel words the same
way, or the row measures the operator instead of the abandonment. An
unset parameter under `set -u` does: `p.sh` fails on line 3 and would
print on line 4, and the file that sourced it prints afterwards.

    # p.sh                        # the program
    echo IN-BEFORE                . ./p.sh
    set -u                        echo "OUT-AFTER st=$?"
    echo X${NOPE}
    echo IN-AFTER

| | IN-AFTER | OUT-AFTER | `$?` at the `.` | shell |
| --- | --- | --- | --- | --- |
| dash | no | **no** | — | ends, 2 |
| bash 5.3 | no | **no** | — | ends, 1 |
| bash-as-sh | no | **no** | — | ends, 1 |
| bash 3.2 | no | **no** | — | ends, 1 |
| ksh93 | no | **yes** | **1** | runs on |
| zsh | no | **yes** | **126** | runs on |

Every shell stops at the failing line *inside* the file, which is not the
axis — that is the rule a failed expansion already follows. What splits
is how far the abandonment reaches: one file, or every file up the stack.
`FatalErrorEndsBorrowedTextOnly` is the field, and the status the
builtin reports is `Diagnostics.SourcedFatalStatus`, because neither
catching shell reports the status the error itself carried and they do not
agree with each other. ksh93's 1 is also not its sourced-*syntax* status,
which is 3.

**`eval` is the same boundary**, and one axis covers both for the reason
`BuiltinSyntaxErrorFatal` does: the four that end the shell for a sourced
file end it for evaluated text too, ksh93 and zsh catch both, and `exit`
is caught in neither. The *status* is not shared, which is why it is a
`Diagnostics` field the file route passes and `eval` does not:

| caught at | ksh93 | zsh |
| --- | --- | --- |
| `. p.sh` | 1 | **126** |
| `eval "…"` | 1 | **1** |

That is the same shape `SyntaxErrorStatus` and `SourcedSyntaxErrorStatus`
already have, measured the same way and for the same shell.

**A statement the shell merely *gives up* is a third thing again**, and it
does not end borrowed text in any shell. `readonly rr=1` then `rr=2`
inside an `eval` or a sourced file reports the refusal, gives up that
statement as far as the end of its line, and runs the line after it —
inside the borrowed text — in bash 5.3 and bash 3.2 alike. Where the same
refusal is *fatal*, ksh93 and zsh give up the text and report 1 and 126 as
above, which is what makes the pair worth measuring on one snippet.

**Four further measurements make it one axis rather than several.**

- **It is not about expansion.** A readonly reassignment where the dialect
  calls one fatal, `$((1/0))`, and `${x@ZZ}` all behave the same way at
  the boundary: caught in ksh93 and zsh, fatal above them elsewhere.

- **One file, not the stack.** A file sourced from a file sourced from the
  program loses the innermost file alone; the middle one runs the line
  after its own `.`.

- **The boundary is the running `.`, not where the text came from.** A
  function *defined* in a sourced file and called after the sourcing has
  finished ends the shell in every member of the panel, ksh93 and zsh
  included. And a `.` inside a function *is* a boundary: the function body
  resumes at the command after it.

- **`exit` and errexit are caught nowhere.** `exit 7` in a sourced file
  exits 7 in all six, and `set -e` firing there ends the shell in all six.
  That is what separates this from the neighboring rule that `exit` in a
  startup file ends the shell and the files after it are not read.

### The one operand that is not an error

`${x?word}` is the exception, and it is the exception in one shell:

| operand in `p.sh` | ksh93 | zsh |
| --- | --- | --- |
| `set -u` then `echo X${NOPE}` | caught, `$?` 1 | caught, `$?` 126 |
| `echo X${NOPE?msg}` | caught, `$?` 1 | **shell ends, 1** |

Two lines apart in the same file, one caught and one not. zsh's own manual
is why that reads as a rule rather than an inconsistency: the `?` form is
documented to print the word and *exit the shell*, which puts it in the
family of the `exit` builtin rather than the family of a diagnostic. So
`ParamErrorIsAnExitRequest` is a second field, asked only at a boundary —
at the top level of a script both operands end the shell everywhere, and
there is nothing there to ask. It splits the same way at an `eval` and at a
startup file.

### The same boundary at a startup file, and there the panel is unanimous

A startup file is a file the shell reads for itself rather than one a
script sourced, and it is a boundary in every shell that reads one with
nobody on the other end:

    # $BASH_ENV, or $ZDOTDIR/.zshenv          # then: sh script.sh
    echo RC-BEFORE                            echo MAIN-RAN
    set -u
    echo X${NOPE}
    echo RC-AFTER

bash and zsh both print `RC-BEFORE`, the diagnostic, and then `MAIN-RAN`:
the startup file stops at the failure and the program the shell was
started for still runs. zsh goes on to read `.zprofile`, `.zshrc` and
`.zlogin` as well. Replace the failure with `exit 3` and neither the later
startup files nor the program run, in either shell — which is the rule
`driver/startup.go` already models on `Exited`.

So there is no axis here, only the same distinction: `Runner.GiveUpTheFile`
is what a front end reading whole files of its own calls, and it catches an
error and never a request to stop. `${x?word}` splits at this boundary too,
exactly as it does at a `.` — the operand stops the file and lets the
program run in bash, and ends the shell before the program in zsh — which
is why that axis is read here as well.

**Why it is worth the trouble.** A real startup sources many files. Before
this, one bad expansion in one of them cost every line after the `source`
in the *outer* file, with no diagnostic saying so — a person loses half
their configuration and cannot tell. It also makes a startup diagnostic
count lie: a new failure early in a sourced file suppresses everything
after it, so the number of complaints *falls* while the shell gets worse.

### The third site: a prompt, and there the panel is unanimous *and* the axis is not asked

**A prompt is a boundary in the same sense a file is.** What a person typed
is one unit of input, and an error in it ends that unit and asks for the
next line:

    set -u
    echo X${NOPE}
    echo STILL-HERE

bash 5.3, zsh 5.9.2, ksh93u+ **and dash** all print the diagnostic and draw
the next prompt, with `STILL-HERE` printed. Not only `set -u`: the same is
true of `${x?word}` and `${x:?word}`, a division by zero, a substitution
that will not read, a readonly reassignment where the dialect calls one
fatal, an error inside a function or a loop typed at the prompt, and a
failed redirection on a special builtin — `exec 3>/nope/x` and
`: 3>/nope/x` alike, in POSIX mode as well as out of it. This shell ended
the session for every one of them, in all four dialects, so one mistyped
variable name under `set -u` closed the terminal (#1124).

`Runner.GiveUpTheLine` is the call, and it is a method of its own rather
than a flag because **the `${x?word}` axis is answered differently here**.
That operand is a request to stop at a file boundary in the two dialects
that document it that way, and it is an *error* at a prompt in all four:

| | in a script | at a prompt |
| --- | --- | --- |
| `echo X${NOPE?gone}` in bash, ksh93 | reports, carries on | draws the next prompt |
| `echo X${NOPE?gone}` in zsh, dash | **ends the shell** | draws the next prompt |

So one axis, three sites, and each site answers it for itself: `.` and
`eval` ask it, a startup file asks it, and a prompt does not.

**The status is not a field here**, which is the other way this site differs
from the `.` one. `.` overrides with `Diagnostics.SourcedFatalStatus`
because a caught error there reports 126 in one shell; at a prompt every
shell reports the status the error itself left — `$?` is 1 in bash, zsh and
ksh93 and 2 in dash, for the unset parameter and for the division by zero
alike — so there is nothing to override.

**`exit` is still not an error**, and this is the half that keeps the catch
from making a shell nobody can leave. Measured at a prompt in all four:
`exit`, `exit 7`, `eval 'exit 7'` and errexit firing (`set -e` then
`false`) each end the session.

## A construct a prompt can never finish

Two things a parser can say about a half-typed line get run together, and a
prompt is the one place the difference shows. `if true; then` is
**unfinished**: the next line continues it. `if; then` is **wrong**: the `if`
has been opened and not closed, so more input is possible, and the `;` after
the keyword is a token the grammar will never take, so no amount of it would
help.

Measured 2026-09-11, `printf 'echo one\nif; then\necho three\n'` into each
shell under `-i` with `PS1` and `PS2` set:

| | after `if; then` | and then |
| --- | --- | --- |
| bash 5.3.15 | refuses at once, no continuation prompt | runs `echo three` |
| ksh93u+ | the same | runs `echo three` |
| dash | the same | — |
| zsh 5.9.2 | draws `PS2` and waits | swallows `echo three` |

The second column is the cost and is why this is not cosmetic: a prompt that
asks for another line reads the command typed next as part of the construct it
has already refused, so that command never runs and nothing says so.

`Semantics.PromptAsksAgainAfterARefusedToken`, true for `zsh` alone, carried by
`driver` to `repl.Shell.AskAgainAfterARefusedToken`. It is read by the front
end rather than by the interpreter, which is what a prompt's question always
is, and it is a plain bool rather than an `Answer` because a prompt has no way
to refuse to run over an axis nobody answered.

**Which failures it is about** is the error's own distinction, which the parser
had all along: `syntax.ErrUnexpected` at the token, never
`syntax.ErrUnterminated` at the construct. `while; do` splits the panel the
same way `if; then` does. `for do` and `case in` split it neither way — every
shell prompts for both, because what is missing there is a word rather than a
word being in the way — and so do `echo one |`, `x='never closed`,
`cat <<EOT` and a trailing backslash.

The prompt asked only whether more input was possible, so this shell gave zsh's
answer to all four (#1893).

### And what it says when it refuses one

A prompt is a **route**, the way a script file and standard input are, and
`Diagnostics.ForPrompt()` is its answer beside `ForScript` and `ForStdin`.
Three of the four shells name no line at all there, where every one of them
names a line for the same failure in a file. Measured 2026-09-11,
`printf 'echo one\nif; then\necho three\n'` into each shell under `-i`, the
shell's own path normalized:

| | at a prompt | in a file |
| --- | --- | --- |
| bash 5.3.15 | ``bash: syntax error near unexpected token `;' `` | `s.sh: line 1: …` |
| ksh93u+ | ``ksh: syntax error: `;' unexpected`` | `s.sh: syntax error at line 1: …` |
| zsh 5.9.2 | ``zsh: parse error near `\n' `` | `s.sh:3: …` |
| dash | `dash: 2: Syntax error: ";" unexpected` | `s.sh: 1: …` |

Two things move, and the second is why a location is not enough.
`PromptLocation` is how bash and zsh say "no line here". ksh93 writes the line
**inside its sentence**, so the sentence is what has to change: the `Prompt…`
wordings, which that dialect builds in pairs with their script forms from one
call, since the rule relating them is exactly one clause.

**dash is the fourth and is different again.** It keeps its line and counts the
**session** rather than the construct — the second line typed is `2` — which is
state the front end would have to keep and is not modeled; see #2022.

**The echoed line stays absent.** `ParseDiagnostic` is handed no source at a
prompt, which is the right answer and not a gap: the line a person typed is
still on the screen above the complaint. An empty source used to split into one
empty line, so bash's second line came out as a bare `` `' `` (#1881).

**A remark reaches a prompt too.** A here-document delimited by the end of the
input is accepted and run by every shell in the panel, and bash warns about it —
at a prompt exactly as in a script. The prompt had no remark path at all, so the
body ran and nothing was said; `repl.Shell.Remark` is that path, and it is
reached only from the end of the input, which is the only place a prompt can
have one (#1892).

### And what a *run-time* failure says there

The same route, and the wider half: every diagnostic a session writes is
worded by the **runner's** `Diagnostics`, so a prompt that answered only for
parse failures still wrote a line number for everything else. Measured
2026-09-11, one typed line at a time into each shell under `-i`, the shell's
path normalized:

| typed | bash 5.3.15 | zsh 5.9.2 | ksh93u+ |
| --- | --- | --- | --- |
| `nosuchcmd` | `bash: nosuchcmd: command not found` | `zsh: command not found: nosuchcmd` | `ksh: nosuchcmd: not found` |
| `cd /nope` | `bash: cd: /nope: No such…` | `cd: no such file or directory: /nope` | `ksh: cd: /nope: [No such…]` |
| `echo ${u?boom}` | `bash: u: boom` | `zsh: u: boom` | `ksh: u: boom` |
| `echo $((1/0))` | `bash: 1/0: division by 0 …` | `zsh: division by zero` | `ksh: 1/0: divide by zero` |
| `echo hi > /nope/x` | `bash: /nope/x: No such…` | `zsh: no such file or directory: /nope/x` | `ksh: /nope/x: cannot create …` |

Not one of them names a line, and every line typed at a prompt is line 1, so
the number this shell wrote said nothing whatever it was. The same failures in
a file name a line in all three.

**A builtin's complaint is a second question.** zsh answers it differently
from its own messages *in the same session* — `cd: no such file or directory`,
the builtin's name with no shell and no line, against `zsh: command not found`
— which is what `PromptBuiltinLocation` is for. A prompt answer applied to
`Location` alone leaves half of a session wrong, and the half it leaves is the
half a person meets most.

**It is the route and not the complaint**, so everything a typed line runs is
located with it: a trap's own command, a command substitution, a function's
builtin, and text handed to `eval` — measured, `eval "cd /nope"` at a prompt is
`bash: cd: /nope: …` and `cd: no such file or directory: /nope`, exactly as the
line itself is.

**A sourced file is the exception, and it is a file however it was reached.**
zsh sourcing a file at a prompt keeps both the name and the line —
`f.sh:cd:1: no such file or directory` — where the same `cd` typed at that
prompt has neither. So the prompt's wording stops at the boundary of borrowed
text that came from a *file*, and does not stop at `eval`'s, which is not one.
`interp.Runner.AtPrompt` is what the front end sets and `Runner.borrowedFiles`
is where it stops applying (#2024).

**One difference left unfixed and stated.** bash sourcing a file at a prompt
names *itself* rather than the file — `bash: cd: /nope: …` where the same
`. ./f.sh` under `-c` is `./f.sh: line 1: cd: …`. This shell names the file on
both routes. That is a question about which name a sourced file's complaint
carries rather than about the line, it was true before this change and is true
after it, and it is not #2024's.

**One divergence recorded rather than reproduced, and it is dash's alone.**
The boundary here is "the shell is prompting", and dash's is narrower than
that: with `-i` reading from a **pipe** rather than a terminal, dash prints
the diagnostic and draws a further prompt and yet does *not* run the line
after it — the same `set -u; echo X${NOPE}` and the same
`echo $((1/0))` that it survives with a terminal. bash 5.3, bash 3.2, bash
invoked as `sh`, ksh93 and zsh all survive both routes, and a plain
`false` is survived by dash on both, so it is the abandonment that changes
and not the prompting. This shell has one rule for both routes and so is
dash's answer on a terminal and not its answer through a pipe; the case
that separates them is one nobody types on purpose (#1165).

**How it was measured, and why that is worth writing down.** Through a
pseudo-terminal, one keystroke at a time, waiting for the terminal to go
quiet between lines and reading a **marker the typed line cannot contain**
— `echo "MARK:$((6*7))"` in, `MARK:42` out. Two earlier attempts on this
same construct failed and both failures were the harness: bursting a whole
script into a pty measures the line discipline rather than the shell, and
waiting on text that appears in the line being typed is answered by the
terminal's own echo, so the check passes for a shell that ran nothing. A
count of prompt anchors is no good either once the shell under test has a
line editor: it redraws the prompt on every keystroke, so the count runs
ahead of the work. Covered by a row in `internal/smoke`, which drives a
real terminal, and by unit tests over the repl's own loop.

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

## One option, fifteen divergences

`set -x` produced more disagreement than any other single feature
measured, and almost all of it is decoration. The shells differ on the
prefix, on whether that prefix repeats at each level of indirection, on
whether an expanded field with a space in it is quoted, on which quoting an
embedded quote gets, on whether `a=1 b=2` is one line or two, on whether
`set +x` prints itself, on whether a `for` header is printed at all, on
whether an array literal has a space inside each parenthesis, on whether
that literal's elements are the words the script wrote or what they
expanded to, on what `case` prints, on how many lines one `[[ … ]]` is
worth, on whether a condition's operands are quoted, on whether `(( ))`
gains a space inside each parenthesis, on whether the parts of a
`for ((;;))` header keep those parentheses at all, and on what the `=~`
operator is *called* in the line that reports it.

**The structure is not what this document said it was.** It said "every
simple command to stderr, expanded, before it runs, and compound commands
not traced", and the second half is false: every shell that has `[[ … ]]`
traces it, every shell that has `(( … ))` traces that, bash and zsh both
print something for `case`, and the three parts of a `for ((;;))` header
are traced as arithmetic commands in their own right. What no shell traces
is a `while`, `until` or `if` header, a `( )` subshell or a `{ }` group —
which is a much narrower rule than "compound commands", and the narrower
rule is the one a reader of a trace needs.

The wrong sentence was not idle. We traced none of the four, and a gap in
a third-party xtrace log — gitstatus writes its own — was read as a
function returning before it reached its guards, when the guards had run
and simply were not printed. A trace that silently drops a whole command
*kind* cannot be read backwards, which is the only way anybody uses one.
`emulate -L sh` turning xtrace off for the rest of a function is a real
gap of the same shape, and its existence is what made the spurious kind
hard to spot rather than easy (#2126).

Thirteen are implemented. The rest are recorded and deliberately not:
ksh93 prints pipeline elements last-first — which follows from its running
the last one in the current shell — and ksh93 prints the *expanded*
elements and the *evaluated* subscript of an assignment where bash and zsh
print what was typed. That last one has a cost rather than only a
judgement behind it: expanding an element list to print it would expand it
twice, side effects and all, which is the double run `Runner.assignValue`
exists to prevent. The same cost decides the `[[ ]]` pattern operand,
which is printed from the string the matcher was handed: a metacharacter
that came from quoted text carries a backslash and a live one does not, so
`p='a*'; [[ abc == $p ]]` traces `a\*` in zsh and `a*` in bash — the same
word, two renderings, each faithful to what its shell was about to match.
ksh93 quotes the unescaped value there, and zsh renames `=~` to
`-regex-match` while ksh93 rewrites it as `== ~(E)…`; a table of three
names for one operator is decoration nothing else reads.

The others are visible only in a debugging aid, and reproducing them costs
more than the fidelity is worth — which is a judgement, and is written here
so it can be revisited rather than rediscovered.

What the trace *is* built from was itself a divergence hiding as a bug. It
was the name and one scalar value, so `a=(1 2)` traced `a=''`, `a[0]=z`
traced `a=z` and `x+=b` traced `x=b` — a construct reported as a different
construct, silently and at status 0. The target comes from the tree and the
value from the expansion, which is the split every shell that has these
constructs makes.

The count matters more than any one of them, and it has only gone up: an
option nobody would call contentious carries fifteen divergences, six of
them found by asking the question this section had already answered
wrongly. That is the strongest evidence yet for the claim this document
opens with — dialect is not a ladder, and the disagreements are not where
anyone expects them — and a second lesson beside it. The sentence that
hid them was a *summary*, and it was never measured; the nine divergences
under it were. What nothing tests is where a stale fact survives.

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


## What a shell with no child subshells reports for a pid

The section above establishes that nothing here forks for a subshell. This
is the consequence a *script* can see, and it is the one question a reader
of this repository asks next: when a script asks for the pid of something
that has no process, what comes back?

`zsh/system`'s `$sysparams[procsubstpid]` is the sharp case. It is the
process id of the most recent process substitution, and real zsh answers
**0** for "none started yet". A `<(cmd)` here runs on a goroutine of the
shell's own process, so there is no such process and there never will be —
it is a property of the shell rather than of the moment it is asked.

**Four answers were available and three of them are worse.** This is a
decided position, not an omission (#2125):

| answer | true? | safe? |
| --- | --- | --- |
| `0`, copying real zsh | no — nothing started | **no**: `kill -- -0` signals the shell's own process group |
| this process's pid | no | **no**: the same catastrophe spelled differently |
| refuse the key by name | yes | no: a refused expansion is fatal to the line, so it takes down the caller mid-way |
| **empty, and the key present** | **yes** | **yes** — the chosen answer |

Empty is the only one that is both. It is true because there is no pid, and
safe because the guard callers put in front of the dangerous line reads it
as nothing to signal. It also carries *more* information than `0` does: a
caller can tell "no process" from "process 0", which real zsh's spelling
cannot. `${+sysparams[procsubstpid]}` is `1` in both shells, so the key
exists; only the value deviates.

The third row is the one that cost something before it was understood.
Refusing by name ended `_p9k_worker_start` one line before it armed its
`zle -F` handler, and the theme's `always` block then tried to remove a
handler that was never installed — two diagnostics per prompt from one
cause, and the async worker dead in a shell that could have run it.

### The guard is doing the work, and not every caller has one

This is the limit of the position and it is worth stating plainly rather
than leaving for somebody to find. Empty is safe **because of what callers
write around it**, not because an empty string is inherently harmless:

| what the caller writes | ours | real zsh | then `kill -- -$pid` |
| --- | --- | --- | --- |
| `[[ -n $pid ]] && kill …` | skipped | runs with `0` | safe here |
| `${sysparams[procsubstpid]-none}` | `` (empty) | `0` | the `-` default is not reached — the key is set |
| `${sysparams[procsubstpid]:-none}` | `none` | `0` | — |
| `${sysparams[procsubstpid]:--1}` | **`-1`** | `0` | **`kill -- -1`** |

The last row is the real one: gitstatus writes exactly
`typeset -gi GITSTATUS_DAEMON_PID_$name="${sysparams[procsubstpid]:--1}"`.
It survives only because it *also* guards with `[[ $daemon_pid == <1-> ]]`
before `kill -- -$daemon_pid`. A program with the `:-` and without the
second guard would ask to signal every process it can reach.

**That hazard is not created by this deviation** — `kill -- -1` means the
same thing in every POSIX shell, and a caller that reaches it has written a
bug the shell cannot see. But it is the reason the `-` and `:-` rows are
distinct and both pinned: a test on `-` alone passes for a shell whose `:-`
is a loaded gun, and `-` is not the spelling the program in the wild uses.

Whether `kill` should refuse `-1` as a process group is a separate
question, with its own measurement and its own axis, because every panel
shell permits it.

### The same deviation, arrived at the same way

`$sysparams[pid]` is this process's at the top level and **empty inside a
subshell**. In zsh the key exists precisely because `$sysparams[pid]`
differs from `$$` there — a subshell is a fork, `$$` keeps the parent's
number, and a body reads the key to learn the one thing `$$` will not tell
it. Here a subshell is a cloned Runner in one process, so answering it made
the two agree — and a body that believes it has a process of its own
believes it leads a process group of its own. This machine's prompt theme
duly tore itself down with `kill -- -$sysparams[pid]` and killed the
interactive shell (#2046).

So the rule generalises past the one key: **where a real shell would name a
process this one does not have, the honest answer is no answer, and the
plausible answer is the dangerous one.**

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

A *status operand* — the word after `exit`, and the word after `return` —
is not read the same way twice:

                         dash   bash 5.3   ksh93   zsh
    exit -1 / return -1  error  255        255     -1
    exit abc             error  error      0       0
    return r    (r=3)    error  error      0       3
    return r+1  (r=2)    error  error      0       3
    return 3abc          error  error      3       math error
    return 300           300    44         44      300

dash takes digits and nothing else. bash takes a sign as well but refuses
text. ksh93 refuses nothing: it reads the number the word begins with and
ignores the rest, which is how `3abc` is 3 and `r` is 0 whatever `r`
holds. zsh refuses nothing either, but for the opposite reason — the
operand is an **arithmetic expression** there, so `return r` is the value
of `r` and `return r+1` is one more.

Four behaviors on a line rather than two sides, so `StatusArgument` is a
policy with four values — the shape `UnterminatedBracket` established,
used a second time without argument.

**One axis for two builtins.** Every row above was measured on `exit` and
on `return`, and the two never parted: zsh answers 3 for `exit r` exactly
as it does for `return r`. A second field for `return` would have been the
failure this tree has met seven times — a copy that omits what the
original learned — and here it nearly happened, with `exit r` sitting at 0
under zsh while `return r` was being taught arithmetic.

The eight-bit mask rides on the policy rather than being an axis of its
own, because each of the four either masks or does not and the four
answers line up one-to-one with the four readings. It is only ever visible
through `return`: a process carries eight bits whatever the shell decided,
so `exit 300` is 44 in all six however the operand was read, and only
`return 300` tells dash and zsh's 300 apart from bash and ksh93's 44.

The status those refusals carry is *not* the fatal-error axis. bash exits
1 for a fatal error and 2 for this, and dash exits 2 for both; a usage
error is its own thing, and unanimous where it happens at all. Whether a
refusal ends the *script* is the same question `BadOptionToSpecialBuiltinFatal`
already asks — dash ends it, plain bash carries on, and the same bash
called as `sh` ends it, which is the POSIX special-builtin rule rather
than a second reading of the operand. The two lenient shells cannot answer
it, because neither refuses any word.

zsh's `return r` was found in `~/.zi/bin/zi.zsh`, whose `.zi-ice` counts
the ice-mods it consumed into an integer and ends `return retval`. Reading
that as anything but arithmetic hands back the previous command's status
in place of the count, and the plugin manager shifts its own command line
by the wrong number of words.

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

### The same corollary through a registration rather than a table

A clone copies fields, and a field holding a *function* is copied with
whatever that function closed over. So a dialect seam registered as a
closure over the runner it was installed on keeps answering for that runner
from inside every subshell — the boundary is not reconstructed, it is
pointed straight through.

The option seams were the case that showed it. `[[ -o name ]]`, the `set -o`
listing and `set -o name` all reach a dialect that has an option namespace of
its own, and all three were installed as closures over the shell the front
end built. Measured on zsh 5.9.2:

| probe | zsh 5.9.2 | ours, before |
| --- | --- | --- |
| `(setopt correct; [[ -o correct ]])` | true | false |
| `(setopt shwordsplit; [[ -o shwordsplit ]])` | true | false |
| `(unsetopt multios; [[ -o multios ]])` | false | true |
| `(setopt nullglob; [[ -o nullglob ]])` | true | false |
| `(set -e; [[ -o errexit ]]; echo alive)` | `alive` | nothing |
| `(setopt autocd; set +o)` holds | `set -o autocd` | `set +o autocd` |
| `(set -o autocd)`, then the outer shell | without it | **with it** |

Every kind of option at once, which is what said the registration was the
cause rather than any one entry's storage. Two of the rows are worse than a
wrong report: under `set -e` a condition that answers false ends the shell it
is in, so the subshell died on the line that asked; and the mover wrote the
*parent's* option, so `set -o` inside a subshell both failed to apply where
it was asked and escaped to where it was not.

The rule that follows is the one a builtin already obeys: **a seam is handed
the runner it is answering about.** `SetOptionNamespace` and `SetOptionTable`
take `func(*Runner, …)`, and a dialect reads the runner it is given rather
than the one it registered against. Nothing needs to re-register on a clone,
which is the point — a clone that had to be told about each seam would lose
one every time a seam was added.

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
`RedirectsUseEveryTarget` axis, asked only when one stream is given
several targets — so no script that redirects the ordinary way pays for
zsh's feature.

**The same axis reaches the reading side**, which the paragraph above did not
say and the code did not do until #1779. A descriptor redirected twice for
*input* arrives as both sources in the order they were written:

    printf 'a\n' >f; printf 'b\n' >g
    cat <f <g           dash, bash, bash 3.2, ksh93 → b
                        zsh                         → a then b

One axis and not two, because zsh spells both with one option: `unsetopt
multios` takes the fan-out and the concatenation together, so two fields
would be two places to forget one of them when the option moves. A
here-document body joins the sources exactly as a file does — `cat <f <<A`
is the file and then the body, measured — and so does a here-string. The
name is about targets rather than about a direction for the same reason.

Where the shell that has it forks a process to tee or to concatenate, this
uses one stream built out of several: a fan-out is an `io.MultiWriter` and a
fan-in an `io.MultiReader`. The observable consequence is the same one zsh
produces — measured on zsh 5.9.2, a command reading one file sees a regular
file on its input and a command reading two sees a **pipe** — because a child
process is handed a pipe for any input stream that is not a file. The
difference the marker type records is on the writing side only, and it is
`exec`: a stream over several files has no descriptor number to hand to a
process replacement, where a concatenation does.

`multios` is this dialect's name for the axis and moves it — it was one of
the recorded names until #1779 — so `(unsetopt multios; …)` stays inside the
subshell the way every axis-backed option does.

**Not implemented, and measured rather than assumed:** the fan-in reaches
standard input and not a numbered descriptor. `exec 3<f 3<g; cat <&3` reads
both files in zsh and reads `g` alone here, because a number in the
descriptor table has to be a real file for a child to inherit — a
concatenation is not one, and giving the table a stream that is not a file is
a change to how every external command is started rather than to this
option. The writing side has the same gap for the same reason: `exec 4>a 4>b`
writes `b` alone. Both are the numbered-descriptor case, which no startup in
the wild sweep uses; standard input, standard output and standard error, which
they all use, are the fan-out and fan-in above. An earlier revision of this paragraph called it deliberately
unbuilt, and the paragraph outlived the decision: the failure mode this
file records about the interpreter's comments applies to its own.

It also demonstrates the blind spot recorded above, on a case chosen for
something else. The two answers differ in *output* and agree on the exit
status, so the behavioral score counts them as agreeing and every other
view calls it wording. A shell that writes to the wrong file is not a
wording difference.

## A here-document the input cut short

**`UnterminatedHeredocGainsATrailingNewline`** — bash **yes** · dash no ·
ksh93 no · zsh no

A here-document whose delimiter never arrived takes the body to the end
of the input, unanimously, and runs the command. Whether the body then
*ends in a newline* is not unanimous. Measured 2026-09-12 with `od -c`
on the raw output, from a script file and again through `-c`:

| shell | `cat <<X` / `body` writes | bytes |
| --- | --- | --- |
| bash 5.3, bash 3.2, bash-as-`sh` | `body` + newline | 5 |
| dash, ksh93, zsh | `body` | 4 |

It is reachable no other way: a here-document closed by its delimiter
always has a body ending in a newline, so this is the only shape in
which the question exists. That is also why the corpus cannot ask it —
`$( )` strips trailing newlines and the harness trims them, so
`heredoc/a-body-that-never-ended-its-last-line` records `body` in all
six columns and could never show the byte. The check is a Go test on the
runner's bytes.

The parser records the fact — `syntax.Redirect.HeredocAtEOF`, that the
body ran to the end of the input — and the interpreter answers the
question, because the two shells read the same text and hand the command
different bytes.

## Where a redirection is expanded, and what that costs

A redirection on a command the shell runs as a process of its own is set up
**in that process**, and two things a script can see follow from it. Both are
one axis per position, and the positions are two: a here-document's *body* is
`HeredocExpandsInTheCommandsProcess`, and a redirection's *target* is
`RedirectTargetExpandsInTheCommandsProcess`.

Measured over a script file with `env -i PATH=/usr/bin:/bin`, against bash
5.3.15, ksh93u+, zsh 5.9.2 and dash. For a target:

| | bash | ksh93 | zsh | dash |
| --- | --- | --- | --- | --- |
| `unset u; cat /dev/null > "${u:=made}"` — `u` after | unset | unset | unset | **made** |
| `set -u; cat /dev/null > "$NOPE"` — script after | alive, 1 | alive, 1 | alive, 1 | **stops, 2** |
| the same two on `: > …`, `exec 3> …`, a function, a group | kept, stops | kept, stops | kept, stops | kept, stops |

The third row is the line: everything the shell runs itself keeps the write
and dies of the failure in **every** column, because there is no other
process for either to land in. So the axis is asked only where the command is
one the shell runs as a process of its own, and only when the expansion
actually wrote something or failed — every other redirection is quiet and
unanimous, and an unanswered dialect must still be able to open a file.

One answer covers both consequences because they are one fact about where the
word was expanded. It is a **second** axis rather than a widening of the
body's, and that is measured rather than tidy: for a *body* dash is the
outlier one way — the write escapes — while its failure half still costs only
the command; for a *target* dash is the outlier the other way and ends the
script. One field cannot say both.

What no shell does, and what this got wrong, is open a target whose expansion
failed. The empty string the failure left behind was opened, so one mistake
produced two diagnostics and the second named a file nobody wrote. That much
is core, and it holds whoever runs the command (#1228).

## A command that is only redirections

A command with no command word, no assignment prefix and at least one
redirection means nothing in five of the six panel columns: the files are
opened — and truncated, where the operator truncates — nothing runs, nothing
is written and the status is 0. zsh instead treats the redirections as a
command's, and runs a command named by a **parameter**:

| written | zsh 5.9.2 | bash 5.3, bash 3.2, bash as `sh`, ksh93, dash |
| --- | --- | --- |
| `<f` | the file, through `$READNULLCMD` | nothing |
| `>g` | `g` truncated by `$NULLCMD` | `g` truncated |
| `print -r -- $NULLCMD $READNULLCMD` | `cat more` | two unset names |

Measured 2026-09-10 on zsh 5.9.2 from Homebrew. The defaults are why `<f` at
a prompt *pages* the file and `>g` truncates it with a `cat` that reads
nothing.

### The probe that separates the routes

Both defaults end up putting the file on standard output, so `<f` printing
the file is consistent with three different mechanisms and falsifies none of
them. Every measurement here instead points each parameter at a function that
prints its own name, and reads which name comes out. That is the same probe
that settled the `$(<file)` fork in #1747 — a hook the form does not consult
is a different mechanism — and it is what says these two parameters are two
routes rather than one:

    R(){ print -r -- R; }; N(){ print -r -- N; }
    READNULLCMD=R; NULLCMD=N

    <f              →  R          one plain input redirection
    3<f             →  R          the descriptor number does not matter
    0<f             →  R
    <f <g           →  N          a second redirection
    <f 2>e          →  N          anything beside it
    2>e             →  N          an output redirection
    <>f             →  N          a different operator
    <<<x , <<A      →  N          a body rather than a filename
    <&0             →  N          a duplication

So the reading parameter is asked for **exactly one** redirection that is a
plain `<`, and everything else goes to the writing one. The descriptor number
is where this test parts company with the one the `$(<file)` form applies to a
body that looks identical: that form is standard input alone, and `$(3<f)` is
not the form.

### The parameter is read when the command runs

Not word-split, not pattern-matched, and not resolved when the shell starts:
`READNULLCMD="print -r -- MULTI"` is `command not found: print -r -- MULTI`
and `NULLCMD="c*t"` is `command not found: c*t` — one word either way, and
the redirection has already happened when the lookup fails, so `>z` leaves
the file behind at status 127.

From there the command is an ordinary command. Measured: `set -x; <f` traces
`more`, `$_` afterwards holds `more`, the command's own status is the shell's
(`R(){ return 7; }` gives 7), a file that will not open stops it before it
runs, and `<f | cat` and `if <f; then` both reach it.

That is why the implementation substitutes the *word* rather than running
anything itself — see `interp/nullcommand.go`. Lookup, redirection, tracing,
`$_`, `set -e` and the sandbox gate are the ones a written command gets,
because it is the same path.

### Three shells, not two

An empty parameter is not the absence of the hook, and the difference is
observable:

| | `<f` | status | `>g` made |
| --- | --- | --- | --- |
| no hook (the core, and five columns) | nothing | 0 | yes |
| the hook, `NULLCMD=cat` | the file | 0 | yes |
| the hook, `NULLCMD=` or unset | `redirection with no command` | 1, **fatal** | no |

The third row abandons the script — `NULLCMD=; >g; echo after` prints neither
`after` nor a file, and the same line inside `( )` ends only the subshell — so
the refusal comes *before* the redirection rather than after it. An emptied
`READNULLCMD` does not refuse: it falls back to `NULLCMD`, so a script that
clears the reader gets the writer.

This is modeled as two named parameters on the vector,
`Semantics.NullCommandVariable` and `Semantics.ReadNullCommandVariable`, plus
a wording, `Diagnostics.RedirectionWithNoCommand`. Names rather than values,
because a script reassigns them between two commands; and the empty-name
refusal is what keeps "no hook" and "a hook with nothing in it" apart, which
a single boolean could not.

### zsh's two option names for it

`cshnullcmd` refuses the command outright and `shnullcmd` makes it `:`.
Measured in both orders, `cshnullcmd` wins while it is on, and turning it off
hands the shell back to `shnullcmd` if that one is still on — so they are one
question asked twice rather than two switches.

Both are implemented in the dialect by pointing the two fields at private
parameter names a script cannot write: one that is always empty, which is the
refusal the vector already has, and one that always holds `:`. No branch in
the core, and no third state on the axis. They left the recorded set in #1779
along with `multios`.

## The clobber-override marker, and the refusal it overrides

`>|` truncates a file that `set -C` would otherwise protect, and every
column in the panel reads it the same way. One column generalizes it: the
marker may be `|` **or** `!`, and it may follow any of the four write
operators rather than `>` alone. Measured 2026-09-07 against `/bin/dash`,
`/opt/homebrew/bin/bash` 5.3.15, that binary under an argv[0] of `sh`,
`/bin/bash` 3.2.57, `/bin/ksh` 93u+ and `/opt/homebrew/bin/zsh` 5.9.2:

| written | five columns | zsh 5.9.2 |
| --- | --- | --- |
| `>\|` | truncates the named file | the same |
| `>!` | **a file named `!`** | truncates the named file |
| `>>\|` | syntax error at the `\|` | appends, creating |
| `>>!` | **a file named `!`** | appends, creating |
| `&>\|` | syntax error, where `&>` exists | truncates |
| `&>!` | **a file named `!`** | truncates |
| `&>>\|` | syntax error, where `&>` exists | appends, creating |
| `&>>!` | **a file named `!`** | appends, creating |

It moves as one thing — no column takes some spellings and refuses others —
so it is one grammar flag, `ClobberOverrideMarker`, and not seven.

The bold cells are the reason it is a flag rather than something the lexer
could simply learn. **The two fallbacks are not the same kind of thing.** A
`\|` marker falls back to a pipe with nothing on its left, so the five
*refuse* the text and say so. A `!` marker falls back to an ordinary word,
so `echo hi >! f` writes a file whose name is the single character `!`,
holding `hi f`, and reports 0. One spelling, two meanings, no diagnostic:
the `&>` shape a third time, and accepting the union here would quietly pick
one dialect's reading for text that legitimately has the other.

That silence is what made it a bug for as long as it was one. The shell
under test read `>>!` exactly as the five do and was therefore *correct for
the core* while being wrong for the dialect that was asking — status 0, a
file created, and not the file that was named (#1247). It shows up in a
parse sweep only on a compound, where `done >>! f` leaves `f` as a word
after the loop and is refused; on a simple command nothing is refused and
nothing is said.

### Noclobber on an append is a separate answer

The append override has nothing to override unless `set -C` stops `>>` from
creating a file, and there the panel splits five to one:

    set -C; echo hi >> f        f absent

    dash, bash 5.3, bash-as-sh, bash 3.2, ksh93 → status 0, f created
    zsh                                         → status 1, f absent,
                                                   `no such file or directory: f`

Appending to a file that *does* exist is the control and is unanimous: all
six append and report 0. So the divergence is about **creating**, not about
appending.

POSIX 2.7.2 puts noclobber on `>` and says nothing about `>>`, which makes
this one of the few axes the standard answers outright: `PosixSemantics`
says No, five columns comply, and zsh is the departure. It is the
`NoclobberBlocksAppendCreate` axis, asked only under noclobber — `>>` is the
commonest redirection there is, and an unanswered axis refuses, so asking it
unconditionally would leave a bare `Semantics` unable to append at all.

The two halves are recorded together because neither is testable alone:
where the axis says No, `>>|` and `>>` do the same thing and nothing can
tell them apart.

### Noclobber protects a regular file, and only a regular file

The refusal is an exclusive create, and that is one rule wider than the
option: `O_CREAT|O_EXCL` fails for **anything** already at the name, so a
shell that stops there refuses `2>/dev/null` — the most written redirection
there is.

Measured 2026-09-10, `set -C; echo probe > TARGET`, across the whole panel
in a scratch directory:

| target | every column |
| --- | --- |
| `/dev/null`, `/dev/zero`, `/dev/stdout`, `/dev/fd/1` | written |
| a fifo with a reader | written |
| a symlink to `/dev/null` | written |
| an existing regular file, or a symlink to one | refused |
| a dangling symlink | refused |
| a directory, a unix socket | refused |

So the discriminator is the **type of the file already there** and not the
path: `>/dev/stdout` is refused when standard output is a regular file, and
a symlink is decided by what it reaches. A regular file is what the option
protects; anything else is opened, and whatever the open says stands.

The last three rows are refused by the open rather than by the option — a
directory, a socket and a dangling symlink cannot be opened for writing —
and that is where the panel splits, over the wording alone. bash, ksh93 and
dash report what the open said, `Is a directory`; zsh reports its own
refusal, `file exists`, for all of them. `NoclobberRefusalCoversAFailedOpen`
is that choice, and it is also what keeps `>/dev/tty` answering `file
exists` in a session with no controlling terminal, where the device passes
the type test and the open then fails with ENXIO.

The second open creates nothing — the file is demonstrably there — and one
more verb follows it: dash says `cannot open d` under the option and `cannot
create d` for the identical redirection with it off, where ksh93 says `cannot
create` both times. `NoclobberFallbackIsAnOpen` is that, and like the field
above it is a wording: the two shells do the same thing.

The order matters and is kept: the create is still the exclusive one, so two
shells racing for a new name cannot both believe they made it, and the type
is read only once EEXIST has come back and there is a file to ask about.

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

## A measured non-conflict, recorded so it is not over-generalized

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

### `\u` and `\U` in a format

Measured with `/opt/homebrew/bin/bash` 5.3.15 (as `bash` and as `sh`),
`/bin/bash` 3.2.57, `/bin/ksh` 93u+ 2012-08-01,
`/opt/homebrew/bin/zsh` 5.9.2 and `/bin/dash`, reading bytes with
`od -An -tx1` from a script file (macOS, 2026-09-05 and re-measured
2026-09-10). bash 3.2 and dash write every escape below as it stands, so
they are left out of the lines that follow; bash 5.3 behaves the same under
either argv[0], so its two columns are one here:

    printf 'a\u0041Z'       bash 61 41 5a   ksh93 61 41 5a   zsh 61 41 5a
    printf 'a\U00000041Z'    bash 61 41 5a   ksh93 61 41 5a   zsh 61 41 5a
    printf 'a\u41Z'         bash 61 41 5a   ksh93 61 41 5a   zsh 61 41 5a
    printf 'a\u00410'       bash 61 41 30   ksh93 61 41 30   zsh 61 41 30
    printf 'a\uZ'           bash 61 5c 75 5a and `printf: missing unicode
                                              digit for \u`, status 0
                             zsh 61 00 5a
                             ksh93 61
    printf '[%s]\uZ' x y    bash 5b 78 5d 5c 75 5a 5b 79 5d 5c 75 5a
                             zsh 5b 78 5d 00 5a 5b 79 5d 00 5a
                             ksh93 5b 78 5d 5b 79 5d

`PrintfUnicodeEscape` is four readings, arrived at from the same three
questions `PrintfHexEscape` asks and splitting in different places:

- **Whether the escape is there.** Three of the six have it and three do
  not, which is a *smaller* set than the five that have `\x`: bash 3.2 and
  dash write `a\u0041Z` as its ten characters where they read `\x41` as an
  `A`, so the two escapes are not one question. It is the **version** that
  decides and not argv[0] — bash 5.3 has the escape as `bash` and as `sh`
  alike, so the corpus's `bash` and `bash-as-sh` columns agree and only
  `bash32` differs. Conflating `bash-as-sh` with a bash 3.2 run under that
  name is how #909's own table came to record the column as not having the
  escape — the mistake `oracle.md` warns about, caught here by the golden
  record rather than by review.
- **How wide the digit run is.** This is the question `\x` splits on and
  this one does not: all three that have the escape take up to four digits
  after `\u` and up to eight after `\U`, accept fewer, and end the run at
  the first character that is not a hexadecimal digit. The letter decides
  the width and nothing else — `\u41Z` is `aAZ` and `\u00410` is an `A`
  followed by a zero, in all three.
- **What an empty digit run means.** The three part three ways, and one of
  the three is a reading no `\x` anywhere in the panel has. bash leaves the
  escape standing and writes `printf: missing unicode digit for \u` on
  standard error with a status that is still 0 — the warning-rather-than-
  failure shape its `\x` has. zsh reads the empty run as a zero and writes a
  NUL. **ksh93 drops the rest of that pass over the format.**

That last reading is why this is its own enumeration rather than
`PrintfHexEscapePolicy` under a second name, and the detail that makes it
an axis worth stating carefully is **what** is dropped. It is the pass and
not the builtin: the loop over the operands runs again, so
`printf '[%s]\uZ' x y` is `[x][y]` in ksh93, where a `\c` that *stops* —
zsh's `PrintfBackslashCStops` — writes `[x]` and ends. One operand cannot
tell the two apart, which is why the corpus row carries two. A pass that
is truncated before it reaches a conversion consumes nothing, and `printf`
ends rather than looping forever: `printf '\uZ[%s]' x y` writes nothing
at all there.

The value is a **code point written in UTF-8** and not a byte, and it is
the *original* UTF-8 rather than the range Unicode later kept: a surrogate
and a value past U+10FFFF are encoded rather than refused — `\ud800` is
`ed a0 80` and `\U00110000` is `f4 90 80 80`, with the five- and six-byte
forms reachable. bash 5.3 and zsh agree on every one. One encoder says all
of it, `interp.EncodeCodePoint`, which `echo`'s `\u` and `print`'s already
use; a second copy is how `print` came to write a replacement character
where the shell it follows writes the encoding (#1840).

**Above ASCII the panel is answering the locale and not the escape**, so
these three rows keep to ASCII: the reading is what they are about, and
ASCII is representable in every encoding, so it is the same in every
locale. Under `LC_ALL=C` — which is what the corpus harness runs in —
bash writes `\u00E9` back with its digits upper-cased, zsh truncates its
output at the escape, and ksh93 writes the UTF-8 regardless; under
`en_US.UTF-8` all three write `c3 a9`.

That is a separate question and it now has a separate axis,
`Semantics.UnicodeEscapeOutsideTheLocale`, with a corpus row of its own
above ASCII — see *A code point the locale has no room for* below. At the
`printf` sites it is **not** answered yet: ksh93 is locale-blind there and
at `$'…'`, which makes the axis three-valued where `echo` needs only two,
and the three-way measurement is recorded in #1851's successor rather
than guessed at here.

The escape is a question about the **format**. A `%b` argument asks the
same four readings at its own site, through `PrintfBUnicodeEscape`, and
ksh93 answers the two differently once again: it reads `\u0041` in a
format and leaves the ten characters as written in
`printf '%b' 'a\u0041Z'`. bash 5.3 and zsh have the escape at both sites
and the other three at neither, so the two axes are exactly the shape `\x`
established. No dialect in the panel answers the `%b` site with the
truncating reading; where it is set there, the *argument's* text ends at
the escape and the builtin runs on.

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
- **`\u` and `\U`.** `PrintfBUnicodeEscape`, the section above.
  bash 5.3 and zsh read them here, and the other four write the characters
  as they stand — ksh93 among them, which has the escape in a format, so
  this site groups the panel differently from that one.
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
the vector rather than a core answer with a dialect apologizing for it.

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

### The listing says where the declaration would land

Measured 2026-09-12, `env -i` with a scratch `HOME`, zsh 5.9.2 `-f`,
bash 5.3.15 and 3.2.57 `--norc --noprofile`, ksh93u+ — the same states
listed from inside a one-line function and again at the top level.

zsh is the one column that answers differently in the two places, and it
has to: inside a function a bare `typeset` declares a **local**, so the
text that recreates a global at the top level shadows it here. It writes
a `-g` word ahead of the flag cluster, and it spells a *local* export
with the command word `local` rather than `export`:

    state                     inside a function          at the top level
    g=1                       typeset -g g=1             typeset g=1
    typeset -a g=(a b)        typeset -g -a g=( a b )    typeset -a g=( a b )
    typeset -r r=1            typeset -g -r r=1          typeset -r r=1
    typeset -i10 n=5          typeset -g -i10 n=5        typeset -i10 n=5
    typeset -T T t=(a b)      typeset -g -T T t=( a b )  typeset -T T t=( a b )
    export q=2                export q=2                 export q=2
    typeset -xa A=(1 2)       typeset -g -ax A=( 1 2 )   typeset -ax A=( 1 2 )
    local l=2                 typeset l=2                —
    local -x e=9              local -x e=9               —
    local -xa a=(1 2)         local -ax a=( 1 2 )        —
    local -xr r=1             local -rx r=1              —

Three readings follow. The letter goes only beside `typeset`, because the
other two words already say where they land. `local` keeps the export
letter where `export` drops it — the word `export` *is* that letter and
the word `local` is not. And an exported compound never earned the
`export` spelling in the first place, so it stays on the `typeset` row
and takes the `-g` with it.

"Local" means local to the **innermost** scope, not to any scope on the
stack: a caller's local read from a called function lists as a global,
because a declaration written there would shadow it rather than reach it.
Measured — `g(){ typeset -p L }; f(){ local L=1; g }; f` writes
`typeset -g L=1`.

bash and ksh93 write one text for a local and a global alike, so neither
has anything to disagree with here; this is part of zsh's listing shape
rather than an axis.

### `export -p` and `readonly -p` are not always the command word

Measured the same day, over `typeset -ix n5=5`, `typeset -ax A5=(1 2);
export A5`, an exported read-only, and a based integer:

    state                     ksh93 / dash              zsh
    typeset -ix n5=5          export n5=5               export -i n5=5
    exported array            export A5=(1 2)           typeset -ax A5=( 1 2 )
    exported readonly rr=4    export rr=4               export -r rr=4
    typeset -i16 h=255        export h=16#ff            export -i16 h=255

So ksh93 and dash repeat the builtin's own word and write no attribute
letters at all, while zsh's `export -p` is its full declaration listing
narrowed to the exported names — letters and all, and picking `typeset`
where `export` cannot carry the value. The letterless form still writes a
**compound** value and a based integer's own text: `export g=([3]=x)`,
`readonly A=(1 2)`, `export h=16#ff`.

### A lone `+` given to `export` or `readonly`

Measured the same day. A sign with no letters after it is an option word
to zsh's `export` and `readonly` as it is to its `typeset`, and the
listing it reaches is the builtin's own attribute with the values left
off — `export +` writes the exported names and `readonly +` the frozen
ones, one a line, at 0.

Every other column reads it as a *name*, and refuses it as one: dash
`export: +: bad variable name` at 2, bash 5.3 and 3.2 ``export: `+': not
a valid identifier`` at 1, ksh93 `export: +: is not an identifier` at 1
and `readonly: +: invalid variable name`.

ksh93 is why this is a question of its own rather than the one `typeset`
asks: **its `typeset +` lists** while its `export +` refuses, so a shell
reading one answer for both builtins is wrong about one of them.

A lone `-` needs no second question. It is already the "is a lone dash an
option" reading, and once it is eaten `export -` is the bare listing —
which is what zsh writes for it.

With an *operand* alongside, zsh's `export + q` is a silent 0 that
neither lists the name nor takes the attribute off it. That corner is
deliberately not followed: an operand here keeps the refusal every other
column gives it.

## A declaration letter with no names is a filtered listing

Measured 2026-09-10, macOS arm64: bash 5.3.15 (`--norc --noprofile -c`,
also as `sh` and as the 3.2.57 build), ksh93u+ 2012-08-01, zsh 5.9.2
(`-f`), all with a scrubbed environment, over one table:

    qa=1; export qb=2; typeset -i qc=3; typeset -a qd=(p)
    typeset -ai qe=(4); typeset -xi qf=6; typeset -r qg=7

`typeset -x` with no operands is not a declaration of nothing: it is the
**filtered listing**, the names carrying the attribute with their values.
The plus sign of the same letter is the same selection with the values
left off — `typeset +x` writes `qb`, `typeset -x` writes `qb=2` — so the
sign picks between two listings rather than between a request and its
undo.

**The row is the bare `export` row and not the `-p` row.** bash writes its
own `-p` text, ksh93 and zsh drop the command word:

    line          bash                          ksh93          zsh
    typeset -x    declare -x qb="2"             qb=2           qb=2
    typeset -i    declare -i qc="3"             qc=3           qc=3
    typeset -a    declare -a qd=([0]="p")       qd=(p)         qd=( p )

which is `Semantics.BareDeclarationListing` exactly — the same field the
bare `export` and `readonly` already answer, since where the bare form
differs from `-p` it differs for both builtins and for this listing too.
A compound keeps each shell's own `-p` spelling of the value: bash's
subscripts and double quotes, ksh93's `(p q)`, zsh's padded `( p q )`.

A name that is **typed and holds nothing** is a row: `local -a qz` then
`typeset -a` writes `declare -a qz` in bash — `declare -a qz='()'` in the
3.2 build — and `qz=(  )` in zsh, which declares such a name empty rather
than leaving it unset. ksh93 has no `local` to ask with.

**Two letters together is the disagreement, and there are three answers.**
One letter reads the same everywhere, so a single-letter row is no
evidence at all about the combination:

    line          bash                  ksh93                zsh
    typeset -xi   qb and qc             qf alone             qb, qc and qf
    typeset -ir   qc, qe, qf and qg     nothing              qc, qf and qg
    typeset -ax   nothing               invalid variable     qb, qd and qf
    typeset -ai   qe alone              invalid variable     the integers
    typeset -aA   nothing               invalid variable     the tables

- **zsh joins** every letter, kind letters included: a name carrying any
  one of the attributes is in.
- **ksh93 intersects** every letter: a name must carry all of them, and
  its kind letters never reach the question — `typeset -ax` is refused
  outright as an invalid variable name.
- **bash** is neither: the *kind* letters `-a` and `-A` narrow and the
  rest join. A name must be every kind that was written and carry any one
  of the remaining attributes, so `-ai` is the array that is also an
  integer while `-ir` is every integer and every read-only name. Letter
  order decides nothing: `-xa` and `-ax` agree.

That is `Semantics.DeclarationListingFilter`, asked only where two letters
were written. zsh's own kind letters are mutually exclusive — it refuses
`typeset -ai x=(1 2)` as an inconsistent type — and two of them together
select one rather than both, by a precedence of its own (`-ai` and `-ia`
alike write the integers). That is a fact about a type system this tree
does not share, where an array *can* be an integer array, and it is
deliberately not modeled.

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

**Except for two constructs in one shell.** Measured 2026-09-11, after a
`false | true` so the replacement is visible:

|  | bash 5.3.15 | bash 3.2.57 | zsh 5.9.2 |
| --- | --- | --- | --- |
| `! [[ a = a ]]` | **1** | **1** | 0 |
| `! [[ a = b ]]` | **0** | **0** | 1 |
| `! (( 1 ))` | **1** | **1** | 0 |
| `! false` | 1 | 1 | 1 |
| `! true` | 0 | 0 | 0 |
| `! x=1` | 0 | 0 | 0 |
| `! { [[ a = a ]]; }` | 0 | 0 | — |
| `! ( [[ a = a ]] )` | 0 | 0 | — |

bash writes the record for `[[ … ]]` and `(( … ))` **after** the negation
and for everything else before it. The first two rows are the axis and need
both: a probe using only a matching test records 0 under one answer and 1
under the other, and a failing one records them the other way round, so
either alone passes for a shell that always writes the same number. Rows 4
to 6 are what confine it to those two constructs — an ordinary command and
an assignment record what *they* reported in both shells — and rows 7 and 8
say it is about the construct rather than about the shape of the line: a
compound holding the same test records its own status, with the `!` not
reaching the record.

A redirection does not move it. `! [[ a = a ]] >/dev/null` is 1 in bash, the
same as without one, where a redirection *does* decide whether the construct
counts as a command at all — so `NegatedTestRecordsThePostNegationStatus` is
asked separately from the axis below even though the two name the same pair
of constructs.

Silent when it is wrong: a plausible one-element record, no diagnostic, and
a script branching on `${PIPESTATUS[0]}` after a negated test reads the
opposite of what the shell it was written for reports (#1513).

**And whether a compound writes it at all is decided by its body's *parse*
in one shell.** Measured 2026-09-11, zsh 5.9.2, each line after a
`false | true`:

| | zsh 5.9.2 |
| --- | --- |
| `if [[ a = b ]]; then :; fi` | `0` |
| `if [[ a = b ]]; then [[ b = b ]]; fi` | `1 0` |
| `{ :; }` | `0` |
| `{ [[ a = a ]]; }` | `1 0` |
| `while false; do [[ a = a ]]; done` | `0` |
| `while [[ a = b ]]; do [[ a = a ]]; done` | `1 0` |
| `( [[ a = a ]] )` | `0` |
| `{ f() { :; }; }` | `1 0` |

The first pair is the whole of it. Neither body runs — the condition is
false both times — and the only difference is the text inside `then`, so an
**unexecuted** `:` is enough to make the compound count as a command. The
rule composes: a compound counts where its body holds anything that would
count standing alone, by the two axes below, and the `while` pair says a
condition is body as well. Four shapes answer for themselves whatever they
hold, because each is a job: a subshell, a coprocess, anything backgrounded,
and a timed pipeline. A function *definition* is the opposite — it runs
nothing, and both shells that keep a record leave it alone for one, so that
is a rule rather than an axis. A redirection on the compound writes the
record whatever the body says, exactly as it does for the two constructs
above.

**And bash's rule is a second mechanism rather than the other answer to that
question.** A compound writes nothing at all there; what stands after one is
whatever the last pipeline that actually *ran* inside it wrote, so a compound
that ran nothing leaves the record from before it. Measured 2026-09-11 on
bash 5.3.15 and 3.2.57 alike, each line after a `false | true`:

| | bash |
| --- | --- |
| `if false; then :; fi` | `1` |
| `if [[ a = b ]]; then :; fi` | `1` |
| `while false; do :; done` | `1` |
| `case a in b) :;; esac` | `1 0` |
| `for i in ; do :; done` | `1 0` |
| `{ [[ a = a ]] & }` | `1 0` |
| `{ [[ a = a ]] \| [[ b = b ]]; }` | `0 0` |
| `{ :; }` | `0` |
| `if false; then :; fi >/dev/null` | `1` |

The `1` rows are the condition's status: `false` ran and recorded it, and the
`if` wrote nothing over it. The `1 0` rows are the record from the pipeline
*before* the compound, untouched because nothing inside ran — which no
reading of the parse can produce, since it is a fact about a run rather than
about a body. The two-element row is what says it is the inner **pipeline**
rather than the inner last command. And a redirection on the compound does
**not** change it, which is the opposite of the rule the neighboring axes
follow and the one place the two mechanisms disagree about a rule they both
state.

A subshell is not a compound for this purpose under either of them: it is a
job and reports its own status, `( false | true | false )` leaving a
one-element `1`.

`Semantics.CompoundPipelineStatusRecord`, whose two answers are those two
mechanisms — the body's parse for `zsh`, what ran for `bash`. The third
reading, a compound writing its own status for having run whatever its body
holds and whatever ran inside it, is what this shell used to do and what no
panel member does; it is ruled out by being unnamed. ksh93 and dash have no
name for the record, so the axis is never reached there.

Silent when it is wrong, both ways: a plausible one-element record where the
pipeline's elements should still be there, which is the shape
`pipestatus/reading-it-twice-in-one-chain` exists to protect (#1931, #2016).

Two more things about it are axes:

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

Modeled as `CloseBraceAlwaysReserved`, a grammar flag, it gets both halves.
Modeled as "brace groups may omit their terminator" it would have got the
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

## `\u` and `\U` in an `echo` argument

Two shells read them and four do not. Measured 2026-09-10 with the bytes
read back through `od`:

    echo 'a\u0041Z'      zsh  aAZ        the rest  a\u0041Z
    echo -e 'a\u0041Z'   bash 5.3  aAZ   under either argv[0]
                                          bash 3.2, dash, ksh93  a\u0041Z

The two letters never split the panel, so they are one axis —
`Semantics.EchoExpandsUnicodeEscapes` — where `\e` and `\E` had to be two.
At most four hex digits after `\u` and eight after `\U`, fewer accepted, and
the value is a **code point written in UTF-8** rather than a byte: `\u00e9`
is two bytes and `\u20ac` is three.

It is the *original* UTF-8 and not the range it was later narrowed to, which
is measured rather than assumed. A surrogate and a value past the last code
point are encoded rather than replaced — `\ud800` is `ed a0 80` and
`\U110000` is `f4 90 80 80` — and the five- and six-byte forms are reachable,
`\U200000` being five bytes and `\U4000000` six. Both shells agree on every
one of those. `interp.EncodeCodePoint` is the one encoder, exported because
`print` reads the same escapes and must not grow a second.

### A code point the locale has no room for

Above ASCII the two shells answer the **locale** rather than themselves,
and so does this one now. The reading is settled above; this is the
question after it, and it is two questions rather than one.

**Whether the locale has room** is state read off the runner's own
variables, the way `PATH` and `IFS` are — not a second axis. `LC_ALL` over
`LC_CTYPE` over `LANG`, the reader `${#s}` already uses, and a plain
assignment is enough with no export. That this shell has a locale at all
was decided once, for every operator, in *Locale, decided as a policy*
below; nothing here invents it.

**What happens when it has none** is the dialect's, and the panel gives
**three** answers — `Semantics.UnicodeEscapeOutsideTheLocale`. Measured
2026-09-11 under `LC_ALL=C`, bytes read with `od`:

| | `echo 'a\u00e9Z'` | then `echo AFTER` | status |
| --- | --- | --- | --- |
| bash 5.3 `-e` | `a\u00E9Z` | runs | 0 |
| zsh 5.9.2 | `character not in range` on stderr, `a` and a newline on stdout | does **not** run | 0 |
| ksh93u+ | the character, `61 c3 a9 5a` | runs | 0 |

The third answer is a shell that never consults a locale for the escape at
all, and it is reachable at two sites rather than at all of them: ksh93 has
no `\u` in `echo`, in `print` or in a `%b` argument, so it is **`$'…'` and
a `printf` format** that need the third constant. `$'…'` is a *core*
construct, which is what makes the three-valued answer reach the common
denominator: a core script holding `$'\u00e9'` under a non-UTF-8 locale is
an unanswered axis rather than a value, in the one place this disagreement
is not a dialect's alone (#2021).

**Every site that reads the escape asks**, and each shell answers the same
at every site it reads it at — measured one site at a time rather than
assumed from `echo`. The sites are `echo`, `print`, a `printf` format, a
`%b` argument, `$'…'`, and the `(g)` and `(p)` expansion flags.

The **status** a refusal leaves differs by where the reading happened, and
that is a fact about the site rather than another axis: a builtin's refusal
leaves **0** — `echo`, `print`, both `printf` sites — and one raised while a
*word* is expanded leaves **1**, since the word it was part of never
finished and the command never ran. Measured on zsh 5.9.2: `x=$'a\u00e9Z'`
exits 1, `(exit 3); x=$'a\u00e9Z'` exits 1 as well, and `${(g::)v}` and
`${(pj:…:)a}` do the same.

Three details of each are pinned because each is a way to get the answer
wrong while looking right.

The escape bash leaves standing is **normalized rather than echoed**:
`\ue9` and `\U000000e9` both stand as `\u00E9`, so the digits are padded
to four and upper-cased and the letter is chosen by the value rather than
taken from the input; a value that will not fit in four digits takes
`\U` and eight, `\U1F600` standing as `\U0001F600`.

zsh's refusal is located as the **shell** and not as `echo` —
`zsh:1: character not in range`, the same prefix from a `printf` format
and from a `$'…'` in an assignment that never reached a command — which is
the tell that this is a fact about reading a word. It reports **once**
however many such escapes the word holds, the text stops at the first of
them, and at a builtin the status is **0**, so a script cannot see it in
`$?`: `(exit 3); echo 'a\u00e9Z'` also exits 0, which is what separates
"zero" from "whatever it already was" — and the expansion sites' 1 is one
for the same reason. A subshell absorbs the abandonment the way it
absorbs any other, so `( echo 'a\u00e9Z' ); echo AFTER` reaches AFTER.

The axis is asked **only** where an escape actually names a code point the
locale refuses, so an ASCII one needs no answer from anybody — `\u007f` is
the DEL byte in both shells under `LC_ALL=C` and `\u0080` is the first one
outside — and neither does any escape at all in a UTF-8 locale.

Two limits are stated rather than hidden.

**An unset locale is a question of its own**, and it decides whether this
axis is reached at all rather than what it answers: under `env -i`, bash
writes `c3 a9` for the escape and answers 5 for `s=héllo; echo ${#s}`,
while zsh refuses the escape and answers 6. That is
`Semantics.UnsetLocaleIsUnicodeAware` — see *Locale, decided as a policy*
below — and with it answered, a dialect reading an unset locale as C
reaches this axis there exactly as it does under `LC_ALL=C` (#2020).

**A single-byte encoding that is not ASCII is treated as ASCII.** bash
transcodes there, writing `\u00e9` as the single byte `e9` under
`en_US.ISO8859-1`, and this implementation has no charset tables. It is
the limit `multibyteLocale` already records for the same reason — every
non-UTF-8 encoding counts bytes — and the alternative is a table per
charset.

### A hexadecimal escape with no digits

The second axis, and one question for `\x`, `\u` and `\U` together:

    echo 'a\xZ'   zsh  a<0x00>Z     bash 5.3 -e  a\xZ
    echo 'a\uZ'   zsh  a<0x00>Z     bash 5.3 -e  a\uZ

`Semantics.EchoEmptyHexDigitRunIsNul`, asked only where such an escape
actually runs out of digits. It is the same split `PrintfHexEscapePolicy`
records at the two `printf` sites, where it is one of the three details that
made a policy out of a bool.

## A function file that defines the function it is named after

A file on `$fpath` may hold the function's **body**, or it may hold a
`name() { … }` definition of the name it is called. Both run.

Measured on zsh 5.9.2, 2026-09-10, with two files in one directory:

    fns/pfn   print "PLAIN ran [$*]"        pfn a b   PLAIN ran [a b]
    fns/kfn   kfn() { print "K [$*]" }      kfn a b   K [a b]

The shape is decided at **load** time rather than at call time, which is what
`autoload +X kfn; functions kfn` settles: the function is listed with the
*inner* body before anything has called it. So the definition is unwrapped
where the file is read, and the first call runs the real body with the
arguments it was made with — no second call and no re-entry.

It is the **whole file** that has to be the definition, and three files
settle which reading that is:

    helper() { … }; tw2() { … }   defines its own name, and is *not* called
    { grp() { … } }               a definition inside a group, not called
    # comment                     a comment above it does not count against
    cmt() { … }                   it, and it *is* called

so it is neither "the load defined this name" nor "the last command was a
definition". A trailing `;` makes no difference either way.

This is the shape #1580 described: a function loaded this way used to be a
no-op that reported success, and the caller above it read the silence as a
result. Nothing was missing afterwards, because the file's text had become
the body and running it *was* the definition — so the second call worked and
`functions` looked right, which is what made it silent.

## What a failed `(( ))` leaves behind

The sentence and the status are two questions, and only the second one
splits the panel here. Measured 2026-09-10:

    (( 1+ ))      zsh   bad math expression: operand expected at end of string, 2
                  bash  arithmetic syntax error: operand expected, 1
    let "1+"      both  the same reason in each shell's words, 1

zsh leaves **2** where the rest leave 1, and it does so for every way the
expression can fail — an unreadable one, a division by zero, a call to a
math function nothing defines — while the same text through `let` is 1 in
zsh, bash and ksh93 alike. That pair is what places the question: it is
the *construct* and not the evaluator, so `Semantics.ArithCommandErrorStatusIsTwo`
is asked at `(( ))`'s error path and nowhere else. A shell that hung the
status on the arithmetic failure itself would have moved `let` with it.

It is a status and not a truth, so `if (( 1+ ))` reaches it the same way:
the condition is false and the status behind it is the dialect's.

ksh93 is not in the split because it does not stay to answer — a math
error there abandons the input, which is a separate divergence from this
one and is not implemented here.

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

Of these, six are real here — `pipefail` (the pipeline code reads it),
`hashall`/`trackall` (one state behind both names: permission to cache
rather than a promise to), `histignoredups` (kept truthfully over a
history this shell does not keep), `braceexpand` (below), and `onecmd`
(below). The rest are recorded with the state we are already in, so that
turning them off succeeds honestly: `interactive-comments` is **on**,
because we do honor comments wherever they are written; everything else is
**off**. That is not a claim about what any other shell defaults to —
bash has `hashall` on and we do not hash at all, so ours is off and a
script turning it off gets what it asked for.

### `braceexpand`, and the letter `set -B`

`braceexpand` was in the recorded set until #1856, and it is the case that
says what is wrong with recording a name wired to something. The option is
**on** here because braces really do expand, so `set -o braceexpand` was
granted and said nothing false; `set +o braceexpand` was refused as not
implemented, and the letter `-B` was refused as a letter this shell has not
got — while the braces went on expanding under either spelling. zsh's own
name for it, `ignorebraces`, had gone the other way in #1739 and was
*accepted*, listed, and read back by `[[ -o ignorebraces ]]`, with nothing
following from it. A request remembered is worse than a request refused,
because only one of the two tells the script the truth.

What it needed was a run-time switch beside the dialect's answer, and the
two are different questions: `Semantics.BraceExpansion` is whether this
shell has braces at all — dash does not — and the option is whether the
script has asked it to stop. Measured 2026-09-11:

| asked | bash 5.3.15 | ksh93 | zsh 5.9.2 | dash |
| --- | --- | --- | --- | --- |
| `set +B; echo {a,b}` | `{a,b}` | `{a,b}` | `a b` | `Illegal option -B` |
| `set +o braceexpand; echo {a,b}` | `{a,b}` | `{a,b}` | `{a,b}` | refused |
| `set +B; set -B; echo {a,b}` | `a b` | `a b` | `a b` | refused |
| `set +B; echo $-` | `hc` | `chs` | `569X` | refused |

Four things follow, and each is a place the switch has to be *read* rather
than only stored. The long name is unanimous among the shells that have
braces and so needs no axis; the letter is not, because zsh means the
terminal bell by `-B` — that is `SetBTurnsOffBraceExpansion`. A redirection
target counts braces of its own, so `: > {a,b}` is one filename while the
option is off and an ambiguous redirect once it is back on. `$-` drops the
letter, which is the only kind of letter `$-` cannot derive from a state —
a startup letter is on before any script runs, so the dialect's string says
so and turning the option off has to take it back out. And it is **not
one-way**: `set -B` after `set +B` restores the expansion, unlike `noexec`,
so a switch asserted in one direction only would be a trap.

The table is a subset of what these shells actually have — ksh93's own
listing runs to `bgnice`, `globstar`, `letoctal`, `markdirs` and a dozen
more. Names outside it are refused by name rather than accepted and
ignored: under `set -o`, accepting an option we do not honor would be a
promise. zsh's `setopt` answers the same question differently and on
purpose — see **zsh's option names** below — because the names a zsh rc
file writes are overwhelmingly about features this shell does not have at
all, where recording a request promises nothing.

### `onecmd`, and the letter `set -t`

**Exit after reading and executing one command**, which is a real behavior
and not a name to record. Measured 2026-09-10 against bash 5.3.15, bash
3.2.57 and ksh93; zsh has the name as a borrowed spelling for
`singlecommand` and refuses to move it, and dash has neither the name nor
the letter.

Five facts, and each of them bounds the implementation:

- **The unit is the line the shell read, not the next command.** `echo A`,
  `set -t`, `echo B` in a script writes `A` and stops — `B` never runs — so
  nothing further is read once the line that set the option finishes.
  `set -t; echo B` on *one* line writes `B`: the line runs to its end first.
- **It is checked after the line, so the line can take it back.**
  `set -t; set +t` on one line reads on to the next.
- **A compound command is one line.** `set -t` inside an `if` runs the rest
  of the branch and stops at `fi`.
- **A sourced file is not the shell that stops.** A file that turns the
  option on runs to *its* end; the shell that was reading when `.` returned
  is the one that reads no further.
- **The status is the last command's**, unchanged by the stopping.

The exit status and the state are visible before the shell goes: `$-`
carries `t` in both shells that have the option — in bash even on the one
route where it never stops — and `SHELLOPTS` carries `onecmd`.

**The command-string route is the disagreement, so it is an axis**
(`Semantics.OneCommandStopsACommandString`). A two-line `-c` string that
turns the option on and then echoes writes the echo under bash and writes
nothing under ksh93. Both stop a script file, both stop standard input, and
both stop after the first line when the option came from the invocation —
`bash -t script.sh` runs one line of it. That route is the reason this
matters beyond the option itself: an agent harness snapshots a shell with
`set -o | grep on | awk '{print "set -o " $1}'`, and `grep on` matches the
*name* `onecmd` rather than the status column, so the snapshot sets the
option on every command it sources ahead of — and bash reads on regardless
(#1709).

Which shells have the letter is a second axis (`Semantics.SetHasTheTLetter`),
because ksh93 has *only* the letter — `set -o onecmd` there is
`bad option(s)` — while zsh refuses the letter as it refuses the name.

**zsh's refusal is a third answer and is worded with the letter.** `set -t`
there is `can't change option: -t` at 1 and fatally — the same sentence,
status and fatality as `set -o singlecommand`, with the letter echoed back
rather than the option's own name. That is not "a letter we have not built",
which is what `Diagnostics.UnimplementedOptionLetters` says, so it has a
table of its own: `Diagnostics.ImmovableOptionLetters`, read first, and a
letter in both would be exactly the pairing failure this section is about.

And the refusal is only of a *move*: asking for the state the option is
already in succeeds, silently, as it does for every long name. Measured
2026-09-10 — `set +t` in a plain zsh is 0 and says nothing, `set -t` in a zsh
started with `-t` is 0 and says nothing, and each of them the other way round
is the refusal. dash refuses both signs alike, because it has not got the
letter at all, which is what says the grant hangs on the shell *having* the
letter and not on the state alone.

**The invocation is a route split inside one shell**
(`Semantics.ImmovableOptionsSetAtInvocation`). Real zsh takes `-t`,
`-o onecmd` and `-o singlecommand` on the command line and runs one line of a
script, refusing only what a *running script* asks for. Measured on zsh 5.9.2,
2026-09-10, against a three-line script: each of the three invocations writes
the first line and stops, `zsh -t -c 'echo "$-"'` is `569Xt`, and `set -t`,
`setopt singlecommand` and `unsetopt singlecommand` inside such a script are
all `can't change option` at 1 and fatal. So the five names that shell calls
fixed are not five states it cannot reach; they are five a script may not
change (#1730).

The axis governs the *refusal* and not the applying, which is what keeps it
one question rather than five. A dialect that answers `Yes` still has to say
what each such name would move: the `-t` letter writes the substrate's
`onecmd`, which is where the core reads the axis, and a name writes whatever
the dialect's own table says — `dialect/zsh/setopt.go`'s `singleCommandOption`
is the one entry in that table with something to apply, and the other four
fixed names are refused at an invocation exactly as they are refused in a
script. Both invocation routes reach the dialect table *after* the compat
spellings have resolved, which is why `-o onecmd` and `-o singlecommand` get
the same answer without the letter reader knowing either name.

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
| substrate-backed | 15 | moves a real `set -o` switch: `setopt err_exit` **is** `set -e`, and `setopt vi` **is** `set -o vi`. `ignorebraces` is the inverted one: it is `set +o braceexpand`, zsh naming the state that *stops* the expansion where the substrate names the expansion |
| axis- or matcher-backed | 12 | moves a semantics axis (`shwordsplit`, `nomatch`, `ksharrays`, `localtraps`, `multios`, `globsubst`, `typesetsilent`) or a pattern-matcher option (`nullglob`, `globdots`, `caseglob`, `extendedglob`, `bareglobqual`). `ksharrays` is one name over **five** axes — see below |
| fixed | 4 | refuses to move, in zsh's own words: `can't change option: NAME`, status 1. Asking for the state it already holds is granted, and one of the four is taken at the *invocation* — see `singlecommand` below |
| store-backed, read by the front end | 4 | `histignorespace`, read by the line editor before it records a line; `checkrunningjobs`, read by `checkjobs` when it recomputes what the exit is held for; and `cshnullcmd` and `shnullcmd`, read together when either moves so that the first can win while it is on. All four are kept where a recorded name is kept, because the substrate has no `set -o` name for any of them |
| switch-backed | 3 | `aliases`, `autocd` and `checkjobs`: each moves a capability the substrate holds under no option name of its own — alias expansion really does stop, a bare directory name really is read as a `cd`, and a job still running really does hold the exit |
| **recorded** | 147 | succeeds, is remembered, and is reported by `setopt`/`unsetopt` — and changes nothing about what the shell does |

**Two names moved out of "recorded" when the history knobs were built**
(#571). `histignorespace` is the fifth row above: its state has nowhere
better to live, because the substrate has no `set -o` name for it, but an
interactive session reads it through this namespace every time it accepts
a line. `histignoredups` was already substrate-backed — zsh's `set -h`
abbreviates it — and was a switch whose state nothing consulted; it is now
consulted too. Both decide what a session writes to its history file, so
neither is recorded any more. `extendedglob` left the same way when the
pattern operators it gates were built (#1244), which is the third name to
move and the reason the matcher-backed row now reads 7. `autocd` is the
fourth, and it left for the same reason with one difference worth naming:
bash spells the identical capability `autocd` too, so a dialect that
implemented it for itself would have implemented it *instead* of the other
one (#1445). It is the sixth row above — one capability in the substrate,
two shells' names over it, and only whether the substitution is announced
told apart, by an axis. `checkjobs` and `checkrunningjobs` are the fifth and
sixth, and they left together because they are one question asked twice: the
first is the master over whether the shell looks at its job table before
leaving, the second narrows that to suspended jobs only, and neither can be
read without the other — which is why one is switch-backed and the other is
store-backed rather than both being one or the other (#1445). bash spells the
master `checkjobs` as well, so this is the second capability in the substrate
with two shells' names over it; the defaults differ (bash off, zsh on) and so
does the reach, since bash's name governs only the running half while zsh's
governs both. `localtraps` is the seventh name to move (#1731): a trap a
function sets goes back at the return, and it is *not* the trap-side reading
of `localoptions` — measured, that option leaves a function's trap installed,
and the save this one takes is per condition and taken at the modification
rather than at the call. `multios`, `cshnullcmd` and `shnullcmd` are the
eighth, ninth and tenth (#1779), and they left together because they are two
questions in one neighborhood: `multios` is zsh's name for the axis that
sends a stream to every target it names and reads it from every source, and
the other two are what a command that is only redirections runs — csh's
reading refuses it, sh's runs `:`, and both are reached by pointing the
null-command parameters at a name a script cannot write.

**Ten names moved the other way in #1739** — out of "fixed" and into
"recorded", except two which went further. Real zsh moves all twelve of the
names this table had wired immovable, measured one at a time with no terminal:

    zsh -f -c 'setopt NAME; print -n "on:$? "; unsetopt NAME; print -n "off:$?"'

answers `on:0 off:0` for `banghist`, `chaselinks`, `emacs`,
`functionargzero`, `hashdirs`, `ignorebraces`, `ignoreeof`,
`interactivecomments`, `notify`, `privileged`, `shglob` and `vi`, where this
shell answered `can't change option` to one direction of each. They are not
one situation and were not swept. `emacs` and `vi` are the substrate's own
editing mode, which really does move and really is one state under two
names — `setopt vi` deselects `emacs`, measured in both shells — so they are
substrate-backed now. The other ten are states this shell holds and does not
leave, which the three-way split had no room for: "recognized, remembered,
and the shell keeps doing the thing" is exactly what `recorded` says, and
saying it is the honest answer where refusing a move nobody can observe was
not. `login` is the tenth and is not from that list at all — see below.

**Two more left "recorded" in #1734 and #1729**, and they left for the
sharpest version of the reason: each was a name this shell read, stored and
reported faithfully while asking it nowhere, which is worse than a name that
is missing. `globsubst` is this shell's own name for `GlobExpansionResults`
and now moves it, so the result of an expansion is read as a pattern on every
path that takes a pattern operand — the per-expansion spelling `${~x}` was
already an override of that axis, which is what said the machinery existed
and the *option* was what nothing consulted. `bareglobqual` decides whether a
trailing `(…)` is a glob qualifier list or pattern text, and it is
`interp.TrailingGroupIsPartOfThePattern` — see
`docs/spec/grammar/patterns.md`, which has both measurements.

`ignorebraces` moved out of "recorded" the same way in #1856, and it is the
clearest case of why the recorded kind is a placeholder rather than an answer:
the name is about a **behavior this shell really performs**, so remembering
the request and going on expanding braces was a shell that agreed it had been
told and then did the opposite. #1739 had moved it *into* recorded, which was
the honest state of that change — the alternative was going on refusing a move
real zsh takes in both directions — and it is now the switch beside
`Semantics.BraceExpansion` instead. Recording is right for a completion knob
this shell has no completer for; it is wrong for a knob wired to something.

`typesetsilent` is the most recent name to leave, and it is the one entry
whose sense is **inverted**: the option being on is
`Semantics.ValuelessDeclarationOfAHeldNameListsIt` answering No. A valueless
declaration of a name the running scope already holds writes that name back —
`f(){ local s; s=1; local s; }` says `s=1` — and TYPESET_SILENT is how a
script asks for the quiet. It moved because powerlevel10k sets it in its
`emulate -L zsh` intro and then re-declares `local` names inside loops, so a
shell that remembered the name and ignored it wrote nine lines to stdout
before **every prompt** of a real interactive session (#2033). That is the
shape of the recorded bargain failing: the complaint it was meant to stop came
back as output instead.

So 147 of 185 are recorded, the count above is the one produced by counting
the constructors in `dialect/zsh/setopt.go`, and **the fixed set is now
exactly the set real zsh refuses**: `interactive`, `shinstdin`,
`singlecommand` and `zle`. `monitor` left it in #1720 because zsh grants it
where the shell has a terminal.

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

The names that refuse to move are the ones about being interactive —
`interactive`, `monitor`, `shinstdin`, `singlecommand`, `zle` — which is
measured rather than chosen: asking a real non-interactive zsh for all 197
names in both directions refused exactly those five (plus their two compat
spellings) and granted every other one. `monitor` left the set in #1720,
because a zsh with a terminal moves it in both directions, and the remaining
four are this shell's too. There is no longer a set of immovable names of
our own: #1739 emptied it, which is what makes this table's refusals a fact
about zsh rather than a confession about this implementation.

`singlecommand` is the one of the four with a route split under it. A running
script may not move it in either direction — measured, and this shell answers
the same — while the command line that started the shell sets it under any of
its three spellings, which is
`Semantics.ImmovableOptionsSetAtInvocation` and the `set -t` section above.

**`login` reads the invocation.** It is one of zsh's 185 and it reads on for
exactly the invocations that made the shell a login shell, which is where zsh
keeps that fact — `Semantics.LoginShowsLInDollarDash` is `Yes` here precisely
because `$-` does not carry it, and bash spells the same fact
`shopt login_shell` instead (#1709). Ours omitted it from the listing
altogether: measured against zsh 5.9.2, `-l -c setopt` writes three lines
there and two here, `login` being the missing one. It is the invocation's
answer and still *movable* on top of that, which was measured rather than
assumed and is the opposite of bash's answer for its own spelling: `setopt
login` in a shell that is not one is 0 and reports `login` afterwards, and
`unsetopt login` in a login shell is 0 and stops reporting it, where `shopt
-s login_shell` is accepted and never moves. So it is recorded over a base
state the front end supplied rather than over a constant (#1727).

**`rcs` reads the invocation too**, and it is the other one of that
kind. It says whether this shell reads its startup files, and zsh
initializes it from the argument vector: `-f` and `--no-rcs` turn it off.
Measured on zsh 5.9.2, 2026-09-11 with an empty home directory —

| asked | zsh 5.9.2 |
| --- | --- |
| `zsh -f -c setopt` | `nohashdirs`, `norcs` |
| `zsh -c setopt` | `nohashdirs` |
| `zsh -f -c '[[ -o rcs ]]; echo $?'` | `1` |
| `zsh --no-rcs -c '[[ -o rcs ]]; echo $?'` | `1` |
| `zsh -c '[[ -o rcs ]]; echo $?'` | `0` |
| `zsh -f -c 'setopt rcs; [[ -o rcs ]]; echo $?'` | `0` |

— so it is the same shape `login` has: the invocation decides the base and a
running script moves it on top, `setopt rcs` in a `-f` shell being 0 and its
listing losing the `norcs` line again. Ours held a constant `on`, so a `-f`
shell reported the files it had just been told to skip and the listing was a
line short exactly where zsh's is a line long. The base is
`Runner.StartupFilesSuppressed`, the invocation fact the front end carries in
beside `LoginShell`, and the option is still recorded: nothing re-reads it to
decide whether a *later* startup file is read, which is `driver`'s question
with the argument vector first-hand (#1864).

The invocation's own options are applied *after* that base is set, which is
the order that makes `zsh -f -o rcs` answer `rcs` on — measured, and the
option moving off a base rather than the base landing on top of the option.
zsh applies the two in argv order and so answers off for `zsh -o rcs -f`;
ours reads `-f` off the vector rather than as a position in it, so it answers
on for both. The measured row is the first spelling.

**The listings.** Every option has one printed spelling — the one that is
off by default, so `noclobber` for an option that defaults on. A bare
`setopt` prints the spellings that are on and a bare `unsetopt` prints the
ones that are off, both ordered by canonical name and both naming the
canonical option rather than the compat spelling that may have set it. In a
shell that has changed nothing that is 1 line and 184.

**`set -o` is this namespace under a POSIX spelling, not a table of its
own** (#1080). Measured: `set +o` writes 185 rows in the same order and the
same spellings a bare `setopt` uses, `set -o autocd` is then visible to
`setopt`, `setopt autocd` is visible to `set +o`, and `set -o Err_Exit` is
folded exactly as `setopt Err_Exit` is. So the two builtins are one
namespace and `set -o` is the third door into it, beside `setopt` and
`[[ -o ]]`.

This dialect wrote the *substrate's* shared names there instead: 23 rows
against zsh's 185, in bash's vocabulary — `braceexpand`, `hashall`,
`histexpand`, `nolog`, `notify`, `onecmd`, `physical` and `trackall` are
eight names ours listed that zsh never writes, and 170 of zsh's were
absent. Both exit 0 and neither says a word, which is what made it the
silent kind rather than a missing feature: `set +o` is a capture surface —
one of the three sections an agent harness snapshots and sources back
before every later command — so a caller recorded a zsh with 23 options,
learnt nothing about the 170 that decide what that shell does, and never
found out it had asked the wrong question. Our *bash* wrote 27 and matched
real bash exactly, which is what said this was a dialect answering with
another shell's vocabulary.

The substrate seam is `Runner.SetOptionTable`, beside the
`SetOptionNamespace` that `[[ -o ]]` already used: a dialect supplies the
listing's rows and a silent mover, and this package words the refusals,
because the same refusal is worded three ways depending on whether a script,
an invocation or an inherited value asked. It is deliberately not
`ApplyNamedOption`, which stays on the substrate's own table — that is the
seam this dialect's entries use to move a substrate option by its substrate
name, so routing it through the table as well would have `setopt err_exit`
call back into the table it was called from.

Two consequences worth stating. Recording is unchanged: a listing 185 rows
long still says nothing about whether a name is acted on, and 151 of them are
remembered and not acted on — the table is longer in the listing because zsh
lists that many, not because more of it is implemented. And a `set -o` name
this shell has and will not move answers `can't change option` at 1,
**fatally**, where it used to answer `not implemented` at 2 and carry on.
That is right for the names zsh itself refuses — measured, `set -o onecmd`
says exactly that and stops the script there too — and since #1739 those are
the *only* names it reaches: `set -o physical` and `set -o histexpand`, the
two the eight above name, are granted now, as real zsh grants them. The
alternative would have been a second, gentler refusal for immovable names of
our own, which would be a second notion of what a refused `set -o` is; there
are none left to need it.

**A known inaccuracy, inherited rather than introduced.** The table records
zsh's default for each name, which is what the listings compare against.
Three entries hold this shell's own state there instead — `banghist`,
`hashcmds` and `interactivecomments` are all measured the other way
round in real zsh — which silences three deviations the listing exists to
show. #1739 moved three of them between kinds and left the defaults
exactly where it found them, because the kind and the default are different
questions: what a name *does* when asked to move is this section, and what
its listing compares against is this paragraph. Correcting them would make the bare `unsetopt` listing byte-identical
to zsh's 184 lines and would move the same lines of divergence onto the
bare `setopt` listing, because the underlying fact is that this shell's
state genuinely differs from zsh's for those three. It is a trade rather than
a fix, and it is left where it was found.

**`emacs` was the fourth and is not any more** (#1858). It is the one of
the four whose default was wrong only because the *state* behind it was:
this shell held the emacs keymap selected from the moment a Runner existed,
so recording zsh's default of off would have shown a deviation on every
listing. Measured on zsh 5.9.2 at a real terminal, `[[ -o emacs ]]` answers
1 in an interactive session that has selected nothing — so nothing is
selected here either until something selects it, the recorded default is off
like zsh's, and the name is silent in every listing until a script moves it.
The spurious `noemacs` row a script got for writing `setopt vi` goes with it.

It is visible in three listings now rather than one, because `set -o` and
`set +o` write from the same table (#1080) and the printed spelling is
derived from the recorded default: those two write `banghist`, `hashcmds`
and `nointeractivecomments` where zsh writes `nobanghist`, `nohashcmds` and
`interactivecomments`. Three rows of 185; the other 182 are byte-identical
to zsh 5.9.2's, in the same order.

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

**Every operator that assigns reaches a target of the same kind.** `=`, `+=`
and the rest of the compound forms took a subscript and `++`/`--` did not, so
`(( m[k]++ ))` was `++ needs a variable` in every dialect while
`(( m[k] += 1 ))` on the same element was fine. A name that can be assigned to
can be incremented — bash, ksh93 and zsh all say so, and they differ here only
by where the first element is — so the two paths are one: a target is a name
and the subscript it carries, read the same way whichever operator wrote it.
An expression that names no storage is still refused, which is what keeps
`(( 1++ ))` an error (#1154).

**On a declared associative name the subscript is a key, not an expression.**
The switch `${m[k]}` already throws has to be thrown inside an expression too,
and it was not: with `m[k]=7`, `m[0]=99` and `k=0`, `$(( m[k] ))` was 99 where
all three shells with the attribute answer 7 — a wrong element with no
diagnostic at all, and an assignment through the same spelling stored under the
number rather than the key. Whitespace belongs to the key here exactly as it
does there: `m[ k ]` and `m[k]` are two keys, measured unanimous.

**An element is read as an operand by the rule a name is read by.** A name
holding a name was chased to a number and an element holding one was refused,
so `y=5; a=(y); echo $(( a[0] ))` errored where bash and ksh93 answer 5. The
storage an operand came out of is not what decides how it reads, and the same
axis answers both — `ArithNameValueRecurses`.

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
bare `|`, we refuse it too and say something else. The behavior matches; the
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
  naming the coprocess as the source. Both constructs are in this grammar
  now — zsh's `coproc` word and ksh93's `|&`, which is
  `Dialect.CoprocPipeOperator` (#1141) — so the letter reads the running
  coprocess's near end where there is one. With none running it is the
  measured refusal, which is what a `-p` outside a coprocess still meets:
  `read: no query process` in ksh93, `-p: no coprocess` in zsh, status 1
  in both, and the variables left exactly as they were — the read failed
  before reaching any input, so the clear-on-EOF rule never fires. The optstring's shape carries the split
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
An explicit `C` or `POSIX` narrows "letter" to ASCII; any other value is
Unicode-aware.

**What no value at all means is the dialect's**, and that half was wrong
here for a while. It was recorded as "unset is Unicode-aware" from one
measurement — bash stripped of every locale variable still uppercases
`café` to `CAFÉ` — and bash is the panel member that reads it that way.
Measured 2026-09-11 under `env -i`, with no locale variable set anywhere,
on three operators that read the same state:

|  | bash 5.3.15 | bash 3.2.57 | ksh93u+ | zsh 5.9.2 | dash |
| --- | --- | --- | --- | --- | --- |
| `s=héllo; echo ${#s}` | 5 | 6 | 6 | 6 | 6 |
| uppercase `café` | `CAFÉ` | *n/a* | `CAFé` | `CAFé` | *n/a* |
| `echo -e 'a\u00e9Z'` | `61 c3 a9 5a` | *n/a* | *n/a* | refused | *n/a* |

One shell reads an unset locale as UTF-8-capable and the rest read it as
C, and each of them reads it the *same* way on every operator — which is
what makes it one axis, `Semantics.UnsetLocaleIsUnicodeAware`, rather than
one question per operator. The case row is spelled per shell (`${x^^}`,
`typeset -u`, `${(U)x}`), and bash 3.2.57 has none of those spellings.

The standard's preset answers **no**: XBD ranks the variables and then
leaves the case where none of them is set to the implementation-defined
default locale, and the default a C program starts in is the C locale —
which is also what every panel member but one does. ksh93 and dash keep
that answer; bash overrides it to yes; zsh states it.

The axis is asked only where the two readings differ — an all-ASCII value
is the same length either way, an ASCII code point fits in every encoding,
and case mapping below 0x80 is the same map — and only after the
operator's own axis has said the question can matter, so a dialect with no
multibyte decoder never reaches it. A shell that never sees a byte above
ASCII never needs an answer, which is what keeps the core from refusing
`${#x}` on `abcd` (#2020).

Decided here once rather than one operator at a time, which is what issue
#367 asked. Case conversion (`${x^^}` and family) follows it now, and so
does a `\u` escape naming a code point the encoding has no room for — see
*A code point the locale has no room for* above, which is the worked
example of an operator being brought under this policy rather than
deferred for it. The character classes in globs, `[[ a < b ]]` and glob
collation follow the same rule as each is brought to the measured
behavior.

The reverse mistake is worth recording, since it cost this repository two
releases of a known-and-unfixed issue: #1851 was deferred on the grounds
that *"how much of a locale this shell has is a larger question than the
escape"*, which had already been settled here. A policy decided in one
place is only useful if the next operator looks for it.

**"And family" was doing real work in that sentence, and it was not true.**
#367 brought the case-changing *operators* — `${x^^}`, `${(U)x}` — under
this policy and left the two sites beside them still calling
`strings.ToUpper` directly: the case-changing **attribute** (`declare -u`,
`typeset -l`) and zsh's **`:u`/`:l` modifier**. Measured 2026-09-11 under
`LC_ALL=C`, uppercasing `café`:

|  | bash 5.3.15 | ksh93u+ | zsh 5.9.2 | ours, before #2027 |
| --- | --- | --- | --- | --- |
| `declare -u s=café` | `CAFé` | `CAFé` | `CAFé` | **`CAFÉ`** |
| `s=café; ${s:u}` | *n/a* | *n/a* | `CAFé` | **`CAFÉ`** |
| `s=café; ${s^^}` | `CAFé` | *n/a* | *n/a* | `CAFé` — right |
| `s=café; ${(U)s}` | *n/a* | *n/a* | `CAFé` | `CAFé` — right |

The panel agrees at every site, so this was a **correction** rather than an
axis: the policy above already answered it and two more operators simply had
to ask. What let them not ask is that the narrowing was three lines each site
repeated, so a site could be added — or left behind — without anything
noticing. It is now one helper, `Runner.caseMapper`, and the four sites call
it; a narrowing every caller has to remember is one some caller will not.

The **test shape** is the other half of the lesson. Each of the four sites
already had its own test and each test agreed with its own site, which is
precisely the arrangement that cannot see drift *between* sites. The pinning
is now one table run over all four spellings, so a fifth site that does not
ask is a failing row rather than a silent one (#2027).

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
  measured axes above and reset the option table to the emulation's
  defaults, `-c` runs a string under the emulation and restores everything
  after, and a bare `emulate` names the mode. `ksharrays` is one of the
  axes it switches and is a **group** of five, so an emulation moves the
  array base, what a plain `$a` is worth, how many fields it is, what
  `${#a}` counts and whether an unbraced name's brackets are a subscript,
  all together — see `dialect/zsh/ksharrays.go` for the measurement and
  #1726 for what moving only the first of them cost.
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
  history this shell does not keep, and `-p` writing to the coprocess
  `cmd |&` started — ksh93's own `no query process` when none is running,
  the same shape `read -p` measured.
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
  function and a math function. The table says what the module *is* and the
  shell answers whether it has it, so the answer is mechanical rather than a
  claim and nothing is stubbed.

  **A module loads when everything it names is either implemented or refuses
  by name on access, and nothing it names may read as empty when it is
  absent.** That is one sentence about what a script is told, and it replaces
  a first version that split on the kind of feature — builtins never held a
  module shut, parameters always did. The reasoning was right and the wording
  was not: what it was reaching for was never presence, it was **legibility**.

  A missing builtin is `command not found: zregexparse` at the word that ran
  it, so `zsh/zutil` loads with three of its four builtins and the fourth
  refuses at its own call site. A missing parameter had no call site — a
  `${#jobstates}` nobody implemented is `0` at status 0, a plausible answer to
  a different question reaching the caller as data — so a module short of one
  refused, and `zsh/parameter` stayed shut over twenty-eight parameters no
  caller touched. A parameter can have a call site too, and now does:
  `jobstates: parameter not implemented yet` at the expansion that read it,
  with the command not run. Of that module's thirty-three, five are
  implemented, ten are empty and right to be — nothing here can define a
  global alias, disable a function or name a directory, so "none" is true —
  and eighteen refuse.
  A condition, a function and a math function have no call site of either
  kind, so those still hold their module shut: `zsh/complete` is refused over
  its four conditions and not over its two builtins.

  **`zsh/datetime` is the one module in the table that needed none of that**:
  all four of its features are implemented, so it loads because the shell has
  it rather than because the rule forgave anything. What the rule bought was
  the *question* — with a module able to load short of a feature, "which of
  these four are worth having" is answerable one feature at a time instead of
  all-or-nothing (#1154). It is also the first second module: until it landed,
  only `zsh/main` could be loaded and nothing could tell the sorted listing
  from an unsorted one.

  **Not a silent success**, which is the failure this builtin is most able to
  cause: a script told `zsh/zutil` loaded and then calling `zparseopts` fails
  several hundred lines later, in a function whose caller has gone, about a
  command nobody wrote. So a refusal names the module and the features it is
  short of — `after, between, prefix and suffix are not implemented yet` — up
  to six of them, beyond which a count speaks instead, because thirty names on
  one line is not something a person reads. Nothing in the table reaches that
  count now that `zsh/parameter` loads; the rule is kept for the next large
  module. The reason clause is the only part that is not zsh's: zsh's is a
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

  **`-F` selects which of a module's features it exposes**, measured 2026-09-09
  against zsh 5.9.2. An operand is `[+-]kind:name` against the module's own
  feature list; a bare name means `+`, the last operand about a feature wins,
  and an operand naming no feature of the module is `module 'X' has no such
  feature: 'Y'` and 1 with *nothing* applied — the module is left unloaded even
  when a good operand came first. The starting point is what makes it a
  selection rather than a set of deltas: a module that is not loaded starts
  with every feature **off**, so `zmodload -F zsh/zutil b:zstyle` finishes with
  one on and `zmodload -F zsh/zutil` with none; a module already
  loaded starts with whatever it has, so the same command moves only what it
  names. A plain `zmodload <module>` puts every feature back, and so does an
  unload followed by a load. `-lF` reports the selection with `+` and `-`
  against each feature and `-LF` writes it as the command that would reproduce
  it. The operands after the module on either listing letter are a filter
  compared to the feature names *as written*, so `b:zstyle` narrows the listing
  to one line and `+b:zstyle` narrows it to none. `-lF` with no module is `-F
  requires a module name`; `-LF` with no module lists every loaded module that
  has features. `-u` with `-F` is `-b, -c, -f, -p and -u cannot be combined
  with -F`.

  Narrowing moves the verdict, which is what makes the letter implementable in
  a shell that cannot load a compiled module: the features a caller names are
  the ones the module is judged on, so `zmodload zsh/complete` is refused for
  four conditions nobody can find and a caller naming something else is asking
  a question this shell can answer.

  **And a feature named under `-F` holds its module shut even when it is a
  builtin** (#1634), which is the one place the legibility rule above does not
  reach. `zmodload <module>` is the *shell* inferring what a script wants, so a
  missing builtin never holds: it is loud at its own call site and refusing
  would stop a file for a command it never calls. `-F` is the script saying
  which feature it wants, and measured against zsh 5.9.2 the status of that
  line is the answer to "have I got this command" — `zmodload -F zsh/files
  b:zf_rm` at 0 is followed by a `zf_rm` that runs, and the same feature
  switched off leaves `zf_rm` not a builtin at all. So `zmodload zsh/zutil` is
  0 and `zmodload -F zsh/zutil b:zregexparse` is 1, naming `zregexparse`; a
  script's `zmodload -F zsh/stat b:zstat || return` is a guard and gets a
  truthful answer. Only the features switched **on** by that command are
  judged; a module already loaded whole keeps the rest under the plain rule.

  **And it takes a feature away.** A builtin the selection leaves off is
  removed from the table, which is measured rather than inferred from the
  listing: `zmodload zsh/zutil; zmodload -F zsh/zutil -b:zparseopts` leaves
  `zparseopts` at `command not found` and 127, with the three features left on
  still answering, and either `+b:zparseopts` or a plain `zmodload zsh/zutil`
  puts it back. `whence` and `type` do not find it while it is off, `disable`
  lists nothing, and `enable zparseopts` is `no such hash table element` — so
  it is not `enable -n` under another name, and `SetBuiltinEnabled` would have
  been the wrong seam (#1635). `interp.Runner.SetBuiltinWithdrawn` is the
  right one: the builtin is kept and the *lookup* fails, which is what lets
  the selection move in both directions without a second table of feature
  names to functions. The feature name is the builtin name, so there is
  nothing to keep in step.

  **The parameter half goes the same way through a seam of its own** (#1841),
  and the two seams differ in the one way the shell does. A deselected
  *builtin* refuses; a deselected *parameter* says nothing at all. Measured
  2026-09-10 with one function defined:

      zmodload zsh/parameter
      zmodload -F zsh/parameter -p:functions
      ${#functions}    0         ${+functions}    0
      ${functions[f]}  (empty)   ${(k)functions}  (empty)
      ${(t)functions}  (empty)   status           0 throughout

  So `SetAbsentParameter` would have been the wrong seam and not merely an
  inexact one: that one refuses by name, which is a diagnostic where this
  shell is silent — "I never had this" against "you asked me to put it down".
  `interp.Runner.SetParameterWithdrawn` takes the name out of the parameter
  tables and keeps what was in them, so the selection moves in both
  directions without a second table of feature names to producers, exactly as
  the builtin half does.

  **Read-only comes off with it**, which is measured rather than tidied.
  `$parameters` and `$builtins` carry the attribute so that a write cannot
  land in a stored table and shadow its own producer; with the producer gone
  there is nothing to shadow, and `zmodload -F zsh/parameter -p:parameters;
  parameters=(a b)` assigns in real zsh and prints `a b` where ours answered
  `read-only variable: parameters`. The mark comes back with the producer.

  One divergence is left and is recorded rather than modeled: putting a
  parameter back **after a script has assigned to the name** is `Can't add
  module parameter` and status 2 in real zsh, where ours restores in silence
  at 0. The value a later read gets is the script's either way.

  Refused by name: `-a` with `-b`/`-c`/`-f`/`-p` (autoloaded builtins,
  conditions, functions and parameters), `-A` and `-R` (module aliases), `-d`
  (the dependency table), `-m` (pattern arguments), `-I` and `-P`. The
  twenty-two letters zsh's `zmodload` does not have at all are `bad option: -q`
  and 1, which is this builtin's wording and `bindkey`'s, not `zstyle`'s
  `invalid option`. Corpus: `zmodload/*`.
- **zsh `zsh/stat`, `zsh/files` and `zsh/net/socket`** (dialect/zsh/statmodule.go,
  filesmodule.go, socketmodule_unix.go): the three modules a prompt theme
  narrows to at startup, measured 2026-09-09 against zsh 5.9.2 with `zsh -f`.

  **Only the names that cannot shadow a command are registered.** Each module
  provides its commands under two spellings — `stat` and `zstat`, `rm` and
  `zf_rm` — and this shell's builtins are the dialect's, registered before any
  script runs, so a `stat` registered at all is a `stat` registered always and
  every `stat -f %z` in every script would stop reaching /usr/bin/stat. zsh
  does not have that problem because the module really is loaded on demand, and
  the manual makes the same recommendation for the same reason. So `zstat`, the
  nine `zf_` names and `zsocket` are implemented, the ten plain names are in
  the feature table as features this shell has not got, and `zmodload -F
  zsh/files b:rm` refuses by that name.

  `zstat` is the whole system call: fourteen elements in a fixed order, one
  selectable with `+element` shortened to any unique prefix, `-A` into an
  array, `-H` into an association (one file only), `-f` for a descriptor, `-l`
  for the element names, `-L` for the link rather than its target — which
  `+link` turns on by itself — `-s` for the written forms of the mode, the two
  owners and the three times, `-F` and `-g` for the format and the zone of
  those times, `-r` for the number *and* the written form, `-o` for an octal
  mode, and `-n`/`-N`/`-t`/`-T` for whether a name and a type appear beside
  each value. A whole listing writes the element name in a seven-wide column;
  a selected one does not. A file that cannot be statted leaves an `-A` array
  untouched, so a script reading it never sees half an answer.

  `zsh/files` is the nine operations with the letters each of them takes, and
  the default query — before replacing or removing a file the shell cannot
  write to — as well as the `-i` that asks about every file. **`-s` is refused
  by name**: it asks that no link be followed *during the descent*, which is a
  promise about how each component is opened, and a version that only checked
  the operand would claim the guarantee while leaving the hole it closes.

  `zsocket` is a Unix stream socket in three forms — connect, listen, and
  accept with `-a` — each ending with a descriptor in the shell's own table and
  `$REPLY` holding the number, `-d` choosing the number and `-v` saying where
  it went. `-t` asks rather than waits and is *silent* about finding nothing.
  A listener asks the kernel to queue **one** unaccepted connection rather than
  the hundred and twenty-eight a runtime's own listener asks for, which is what
  a `while zsocket $sock; do` loop written to stop on the refusal needs. What a
  full queue then does is the kernel's and differs: measured 2026-09-10 against
  real zsh, macOS queues one and refuses the next while Linux queues two and
  **blocks** on the next. This shell answers as zsh does on each.

  All three resolve a relative name against `Runner.Dir` and never the
  process's directory, which are two different directories the moment a script
  writes `cd`. Corpus: `zmodload/the-startup-modules-a-prompt-narrows`,
  `stat/*`, `files/*`.
- **zsh `zsh/system`'s six builtins** (dialect/zsh/systemio.go,
  systemlock.go, systemseek.go):
  `sysopen`, `sysread` and `syswrite`, measured 2026-09-10 against zsh 5.9.2
  with `zsh -f`. The module already loaded here — its two parameters and its
  math function are implemented — and installed none of its builtins, which is
  the worse half of a silent failure: a script's `zmodload zsh/system ||
  return` passed and the wall arrived several hundred lines later as `command
  not found: sysopen`, inside a prompt theme's asynchronous worker on the line
  its `|| return` guards (#1737).

  **Six of six, in two halves.** #1737 was the three that move bytes and left
  `zsystem`, `sysseek` and `syserror` registered in the feature table and
  refused *by name* — at their call site, and under `zmodload -F zsh/system
  b:zsystem`, which was 1 here and 0 in zsh. That is the right state for a
  half-written module and not the right state to leave one in, and #1749
  closed it. The corpus row that recorded the difference now records
  agreement, and keeps one line — `-F` naming a feature nobody has — that must
  still be able to say no.

  `sysopen` is `exec {fd}<file` with the flags spelled out. `-u name`
  allocates a descriptor and puts the *number* in the named parameter, by the
  same route `exec {name}<` takes, so `-u 'h[k]'` resolves as it does there; a
  `-u` that is all digits is a number instead. No direction letter is
  read-only, `-w` is write-only and **does not truncate**, `-a` appends, and
  `-r` with either of the others is read-write. `-o` is a comma list of nine
  names — case-insensitive, an `O_` prefix allowed, and not prefix-matched —
  of which `excl` carries O_CREAT with it (measured: `-o excl` alone *creates*
  the file, and O_EXCL without it is ignored by the system, so a guard written
  that way would claim a lock somebody holds) and `cloexec` is a mark on the
  shell's descriptor table rather than a flag on the open, because the runtime
  opens everything close-on-exec already and the boundary a child inherits is
  rebuilt from that table. `-m` is an octal mode and 0666 is the default the
  umask then trims.

  `sysread` is **one** read: `-s` bounds it, a short read is a success, nothing
  is split, and the status is its whole vocabulary — 1 a usage error, 2 the
  read failed, 3 the copy `-o` asked for failed, 4 `-t` elapsed, 5 end of
  input. A theme's receive loop reads on for the 4 and gives its worker up for
  anything else, so collapsing any two of those stops the worker on its first
  idle turn. `-o` **diverts rather than duplicates**: the bytes go to the
  descriptor and the parameter is left unset. A **subscripted** destination is
  filled the way an assignment fills it — `sysread 'buf[$#buf+1]'` appends to
  the string `buf` holds, `'h[k]'` writes the key, `'a[2]'` the element and
  `'span[2,3]'` the span — and `-c`'s count takes a subscript of its own. It
  was refused by name while this shell's assignment path turned a scalar into
  an array (#1746); the path splices now and the name goes through (#1828).
  The refusals that stay are the *shape* of the name — `buf[]`, `buf[(]`,
  `buf[a)b]`, `buf[1][2]` and `buf[\]` are each `not an identifier` at 1 with
  the parameter untouched — while a subscript that will not evaluate, a frozen
  name and a subscript below the first character end the script with the
  **shell's** wording and no `sysread:` in the location.

  `syswrite` writes every byte or reports why it could not, and a descriptor
  that refuses is a status with **nothing said**, which is what lets `while
  syswrite $'\x05'; do …; done` end quietly when the far side goes.

  Two places this does not reproduce zsh, both measured and both filed: a
  numeric `-u` of 10, 20 or `07` is status 0 in zsh with the descriptor then
  unreachable, where every number named here is the number it goes on; and
  `-o nonblock` on a process substitution reads end-of-file where zsh reads the
  command's output, because this shell's `<(cmd)` is a named pipe unlinked when
  the command that named it ends (#1750).

  `zsystem` is `supports` and `flock`. **`supports` answers about what was
  built** — `flock` and `supports` are the whole subcommand vocabulary and
  everything else is 1, *including the module's own builtins' names* — and it
  is a status with nothing printed, which is why a version answering 0 to
  everything would look correct from every angle except a script asking before
  it commits to a lock. Its two operand-count refusals are **255**, the only
  status like it in the module.

  `flock` is a **POSIX record lock**, not flock(2), and that is measured rather
  than chosen: locking one file twice in one shell succeeds both times, and a
  lock does not survive into a forked child. Both are what fcntl does and
  neither is what flock(2) does. A read lock opens the file for reading and a
  write lock for writing, so `-r` works on a file nobody may write. The three
  ways a lock somebody else holds can end are distinct and a caller reads all
  three: `-t 0` is 1 *and says so*, a positive `-t` is 2 **in silence** after
  the wait, and no `-t` at all blocks until the holder lets go. The silence is
  load-bearing — `zsystem flock -t 1 … || fallback` is written inside a prompt,
  and a diagnostic there would put a line on a person's terminal every time the
  lock was busy. `-t` and `-i` are **arithmetic expressions in seconds**, so
  `-t bogus` is a zero timeout (an unset variable is 0) while `-i bogus` is
  refused, because zero is not an interval. Unlocking closes the descriptor,
  which is what releases the lock: a process's record locks on a file all go
  when any descriptor it holds on that file closes, so a shell holding two
  locks on one file gives up both when it unlocks either. zsh closes it too.

  Two divergences, both from this shell's subshells being cloned Runners in one
  process rather than forks. A lock taken in a subshell is the *process's*, so
  the parent is not excluded by it and the parent's lock does not exclude the
  subshell — in zsh a subshell is a fork and both hold. And the descriptor `-f`
  reports is a real entry in this shell's table, so `print -u $fd` reaches it
  here, where zsh keeps its lock descriptors to itself and answers `bad file
  number`.

  **A descriptor opened for reading will not take a write, and says so.**
  `sysopen -u ro f` with no direction letter opens read-only, and
  `print -u $ro x` there is `bad mode on fd 3` at 1 in zsh 5.9.2 — a
  different complaint from `bad file number`, which is what a number
  nothing is open at draws. The mode is read from the refusal rather than
  asked for in advance: this shell's table holds a *stream*, and only the
  write can say whether the file behind it will take one, so an EBADF on a
  number the table does hold is that answer (#1751). It completes the pair
  the direction letters are for — the read-only one cannot be written and
  the write-only one cannot be read — of which only the second half could
  be asserted before.

  `sysseek` is one lseek: `-u` the descriptor and 0 the default, `-w start`,
  `current` or `end` (whole words, case-insensitive, not abbreviated), and an
  offset that is arithmetic — so a word that is not a number is zero. A seek
  the kernel refuses is a **silent 2**, the same vocabulary `syswrite` uses.

  `syserror` is the platform's own sentence for an error number or an `$errnos`
  name, in the platform's own **capitalization**, which is not this dialect's
  elsewhere: `syserror 2` is `No such file or directory` and `sysopen /no/such`
  is `no such file or directory`, both measured in zsh. `-p` prefixes and `-e`
  **diverts** the sentence into a parameter instead of printing it. A word that
  is neither a number nor a name is a silent 2. **The no-operand form refuses
  by name**, which is a deliberate divergence: in zsh it reports the C
  library's `errno` at that instant — measured twice in one session as `No such
  file or directory` and then `Interrupted system call`, so not a behavior a
  shell can be held to — and this shell has no such variable. Answering the
  `Undefined error: 0` that *means nothing went wrong*, at status 0, to a
  script asking what went wrong is the accepting-and-inert failure this module
  has produced twice. A script that has put a number in `ERRNO` gets that
  number's sentence, which is the part that can be answered honestly.

  Corpus: `system/*`.
- **zsh `zsh/zselect`** (dialect/zsh/zselect.go): one builtin, which waits
  until a descriptor is ready and says which one. Measured 2026-09-10 against
  zsh 5.9.2 with `zsh -f`.

  It is the wall behind `sysopen` in the same prompt theme's worker (#1768):
  `zmodload zsh/zselect || return` and `! { zselect -t0 || (( $? != 1 )) } ||
  return` are consecutive lines, and the module was not in the feature table at
  all. The second is a *probe* — a wait of no time on nothing at all, insisting
  the answer is exactly 1 — so a shell answering 0 there fails the guard as
  surely as one with no builtin, and so does one answering 1 with a diagnostic.

  **`-t` is in hundredths of a second**, which nothing else in this shell uses:
  the theme's `zselect -t 1000` is a ten-second wait, and reading the number as
  milliseconds turns its heartbeat loop into a spin. `-r`, `-w` and `-e` name
  descriptors to watch and a **bare number joins the set the last letter
  named**, defaulting to reading. Letters bundle, and one for a set takes the
  rest of its word only when the rest is a number — so `-r0` is `-r 0` and
  `-rt 0` is a read set with a timeout. A dash before a digit is a sign rather
  than an option marker.

  The answer is `$reply`, or the array `-a` names: a set's letter followed by
  every descriptor ready in it, the sets in `r`, `w`, `e` order and ascending
  inside each. `-A` keys an association by the descriptor instead, whose value
  is the letters it was ready in. **Nothing is assigned when nothing was
  ready** — a shell that emptied the array would tell a caller holding a stale
  answer that it now has a fresh one. A descriptor this shell cannot ask the
  kernel about is dropped before the call rather than complained about, because
  the kernel's answer to a set with one dead entry is a complaint about the
  *call*, so every live descriptor beside it would lose its turn. An option
  letter this builtin has not got is `expecting file descriptor`, not `bad
  option`, since a dashed word that is none of the six is read as one that
  should have been a descriptor.

  Corpus: `zselect/*`.
- **zsh `zsh/datetime`** (dialect/zsh/datetime.go): the clock, as a script
  reads it — `$EPOCHSECONDS`, `$EPOCHREALTIME`, `$epochtime` and the
  `strftime` builtin. Measured 2026-09-06 against zsh 5.9.2 with a scratch
  HOME and no startup files.

  All four are implemented rather than refused, because none of them needs a
  seam that does not exist. The three parameters are one clock read each
  through `interp.Runner.Now`, which is the hook `printf '%(fmt)T'` already
  reads — so an embedder that pins the clock pins these — and they are
  **produced, not stored**, for the reason `$SECONDS` is: a clock read once is
  wrong from the instant afterwards, and silently, because the caller still
  gets a number. `strftime` is a formatter over the same format language
  `printf '%(fmt)T'` writes and calls `interp.Strftime` rather than carrying a
  second copy of it.

  All three parameters are **readonly**, which zsh's own
  `${(t)EPOCHSECONDS}` also says — `integer-readonly-hide-hideval-special` —
  and it is load-bearing rather than decoration: a produced parameter a
  script can assign to is shadowed by the assignment from then on, so it
  would stop tracking the clock and never say so. They appear in `readonly`
  and `readonly -p` as bare names, exactly as zsh's do; zsh writes a kind
  letter with the readonly one — `-ir`, `-Fr`, `-ar` — and this shell writes
  `-r` alone, because a produced parameter has no integer or float attribute
  here to show.

  They are **not** also marked hidden, though zsh's are. `MarkHidden` keeps a
  produced *table* out of a listing, which is why the `builtins` association
  needs it; a produced scalar or array is in none of the tables a listing
  walks, so readonly is what puts it there — as a bare name with no value,
  which is what zsh writes — and hiding it changes nothing observable.
  Measured against a build with the call in: `readonly`, `readonly -p`,
  `typeset` and `typeset -p` are byte-identical either way, and the one
  listing that would tell them apart is `typeset -H`, which this shell has
  not got.

  `$EPOCHREALTIME` carries **ten decimal places**, which is `typeset -F`'s
  default precision, and the last of them are the rounding a float64 of a
  nanosecond epoch gives — the same digits a real zsh shows. The count is a
  dialect answer and not a shape: bash has its own `$EPOCHREALTIME` from 5.0
  and writes **six**, and bash 3.2, ksh93 and dash have no such parameter.
  This shell's bash has neither, which is recorded rather than fixed here.

  `strftime [-n] [-r] [-s scalar] format [seconds [nanoseconds]]`. The
  letters cluster, so `-rs v` is `-r -s v`, and `-s`'s name is the rest of the
  word or the next word whatever it looks like — `strftime -s -n …` is
  `not an identifier: -n` rather than two options. `-n` drops the trailing
  newline, `-s` writes nothing and assigns instead, no epoch operand at all is
  the clock, and a third operand is nanoseconds. Five conversions go beyond
  POSIX: `%N` the nanoseconds in nine digits, `%.` the fraction in three or
  `%<n>.` in n of them **rounded**, and `%f`, `%K` and `%L` the unpadded
  spellings of `%d`, `%H` and `%I`. A conversion nothing knows keeps its
  letter and loses the `%`, which is the C library's answer rather than the
  shell's.

  `-r` reads a time back out of a string, which is the direction
  `lib/zsh/install.zsh` uses on an HTTP `Last-Modified` header. **An unnamed
  field is the start of 1900**, not today — measured, `-r "%H:%M" 1:2` is
  -2208985080 — and that is the answer to keep, because the alternative
  silently invents a year. Fewer digits than a field's width still match, a
  run of whitespace in the format matches any run in the input, and input left
  over is a **warning** at status 0 with the seconds still written, rather
  than a refusal that would lose a header that read perfectly well.

  Every complaint carries the builtin's own name in the location —
  `<file>:strftime:2:` — and none of them is fatal: status 1 and the script
  carries on. `not enough arguments`, `too many arguments` and
  `bad option: -Q` are counted before any operand is looked at;
  `not an identifier: 1bad`, `abc: invalid argument` and `format not matched`
  are about the operands. A conversion `-r` has not got is **not** `format not
  matched` — that would send a script looking at its data for a shortfall that
  is this shell's — but `-r: %V is not implemented yet`. Corpus:
  `datetime/*`, `zmodload/loading-the-clock-module`.
- **zsh `autoload`** (dialect/zsh/autoload.go): a name defined from
  `$fpath` the first time it is called. Measured 2026-09-06 under `env -i`
  with a scratch HOME and no startup files.

  **The name becomes a function immediately**, before anything is read:
  `autoload -Uz myfunc` is silent and 0, and `whence -w myfunc` answers
  `myfunc: function` at once. The stub *is* the record, which is why
  nothing keeps a separate list of pending names. The **call** is what
  searches `$fpath`, and a name whose file is not there fails at the call
  and not at the declaration — a shell that resolved early would report
  the failure in the wrong place, and a script's `autoload || return`
  would fire when nothing was wrong yet. The file's contents become the
  function's *body*, so its `$*` is the call's arguments and a second call
  runs the loaded body rather than searching again.

  `$fpath` is walked in order and the first readable file wins; an entry
  with no such file, or one that cannot be read at all, is a search that
  goes on rather than a failure, which is what makes a stale entry
  harmless. A name with a directory in it is read from where it says and
  `$fpath` plays no part.

  **A name that is already a function is left alone.** Measured 2026-09-07:
  with a file for `cfn` on `$fpath` and `cfn` already defined,
  `autoload -Uz cfn` is 0, the body survives, and the name is absent from
  the bare listing afterwards — so the declaration is a no-op rather than a
  re-marking. `+X` over such a name refuses instead of resolving: status 1,
  and nothing on either stream. Only the plus sign asks this. `-X` is the
  opposite case by construction — the function it replaces is the one it is
  running inside, which always has a body — so the two signs cannot share
  the guard, and a guard that was shared refused every autoload the moment
  the generated stub called it.

  It matters more than the corner suggests, and it is the second reason a
  stock function fails to autoload. A real startup file declares a name it
  may already have — a plugin manager writes `builtin autoload -Uz
  is-at-least` and runs the line again on every reload — and a shell that
  wrote its stub over the definition turned a working function into
  `function definition file not found` at the *next* call. The stub is the
  record, so writing one over a real body is not a note about the name, it
  is losing it. Corpus: `autoload/an-existing-function-is-left-alone`,
  `autoload/resolve-now-refuses-an-existing-function`.

  **`$fpath` is not empty at startup, and what is on it is the front
  end's.** Measured 2026-09-07 against zsh 5.9.2 under `env -i` with a
  scratch HOME and `-f`, so no startup file is speaking: that shell arrives
  with three directories on it, all three of them belonging to *that
  installation* — its own function library plus the two site directories
  third-party packages install into. An `FPATH` in the environment replaces
  the lot rather than adding to it, and does so even when it is the empty
  string, so the default is a fallback for a name the environment does not
  mention rather than for one it leaves blank.

  Which directories is a fact about where a shell was installed, and no
  dialect may hold one: the same machine's other zsh — Apple's 5.9 at
  `/bin/zsh` — answers a different prefix *and* a different layout, with a
  version segment under `share/zsh` where the Homebrew build has none. Two
  builds of one shell on one machine, so there is no expression that
  derives one installation's directories from another's, and a table of
  paths would be the recording machine's rather than any machine's.

  So the dialect names the parameter — `Semantics.FunctionSearchVariable`,
  `FPATH` here and empty in the other three — and the **front end** fills
  it with this installation's own directories, read off where the running
  binary sits: `<prefix>/share/sh/site-functions` then
  `<prefix>/share/sh/functions`, site first, which is the order the shell
  being imitated has and the useful one. `<prefix>` comes from the binary's
  own resolved path, recognizing the `libexec/sh` layout `make install` and
  the Homebrew formula both use and falling back to the parent of whatever
  directory the binary is in. Nothing is stat-ed: an entry that is not there
  is carried anyway, which is what the shell being imitated does with
  `/usr/local/share/zsh/site-functions` on a machine that has no such
  directory. Corpus: `tie/FPATH-arrives-with-a-value`,
  `tie/an-FPATH-in-the-environment-replaces-the-default`,
  `tie/an-empty-FPATH-in-the-environment-suppresses-the-default`.

  What this deliberately does **not** do is point the search at another
  shell's installed function library. It is reachable — a person puts one on
  `FPATH` — and it is the only thing that makes a stock `add-zsh-hook` or
  `is-at-least` load today, because this installation ships no functions of
  its own yet. It is not the default: the directories cannot be found
  portably, and which library a shell reads is a decision about what that
  shell *is* rather than a default to arrive at by derivation. A name that
  cannot be found still refuses by name, which says more than a silent
  empty does (#1250).

  **`+X` and `-X` are two commands rather than one letter with a sign.**
  `+X name…` resolves the names given and does not run them; `+X` with
  nothing is silence and 0. `-X` takes *no* name — it means "the function
  I am running inside" — so `autoload -X` at the top level and
  `autoload -X foo` anywhere are both `bad autoload`. zsh ends the script
  there and this shell reports and runs on: a dialect builtin has no way
  to say "and stop" that this engine offers, and inventing one for a
  spelling only the shell's own generated stub ever writes would be more
  surface than the corner earns.

  The one place this shows its own workings is `typeset -f NAME` on a name
  that has not been called yet, and the stub written there is zsh's own:
  `builtin autoload -XUz`, the same builtin called with no name, acting on
  the function it is running inside. It was `builtin autoload +X NAME &&
  NAME "$@"` — a re-entry written out longhand — which behaved correctly
  and read as something no zsh ever wrote, and the thing that reads a stub
  back is a *script* (#1697).

  `-X` acts on the **innermost** function, and what happens after the
  replacement is the difference between the two spellings that reach it.

  **A generated stub is replaced, not called through** (#1842). Calling an
  autoloaded name opens a frame, the stub's one line runs, and the loaded
  body then runs *in that frame* — so the first call is at the same depth as
  every call after it. Measured on zsh 5.9.2 with one file on `$fpath`:

      fpath=(fns); autoload -Uz fstk; fstk a
        first call   n=1 stack=fstk      second call   n=1 stack=fstk

  where a nested call answers `n=2 stack=fstk fstk` on the first and agrees
  on every one after. That is a frame that is there on the cold call and gone
  afterwards, and `$funcstack` is read by real prompt and completion code to
  decide where it is. The same seam shows from the other side in a
  diagnostic: zsh locates one raised in the loaded body at `fstk:1:` on both
  calls, where the nested arrangement wrote `fstk:builtin:1:` on the first,
  the builtin that made the call still being on the stack.
  `interp.Runner.RunFunctionBodyInPlace` is the seam — no frame, no scope, no
  added recursion depth, and the builtin's name out of the way so the body
  speaks for the function.

  **A hand-written `-X` still nests**, because there the stub is a function
  the script really wrote. Measured: `${#funcstack}` counts more than one
  inside the loaded body, the body sees the stub's locals, and the stub
  carries on afterwards with the body's status in `$?`. The two are told
  apart by whether the name is a stub `autoload` generated and nothing has
  defined since — asked *before* the resolution, which takes the stub's body
  away.

  Letters: zsh has `d k m r R t T U w W X z`, measured a letter at a time
  against all fifty-two, and refuses every other as `bad option`. `-z`
  (zsh-style parsing) is accepted and changes nothing here — an autoloaded
  file is parsed with this shell's own grammar, which *is* zsh's. `-U`
  **suppresses alias expansion while the file is read**, and it is acted
  on rather than only recorded. Measured 2026-09-11 on zsh 5.9.2 with a
  file for `af` holding the one word `myalias` and
  `alias myalias='print -r -- X'` defined before the call:

      autoload -Uz af    af:1: command not found: myalias
      autoload -z  af    X
      autoload     af    X

  Every declaration a real startup writes is `-Uz`, which is the column we
  already matched and the whole reason a shell that behaved as though the
  letter were always given went unnoticed (#1993).

  What decides it for a file with no `-U` is this shell's **`aliases`
  option**, not the route the program arrived by. Measured under `env -i`
  with `-f`: `zsh -c 'alias myalias=…; autoload af; af'` prints `X`, where
  the same invocation leaves an alias written in its own command string
  alone. `unsetopt aliases` leaves the file unexpanded on both routes. The
  table read is the one live **at the call**, which is where the file is
  read — an `unalias` between the declaration and the call takes the
  expansion away. Corpus:
  `autoload/a-function-file-expands-aliases` and
  `autoload/a-function-file-with-minus-u-does-not`.

  The other nine are named as missing: per-function
  tracing (`-t`/`-T`), the ksh-style and pattern forms (`-d`/`-k`/`-m`),
  resolving the path now (`-r`/`-R`) and compiled `.zwc` files
  (`-w`/`-W`).

  A file that is found but is not a body this shell can read gets its own
  complaint — `NAME: bad function definition` — rather than "not found",
  which would send somebody looking for a file that is right there. zsh
  reports the parse error itself, `badfn:1: parse error near 'fi'`; the
  status is the same and the reason is not said here.

  ksh93 has the word too — `autoload` is `typeset -fu` there — which is
  why its corpus column is a `typeset` usage line rather than a
  not-found; bash and dash have no such builtin. The seam this needed is
  `interp.Runner.DefineFunction`, the write half of `FunctionText`: a
  definition arriving from outside the script rather than from a
  `f() { … }` the parser already read. It has a second spelling for the
  alias question — `DefineFunctionExpandingAliases` — and both, with
  `DefineFunctionFromText` beside them, are one implementation: three ways
  of reading a body out of text is three places for the next fix to reach
  two of, which is exactly how #1993 came to need fixing twice. Corpus:
  `autoload/*`.
- **zsh `bindkey`** (dialect/zsh/bindkey.go): the line editor's key table.
  See below for why this moved out of the not-built list.
- **zsh `zle`** (dialect/zsh/zle.go): defining an editing action in shell,
  and running one. The other half of `bindkey`, and it moved off the
  not-built list for the same reason and by the same rule — see below.
- **zsh `sched`** (dialect/zsh/sched.go): a command line put aside until a
  time. Unrelated to the line editor despite arriving with it; see below.
- **bash `bind`** (dialect/bash/bind.go): the same key table under
  readline's names, and the two `set -o` editing modes with it. See below
  for why this moved out of the not-built list, and for the two measured
  findings that make it a different builtin from `bindkey` rather than a
  translation of one.

Deliberately **not** built, so the next sweep counts each as scoped rather
than missing:

- zsh `zmodload`: there are no loadable modules here; the name would be a
  table of refusals.
- zsh `autoload` (and `fpath`): function-file loading is an interactive
  startup mechanism; a non-interactive core sources files by name.
- zsh `vared`: the line editor as a callable, editing a variable's value
  rather than a command line. `zle` was on this list beside it and has come
  off — see below — but `vared` needs something else again: the editor
  entered from inside a running command, with the line it is holding
  belonging to a parameter. `zcompile` is on this list too, further down.
- zsh `zle -F`, `zle -R`, `zle -M` and `zle reset-prompt`: the descriptor
  callback and the three spellings of redisplay. `zle` itself is built (see
  below) and these refuse by name inside it, which is the whole point of
  having built it: a `zle` that accepted everything would be worse than the
  `command not found` it replaced, because a plugin would then believe its
  widget existed. `-F` is the one that matters — it is how a plugin in this
  shell does asynchrony, and #1320 records that without it there is no async
  in a zsh theme at all. It needs the read loop to wait on more than the
  terminal, which is a change to how a key is read rather than an addition
  beside it.
- zsh `zle <one of the editor's own actions>` from inside a widget — `zle
  end-of-line`. The name resolves; what it would take is a shell function
  reaching back into the editor mid-keystroke, which is re-entering the read
  loop rather than transforming the line. It refuses by name. `repl` names the *actions* — a Widget
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

  **bash `bind` is the same layer with a different vocabulary, and two
  measured facts make it a different builtin rather than a translation.**

  The first is that **the two argument forms are not one form.** `bind
  'keyseq:function'` reads its left side as the name of a *single key* —
  `\C-l`, `Control-l`, `DEL`, `x` — and `bind '"keyseq": function'`, with
  double quotes inside the word, reads it as a whole escape sequence.
  Measured under a pseudo-terminal against bash 5.3.15:

  | written | what it binds |
  | --- | --- |
  | `bind "\C-l":clear-screen` | `^L` — a valid key name |
  | `bind "\C-x\C-a":beginning-of-line` | **nothing**, silently, at status 0 |
  | `bind '"\C-x\C-a": beginning-of-line'` | `^X^A` |
  | `bind "\e[Z":beginning-of-line` | a backslash, silently, at status 0 |

  So an implementation reading the second row as two bytes would bind a key
  real bash leaves alone. The three `bind -m vi-insert "\C-l":clear-screen`
  lines that opened #1352 are all the first row's shape, which is why they
  work in bash and had to be made to work here.

  The second is that **an unknown function name is not stored** — the
  opposite of zsh. `bind '"^X^T": no-such-widget'` is status 0 and silence
  and `bind -p` has no row for it, so the key goes on doing what it did;
  zsh stores its unknown name and the key stops working. Both were measured
  the same way. Asking about the name is a different question with a
  different answer: `bind -q no-such` is `unknown function name` at 1.

  What it does not do, on the same rule `bindkey` follows: **`bind -l` names
  this editor's functions and not readline's 176.** `kill-whole-line` is
  deliberately absent, because this editor's `^U` takes the line back to its
  start — which is readline's `unix-line-discard`, a name that *is* offered
  — where readline's `kill-whole-line` takes the whole line and is bound to
  no key in bash 5.3. Offering it would be offering a name that does
  something else. `bind -s` stores and lists a key bound to text and the key
  is inert, which is the same partial `bindkey -s` keeps.

  **The default listing is not a third copy of the editor's keys.** Both
  builtins derive it from `repl.DefaultBindings`, whose test types every key
  in the table and compares the accepted line against the same widget
  reached through the override layer — so an entry naming the wrong action,
  or naming a key the dispatch ignores, fails. The one hand-written copy
  that preceded it had already drifted: it was missing `M-^H`, which kills a
  word back, and all four numbered spellings of Home and End, so `bindkey`
  reported five keys as unbound in a shell where pressing them works.

  **`set -o vi` and `set -o emacs` are one state with three values.**
  Measured in bash 5.3 and ksh93 alike: `set -o vi` turns `emacs` off in the
  same breath, and `set +o vi` afterwards leaves **both** off rather than
  putting `emacs` back — so "neither" is a state a script can observe and a
  bool could not hold. Turning one on is the only way back. What the mode
  selects here is which keymap `bind` acts on without `-m`: `set -o vi`
  makes `vi-insert` current, measured, which is what makes an rc file's `set
  -o vi` followed by `bind -m vi-insert` do what it says. It now selects one
  thing more: whether Escape leaves insert mode for the **command mode**,
  which repl gained in #1427 and both dialects reach — `docs/spec/editing.md`
  has the measured key table. The other shell asks for the same mode a second
  way that leaves this option alone (`bindkey -v`, measured), which is why
  what repl reads is a dialect's answer rather than this state.

  **Nothing is selected until something selects it** (#1858). A
  non-interactive bash reports both `vi` and `emacs` off, and so does this
  shell now; under `-i` bash reports `emacs on` and so does this one, which
  is `InteractiveSelectsEmacs` and the only dialect answering yes to it.
  Reporting `emacs` on in a script had been deliberate — the editor really
  does read `^A`, `^E` and `^B` — but it was the wrong kind of honesty: a
  script has no line to edit, and what the mode selects is a keymap. The
  zero value is a fourth state, "not chosen yet", because "chosen off" reads
  differently in exactly one place: `bash -i -c 'set +o emacs; set -o'`
  reports `emacs off` where `bash -i -c 'set -o'` reports it on.

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
- zsh `setopt` names of the **recorded** kind: 141 of the 185 are recognized,
  remembered and reported without being acted on. See "zsh's option names".
  (This line read 157 while the table above read 150; neither was the count
  the table produces. It is now counted from the constructors.)
- zsh `emulate csh`: the mode is recorded and nothing changes with it —
  csh's differences are not modeled anywhere else either. (`emulate -L` was
  on this list, refused for want of a restore-on-return seam. The seam is
  `Runner.AtEveryFunctionCall` and the rule is `LOCAL_OPTIONS`, which is what
  the letter turns out to be; see dialect/zsh/localoptions.go.)
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
- bash `bind -x` and `-X`, and `-v`, `-V` and `-f`: running a shell command
  from a key, and readline's own variables. `bind` itself is built (see
  below) and these refuse by name inside it, for the same reason `zle`'s
  missing halves do. `-x` is the one with machinery already behind it —
  `repl.Binding.Function` and `driver.Shell.RunWidget` carry a name this
  package does not look inside, so the command text could ride that seam —
  and what is missing is bash's own half: what `READLINE_LINE` and
  `READLINE_POINT` are while the command runs, and what happens to the line
  when it changes them, are unmeasured. `-v` and `-V` would be reporting
  settings nothing here reads.
- bash `bind -m emacs-meta` and `-m emacs-ctlx`: readline's two **prefix**
  keymaps. This editor reads a key sequence whole rather than through a
  prefix map, so a binding recorded in one could never fire; refused as not
  implemented rather than as an invalid keymap name, since bash really has
  them. It is the same refusal `bindkey -p` makes, for the same reason.
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
      zsh    aAfFgHilpruUTx
                           plus floats, padding, namerefs…, unimplemented;
                           -F taken in silence, see below
      ksh93  aAilprux      plus -f -F -b -n… and its own -H, unimplemented
      dash   —             no typeset at all
    local
      bash   aAgilprux     plus -f -F -I -n -t, unimplemented
      zsh    aAHilpruUTx
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
not the zsh one under another spelling, and modeling the two as one
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

**`-T` ties a scalar to an array, and it is one shell's letter with a
second shell's letter spelled the same.** Measured 2026-09-06 against zsh
5.9.2. `typeset -T SCALAR array [sep]` makes two names for one value:
writing the scalar splits it on the separator and the array becomes those
fields, writing the array joins them and the scalar becomes that string.
`SCA=a:b:c` is three elements; `sca=(x y z)` is `x:y:z`; `sca+=(w)` is
`x:y:z:w`; `sca[1]=Z` is `Z:y:z:w`. The separator is the third operand and
defaults to `:` — with `#`, `S2=a#b#c` is three fields.

**`unset` of either name unsets both**, and forgets the tie: `unset SCA`
leaves `sca` unset rather than merely empty, and a later `SCA=c:d` is a
plain scalar with `${#sca}` still 0. Half a tie is not a state this shell
has.

The declaration follows the rule every other attribute follows — a name
that already holds a value keeps it and is read back through what has just
arrived, so `V=one:two; typeset -T V v` fills `v` rather than emptying
both. That is the rule that makes a tie over `PATH` safe. A *valueless*
declaration leaves the scalar set and empty and the array with **no**
elements, which is not the same as `SCA=''`: the empty string splits into
one field and a declaration splits nothing.

The other letters go to the halves they belong to. `-U` and `-r` to both —
`typeset -TU A a` lists as `export -UT` and `typeset -aUT`, `-r` as `-rT`
and `-arT` — and **export to the scalar alone**, because the scalar is what
a child can be told. `T` is written last of all the letters, after `U`.

Five refusals, and their fatality is **not** one rule: `-T requires names
of scalar and array`, `can't tie already tied scalar: S` and `second
argument of tie must be array: s` are reported and the script runs on,
while `can't tie a variable to itself: A` and a half that is not a name at
all end it. The last of those goes through the refusal every declaration
operand goes through, so it takes `BadNameToDeclarationFatal` rather than a
rule of its own — and that check earns its place twice over, because this
parser lifts an array literal out of the operand list and appends its bare
name, so `typeset -T R r=(a b) ':'` arrives as `R`, `:`, `r` and a
positional reading would tie `R` to `:`.

`typeset -T` with nothing to tie lists the ties, both halves of each, as
plain assignments — `A=1:2` and `a=( 1 2 )`, not `typeset -T A a=( 1 2 )`.
Not quite the bare listing either, which in this shell writes scalars
alone.

There is no axis. bash refuses `-T` under both spellings with its usage
line and 2. **ksh93 is the column that makes this a letter rather than an
attribute**: `typeset -T tname` declares a *type* there, so it takes
`typeset -T TS ts` without a word and leaves `ts` empty — the same letter,
silently doing something else, which is why it stays refused by name there
rather than being modeled as one attribute with two readings. So the
letter lives in `Semantics.DeclareOptions` and `LocalOptions`, and the
mechanism is `interp/tiedscalar.go`: a pair recorded under both names, and
each of the two choke points — `setVarAs` and `storeArray` — mirroring
into the other under a re-entrancy guard, because the mirrors would
otherwise call each other forever.

**The eight ties this shell arrives with** are made from the same
machinery, before a script says anything — `dialect/zsh/builtintie.go`,
through `interp.Runner.Tie`, which is `typeset -T` without the flags, the
scope or the refusals a builtin needs. Measured under `env -i` with a
scratch HOME and no startup files, one pair at a time:

    export -T PATH path            typeset -T MANPATH manpath
    typeset -T FPATH fpath         typeset -T MAILPATH mailpath
    typeset -T CDPATH cdpath       typeset -T MODULE_PATH module_path
    typeset -T PSVAR psvar         typeset -T FIGNORE fignore

Every one of them joined on `:`. This is what makes `$path` an array at
all, and what makes `path=( /new "${path[@]}" )` — the line every rc file
in the world writes — reach `PATH` and the command lookup that follows it.
Before, it wrote an ordinary array nothing read, **silently**.

**The export attribute is inherited, never conferred.** `PATH` lists as
`export -T` when the environment supplied it and as plain `typeset -T`
when it did not — measured both ways under `env -i` — and writing
`cdpath` never puts `CDPATH` into a child's environment. So the tie
carries whatever the scalar already was.

Two things are deliberately absent. `ZSH_EVAL_CONTEXT`/
`zsh_eval_context` is listed among that shell's ties but is a *produced*
parameter — `typeset -p ZSH_EVAL_CONTEXT` writes nothing there — and a
produced parameter is a different mechanism. And **no default value**:
zsh fills `FPATH` with its own function directories and `MODULE_PATH`
with its module directory when the environment names neither, and those
are that installation's files. Inventing them here would point this
shell's `autoload` at another shell's function library, so a pair the
environment says nothing about starts empty — which is what the same
shell does for `CDPATH`.

**A `local` of one half of a tie has two answers, and which one it gets
depends on who made the tie.** The shell's own pairs are *special*
parameters — the tie belongs to the name — so a `local` of either half
displaces **both** and stays tied: `f() { local PATH=/x; }` moves `path`
with it for the duration and hands the caller back both `PATH` and
`path`. `compaudit` opens with `local -a -U +h fpath` and is handed a
fresh, empty array and an emptied `FPATH` beside it; the caller's entries
in view mean the code deciding whether the completion directories are
secure audits the caller's search path rather than a copy of it.
Each half is emptied in its own kind, which is why the counts differ by
one: `local PATH` sets an empty string and the mirror splits it into the
single field it has, where `local path` sets no elements and the mirror
joins them into nothing.

A tie a *script* made with `typeset -T` is a property of the parameter,
and `local` makes a new parameter — so it is an ordinary, untied local
and the other half goes on naming the outer cell: `typeset -T S s;
S=a:b:c; f() { local S=zzz; }` leaves `$#s` at 3 inside `f`. The `+h`
letter does not tie it back, which is what says this is not that
attribute wearing another name. Only a scope *deeper* than the one a tie
was made in suspends it, so `typeset -T` inside a function does not turn
off the tie it is making with its own shadow. `interp/tielocal.go`.

Corpus: `declare/tie-*`,
`declare/unsetting-half-a-tie-unsets-all-of-it`,
`declare/a-tie-over-a-standing-value`, `declare/tying-a-name-to-itself`,
`declare/local-of-half-a-built-in-tie-shadows-the-other-half`,
`declare/local-of-a-built-in-ties-array-half-is-a-fresh-array`,
`declare/local-of-half-a-script-tie-is-an-ordinary-local`, `tie/*`.

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

**A declaration may shadow a frozen name in one shell and not the
other** — `Semantics.DeclarationMayShadowAReadonly`. Measured 2026-09-06,
`env -i PATH=/usr/bin:/bin` with a scratch `HOME`, `ZDOTDIR` and
`HISTFILE`, over a script file:

    typeset -r x=1
    f() { local x=2; echo "in=[$x]"; echo running; }
    f; echo "st=$? out=[$x]"

    zsh    in=[2]   running   st=0 out=[1]
    bash   local: x: readonly variable   in=[1]   running   st=0 out=[1]

ksh93 has no `local` and dash has no `typeset -r`, so bash and zsh are
the two shells that can be asked and they disagree. **One field for the
whole family**, not one per spelling: zsh takes `local x=2`, `local x`,
`typeset x=3`, `local -r x=4` and `local y=1 x=5 z=2` alike, and bash
refuses every one of them and reports 1 from the builtin each time.
Splitting them would have been five fields whose answers can only ever
agree.

Where the answer is **yes** the shadow takes the attribute with it: the
local cell is writable, and the outer name is **frozen again** when the
function returns. That last part is only visible from outside the
function and is what a shadow that merely cleared the attribute would get
wrong — a readonly quietly thawed by a function call is a hole in the
whole point of it.

Where the answer is **no**, three things follow and all three were wrong
here:

- the refusal **names the builtin the script wrote** — `local: x:
  readonly variable`, which is `Diagnostics.ReadonlyVariableInDeclaration`
  and why `ReadonlyRefusalNamesBuiltin` now has a `local` entry alongside
  `declare` and `typeset`;
- the builtin **reports 1 and nothing is abandoned**: the rest of the
  function runs, so `local x=2 || echo fired` prints `fired` and the next
  line still runs. Ours gave up the function and reported 1 from it;
- the **other operands are still declared**. `local y=1 x=5 z=2` over a
  frozen `x` leaves `y` and `z` local in bash and refuses only `x`; a
  refusal that gave up the command would have left both assigned
  globally, which is a change to the caller's variables that nothing
  said.

**The valueless spelling is the one that used to slip past.** With
nothing to assign there was no assignment to meet the refusal, so
`local x` over a frozen name made the local in silence and the body saw
an empty name where bash shows it the frozen value. Asked before the
shadow rather than through the assignment — `Runner.declarationShadowRefused`
— which is what makes the five spellings one answer.

Asked only where a declaration meets a name that is *already* frozen, so
an ordinary `local` never reaches it, and a frozen name assigned at top
level is the plain readonly refusal and no part of this.

**Ours was fatal in the zsh dialect where zsh carries on**, which for an
interactive shell was the whole of it: `local EPOCHSECONDS=…` or
`local builtins=…` in a function ended the session. `zi.zsh:2159` is
`[[ $1 = burst ]] && local -h EPOCHSECONDS=$(( EPOCHSECONDS+10000 ))`,
exactly that shape. It reaches two named refusals now — `local -h` and
an association assigned whole — rather than a dead session.

Corpus: `declare/a-local-that-shadows-a-readonly`,
`declare/a-valueless-local-that-shadows-a-readonly`,
`declare/a-refused-shadow-keeps-the-other-operands`,
`declare/a-local-that-shadows-a-readonly-and-the-name-afterwards`.

**`set -A name value …` assigns an array through a name a variable
holds**, which is the thing `name=(…)` cannot do: the name is a literal
there, so a script that holds it in a variable has no other spelling.
ksh93 and zsh have the letter; bash calls it an invalid option and
prints a usage block, dash calls it illegal and stops. So it is a
dialect's answer — `Semantics.SetArrayLetter` — and that field is
**read rather than asked**: where the answer is not yes the word is
somebody else's invalid option, and the refusal already in place is
that shell's own. Asking an axis there would replace a correct answer
with a complaint about a missing dialect.

It matters because zsh's own `add-zsh-hook` installs a hook with it:

    typeset -ga $hook
    set -A $hook ${(P)hook} $fn

The assignment is the same whole-array store `name=(…)` reaches, so the
array base, the unique attribute, the scalar view, a tied scalar and the
readonly refusal are all answered once. What is new is only the parse: a
letter that takes an operand, and which of the words behind it are
values. Measured 2026-09-06, `env -i PATH=/usr/bin:/bin` with a scratch
`HOME`, `ZDOTDIR` and `HISTFILE`, over a script file.

**The plus form replaces from the front** and leaves the rest of the
array standing, which is a different operation rather than the same one
and is the half a single implementation gets wrong:

    set -A b 1 2 3 4 5; set +A b Q R      [Q R 3 4 5]  — both shells
    set -A c 1 2 3; set +A c              [1 2 3]      — both shells

Unanimous, so no field. The *minus* form's answer to the same emptiness
is not: **`Semantics.SetArrayWithNoValuesUnsetsTheName`** — ksh93 unsets
the name and zsh leaves an array with no elements. Both count 0, so the
difference reaches a script only through `${a+x}` and a listing, which is
what makes it a field: a script asking whether the name is set at all
gets opposite answers. `typeset -p a` says the same from the other side,
nothing at all against `typeset -a a=(  )`.

**Whether the options carry on past the name** —
`Semantics.SetArrayOptionsContinuePastTheName`:

    set -A ff -x -y      ksh93 `-y: unknown option`   zsh [-x -y]
    set -A dd -- 1 2     ksh93 [1 2]                  zsh [-- 1 2]

ksh93 keeps parsing, so the values are exactly the words that would have
become the positional parameters and a `--` among them still ends the
options; zsh stops at the name and every word behind it is a value.
**One question, not two** — whether `--` is an operand *is* whether
options are still being read, so both rows move together.

The positional parameters are left alone either way: `set -- one two
three; set -A a x y` keeps all three in both shells, so the letter's
other meaning is never reached.

**The refusals are where the two shells are furthest apart, and the
location is the finding.** The name operand goes through the check every
other builtin's operands go through, fatality included — both shells end
the script over a bad name — and:

- zsh does **not** name the builtin in the location for `set -A 1v q`
  where `unset 1x` and `typeset 1w` from the same shell are
  `<script>:unset:1:` and `<script>:typeset:1:`:
  `Diagnostics.BadNameRefusalHidesTheBuiltin`.
- ksh93 **does** name it for a frozen name, and keeps its *bracketed*
  builtin location under it — `<script>[2]: set: ro: is read only`
  against `<script>: line 2: ro: is read only` for a plain `ro=(x y)`.
  One table decides both, because a dialect that puts the name in the
  sentence is the one that keeps the builtin's location:
  `Diagnostics.ReadonlyRefusalNamesBuiltin`.
- `set -A` with nothing after it is a refusal in ksh93 —
  `Diagnostics.SetArrayNeedsAName`, `set: -A: name argument expected`
  with set's usage under it — and a *listing* of every array in zsh,
  which is not built and is named as missing.

**Three things are refused by name rather than built**: that listing, a
subscripted name — both shells place the values from that subscript on,
and the placement reads the array base, which is the whole of what could
go wrong in silence — and an association, which is a different operation
under the same spelling that the two shells do not agree about: zsh reads
the operands as key-and-value pairs, `set -A m k1 v1 k2 v2` being two
entries, where ksh93 stores four counted elements. Picking either would
answer the other shell's script wrongly and say nothing about it.

Corpus: `setarray/*`.

**`integer` is the declaration under a third name**, and which names a
shell has is a dialect's answer rather than an axis for the third time:
dash has none of them, ksh93 has `typeset` and `integer`, bash has
`typeset` and `declare`, and zsh has all three. Measured 2026-09-06,
`env -i PATH=/usr/bin:/bin` with a scratch `HOME`, `ZDOTDIR` and
`HISTFILE`, over a script file.

    integer n=3            ksh93, zsh  st=0 n=3
                           bash, dash  command not found
    integer -r r=5         ksh93, zsh  st=0 r=5

It matters because zsh's own `add-zsh-hook` declares with it before it
does anything else: run that function under a shell with no `integer`
and it fails on its opening declaration, so such a shell cannot install
a precmd hook — which is the whole of what a zsh startup file does with
the function (#1155). Recorded from **running** the function rather than
from reading it; `CLEANROOM.md` has why the difference is load-bearing,
and #2167 has what this paragraph used to say.

The attribute, the arithmetic a later assignment means, the function
shadow, the readonly refusal and every letter's meaning are the
declaration's, not a second implementation's: `interp/integerbuiltin.go`
registers `Runner.declareNames` under the second name. Three things the
name decides for itself, and each is a table of values:

**Its letters are narrower than the declaration's, and narrowed
differently.** zsh's `integer` refuses `-a`, `-A`, `-f`, `-F`, `-T` and
`-U` as bad options where its own `typeset` takes all six; ksh93 hands
`integer` the whole typeset grammar and takes every one of them.
`Semantics.IntegerOptions`, empty where the shell has no such word — so
a shell that reused `DeclareOptions` would accept `integer -A m`, which
is an associative array in neither shell.

**A plus word on `integer` removes nothing in ksh93 and everything in
zsh** — `Semantics.IntegerPlusFormTakesAttributesOff`:

    integer n=5; integer +i n; n=3+4      zsh [3+4]   ksh93 [7]
    integer -x e=1; integer +x e          zsh gone    ksh93 exported
    integer n=5; typeset -p n             zsh typeset n=5
                                          ksh93 typeset -l -i n=5

zsh prepends the letter to an ordinary declaration, so every plus form
means there what it means on `typeset`; ksh93 has a declaration command
whose type the word itself fixes, and a plus form reaches neither the
type nor the export. **One question about the word rather than one per
letter**: both rows move together, and ksh93's own `typeset +x` *does*
unexport, so this is not the letter's answer being asked twice. Corpus:
`declare/integer-and-a-plus-form`.

**`-i` takes an output base in the two shells that have `integer`** —
`Semantics.IntegerAttributeTakesABase`:

    integer -i 16 b=255      ksh93 [16#ff]   zsh [16#FF]
    typeset -i2 c=5          ksh93, zsh [2#101]
    typeset -i 16 b=255      bash `16': not a valid identifier, st=1
    typeset -i2 c=5          bash -2: invalid option, st=2

Asked only where a base is actually written, so an ordinary `-i` never
meets it and a dialect whose `-i` takes none — bash — is left alone;
there the word after the letter is an operand and `-i16` is somebody
else's bad option.

**The rendered text is what is stored**, not a way of printing what is,
and this is the row the whole feature has to be built around:

    typeset -i16 h=255
    ${#h}         5              five characters, not three
    g=$h          g is 16#ff     a plain copy copies the text
    $(( h + 1 ))  256            arithmetic parses it back
    export h; env h=16#ff        the child is told the text

A shell that merely printed the name differently would answer the length
3, the copy `255` and the child `h=255`. ksh93 and zsh agree on all four,
differing only in the case of the digits.

**The base belongs to the name** and not to the assignment that met it:
`typeset -i8 c; c=64` is `8#100`, a second declaration re-renders what
the name is already holding — `typeset -i i=5; typeset -i16 i` is `16#5`
and `typeset -i16 h=255; typeset -i8 h` is `8#377` — and that is a change
of spelling rather than a re-read, so it meets no dialect and no readonly
refusal. `+i` takes the base off with the attribute and leaves the
characters that are there alone, so only the *next* assignment is plain.

**Base ten marks nothing**, and it is why the range check and the
rendering cannot be one question: `typeset -i10 e=255` is `255` in both,
so ten is a base a shell takes and writes nothing with. Joining the two
made the shell that refuses a bad base refuse base ten as well. What the
two shells do *record* when ten is written is a separate answer, below.

**`Semantics.IntegerBaseDigits`** — bash empty · dash empty · ksh93 sixty-four long, lower case first · zsh thirty-six long, upper case

The alphabet a dialect counts in, and its length is the largest base it
can spell. Two facts in one string because they are one fact about the
shell:

    typeset -i36 b=100    ksh93 36#2s    zsh 36#2S
    typeset -i64 b=100    ksh93 64#1A    zsh refuses
    typeset -i64 b=63     ksh93 64#_     — 61 is Z, 62 is @, 63 is _
    typeset -i1  b=5      ksh93 5        zsh refuses

**`Diagnostics.IntegerBadBase`** is what a dialect says about a base
outside that range, and its absence is the other answer: ksh93 takes any
base in silence and renders plain what it cannot spell, and zsh writes
`invalid base (must be 2 to 36 inclusive): 64`, leaves the name holding
nothing, and carries on. So the refusal is the presence of a wording
rather than a second field.

**`Semantics.IntegerBaseComesFromTheValueAssigned`** — bash no · dash no · ksh93 no · zsh yes

The half of the base that is not the letter, and zsh alone. The base is
learned from the *radix prefix* of the value assigned, and it sticks to
the name:

    typeset -i a; a=0x10           ksh93 16    zsh 16#10
    typeset -i b; b=0x10; b=5      ksh93 5     zsh 16#5
    typeset -i c; c=8#7;  c=99     ksh93 99    zsh 8#143
    a=0x10; typeset -i a           ksh93 16    zsh 16#10

Two things teach it nothing in any column: a leading zero, which is not a
radix, and a value that arrived already evaluated — `$((0x10))` hands the
assignment four decimal characters and there is no prefix left to read. A
*sign* in front of one does not hide it: `b=-0x10` is `-16#10`.

The leading-zero row splits the panel for a reason of its own, which is
#1270 and not this: `016` is 14 in the three bash columns, which read it
as octal, and 16 in both shells that have a base.

What is learned is a base the shell **takes**, not only one it writes a
value in, and ten is the row that says so: `b=10#5` lists back as
`typeset -i10 b=5` while `$b` is a plain `5`, so a reading that kept only
the bases something is written in would answer that listing wrong and
every other row right. Corpus:
`declare/a-learned-output-base-says-itself-back`.

**`Semantics.IntegerBaseNegativeIsTwosComplement`** — bash no · dash no · ksh93 yes · zsh no

    typeset -i16 h=-255    ksh93 16#ffffffffffffff01    zsh -16#FF
    typeset -i2  c=-5      ksh93 sixty-four binary digits   zsh -2#101

Nothing about the value differs, only how it is written, and a reading
that took either for the rule gets the other's row wrong by a whole word.

**`Semantics.IntegerBaseTenIsNoBase`** — bash no · dash no · ksh93 yes · zsh no

Whether "no base" is a state or just base ten, which is the question
#1130 left open, and the two shells answer it differently:

    typeset -i10 d=255; typeset -p d    ksh93 typeset -i d=255
                                        zsh   typeset -i10 d=255
    typeset -i16 a=255; typeset -i a    ksh93 255      zsh 16#FF
    typeset -i16 b=255; integer b       ksh93 255      zsh 16#FF
    typeset -i16 c=255; typeset -x c    ksh93 16#ff    zsh 16#FF

Those are one answer and not two. ksh93's letter always names a base and
ten is what it names when nothing is written, so a bare `-i` is `-i10`,
ten records nothing, and a name that had a base loses it — `integer`
being the same declaration under another word, it does this too. In zsh
ten is a state: it is recorded, the listing says `-i10` back, and a later
bare `-i` leaves the base where it was.

The value reads the same either way, ten being the base nothing is
written in, so this is not `IntegerBaseDigits` asked twice: what it
changes is the listing and what a second declaration does to a base
already there. The last row is the control both readings have to pass —
another letter is not this question, and `typeset -x` over a based name
leaves the base alone in both.

Degenerate bases in ksh93 are measured and not modeled, and #1308 has
them: `-i0` leaves a standing base alone where `-i1` takes it off, both
list without a base word, and a base above the alphabet is kept and
renders in ten with the mark on — `typeset -i65 d=100` is `10#100`.
zsh reaches none of them, refusing everything outside 2 to 36.

**The two listings write the base two ways**, and one of them writes the
number rather than the text the name is holding:

    typeset -i16 a=255; typeset -p a
      ksh93   typeset -i 16 a=16#ff     a word of its own, value as stored
      zsh     typeset -i16 a=255        attached, value decoded to decimal

The unquoted `16#ff` is ksh93 reading the text as the number it is; every
other `#` is quoted there, and that wider rule is #1271. Corpus:
`declare/an-integer-attribute-carries-an-output-base`,
`declare/an-output-base-is-the-value-and-not-a-rendering`,
`declare/zsh-learns-an-output-base-from-the-value`,
`declare/an-output-base-belongs-to-the-name`,
`declare/an-output-base-outside-what-the-shell-spells`,
`declare/a-negative-value-in-an-output-base` and
`declare/an-output-base-says-itself-back`,
`declare/a-bare-integer-letter-and-what-base-ten-is`, with `declare/integer-with-an-output-base` reaching it through the second name.

**`integer` with no names is a filtered listing** — the integer
variables in ksh93 and every integer parameter, its own specials
included, in zsh — which is the listing `typeset -i` with no names is
and is not built either. It refuses by name rather than falling through
to the bare declaration listing, which would answer with the whole
variable table: a wrong answer rather than a missing one.

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

## The bare declaration listing, and the sign that reaches it

Measured 2026-09-12, bash 5.3.15 `--norc --noprofile` with a scrubbed
environment.

A bare `declare` in bash is not a listing of its own. It is that shell's
`set` listing, to the byte:

    diff <(declare) <(set)        empty
    diff <(declare +f) <(declare) empty
    diff <(declare +F) <(declare) empty

— every variable as an assignment, sorted, and then every function laid
out the way that shell says a function back.

That settles two things at once. The bare word had no answer here at all,
so `declare` refused; and the plus-signed function letters had nothing to
fall through to.

**The plus sign on `f` or `F` takes the function attribute off**; it is
not a names-only listing, which is what the two shells that have one do
with the same spelling. What is left depends on whether an operand was
written:

    declare +f f    silent, 0 — and `declare -F` after it still names f
    declare +f      the bare listing above
    declare +F      the same listing again

The operand row is the discriminating one: a shell that ignored the sign
on its way to a listing writes the body there, and a shell that read it
as "name the functions" writes the name. bash writes neither.

The three shells apart: zsh writes the bare name for `typeset +f` with an
operand and without one alike; ksh93 writes the spelling the function was
declared with — `f()` for the parenthesised form and the bare name for
the keyword one; dash has no `typeset`.


## When the case attributes act

Measured 2026-09-12, `env -i` with a scratch `HOME`, zsh 5.9.2 `-f`, bash
5.3.15 and 3.2.57 `--norc --noprofile`, ksh93u+, over `typeset -l lo=AB`.

`-l` and `-u` fold a name's value. **Where** the fold happens is the
disagreement, and it is invisible in the value:

    shell      $lo   typeset -p lo        typeset +l lo; $lo
    bash 5.3   ab    declare -l lo="ab"   ab
    ksh93u+    ab    typeset -l lo=ab     ab
    zsh 5.9.2  ab    typeset -l lo=AB     AB

bash 3.2 has neither letter and dash has no `typeset`, so neither is in
the row.

The third column is the discriminating one: taking the attribute off asks
the store directly, and only one of these shells has anything left to
reveal. A shell that folded on the way in and kept a private copy for the
listing would match column two and fail column three.

Two consequences of folding on the read, both measured and neither
derivable from the table:

- **An append joins the stored text.** `typeset -l lo=AB; lo+=CD` lists
  back as `ABCD` and reads as `abcd`. There is a third answer available
  and it is the one an implementation reaches by routing the append
  through an ordinary read — `abCD`, which no shell in the panel writes.
- **A pattern operator sees the folded text**, because it is a read like
  any other: `${v/A/x}` leaves `ab` alone. The storing shells answer the
  same thing for a different reason — they never had an `A` — and the two
  readings part on `${v/a/x}`, which is `xb` everywhere.

**The environment is a read.** `typeset -l v=AB; export v` puts `v=ab` in
a child's environment under both answers, so the store is not simply
copied outward. (ksh93 has a corner of its own here that is not this
question: `typeset -lx v=AB`, with both letters on one line, does not fold
at all there, while `typeset -l v=AB; export v` does.)

**Arrays are outside this on both answers**, and for different reasons, so
the question is not asked of one. The shell that folds on the read does
not fold an array's elements at all — `typeset -al arr=(AB Cd)` reads back
`AB Cd` — and for the shells that fold on the way in, whether an element
goes through the attribute is its own measured question.


## Which letters say what a name's values are, and how they combine

Measured 2026-09-12 from a script file, `env -i PATH=/usr/bin:/bin` with a
scratch `HOME`, `ZDOTDIR` and `HISTFILE`: zsh 5.9.2, ksh93u+, bash 5.3.15,
bash 5.3.15 as `sh`, bash 3.2.57 and dash. bash 3.2 has neither case
letter and dash has no `typeset`, so neither appears below.

Four letters say what a name's values *are* rather than where a
declaration lands or who may see it: `-i`, `-F`, `-l` and `-u`. Two
questions follow, and the panel answers them differently.

### One family, or two

    typeset z=1; typeset -l z; typeset -i z; typeset -p z
    typeset y=1; typeset -i y; typeset -l y; typeset -p y

    shell        -l then -i         -i then -l
    ksh93u+      typeset -i z=1     typeset -l y=1
    zsh 5.9.2    typeset -i z=1     typeset -il y=1
    bash 5.3.15  declare -il z="1"  declare -il y="1"

ksh93 holds one family: a name carries one such letter and the last one
written replaces the earlier. zsh replaces in **one direction only** — a
numeric letter takes a case attribute off, a case letter leaves the
numeric one standing — and bash replaces in neither. Two directions, three
answers between them, which is why there are two axes rather than one.

The float letter behaves as the integer letter does in both shells that
spell it: `typeset -l v; typeset -F v` is `typeset -F v=1.0000000000`, and
`typeset -F w; typeset -l w` is `typeset -l w=1.0000000000` in ksh93 and
`typeset -Fl w=1.0000000000` in zsh.

**The value the earlier letter produced stays.** The float rendering
survives the letter that produced it going away, which is the same reading
`+i` and `+F` already have: what is taken off is how the *next* store is
written, not what is standing there.

Only a listing observes any of this, so a wrong answer costs a `typeset
-p` that says more than the shell would.

### A type letter beside an array literal

    typeset -ia z=(1 2); echo "st=$?"; typeset -p z; echo tail

    zsh 5.9.2    typeset: z: inconsistent type for assignment, status 1,
                 and the script ends
    bash 5.3.15  st=0 · declare -ai z=([0]="1" [1]="2") · tail
    ksh93u+      st=0 · typeset -a -i z=(1 2) · tail

The integer letter makes a name a *scalar* of that type in zsh, so a
declaration that also assigns an array literal asks for two kinds at once.
Three rows say what the refusal turns on, and none of them is the array
letter:

- `typeset -i z=(1 2)`, with no `-a` at all, is the same refusal.
- `typeset -F 3 z=(1 2)` and `typeset -E 3 z=(1 2)` are refused too, and
  `typeset -Z 4 z=(1 2)`, `typeset -L 4 z=(ab cd)` and `typeset -R 4 z=(ab
  cd)` are **taken**, listing as `typeset -aZ4 z=( 1 2 )` and the like. So
  it is the letters naming a numeric *type* and not the wider
  number-taking family.
- `typeset -ua q=(ab cd)` is taken and lists as `typeset -au q=( ab cd )`.
  A case letter says what happens *to* a value; only a type letter says
  what the value is.

**It is the letter on this line and never the attribute the name is
carrying.** `typeset -i z; typeset z=(1 2)` is taken in the same shell and
leaves `typeset -a z=( 1 2 )`, the integer letter simply lost. An
implementation that asked about the name's attribute would refuse a line
every shell in the panel writes an array for.

All four declaration utilities refuse it, each naming itself in the
location: `readonly:`, `export:`, `typeset:`, and `f:local:` from inside a
function. The sentence is the one a plain word over a name already holding
an array gets — one wording, two questions that reach it.

The refusal ends the script rather than the command: inside `( … )` only
the subshell stops, at status 1, and the line after it runs.


## What a child is told about a declared name with no value

Measured 2026-09-12 from a script file, `env -i` with a scratch `HOME`,
reading a real child's environment with `env`: zsh 5.9.2, ksh93u+ and bash
5.3.15. Only a child can see any of this — every listing spells the two
cases the same way.

    typeset -x A;  env | grep '^A='

    zsh 5.9.2    nothing
    ksh93u+      nothing
    bash 5.3.15  nothing

That is the agreement the record exists for, and each shell reaches it for
its own reason: bash and ksh93 leave a valueless declaration's name unset,
while zsh sets it to the empty string and still tells no child. Two things
break the agreement.

### A second declaration gives the name the empty in its own right

In the shell whose valueless declaration sets the name, a *later*
declaration naming an attribute makes the name hold the empty for itself,
and the child is then told:

    typeset -x A; typeset -x A     A=
    typeset -x A; export A         A=
    typeset -x A; readonly A       A=
    typeset -x A; typeset +r A     A=
    typeset -x A; typeset -u A     A=
    typeset A;    export A         A=
    typeset -x A; typeset A        nothing
    typeset -x A; typeset -p A     nothing

The last two are what makes this *naming an attribute* rather than *a
second command*: a bare `typeset A` over a name that already holds
something is a listing in that shell, and so is `typeset -p`. It is not a
corner — `typeset -x V` followed later by `export V` is how a script
declares an exported name it means to fill in, and every one of them
reached a child as absent here.

### A numeric type hands the child a zero the shell does not hold

    typeset -ix Z; echo "read=[${Z-UNSET}]"; env | grep '^Z='

    ksh93u+      read=[UNSET]   ·   Z=0
    bash 5.3.15  read=[UNSET]   ·   nothing
    zsh 5.9.2    read=[]        ·   nothing

The `read=` half is what says the zero is not a value: ksh93 reports the
name as unset in the same breath, and `typeset -p Z` writes `typeset -x -i
Z` with nothing after it. The zero is what the *type* makes of nothing, and
it is produced for the child alone.

It is the numeric letters and no others. `typeset -ux U; export U` and a
plain `typeset P; export P` tell that child nothing, and the float letter
does the same as the integer one without carrying its precision —
`typeset -F 3 F; export F` hands over `F=0` rather than `0.000`.

Two recorded and not modeled, both in ksh93. Taking the letter off does not
take the zero away — `typeset -ix Z; typeset +i Z` still hands over `Z=0` —
and an output base does not reach the child either: `typeset -i8 D; export
D` is `D=0` there where zsh, by the other route, writes `D=8#0`.


## `typeset -f` that marks rather than lists

Measured 2026-09-12, `env -i` with a scratch `HOME`, zsh 5.9.2 `-f` and
ksh93u+, each stub read back with `functions nm`.

A `-f` declaration is a *listing* of the function table — except when it
carries a letter that says the operands are names to be defined later.
Then it is the shell's autoload declaration under a second word, and no
listing happens at all:

    typeset -fu nm     builtin autoload -X       identical to `autoload nm`
    typeset -fU nm     builtin autoload -XU      identical to `autoload -U nm`
    typeset -fuz nm    builtin autoload -Xz
    typeset -fUz nm    builtin autoload -XUz
    typeset -fuU nm    builtin autoload -XU      the letters are a set
    typeset -fzu nm    builtin autoload -Xz      order does not matter
    typeset -f -u nm   builtin autoload -X       nor does the bundling

Three readings follow, and each is a row above rather than an inference.

**`u` and `U` start a marking; `z` cannot.** `typeset -fz nm` marks
nobody: the name stays undefined, `functions nm` is a silent 1, and the
same `z` written beside a `u` reaches the stub and is recorded on it. So
a letter's role here depends on the company it keeps.

**Operands are required and the sign is a minus.** `typeset -fu` with no
names is 0 and silent, and `typeset +fu nm` is `invalid option(s)` at 1 —
not a marking under another name.

**A definition is never replaced.** `f(){ echo body; }; typeset -fu f`
leaves the body: it runs, and it still lists as itself. That matters more
than the corner suggests — a real startup file writes the same marking on
every reload, and a stub written over a definition turns a working
function into a file that cannot be found at the next call.

ksh93 has the same spelling with `u` alone and no `U`, and renders an
undefined function as a declaration rather than a body: `typeset -fu nm`
lists back as `typeset -fu nm`. The two bashes have no `-u` on `declare`
and refuse the letter; dash has no `typeset`.


## A function said back, and the word it was declared with

Measured 2026-09-12 on ksh93u+ 2012-08-01 through `od -c`, from a script
file so that what follows each definition is a newline.

`functions` is a builtin in ksh93 as well as in zsh, and there it is
`typeset -f` under a second word: same listing, same status, and `-p`
alongside changes nothing.

    f(){ :; }                    f(){ :; }
    function g { typeset x=1; }  function g { typeset x=1; }
    f(){ echo a; echo b; }       f(){ echo a; echo b; }
    f(){ :; }; g(){ :; }         f(){ :; } and g(){ :; }, a line each

**The keyword is not cosmetic here.** `typeset` declares a local in a
`function f { … }` body and assigns the global in an `f() { … }` one, so
the two spellings are two programs: a listing that wrote `f () ` back for
a keyword function hands over code whose variables are global where the
original's were local. The test that says so is the round trip — `eval
"$(functions f)"` and then call it — because a comparison of strings would
pass against a word written where the grammar reads something else.

bash 5.3, bash 3.2 and zsh 5.9.2 write `f () ` back for either spelling,
which they may because `typeset` declares a local in both bodies there.

The **names-only** listing keeps the same distinction with punctuation
instead of a word: `f() { :; }; function g { :; }; typeset +f` writes
`f()` and then `g`. zsh writes both bare.

### What this listing does not promise

**ksh93 prints the source text back verbatim.** `f(){    echo     a   ;
  }` lists with every one of those spaces, and a definition written on a
`-c` line ends its listing with the `;` that followed it rather than with
a newline. This implementation keeps a tree and not the source, so what it
writes is a *layout* — the compact one, with the source's own separators —
that reproduces that text for a definition written the way anybody writes
one, and normalizes the spacing of one that is not.

Byte-identical, measured: a one-line body, either header spelling, several
statements separated by `;`, and a bare listing of two functions. The one
shape that differs is a body whose statements were separated by
**newlines**, where the real shell keeps the last newline before the
closing brace and this writes `; }`.


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

**And a fourth trigger, filed as #1277.** The same wait had a second way
of never ending, and a coprocess reached it every time rather than by
arrangement: `coproc read x; echo AFTER` printed nothing here and prints
the marker at once in zsh 5.9.2, bash 5.3.15 and ksh93u+ 2012-08-01. A
coprocess reads a pipe the shell holds the write end of, so a `read` in
its body waits by construction; nothing external runs, the job never
ends, and the shell that started it waited on a process id that was never
coming. `coproc cat` did not hang, which is what says the subject is the
*builtin* and not a body that reads — `cat` is a program, and a program
starting is one of the triggers already there.

So: **a background job is settled without a process id when it is about to
read a stream that has nothing waiting on it.** The stream is asked rather
than assumed, with the same poll `read -t 0` uses — a regular file, a
here-document and an exhausted stream all answer a read at once, so the
job is left alone and `{ read x < file; sleep 1 } &` still reports the
sleep's process id. That is the `> log` half of the trade above, in the
other direction.

The coprocess is not the only route. `{ read x } &` finishes at once under
`/dev/null` and hangs when standard input is a pipe with a writer and no
data, so on the `&` route it is the stream's kind that decides. Nor is
`read` the only builtin: `mapfile` and `readarray` under their one
implementation, and the `select` clause, wait on a stream the same way and
hung the same way — three places that read rather than one, which is why
the settling is a call each of them makes rather than something written
into `read`. Both spellings of a coprocess get it, `coproc` and ksh93's
`|&` alike, because both start the job through one path.

**And a fifth, filed as #1283.** A job whose body neither reads nor ever
runs a program reached none of the four: `{ while :; do :; done } & echo
AFTER` printed nothing here and prints the marker at once in every shell
in the panel, and the issue's own repro —
`{ while :; do :; done } & sleep 0.2; kill %1; echo reached` — never
reached. There is no point in such a body where the job waits on anything
*outside* the shell, so the four triggers above cannot be stretched to
cover it.

So: **a background job is settled without a process id once it has
completed a pass of a loop whose end the shell cannot see.** The back edge
and not the start of the body, which is what makes it cost less than the
contract #1283 weighed: settling the moment a job begins would make
`{ echo hi; sleep 1 } & echo $!` print 0 where it prints the sleep's
process id today, on the most ordinary shape there is. A pass that has
*completed* says something narrower — the job has already run a whole
round of a loop without starting a process, and the shell has no way to
learn whether it ever will.

Only the loops whose end the shell cannot see. A `for` over a word list
and a `repeat` count know how many passes they have before the first one,
and a `select` reads on every pass and so is the fourth trigger already;
`while`, `until` and `for ((;;))` are the three that can run forever on
nothing at all. So `{ for i in 1 2 3; do :; done; sleep 1 } & echo $!`
still prints the sleep's process id, and a loop whose *first* pass starts
a program keeps that program's — the process starts before the back edge
is reached.

What it costs, stated rather than discovered: a bounded computation
written as a conditional loop settles at its first back edge, so
`{ i=0; while [ $i -lt 1 ]; do i=1; done; sleep 0.2; } & echo $!` reads 0
where every shell in the panel names a process. That is the trade, paid
on the rarer shape — a hang has no status to check and no diagnostic to
read, and a job reported as having no process of its own is the answer
this shell already gives for every background builtin.

**What a `&` job reads for standard input** is an axis, and it splits the
panel three ways rather than two (`BackgroundJobInput`, #1287). The probe
reads one descriptor twice, so the answers come out in opposite orders
and neither can be mistaken for the other:

```sh
printf 'DATA\n' > f
<shell> -c '/bin/cat & wait; echo ---; /bin/cat' < f
```

| | the job reads | the script then reads |
| --- | --- | --- |
| dash, bash 5.3.15, bash-as-`sh`, bash 3.2.57, ksh93u+ | *(nothing)* | `DATA` |
| zsh 5.9.2 | `DATA` | *(nothing)* |

Measured 2026-09-07, and `ls -l /dev/fd/0` inside the job names what the
five handed it: a character device with `/dev/null`'s rdev in the four,
and the file itself in zsh. POSIX XCU 2.9.3, Asynchronous Lists, says a
background command's standard input "shall be assigned to an empty file
or /dev/null" while job control is disabled, so the majority is the
specified answer and zsh is the divergence — and it is the direction that
*steals*, because the job and the script share one descriptor:

```sh
while read -r line; do process "$line" & done < input.txt
```

loses whatever `process` reads, at status 0, with nothing said. This
shell answered zsh's way in every dialect.

**Stdin's kind is an axis of the measurement, not a detail of it.** A file
and a pipe answer alike; `/dev/null` cannot tell the two apart, which is
why the probe uses neither; and a *terminal* under job control is a third
answer that neither dialect chooses. On a pty,
`bash -i -c '/bin/cat & sleep 0.3; jobs'` lists the job `Stopped` and zsh
lists it `suspended (tty input)` — both handed it the terminal and let the
kernel stop it with SIGTTIN, which an empty input can never produce. So
the substitution is conditioned on job control being off, exactly as XCU
2.9.3 states.

The third value is what a *closed* descriptor does, and it splits the five
that substitute: `exec 0<&-; /bin/cat & wait "$!"` is silent at 0 in dash
and every bash, which replace even a descriptor that is not there, and
`cat: stdin: Bad file descriptor` in ksh93u+, which substitutes only what
it can dup — the same answer zsh gives for the different reason that it
never substitutes at all. It is read through the one field rather than a
second, because it is the same decision asked of an input that is not
there.

Read without asking, like the `$!` fields above: an unanswered axis here
would have to refuse `&` itself, and backgrounding a command is ordinary
where a background job that reads standard input is rare. Unanswered is
the POSIX answer. A redirection on the job still wins — `cat < f &` reads
`f` in every dialect — because only the *inherited* descriptor is in
question. Corpus:
`jobs/a-background-jobs-standard-input` and
`jobs/a-background-jobs-standard-input-when-the-script-closed-it`.

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

**The quiet letter is `cd`'s and is handed on rather than answered
here** (#1789). zsh's `pushd -q`, `popd -q` and `pushd -q +N` move
without running the directory-change hook, which is exactly what `cd -q`
already does — so the option loop reads the letters `cd` reads (`-L`,
`-P`, `-q`, bundled or apart) and passes them through, and nothing about
the suppression lives in the prelude. Measured on zsh 5.9.2, 2026-09-10,
with a `chpwd` defined: the plain forms print the hook's marker and the
`-q` forms do not, and all of them move. It matters because the letter
was previously read as the *directory*: the operand was dropped, the
shell went to `$HOME`, and the stack entry went with it at status 0.

A word this loop does not recognize still ends the options and becomes
the operand, which is why a letter the core's `cd` has not got — zsh's
`-s` (#1569) — is left alone rather than half-read out of a bundle.

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
does not hold still under the neighboring probes: **bash 5.3.15 and bash
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

#### All thirteen, and the fourteenth is not a letter

Issue #699. Six are a pure function of the string:

    :h  everything before the last slash, trailing slashes off first,
        `.` where there is none — `/a/b//` → `/a`, `a/` → `.`, `/` → `/`
    :t  everything after it — `/a/b//` → `b`, `/` → nothing
    :r  the value with its suffix taken off, counting a leading dot:
        `.hidden` → nothing, `x/.hidden` → `x/`, `a/b.c/d` unchanged
    :e  what `:r` takes off, without its dot
    :l  lowercase        :u  uppercase

Seven need something a string does not carry, which is why performing
them made `applyModifier` a method:

    :a  an absolute path, made **lexically** — `..` and `.` canceled by
        name and no link followed. Against the *physical* working
        directory, which is neither `$PWD` nor the Runner's logical
        directory after a `cd` through a link: measured, with `/tmp` a
        link, `cd /tmp; ${x:a}` on `rel/f` is `/private/tmp/rel/f`, and
        setting `PWD` to something invented does not change it. An empty
        value stays empty.
    :A  the same, then resolved on the disk.
    :P  resolved on the disk, with `..` applied to what has already been
        resolved rather than canceled by name. That is the whole
        difference from `:A`, and `link2/..` shows it: `:A` gives the
        link's parent by name, `:P` the parent of what it points at.
        Two smaller ones — an empty value is the working directory here
        and stays empty there, and a trailing slash survives on a path
        that could not be fully resolved.
    :c  the path the command search would find, and the value unchanged
        where it would find nothing. Three narrower rules, all measured
        and all the opposite of a guess: a name holding a slash is left
        exactly as written, a **function is not a command** and a builtin
        resolves to the external file of that name (`echo` → `/bin/echo`),
        and an **empty PATH finds nothing** where the same shell's command
        *lookup* runs a `mycmd` sitting in the current directory.
    :q  quoted in this shell's own quoting. The same table `${(q)x}` uses
        with one difference: `${x:q}` on an empty value is empty where the
        flag is `''`.
    :Q  the inverse, and identical to `${(Q)x}` on every input tried.
    :s  `s<d>pattern<d>replacement<d>`, where the delimiter is whatever
        byte follows the letter — `/ | # , :` all measured, and the
        closing one optional.

Both resolving modifiers stop where the disk stops and leave the rest
alone: `/tmp/no/such` is `/private/tmp/no/such`, and a **dangling link is
its own answer** rather than the name it points at.

**`:s` is not a pattern**, which is what makes `${x/a/b}`'s machinery the
wrong tool. Measured three ways with `x=abc`: `${x:s/?/Z/}`,
`${x:s/[ab]/Z/}` and `${x:s/b*/Z/}` all answer `abc`, and each of `?`,
`[b]` and `*` is replaced where the value really holds it.

**And it leaves something behind.** The pattern and replacement are
remembered for the whole shell rather than per parameter, so
`${x:s/X/-/}` then `${y:s//+/}` reuses the `X` — and the reuse *writes
back*, so a later `:&` repeats `X → +` and not the substitution two
expansions earlier. `:&` is the fourteenth modifier and the one that is
not a letter; with nothing remembered it is silence, where an empty
pattern with nothing remembered is `no previous substitution`.

**`g` is a prefix, not a letter.** `${x:g}` names `g` as no modifier,
`${x:gh}` is the head, and it changes the answer only for `s` and `&`.

**A digit after `h` or `t` counts separators, not repetitions** — from
the left and from the right respectively, and only those two letters take
one. On `/a/b/c/d/e`, `${x:h1}` is `/` where the head three times over is
`/a/b`; `${x:h3}` is `/a/b`, which is the same answer by coincidence and
the reason a repetition reading survived being written down. A separator
is a run of slashes with something after it, so a trailing run separates
nothing and `${x:h3}` on `/a/b//` is the whole value. No digit and `0`
are the same answer, and neither is `1`.

Both left-over shapes are read now. `${x:1:5:t}` is a modifier after both
an offset and a length: a range is split once, so `5:t` arrived whole and
reached the evaluator as an expression. And `${x:h2}` is the count above
rather than a digit swallowed by the letter.

**The refusal names one byte.** `${x:zz}` is a complaint about `z`; the
second `z` has not been looked at, and naming both would say the pair is
the modifier that is missing. A letter that *is* a modifier with junk
after it is the other shape and names nothing. `${x:s}` is neither: a
substitution with no body is a bad substitution, like any other
malformed `${ }`.

**A backslash protects the byte after it, and the escapes are the
modifier's own.** Measured 2026-09-09 on zsh 5.9.2, quoted and unquoted
alike:

| written | value | answer |
| --- | --- | --- |
| `${x:s/\//:/}` | `a/b/c` | `a:b/c` — the escaped delimiter does not end the field |
| `${x:s/\./:/}` | `a.b` | `a:b` — the backslash goes, whatever it stood before |
| `${x:s/\\a/:/}` | `a\ab` | `a:b` — `\\` is one literal backslash |
| `${x:s/X/[\&]/}` | `aXbXc` | `a[&]bXc` — `\&` is a literal ampersand |
| `${x:s/X/[\\&]/}` | `aXbXc` | `a[\X]bXc` — a backslash, then the match |

The last two are why the replacement keeps its escapes until it is used:
an `&` is the matched text and `\&` is the character, so resolving the
escapes before answering the ampersands would turn the fifth row into the
fourth. The pattern half has no such question and is resolved as soon as
its field is found.

A modifier reads its own text rather than a value, which is what makes
this the modifier's question at all. The escape used to be gone before
the modifier saw anything — a lexer removes it while reading `${ }`, and
the joined text of a word is its spans with their delimiters taken off —
so `${x:s/\//:/}` arrived as `s///`, an empty pattern, and reported that
there was no previous substitution (#1198). The byte was never lost from
the tree, only from the joined text: a protected character is a span of
its own, so the backslash is written back in front of it.

**Quotes inside `${ }` are still not reproduced.** That shell reads the
brace's text raw, so a quote there is an ordinary character:
`${x:s/'.'/:/}` replaces a three-character `'.'` and leaves a plain `.`
alone. This one's lexer removes the quotes, so the same modifier replaces
the `.`. A backslash is enough to write any of these, and is what a
script would ordinarily use.

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

#### zsh's third wording: a byte the reader refuses outright

Issue #698. zsh has a third sentence for a byte that is part of no
arithmetic token — `illegal character: @` — and it is not a question of
*which* byte alone; it is also where the byte stands:

    $((@))       illegal character: @          nothing read yet
    $((1 @))     illegal character: @          where an operator belonged
    $((1+@))     operand expected at `@'       where an operand belonged
    $((+@))      operand expected at `@'       after a unary operator too
    $((  @  ))   illegal character: @          whitespace is not reading
    $((1 @@))    illegal character: @          the byte, not the run
    $((1 2))     operator expected at `2'      a value where an operator belonged

**The observable rule.** The byte's own verdict stands where the
expression could legally have stopped — before anything has been
consumed, or where an operator belonged — and the parser's "operand
expected at <rest>" wins where an operator has just been consumed and a
value is wanted. The two sentences also name different things about the
same failure: the byte's names the **single byte**, the operand's names
the text from it to the end of the expression. `$((1+@2))` is
``operand expected at `@2'`` and `$((1 @@))` is `illegal character: @`.

**Two tables and no code path.** `Dialect.ArithBytesRefusedOutright` is
the bytes and `Diagnostics.ArithIllegalByte` the sentence; both are empty
for the three shells that have neither, and an empty table leaves every
failure with exactly the kind it had. The position is the parser's, and it
is asked of the text rather than kept as a flag: "has anything but
whitespace been consumed" is a property of what was read, where a flag
would have to be set at each of the seven frames that consume an operator
and would be wrong the first time one was added. `$((  @  ))` is what
rules out the cursor's offset as the test.

**The refused set measured is `' ; @ \ ] { }`** — seven bytes; every
other punctuation byte tried is either a math token in that shell
(`% & * = | : , < > / ? #`) or can begin a value (`$` is the process id,
`?` the last status, `#` the character-code operator). #698 recorded five
of the seven, which is the set reachable through `$(( ))`: a `\` escapes
the closing paren and `$((]))` is re-read as `$( (]) )`, so those two
reach the reader only through `let`, where they are measured and where
they agree. A backquote is refused by that reader too and is deliberately
**not** in the table — nothing in this shell reaches the reader with one,
so it would be a row no measurement of ours could hold honest.

**Still recorded rather than reproduced**, and each is a different
mechanism rather than more of this one:

- The **control bytes.** That shell refuses every byte but tab and
  newline, in caret notation — `illegal character: ^A` — and the caret
  reaches the *operand* sentence as well (``operand expected at `^A'``),
  so it is a rendering rule across three wordings rather than seven more
  table entries.
- **`$((1 :))`** is `operand expected at end of string` there and
  `operator expected at `:'` here, and **`$((1 : 2))`** is a *fourth*
  sentence — `':' without '?'`. A stray `:` is a math token that shell
  consumes and then wants an operand after; ours only takes one after a
  `?`. A parser-shape difference, not a byte.
- **`$((}))`** reaches our word lexer as `parse error near `}'` where
  `$(( } ))` reaches the reader and answers correctly.

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
  rather than where the boundary is. The same fact retired an axis —
  `UnsetNameAtIsOneEmptyField` below, which was `EmptyArrayAtIsOneEmptyField`
  and was measured from `a=()` counting one field in ksh93 (#1379). This
  entry and the table in `grammar/commands.md` were each enough to refuse
  that measurement; neither was consulted, because the corpus row that fed
  it counted fields and never printed one. **Cross-reference an entry here
  before reading a divergence off a count.**
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
`StatusArgumentPolicy`, `TrapBodyLineStyle`, `SelectMenuLayout`,
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
and are not asked about, and `-q` is asked about separately below because
it is a letter one shell genuinely *has*.

**`CdHasQuietOption`** — bash no · dash no · ksh93 no · zsh yes

Gives `cd` the `-q` of zsh, the one option letter beyond `-L` and `-P`
that any of the panel has.

What the letter means there is hook suppression, and nothing else.
Measured 2026-09-08: a `chpwd` function and a name in `chpwd_functions`
both ran on a plain `cd` and neither ran on `cd -q`; `cd -q -` still
wrote the directory at an interactive prompt, and a CDPATH move stayed
silent with the letter and without it. Measured again 2026-09-10 for
`pushd -q` and `popd -q`, which move through `cd` and are quiet for the
same reason.

The letter is carried to the site that fires the hook rather than
swallowed at the option loop: `cd` runs `DirectoryChangeHook` and `cd -q`
does not. It was free while this shell had no such site, and #1775 gave
it one. It is carried at all because it is a *letter*: without it
`cd -q /tmp` went looking for a directory called `-q`, which is the shape
a plugin manager that wraps every move in `cd -q` cannot survive (#1558).

Asked only when a `q` is actually seen, so the five columns without the
letter never reach the question and answer the word the way they answer
any other letter they do not have — `CdRefusesUnknownOption` above is the
next question when this one says no.

zsh has a fourth letter, `-s`, which refuses a path with a symlink
component: `cd -s link` and `cd -s link/deep` are both `not a directory`
there while `cd -s real` moves. It is not carried and reaches the
unknown-letter question, so `cd -s dir` is read as a directory called
`-s` in our zsh where the real one moves. Filed as #1569 rather than
guessed at.

**`DirectoryChangeHook`** — bash — · dash — · ksh93 — · zsh `chpwd`

Names the function `cd` calls once it has moved. Empty is a shell without
one, which is three of the four: measured 2026-09-10, a `chpwd` function
defined in bash 5.3.15, in that binary under an argv[0] of `sh`, in bash
3.2.57, in dash and in ksh93 ran on none of their `cd`s and none of them
said anything about it.

It fires on the *move* rather than on the change — `cd` to the directory
the shell is already in runs it — and not at all on a `cd` that failed or
a `cd -q`. It is the last thing `cd` does, after anything `cd` itself
printed, and it is told no arguments, with `$PWD` and `$OLDPWD` already
set. `pushd`, `popd` and a bare directory name under `autocd` are `cd`
here and there both, so one placement answers all of them; assigning to
`PWD` is not a move and runs nothing. `docs/spec/hooks.md` has the whole
table and the chain it shares with the prompt hooks. #1775.

**`ExitHook`** — bash — · dash — · ksh93 — · zsh `zshexit`

Names the function the shell calls on the way out, which is where a
plugin tears down what it started. Empty is a shell without one, which is
three of the four: measured 2026-09-12, a `zshexit` function defined in
bash 5.3.15, in that binary under an argv[0] of `sh`, in bash 3.2.57, in
dash and in ksh93 ran on none of their exits and none of them said
anything about it.

It fires **after** the EXIT trap — the trap is the script's own last word
and the hook is the shell's — and it is told no arguments, with `$?` the
status the shell is leaving with, put back before each item of the chain.
A `return` cannot change that status and an `exit` can, the last one
winning; and, alone among the hook chains here, an item that exited does
not stop the ones after it. That follows from the site rather than being
a special case: the session is already over, so `exit` has nothing left
to end. Neither this nor the EXIT trap runs after a fatal signal.
`docs/spec/hooks.md` has the whole table. #2111.

**`HookListSuffix`** — bash — · dash — · ksh93 — · zsh `_functions`

What a hook's list of *extra* function names is spelled by: the hook's
own name plus this. Not decoration — `add-zsh-hook chpwd f` defines no
function called `chpwd`, it appends `f` to `chpwd_functions`, so a shell
reading only the named function would find a correctly registered hook
and run nothing (#1281).

On the semantics vector rather than on `repl.HookStyle`, where it began,
because the hook *sites* are on both sides of that line: `precmd` fires
in a prompt loop and `chpwd` fires inside `cd`. One home for the suffix,
one `Runner.HookChain` that applies it.

**`CdWithoutHomeIsAnError`** — bash yes · dash no · ksh93 yes · zsh no

Makes `cd` with no operand and no HOME a failure. True in bash and
ksh93; dash and zsh stay where they are and report success, which is the
quieter answer and the surprising one. The same axis answers `cd -` with
no OLDPWD.

**`CdEmptyOperandIsAnError`** — bash yes · dash no · ksh93 yes · zsh no

Refuses `cd ""` instead of taking it as the directory the shell is
already in. An empty operand is not the same thing as no operand, and it
is not nothing either: `cd /tmp; OLDPWD=MARK; cd ""` leaves OLDPWD as
`/tmp` in dash, bash 3.2 and zsh and fires zsh's `chpwd`, so where it is
accepted it is a real move to the same place. bash 5.3 and the same
binary called as `sh` say `cd: null directory`; ksh93 says `cd: bad
directory`; both at 1 and both staying put. A fifth branch — neither
"cannot change" nor "HOME not set".

**`CdEmptyHomeIsAnError`** — bash no · dash no · ksh93 yes · zsh no

Refuses `cd` with HOME set to the empty string, rather than going where
the shell already is. A separate question from `CdWithoutHomeIsAnError`,
which is about a HOME that is *absent*, and separate again from the axis
above: bash answers yes to the first, no to this one and yes to the
third, so no two of them can be one field. ksh93 refuses an empty HOME in
the same words it refuses an empty operand with.

**`CdSubstitutesTheOperands`** — bash no · dash no · ksh93 yes · zsh yes

Reads `cd old new` as a rewrite of the current directory — the first
occurrence of old in `$PWD`, in the *string* rather than the path
component, replaced by new — instead of as too many operands. From
`…/a/q/a/w`, `cd a Z` lands in `…/Z/q/a/w` in both shells that have the
form. bash refuses the shape outright at status 2; dash and bash 3.2
ignore everything after the first operand.

**`CdSubstitutionPrintsTheDirectory`** — bash no · dash no · ksh93 yes ·
zsh no

Writes where a `cd old new` went, the way `cd -` writes where it went.
Asked only on a substitution that arrived somewhere: a rewrite naming a
directory that is not there prints nothing in either shell.

**`CdRefusesExtraOperands`** — bash yes · dash no · ksh93 yes · zsh yes

Refuses operands after the first instead of ignoring them, in a dialect
that does not read two as a substitution. Asked only where
`CdSubstitutesTheOperands` said no, which is the point the two shells
that have the form are no longer in the conversation — so the yes for
ksh93 and zsh is recorded rather than reached.

**`CdpathAnnouncesTheDirectory`** — bash yes · dash yes · ksh93 yes · zsh no

Prints where CDPATH sent a `cd`, when the winning entry was not a plain
dot — three of the four; zsh moves in silence.

**`AutoCdAnnouncesTheSubstitution`** — bash yes · dash unspecified ·
ksh93 unspecified · zsh no

Writes the `cd` that a bare directory name was read as, before moving.
Only two of the panel have the option that turns the behavior on, and
both spell it `autocd`, so the capability is the substrate's and this is
the one thing the two shells do differently with it: bash writes
`cd -- subdir` and zsh writes nothing. Measured 2026-09-08 through a
pseudo-terminal, which is the only route either shell has for it —
`bash -c 'shopt -s autocd; subdir'` and `zsh -c 'setopt autocd; subdir'`
both say `command not found`, so a `-c` probe cannot see the behavior at
all, let alone the sentence.

Unanswered in the base rather than given the quieter default, because
dash and ksh93 have no way to reach it: with no option to turn the
capability on, nothing in those shells ever asks.

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

**An omitted who before `=` is not an axis.** `umask -- =w` means all
three groups in every column of the panel, and `SymbolicMaskSetsWithoutAWho`
recorded a disagreement that is not there. See *A probe that measured the
wrong shell's feature* below for how it got in and how it came out (#2057).

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

**`LoopControlOutsideALoopIsFatal`** — bash no · dash no · ksh93 no · zsh
yes

Ends the script when `break` or `continue` is run with no loop around it,
instead of reporting it — or not — and running the next command. `echo t;
break; echo after` prints `after` and ends at 0 in dash, ksh93, bash 5.3
and bash called as `sh`; zsh prints neither `after` nor anything after it
on a later *line* either, so it is the script that stops and not the
line. The status is then the dialect's own for a fatal error, which is 1
there.

Whether anything is *said* is a separate question and cuts the panel
differently — bash reports and carries on, zsh reports and stops, dash
and ksh93 say nothing and carry on — so the wording is
`Diagnostics.LoopControlOutsideALoop` rather than a second answer here.

The question is only about the misuse. A `break` with a loop around it is
ordinary control flow everywhere, and a `break` inside a *subshell* that
is inside a loop leaves that subshell in four of the six, which is why
the count this is asked against is the dynamic one a cloned Runner
carries with it.

**`ReturnOutsideAFunctionIsRefused`** — bash yes · dash no · ksh93 no · zsh no

Reports a `return` that has nothing to return from and carries on,
instead of ending the script with the status it was given. True in bash
alone.

Asked only where there is nothing to return from. Inside a function and
inside a sourced file all four obey it, so the question is about the one
case they split on.

**`StartupFileReturnCarriesItsArgument`** — bash no · dash yes · ksh93 yes · zsh yes

Makes `return 3` at the top of a startup file leave `$?` as 3, instead of
leaving whatever the command before it left.

A startup file *is* a sourced script, so a `return` in one is accepted by
every shell in the panel, stops reading the file there, and is diagnosed
by nobody. The split is over the argument alone. Measured 2026-09-07
through a pty, reading `$?` at the first prompt:

| rc file | bash 5.3.15 | bash32 | bash-as-sh | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `return 3` | 0 | 0 | 0 | 3 | 3 | 3 |
| `false; return 3` | 1 | 1 | 1 | 3 | 3 | 3 |
| `false; return` | 1 | 1 | 1 | 1 | 1 | 1 |
| `(exit 5)` | 5 | 5 | 5 | 5 | 5 | 5 |

The last two rows are what make it the argument and nothing else: bash
does carry a startup file's status out, and a bare `return` means the
last command's status everywhere.

Asked only of a `return` at the top level of the startup file. One inside
a function the file calls, or inside a file the file sources, carries its
argument in bash too — measured, an rc running `f(){ return 3; }; f` or
`. inner.sh` leaves 3 in bash 5.3.15 — so it is a property of the
outermost frame rather than of `return`. See `docs/spec/invocation.md`
for the neighboring routes.


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
with none, which is what a script on a pipe has: bash and ksh93 grant the
option silently; dash remarks `can't access tty; job control turned off`
and reports success with the option left off; zsh refuses it at 1,
fatally. The two refusal shapes are the dialect's own wording and status —
Diagnostics.MonitorDenied and MonitorDeniedStatus. Turning the option
*off* is granted everywhere.

**The terminal and nothing else**, which is measured on the other side of
the split and was got wrong here until #1720. On a pseudo-terminal, with
no prompt and nothing interactive about the invocation, all six columns
grant `set -m` inside a plain `-c` string and put `m` in `$-`:

| shell | `zsh -c 'set -m'` on a pipe | the same on a pseudo-terminal |
| --- | --- | --- |
| bash 5.3.15 | 0, `m` in `$-` | 0, `m` in `$-` |
| bash 3.2.57 | 0, `m` in `$-` | 0, `m` in `$-` |
| ksh93u+ | 0, `m` in `$-` | 0, `m` in `$-` |
| dash | the remark, option off | 0, `m` in `$-` |
| zsh 5.9.2 | `can't change option: -m`, 1, fatal | 0, `m` in `$-` |

So the fact the question is asked of is `Runner.Terminal` — a descriptor
fact every route carries — and not `Runner.JobControl`, which says there
is a *person* to announce a job to and is a prompt and nothing else.
Reading the second answered the terminal question with the prompt's
answer and refused every script that had a terminal, which is where a
prompt theme's `setopt monitor` was refused by a shell that was already
running a monitor.

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

**It is `set`'s fatality and not the option's**, which a dialect with a
second option builtin makes visible. Measured on a pipe in zsh 5.9.2,
2026-09-10:

| written | said | status | what runs after |
| --- | --- | --- | --- |
| `set -m` | `zsh:set:1: can't change option: -m` | — | nothing; the script ends |
| `setopt monitor` | `zsh:setopt:1: can't change option: monitor` | 1 | the rest of the script |
| `set -o nosuch` | `zsh:set:1: no such option: nosuch` | — | nothing; the script ends |
| `setopt nosuch` | `zsh:setopt:1: no such option: nosuch` | 1 | the rest of the script |

Same option, same sentence, same status on the builtin — so what decides
is which builtin asked. `set` is one of the standard's special builtins,
whose failure ends a non-interactive shell; `setopt` is an ordinary
builtin the dialect added. The axis therefore governs `set` alone, and a
request arriving through Runner.ApplyNamedOption — the seam a dialect's
own option builtin uses, and the one the environment's option list uses —
never ends the script whatever this says.

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

**`InteractiveSelectsEmacs`** — bash yes · dash no · ksh93 no · zsh no

Turns the `emacs` editing mode on when the shell becomes interactive, and
leaves both mode names off otherwise. bash alone. Measured 2026-09-11,
without a terminal and at one:

| asked | bash 5.3.15 | ksh93 | zsh 5.9.2 | dash |
| --- | --- | --- | --- | --- |
| `set -o` in a script | `emacs off`, `vi off` | both off | both off | both off |
| `set -o` under `-i` | **`emacs on`** | both off | both off | both off |
| an interactive session at a real terminal | `emacs on` | both off | `[[ -o emacs ]]` answers 1 | both off |

Two things follow. The trigger is **interactivity and not a tty** — bash
with no terminal but `-i` reports `emacs on` — so this is a question about
*when* a mode is chosen rather than about which. And the zero value has to
be a fourth state, "not chosen yet", because "chosen off" reads differently
in exactly this shell: `bash -i -c 'set +o emacs; set -o'` reports `emacs
off` where `bash -i -c 'set -o'` reports it on. bash 3.2 and bash invoked
as `sh` agree with 5.3 throughout.

Read rather than `ask`ed, as `DefaultOptionLetters` is: reporting an option
is not the place to refuse a script over a disagreement, and a dialect that
answers nothing gets the majority's no.

**`SetFTurnsOffGlobbing`** — bash yes · dash yes · ksh93 yes · zsh no

Makes `set -f` the short spelling of `set -o noglob`. True in bash, dash
and ksh93. zsh spells that option the long way only: there `-f` is about
startup files and leaves globbing alone, so `set -f; echo *.txt` lists
the files.

**`SetBTurnsOffBraceExpansion`** — bash yes · dash no · ksh93 yes · zsh no

Makes `-B` the short spelling of `braceexpand`, so `set +B` stops `{a,b}`
expanding and `set -B` puts it back. Measured 2026-09-11: bash 5.3.15 and
ksh93 write `{a,b}` after `set +B` and `a b` after setting it again, and
`$-` drops the letter while it is off. zsh has the letter and means the
terminal bell by it — `set -B` there leaves braces expanding — so that
dialect keeps `B` among the letters it refuses, and a `no` here falls
through to the dialect's own refusal rather than to a silent no-op. dash
has neither the letter nor braces.

Asked only where the letter is written, like SetFTurnsOffGlobbing: the
long name `braceexpand` raises no question, because a shell either
declares it or has never heard of it.

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

**`BadNameToReadFatal`** — bash no · dash no · ksh93 no · zsh yes

Ends the script when `read` is given an operand that is not a name.
`read 1bad; echo after` prints the refusal and nothing else in zsh, and
prints `after` in the other five.

A third field rather than either of the two above, and not because the
panel splits differently — it does, but that alone would only make it a
separate value. `read` is not a special builtin in any shell, so no
dialect's rule about special builtins reaches it: dash and ksh93 stop
the script for `export 1x` and carry on past `read 1x`, which is the
same shell answering the same kind of failure two ways depending on the
builtin. zsh is the one that stops here, and it stops for `export` too.

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

**`ReadRefusesABadNameBeforeReading`** — bash yes · dash no · ksh93 yes · zsh no

Judges `read`'s first operand as a name before it goes to the stream,
rather than after.

Observable, and only through the input: a refusal that comes first
leaves the line for the next reader, and one that comes after has eaten
it. `printf 'AAA\nBBB\n' | { read 1bad; cat; }` prints both lines in
bash 5.3 and ksh93 and only BBB in dash and zsh. bash 3.2 is on dash's
side, which makes this a change within bash rather than a difference
between shells.

It is the *first* operand and not the whole list. Every shell in the
panel assigns the names in front of a bad one and then refuses:
`printf 'X Y Z\n' | { c=keep; read a 1bad c; }` leaves a as X and c as
keep in all six, and the line is consumed in all six — including the two
that would not have read it had `1bad` come first.

**`ReadCountJudgesTheNamesAfterTheFirst`** — bash yes · dash unanswered · ksh93 no · zsh unanswered

Keeps judging `read`'s operands as names when `-n` or `-N` gave it a
count.

bash does; ksh93 stops at the first. `printf 'XYZW\n' | { b=keep; read
-n 3 a 1bad b; }` refuses `1bad` in bash and says nothing in ksh93, where
`read a 1bad` is refused in both — so it is the count that moves it and
not the operand. The filling stops at the bad name in each, b keeping
what it had, so what a count releases is the complaint and not the list.

Left unanswered in the two that cannot reach it: dash has no count
letter at all, and zsh's `-n` is a flag rather than a count while its
`-k` reads from the terminal. An answer there would be a claim nothing
measured.

**`ReadPromptOperand`** — bash ReadOperandIsAllName · dash ReadOperandIsAllName · ksh93 ReadPromptNeedsANameBeforeIt · zsh ReadPromptAloneNamesTheDefault

Says whether `read`'s first operand may carry a prompt after a `?`, and
what an operand that is nothing else names.

The form is ksh93's and zsh inherited it: `read "v?Name: "` reads into v
and writes `Name: ` at a terminal, which is `read -p` in one word. It is
the first operand alone — `read v "w?p"` is a bad name `w?p` in both —
and the prompt is written for a terminal only, so a piped `read "v?p"`
is a plain read into v. Where the mark stands alone the two part company:
ksh93 refuses the empty name it is left with, naming that empty word,
and zsh reads into REPLY.

It has to be answered wherever the name check is, not beside it: the
word a shell judges is the part in front of the `?`, so a check that did
not know the form would refuse the idiom in the two shells that spell it.

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

**`BareSubscriptIsASubscript`** — bash · dash · ksh93 unanswered · zsh yes

Reads the `[…]` an *unbraced* `$name` carries as a subscript rather than
as three ordinary characters behind the parameter:

    a=(xx yy zz)
    $a[1]      subscript  → the element    text → `xx[1]`
    ${a[1]}    unaffected either way — the braces say where it ends

Unanswered in the three shells whose grammar has no such construct at
all: `BareSubscript` is off there, so nothing ever asks. A grammar that
turns the flag on and leaves this unanswered is a gap and says so out
loud rather than picking a side.

The distinction from the grammar flag is the point of having both. The
flag decides whether the brackets *belong to* the expansion — a word
boundary, answered when the word is read. This decides what they mean,
and it is answered when the word is **expanded**: zsh moves it at run
time with `ksharrays`, and a function body written under one answer and
called under the other takes the caller's. Measured 2026-09-10 on zsh
5.9.2 and 5.9, from `-c`, from a script file and across a `source`:

    a=(xx yy zz); f() { echo "$a[1]"; }
    f                      xx
    setopt ksharrays; f    xx[1]     the same body, the caller's answer

Deciding it while reading gives a shell that is right in a script and
wrong in `eval`, or the reverse.

The two halves of the "text" answer are one answer, and both are needed:
the parameter loses the subscript **and** the brackets come back as the
word's own text — expanded, split and read as a pattern wherever the
word around them would be. `b=2; $a[$b]` is `xx[2]`, and an unquoted
`$a[1]` is the pattern `xx[1]`, which is what leaves a `$dir[0-9]*`
written for another shell the glob its author meant.

**`KeyedTableScalarIsTheFirstValue`** — bash no · dash unspecified · ksh93 no · zsh yes

Says *which* element a plain `$m` gives when `m` is a keyed table and
`ArrayScalarIsTheWholeArray` has answered "one element". The same
disagreement about whether such a table has an order at all: bash and
ksh93 look up the key `0` and hand back nothing where there is no such
key, while zsh — whose tables keep their insertion order — hands back the
first value in it. Measured 2026-09-10:

    m=(a 1 b 2)   $m   zsh 1      bash, ksh93 empty
    m=(z 9 a 1)   $m   zsh 9      so it is the order, not a sort
    m=(a 1 0 x)   $m   zsh 1      and not the key `0` under another name

A second axis rather than a widening of the first, because the two shells
that share the first answer do not share this one — and it is reachable
only after the first has been answered, so a dialect where a bare name is
the whole table never asks it. zsh reaches it only under `ksharrays`.

"First" is whatever order `${m[@]}` yields, which is a separate question
and is answered below. The axis says which end of the order to read; it
does not say what the order is.

### KeyedTableOrder: the order a table lists in is nobody's to promise

The axis above was recorded as "zsh keeps insertion order and this shell
sorts by key". **Insertion order is not what any shell in the panel
does.** Measured 2026-09-11 — every table built by assigning three keys,
then listed:

| built as | bash 5.3.15 | ksh93u+ | zsh 5.9.2 | here |
| --- | --- | --- | --- | --- |
| `m[z]=1; m[a]=2; m[m]=3` | `z m a` | `a m z` | `z m a` | `a m z` |
| `m[a]=2; m[m]=3; m[z]=1` | `z m a` | `a m z` | `z m a` | `a m z` |
| `m[m]=3; m[z]=1; m[a]=2` | `z m a` | `a m z` | `z m a` | `a m z` |
| `m[B]=1; m[a]=2; m[1]=3; m[_x]=4` | — | `1 B _x a` | `_x a 1 B` | `1 B _x a` |

Three findings, and the first is the one that matters:

- **The order does not depend on the order of insertion.** All three
  spellings of the same table list identically in every column, so no
  shell here is recording the order the keys arrived in. Unsetting a key
  and assigning it again does not move it either: `unset "m[z]"; m[z]=9`
  leaves `z` exactly where it was in zsh.
- **bash and zsh list in the order their hash puts the keys in.** It is
  deterministic for a given set of keys — the same table lists the same
  way on every run — and it is neither sorted nor historical. Insertion
  order shows only *between keys that collide*: `one two three four` and
  `four three two one` list as `one four two three` and `one two four
  three` in zsh, which is the one place the arrival order survives.
- **ksh93 sorts by key**, in byte order, and that is exactly what this
  shell does. The fourth row is the discriminator: `1 B _x a` is the ASCII
  sort, and no hash produces it by accident.

So "insertion order" was a reading of two probes whose insertion order and
hash order coincided. A probe over a table whose keys are already in
sorted order cannot tell any of these three apart, and a probe built by
assigning keys in one order cannot tell insertion order from a hash.

**What this implementation promises, and what it does not.** The order is
by key, in byte order: deterministic, independent of how the table was
built, and stable when a key is removed and put back. That is ksh93's
answer exactly and it is the only one of the three that can be written
down as a behavioral fact — a hash order is a property of a hash function
and a table size, which are implementation and not behavior, and this
repository does not read another shell's implementation. Reproducing it
would also be pinning an accident: a script that depends on it is already
broken across the two shells that have one, which do not agree with each
other.

The consequence for the axis above is stated rather than hidden: under
`ksharrays`, `$m` on a keyed table is our first key's value where zsh
gives its first hashed key's value, and the two coincide only when the
orders do.

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

**`DeclarationTakesAnAppendOperand`** — bash yes · dash no · ksh93 no · zsh no

A declaration builtin's `name+=value` operand is the append operator
rather than a name with a `+` on the end of it:

    declare a=1; declare a+=2   bash        → 12
                                ksh93       → typeset: a+: invalid variable name
                                zsh         → not valid in this context: a+
                                dash        → export: a+: bad variable name

Measured 2026-09-09 under `declare`, `export`, `readonly` and `local`
alike; bash 3.2 and bash-as-`sh` answer `12` with bash 5.3. So it is one
shell's operand and not a core one.

The three that refuse it all name **`a+`** — the text in front of the
`=` — and not the whole operand, which is what two of them do for
`typeset 1x=v`. That is the tell that they read the `+=` as an operator
too, and then refuse the name it leaves behind.

The value joins through the name's own attributes, which is the same join
the bare `a+=2` statement performs rather than a second rule:
`declare -i a=1; declare a+=2` is 3, `arr=(p q); declare arr+=x` is
`px q`, and a declared table joins its `0` key. It joins **the cell the
declaration writes**, so a declaration that takes a fresh scope has
nothing to join to — `a=1; f(){ local a+=2; }` leaves `2` in the local
and `1` in the caller, where `export a+=2` and `declare -g a+=2` take no
scope and leave `12`.

Asked only where an operand's name ends in `+` and a value follows it. A
`+` with no `=` is not this spelling — `declare a+` is refused as a name
in every column, the one that takes the operator included.

**`NegativeSubscriptCountsOverAPromotedScalar`** — bash yes · dash unspecified · ksh93 no · zsh unspecified

An element write over a name holding a string keeps the string as the
first element — core, and unanimous in every shell in the panel that has
arrays: `a=abc; a[1]=x` leaves `abc` beside the `x`. *When* the promotion
happens relative to reading the subscript is not unanimous, and the only
spelling that can tell is one counting back from the end:

    a=abc; a[-1]=x      bash        → declare -a a=([0]="x")
                        ksh93       → a: subscript out of range

bash promotes and counts back over the one element it just made; ksh93
counts back first, over an array with nothing in it, and refuses. bash
3.2 gives the same complaint for `a=(p q); a[-1]=x`, so it has no
negative subscripts at all rather than a third answer here.

Asked only where there is a scalar to promote and the subscript is
negative. A non-negative subscript lands at the number it names under
either reading, and an unset name has nothing to promote — `unset a;
a[-1]=x` is refused in both columns. The shell where a subscript on a
string names a *character* never reaches the question: there is no array
to promote into on that side, which is `ScalarSubscriptIsACharacter`.

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

**`WholeSubscriptOnAScalarMeasuresIt`** — bash no · dash no · ksh93 no · zsh yes

Makes `${#s[@]}` on a name holding one string the **width of that
string** rather than the count of a list of one. Measured 2026-09-10:

    ${#a[@]} on      a=""   b=x   h="a b"
    bash 5.3.15      1      1     1
    bash as `sh`     1      1     1
    bash 3.2.57      1      1     1
    ksh93            1      1     1
    dash             bad substitution
    zsh 5.9.2        0      1     3

The three-character row is what says which reading it is. An empty
scalar answering 0 on its own would only say "no elements"; `a b`
answering 3 says a whole-array subscript on a scalar reaches the
*value*. One answer against one, so it is a disagreement and not a
majority.

Asked of the **length alone**, because the length is where *this* split
falls. The plain value is unanimous — `set -- "${h[@]}"` leaves one
parameter holding `a b` in every column — so the fields ask nobody. The
**slice** is not unanimous and is not this question either; it is the
axis below. This paragraph used to say everything but the length agreed,
which is what let the slice keep the count's reading for a day (#1850).

It is how a script asks "did I get anything?" after a parse:
`local -a opts; zparseopts …; (( ${#opts[@]} ))` reads 1 for a name that
never became an array, which is a count agreeing with the wrong answer
(#1553).

**`WholeSubscriptOnAScalarSlicesIt`** — bash yes · dash unspecified · ksh93 no · zsh yes

Makes `${s[@]:off:len}` on a name holding one string a slice of **that
string's characters** rather than of a list whose only element is the
whole value. Measured 2026-09-11, on `h="a b"` and `h=abcdef`:

    on h           ${h[@]:0:1}  ${h[*]:0:1}  ${h[@]:1}  ${h[@]:2:3}
    bash 5.3.15    a            a            ` b`       cde
    bash as `sh`   a            a            ` b`       cde
    bash 3.2.57    a            a            ` b`       cde
    ksh93          a b          a b          (no field) (empty)
    dash           bad substitution
    zsh 5.9.2      a            a            ` b`       cde

A **different split from the length above**, and that is the whole
reason it is a second field rather than a second reading of the first:
bash counts a list of one for `${#h[@]}` and slices the characters here,
so no single answer about "what a whole subscript on a scalar reaches"
fits both rows. ksh93 keeps the list reading for the slice and the count
reading for the length; zsh takes the value for both.

The offsets index the value exactly as `${h:off:len}` does — `${h:0:1}`
is `a` in every column, which is the control saying the character
reading is not new — and the negative offset, the negative length and
the locale's idea of a character are the substring's own questions,
already answered, rather than anything the list slice re-decides.

Asked of the **slice alone**, and only of a name that is set and holds
one string: an unset name is empty under both readings and a real array
is a list in every column, so neither has two readings to choose
between. The one-element array is the control that separates the two —
`a=("a b"); "${a[@]:0:1}"` is the whole element in every column,
including the ones that cut a scalar.

Its silence is why it is an axis and not a default. `${line[@]:0:1}`
reads as the first character to whoever wrote it and comes back as the
whole line under the list reading, and `${h[@]:1}` — drop the first
character — comes back as nothing at all, both at status 0 (#1850).

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

**`UnsetNameAtIsOneEmptyField`** — bash no · dash n/a · ksh93 no · zsh yes

Hands a quoted `"${a[@]}"` written on a name that holds nothing at all
one empty field. zsh alone, where a name that is not a declared array
reads as a scalar, and an unset scalar under quotes is the one empty
field `"$a"` gives.

A question about **existence** and not about emptiness. An array that
exists and has no elements is no field in every column, so that half is
core and asks nobody. Measured 2026-09-07, with a *function* rather than
`set --` so the positional-parameter builtin is not a confound, and with
each shell's own way of declaring an empty array:

| shape, count of fields | dash | bash 5.3 | bash as sh | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `unset a; f "${a[@]}"` | *n/a* | 0 | 0 | 0 | 0 | **1** |
| declared, no elements | *n/a* | 0 | 0 | 0 | 0 | 0 |
| `f "${a[@]}"` on one element | *n/a* | 1 | 1 | 1 | 1 | 1 |

The compound variable is not new evidence: it is in `grammar/commands.md`'s
array-literal table and in the "recorded rather than reproduced" list above.

The declared row is spelled `a=()` for bash and zsh and `set -A a` for
ksh93, and the difference is the whole reason this axis is named for
existence. It was `EmptyArrayAtIsOneEmptyField`, ksh93 yes, recorded from
`a=(); set -- "${a[@]}"; echo "n=$#"` answering `n=1` there. **ksh93 does
not read `a=()` as an array literal.** It builds a *compound* variable —
`typeset -p a` reports `typeset -C a=()` — whose value is the three bytes
`(`, newline, `)`, so the one field that row counted was not an empty
one. A count-only snippet cannot tell those apart, and the corpus row
recorded the agreement of a coincidence for as long as it stood. Asked
with `set -A a`, ksh93 gives no field; and `set -A a` on an array that
already has elements leaves the name *unset*, so ksh93 has no
declared-and-empty state to answer for at all.

zsh is the column that splits the two states, and in the opposite
direction from the one that story predicted: `a=()` there is a set, empty
array and no field, while a name nothing declared is one field. The
careful idiom `"${a[@]+"${a[@]}"}"` guards the unset name, which is what
it was always for.


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

**`EmptyArithSubscript`** — bash reported, and zero · dash unspecified · ksh93 the empty expression · zsh an invalid subscript

Is what `a[]` means where an expression reads it — brackets written with
nothing between them. It is reached far more often than it is written:
an arithmetic expansion substitutes its parameters *before* it parses,
so `$(( a[$w] ))` with an empty `$w` **is** `$(( a[] ))` by the time the
expression exists. That is the line a widely installed completion
plugin's `bind_count=$((_ZSH_AUTOSUGGEST_BIND_COUNTS[$widget]))` runs
with an empty `$widget`, and the diagnostic a real interactive startup
stopped on.

Measured 2026-09-10, `-c`, on a name already declared so that no other
axis answers first:

    a=(5 6 7); echo $(( a[] )); echo after

    bash 5.3, bash 3.2   a[]: bad array subscript   0   after
    ksh93                5   after
    zsh 5.9.2            invalid subscript          (nothing after)

Three answers that part on all three of the wording, the value and
whether the input survives, so it is a policy type
(`EmptyArithSubscriptPolicy`) and not a switch. dash has no arithmetic
subscript at all and never reaches it.

The array in that probe is load-bearing. bash's value is a flat **zero**
and ksh93's is the *element the subscript names* — an empty expression
is zero, so `a[]` is `a[0]` there, and `(( a[]++ ))` steps the first
element. An earlier reading called ksh93's answer "zero"; against an
unset name every column reads zero and no probe could have caught it.

zsh's sentence is the subscript machinery's own rather than the
expression parser's: `invalid subscript`, with no `bad math expression`
in front of it and no name after it, where an expression that will not
parse in the same position carries that prefix.

**`EmptyParamSubscriptIsAnError`** — bash yes · dash unspecified · ksh93 no · zsh yes

Refuses `${a[]}` — the same brackets one construct over, where a
*parameter expansion* reads them instead of an expression. Five of the
six columns refuse and each ends the input; one reads it.

Measured 2026-09-11, on a script file so that the column which gives up
only the *line* can be told from the ones that give up the shell:

    s=hi
    echo "[${s[]}]"
    a=(5 6 7)
    echo "[${a[]}]"
    echo after

    bash 5.3, as sh, 3.2   [${s[]}]: bad substitution   …then `after`, status 0
    dash                   Bad substitution             (nothing after)
    ksh93                  [hi]  [5]  after             status 0
    zsh 5.9.2              invalid subscript            (nothing after)

ksh93 reads it by the rule it reads the arithmetic one with: the
brackets hold an expression that happens to be empty, which is zero, so
`${a[]}` is the first element and `${s[]}` on a scalar is the scalar.
That scalar row is what says the answer is the **element** and not the
number — a policy worded as "zero" would have given `0` where the shell
gives `hi`.

**The question is asked of the written brackets and answered before
anything is expanded**, which is the opposite of the arithmetic site. A
subscript that *arrived* empty is a different construct with an answer of
its own — see `EmptySubscriptTextIsAMathError` below, where the columns
fall two against one rather than five against one, and where the shell
that refuses both gives them different sentences. `${m[""]}` is a third
thing again — an empty *key*, written as two characters, which the
associative columns look up.

The wording is `Diagnostics.EmptyParamSubscript`, and it is empty for
four of the five refusing columns: their sentence is the ordinary
bad-substitution one, subject and all. zsh alone has its own, and it is
the same `invalid subscript` its arithmetic site uses — but a field of
its own, because bash's two sites do *not* coincide. There the
arithmetic one names the subscript and carries on with zero, and this
one refuses the expansion.

The operator makes no difference in any column — `${#a[]}`, `${a[]-d}`,
`${a[]:-d}`, `${a[]/x/y}` and `${a[]%x}` all answer as the bare shape
does — so the question is asked where the subscript is read rather than
once per operator. dash is in the table for completeness and answers
nothing: no subscript reaches a parameter expansion there at all.

**`EmptySubscriptTextIsAMathError`** — bash no · dash unspecified · ksh93 no · zsh yes

Refuses a subscript whose text is **empty or blank once it has been
expanded**, where an expression reads it. `${a[$w]}` with an empty `$w`
is the shape, and `${a[ ]}` is the blank one beside it.

Measured 2026-09-11, `-c`, with `a=(5 6 7)` and `w=`:

    probe       bash 5.3   ksh93u+   zsh 5.9.2
    ${a[$w]}    5, 0       5, 0      bad math expression: empty string, and the shell ends
    ${a[ ]}     5, 0       5, 0      bad math expression: operand expected at end of string

Two columns read the empty text as the expression that is zero; one will
not read it at all. The refusing column gives **two** sentences and not
one — the expression reader complaining about two different positions —
which is why the blank half is worded by `Diagnostics.ArithExpressionRanOut`,
the sentence `$(( a[ ] ))` already earns, and only the empty half needs a
wording of its own, `Diagnostics.EmptySubscriptTextExpanded`.

It reaches every place a subscript is read as a number and not only the
plain expansion: a scalar (`${s[$w]}`), a length (`${#a[$w]}`), a name
nothing declared (`${nosucharr[$w]}` — the absence is no excuse there,
unlike the arithmetic site) and an element assignment (`a[$w]=z`).

Three neighbors keep their answers, and each says where the question is
asked:

- **An association's subscript is a key and never an expression**, so
  `typeset -A m; ${m[$w]}` is the empty string at status 0 in every column
  that has the attribute. The key path is reached before this is asked —
  the ordering `arithElement` keeps for the same reason. bash writes
  `m: bad array subscript` beside that empty string and answers it anyway,
  which is #1972 and not this axis.
- **A substring's offset is not a subscript**: `x=abcdef; ${x:$w:2}` is
  `ab` in every column, the refusing one included. So the question is
  asked where a subscript is evaluated rather than in the arithmetic the
  two sites share.
- **The blanks are the subscript**, not space around one. A subscript is
  trimmed only when it has something in it, which is also what the panel
  does with a blank *key*: bash's `m[" "]` and `${m[ ]}` name the same
  element.

dash has no subscript in a parameter expansion to ask about.

**`BlankArithSubscriptIsTheEmptyExpression`** — bash yes · dash unspecified · ksh93 yes · zsh no

Reads brackets holding whitespace and nothing else — `$(( a[ ] ))` — as
the blank expression, which is zero, so the operand is the *element* that
subscript names rather than a flat zero. It arrives the same way the
empty pair does: `$(( a[$w] ))` with a `$w` holding spaces **is**
`$(( a[ ] ))` by the time the expression exists.

A second axis one text along from `EmptyArithSubscript`, because bash
answers the two apart: `a[ ]` is silently element zero there and `a[]` is
`bad array subscript` and a flat zero. ksh93 gives element zero to both.
zsh refuses this one with the expression reader's own end-of-input
sentence — `operand expected at end of string` — and not the
`invalid subscript` it gives the empty pair.

Asked at the subscript and not at the expression, which is where the
panel actually splits: `$((   ))` is zero in all three of those shells,
so an axis on the blank *expression* would have moved a row they agree
about. Not reached on an associative name, where the subscript is a key
and never an expression, nor on a name that is not set, where
`ArithSubscriptSkippedWhenNameUnset` answers first. The same split holds
with the subscript standing as an assignment target.

**`ArithWholeArraySubscriptIsTheSlice`** — bash no · dash unspecified · ksh93 no · zsh yes

Reads a `*` or `@` subscript inside an arithmetic expression as the
**slice** the same subscript takes in an expansion — the elements joined
on the first character of IFS — rather than as an ordinary subscript.

Measured 2026-09-11, `-c`:

    typeset -A m; m[k]=9; $(( m[*] ))     bash 0 · ksh93 0 · zsh 9
    typeset -A m; m[k]=9; $(( m[@] ))     bash 0 · ksh93 0 · zsh 9
    typeset -A m; m[k]=9; "${m[*]}"       9 in all three
    a=(3); $(( a[*] + 1 ))                bash 1 · ksh93 error · zsh 4

The third row is what makes this an axis rather than a defect on one
side: the *expansion* route agrees everywhere, so the two routes part
inside one shell rather than between two. bash and ksh93 read the
brackets as arithmetic, fail to make a subscript of `*`, and answer zero;
zsh expands the slice first.

The joined text is then read **as an expression** and not as a numeral,
which is the same reading an element's value gets: `a=(1+1);
$(( a[*] * 3 ))` is 6 there, exactly as `$(( a[1] * 3 ))` is. So a slice
of more than one element is usually a *failure* rather than a number —
`a=(3 4 5); $(( a[*] ))` is ``operator expected at `4 5''`` — and that is
the answer rather than a defect in it. An empty array joins to nothing
and is zero; a name that is not set never reaches the brackets at all,
which `ArithSubscriptSkippedWhenNameUnset` answers first.

`@` joins on IFS here exactly as `*` does — `a=(3 4); IFS=:` gives both
spellings `3:4` to read — so this is not the unquoted-`@` question, which
is about field splitting and has none to be about inside an expression.

Asked **before** the association is consulted, because the key `*` is
precisely what the other answer makes of the same text: a table read
first would answer the key and the two readings would collapse into one.
That is the ordering a subscript that is not a number already follows,
with this question inserted at its front.

The two columns that answer no *report* the subscript on an indexed name
— `a[*]: bad array subscript` in bash, a syntax error in ksh93 — and are
silent on an associative one. That is a diagnostic of its own (#1978)
rather than this axis, which is about the value.

**`ArithSubscriptSkippedWhenNameUnset`** — bash no · dash no · ksh93 no · zsh yes

Looks the name up before it reads the brackets, and answers zero for a
name that is not there without evaluating the subscript at all. zsh
alone:

    echo $(( nodecl[1/0] ))          zsh 0 · the other three divide by zero
    i=0; echo $(( nodecl[i++] ))     zsh leaves i at 0 · the others step it

It is not a rule about empty subscripts, and it is what answers one.
`$(( m[$w] ))` with an empty `$w` on a name nothing declared never
reaches the brackets, so the operand is the plain unset zero
`$(( nosuchvar ))` is — which is why the same text is silent there and
`invalid subscript` on a name that exists. Modeling that as a special
case for the empty subscript would have been a rule no probe could tell
from this one: the two agree on every empty-subscript row and part only
on a subscript that errors or assigns.

What "not there" means is set-ness and not emptiness. `e=` then
`$(( e[1/0] ))` divides by zero in zsh too, and so does an array
declared with nothing in it — and a scalar, an integer and `PATH` all
take the `invalid subscript` refusal, which is what says the question is
whether the name exists rather than whether it is an array.

No answer reads as no, which is the majority and the harmless side: a
subscript with no error and no side effect gives the same zero either
way, so an unanswered preset is not refused over `$(( a[0] ))`.

**`ArithSubscriptSkippedWhenNameUnset`** — bash no · dash no · ksh93 no · zsh yes

Looks the name up before it reads the brackets, and answers zero for a
name that is not there without evaluating the subscript at all. zsh
alone:

    echo $(( nodecl[1/0] ))          zsh 0 · the other three divide by zero
    i=0; echo $(( nodecl[i++] ))     zsh leaves i at 0 · the others step it

It is not a rule about empty subscripts, and it is what answers one.
`$(( m[$w] ))` with an empty `$w` on a name nothing declared never
reaches the brackets, so the operand is the plain unset zero
`$(( nosuchvar ))` is — which is why the same text is silent there and
`invalid subscript` on a name that exists. Modeling that as a special
case for the empty subscript would have been a rule no probe could tell
from this one: the two agree on every empty-subscript row and part only
on a subscript that errors or assigns.

What "not there" means is set-ness and not emptiness. `e=` then
`$(( e[1/0] ))` divides by zero in zsh too, and so does an array
declared with nothing in it — and a scalar, an integer and `PATH` all
take the `invalid subscript` refusal, which is what says the question is
whether the name exists rather than whether it is an array.

No answer reads as no, which is the majority and the harmless side: a
subscript with no error and no side effect gives the same zero either
way, so an unanswered preset is not refused over `$(( a[0] ))`.

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

**`FirstAllocatedDescriptor`** — bash from ten · dash from ten · ksh93 from
ten · zsh **from eleven**

The number the shell counts up from when it picks a descriptor for
itself. Measured 2026-09-12, `exec {fd}< /etc/hosts; echo $fd`: bash
5.3.15 and ksh93 say 10, zsh 5.9.2 says 11; dash and bash 3.2 have no
`{name}` token. The same number is what `zsocket` reports in `$REPLY`
and what `sysopen -u name` writes, so it is visible from three builtins
as well as from the redirection.

**The one axis with no unanswered value.** Every other field can be left
unanswered and refused by name at the disagreement; there is no shape in
which a shell declines to pick a number, so a zero value that refused
would refuse a construct every shell performs.

**`ReadFailureInAFileSubstitutionFailsIt`** — bash no · dash no · ksh93
no · zsh **yes**

`$(<file)` whose *read* fails after the open worked — a directory is the
reachable shape. Measured 2026-09-12, `mkdir dir; v=$(<dir)`: zsh is
status 1 with `error when reading dir: is a directory`, bash 3.2 is
status 1 in silence, and bash 5.3 and ksh93 leave the status at 0 with
nothing said. bash 3.2 is what makes the status and the sentence two
questions rather than one; the sentence is
`Diagnostics.FileSubstitutionReadError`. An open that fails is a
different event and is already `redirectFailureStatus`.

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

**`GetoptsPositionIsFunctionLocal`** — bash no · dash no · ksh93 no · zsh yes

Gives every shell function call its own `getopts` cursor: `OPTIND` starts
the call at 1 whatever the caller had reached, and the caller's position
comes back when the call returns.

It is the *parameter* that is local and not only the builtin's
bookkeeping, which a snippet with no `getopts` in it shows
(`getopts/an-assignment-to-optind-inside-a-function`):

    g() { echo "entry=$OPTIND"; OPTIND=7; }
    OPTIND=3; g; echo "after=$OPTIND"

answers `entry=1 after=3` in zsh and `entry=3 after=7` in the other five.
The position *inside* a clustered word travels with the value, which a
shared cursor cannot express: with `-ab` half read, a function scanning
its own `-cd` reads both `c` and `d` in zsh and only `d` everywhere else,
and the caller still finds its `b` on return
(`getopts/a-functions-cursor-inside-a-clustered-word`). Each call has its
own, so nesting unwinds frame by frame rather than through one saved copy
(`getopts/a-nested-call-has-its-own-cursor`), and zsh's *anonymous*
function gets one too — it is the call that localizes, not the `function`
word (`getopts/an-anonymous-function-has-its-own-cursor`).

dash is the near-miss and has to be told apart deliberately: its
`getopts` restarts a scan that found no option where it was pointed, so
it can also reach 1 on a second call
(`getopts/dash-restarts-a-scan-that-found-nothing`). The signatures
differ — dash's 1 is visible outside the function as well, where zsh
reads 2 inside the call and 1 outside, which only a restore produces. A
fix that reset the cursor whenever a scan came up empty would match
dash's row and still leave the bug below in place.

Two limits, both measured, and both silences rather than values. A call
entered with `OPTIND` *unset* is not handed a cursor at 1
(`getopts/an-unset-optind-is-not-a-fresh-cursor`), and a call that unsets
`OPTIND` itself does not get the caller's back
(`getopts/unsetting-optind-in-a-function-keeps-it-gone`): `unset` takes
this parameter away rather than emptying it, so there is nothing left to
restore. What decides is the state of the name when the call *returns*
and not whether an `unset` was executed — unset and then assigned again,
zsh still restores the caller's value
(`getopts/optind-unset-then-assigned-in-a-function`). dash refuses to
unset the name at all, complaining `unset: Illegal number:` about an
argument it read as a count, so those rows are agreed by the five that
can reach the question.

Why it is an axis and not a curiosity: a shell function that parses
options is only reusable if the second call starts over, so zsh's own
function library is written *without* the `local OPTIND=1` the others
need. `add-zsh-hook -Uz precmd f` followed by any second `add-zsh-hook`
had the second call reading its arguments from index 2 and printing its
usage, which is what a shared cursor does to a script that never asked
for one (`getopts/an-option-parsing-function-called-twice`).

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

**`ShiftNamesAreArrays`** — bash no · dash no · ksh93 no · zsh yes

Reads `shift`'s operands as the names of arrays to shift, instead of the
positional parameters. zsh's synopsis is `shift [ n ] [ name ... ]`; the
others have one operand and it is the count.

    a=(1 2 3); shift a          zsh   a=(2 3), status 0
                                bash  `a: numeric argument required`
                                ksh93 `a: bad number`
                                dash  no array syntax to write it with
    a=(1 2 3 4); shift 2 a      zsh   a=(3 4)
                                bash  `shift: too many arguments`
                                ksh93 `2: bad number`

The count stays optional in front of the names, so the first word is
ambiguous — and zsh settles it by **type**, not by whether the word looks
like a number:

    q=5;   a=(1 2 3 4 5 6); shift q a   a=(6)           q was a count
    q=(5); a=(1 2 3 4 5 6); shift q a   q=() a=(2 3 4 5 6)  q was a name

An array is a name; anything else — a scalar, an association, a name that
was never set — is a count, evaluated as arithmetic the way any other
count word is. A scalar `s=1` shifts the positional parameters by one and
a scalar `s=9` overruns them, which is what tells the two readings apart:

    set -- x y z; s=1; shift s   [y z]
    set -- x y z; s=9; shift s   `shift count must be <= $#`, status 1

What protects the positional parameters is giving **any** operand, not the
operand turning out to name an array. `set -- x y z; shift 1 nosuch`
leaves `$@` alone at status 0, and so does `shift nosuch` — a name that
is not an array is passed over in silence rather than complained about.

One count applies to every name, and a count past the end of one of them
is the same `ShiftTooMany` complaint the positional reading makes —
naming `$#` even though the operand is an array — without stopping the
names behind it. `a=(1 2 3); b=(x y); shift 3 a b` empties `a`, leaves
`b`, prints one line and exits 1. `ShiftPastEndFatal` is no in zsh, which
is what lets the loop carry on past a name that overran.

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

**`AttributeRereadsTheValueItFinds`** — bash no · dash unspecified · ksh93 yes · zsh yes

Makes an attribute a declaration adds re-read the value the name is
already holding, on the spot, rather than waiting for the next
assignment.

    FOO=bar;   typeset -i FOO      bash [bar]     ksh93, zsh [0]
    d=MiXeD;   typeset -u d        bash [MiXeD]   ksh93, zsh [MIXED]
    e=MiXeD;   typeset -l e        bash [MiXeD]   ksh93, zsh [mixed]

Measured 2026-09-06, `env -i PATH=/usr/bin:/bin` with a scratch `HOME`,
from a script file and through `-c`, and in zsh under `emulate sh`,
`emulate ksh` and `emulate zsh` alike. The three bash columns answer no
and both of the others answer yes; dash has neither builtin. The issue
that filed this had zsh on bash's side and had `-u` as unanimous, and
both were wrong — the measurement is what made it one field instead of a
rule plus an axis.

**Both answers lose data, which is what makes it a field.** A shell that
re-reads destroys text: `bar` is an expression made of an unset name, so
0 is what the name holds afterwards. A shell that does not leaves a name
declared integer holding text that is not a number. There is no reading
under which both are kept, so a dialect has to say which shell it is.

**One question over every letter that has something to say about a
value**, not one per letter. The shells that re-read `-i` fold `-u` and
`-l` on the spot too, and the shell that does not, does not; `-x`, `-r`
and `-a` say nothing about a value and never reach it. Splitting them
would have been three fields whose answers can only ever agree.

The re-read canonicalizes rather than merely evaluating, which is where a
narrower reading goes wrong: `08` is 8 and not an octal complaint, `" 7 "`
loses its spaces, `+7` its sign, `5+2` is 7, and an empty value is 0.

**The question has to be asked without evaluating.** Folding first to
find out whether the two readings differ makes the question's own answer
depend on the dialect's, in the loud direction: `FOO=08; typeset -i FOO`
reported `value too great for base` and failed at 1 under the bash
answer, where real bash reads `08` back in silence. So the predicate asks
the narrower "is this text already the canonical spelling of itself",
which is pure.

Asked only where a name is *already* holding something in the cell being
declared. A declaration that creates the name has nothing to re-read, and
inside a function the cell a shadow just made is new whatever the caller
held — that is `DeclaredNameWithoutValueIsEmpty` and not this, and the
two were one branch until this field existed, so zsh got the re-read by
accident of answering that question yes and ksh93, which answers it no,
never reached it at all.

The *next* assignment is unanimous and no part of this: `typeset -i a;
a=3+4` is 7 and `typeset -u d; d=again` is AGAIN in every shell that
spells the letter. What is asked is only whether the attribute reaches
backwards.

Not an assignment, so it does not meet the readonly refusal: `typeset -r
r=1; typeset -i r` is 1 at status 0 in the two shells that re-read.

Pinned by `declare/an-attribute-re-reads-the-value-the-name-already-holds`,
`declare/an-integer-attribute-over-a-number-written-oddly`,
`declare/an-attribute-that-would-change-nothing-needs-no-dialect`,
`declare/an-attribute-over-a-cell-a-function-just-shadowed` and
`declare/an-attribute-added-to-an-exported-name-reaches-the-child`, the
last of which asks it of a *child* — so the re-read is a change to the
value and not a way of rendering it.

Two divergences beside it, measured and not modeled: ksh93 folds an
zsh renders `0x10` under `-i` as `16#10` where ksh93 gives `16`; that one is
open as its own question.

**`CompoundAttribute`** — bash keeps the elements · dash unspecified · ksh93 folds every element · zsh replaces it with a scalar

The same question of a value that is *compound* — an array or a keyed
table — and it splits the panel **three** ways where the scalar one splits
it two. The two shells that share the scalar answer disagree with each
other about what reaching back into an array even means.

    arr=(a b); typeset -i arr      bash `a b`   ksh93 `0 0`   zsh `0`, one element
    brr=(a b); typeset -u brr      bash `a b`   ksh93 `A B`   zsh `a b`
    typeset -A m; m[k]=v
    typeset -i m; export m         bash `v`, nothing to the child
                                   ksh93 `0`, nothing to the child
                                   zsh   empty, and the child is told `m=0`

Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch `HOME`,
`ZDOTDIR` and `HISTFILE`, from a script file.

zsh's answer is the strangest and the row worth repeating: the array does
not keep its length — `${#arr[@]}` is 1 afterwards — and a later
`arr[0]=3+4` is refused, the name no longer being an array. It is a
**fresh** scalar and not a fold of anything: `(7 8)`, `(x y)` and
`(0x10 9)` all leave `0`, which is exactly what that dialect's
`DeclaredNameWithoutValueIsEmpty` = yes gives a name it has never held. So
nothing is stored for it here; the compound value is discarded and the
declaration carries on into the branch a fresh name takes.

**The letter matters for that answer and for that one only.** A shell
replaces the compound because the letter changed what *kind* of name it
is, and a case letter changes no kind: in zsh `typeset -u` over an array
leaves two elements where `typeset -i` leaves one. The other two answers
apply to every letter alike.

The keyed table is the row a scalar-only reading cannot produce. An array
keeps its first element in the scalar table, so a scalar re-read of it
rewrites a copy nothing reads back; a table keeps nothing there, so the
same slip invents a scalar — and `export m` then hands a child `m=0` for a
name with no scalar value at all.

Not a widening of `AttributeRereadsTheValueItFinds`: bash is the one
column where the two questions cannot be told apart, and the other two
part company across both spellings. It sits in the family `ArraysAreSparse`
and `ArrayBaseIsZero` belong to.

Pinned by `declare/an-attribute-over-an-array-is-three-answers`,
`declare/an-attribute-over-a-keyed-table` and
`declare/an-array-becoming-a-scalar-keeps-nothing`.

**`ScalarUnderAnArrayDeclaration`** — bash becomes the first element · dash unspecified · ksh93 stays a scalar · zsh discards it

**`ScalarUnderATableDeclaration`** — bash becomes the first element · dash unspecified · ksh93 becomes the first element · zsh discards it

The **converse** of `CompoundAttribute`: what an array or table *letter*
makes of a scalar the name is already holding. It splits the panel three
ways as well, and no two answers lose the same thing.

    b=1; typeset -a b     bash `declare -a b=([0]="1")`  n=1  `$b` is 1
                          ksh93 `b=1`                    n=1  `$b` is 1
                          zsh   `typeset -a b=( )`       n=0  `$b` is empty
    a=1; typeset -A a     bash `declare -A a=([0]="1" )` n=1
                          ksh93 `typeset -A a=([0]=1)`   n=1
                          zsh   `typeset -A a=( )`       n=0

Measured 2026-09-08, panel and machine as `oracle.md`. dash has neither
letter; bash 3.2.57 answers `-a` as bash 5 does and has no `-A`.

**Two fields because ksh93 answers the two letters differently**, and it
is the whole of why: bash promotes under both and zsh discards under
both, so one field would have had to be wrong for one of ksh93's letters
whichever way it was set.

ksh93's `-a` answer is a genuine third state and not a rendering of
bash's, which two probes say. A one-element array of that shell's own
making *does* carry the letter — `b=(1); typeset -p b` is
`typeset -a b=(1)` — and a bare `typeset -a`, which lists every name
carrying the attribute, prints nothing at all after `b=1; typeset -a b`.
The declaration recorded nothing and converted nothing; the name is still
the scalar it was, and that shell lets a scalar be subscripted, which is
why `${b[0]}` and `${#b[@]}` cannot tell it from bash's promotion.

Asked only where the name is holding a scalar in the cell being declared.
An unset name is unanimous — `unset b; typeset -a b` is an array of no
elements in all three — and so is a name already holding an array, which
every column leaves standing. A declaration carrying its own value is not
this question either: `b=1; typeset -a b=(9)` is the one element `9`
everywhere.

**A local declaration builds the array cell rather than converting one**,
and that is core rather than a fourth answer. bash promotes at the top
level and through `-g` — `b=1; typeset -a b` and
`b=1; f() { typeset -ga b; }; f` both list `declare -a b=([0]="1")` —
and empties under every local spelling, with `f() { local b=1; local -a b; }`,
`local b=1; declare -a b` and `local b=1; typeset -a b` all
`declare -a b=()`. The shadow being *fresh* is not what decides it, since
`local b=1` has already taken the copy, and it is not the letter in
general, since `local b=1; local -i b` keeps the `1`. No dialect is asked:
zsh discards a held scalar wherever it finds one and ksh93 declares
nothing local in a plain function, so the two shells with an answer of
their own reach the same place by their own route.

It shows through an append, which is how it was found:
`a=1; typeset -A a; a+=([k]=v)` is two entries in bash and ksh93 and one
in zsh, because the value is already placed or already gone by the time
the append runs. This implementation answered every column with zsh's,
which matched one shell by accident and lost the script's own value in
the other two at status 0 (#1572).

Pinned by `declare/an-array-declaration-over-a-name-holding-a-scalar`,
`declare/a-table-declaration-over-a-name-holding-a-scalar`,
`declare/an-array-declaration-over-an-empty-scalar`,
`declare/an-array-declaration-over-an-unset-name`,
`declare/an-array-declaration-over-a-name-already-holding-an-array`,
`declare/an-array-declaration-over-a-scalar-holding-a-space`,
`declare/a-table-declaration-over-a-scalar-then-appended` and
`declare/a-local-array-declaration-over-a-local-scalar`.

**`ScalarAppendedToAnArrayBecomesANewElement`** — bash no · dash unspecified · ksh93 no · zsh yes

Where `a+=x` puts the value when the name is holding an **array**: joined
onto the first element, or added after the last.

    a=(1 2); a+=x     bash `([0]="1x" [1]="2")`  n=2
                      ksh93 `(1x 2)`             n=2
                      zsh   `( 1 2 x )`          n=3

Measured 2026-09-08; bash 3.2.57 and bash under argv[0] of `sh` answer as
bash 5 does, and dash has no arrays.

The join is at the **base** and not at the lowest subscript standing:
`a=([5]=q); a+=x` is `declare -a a=([0]="x" [5]="q")`, so an element grows
at 0 where there was none and `q` does not move. It is `a[0]+=x` and not
"append to the first element there is", which are the same operation on a
dense array and different ones here. The empty string is a value on both
sides — `a=(1 2); a+=""` leaves bash's two elements alone and gives zsh a
third that is empty — and the appended value is one value however many
words it looks like.

Asked only where the name is holding an array. A name holding a scalar or
holding nothing is the ordinary string append and stays a plain scalar in
every column, and the array-literal spelling `a+=(x)` is not this question
either: it adds an element in every shell that has arrays, which is why
that one has no field. Note that zsh's answer here is **not** its answer
to `a+=(x)`.

A plain `a=x` over an array is a third question and is open (#1390): bash
and ksh93 write the first element and leave the rest, zsh replaces the
array with a scalar.

Pinned by `array/appending-a-scalar-to-an-array` and the five rows beside
it. This implementation deleted the array and left a plain string — `1x`
under bash's reading and `1 2x` under zsh's, the scalar *view* of the
whole array with the value stuck on the end — at status 0 (#1571).

**`CompoundElementsGoThroughTheAttribute`** — bash yes · dash unspecified · ksh93 yes · zsh no

Folds what is written to *one element* of an array or a keyed table
through the attribute the name carries, the way a scalar assignment
already does everywhere.

    typeset -ia a=(1 2); a[1]=3+4       bash, ksh93 `1 7`
    typeset -ua q=(ab cd); q[1]=ef      bash, ksh93 `AB EF`   zsh `ef cd`
    typeset -A m; typeset -i m
    m[k]=7+7                            bash, ksh93 `14`

A *later write* rather than a reach-back, which is why it is a separate
question from the one above: bash answers no there and yes here, exactly
as it does for a scalar. For a scalar the write needs no dialect at all —
`typeset -i n; n=3+4` is 7 and `typeset -u d; d=again` is AGAIN in every
shell that spells the letter — and an element is where the panel splits.

zsh does not fold an array's elements at all: its case letters reach a
scalar's expansion and stop there, and its integer letter never meets an
array, having replaced it under the field above. So the answer is read off
the case letters, which are the only ones that dialect can be asked about
here.

Asked only where the name carries one of the three letters and a value the
fold would change, so an ordinary array never needs a dialect and an array
of canonical numbers under `-i` needs none either.

Pinned by `declare/an-element-written-through-the-names-attribute` and
`declare/an-append-and-a-declarations-own-literal-both-fold`.

**`ArrayLiteralAssignmentStartsTheNameOver`** — bash no · dash unspecified · ksh93 yes · zsh no

Makes `a=(x y)` *re-create* the name — the attributes it carries and all —
rather than replacing only its elements.

    typeset -ia z=(1); z=(5+5 6+6); typeset -p z; z[0]=3+4
      bash    declare -ai z=([0]="10" [1]="12")   then `7 12`
      ksh93   typeset -a z=(5+5 6+6)              then `3+4 6+6`

ksh93 keeps neither the fold nor the letter: the listing has lost the
`-i`, and the element write after it is not folded either — which is what
says the attribute is **gone** rather than merely bypassed by the one
assignment that replaced the elements. The case letters answer the same
way (`typeset -a q=(gh ij)` in ksh93 against bash's `declare -au`), and
zsh, which can only be asked through a case letter, keeps it.

The same idea `unset` is a rule about and that
`InheritedValueSurvivesADeclaredType` is the other side of: a name whose
whole value is replaced may be a *new* name in one of these shells.

Asked only for three things at once, and each is what a wider reading gets
wrong:

- The **plain assignment** spelling. A declaration's own operand —
  `typeset -ia d=(5+5 6+6)` — folds to `10 12` in both, so the letters
  cannot have gone there. `syntax.Assign.Operand` tells the two apart.
- Not an **append**: `f+=(8+8)` is 16 in both.
- Not a **keyed** literal: `typeset -A m; typeset -i m; m[k]=1;
  m=([j]=2+2)` keeps the attribute in both and folds the `2+2` to 4.
- The name must have something to **start over**. The *first* array
  literal a declared name receives keeps the letter and folds in both —
  `typeset -ia b; b=(5+5 6+6)` is `10 12` and lists as `typeset -a -i
  b=(10 12)`.

One measured shape is left out by that last reading and is recorded rather
than modeled: `typeset -i a; a=(5+5 6+6)`, where the declaration named no
array letter at all, drops the attribute in ksh93 even though `a` was
holding nothing. What ksh93 turns on there is whether `-a` was written,
and this engine does not record that letter — an array is dynamic here, so
`typeset -a arr` needs no record to work. Recording it to reach that one
shape is a change to that decision rather than part of this one.

Pinned by `declare/a-whole-array-assignment-re-creates-the-name`,
`declare/a-keyed-table-replaced-keeps-its-attribute` and
`declare/an-append-and-a-declarations-own-literal-both-fold`.

**`InheritedValueSurvivesADeclaredType`** — bash yes · dash unspecified · ksh93 no · zsh yes

Keeps the value a name was born with when a declaration gives it a
*type* — `-i`, `-u` or `-l` — that the same declaration does not also
export or freeze.

The **third** answer to the question above, on the one input where the
two shells that agree about a scalar part company:

    INHERITED=bar, then `typeset -i INHERITED`
      bash    [bar]   the child is told INHERITED=bar
      zsh     [0]     the child is told INHERITED=0
      ksh93   unset   the child is told nothing at all

Measured 2026-09-07, `env -i PATH=/usr/bin:/bin INHERITED=bar` with a
scratch `HOME`, `ZDOTDIR` and `HISTFILE`, from a script file, reading the
child's view through `env | grep`.

bash and zsh are the field above answering no and yes, and both still
hand the name down. **ksh93 does neither**: the value goes *and* the
export goes, so the name reaches no child. Modeling that as the re-read
gives the wrong answer twice over — a re-read produces 0 and keeps the
export, and this produces neither.

Nor is it "a declaration empties what it touches". `${INHERITED+SET}` is
empty afterwards, so the name has no value rather than an empty one —
which is exactly what that shell's `DeclaredNameWithoutValueIsEmpty` = no
leaves a **fresh** name holding. The declaration is starting the name
over, and everything after it follows: the attribute is intact
(`INHERITED=3+4` reads 7), `typeset -p` says `typeset -i INHERITED` with
no value, and a later assignment does not put the export back.

Asked only where every part of the shape holds, and each part is what a
narrower or wider reading gets wrong:

- The value is the one the shell was **started with** and the script has
  never assigned. `D=$D; typeset -i D` is 0 with the export kept in all
  three, and so is `export FOO=bar; typeset -i FOO` — so the question is
  about where the value lives, not about the export attribute.
- A **type** letter arrived. A bare `typeset`, `typeset -x` and
  `typeset -r` leave an inherited name entirely alone in every shell.
- Whether the fold would change anything is **not** part of it, which is
  where this parts company with the field above: an inherited `7` meeting
  `-i` and an inherited `UPPER` meeting `-u` are discarded too. So it is
  asked *ahead* of the canonical-spelling predicate rather than behind
  it, where the shape's commonest spellings would have gone unanswered.
- The **same command** must not also name `-x` or `-r`. `typeset -ix G`
  and `typeset -ir K` keep the value and re-read it; splitting them in
  two — `typeset -x P; typeset -i P` — discards it, and so does any other
  extra letter, measured with ksh93's `-t`. It is the letters this command
  carries and not the ones the name already has.

Pinned by `declare/a-type-over-a-name-the-shell-was-started-with`,
`declare/a-type-over-an-inherited-name-a-fold-would-not-touch`,
`declare/an-inherited-name-started-over-is-a-fresh-one`,
`declare/an-inherited-name-the-same-declaration-also-exports` and
`declare/an-inherited-name-the-script-assigned-first`.

**`DeclarationNameOperands`** — bash PlainNamesOnly · dash PlainNamesOnly · ksh93 PlainNamesOnly · zsh NamesAndSpecialParameters

Says what may stand where `export` and `readonly` want a name, beyond a
plain name itself.

zsh is the only one that takes anything more: the special parameters are
names to it, which is why `export -` is a complaint in three of the four
and not in the fourth.

**`DeclarationTakesASubscript`** — bash no · dash no · ksh93 yes · zsh yes

Accepts `export a[0]` and `readonly a[0]`, naming an element rather than
a variable. ksh93 and zsh do; bash and dash refuse it in the words they
give any other bad name.

A separate question from the name strictness above, because the answer
is per builtin: bash refuses it here and takes it for `unset`, and the
two builtins sit on different strictnesses in every shell, so no rule
over that strictness gives all four.

zsh's cell read *no* until 2026-09-07 and was never reachable: a
declaration's operand went through pathname expansion first, so
`export a[1]=v` died as `no matches found` before the name check ran
(#1203). `local` reads this family's *other* answer,
`TypesetTakesASubscript`, which is what bash needs — it takes
`local a[1]=v` and refuses `export a[1]=v`.

**`BadNameDeclaresTheOperandsAfterIt`** — bash no · dash no · ksh93 yes · zsh yes

Keeps declaring past an operand the builtin refused, where the refusal is
fatal. Asked only on the fatal path — bash reports each bad operand and
carries on, so its whole list is declared by the loop rather than by
this, and the answer recorded for it is the one bash-as-`sh` gives, which
is the same binary with the fatality turned on.

Measured 2026-09-07 from a script file, reading the names back from an
**EXIT trap**: a reader written on the next line never runs in the four
columns that stop, so "nothing was declared" and "the script ended" look
identical without one. The position of the bad name is the variable, and
only the middle position can tell the two answers apart.

| `export … ok1=1 … ok2=2 …`, bad name at | bash 5.3 | bash-as-`sh` | dash | ksh93u+ | zsh 5.9.2 |
| --- | --- | --- | --- | --- | --- |
| first | `[1][2]`, not fatal | `[U][U]` | `[U][U]` | `[1][2]` | `[1][2]` |
| middle | `[1][2]`, not fatal | `[1][U]` | `[1][U]` | `[1][2]` | `[1][2]` |
| last | `[1][2]`, not fatal | `[1][2]` | `[1][2]` | `[1][2]` | `[1][2]` |

The same holds for `typeset`, `readonly` and `unset` — a fatal
`unset ok1 ":" ok2` removes both names before it stops — and for `local`
where the shell has one. `set -A` takes a single name and so has nothing
to keep.

The operands behind the refusal are collected **in silence**: every
column that stops writes exactly one diagnostic however many bad names
follow, so judging them again would add a line no shell writes. bash,
which does not stop, writes one per bad operand.

This was `nil`: a fatal refusal handed the caller no operands at all, so
a declaration with one bad name among good ones declared none of them and
a sourced file left the caller without the names it had set (#1211).

Pinned by `declare/a-bad-name-among-good-ones`,
`export/a-bad-name-in-front-of-good-ones`,
`export/a-bad-name-among-good-ones`,
`export/a-bad-name-behind-good-ones`,
`readonly/a-bad-name-among-good-ones`,
`unset/a-bad-name-among-names-to-remove` and
`declare/a-bad-name-with-more-bad-names-behind-it`.

### A declaration whose operand names an element

`typeset a[1]=v` writes the element in every column that takes the
operand at all, and its name half is never a pattern — measured with a
file literally named `a1=v` on disk, which is the only shape that can
tell an assignment from a word that happens to have no match. What the
declaration does to the *array* beside writing that one element is where
the panel splits, and it splits three separate ways.

Measured 2026-09-07, `env -i` with a scratch `HOME`, `ZDOTDIR`,
`HISTFILE` and `ENV`, from a script file.

| declaration | bash 5.3 / as-`sh` / 3.2 | ksh93u+ | zsh 5.9.2 |
| --- | --- | --- | --- |
| `typeset a[1]=v` | `[v]` | `[v]` | `[v]` |
| `typeset -x a[1]=v` | `[v]` | `[v]` | `[v]` |
| `typeset -g a[1]=v` | `[v]` | *no `-g`* | `[v]` |
| `export a[1]=v` | bad name | `[v]` | `[v]` |
| `typeset -i a[1]=0x10` | `[16]` | `[16]` | **refused** |
| `typeset a[1]=v` in a function | local array | the caller's | **refused** |
| `readonly a[1]=v` | bad name | `[v]`, then frozen | **refused** |
| `typeset -r a[1]=v` | frozen empty, then `a: readonly variable`, status 0 | `[v]`, then frozen | **refused** |

zsh words the three refusals differently, which is how a script tells
which of them it ran into: `can't create readonly array elements`,
`inconsistent array element or slice assignment` and `can't create local
array elements`. All three end the script.

**`SubscriptedOperandTakesTheIntegerAttribute`** — bash yes · dash absent · ksh93 yes · zsh no

Lets `typeset -i a[1]=0x10` give the array the integer attribute and
store the converted value under the element. zsh refuses the operand
instead: an element is not a name there, and the attribute belongs to
the name.

**`SubscriptedOperandTakesALocalDeclaration`** — bash yes · dash absent · ksh93 yes · zsh no

The same split for a declaration inside a function, and a separate field
because it is a different thing being done to the variable. bash makes
the array local and holds the element in the local one; ksh93 has no
scope to take and writes the caller's array, which is its ordinary answer
about scope rather than anything about subscripts; zsh refuses.

Asked only where there is a scope to take, so a declaration at the top
level never reaches it. Folding the two fields into one would have given
zsh's refusal to whichever of the two the other shell was measured for.

**`ReadonlyElement`** — bash unspecified · dash absent · ksh93 written · zsh refused

What a declaration does when it would freeze the array whose element its
operand names. Three answers rather than two, which is why it is a policy
and not an `Answer`, and only two of the three are modeled.

bash's is the third: `typeset -r a[1]=v` creates the array **frozen and
empty**, then reports `a: readonly variable` about the element write it
has just made impossible, and reports success — `declare -p a` reads back
`declare -ar a=()` and `$?` is 0. Implementing it needs the freeze to
happen before the write rather than instead of it, which is a change to
the order every other declaration keeps; it is measured and recorded here
and left unanswered, so the combination refuses by name in that dialect
rather than doing something plausible in silence. `readonly a[1]=v` never
reaches the question in bash, which refuses the operand as a bad name.

The refusal is asked without a value as well — `readonly "a[1]"` is
refused in the same words as `readonly a[1]=v` — because it is about the
attribute rather than about the assignment.

Pinned by `declare/a-subscripted-operand-to-a-declaration`,
`declare/a-subscripted-operand-is-not-a-pattern`,
`declare/a-subscripted-operand-with-an-expanded-subscript`,
`declare/a-subscripted-operand-with-the-integer-attribute`,
`declare/a-subscripted-operand-inside-a-function`,
`declare/a-readonly-subscripted-operand`,
`declare/an-exported-subscripted-operand` and
`local/a-subscripted-operand-to-local`.

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
from `UnsetNameAtIsOneEmptyField`, which is about how many *fields* a
quoted `"${a[@]}"` on a name holding nothing makes and is reached only
once this one has taken the array away.

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

**`ReadNameOperands`** — bash PlainNamesOnly · dash PlainNamesOnly · ksh93 PlainNamesOnly · zsh NamesAndPositionals

Is that question for `read`, and is a fourth field because zsh gives
`read` a fourth answer: `read 1` fills `$1` there, where `export 1` and
`unset ?` are both refused and `read ?` is not.

Every shell in the panel refuses a word that is not a name — that is not
the axis, and the refusal itself is the core's. `read` took such a word
as a variable name in silence, at status 0, until #1440. What splits the
panel is only how far the set reaches past a plain name.

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

**`ReplacementOperandTakesTheEnclosingQuoting`** — bash no · dash unspecified · ksh93 no · zsh yes

Reads the replacement half of `${v/pat/repl}` as *content* of the quoting
around the expansion rather than as a word of its own. With `s=xay` and
`v=VAL`, from a script file:

| probe | bash 5.3 | bash-as-`sh` | bash 3.2 | ksh93 | zsh 5.9.2 |
| --- | --- | --- | --- | --- | --- |
| `"${s/a/'$v'}"` | `x$vy` | `x$vy` | **`x'VAL'y`** | `x$vy` | **`x'VAL'y`** |
| `"${s/a/\q}"` | `xqy` | `xqy` | **`x\qy`** | `xqy` | **`x\qy`** |
| `"${s/a/~}"` | the home | the home | **`x~y`** | the home | **`x~y`** |
| `${s/a/'$v'}` unquoted | `x$vy` | `x$vy` | `x$vy` | `x$vy` | `x$vy` |

The third of three readings a quote in a `${ }` operand can take, and the
only one the panel divides on: a **word** operand takes the enclosing
quoting unanimously, a **pattern** operand's quotes quote, also
unanimously, and a **replacement** operand splits. So it cannot borrow
either of the other two answers, and neither of them may move with it.

bash moved between its two builds, which is what says a field named for a
shell could not carry this. dash has no `/` operator at all.

Asked only at the disagreement. The two readings coincide unless the
expansion is double-quoted *and* the operand holds one of the three
characters they part on — a single quote, a backslash, or a tilde at the
front. A double quote is removed under both readings, a glob is a glob
under both, and a tilde off the front expands under neither, so none of
those reaches the axis. The parser decides whether it can arise and keeps
both readings when it can; see `syntax.ParamExpr.Arg2Enclosed`.

The **backslash** of those three has to be looked past rather than
counted, because it parts the readings only before a character the
enclosing reading does not escape itself. Measured 2026-09-10 with
`s=xay` and `v=V`:

| probe | bash 5.3 | bash-as-`sh` | ksh93 | zsh 5.9.2 |
| --- | --- | --- | --- | --- |
| `"${s/a/\$v}"` | `x$vy` | `x$vy` | `x$vy` | `x$vy` |
| `"${s/a/\\}"` | `x\y` | `x\y` | `x\y` | `x\y` |
| `"${s/a/\"}"` | `x"y` | `x"y` | `x"y` | `x"y` |
| `"${s/a/\}}"` | `x}y` | `x}y` | `x}y` | `x}y` |
| `"${s/a/\{}"` | `x{y` | `x{y` | `x{y` | **`x\{y`** |
| `"${s/a/\q}"` | `xqy` | `xqy` | `xqy` | **`x\qy`** |

The first four are unanimous and must not reach the axis: `$`, a
backslash and `"` are escaped under both readings because both apply the
double-quote set, and `}` joined that set when the closing brace became
escapable in an operand (#1966). Only the last two part the panel. Asking
the axis on the backslash alone refused `"${s/a/A\}B}"` in the core by
name, where every column agrees on `xA}By` — the same "refusing where the
panel agrees" this axis's narrowness exists to avoid. bash 3.2 keeps the
backslash on all six.

bash 3.2 keeps a double quote as a character too, which no other column
does under either reading. That is a further difference inside the
keeping group rather than a third value of this axis, and no dialect here
targets that build, so it is recorded in the corpus and not modeled.

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

Whether anything is *said* about it is the dialect's wording rather than
a second axis. Asked only when a write has actually failed, so `echo hi`
on an open stream needs no dialect.

It takes **two** wordings, and zsh is why. bash and dash complain
through `Diagnostics.BuiltinWriteError`, which is reached only after
this axis answers yes, and ksh93 fails silently. zsh answers this axis
no and still says something — on every route but one:

| written, zsh 5.9.2 | said | status |
| --- | --- | --- |
| `echo hi >&-` | — | 0 |
| `exec 1>&-; echo hi` | `write error: bad file descriptor` | 0 |
| `exec 1>&-; echo hi >&-` | — | 0 |
| `{ echo a; echo b; } >&-` | the sentence, twice | 0 |
| `f(){ echo hi; }; f >&-` | `f: write error: …` | 0 |
| `exec 1>&-; echo hi 3>&-` | the sentence | 0 |

Measured 2026-09-12. The line is not `exec` against a per-command
redirection: a close the writing command *restates* silences the
sentence even after `exec` parked one, and a close written on a group or
on a function call does not silence the commands inside it. What decides
is whether **this command's own redirection list** closed the stream it
writes to. That is `Diagnostics.InheritedClosedStreamWriteError`,
emitted before the axis is asked — moving `BuiltinWriteError` in front
of the axis instead would make zsh speak for `echo hi >&-` as well,
which is the row that is right today.

The control is a write that did not fail: `exec 1>&-; true` says nothing
in all six. A write that failed on a stream *nobody* closed does reach
the sentence — with SIGPIPE arranged away, zsh says `write error: broken
pipe` for a builtin writing into a pipe whose reader has gone.

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

## An array literal through a subscript: three answers and no middle

`a[i]=(p q)` is an array literal standing where one element's value goes,
and it is the sharpest example so far of rule 4 read the other way: the
panel does not merely disagree about a detail, it disagrees about what
the construct *is*.

Measured 2026-09-07, zsh 5.9.2, bash 5.3.15, bash 3.2.57, ksh93u+, dash:

    i=1; a=(x y); a[$i]=(p q); print -rl -- "${a[@]}"

    zsh       p q y      the element is replaced by the two words
    bash 5.3  a[$i]: cannot assign list to array member   (status 1, line abandoned)
    bash 3.2  the same sentence, the same status
    ksh93     x p        a nested value, which this interpreter cannot hold
    dash      a syntax error before any of it

There is nothing to fall back on. The two readings that can be built here
leave arrays of *different lengths*, so a core that guessed would be wrong
in a way no script could see: status 0, an array on the other side, and
plausible contents. That is what it was doing — the subscript was dropped
and the literal became the whole array — and `SubscriptedArrayLiteral` is
the axis that replaces the guess, with an unanswered dialect refused by
name.

### The rows that fix the reading

Splicing agrees with "replace the whole array" on a one-element array, and
with "insert after the element" on everything except an empty literal, so
the probe set has to include both:

    a=(x y z); a[2]=(p q)      x p q z    in the middle, not at the end
    a=(x y z); a[2]=()         x z        an empty literal *removes*
    a=(x y);   a[1]+=(p)       x p y      `+=` appends at the element
    a=(x y);   a[5]=(p q)      x y ‸ ‸ p q   padded on the way, six elements
    a=(x y z); a[-1]=(p q)     x y p q    counts back, asks nothing of the base
    a=(x y z); a[2]=([3]=p)    x ‸ ‸ p z  the literal places its own elements

(‸ is an empty element.) The last is the one that says what the *value* is:
the words are exactly the array that same literal would have built on its
own, placement and all, which is why the two spellings share one function
rather than each expanding the elements their own way.

`+=` is the same distinction the scalar element assignment already draws —
`a[0]+=Q` joins the element where `a+=(Q)` adds one after the last — so the
subscript is what says which of the two operations the operator means, on
both routes.

### The refusals belong to the axis too

The splicing shell has two complaints of its own, and they are not the
refusing shell's one sentence wearing a different coat:

    s=abc;          s[2]=(p q)   s: attempt to assign array value to non-array
    typeset -A h;   h[k]=(p q)   h: attempt to set slice of associative array

Neither names the subscript, because what is wrong is the name's kind. A
table has keys rather than positions, so there is no span for the words to
replace; a string is not an array at all. An **unset** name is neither — it
becomes an array, with empty elements in front of the subscript.

The refusing shell says its one sentence to all of them, quotes the
subscript back as written, and does not evaluate it: `a[1/0]=(p q)` is
`cannot assign list to array member` there and `division by zero` in the
shell that splices.

### And where the subscript is a range

The dialect that reads `a[1,3]` as a range (`SubscriptCommaIsARange`) reads
one on the *left* of an assignment too, and there it names a span of
elements the words replace. So the length changes by the words' count less
the span's — it grows, shrinks, or holds — where the single-subscript form
above always changes it by the literal's count less one.

Measured 2026-09-07 on zsh 5.9.2, with `a=(1 2 3)` and the fields printed
rather than counted:

    a[2,3]=(x y)      [1][x][y]        an equal count replaces, length holds
    a[2,3]=(x)        [1][x]           fewer shrinks
    a[2,3]=(x y z)    [1][x][y][z]     more grows
    a[2,3]=()         [1]              none deletes the span
    a[2,3]=x          [1][x]           a plain value is one word
    a[2,10]=(x y)     [1][x][y]        an end past the last is the last
    a[2,-1]=(x)       [1][x]           a negative end counts back from it
    a[0,2]=(x y)      [x][y][3]        a start below the first is the first
    a[3,2]=(x)        [1][2][x][3]     an empty span *inserts* and takes nothing
    a[5,6]=(x)        [1][2][3][‸][x]  a start past the last pads, then places
    a[0,0]=(x)        refused          the span is wholly below the first
    a[1,2,3]=(x y)    [x][y]           the span 1 through the arithmetic `2,3`

(‸ is an empty element.) Every one of those is the span `unset "a[lo,hi]"`
already resolves — the same clamps, the same reversed-range insertion, the
same below-the-first-element refusal — so the two constructs share one
resolution rather than each having its own. The reversed range is again the
row that proves the reading is a replacement: an empty span still puts the
words in.

Two of these rows are the panel's, not a symmetry's, and both were wrong
here until they were run.

**`+=` reads no range at all.** It takes the arithmetic-comma value of the
whole subscript, exactly as it does without a comma. Only an end out of
reach can show it, because every pair that fits inside the array gives the
same answer under both readings:

    a[2,10]+=(x)   [1][2][3][‸][‸][‸][‸][‸][‸][‸][x]   eleven, not four
    a[2,3]+=x      [1][2][3x]                          joins element 3

So the operator decides whether there is a range, and a fix that read the
comma wherever it saw one would have turned the first row into four
elements with nothing else able to see the loss.

**A table's subscript is the one place a comma means nothing.** With a
plain value the characters are part of the key — `typeset -A h; h=(k v);
h[a,b]=x` stores under the three characters `a,b` and leaves `k` alone,
which is `assoc/the-subscript-is-not-arithmetic` at a comma. With a literal
it is refused as a slice, the same sentence a table's single subscript gets
above. The attribute has to be asked about before the comma is read.

The reading also parts company with the expansion's over a *third* half.
`${a[1,2,3]}` is `bad substitution` there, while the assignment splits at
the first comma and lets the arithmetic have the rest. Measured, not
tidied — making both refuse would break the half a script can reach.

It was doing none of this. The whole subscript went through the arithmetic
comma operator, whose value is its right operand, so `a[2,3]=(x y)` ran as
`a[3]=(x y)`: four elements, the old element 2 still standing in front of
the new ones, status 0, and nothing said. A count would not have found it
either — an off-by-one at either end of a span leaves the length right and
the contents wrong, which is why every row above is compared field by
field.

A range on a plain *string* is a span of characters there (`s=hello;
s[2,3]=x` is `hxlo`), and that is not modeled: the single subscript
`s[2]=x` has no character write behind it yet either, so a range would be
the same absent construct wearing a pair.

### The letter that had to start meaning something

`typeset -a a; a[2]=(p q)` splices and `typeset a; a[2]=(p q)` is refused,
which is one question — is this name an array holding nothing, or a scalar
holding nothing — that no store with nothing in it can answer. `-a` was
accepted and recorded nowhere, because an element assignment brings an
array into being on its own and nothing had ever needed the attribute.

So the attribute *is* an empty store, exactly as `-A` already was. One
thing had to move with it: `typeset -ia b; b=(5+5 6+6)` keeps the integer
letter and folds to `10 12`, so an array with nothing in it is not a
compound value the name can be started over from — measured in bash 5.3.15,
where `typeset -ia b=(); b=(5+5 6+6)` gives the same `10 12` as the
valueless declaration.

## An axis nothing objects to is not a measurement

Every field above claims a fact about real shells: they were run, they
disagreed, and this records how. A field that can be moved to another
value with nothing anywhere failing makes that claim without it being
true of anything — and the next person to read the file cannot tell the
two apart, because a measured axis and an invented one are spelled
identically.

That is worse than an untested code path. An untested path is a risk; a
vacuous axis is *documentation of a fact nobody checked*, and it will be
cited.

Four were found vacuous in a single day, every one by accident — an agent
editing the line beside it noticed that reverting the assignment failed
nothing. `make check` was green for all four, and would be: it does not
run the conformance grader, so an axis can be set, shipped and believed
while no row anywhere can tell its answers apart.

`make axis-sweep` is the instrument. For each field it writes each of the
field's other legal values, runs the graded corpus against the shell each
dialect claims to be, and reports the fields for which nothing objected.
That list is the deliverable — the run is expected to *find* things, so it
exits nonzero when it does.

It is not in `make check` and must not be: it is thousands of shell
processes against three thousand rows. It runs on demand, and what it
produces is a backlog rather than a gate.

### The enumeration is derived, not kept

The fields come from reflection over the struct and the values from the
constants the source declares, so an axis added tomorrow is swept without
anyone remembering to add it. A field whose type the sweep cannot move is
an **error** and never a skip — a silent skip reports as "nothing
objected", which is a bug filed against an axis that was never touched.

This is the third time the same shape has cost something here. A
`reflect.Map` walk over one struct while the tables behind a slice escaped
it; a boundary guard that read only the packages holding a `Boundary`
while every dialect escaped it. A guarantee that is *checked* rather than
*enumerated* is only ever as wide as the set it walks.

### Four things an unpinned axis can be

The list the sweep produces is not a list of missing tests. It is a list
of axes whose status is *unknown*, and triage separates three cases that
need opposite fixes:

1. **Unpinned but real.** The disagreement exists and nothing discriminates
   it. Add the corpus row. This is the common case.
2. **Not reachable from the corpus.** The disagreement is real but no
   snippet can show it — a wording whose default is never read, an axis a
   dialect's grammar cannot reach. A Go test, or a note on the field
   saying why not.
3. **The disagreement is not there.** Re-measured, the shells agree. Then
   the axis is not an unpinned measurement but a **false** one, and
   pinning it with a row would carve the false fact into the golden
   record. Delete it, with the measurement.
4. **The type cannot express the panel.** The disagreement is real and the
   axis is real, but one of its legal values stands for a reading no shell
   has — an `Answer` where the panel splits three ways, and the spare
   value quietly became a fiction. Widen it to a policy with the measured
   values, as `CompoundBodyDecidesThePipelineStatusRecord` was widened.
   Never pin the fictional value with a row.

**The discriminator between 1 and 3 is to re-measure the panel, and it is
not optional.** An axis nothing exercises is precisely where a mistaken
measurement survives: nothing has ever contradicted it. Landing rows for
the whole list without re-measuring pins whichever of the two each one is,
and leaves nobody able to say which was done.

### The fourth one is invisible to the flip test

An axis whose type is too narrow looks **pinned**, not vacuous. Both of its
values reach different code, so flipping between them fails a row, the
sweep calls it healthy and moves on — while half of it is still a claim
about a shell that does not exist.

What sees it is asking the presets rather than the corpus, and that costs
no processes at all. `make axis-sweep ARGS=-presets` reports two lists in
about a second:

- **axes every dialect answers the same way** — an axis exists to record a
  disagreement, and these record none; and
- **axes with a legal value no dialect holds** — a reading nothing in the
  panel exhibits, marked when no axis of that type holds it anywhere.

Neither is a verdict, and the second in particular has a trap: a preset is
not the whole of a dialect. `FunctionLocalTraps` is answered alike by all
four presets and its other value is held by none of them, and it is
nonetheless real — zsh reaches it through the `localtraps` option at run
time. Both lists are things to re-measure, exactly like the unpinned list.

### What the triage found, and where a verdict lives

The first run of `-presets` reported 2 axes every dialect answers alike
and 25 holding a legal value no dialect holds. **Neither number is the
number to drive to zero, and that is the finding.** Both lists measure
the *shape* of the vector rather than the state of the work:

- an axis whose other answer is reached by a run-time option is
  permanently a value no preset holds, because a preset is not the whole
  of a dialect; and
- a type shared by several axes — one enumeration so that two questions
  are asked in the same words — permanently has values each single axis
  does not hold.

So counting entries counts the design. What counts the work is how many
entries **nobody has re-measured**, and the two used to be spelled
identically. They no longer are: a verdict is a line in the field's own
doc comment, `unexhibited SomeValue: who holds it, and what measured
that`, or `unanimous: why the axis records something even so`, and the
sweep reads them back and reports what is left. `-presets` exits nonzero
while anything is untriaged, on the same rule as the corpus half: the
instrument exits nonzero when it has found something.

It is **not** wired into `make check`. A brand-new axis legitimately has
no verdict yet — the measurement is the work — and a gate that failed
every commit adding one would buy tidiness with the thing the axis is
for.

Triaging all 25 — the two unanimous entries are axes that appear on
both lists — separated four kinds (2026-09-12, panel as above):

1. **Reached by a run-time mode, not a preset.** Four, including all
   three of the strong form — the values no axis of that type holds
   anywhere. `TrapsGoBackAtTheReturn` is zsh under `setopt localtraps`;
   `ForNameEndsTheScriptAsASyntaxError` and
   `FuncNameEndsTheScriptAsASyntaxError` are bash in POSIX mode, moved by
   `Runner.SetPosixMode`; and `BareSubscriptIsASubscript` answers `No`
   under zsh's `ksharrays`. All four are real and none can ever appear in
   a preset.
2. **A value the type shares with a sibling axis.** Fifteen of the 25,
   and every one of them was traced to the sibling that does hold it:
   the four `DeclarationListingForm` fields, the four
   `ListingQuotingStyle` fields, the three `NameOperands` fields, both
   `Printf…EscapePolicy` pairs, `ScalarUnderATableDeclaration` against
   its array twin, and `BareTypesetListing` against `BareLocalListing`.
   The shared vocabulary is deliberate and the unheld value is what
   makes the two fields worth separating.
3. **A reading the panel has, at an axis that is *read* rather than
   asked.** `ArithSubscriptSkippedWhenNameUnset`,
   `PrintfOutputPrecedesComplaint` and `SetArrayLetter` are consulted as
   `== Yes`, so `No` and silence reach the same code. The majority
   reading is measured and recorded on the field; writing `No` into
   those presets would add a line and no fact, and for `set -A` it would
   replace a shell's own `invalid option` with a dialect's complaint.
4. **The other side of a binary `Answer` that nothing in the panel
   holds.** `SplitCommandSubstitution`,
   `TransformLetterCheckedOnlyWhenValued` and
   `ImmovableOptionsSetAtInvocation` — the first because all six columns
   answer alike, the other two because one column has the construct at
   all and the rest never reach the question. The value stays because it
   is the other side of a question a dialect has to be able to answer:
   `No` is the null hypothesis these axes exist to make the odd shell
   argue against, and deleting it would leave the odd answer looking
   like the rule.

**Nothing was deleted.** The fiction the fourth kind was supposed to
catch — a value belonging to no shell — did not turn up once; what
looked like one was always a mode, a sibling, or the null hypothesis of
a one-sided question. That is a result about the vector and not a
formality: #2029 was real, and the same shape did not repeat.

Three things came out of re-measuring that the lists themselves did not
say:

- **bash in POSIX mode writes `export V="1"` where the default writes
  `declare -x V="1"`.** `export -p`, `readonly -p` and both bare forms
  move; `declare -p` does not. It is the mode and not the build — `set
  +o posix` puts the clustered form back on 5.3.15 and 3.2.57 alike — so
  three more axes belong in `SetPosixMode`, which moves four today and
  none of these. Filed as #2154.
- **`read ?` was a contaminated probe, and this document's field comment
  cited it.** A leading `?` argument is a *prompt* in zsh, so the value
  lands in `REPLY` and the word never stood where a name belongs. Worse,
  an unquoted `?` is a glob there and fails as `no matches found` before
  any builtin sees it — which makes every unquoted probe of the
  name-operand axes a measurement of globbing. `read 'a-b'` against
  `read '1'` is what discriminates.
- **`UnsetNameOperands` splits bash 5 from bash 3.2**: 5.3.15 takes
  `unset '?'`, `'-'`, `'1'` and `'12'` and 3.2.57 refuses all four. The
  preset holds bash 5's reading, which is right, and the older column is
  `PlainNamesOnly` — the exact trap the report's own header warns about,
  found by looking.

### A probe that measured the wrong shell's feature

The first full sweep left 111 axis/dialect pairs the corpus did not object
to, and the rule above says the first thing to do with any of them is
**re-measure**. One of them is why the rule is written that way.

`SymbolicMaskSetsWithoutAWho` recorded that zsh, alone in the panel,
refuses `umask -- =w` — and the field said so with a detail nobody would
invent: the complaint names a `/` that is not in the input. There was a
corpus row, `umask/symbolic-set-with-no-who`, and a Go test whose whole
purpose was to pin the `/`. Two instruments agreeing, and both wrong.

`=w` is not that operand in zsh. It is that shell's `=cmd` expansion, and
it reaches `umask` as `/usr/bin/w`. The `/` was in the input all along —
it arrived one expansion before the builtin. Quoted, or under `setopt
noequals`, zsh gives `umask "=w"` the same 0555 the other five give it:

    umask 022; umask -- "=w"; umask     0555 in all six columns
    umask 022; umask -- =w;   umask     zsh: bad symbolic mode operator: /

So the axis came out, the consult with it, and the corpus row now writes
the operand quoted. What the unquoted spelling was really measuring keeps
a row of its own — `expand/equals-names-a-command` — under the axis that
owns it, `EqualsExpansion`. One cell of the golden record changed, and
that cell was the whole of the evidence.

This is the second time in two days that an unquoted metacharacter in a
probe turned out to be measuring the shell's *word expansion* rather than
the builtin under test; the first was `read ?`, where a leading `?` is a
prompt in zsh and an unquoted one is a glob (#2060). **Quote the operand
whenever the probe is about what a builtin does with a word.**

### What the flip backlog is, once it is triaged

The list `make axis-sweep` produces is not one kind of thing either, and
the same reframing the preset lists needed applies here: **some of these
pairs can never be pinned, so "unpinned reaches zero" was never a state
the struct could be in.** An exit status nobody can clear is one nobody
reads, so the sweep now counts the entries with no recorded verdict, and
a verdict is a line on the axis — `unpinned zsh: why` — exactly as the
preset lists record theirs.

Four kinds, and the sweep can tell one of them apart by itself:

1. **A missing corpus row.** The common case, and the one to act on.
   Twelve rows landed with this triage — eleven new and one corrected —
   each verified to move when its axis moves before it was written down.
2. **The axis is never consulted in that dialect.** `make axis-sweep` now
   asks this directly: after the sweep it moves each unpinned axis to its
   "no answer" constant, which refuses *wherever the axis is consulted*,
   and reports `never reached` for a dialect that does not notice. Two
   flips tell three states apart, and it costs what the backlog costs.
   `TypeNamesTheKindWithDashT` in bash is the clean example — the letter
   is already in that dialect's `TypeOptions`, so the axis that predates
   the optstring is dead there (#2180).
3. **Reached, and both answers produce the same output anyway**, because
   a second axis swallows the difference. `PrintfEmptyIsNotANumber` in
   zsh: `Yes` sends an empty operand on to the bad-number complaint, and
   that shell reports no bad numbers at all.
4. **Every row that reaches it fails at baseline**, so none can pin it —
   a flip can only be caught by a row that was passing.
   `SetBTurnsOffBraceExpansion` in zsh is blocked behind the `B` letter
   being refused as unimplemented (#1856).

And a fifth, which is the one the rule at the top exists for: **the
disagreement is not there**, as `SymbolicMaskSetsWithoutAWho` was not.

This pass moved 21 of the 82 pairs: seventeen now have a row that catches
them — `AliasHasPrintOption`, `UnaliasAllRefusesOperands`,
`TypePSearchesPathPastTheShell`, `KeyedTableScalarIsTheFirstValue`,
`ListingControlEscape`, `UnsetLocaleIsUnicodeAware` and
`GetoptsClearsOptarg` in both dialects, and the three `trap` letters in
bash — and four carry a verdict saying no row ever will. **The other 61
are untriaged and the sweep says so**, which is the point of counting
them separately: what is left is a number somebody can work down rather
than a permanent property of the struct.

Three candidate rows were written, measured, and **not** landed, because
moving the axis under them changed nothing: a `type -t` probe against an
axis bash never consults, a `printf '%d' ""` probe whose complaint zsh
swallows, and a `local u` probe against an axis zsh never reaches. Each
would have gone into the corpus reading as a measurement. Checking a row
against the mutated shell before writing it down is what caught them, and
it costs two processes:

    SH_AXIS_MUTATION=Field=<value> build/axis-sh -dialect zsh -c '<probe>'

A last trap belongs with these, because the instrument reported it with
complete confidence: `-bin` given a path relative to the module root
makes every row error, which leaves nothing passing, which makes **every
axis unpinned and every one of them "never reached"**. A baseline of zero
is now a hard error naming the likely cause.

### A flip to `Unspecified` asks a different question

Moving a specified axis to `Unspecified` makes the shell refuse wherever
the axis is consulted, so almost everything objects to it. What it
measures is whether the axis is *reached*, which is a much easier bar than
whether anything can tell `Yes` from `No`. Only a flip between two answers
asks the question that found the four, so that is what the sweep counts;
the reachability flips are behind `-unspecified`.
