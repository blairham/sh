# Conditions: `[[ … ]]`

The last construct the parser was treating as an ordinary command.
`commands.md` says where it sits in the grammar and why the *parser*
rather than the lexer reinterprets `<` and `>` inside it; this says what
the operators mean.

Absent from dash, where `[[ a < b ]]` is a command named `[[` with a
redirection that opens the file `b` — see `commands.md`. Everything below
is measured on bash, ksh93 and zsh.

## It is a compound command, not a builtin

That single fact explains most of the behavior, and it is why `[[ … ]]`
exists alongside `[ … ]`. Because the shell parses it rather than passing
words to a program, **the expansions inside it are not split and not
globbed**:

| probe | result |
| --- | --- |
| `x="a b"; [[ $x == "a b" ]]` | matches — no field splitting |
| `unset u; [[ -z $u ]]` | fine — no "unary operator expected" |
| `[[ * == "*" ]]` in a directory of files | matches — no pathname expansion |

All unanimous. Every one of those is a case where the `[ … ]` builtin
needs its argument quoted and this does not, because the words never
become arguments.

### A newline inside continues the condition

Because it is parsed, a newline in the middle is not a command
terminator. It is skipped wherever the grammar is still waiting for
something, which the three shells with `[[ ]]` agree on at every
structural point — after `[[`, after `&&`, `||` and `!`, on both sides
of a group's parentheses, and before `]]`:

    [[
    -n x ]] && echo ok            →  ok, in bash 5.3, bash 3.2, ksh93, zsh
    [[ -n x &&
    -n y ]] && echo ok            →  ok
    [[ -n x
    ]] && echo ok                 →  ok

**Not after a binary operator.** `[[ 1 ==` followed by a newline is an
error in bash (`unexpected argument 'newline' to conditional binary
operator`) and in ksh93 (`` `newline' unexpected ``); only zsh takes it.
That one position is left refused, because it is what the two agree on
and accepting it would let a genuinely truncated condition through
silently.

## The right side of `==` is a pattern

    [[ abc == a*   ]]   →  matches
    [[ abc == "a*" ]]   →  no match

Unquoted, the right operand is a pattern in the language of
`patterns.md`; quoted, it is a literal. Same rule as `case`, and the same
per-span quoting deciding it.

Where the pattern arrives through a variable, the panel splits:

| probe | bash | ksh93 | zsh |
| --- | --- | --- | --- |
| `p="a*"; [[ abc == $p ]]` | matches | matches | **no match** |
| `p="a*"; [[ abc == "$p" ]]` | no match | no match | no match |

zsh does not treat the *result* of an expansion as a pattern, which is
the `GlobExpansionResults` axis from `semantics.md` reaching into
conditions. It is the same rule that stops `x="et*"; echo $x` globbing
there, applied in a second place — so the axis is one behavior, not two.

### A group may start the pattern, in the dialect that has bare groups

    [[ $k == (a|b) ]]

One shell reads it and the other four call the `(` a syntax error.
Measured 2026-09-05 (panel and machine as `../oracle.md`), on `-c`:

| probe | dash | bash 3.2 | bash 5 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `[[ $k == (a\|b) ]]` | error | error | error | error | **matches** |
| `[[ $k = (a\|b) ]]`, `!=` | error | error | error | error | **matches** |
| `[[ $k == (a\|b)* ]]` | error | error | error | error | **matches** |
| `[[ $k == ((a\|b)\|x)(b\|c) ]]` | error | error | error | error | **matches** |
| `[[ $k == "(a\|b)" ]]` | — | literal | literal | literal | literal |
| `[[ -n (a\|b) ]]` | error | error | error | error | **error** |
| `[[ (a\|b) == $k ]]` | error | error | error | error | **error** |
| `[[ $k == () ]]` | error | error | error | error | **error** |
| `[[ $k ==(a\|b) ]]` | error | error | error | error | **error** |

**It is the pattern operand and not the condition.** The last four rows
are the boundary and they are all refused in the same shell that takes
the first four: a group is read where a *pattern* is read — after `==`,
`=` and `!=` — and nowhere else in the construct. `-n`'s operand is not a
pattern, the left operand is not a pattern, an empty group is not one,
and a `(` welded to the operator is not the operand at all. `-eq`'s
operand takes a `(` for a different reason and with a different meaning:
it is arithmetic there, so `[[ 1 -eq (1|2) ]]` is `1 -eq 3`.

Nothing here is a new grammar flag. `PatternAlternation` already says the
dialect has bare groups, and a group *mid-word* already worked:
`[[ $k == a(b|c) ]]` matched while `[[ $k == (a|b) ]]` did not. The
difference is entirely about the first character of the operand — `(` is
in the operator table, so a token that begins with one is taken as an
operator and the word scanner is never entered. The lexer is told it is
reading a pattern operand, exactly as it is already told it is reading a
regular expression for `=~` and for the same reason.

**The group is one word, spaces and all.** `[[ "a b" == (a b) ]]`
matches, so the scanner takes everything to the matching `)` rather than
stopping at whitespace — which is the rule a group already had, reached
from a new place. And a nested group starts `((`, which is the one place
in the grammar where two parentheses are otherwise a single token: the
pattern operand is read before that, or `[[ $k == ((a|b)|x) ]]` would be
an arithmetic command.

**And the reading stops at the operand.** Whatever the lexer is told about
a pattern operand has to stop being true when the operand ends, or every
`(` after a condition is scanned as part of a word — so
`[[ 1 == 1 ]] && (echo x)` becomes a command *named* `(echo x)`, which
still parses and so shows up as "command not found" rather than as a
syntax error. Found by mutation rather than by a shell.

**What this rule does not cover.** The shell with bare groups also takes
a `(` after operators whose operand is not a pattern, and means something
different by it each time: `[[ 3 -eq (1|2) ]]` is *true* there, so the
group is arithmetic — `1|2` is 3 — and `[[ x -nt (a|b) ]]` parses with
the group as a filename. Neither is implemented, and neither is this
rule: reading them as patterns would make `[[ 1 -eq (1|2) ]]` true.

Corpus: `cond/a-pattern-operand-may-start-with-a-group`,
`cond/a-group-may-be-followed-by-more-pattern`,
`cond/a-quoted-group-is-a-literal`,
`cond/a-group-is-one-word-whitespace-and-all`,
`cond/a-group-is-the-patterns-and-not-the-conditions`,
`cond/a-group-may-not-start-the-left-operand`,
`cond/an-empty-group-is-not-a-pattern`,
`cond/a-paren-after-a-condition-is-still-a-subshell`.

**The refusal is worded per dialect now.** A token standing where a
conditional operator wanted a word is `ErrCondOperand`, which bash alone
words as a statement about the operator — "unexpected argument `(' to
conditional binary operator", with `unary` for the one-operand family —
where ksh93 and zsh say what they say about any token the grammar did not
want. It replaced one message of our own that matched nobody. bash
follows its line with a `syntax error near` line and an echo of the
source; only the first of the three is written here.

### The completion conditions are a different question

zsh's `[[ -prefix - ]]` and `[[ -after x ]]` come from the same sweep and
are **not** this code path: they are unary operator *names*, read from
the operator table, where this is about how a pattern operand is lexed.
They are recorded in "What this does not cover" below rather than folded
in here.

## `-gt` is numeric and `>` is a string comparison

    [[ 10 -gt 9 ]]  →  true
    [[ 10 >   9 ]]  →  **false**

Unanimous, and the sharpest trap in the construct: `"10"` sorts before
`"9"`. The two families are not interchangeable and the numeric one is
spelled with words:

| numeric | string |
| --- | --- |
| `-eq -ne -lt -le -gt -ge` | `= == != < >` |

`=` and `==` are both accepted and mean the same thing.

## `=~` matches a regular expression

    [[ abc =~ ^a.c$ ]]  →  matches

This is the one place in the shell where the pattern language is **not**
the one in `patterns.md`: `.` and `^` and `$` have their regular
expression meanings. Quoting it diverges:

| probe | bash | ksh93 | zsh |
| --- | --- | --- | --- |
| `[[ abc =~ "^a.c$" ]]` | **no match** | matches | matches |

bash treats a quoted right operand as a literal string, so the regex
metacharacters stop working; ksh93 and zsh still read it as a regex.
Quoting a regex is therefore not portable in either direction, and the
portable spelling is to keep it unquoted or to put it in a variable and
use that unquoted.

Semantics axis: `RegexQuotingMakesLiteral` — bash yes, ksh93 and zsh no,
dash not applicable because it has no `[[ ]]`. Unanswered in the core,
and POSIX has no answer either, since `=~` is not in the standard: bash
is the outlier here rather than the rule, so an entry that called its
behavior the default would have named the one shell that disagrees with
the other two.

## Unary and logical operators

    -n s   -z s                     non-empty, empty
    -e f   -f f   -d f   -r f
    -w f   -x f   -s f              file tests
    -L f   -h f                     symbolic link, both spellings
    -b f   -c f   -p f   -S f       block, character, fifo, socket
    -g f   -u f   -k f              setgid, setuid, sticky
    -t fd                           the descriptor is a terminal
    -o name                         the shell option is set
    !                               negation
    &&     ||                       conjunction, disjunction
    ( … )                           grouping

The file-kind and permission-bit tests are unanimous across the three
shells that have the construct, measured against a fifo, `/dev/null`,
a block device, and files with each bit set — and they are the same
questions `test` asks, so the two constructs share the code that asks
them.

`-t` is unanimous on every descriptor the harness can offer: stdin on
`/dev/null`, stdout into a pipe, and a descriptor that was never open
are all a quiet false. A runner whose streams are io.Writers gives that
answer always — the honest one for a library, and the same one the
panel gives a shell whose streams are pipes. The operand diverges when
it is not a number at all:

| probe | bash | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `[[ -t x ]]` | **`integer expected`, status 2** | 1 | 1 | 1 |
| `test -t x` | **status 2** | 1 | 1 | 1 (dash: **status 2**) |

bash refuses loudly — and bash 3.2 does not, so the complaint is
younger than the operator. dash, which reaches the question only
through `test`, refuses too with its `Illegal number` wording.

Semantics axis: `TerminalTestRequiresANumber` (bash and dash yes, ksh93
and zsh no; unanswered in the core), asked only for such an operand.

## `-o`: the option test

`[[ -o name ]]` is true when the named shell option is set. It is
**core**, on the same head count that put `[[ ]]` itself there: bash
5.3, bash 3.2, bash-as-`sh`, ksh93 and zsh all have it, and dash is
absent from the row only because it has no `[[ ]]` to put it in. There
is no grammar for a dialect to switch here, because what the shells
disagree about is which *names* exist — a question the parser never
asks.

The operand is an **ordinary word**, unanimously: it is expanded, its
quotes come off, and it is not globbed.

    v=errexit; [[ -o $v ]]          reads the variable
    [[ -o 'aliases' ]]              the same name as the bare spelling
    [[ -o err* ]]                   a name literally spelled `err*`; false

Missing, it is refused everywhere, in three different ways — bash and
ksh93 make `[[ -o ]]` a *parse* error, and zsh takes the `-o` for a
condition name it does not know and answers 2 at run time. No shell
lets it through, so this parser refuses it too.

### Which names, and what an unknown one does

Three separable questions, and only the last is an axis.

**Does the shell have the name?** The option vector's, already:
`interp` holds the names every shell has and each dialect declares its
own extras. Names that read the same state in the whole panel include
`errexit`, `nounset`, `xtrace`, `noexec`, `verbose` and `monitor`.
Names that belong to some and not others do not: `braceexpand`,
`posix` and `privileged` are bash's, `aliases` and `functionargzero`
are zsh's, and `interactive` is ksh93's and zsh's and not bash's.

**How is a spelling folded onto a name?** Measured, and it is not
uniform: bash matches exactly, ksh93 also ignores underscores, and zsh
folds case, ignores underscores, and reads a single leading `no` as a
negation of what follows.

| probe | bash | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `[[ -o errexit ]]` under `set -e` | true | true | true | true |
| `[[ -o Err_Exit ]]` under `set -e` | false | false | true (`err_exit`) | true |
| `[[ -o noerrexit ]]` under `set +e` | false | false | true | true |

So zsh's option namespace is not its `set -o` names — about a hundred
and eighty against a couple of dozen — which is why the substrate takes
the lookup as a function a dialect supplies
(`Runner.SetOptionNamespace`) rather than a longer list of its own. A
shell that installs none reads its `set -o` names, which is bash's
shape and ksh93's.

**What happens to a name the shell does not have?** This is the axis.
bash and ksh93 answer a quiet false at 1 and say nothing. zsh
complains and answers **3** — a status that is neither of the two a
condition otherwise gives. Both carry on to the next command, so this
is *not* the fatality the same shell gives `set -o nosuchoption`
(`BadSetOptionNameFatal`); one construct's refusal is not the other's.

Semantics axis: `UnknownConditionOptionIsAStatus` (zsh yes; bash, ksh93
and dash no; unanswered in the core), asked only where the shells
disagree — at a name none of them would recognize. Wording and status
are `Diagnostics.UnknownConditionOption` and
`UnknownConditionOptionStatus`.

The 3 is a **third value and not a false**, which is only visible
through the operators that combine conditions. Measured across the
whole truth table on zsh 5.9.2, with `zzz` a name it does not have:

| probe | bash / ksh93 | zsh |
| --- | --- | --- |
| `[[ -o zzz ]]` | 1 | **3** |
| `[[ ! -o zzz ]]` | 0 | **3** |
| `[[ ! ! -o zzz ]]` | 1 | **3** |
| `[[ -o zzz \|\| 1 == 1 ]]` | 0 | 0 |
| `[[ -o zzz \|\| 1 == 2 ]]` | 1 | 1 |
| `[[ -o zzz && 1 == 1 ]]` | 1 | **3** |
| `[[ 1 == 1 && -o zzz ]]` | 1 | **3** |
| `[[ 1 == 2 && -o zzz ]]` | 1 | 1 |
| `[[ 1 == 1 \|\| -o zzz ]]` | 0 | 0 |

The rule every row fits: `[[ ]]` combines *statuses*. `||` stops on a
zero and otherwise takes the right-hand answer, `&&` stops on a
non-zero and otherwise takes the right-hand answer, and `!` maps 0 to 1
and 1 to 0 while leaving anything else alone. A false and a 3 differ
only under `!` and under `||`, which is exactly where an
implementation that returned false with a status painted on would give
the wrong answer.

A condition that failed for some *other* reason is a plain false, not a
third value: `[[ x =~ "[" ]]` is 1 in all three, `[[ ! x =~ "[" ]]` is
0 in all three, and zsh writes `failed to compile regex` beside its 1.
So the third value belongs to this operator and not to `[[ ]]` errors
in general.

### The single-bracket `-o` is a different operator

POSIX gives `[` a binary `-o` meaning *or*, and conflating the two is
the mistake worth naming:

| probe | bash | bash 3.2 | dash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `[ -o errexit ]` under `set -e` | true | true | `unexpected operator`, 2 | true | `too many arguments`, 2 |
| `[ -o nosuchoption ]` | 1 | 1 | `unexpected operator`, 2 | 1 | `too many arguments`, 2 |
| `[ -o ]` | 0 | 0 | 0 | 0 | 0 |
| `[ '' -o x ]` | 0 | 0 | 0 | 0 | 0 |

bash and ksh93 do carry the option test into the builtin; dash and zsh
do not. So the single-bracket spelling is **not** core the way the
double-bracket one is, and this shell does not implement it — `[ -o x ]`
is `unary operator expected` here. One argument (`[ -o ]`) is a
non-empty string test and true everywhere, and three arguments are the
standard's *or* everywhere.

## The file comparisons: `-nt`, `-ot`, `-ef`

    [[ new -nt old ]]   →  true when new's mtime is later
    [[ old -ot new ]]   →  the mirror
    [[ a   -ef b   ]]   →  true when the two names are one file

With both files present, all of it is unanimous — including that equal
times are neither newer nor older, and that `-ef` is identity rather
than equality: a hard link, or a symlink followed to the file, compares
equal, and a second file with identical content does not.

All three exist in `test` as well, in every shell in the panel — dash
included, whose lack of `[[ ]]` does not extend to the builtin.

A file that does not exist splits the panel, in both constructs the
same way per shell:

| probe (f exists) | bash | ksh93 | dash (`test`) | zsh |
| --- | --- | --- | --- | --- |
| `f -nt missing` | true | true | **false** | **false** |
| `missing -ot f` | true | true | **false** | **false** |

bash and ksh93 count a missing file as older than any file that does
exist; dash and zsh want both files present. The mirrored cases ask
nothing — `missing -nt f` and `f -ot missing` are false everywhere, a
missing file never being *newer* — and so are the both-missing cases.

Semantics axis: `MissingFileIsOlder` (bash and ksh93 yes, dash and zsh
no; unanswered in the core). One axis for `test`, `[` and `[[ ]]` alike,
because every shell answers its two constructs the same way.

`&&` and `||` inside `[[ ]]` join *conditions*, not commands, and `( )`
groups conditions rather than starting a subshell.

**And they do not have the precedence they have outside it.** Measured:

    [[ -n a || -n b && -z x ]]   →  true

`-z x` is false, so a true result requires `-n a || (-n b && -z x)` —
`&&` binding tighter, as in C. Equal precedence with left association
would give `((-n a || -n b) && -z x)`, which is false.

The command language is the other way, and `commands.md` measures it
there: `true || true && false` exits 1, which needs
`(true || true) && false`.

So the same two operators have different precedence on either side of a
`[[`. That is the sharpest reason the parser needs its own production for
the inside rather than reusing the one for and-or lists — reuse would be
correct-looking and wrong. That is another
consequence of it being parsed rather than executed, and it is why the
parser needs its own production for the inside rather than reusing the
one for lists.

## What this does not cover

`-v` and `-o`, which test a variable and a shell option rather than a
file and exist in some of the panel only.

`-O`, `-G` and `-N`, which are file tests the listed set does not carry:
the file is owned by the effective uid, the file is owned by the
effective gid, and the file has been modified since it was last read.
The first two are unanimous and `-N` is bash, ksh93 and zsh — dash
answers `test: -N: unexpected operator` with status 2 — so this is a gap
in what is implemented rather than a divergence to model.

The parser refuses all five along with everything else it does not list,
which is the rule: an operator is either implemented or refused at parse,
never parsed and then refused at run time. Recorded so their absence is a
decision.

zsh's completion conditions, which are the same gap reached from the
other end. Measured 2026-09-05 on zsh 5.9.2, with `-n` for the parse and
a run for the rest:

| probe | parses | run |
| --- | --- | --- |
| `[[ -prefix x ]]` | yes | `condition can only be used in completion function`, 1 |
| `[[ -suffix x ]]` | yes | the same, 1 |
| `[[ -after x ]]` | yes | **SIGSEGV** |
| `[[ -before x ]]`, `-equal`, `-between` | yes | `unknown condition: -X`, 2 |
| `[[ -nosuch x ]]` | yes | `unknown condition: -nosuch`, 2 |

So the general rule in that shell is that **any** `-word` followed by an
operand parses as a unary condition and an unknown one is refused when it
runs — which is precisely the shape the rule above forbids, and adopting
it would be a decision to change the rule rather than a gap to fill. The
other four disagree with it and with each other: bash 5 and ksh93 make
`[[ -nosuch x ]]` a syntax error, and bash 3.2 accepts it.

`-after` crashing rather than refusing is a bug in that build, not a
behavior to model; it also means the row cannot be graded against a run.
