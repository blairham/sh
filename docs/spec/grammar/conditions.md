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
from a new place. And a nested group starts `((`, which the arithmetic
command would otherwise claim as a single token: the pattern operand is
read before that, or `[[ $k == ((a|b)|x) ]]` would be an arithmetic
command. (Inside a condition nothing claims it any more — see *Two
parentheses that touch* below — but the pattern operand still has to be
read first, because it wants the whole group as one **word** and not two
tokens.)

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

### Two parentheses that touch are two groups

Everywhere a command may begin, `((` is the arithmetic command and `( (`
is a subshell containing one. The distinction is textual, and no shell
needs to know *which* command position it is at: `((echo hi))` is an
arithmetic error in bash, ksh93 and zsh with no space, and two nested
subshells that print `hi` in dash.

Inside `[[ ]]` no command may begin at all, so there is no arithmetic
command to be had and `((` is simply two grouping parentheses:

    [[ ((1 -eq 1)) ]]        →  status 0
    [[ ( (1 -eq 1) ) ]]      →  status 0

Unanimous across every panel member that has the construct — bash 3.2.57,
bash 5.3.15, bash-as-`sh`, ksh93 and zsh 5.9.2 — at every position where
a condition may begin: after `[[`, after `&&`, after `||`, after `!`,
after another `(`, and before a unary operator, where `(( -z ""` is the
shape that most looks like an arithmetic command and least is one.
dash has no `[[ ]]` and abstains, so the intersection is unanimous and
this is core rather than a dialect flag.

The disambiguator is context, and it is the *lexer's* to hold: `(` is an
operator, so a token beginning with one never reaches the word scanner
and the parser never gets to decide. The condition already tells the
lexer it is inside one — the same flag the bare-pattern-group dialect
reads — and that flag now also suspends the arithmetic command. It is
cleared when the condition ends, so one character past `]]` the
arithmetic command is back:

    [[ 1 -eq 1 ]] && (( x++ ))

The other direction is unchanged. An arithmetic command whose expression
opens with a parenthesis — `(( (1+2)*3 ))` — was never in doubt, because
the scan for the closing `))` tracks nesting rather than counting.

Corpus: `cond/touching-grouping-parens`,
`cond/spacing-the-grouping-parens-changes-nothing`,
`cond/touching-grouping-parens-after-oror`,
`cond/touching-grouping-parens-after-andand`,
`cond/touching-grouping-parens-after-not`,
`cond/touching-grouping-parens-before-a-unary`,
`cond/three-touching-grouping-parens`,
`cond/a-regex-group-that-opens-with-a-group`,
`cond/the-arithmetic-command-survives-the-condition`,
`arith/a-command-expression-may-open-with-a-paren`,
`arith/two-parens-at-command-position-are-not-a-subshell`,
`cmd/a-subshell-that-opens-with-a-subshell`.

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
