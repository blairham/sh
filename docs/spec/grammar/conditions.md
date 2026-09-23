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

## A `]]` where a condition would begin

The closer is a closer where the condition has something to close over, and
the panel splits three ways about what it is where the condition has nothing.
Measured 2026-09-18, `env -i PATH=/usr/bin:/bin LC_ALL=C`:

| written | bash 5.3.20 | ksh93u+ | zsh 5.9.2 |
| --- | --- | --- | --- |
| `[[ ]] ]]` | refuses | **0** | refuses |
| `[[ ]] == x ]]` | refuses | **1** | refuses |
| `[[ ]] && x ]]` | refuses | **0** | refuses |
| `[[ -n ]]` | refuses | refuses | refuses |
| `[[ x == ]]` | refuses | refuses | refuses |

**ksh93 reads it as an ordinary word** where a condition *term* belongs — the
two characters are a word, true for being non-empty, comparable as a string,
joinable by a connective — and takes it as the closer everywhere else.
`ConditionCloserIsAWordWhereATermBegins` is the flag, and the two rows at the
bottom are its scope: an **operand**'s position is a separate question and
every column answers it alike.

The five places a term may begin are the `[[`, a `!`, a `&&`, a `||` and a
group's `(`.

### zsh refuses it too, and names something else

It looks like the same reading from a `-c` probe and is not. zsh refuses the
token; what it does differently is report the refusal at the token **behind**
the closer, at that token's own line. The routes are what say so, and this is
why the row could not be settled from `-c` alone:

| route | bash 5.3.20 | zsh 5.9.2 |
| --- | --- | --- |
| `-c '[[ ]]'` | ``near `]]'``, line 1 | ``near `]]'``, line 1 |
| a file with a final newline | ``near `]]'``, line 1 | a newline, line 2 |
| a file without one | ``near `]]'``, line 1 | ``near `]]'``, line 1 |
| standard input | ``near `]]'``, line 1 | a newline |
| `[[ ]]` then `echo after` | ``near `]]'``, line 1 | ``near `echo'``, line 2 |
| `[[ ]]; echo after` | ``near `]]'``, line 1 | ``near `;'``, line 1 |
| `[[ ]] echo after` | ``near `]]'``, line 1 | ``near `echo'``, line 1 |
| `[[ ]] ]]` | ``near `]]'``, line 1 | ``near `]]'``, line 1 |
| `[[ ]] == x ]]` | ``near `]]'``, line 1 | ``near `=='``, line 1 |

`ConditionTermMissingBlamesTheTokenAfterTheCloser` carries it. Blank lines
between are skipped, so three of them before an `echo` put the complaint on
the `echo`'s line; where nothing follows at all the `]]` is still the last
token read and is what gets named, which is why the first, third and eighth
rows agree in both columns.

**One shape is measured and not modeled.** `[[ ]] && x ]]` is
`condition expected: x` in zsh — a run-time complaint about a word, the `&&`
there being read as the list operator it also is — where this reading names
the `&&` (#2964).

Where a newline follows the word ksh93 names the **newline** rather than what
stands behind it, and that is not this reading either: it is the section
below, reached here because the closer *is* the word (#3627).

## A term's first word at the end of its line

A newline after a term whose first word has been read and whose shape is not
yet settled — a binary operator may still follow it — is a **refusal** in two
of the three columns, and the third takes it. Measured 2026-09-18, script
files under `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin `/dev/null`:

| written | zsh 5.9.2 | bash 5.3.20 and 3.2 | ksh93u+ |
| --- | --- | --- | --- |
| `[[ y` ⏎ `]]` | runs | ``token `newline', conditional binary operator expected`` | ``` `newline' unexpected ``` |
| `[[ y` ⏎ `&& -n z ]]` | runs | the same | the same |
| `[[ y` ⏎ `\|\| -n z ]]` | runs | the same | the same |
| `[[ ( y` ⏎ `) ]]` | runs | the same | the same |
| `[[ -n x && y` ⏎ `]]` | runs | the same | the same |
| `[[ y` ⏎ `== z ]]` | refuses | the same | the same |
| `[[ -n x` ⏎ `]]` | runs | runs | runs |
| `[[ y == z` ⏎ `]]` | runs | runs | runs |
| `[[ ( -n x )` ⏎ `]]` | runs | runs | runs |

The last three rows are the control: a term that is **finished** takes a
newline everywhere, which is what says this is a rule about one position and
not about newlines inside a condition. The sixth is the other side of it —
a binary operator with no operand is refused in every column, including the
one that takes the rest, so the flag reaches a term with no verdict and not
a newline anywhere in the construct.

`ConditionNewlineMayFollowATermsFirstWord` is the flag, **off** in the core:
two of the three refuse, and what moves is what the parser accepts rather
than how a refusal is worded. Reading it as core-accepts is what made this
shell run `[[ y` ⏎ `]]`, a line bash and ksh93 both reject.

The refusal names the newline, located at the **last** of the blank lines
rather than the first: three of them between `[[ y` and an `echo` put ksh93's
complaint on the `echo`'s line with `newline` still the word quoted — the
same skip `ConditionTermMissingBlamesTheTokenAfterTheCloser` takes one
construct over. bash writes a sentence of its own about the operator it was
waiting for, `Diagnostics.CondTermUndecidedPreamble`; the two lines it prints
after that one still name the newline where bash names the word in front of
it, which is the generic echo and is left as it stands.

**It is the position and not the newline.** bash writes the same sentence for
*any* token standing behind a term of one bare word, which is what makes this
a rule about where the reading stood rather than about what a newline means.
Measured 2026-09-22 on bash 5.3.20 under `-c`:

| written | bash 5.3.20 |
| --- | --- |
| `[[ 4 & ]]` | ``unexpected token `&', conditional binary operator expected`` |
| `[[ x ; ]]` | the same, naming the `;` |
| `[[ x \| ]]` | the same, naming the `\|` |
| `[[ x 7 ]]` | the same, naming the `7` |
| `[[ -Q 7 ]]` | the same — `-Q` is no operator, so `-Q` is the bare word |
| `[[ -n x && y z ]]` | the same, naming the `z` behind `y` |
| `[[ -n x y ]]` | ``syntax error in conditional expression: unexpected token `y'`` |
| `[[ a == b == c ]]` | the same, naming the second `==` |

The last two rows are the control: a term the operator has already settled
takes the ordinary conditional wording, so the two sentences are chosen by the
shape of the term and not by the token.

## A condition group that never closed

A refusal falling where a group's `)` was wanted names **that closer** in the
same sentence as the token, and the group is then not also counted among the
ones still open. Measured 2026-09-22 on bash 5.3.20 under `-c`:

| written | bash 5.3.20 |
| --- | --- |
| `[[ ((1 -eq 1) ]]` | ``unexpected token `]]', expected `)'``, then the ordinary `near` and echo |
| `[[ ( -n x ; ]]` | the same, naming the `;` |
| `[[ ( -t X` | ``unexpected token `EOF', expected `)'``, then ``unexpected end of file from `[[' command on line 1`` |
| `[[ ( ( -t X` | the same, with one ``expected `)'`` between them for the outer group |
| `[[ ( x & ) ]]` | the *term's* sentence instead — the `&` stood behind a one-word term — with ``expected `)'`` under it |
| `[[ -t X` | ``unexpected EOF while looking for `]]'`` — the control, with no group around it |

The last row is what says this is a sentence of its own rather than the
unterminated-condition one reworded. This shell answered every one of these
with `expected ) in a condition`, a sentence no shell in the panel writes
(#4173).

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
want. It replaced one message of our own that matched nobody.

bash's sentence is a **preamble**: it is followed by the ordinary
`syntax error near` line and the echoed source, the same three-line shape
every other token refused inside a condition gets. Measured 2026-09-22 on
bash 5.3.20 under `-c`:

| written | bash 5.3.20 |
| --- | --- |
| `[[ -n & ]]` | ``unexpected argument `&' to conditional unary operator``, ``syntax error near `&'``, ``` `[[ -n & ]]' ``` |
| `[[ 4 > & ]]` | the binary spelling of the same three |
| `[[ -n ]]` | the `]]` named as the surplus argument, then the same two |

This document used to say only the first line was written, and this shell
wrote only the first line; that cost `cond.tests` six lines (#4173).

### The completion conditions are a different question

zsh's `[[ -prefix - ]]` and `[[ -after x ]]` come from the same sweep and
are **not** this code path: they are unary operator *names*, read from
the operator table, where this is about how a pattern operand is lexed.
Two of them are now in that table — see the next section — and the rest
are recorded in "What this does not cover" below.

## The four completion conditions are conditions in one dialect

    [[ -prefix : ]]                  parses in zsh, and in nothing else
    [[ -prefix //(a|b)/ ]]           the same
    [[ -suffix : ]]                  the same

They are in that shell's condition **grammar** unconditionally, and the
restriction is on where they may *run*. Measured 2026-09-12 on zsh 5.9.2,
`env -i PATH=/usr/bin:/bin` with a scratch `HOME`:

    $ zsh -n s.zsh          # [[ -prefix : ]]
    (nothing)
    $ zsh s.zsh
    s.zsh:1: condition can only be used in completion function
    $ echo $?
    1

The refusal is **fatal** — a line after it does not run — and the
sentence names neither the operator nor the operand, so all three operand
shapes get the same one. Grammar flag: `CompletionConditions` — core off,
`zsh` on. Diagnostic:
`Diagnostics.CompletionConditionOutsideCompletion`.

That is the split this substrate draws everywhere else, and it is what
makes the gap a *parser* one: a completion function is a file, and a file
that will not parse never gets as far as the restriction. Two files in an
ordinary `~/.zi` tree reach it, both completions shipped by
`zsh-users/zsh-completions` (#1879).

**The operand is a pattern**, read the way `==`'s right-hand side is
rather than the way `-o`'s option name is. `//(127.0.0.1|localhost)/` is
one of the two real occurrences.

**With no operand the word is ordinary**, and that row is the one that
had to be measured before the flag could be added at all:

| probe | dash | bash 3.2 | bash 5 | bash as sh | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `[[ -prefix ]]` | `[[: not found`, 127 | error | 0 | 0 | 0 | **0** |
| `[[ -prefix && -n x ]]` | the same | error | 0 | 0 | 0 | **0** |
| `[[ ( -prefix ) ]]` | the same | error | 0 | 0 | 0 | **0** |
| `[[ -n ]]` | the same | error | error | error | error | `unknown condition: -n` |

`-after` and `-between` answer 0 there too, measured 2026-09-19:
`[[ -after ]]`, `[[ -between ]]`, `[[ -after && -n x ]]`,
`[[ -between && -n x ]]` and `[[ ( -between ) ]]` are all 0 with nothing
said.

### The arities, and what a count outside them does

Measured 2026-09-19 on zsh 5.9.2, `-n` for the parse and a run for the
verdict. Every row **parses**; the difference is what happens next:

| probe | run |
| --- | --- |
| `[[ -prefix a ]]` | `condition can only be used in completion function`, 1 |
| `[[ -prefix 1 '*=' ]]` | the same — the optional count |
| `[[ -prefix a b c ]]` | `unknown condition: -prefix`, 2 |
| `[[ -after a ]]` | **SIGSEGV** |
| `[[ -after a b ]]` | `unknown condition: -after`, 2 |
| `[[ -between a b ]]` | **SIGSEGV** |
| `[[ -between a ]]`, `[[ -between a b c ]]` | `unknown condition: -between`, 2 |

So `-prefix` and `-suffix` take one or two operands, `-after` exactly one
and `-between` exactly two, and a count outside the range is the refusal
`ConditionArityIsCheckedWhenItRuns` already carries. An **operand that is
itself an operator ends the reading**: `[[ -prefix -n x ]]` is
``parse error near `x'`` there, the `-n` being the whole of `-prefix`'s
operands — the same rule `[[ -n -z x ]]` follows above.

The two SIGSEGV rows are a crash in zsh 5.9.2 and not a behavior to
reproduce. Here they get the sentence their two neighbors get.

### Loading `zsh/complete` is not what makes them exist

Measured 2026-09-19 on a fresh `zsh -f`, whose `zmodload` listing is
`zsh/main` alone:

    $ zsh -f -c '[[ -prefix foo ]]'
    zsh:1: condition can only be used in completion function          # 1
    $ zsh -f -c 'zmodload zsh/complete; [[ -prefix foo ]]'
    zsh:1: condition can only be used in completion function          # 1

Identical before and after, so the module is a no-op for every one of
them and the grammar carries them at all times. What the module's load
status *does* decide is `zmodload zsh/complete`'s own exit status, which
is why the conditions had to be implemented for it to succeed (#3042).

### What each one is true of

From `zshcompwid(1)`: each is true where the corresponding `compset`
option's test would succeed, and the special parameters are **not**
modified. `-prefix` is `compset -P`, `-suffix` is `compset -S`, `-after`
is `compset -N` with only the start pattern, and `-between` is
`compset -N` with both.

Measured 2026-09-19 through a pseudo-terminal, inside a `zle -C` widget,
with `git sub a=b=c tail` typed and the cursor at the end of the third
word — `PREFIX=a=b=c`, `SUFFIX=` empty, `words=(git sub a=b=c tail)`,
`CURRENT=3`:

| probe | status |
| --- | --- |
| `[[ -prefix a ]]`, `[[ -prefix *= ]]`, `[[ -prefix 1 *= ]]` | 0 |
| `[[ -prefix '*=' ]]` | 1 — quoted, so a literal |
| `[[ -suffix c ]]` | 1 — `SUFFIX` is empty |
| `[[ -suffix '' ]]` | 0 |
| `[[ -after git ]]`, `[[ -after sub ]]`, `[[ -after *u* ]]` | 0 |
| `[[ -after tail ]]` | 1 — behind the cursor |
| `[[ -between git tail ]]` | 0 — the end pattern is after the cursor |
| `[[ -between git sub ]]` | 1 — it is before it |
| `[[ -between git a=b=c ]]` | 1 — it is *at* the cursor |
| `[[ -between git zz ]]` | 0 — it matches no word, so it is as if absent |

Two of those rows carry the whole design. **The operand is a pattern and
quoting decides it**, exactly as for `==`'s right-hand side. And
**`-between` is not `-after` with a second word nobody reads**: the four
`-between git …` rows differ only in the end pattern and answer 0, 1, 1
and 0.

Every other one-operand test in the table demands its operand and says so
when it is missing. These two do not: with nothing after them the
condition is the bare-word test for non-emptiness, and `-prefix` is not
empty. bash 5.3, that binary as `sh` and ksh93 answer 0 because they have
no such operator to demand anything; zsh answers 0 with the operator, and
that is the row this carve-out is for — **without it, adding the pair
would have made this dialect refuse a line three other columns run.**

The two columns that do not answer 0 are not counter-examples. dash has
no `[[ ]]` at all, so `[[` is a command it cannot find. And bash 3.2 is
the odd one out even against bash 5.3: it reads `-prefix` as *a*
conditional unary operator and complains that `]]` is an unexpected
argument to it, where 5.3 reads the same word as an ordinary one.

The look for an operand is at the source rather than at a token, because
reading the token would consume it.

**Three or more words is a different question and is not this flag.**
`[[ -prefix -foo : ]]` parses in zsh and is refused here, and so is
`[[ -nosuch x ]]`; the second is still the unknown-operator rule in "What
this does not cover" below. The arity of a *known* operator is the
section that follows.

## A known operator's arity is checked when the condition runs — zsh only

    echo pre; [[ -n x y ]]; echo post

| shell | `pre` | what it says | status |
| --- | --- | --- | --- |
| bash 5.3, and as `sh` | no | ``syntax error in conditional expression: unexpected token `y'`` | 2 |
| bash 3.2 | no | `syntax error in conditional expression` | 2 |
| ksh93 | no | ``syntax error at line 1: `y' unexpected`` | 3 |
| zsh 5.9.2 | **yes** | `unknown condition: -n` | 2 |
| dash | yes, and `post` | `[[: not found` | 0 |

The `echo pre` is the whole point of the row: it is the difference
between a refusal made while *reading* and one made while *running*, and
no wording can show it. Three of the columns never run the `echo`; zsh
runs it, then refuses, then ends the shell. dash is a third shape rather
than agreement — it has no `[[ ]]` and runs both echoes.

**The operator is what is named**, not the surplus word, which the other
end of the arity says without ambiguity: `[[ -n ]]` is `unknown
condition: -n` there too, with no surplus word to name. Same status, same
sentence, and the refusal reaches out of a negation, out of a group and
out of the right-hand side of a `&&`.

That is `syntax.Dialect.ConditionArityIsCheckedWhenItRuns`, and
`syntax.CondArity` is the node it produces — an operator and every word
that stood with it, so a formatter writes the line back as it was and the
interpreter names the operator. The sentence is
`interp.Diagnostics.UnknownCondition` and the status
`UnknownConditionStatus`, which is 2 and is *not* that shell's generic
fatal status of 1.

**Two boundaries, each measured**, and each is a row a simpler rule gets
wrong:

| written | zsh 5.9.2 | why it is not the rule above |
| --- | --- | --- |
| `[[ -bogus ]]` | 0 | a word that is no operator is a bare-word test, and `-bogus` is not empty |
| `[[ -n -n ]]` | 0 | an operator-shaped *operand* is an ordinary word |
| `[[ -n -z x ]]` | ``parse error near `x'`` | so that operand starts a reading of its own, and the word after it is unexpected rather than surplus |

A reading that collected every word after the operand would answer
`unknown condition: -n` for the third, and one that fired on any `-word`
would refuse the first.

## A process substitution as an operand — bash only

    [[ x == <(:) ]]

| shell | what happens |
| --- | --- |
| bash 3.2, 5.3 | the command runs and the operand is the path; the test is false |
| zsh 5.9 | `process substitution <(:) cannot be used here`, and the rest of the input does not run |
| ksh93 | ``syntax error … `<(' unexpected``, while reading |
| dash | no `[[ ]]` at all |

Three shells say no and differ only in *when* and in *what words*, which
is exactly the line between the semantics vector and Diagnostics:
`ProcessSubstitutionInCondition` is the axis — bash `Yes`, ksh93 and zsh
`No`, unanswered in `core` — and `ProcessSubstitutionNotInCondition` is
the sentence.

The axis is asked **before** the word is expanded. A shell that refuses
the operand must not have started the command first, and that is
observable: the command has side effects, and a refusal that came after
the expansion would leave them behind.

### Every operand, and a status that belongs to the expression

It is asked at **every operand of every operator**, not only at the one a
comparison holds. Measured 2026-09-18 on zsh 5.9.2, script files under
`env -i PATH=/usr/bin:/bin LC_ALL=C`, with a `printf` in front of the
condition and another behind it:

| written | named | status |
| --- | --- | --- |
| `[[ -e <(echo x) ]]` | `<(echo x)` | 1 |
| `[[ -n <(echo x) ]]` | `<(echo x)` | 1 |
| `[[ <(echo x) == x ]]` | `<(echo x)` | 1 |
| `[[ x -nt <(echo x) ]]` | `<(echo x)` | 1 |
| `[[ a =~ <(echo x) ]]` | `<(echo x)` | 1 |
| `[[ ! -e <(echo x) ]]` | `<(echo x)` | 1 |
| `[[ ( -e <(echo x) ) ]]` | `<(echo x)` | 1 |
| `[[ x == <(echo x) ]]` | `<(echo x)` | **2** |
| `[[ x != <(echo x) ]]` | `<(echo x)` | **2** |
| `[[ x == >(echo x) ]]` | `>(echo x)` | 1 |
| `[[ x == =(echo x) ]]` | `=(echo x)` | 1 |
| `[[ <(echo x) == <(echo y) ]]` | `<(echo x)` | **2** |
| `[[ <(echo x) == >(echo y) ]]` | `<(echo x)` | 1 |

The first `printf` runs and the second does not in all thirteen, so the
sentence and the abandoning are one answer; what moves is the number.
**One is the answer and 2 is the exception**: it comes back only where the
**right** operand of a pattern comparison is the **input** spelling. The
last two rows are what say the status belongs to the expression rather
than to the word the sentence names — both refuse the *left* substitution
by name and differ only in what stands behind the operator.

It is also **lazy**, because the condition is: `[[ x == y && -e <(echo x) ]]`
starts no command and answers 1.

This tree asked the axis at the pattern comparison's right operand alone,
so a file test ran the command and answered true, and `>(` left 2 where
the shell leaves 1 (#3280).

ksh93's refusal is the parser's, and it is **not about conditions**.
Re-measured 2026-09-13 under `env -i PATH=/usr/bin:/bin` with a scratch
`HOME`, ksh93u+ 2012-08-01 over `-c`, the opener is refused in five
positions and taken in two:

| written | ksh93 |
| --- | --- |
| `[[ x == <(:) ]]`, `[[ -f <(:) ]]`, `[[ <(:) ]]` | ``` `<(' unexpected ```, 3 |
| `case <(:) in *) :;; esac` | the same |
| `case x in <(:)) :;; esac` | the same |
| `for i in <(:); do :; done` | the same |
| `select i in <(:); do break; done` | the same |
| `a=( <(:) )` | the same |
| `cat <<< <(:)` | the same |
| `[[ x == >(:) ]]` | ``` `>(' unexpected ```, 3 |
| `echo <(:)`, `cat <(:)`, `: <(:)`, `set -- <(:)` | a path |
| `cat < <(:)` | runs — a redirection target |
| `for i in a; do echo <(:); done` | a path — the body is commands |

So the rule is **where a word stands**, not what a condition may hold: a
process substitution stands only where a command takes a word — an
argument, or a file redirection's target — and the condition is one of
five positions that are not it. The grammar flag is named for that:
`ProcessSubstitutionOnlyWhereACommandTakesAWord`, ksh93 only. A flag
spelled for the condition operand would have accepted the other eight
lines.

It is the parse and not the run, which the rows above cannot show on
their own. `false && [[ x == <(:) ]]; echo reached` prints nothing and
exits 3 — the condition is in a branch never taken, so a shell refusing
it at the run would have printed `reached`. zsh prints it.

Two boundaries the same measurement draws, and neither is this rule:

- At the *start* of a command ksh93 never lexes the opener, `<` being a
  redirection operator there. `if <(:); then :; fi`, `echo | <(:)` and
  `! <(:)` are ``` `)' unexpected ``` — the paren, not the pair.
- The opener also ends the word before it: `echo a<(:)b` writes three
  fields in ksh93 where this shell writes one.

## An operand whose expansion failed ends the condition

    [[ $((1/0)) -eq 0 ]]

The expansion fails, every column says so, and what the condition then
answers is where they part. Measured 2026-09-18, script files under
`env -i PATH=/usr/bin:/bin LC_ALL=C`, with a `printf` on either side:

| shell | what happens |
| --- | --- |
| bash 5.3.20, bash 3.2.57 | the division is reported, the condition is **false** at 1, and the next line runs |
| zsh 5.9.2, ksh93u+, BusyBox ash | the division is reported and the script ends |

So the panel needs **no axis of its own here**: a failed expansion is
already fatal in three of the four columns and already not in the fourth,
which `Semantics.FailedExpansionAbandonsTheLine` and
`Semantics.FatalErrorStatusIsOne` answer. What was missing is that the
condition asked neither of them — the failed expansion left an empty
string, and an empty string compares equal to zero.

**The condition is abandoned whole rather than the primary being false**,
which two shapes say and a plain false could not:

| written | bash | a false primary would give |
| --- | --- | --- |
| `[[ ! $((1/0)) -eq 0 ]]` | 1 | 0 |
| `[[ $((1/0)) -eq 0 \|\| 1 -eq 1 ]]` | 1 | 0 |

and `[[ 1 -eq 1 \|\| $((1/0)) -eq 0 ]]` is the control: 0, with no
division attempted at all, because the condition is evaluated left to
right and lazily. The left operand is settled before the right one is
expanded, so a command written in the right operand does not run.

**An empty operand is not this.** `[[ "" -eq 0 ]]` and
`[[ $nosuch -eq 0 ]]` are both 0, so the rule is about the expansion
having failed and not about the text it left behind — which is what makes
`[[ -z $((1/0)) ]]` and `[[ $((1/0)) == "" ]]` the sharpest rows: an empty
string passes both and the failure answers 1 (#3556).

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

### What a numeric operand is read as

Inside `[[ ]]` it is an **arithmetic expression** in every shell that has
the construct, so `[[ n -eq 5 ]]` holds with `n=5`. The single-bracket
`test` and `[` are where the panel splits. Measured 2026-09-12, `-c`
under `env -i`:

| probe | dash | bash 5.3 | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `n=5; [ n -eq 5 ]` | refuses | refuses | refuses | **true** | refuses |
| `[ 1+1 -eq 2 ]` | refuses | refuses | refuses | **true** | refuses |
| `[ 16#10 -eq 16 ]` | refuses | refuses | refuses | **true** | refuses |
| `n=5; [ "n=9" -eq 9 ]` | refuses | refuses | refuses | **true, and `n` is 9** | refuses |

The last row is what says ksh93 reads the whole expression language
there rather than looking a name up: an assignment written in an operand
lands. A refusal on that side is the arithmetic's complaint behind the
name the builtin was called by, at status 1 and **not** fatal —
`[ 1x1 -eq 0 ]` is `ksh: [: 1x1: arithmetic syntax error` and the script
runs on, where `[[ 1x1 -eq 0 ]]` in the same shell abandons the input.

Semantics axis: `TestBuiltinComparisonOperandsAreArithmetic` (ksh93 yes,
the other five no; the core's preset is no, POSIX giving `-eq` two
integers to compare).

### A condition operand's leading zeros come off in front of a prefix

The shell that reads an operand as arithmetic also reads a leading zero
in it in **decimal** rather than as an octal prefix — the same rewrite
`ArithStoredValueReadsALeadingZeroAsDecimal` records for a value read out
of a variable. At a condition operand it reaches one step further.
Measured 2026-09-12 on ksh93u+ 2012-08-01:

| written | as a condition operand | as `k=…; $(( k ))` |
| --- | --- | --- |
| `010` | 10 | 10 |
| `0x10`, with `x10=7` | 7 | 16 |
| `0x10`, with `x10` unset | 0 | 16 |
| `0xg`, with `xg=9` | 9 | arithmetic syntax error |
| `00x10`, with `x10=7` | 7 | 7 |
| `1+0x10` | 17 | 17 |

So the zeros are taken off the front of the text and what follows is
read as an expression; a single zero in front of an `x` survives that in
a *value* and does not in a condition operand, which is why
`[[ 0x10 -eq 16 ]]` is false in ksh93 while `$(( 0x10 ))` is sixteen. The
last row is the control: with something in front of the zero there is
nothing to take off, and the reader plainly does have hex in it.

Corpus: `test/comparison-operands-are-arithmetic`,
`test/a-comparison-operand-that-will-not-read`,
`test/a-comparison-operands-leading-zeros`,
`arith/a-values-leading-zeros-in-front-of-a-name`.

### An empty `=~` operand is refused two ways

POSIX ERE has no empty expression, and the two columns built on one say so
— in different words and at different statuses. Measured 2026-09-18,
`[[ abc =~ "" ]]` in a script file:

| shell | says | status |
| --- | --- | --- |
| bash 5.3.20 | ``[[: invalid regular expression `': empty (sub)expression`` | 2 |
| zsh 5.9.2 | `failed to compile regex: empty (sub)expression` | **1** |
| ksh93u+, BusyBox ash | nothing — the empty pattern matches | 0 |

`Semantics.EmptyRegexOperandIsAnError` says *whether*, and it has to,
because the engine this shell is built on has the third opinion: Go's
regexp compiles the empty pattern and matches the empty string at every
position. `Diagnostics.EmptyRegexOperand` carries the sentence and
`Diagnostics.EmptyRegexOperandStatus` the number — 2 where the construct
**failed** and 1 where it is a match that did not happen, which is the
same number a condition that simply did not hold gives. `[[ abc =~ b ]]`
at 0 and `[[ abc =~ x ]]` at 1 are the controls that say the operator
works in both (#3279).

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

**Quoting decides the operand a span at a time**, exactly as it decides a
glob operand: the quoted *portions* are literal and the rest stays an
expression. Measured 2026-09-22 on bash 5.3.20:

| probe | bash 5.3.20 |
| --- | --- |
| `[[ ab =~ ^"a"b$ ]]` | matches — the anchors are still anchors |
| `[[ ab =~ ^'ab'$ ]]` | matches |
| `[[ ab =~ ^\ab$ ]]` | matches — a backslash quotes its one character |
| `[[ axb =~ "a".b ]]` | matches — the `.` outside the quote is a `.` |
| `[[ aab =~ "a"a*b ]]` | matches |
| `[[ x =~ [$"a"-z] ]]` | matches — a quote inside a bracket expression |
| `[[ axb =~ "a.b" ]]` | **no match** — the control, where the quote is about the `.` |
| `[[ a.b =~ "a.b" ]]` | matches |

So the operand cannot be escaped as a finished string: only the spans know
which characters were quoted, which is why the tree keeps the word here as it
does for a pattern. This shell asked the *word* whether anything in it was
quoted and escaped the whole expanded value, so one quote anywhere turned
every metacharacter in the operand into a letter (#4173).

**The match is leftmost-longest.** POSIX defines a regular expression match
that way, and Go's `regexp` prefers the leftmost match the first alternative
reaches instead. The status is the same either way and the recorded match is
not, so the difference is a value a script carries forward:

| probe | bash 5.3.20 |
| --- | --- |
| `r='a\|ab'; [[ ab =~ $r ]]` | matches `ab` |
| `r='ab\|a'; [[ ab =~ $r ]]` | matches `ab` |
| `r='a\|aa\|aaa'; [[ aaa =~ $r ]]` | matches `aaa` |
| `r='x\|aaa'; [[ xaaa =~ $r ]]` | matches `x` — leftmost still wins over longer |

Not an axis: it is the expression language's rule and every column with the
operator compiles ERE.

## An operand ending in `(#q…)` is matched against the filesystem

Nothing inside `[[ … ]]` is split or globbed, which is why `[[ -z $u ]]`
needs no quoting where the `[` builtin does. There is one exception, and
the vendor manual states it as a rule rather than leaving it to be found:
within conditions using the `[[` form, a parenthesized `(#q…)` expression
at the end of a string says that globbing should be performed. The
expression may hold glob qualifiers and is valid as a bare `(#q)`. It
does **not** apply to the right-hand side of a pattern-match operator,
where the syntax already means something else.

Measured 2026-09-12 on zsh 5.9.2 with `extendedglob` on, in a directory
holding `a.txt` and `b.txt` and nothing called `zz`:

| probe | result |
| --- | --- |
| `[[ -n a.txt(#qN) ]]` | true |
| `[[ -n zz(#qN) ]]` | **false** — the word came to nothing |
| `[[ -n zz ]]` | true, the same word without the group |
| `[[ -n zz(#q) ]]` | `no matches found`, the miss the dialect gives |
| `[[ -e a.txt(#qN) ]]` | true, so a file test's operand is matched |
| `[[ a.txt(#qN) == a.txt ]]` | true, so the *left* side is matched |
| `[[ *.txt(#qN) == 'a.txt b.txt' ]]` | true — two matches are one operand, joined with a space |

And five that say the group has to be **written**, at the end, and
unquoted. Each is true, meaning the word stayed the text it was:

| probe | why it does not glob |
| --- | --- |
| `[[ -n "zz(#qN)" ]]` | the word is quoted |
| `[[ -n zz"(#qN)" ]]` | only the group is |
| `V='zz(#qN)'; [[ -n $V ]]` | the group is a value, not written |
| `[[ -n ${~V} ]]` | and the tilde flag does not change that |
| `[[ -n zz(#qN)x ]]` | the group is not at the end |

The `${~V}` row is the one that settles the shape. That flag makes a
value's pattern characters live everywhere else, so a reading that looked
at the expanded text would glob here; the group is read off the word as
the script wrote it.

It is gated on the dialect having glob qualifiers at all rather than on a
semantics axis: a shell without `(#q…)` reads those six characters as
text, which is the same answer it gives for the whole construct, so there
is no disagreement for an axis to record.

**This is how powerlevel10k's directory segment shortens.** Its
`prompt_dir` asks `[[ -n $dir/${~MARKER}(#qN) ]]` of each component of
the working directory to decide which components are *anchors* —
directories holding `.git`, `go.mod` and the like — and an anchor is
never shortened. A shell that reads the word as text answers true for
every component, so every component is an anchor and the path is drawn at
full length at every width, with no arithmetic anywhere having gone wrong
(#2119).

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

### An empty operand is not a path

Every file test above is **false** when its operand is empty, without
asking the filesystem. Measured 2026-09-07 on bash 5.3, bash 3.2, dash,
ksh93 and zsh 5.9.2 under `env -i`, in `[ ]`, `test` and `[[ ]]`: all
sixteen operators, all five shells, status 1. Unanimous, so core, and no
axis.

It is worth its own entry because getting it wrong is silent. An empty
operand joined onto the shell's working directory becomes a path that
*certainly* exists, so `-e`, `-d`, `-s`, `-r`, `-w` and `-x` all answer
**true** — and `-f` still answers false, a directory not being a regular
file. Six wrong and one right, and the one that is right is the one most
scripts use, so nothing looks broken until something else fails:

    [[ -d $XDG_CACHE_HOME ]] && CACHE=$XDG_CACHE_HOME/app

With the variable unset that guard passes, `CACHE` becomes `/app`, and
what is observed is a `mkdir` failing at the filesystem root, several
frames away from the test that lied. `[[ -d $dir ]]` is *the* idiom for
"did I compute a path", and it has to be able to say no.

The pair that tells the two readings apart:

    [[ -d "" ]]   →  false
    [[ -d .  ]]   →  true

`.` is the spelling that means the working directory. An empty word
names nothing.

Unset, set-and-empty, and a substitution that printed nothing are one
case, not three: expansion has already made them the same empty word
before the test sees it, and the panel does not distinguish them either.
A caller needs all three rejected and gets that from the one rule.

The binary comparisons take their operands the same way, so the rule is
theirs too — an empty side is a file that is not there, which puts it on
the `MissingFileIsOlder` axis below with exactly the values a missing
*name* gets. `[[ "" -ef "" ]]` is false everywhere; resolving both sides
against the working directory would compare that directory with itself
and answer true.

Corpus: `cond/empty-operand-is-not-a-path`,
`cond/empty-operand-is-not-the-working-directory`,
`cond/empty-operand-in-the-file-comparisons`,
`test/empty-operand-is-not-a-path`,
`test/empty-operand-however-it-became-empty`.

`-t` asks whether **this shell's** descriptor is a terminal. The number
is the shell's own and not the process's — `exec 3< file` puts a file
at 3 and `-t 3` answers about it — and the three standard ones are
whatever the front end handed in, so a shell binary answers about the
terminal it was started at and a runner an embedder built answers about
the streams it was given. Measured on a pseudo-terminal, all six panel
columns: `[ -t 0 ]`, `test -t 0`, `[ -t 1 ]`, `[ -t 2 ]` and
`[[ -t 0 ]]` are all **true**.

It is false for everything that is not one, unanimously: the null
device, a regular file, a pipe, a descriptor nothing is open at, and a
negative number. Those answers are the same at a terminal as on a pipe,
which is why the corpus rows redirect each descriptor rather than
reading whatever the run was handed.

This answered a **fixed false** until #1967, in all three spellings.
That is the correct answer for a stream that is not an open file, and
it was being given to every descriptor — so `[[ -t 1 ]]` in a startup
file took the non-terminal arm in a real session, which is the branch a
rc uses to decide whether to draw color, load a prompt theme or install
completions.

With **no operand at all** the panel splits. POSIX gives `test` with
one argument to the string rule, and `-t` is a non-empty string:

| probe, descriptor 1 redirected away from any terminal | dash | bash | bash-as-`sh` | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `[ -t ]` | 0 | 0 | 0 | 0 | **1** | **1** |
| `test -t` | 0 | 0 | 0 | 0 | **1** | **1** |
| `[ ! -t ]` | 1 | 1 | 1 | 1 | **0** | **0** |
| `[ -f ]` | 0 | 0 | 0 | 0 | 0 | 0 |

ksh93 and zsh read the lone `-t` as `-t 1`; the other four keep the
string rule. At a pseudo-terminal with nothing redirected all six
answer 0, which is what says the two are answering about descriptor 1
rather than refusing the word. `-t` is alone in this — `[ -f ]` is
beside it in the table because it is true in all six, so the exception
is the one word and not a general rule about an operator with no
operand. `[[ -t ]]` asks nothing: every shell in the panel that has the
construct refuses it as a syntax error.

Semantics axis: `BareTerminalTestIsDescriptorOne` (ksh93 and zsh yes,
dash and the three bashes no; the core's preset is no, POSIX having
given the one-argument form to the string rule with no exception in
it).

The operand diverges when it is not a number at all:

| probe | bash | bash 3.2 | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `[[ -t x ]]` | **`integer expected`, status 2** | 1 | 1 | 1 |
| `test -t x` | **status 2** | 1 | 1 | 1 (dash: **status 2**) |

bash refuses loudly — and bash 3.2 does not, so the complaint is
younger than the operator. dash, which reaches the question only
through `test`, refuses too with its `Illegal number` wording.

Semantics axis: `TerminalTestRequiresANumber` (bash and dash yes, ksh93
and zsh no; unanswered in the core), asked only for such an operand.

A number too wide for the descriptor is a second question, and one shell
answers it. Measured 2026-09-12 under a pseudo-terminal with descriptors
0 and 1 on the terminal and 2 redirected away:

| operand | narrows to | ksh93 | bash 5.3 |
| --- | --- | --- | --- |
| `-1` | -1 | **true** | false |
| `-2` | -2 | false | false |
| `4294967296` | 0 | **true** | false |
| `4294967297` | 1 | **true** | false |
| `4294967298` | 2 | false | false |
| `9223372036854775807` | -1 | **true** | false |
| `99999999999999999999` | -1 | **true** | `integer expected`, 2 |

ksh93 reads the operand at the width of a machine `int`: a value too
wide for the shell's own integer saturates, and what is left is taken
modulo 2**32 as a signed number. The `4294967298` row is the control
that makes it a narrowing rather than "a big number is true" — it is
descriptor 2, and descriptor 2 is not a terminal in that run. Repeating
the whole table with every stream redirected to a file answers true only
for the operands that narrow to -1, so -1 holds whatever the shell is
holding, and `-2`, `-3` and `-100` are false in all six columns.

Semantics axes: `TerminalTestDescriptorNarrowsToThirtyTwoBits` and
`TerminalTestMinusOneIsATerminal` (ksh93 yes, the other five no; the
core's preset is no for both), each asked only where it can change the
answer.

Corpus: `test/a-terminal-test-descriptor-too-wide`,
`cond/terminal-test-closed-descriptors`,
`cond/terminal-test-redirected-descriptors`,
`cond/terminal-test-non-number-diverges`,
`test/terminal-test-redirected-descriptors`,
`test/terminal-test-non-number-diverges`,
`test/bare-terminal-test-is-descriptor-one`.

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

zsh's `-word` conditions **other than the four completion ones**, which
are the same gap reached from the other end. Measured 2026-09-05 on zsh 5.9.2, with `-n` for the parse and
a run for the rest:

| probe | parses | run |
| --- | --- | --- |
| `[[ -prefix x ]]` | yes | `condition can only be used in completion function`, 1 |
| `[[ -suffix x ]]` | yes | the same, 1 |
| `[[ -after x ]]` | yes | **SIGSEGV** |
| `[[ -between x ]]` | yes | `unknown condition: -between`, 2 — the wrong arity, and `[[ -between x y ]]` is the operator |
| `[[ -before x ]]`, `-equal` | yes | `unknown condition: -X`, 2 |
| `[[ -nosuch x ]]` | yes | `unknown condition: -nosuch`, 2 |

So the general rule in that shell is that **any** `-word` followed by an
operand parses as a unary condition and an unknown one is refused when it
runs — which is precisely the shape the rule above forbids, and adopting
it would be a decision to change the rule rather than a gap to fill. That
question is #965, which took the half of it that is about a **known**
operator's arity — see the section above — and left this one: an operator
this shell does not have at all is still refused while reading here.
The four completion conditions were taken *as named operators* instead,
which keeps the rule, and leaves `-before`, `-equal` and the rest here.
The other four disagree with it and with each other: bash 5 and ksh93
make `[[ -nosuch x ]]` a syntax error, and bash 3.2 accepts it.

`-after` crashing rather than refusing is a bug in that build, not a
behavior to model; it also means the row cannot be graded against a run.
