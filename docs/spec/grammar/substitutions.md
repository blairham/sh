# Substitutions

`$(…)`, `` `…` ``, `$((…))`, `${…}` and `<(…)` — specifically **where
they end**, which is what the lexer needs to know. What is *inside* them
is a later document: this one is about delimiting, because a word's
boundaries depend on it and nothing else can be tokenized until they are
right.

Citation: POSIX.1-2024 XCU §2.6.2–§2.6.4. Panel measurements as in
`../oracle.md`.

## A substitution does not end the word

    $(printf a)b            →  one word, "ab"
    x$(printf "a b")y       →  two fields, [xa] and [by]

Unanimous. The first shows that a substitution is *part of* a word rather
than a word of its own. The second shows why that matters: the result is
field-split like any other unquoted expansion, and the literal text on
either side attaches to the first and last resulting fields rather than
becoming separate words.

A lexer that emitted a substitution as its own token would make both
impossible to reconstruct.

## The closing delimiter is not found by counting

This is the rule that decides the implementation:

| probe | all four |
| --- | --- |
| `echo "[$(echo ")" )]"` | `[)]` |
| `echo "[$(echo "a)b")]"` | `[a)b]` |

A `)` inside quotes **does not close the substitution**. Scanning for a
balanced paren by counting is therefore wrong, and wrong in a way that
truncates the substitution and silently changes the program.

Finding the end requires tracking quoting while scanning — the same
quoting rules as `tokenization.md`, applied recursively. In practice the
scanner is the lexer again, run to a closing token.

Nesting works for the same reason and needs no separate rule:

    $(echo "$(echo deep)")  →  deep

## `$((` opens two constructs, and the parentheses say which

    $((1+2))        →  3       arithmetic
    $( (echo sub) ) →  sub     a subshell inside a substitution

Unanimous with the space written, which is the spelling POSIX tells an
author to use. POSIX says nothing about the shell that meets the two
parentheses together, and the panel does not agree there:

| probe | bash 5.3, bash 3.2, bash-as-`sh`, ksh93, zsh | dash, ash |
| --- | --- | --- |
| `echo $((echo ab cde) )` | `ab cde` | refused: missing `))` |

Five of seven fall back to reading `$( ( … ) … )`. That is the same head
count that put process substitution in the core, so the core has the
fallback and the two minimal shells turn it off
(`Dialect.ArithSubstFallsBackToCommandSubst`). Measured 2026-09-12; dash
0.5.12 and BusyBox ash 1.37.0 in containers, the rest on the machine.

**The rule is positional, not "try arithmetic and fall back on a parse
failure".** Counting from one after the `$((`, find the `)` that brings
the count back to zero: the construct is arithmetic when the very next
byte is another `)`, and a command substitution otherwise.

| probe | five shells |
| --- | --- |
| `echo $(( (1+2) ))` | `3` — the closers touch |
| `echo $(( (1+2)) )` | runs `1+2` as a command |
| `echo $(( 1 ) + (2 ))` | a command substitution, and a syntax error inside it |

The third is what rules the other reading out: `(1) + (2)` is perfectly
good arithmetic, and the five still read it as a substitution, because
the first `)` closes the count and a `+` follows it.

Where the input runs out before the count reaches zero, the construct
stays arithmetic and the complaint is the one an unfinished `$((` gets.

Quoting is the one edge the five do not share:

| probe | bash 5.3, 3.2, bash-as-`sh` | ksh93, zsh |
| --- | --- | --- |
| `echo $(( '0)' + 1 ))` | an arithmetic error | runs `0)` as a command |

bash tracks quoting in the deciding scan, so a `)` inside a string closes
nothing — the same rule as *The closing delimiter is not found by
counting* above. ksh93 and zsh let it close. We follow bash.

The choice belongs to the lexer, since by the time the parser sees tokens
it has been made.

## Backticks

    `echo hi`               →  hi
    `echo \`echo deep\``    →  deep

The older form. It nests only with backslash escaping, which is why the
`$( )` form exists and why this one is worth supporting but never
recommending. Its delimiter is a backtick that is not backslash-escaped.

### A line continuation is removed before the text is parsed

A backslash-newline inside the backquotes is taken out — both characters,
before the command text is read — and it is taken out **whatever quoting
it stands in inside that text**. This is the one rule where the two
spellings of a command substitution part company, and it is the reason
`tokenization.md`'s "a continuation does not apply inside single quotes"
has an exception.

Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin
LC_ALL=C`, and unanimous in bash 5.3, bash 3.2, zsh 5.9.2, ksh93u+ and
dash:

    "`printf %s 'a\
    b'`"                    →  [ab]    inside single quotes
    "`printf %s "a\
    b"`"                    →  [ab]    inside double quotes
    "`printf %s a\
    b`"                     →  [ab]    unquoted in the text
    "`cat <<'E'
    a\
    b
    E
    `"                      →  [ab]    inside a *quoted* here-document body

The last row is what says this is not a continuation the inner parse
removes for itself: nothing inside a quoted here-document removes one.
The pair is gone before that parse begins.

`$( )` is the control and keeps the pair in all five columns:

    "$(printf %s 'a\
    b')"                    →  a, a backslash, a newline, b

An escaped backslash is not this. `'a\\⏎b'` is `a\⏎b` in all five: the
`\\` is unescaped to one backslash and the newline behind it is an
ordinary character — which falls out of doing the removal in the same
pass that unescapes the rest.

It reaches arithmetic, because the substitution's value is what the
expression is read from: `$(( `printf %s 'a\⏎b' | wc -c` ))` is 2 on the
whole panel (#3453).

### One shell says so, and only when it is not going to run

ksh93 remarks on every backquote substitution it reads:

    $ ksh -n bq.sh
    bq.sh: warning: line 1: `...` obsolete, use $(...)

Measured 2026-09-12 on 93u+ 2012-08-01, from a script file, from `-c`
and from standard input. dash, bash 5.3, bash-as-`sh`, bash 3.2 and zsh
read a backquote without a word, so this is one column's and the
wording is a `Diagnostics` field, `BackquoteObsolete`, empty in the
rest.

**It is said only while the shell is not going to execute**, which is
the half that decides where the rule lives:

| route | ksh93 |
| --- | --- |
| `ksh -n bq.sh` | one line per backquote |
| `ksh bq.sh` | nothing at all |
| ``ksh -n -c 'x=`echo hi`'`` | one line |
| ``ksh -c 'x=`echo hi`'`` | nothing |
| a backquote typed at a prompt | nothing |

So the parse produces the remark either way and the front end decides
whether to say it — `interp.RemarkOnlyWhenNotRunning` answers that per
remark kind, because the other remark this substrate has, a
here-document that ran to the end of the input, is said either way.

It accompanies a refusal as well, warning first:

    $ ksh -n bqu.sh
    bqu.sh: warning: line 1: `...` obsolete, use $(...)
    bqu.sh: syntax error at line 1: ``' unmatched

which is the same shape the here-document remark has, and the reason
remarks are read whether or not the parse also failed.

The line is inside the sentence rather than in the location in front of
it — `<script>: warning: line 1: …` and not `<script>: line 1: warning:
…` — which is the split this shell's syntax errors already make and
which `Diagnostics.RemarkNamesItsOwnLine` records.

Corpus: `core/a-backquote-substitution-under-a-syntax-check`,
`core/a-backquote-substitution-when-it-runs`.

And it is not alone. `ksh -n` is a **lint mode with rules** rather than a
parse check that happens to warn: two operators written with no blank
between them draw a second line of the same shape, on the same route and
under the same restriction. See *Two operators with no blank between
them* in `tokenization.md` (#2409).

## `${…}`

`${x:-y}` and its relatives lex as one word, and the delimiting rule is
the same as for `$( )` — track quoting, stop at the matching close. That
is what makes a `}` inside a quoted section ordinary rather than the end
of the expansion:

    ${x:-"a}b"}   →  a}b   unanimous
                          (`subst/brace-in-quotes-does-not-close`)

Counting braces without tracking quotes would stop at the first `}` and
leave `b"}` behind as text. The **operators inside** (`:-`, `#`, `%`,
`/`, `:offset:length`, `[index]`) are a separate specification this
document does not attempt. The lexer only has to find the end.

### Tracking the quoting is not enough on its own

A quoted run inside the body can hold a substitution, and what that
substitution holds is a **program**, not more of the run's text. Its
quoting is the program's own, so a quote character inside it is not a
quote character of the run:

| probe | all six |
| --- | --- |
| `${x:-"$( echo 'a"b' )"}` | `a"b` |
| `${x:-"$( echo "c'd" )"}` | `c'd` |
| `${x:-"$(( 1 + $( echo 1 ) ))"}` | `2` |
| ``${x:-"`echo 'e"f'`"}`` | `e"f` |

    (core/a-substitution-inside-an-expansion-brings-its-own-quoting)
    (core/what-a-nested-substitution-holds-is-not-a-delimiter)
    (core/the-older-substitution-spelling-brings-its-own-quoting-too)

"Its own quoting" is the short way of saying "it is a program", and the
parentheses go with it. A `case` arm's `)` closes nothing here either,
one level further in than the rule above:

    ${x:-"$( echo `case a in a) echo y;; esac` )"}   →  y   all six

    (core/a-case-arm-inside-backquotes-inside-an-expansion)

The scan for the closing `}` therefore has to step over such a
substitution whole rather than read across it. A scan that does not takes
the `"` in `'"'` as the run's closer, continues from the `'` after it as
though a single-quoted string began there, and swallows the rest of the
input.

The failure is not always a refusal, which is what makes it worth a rule
of its own. Two of the shape in one line put the stray quotes back in
balance:

    printf '[%s]' "${x:-"$( echo 'a"b' )"}" "${y:-"$( echo "c'd" )"}"
      →  [a"b][c'd]     all six
      →  [a"b} c'd]     a scan that reads across, exit status 0

One field where there are two, the grammar's own `}` in the output, and
no diagnostic anywhere.

    (core/a-swallowed-quote-changes-the-program-rather-than-refusing-it)

**A nested `${ }` is not one of these.** Its body is a word rather than a
program, and the panel divides on it — `${x:-"${y:-'"'}"}` is accepted by
bash alone and refused by the other five — so it is a dialect question
and not part of this rule.

### Every scanner that has to find a closing bracket reads the same way

The rule is not the expansion's; it is the *substitution's*, so it holds
wherever one can be written. `(( … ))` is the site that had it wrong
longest, and the C-style `for` header — which is that token cut on its
own semicolons — inherited the mistake:

| probe | bash 5.3 | ksh93 | zsh |
| --- | --- | --- | --- |
| `(( $(case q in q) echo 7;; esac) ))` | runs | runs | runs |
| `for (( $(case q in q) echo 7;; esac) ;; ))` | runs | runs | runs |
| `for (( ${ case q in q) true;; esac; };; ))` | runs | runs | `bad substitution` |

Measured 2026-09-12; zsh's last row is that shell not having the brace
form of a substitution at all, which is a different fact and is recorded
in `parameter-expansion.md`. Counting parentheses stopped the arm at its
pattern and left the rest of the arithmetic looking like a stray `)`, or — once
the header was cut before it was read — like a header holding three
separators. `$((` is the one shape left to the counting, deliberately: it
holds an expression rather than a program, so its two opening parentheses
balance against its two closers on their own.

## `${ cmd;}` — a substitution that runs in the current shell

A space after the brace turns the same delimiters into a **command**
substitution whose command runs in the shell itself rather than in a
subshell, so an assignment inside it survives:

    echo "[${ echo hi;}]"     →  [hi]   in bash 5.3 and ksh93
                                        bad substitution in bash 3.2,
                                        dash and zsh

Measured 2026-09-05 across the panel. bash 3.2 is on the refusing side,
which dates the construct rather than disputing it — but the two shells
that have it are bash 5.3 and ksh93, so it is a two-shell construct and
not core.

**The space is the entire grammar.** `${x}` is a parameter and `${ x}` is
a command, and the decision is made on the one character after the brace
rather than by trying to read a name and failing — which is possible
because a parameter name may not begin with a blank. Measured, the
characters that make it a command are exactly space, tab and newline.

The body is delimited here as the parameter form is: a `}` inside quotes
does not close either, and both nest. Grammar flag:
`CurrentShellSubstitution`, on for bash and ksh, off elsewhere including
the core.

**That last sentence is what this shell does and not what the two shells
do**, measured 2026-09-13 and filed as #2724. Their rules differ from
each other: bash ends the body where `{ list; }` ends, at a `}` in
command position, so `${ echo } ;}` passes a literal brace to `echo`
there; ksh93 ends it at a `}` that begins a **token**, argument position
included, and refuses that same line. Ours takes the first unquoted `}`
wherever it stands, which agrees with neither on `${ echo a}b;}` — `a}b`
in both shells and a syntax error here — or on `${ echo hi}`, which both
shells refuse and this shell runs.

### The body is a frame a `return` leaves

Measured 2026-09-16 from script files under `env -i`, standard input the
null device:

    v=${ echo hi; return 42; }; printf '[%s] st=%s\n' "$v" "$?"; echo alive

    bash 5.3.20   [hi] st=42  alive
    ksh93u+       [hi] st=42  alive

So the body is something to return from: the value is what it printed up
to the `return`, the status is the operand, and the script carries on.
The forked spelling is the control and answers the other way — `v=$(echo
hi; return 42)` is `return: can only 'return' from a function or sourced
script` at status 2 in bash, because there the body really is a shell of
its own with no frame in it. Inside a function the body's `return` leaves
the *body*: the function runs on.

### And in one column the frame carries a variable scope

The two columns that have the spelling disagree about whether a
declaration written in a body is local to it, which makes this an axis —
`Semantics.CurrentShellSubstitutionBodyIsAScope` — rather than a
correction. Measured 2026-09-16, printing the substitution's value and
then the shell's own name:

| probe | bash 5.3.20 | ksh93u+ 2012 |
| --- | --- | --- |
| `x=outer; v=${ typeset x=in; printf %s "$x"; }` | `[in][outer]` | `[in][in]` |
| `x=outer; v=${ declare x=in; …` | `[in][outer]` | no `declare` |
| `x=outer; v=${ local x=in; …` | `[in][outer]` | no `local` |
| `f() { local x=fn; v=${ local x=in; … }; f` | `[in][fn]` | — |

`typeset` is the row that decides it, being the one spelling both columns
have. So in bash the body behaves like a call for declarations and in
ksh93 it does not, and POSIX has nothing to follow here — the standard
has neither the spelling nor `local`. The preset takes ksh93's reading
for the same reason it takes it nowhere else: the spelling's own
definition is that the body runs in *this* shell, whose variables are the
ones it has, and a scope is what bash adds on top of that.

Three things bound the axis, and each is a row a wrong reading gets
wrong. A **plain assignment** is not a declaration and still writes the
shell's own name in both columns — `x=outer; v=${ x=in; }` leaves `x` as
`in` — which is the whole difference between this spelling and the forked
one, unchanged. `local` **being legal** inside a body follows from the
scope rather than being a second decision: the word is refused for having
no function to be local to, and a body with a scope has one; in `$( … )`
it is refused in bash exactly as it is at the top level, which is the
control that says this is about the shared-state form. And the scopes
**nest**: a body inside a call unwinds into the call's declaration and not
past it.

The scope is opened inside the `${| … ;}` form's hiding of `REPLY` and
closed before it, so a body that declares `REPLY` shadows the hidden name
and the outer one is put back over that. The two are separate mechanisms
and not one seen twice — measured on bash 5.3.20, `REPLY=outer; v=${|
unset REPLY; }` is the empty string with `REPLY` still `outer`, which the
hiding answers on its own. #2656 recorded that row as the visible corner
of the missing scope, on the reading that unsetting a local reveals what
it shadows; re-measured with the rest of this section, bash does not do
that, so the note is corrected rather than carried.

## `${(list)}` — the parenthesized body

A `(` adjacent to the brace is a spelling of its own, and it belongs to
one shell:

    echo "A${(echo hi)}B"     →  AhiB   in ksh93
                                        bad substitution in bash 5.3,
                                        bash 3.2, dash and ash;
                                        zsh reads the parenthesis as its
                                        own expansion flags

Measured 2026-09-13 on ksh93u+ 2012-08-01. What runs is the parenthesis,
so nothing it assigns survives and nowhere it goes is anywhere the caller
went — `v=1; echo ${(v=2; echo x)}` leaves `v` at 1, and
`${(cd /tmp; pwd)}` leaves the caller's directory alone.

**Its extent is the parenthesis and not a command list**, which is the
whole reason it is a spelling of its own rather than the form above with
a `(` for an opener. The isolation invites the other reading — a body
whose first command happens to fork would answer both rows above
identically — and these are what rule it out, run from a script so that
`ksh -n` can say which stage refused:

| written | ksh93 | refused |
| --- | --- | --- |
| `${(echo a)}` | `a` | — |
| `${(echo a);}` | `` `}' unexpected `` | while reading |
| `${(echo a) ;}` | `` `}' unexpected `` | while reading |
| `${(echo a); echo b;}` | `` `}' unexpected `` | while reading |
| `${(echo a)b}` | `` `b}' unexpected `` | at the run |
| `${(echo a}` | `` `(' unmatched `` | while reading |

So the `}` has to sit directly behind the matching `)` with nothing
between them, not even a blank. The blank form is the contrast and it
*is* a list: `${ (echo a); echo b;}` runs both commands in ksh93 and in
bash 5.3.

The end of the parenthesis is found by reading a list and stopping where
it stops — the same `parseToClose` the two parenthesis scanners use — so
a `)` inside quotes or in a `case` arm closes nothing, and a nested
subshell is stepped over. Grammar flag: `SubshellSubstitution`, on for
ksh alone.

A word the rule does not fit is left alone rather than refused, and that
is ksh93's own order rather than a fallback: its `-n` refuses
`${(echo a);}` while reading and passes `${(U)a}` to the run. So the
`${(flags)word}` an expansion written for zsh reaches this parser with is
still the parameter form, and still names the rest of the word —
`echo "AA${(U)a}BB"` is `` `a}BB' unexpected `` in ksh93 and here, the
blamed token running *past* the brace that would have closed a
substitution.

`${((expr))}` is **not** this construct: two adjacent parentheses are
ksh93's braced arithmetic, `${((1+2))}` is 3 there and `${((echo hi))}`
an arithmetic syntax error, where one space apart `${( (1+2) )}` is a
subshell running a command called `1+2`. That spelling is not implemented
and is #2725.

Measured: `subst/a-paren-opens-a-current-shell-body-too`,
`subst/a-paren-body-does-not-share-what-it-assigns`,
`subst/a-paren-body-is-the-parenthesis-and-not-a-list`,
`subst/a-blank-opened-body-holding-a-subshell-is-a-list` and
`subst/a-brace-body-opens-on-a-blank-or-a-paren-and-nothing-else`.

## `${| cmd;}` — the same body, valued from `$REPLY`

bash 5.3 added a fourth spelling. The body runs in the current shell
exactly as the one above does, and the substitution expands to whatever
the body left in `$REPLY` rather than to what it printed:

    f() { REPLY=zz; }
    echo "[${| f;}]"       →  [zz]

It is the one expansion that lets a function return a value without a
subshell and without the caller naming a variable, which is what every
shell library otherwise does with a global by convention.

**One shell in the panel has it**, measured 2026-09-13 over
`echo ${| REPLY=hi; }`:

| shell | answer |
| --- | --- |
| bash 5.3.15, and that binary as `sh` | `hi` |
| bash 3.2.57 | `bad substitution` |
| dash | `Bad substitution` |
| zsh 5.9.2 | `bad substitution` |
| BusyBox ash | `syntax error: bad substitution` |
| ksh93u+ | ``syntax error at line 1: `\|' unexpected`` |

So it is a dialect's own construct and there is nothing to ask the
semantics vector — the four dialects that refuse it agree with what this
shell already did. Grammar flag: `ReplySubstitution`, on for bash alone.
The ksh column is the interesting refusal: that shell *has* the blank
form and still rejects this one, which is what says the pipe belongs to
one shell rather than to the construct.

**Three things the value's source changes**, all measured on bash 5.3.15:

    y=${| echo printed; REPLY=val;}      printed goes to the shell's own
                                         output, and y is val
    REPLY=outer; y=${| true;}            y is empty, and REPLY is outer
                                         again afterwards
    v=$'a\n\n'; echo "[${| REPLY=$v;}]"  [a\n\n] — nothing is trimmed

The body's output is not captured at all; `$REPLY` is localized around
the body, with absent and empty kept apart on the way back; and the value
is a parameter's rather than captured output, so the trailing newlines
the other three spellings strip are text here.

**The lexical rule is its own, and is not the blank form's.** A `|`
*adjacent* to the brace is the marker, and a blank first is a syntax
error rather than the same construct spelled loosely:

    ${|REPLY=hi; }     hi             no blank needed after the pipe
    ${| REPLY=hi; }    hi             a blank after it is body text
    ${ | REPLY=hi; }   syntax error   `|' unexpected, looking for `}'
    ${|}               empty          at status 0

The third row is what makes these two rules rather than one reading of
"a blank **or** a pipe after the brace": with the blank first the body
has already begun, and a `|` opening it is a pipeline with nothing on its
left. A combined rule would accept the line bash refuses.

**What this shell does not give either spelling is a variable frame.**
bash gives both bodies one — `local` is legal inside `${ cmd;}` and
`${| cmd;}` there and an error at the top level — and here it is
`local: can only be used in a function` in both. The visible corner is
that `${| unset REPLY;}` reads bash's *outer* `REPLY`, because unsetting
a local there reveals what it shadows and there is nothing here to
reveal. One gap and not two, recorded rather than worked around, so a
frame fixes both spellings at once.

Measured: `subst/a-body-valued-from-what-it-left-in-reply`,
`subst/a-reply-body-does-not-capture-its-output`,
`subst/a-reply-body-starts-with-no-reply`,
`subst/a-pipe-needs-no-blank-after-it` and
`subst/a-blank-before-the-pipe-is-not-the-reply-form`.

## Process substitution: `<(cmd)` and `>(cmd)`

A command run with one end of a pipe, expanding to a **path** the other
end can be opened by. It belongs here because it is delimited exactly
like the forms above — matching parentheses, quoting tracked — and
because the only thing the lexer has to decide is where it ends.

Core: bash, ksh93 and zsh have it and dash does not, calling the `(`
unexpected. Grammar flag: `ProcessSubstitution` (on in the core, off for
`posix` and `dash`). Measured: `procsub/reads-a-command-as-a-file`,
`procsub/two-of-them-in-one-command`, `procsub/feeds-a-loop`,
`procsub/a-redirection-where-a-target-belongs`.

**The `(` must be adjacent to the `<` or `>`.** With a space the text is
a redirection followed by a subshell, and every shell in the panel
refuses it — including the three that have the construct:

    cat < (echo hi)   →  syntax error in dash, bash 5.3, bash 3.2,
                         ksh93 and zsh alike

So adjacency is not a convention this implementation adopted; it is the
grammar, and it is why the construct is the lexer's rather than the
parser's.

**It is unquoted only, and nothing is lost by that.** `"<(echo hi)"` is
its own ten characters of text in every shell in the panel, dash
included, and the reason is that what the construct produces is a
*path*: a quoted path is still a path, so there is nothing for the
quoting to change. The lexer therefore reads the form outside quotes and
nowhere else, and that is a completeness statement rather than an
omission. Pinned by `procsub/quoted-is-not-a-substitution`; the claim
said "the five characters" until the case was written, which is a count
of nothing in the example beside it and the sort of arithmetic a
sentence nobody can run keeps.

**What the path looks like is the shell's business, not the grammar's.**
Measured with `echo <(true)`: bash answers `/dev/fd/63`, ksh93
`/dev/fd/3`, zsh `/dev/fd/11`. This implementation answers `/dev/fd/N`
too — the lowest descriptor number it had free — which is the shape and
not the number, since all the spec requires is a path the child can
open. The numbers above are recorded so a reader does not mistake one
shell's for the specification. It was a path under `$TMPDIR` until
#2893; *How long the pipe has to last* below says why, and why the
reason did not hold.

**In bash and zsh the substitution is part of the word; in ksh93 it is a
word of its own.** This one has no axis behind it yet:

| probe | bash 5.3 | ksh93 | zsh |
| --- | --- | --- | --- |
| `set -- x<(true)y; echo $#` | 1 | **3** | 1 |
| the fields | `x/dev/fd/63y` | `x`, `/dev/fd/4`, `y` | `x/dev/fd/11y` |

Ours follows bash and zsh, which is the two-of-three answer and the one
the corpus's cases exercise.

The **read** side works wherever the construct does, as an argument and
as a redirection target: `cat <(echo hi)` and `cat < <(echo hi)` both
answer `hi` in bash 5.3, bash 3.2, ksh93 and zsh. The **write** side
splits by position in the panel's ksh93 (AJM 93u+ 2012):
`echo hi | tee >(cat > f)` works there as an ordinary argument, while
`echo hi > >(cat)` — the same substitution as a redirection *target* —
fails with `cannot create` naming an unprintable path. bash and zsh take
both.

**It is the sharpest evidence that a dialect is a runtime switch rather
than a build-time identity**, and the evidence is a single binary:
bash 3.2 has process substitution as `bash` and refuses it as `sh`,
which is `argv[0]` alone changing the language. Re-measured 2026-09-05
with the panel's `bash-as-sh` route: bash 3.2 as `sh` reports a syntax
error at the `(`, and bash 5.3 as `sh` keeps the construct — so the
version and the invocation are two variables and a claim naming only one
of them is incomplete.

### How long the pipe has to last

A real shell forks for `<(cmd)` and hands the child a descriptor, so the
writer exists from the moment the word is expanded. So does this one: the
word expands to `/dev/fd/N`, and *N* is one end of a pipe both of whose ends
exist before the body starts. The shell holds the end the command will open
from the moment the word expands until the end of the command that named it,
which is what keeps the pipe there for a command that has not opened the path
yet — and closing it is what delivers end-of-file to a `>(cmd)`'s body and
`EPIPE` to a `<(cmd)`'s body that is writing into a pipe nobody reads any
more.

It was a named pipe until #2893, and everything that arrangement needed is
gone with it. A FIFO's two ends have to **meet**: opening one blocks until the
other is opened, and a command handed a path is under no obligation to open
it — `echo <(true)` prints a path and is an error in no shell — so the
shell's own open was a wait that might never end, bounded by polling for a
peer and giving up when the name went away. The pipe itself existed only
while somebody held it, so a shell that opened, wrote and closed inside the
window a reader was still *inside* `open(2)` ran a whole pipe's life cycle
beside a reader attached to none of it (#2733), and a last-writer close could
be delivered to nobody (#1079). An anonymous pipe has no rendezvous, cannot
be torn down while either end is held, and needs neither the poll, nor the
placeholder end that kept it alive, nor the repeated close.

One clause of the old lifetime survives, because it is about the shell rather
than about the FIFO: a substitution's pipe **outlives the command that named
it while one of this shell's own descriptors is open on it**. `exec {fd}<
<(cmd)` and `sysopen -r -o nonblock -u fd <(cmd)` both put the pipe in the
script's hands, and the body then has to be waited for by whatever scope owns
that descriptor rather than by the command (#1750).

**And one read is not the output.** A single `read(2)` on a pipe returns what
has arrived, so a substitution whose body writes twice answers one read with
however much of it had been written by then — `[one]` where the whole of it is
`onetwo`, 4 runs in 15 on this machine. That is true of the shell being copied
as well and is not a divergence. A reader that wants the output reads **to end
of input**, and a test that reads once is measuring the scheduler (#1907).

### What reaches the command, and nothing else

`/dev/fd/N` is only openable by a process that holds *N*, so the descriptor has
to reach the command. Everything Go opens is close-on-exec, and clearing that
flag hands the descriptor to **every** command the shell runs afterwards — for
`>(cmd)` that is fatal rather than untidy, since a later command holding the
writing end open means the body never reads end-of-file. `echo x | tee >(tr
a-z A-Z); sleep 0.4` produced nothing at all for exactly that reason, and it
is why the construct was written with a FIFO in the first place.

The flag never had to be cleared. A descriptor reaches a child **by number**,
through the table the shell rebuilds for it — the same table `exec 3>out3;
cmd` already runs on — and that table is per command. So the end the command
opens is parked close-on-exec and put in the table of the shell that named the
path, and of no other: a substitution's own body is a shell of its own with an
empty list, so `tee >(cat)` cannot hand the writing end to the `cat` that is
reading the other side of it.

Two things follow that a name in a directory did not have, and both match the
shells being copied. A descriptor number is **reused**, so a path captured
from an earlier command can name a live pipe again later. And nothing is left
on the filesystem for anything to remove: `p=$(echo <(true)); [ -e "$p" ]` is
false afterwards because the number is closed, not because a file was
unlinked.

### When a writing body's output lands

`>(cmd)` is the direction whose body has nobody waiting for it. `<(cmd)`
writes into the pipe, so the command that named the path reads it to an
end; `=(cmd)` has run to completion before the path exists at all. A
writing body writes into the **shell's own output**, which nothing
downstream ends and nothing in the shell reads.

**No shell in the panel loses it.** Measured 2026-09-12 on Linux with

    printf "PIPE\n" | tee >(read -r v; printf "[%s]" "$v") >/dev/null

bash 5.2, zsh 5.9 and ksh93 all answer `[PIPE]` — 200 runs each, idle
and again with the machine oversubscribed two to one, 1200 answers and
no empty one. In a forking shell that costs nothing: the body is a
process holding the shell's standard output, so whatever is reading that
stream reads until the body has closed it too.

**When the shell stops is a disagreement.** With a body that outlives
its input, zsh waits for it and bash and ksh93 do not:

| probe | bash 5.2 | ksh93 | zsh 5.9 |
| --- | --- | --- | --- |
| `printf x \| tee >(sleep 3) >/dev/null` | 0s | 0s | **3s** |
| `echo >(sleep 3)` | 0s | 0s | **3s** |
| `echo hi > >(sleep 3)` | 0s | 0s | **3s** |
| `exec > >(cat); echo hi` | 0s | 0s | 0s |
| `…; printf AFTER` after the first row | `AFTER[PIPE]` | `AFTER[PIPE]` | **`[PIPE]AFTER`** |

**The bytes are the part the whole panel agrees on**, and they are what a
goroutine cannot promise for free: the output is the caller's `io.Writer`,
which stops being read the moment the shell is done, so the descriptor's
lifetime has to be reconstructed as a join. Before there was one this
shell answered the empty string in 49 runs in 300 of the binary under
that load, and in 4 of 60 of the test covering it (#2183).

**Where that join sits is the axis**, and it is
`Semantics.WritingSubstitutionIsWaitedForAtTheCommand`: zsh yes · bash
no · ksh93 no · dash and ash unanswered, having no `>(cmd)` at all. Yes
holds the command that named the body until the body is done — the
`[PIPE]AFTER` ordering. No lets the command finish and moves the join
out to the scope that owns the stream the body is writing into, which
delivers `AFTER[PIPE]`.

The **duration** rows above are not reproducible under either answer and
are not what the axis moves. A real shell's body is a process that
outlives the shell, so `echo >(sleep 3)` costs it nothing; here the
process cannot leave before the goroutine has written, so the join is
paid at the end of the script instead of at the command. What moves is
the **ordering**, which is the row the corpus grades.

**Which scope owns the stream** is a second measurement, and a subshell
is not it. Measured 2026-09-15 on bash 5.3.15 and ksh93:

| written | bash 5.3 | ksh93 | zsh 5.9.2 |
| --- | --- | --- | --- |
| `( printf P \| tee >(slow) >/dev/null ); printf AFTER` | `AFTER` then `[PIPE]` | the same | `[PIPE]AFTER` |
| `v=$(printf P \| tee >(slow) >/dev/null; printf IN); echo "[$v]"` | `[IN[PIPE]]` | `[IN]` | `[[PIPE]IN]` |

So a **command substitution is a boundary** — the bytes are captured into
the value, after everything the substitution's own commands wrote — and a
plain subshell and a pipeline element are not: they write into their
caller's stream and their bodies are the caller's to join. That is
`Runner.bodies`, shared with a clone and replaced only by a command
substitution.

ksh93's second cell is a third answer and is not reproduced: it drops the
body's bytes rather than capturing them, which is the one outcome the
first paragraph says no shell has.

**Except where the script is itself still holding the pipe.** `exec >
>(cat)` hands the shell's output to a body that reads until that end
closes, and the end is now the shell's: waiting there is waiting for a
descriptor only this shell can close. zsh does not wait for that shape
either — the one 0s row above among its 3s ones — so the exception is
the construct's rather than this implementation's. It is the same clause
that keeps the pipe's *name* while one of this shell's own descriptors
is open on it, above.

### Which standard input the body reads — an axis

A substitution's body is a shell of its own and has to read *something*.
Two candidates: the standard input of the command whose word the
substitution stands in, or the standard input of the **shell**. They are
the same stream almost everywhere, which is why the obvious probe cannot
see the question — `cat <(cat)` reads the shell's input in every shell
that has the construct, because the command's input *is* the shell's.

They part inside a **pipeline element**, whose input is the pipe.
Measured 2026-09-11 and re-measured across the panel, with the shell's
own standard input a file holding `OUTER`:

| probe | bash 5.3 | bash 3.2 | bash as `sh` | ksh93 | zsh 5.9.2 |
| --- | --- | --- | --- | --- | --- |
| `printf "PIPE\n" \| cat <(cat)` | `PIPE` | `PIPE` | `PIPE` | `PIPE` | **`OUTER`** |
| `printf "PIPE\n" \| cat =(cat)` | — | — | — | — | **`OUTER`** |
| `cat <(cat)` | `OUTER` | `OUTER` | `OUTER` | `OUTER` | `OUTER` |

dash has no such construct in any row. So the panel **splits**: this is
an axis and not a correction. `ProcessSubstitutionBodyReadsTheShellsInput`
is it — zsh `Yes`, bash, bash-as-`sh`, bash 3.2 and ksh93 `No`, and the
POSIX base `No` because the standard has no construct to answer for.

The third row is the control, and it is why the divergence was silent:
without a pipeline the two readings name one stream and nothing
distinguishes them. What goes wrong under the wrong answer is quiet too —
nothing errors, the body simply eats the pipe the outer command was
going to read.

**The reading behind zsh's answer bounds the axis.** A pipeline element's
pipe is one of that element's *redirections*, and a redirection is
applied after the element's words have been expanded — so a substitution
performed while expanding them is still looking at the shell's own input.
Everything that happens *after* that point sees the pipe, and zsh agrees
with the rest of the panel at every one of them:

| probe | every shell with the construct |
| --- | --- |
| `printf "PIPE\n" \| { cat <(cat); }` | `PIPE` |
| `printf "PIPE\n" \| ( cat <(cat) )` | `PIPE` |
| `f() { cat <(cat); }; printf "PIPE\n" \| f` | `PIPE` |
| `printf "PIPE\n" \| eval 'cat <(cat)'` | `PIPE` |
| `printf "PIPE\n" \| cat < <(cat)` | `PIPE` |

A compound command's body, a function's body and an `eval`'s program all
run once the element's redirections are in place; a substitution written
as a redirection **operand** is expanded with them rather than before
them. So the answer reaches one simple command's words and stops there —
a fix written as "a pipeline element's substitutions read the shell's
input" gets the last row wrong, in the accepting direction, silently.

Two more shapes, both measured and both the same split as the first row:
a substitution **nested** in another (`printf "PIPE\n" | cat <(cat
<(cat))`) answers `OUTER` in zsh, because the outer body is a shell whose
own input is whatever the axis handed it and the inner body inherits
that; and a **backgrounded** pipeline (`printf "PIPE\n" | cat <(cat) &
wait`) answers `OUTER` too, so what the body reads is the shell's real
input rather than the empty one an asynchronous job is often given.

**`>(cmd)` cannot observe it.** That spelling hands its body the reading
end of its own pipe, which replaces whatever the body would otherwise
have read, so all five answer alike. The axis is still asked in the one
place all three spellings are prepared — `substRunner` in
`interp/procsubst.go` — which is what keeps `=(cmd)` from drifting away
from `<(cmd)`. Deciding it per spelling is exactly the shape #1933 was
filed to prevent, since the file form arrived after the pipe forms and
would have been the second copy.

The corpus writes each body with the shell's own `read` rather than an
external `cat`. The two are measured to answer identically in every
column; what the external spelling adds is a process started with the
element's descriptor, which is #2144 and is older than this axis.

Measured: `procsub/a-body-in-a-pipeline-reads-the-shells-input`,
`procsub/a-file-substitutions-body-in-a-pipeline`,
`procsub/a-body-outside-a-pipeline-reads-the-same-input`,
`procsub/a-body-in-a-grouped-pipeline-element`,
`procsub/a-body-in-a-function-called-from-a-pipeline`,
`procsub/a-body-in-a-redirection-operand-of-a-pipeline-element`,
`procsub/a-nested-body-in-a-pipeline`,
`procsub/a-body-in-a-backgrounded-pipeline`,
`procsub/a-writing-body-in-a-pipeline-reads-its-own-pipe`.

## Process substitution to a file: `=(cmd)`

The same construct with a **regular file** where the two spellings above
have a pipe. The command runs to completion, its output lands in the
file, and the word becomes that file's path.

Grammar flag: `ProcessSubstitutionToFile`, off in the core and on for
`zsh` alone. Measured 2026-09-11 on zsh 5.9.2 against the whole panel —
dash, bash 5.3, that binary as `sh`, bash 3.2 and ksh93 all read the `=`
as an ordinary character and report the `(` — so this is additive
grammar for one dialect rather than a disagreement, and there is no
semantics axis behind it. Measured:
`procsub/a-file-rather-than-a-pipe`,
`procsub/a-file-substitution-outruns-a-pipe-buffer`,
`procsub/a-file-substitution-discards-the-bodys-status`,
`procsub/a-file-substitution-lives-as-long-as-its-command`,
`procsub/a-file-substitution-opens-only-at-a-word`,
`procsub/a-file-substitution-in-a-condition`,
`procsub/a-quoted-file-substitution-is-text`.

**A file is what the construct is for.** `diff =(sort a) =(sort b)`
seeks in both operands and an editor opens one; neither is something a
pipe can do. It follows that the body cannot be left running beside the
reader: there is nobody to fill the buffer for and nothing to deadlock
against, so it runs to completion first. `wc -c < =(head -c 200000
/dev/zero)` answers `200000`, which is well past any pipe buffer and is
the probe that separates the two readings — a shell that handed over a
pipe here would stop rather than answer wrong.

**Where it opens is most of the grammar, and it is narrow.** Unlike
`<(` and `>(`, which open a substitution anywhere in an unquoted word,
`=(` opens one in exactly two positions:

| probe | zsh 5.9.2 |
| --- | --- |
| `echo =(echo hi)` | a path |
| `echo =(echo hi)x` | that path with `x` behind it |
| `a=(=(echo hi))` | a path, the front of an array element being the front of a word |
| `a==(echo hi)` | a path, the front of an assignment's **value** |
| `a[1]==(echo hi)`, `a+==(echo hi)`, `typeset a==(echo hi)` | a path |
| `echo x=(echo hi)` | `missing end of string` |
| `echo a=b=(echo hi)` | `missing end of string` |
| `echo a==(echo hi)` | `missing end of string` |
| `echo =(echo hi)=(echo ho)` | `missing end of string` |
| `cat > a==(echo hi)` | `missing end of string` |
| `case a=(b) in a*)` | matches: the subject is the literal `a=(b)` |
| `echo \=(echo hi)`, `"=(echo hi)"`, `'=(echo hi)'` | the text as written |

The last four rows are what a rule of "an `=` in front of a `(`" gets
wrong, and the reason the position has to be asked about rather than the
characters: an `=` is an ordinary character everywhere else in a word,
and a `(` behind one already means an array literal or a pattern group.
Taking every `=(` for a substitution is the failure #1288 recorded,
where a pattern with an `=` in the middle of it stopped a real prompt
theme from parsing.

The two accepting positions are the front of a **word** and the front of
an assignment's **value**, and the second is only where an assignment may
be written at all: the same word is refused as an argument, as a
redirection's target and as a `case` subject.

**Its lifetime is the command that named it**, exactly as a pipe's is:
`f==(echo hi); cat $f` is `No such file or directory`. So the file is
removed by the same step that removes the pipes, at the end of the
command, and the path a script captured out of one leads nowhere
afterwards.

**The body's status is discarded.** `cat =(false)` and `cat =(exit 7)`
are both 0, and `echo =(nosuchcmd)` prints the body's diagnostic, prints
a path, and still exits 0. A word expands to a path or it fails to
expand; a command that ran and failed still wrote the file it was given.
`=()` and `=(:)` each give a valid file of zero bytes.

**What the file looks like** is the shell's business the way the pipe's
path is. zsh writes it under `$TMPPREFIX` — default `/tmp/zsh` — and
ignores `TMPDIR`, and the mode is `0600`, which is also why naming one
as a command is `permission denied` at status 126 rather than running
it. This implementation keeps the mode and puts the file in the same
per-shell directory as its pipes, for the reason recorded under the
pipes: a path chosen by the interpreter belongs to the Runner rather
than to the process, and there is no panel behavior to match because no
other shell has the construct.

**In a condition it is refused, with the same sentence as the other two
and a different status.** `[[ x == =(x) ]]` is
`process substitution =(x) cannot be used here` at status **1**, where
`[[ x == <(x) ]]` is the identical sentence at status **2**. Both
abandon the rest of the input. The status is the only thing that tells
them apart, and it is a fact about the spelling rather than about the
shell — no second shell has the file form to disagree about it.

## `$(<file)` — the substitution that reads a file

A command substitution whose **whole body is one input redirection and
nothing else** is a special form: the file is opened, its bytes become
the substitution's result, and no command runs.

    printf 'hello\n' > f
    printf "[%s]" "$(<f)"    →  [hello]  in zsh 5.9.2, bash 5.3,
                                         bash 3.2, bash as `sh`,
                                         bash --posix and ksh93
                             →  []       in dash

Measured 2026-09-10 across the whole panel. Grammar flag:
`ReadFileSubstitution`, on in `Core()` and therefore in bash, ksh and zsh;
off for POSIX and dash.

**It is additive rather than a disagreement**, and that is the reason it
is a grammar flag rather than a semantics axis. dash reads the same text
as an ordinary redirection with no command name: the file is opened, so a
name that will not open is still reported, and nothing runs, so nothing is
written and the substitution is empty. The shell without the form does not
mean something *else* by the text — it has no form to mean anything by.

### It is not the null-command hook

zsh runs a command consisting only of redirections through `NULLCMD` and
`READNULLCMD`, so `<f` at a prompt pages the file. It would be easy to
conclude that `$(<f)` is that mechanism seen through a substitution, and
it is not. Measured 2026-09-10 on zsh 5.9.2, with `READNULLCMD` set to a
function that prints `CHANGED`:

    <f                  →  CHANGED     the hook
    $(<f)               →  hello       the form
    $(:; <f)            →  CHANGED     the hook again
    $(<f; :)            →  CHANGED
    cat < a*b           →  the file    a target is matched as a pattern
    $(<a*b)             →  no such file or directory: a*b

The last pair is the same conclusion from the other side: zsh matches an
ordinary redirection target as a pattern and does not match this one, so
the two operands travel different paths. A hook the form does not consult,
and an expansion the form does not do, is a different mechanism.

The distinction is load-bearing rather than trivia. Implementing the form
as "a command with no name copies its input to its output" would make `<f`
print the file in bash and ksh93, where measurably it prints nothing, and
would make `$(<f; :)` print it everywhere, where measurably only zsh does.

### What is the form and what is not

Unanimous among the five columns that have it, measured 2026-09-10:

| body | result | why |
| --- | --- | --- |
| `$(<f)` | the file | the form |
| `$(< f)` | the file | a space before the operand changes nothing |
| `` `<f` `` | the file | the older spelling of the same substitution |
| `$(0<f)` | the file | standard input written out is still standard input |
| `$(<$name)`, `$(<~/x)` | the file | the operand expands as a redirection target does |
| `$(<f echo hi)` | `hi` | a command word takes the file as its input |
| `$(x=1 <f)` | empty | an assignment prefix leaves an ordinary redirection |
| `$(<f <g)` | empty | one redirection, not the first of several |
| `$(3<f)` | empty | a descriptor other than standard input (bash 3.2 dissents) |
| `$(<<<hi)` | empty | a here-string body is not a filename |
| `$(<f; :)`, `$(:; <f)` | empty | not the whole body — zsh's hook answers these |

The result is trimmed of trailing newlines exactly as any other command
substitution's is, and unquoted it is field-split the same way. A name
that will not open is reported in the dialect's own words, the
substitution expands to nothing, and its status is the one that dialect
gives any redirection that would not open — 1 in zsh, bash and ksh93, 2 in
dash. Under `set -e` that status ends the script.

### What this entry does not settle

Three corners where the panel does not agree, each left for a measurement
of its own rather than folded into this form:

- **A directory operand.** zsh 5.9.2 says `error when reading …: is a
  directory` and leaves status 1; bash 5.3, ksh93 and dash say nothing at
  status 0, and bash 3.2 says nothing at status 1. Three answers, so an
  axis rather than a rule.
- **A pattern in the operand.** bash matches it — `$(<a*b)` reads `aXXb` —
  where zsh, ksh93 and dash report the pattern as the name. That is the
  ordinary redirection-target question asked in this position.
- **zsh's `NULLCMD` / `READNULLCMD`.** The hook itself, which is a
  mechanism of its own and is now implemented as one — see
  "A command that is only redirections" in `docs/spec/semantics.md`. The
  boundary this section draws is what it is built against: the form
  intercepts the body before any command runs, so a substitution never
  reaches the hook and pointing `READNULLCMD` somewhere else still reads
  the file.

## When a `$( … )` body is parsed — four columns against three

The text between `$(` and `)` is kept as source and parsed a second time
when the substitution runs, which is what `syntax.Span` says of it. Four
of the panel do not wait that long: they read the body with the line that
holds it, before anything on that line has run.

Measured 2026-09-15, `env -i PATH=/usr/bin:/bin`, over `-c`, with a body
that is not a program:

    echo before; v=$(if); echo after

| shell | `before` | when the refusal arrives |
| --- | --- | --- |
| dash | no | with the line |
| bash 5.3 | no | with the line |
| bash 5.3 as `sh` | no | with the line |
| BusyBox ash | no | with the line |
| **bash 3.2** | **yes** | when the substitution runs |
| ksh93 | yes | when it runs |
| zsh 5.9.2 | yes | when it runs |

So the split runs *through bash*: 3.2 reads the body lazily and 5.3 reads
it with the line. That is worth stating plainly, because #2357 records
this as dash alone against ksh93 and zsh — an artifact of the instrument
it used, which was an alias defined and used on the same line. An alias
only reaches the question where the shell expands aliases at all, and
bash does not in a non-interactive shell, so bash's column read as
agreement when it is the opposite.

The second probe is what makes the first one mean the parse moment rather
than how far a failure reaches:

    false && v=$(if); echo "after=$?"

The substitution is never reached. The three lazy columns print `after=1`
with nothing said; the four eager ones refuse before `false` has run.

**This implementation is on the lazy side in every dialect**, so `bash`,
`dash` and `ash` are off here. Both rows are in the corpus —
`subst/a-body-that-will-not-parse-stops-the-line` and
`subst/a-body-that-will-not-parse-in-a-branch-never-taken` — so a change
is graded rather than argued.

**What taking the eager side needs**, and why it is not a semantics axis
alone: the body would have to be parsed where the line is, with the alias
table and the dialect *as they stood then*, and the result kept — which
is a field on the span beside `Span.Arith` and `Span.Param`, filled by
the parser, rather than a flag the interpreter reads. Validating the body
at the top of each line without keeping the tree would answer the two
rows above and still get the alias row wrong, which is one rule with two
implementations and the shape this tree has been bitten by before.

## Where a refused body says it happened, and what it was looking for

A `$( … )` body that will not parse is refused at run time here, and one
dialect writes two facts into that refusal that the rest do not.

### The route the text arrived by

Measured 2026-09-17, `env -i PATH=/usr/bin:/bin LC_ALL=C bash s.sh` with
stdin from `/dev/null`, on bash 5.3.20. Each row holds
`v=$(echo hi; for)` somewhere, and both lines of the refusal carry the
same prefix:

| where the substitution is written | location |
| --- | --- |
| on the script's own line | `s.sh: line 2:` |
| inside `eval '…'` | `s.sh: eval: line 2:` |
| in an EXIT trap's body | `s.sh: exit trap: line 1:` |
| in an ERR trap's body | `s.sh: error trap: line 2:` |
| in a DEBUG trap's body | `s.sh: debug trap: line 2:` |
| in a RETURN trap's body | `s.sh: return trap: line 2:` |
| in an INT trap's body | `s.sh: interrupt trap: line 1:` |
| in any other signal's trap body | `s.sh: trap: line 1:` |
| under `-c` | `bash: -c: line 1:` |
| as `$( … )` inside a backquoted body | `s.sh: command substitution: line 1:` |
| in a here-document body | `s.sh: command substitution: line 2:` |
| inside a file `.` read | `./inner.sh: line 2:` |
| as `$( … )` inside `$( … )` | `s.sh: line 1:` |

The last two rows are the controls, and they are what make this a
question about the **route** rather than about depth. A sourced file
replaces the shell's own name and names no route beside it. A
substitution inside a substitution names none either, because that shell
reads both bodies while it is reading the script's line — and the two
rows that *are* named `command substitution` are the two it reads at
expansion time instead.

A **run-time** diagnostic from the same places is located plainly: `eval
'nosuchcmd'` is `s.sh: line 1: nosuchcmd: command not found`, with no
`eval:`. So this belongs to the refusal and not to the frame.

`INT` is the one signal with a word of its own; `TERM`, `HUP`, `QUIT`,
`USR1`, `USR2` and `ALRM` all fall back to the builtin's name. That is
measured rather than assumed from the four pseudo-conditions each having
one.

A here-document body is lexed again from its own text, so everything in
it is numbered from the body's first line. The refusal is located in the
**file** — `line 4` for a body the here-document holds on line 4, and
`line 4` again for the second line of a two-line body — while a command
that is not found inside the same substitution is reported at the line
the *redirection* is on. The two are different numbers in that shell and
are read from different places here.

The text the here-document body's refusal quotes back is a line of the
substitution rather than of the script:

    cat <<E
    before $(echo hi; for) after
    E

is `` `echo hi; for) after' `` — from just past the opener, because that
is where the text being read began — and a body whose substitution runs
onto a second line quotes that line whole, `` `for) y' ``. The older
spelling does not take that cut: a `$( … )` refused inside a backquoted
body is quoted `` `echo $(for)' ``, the enclosing line entire.

### The closer it was still looking for

A token the grammar did not want *inside* a `$( … )` body names the
parenthesis that would have closed it. Measured the same way:

| body | sentence |
| --- | --- |
| `v=$(echo hi; ;)` | ``syntax error near unexpected token `;' while looking for matching `)'`` |
| `v=$(for z in 1 2 3; done)` | ``… `done' while looking for matching `)'`` |
| `v=$(} )` | ``… `}' while looking for matching `)'`` |
| `v=$(esac)` | ``… `esac' while looking for matching `)'`` |
| `v=$(echo hi; for)` | ``syntax error near unexpected token `)'`` |
| `` v=`echo hi; ;` `` | ``syntax error near unexpected token `;'`` |

Two things it is not. It is not every refusal: where the unexpected token
*is* the closer, nothing is added, because that is the very thing the
shell was looking for and it found it. And it is not both spellings: the
older one is refused with no such clause, which is the same split the
construct tag turns on one message over.

The closer named is the construct's own. `v=${ echo hi; ;}` is
``syntax error near unexpected token `;' while looking for matching `}'``
in the same shell.

## Where a body's lines are counted from

A `$( … )` body is parsed on its own, so something has to say which file
line its first line is. Two of the three answers in the panel are already
written down — a `$( … )` is numbered from the file everywhere, and the
older spelling restarts from one in dash — and the third is a `$(` that
ends the line.

Measured 2026-09-17 over a script file with `env -i PATH=/usr/bin:/bin
LC_ALL=C` and stdin from `/dev/null`, `echo one` on line one and the
substitution written from line two:

| body | physical line of the command | bash 5.3.20 | zsh, ksh93, dash |
| --- | --- | --- | --- |
| `$(⏎echo "L=$LINENO"⏎)` | 3 | **2** | 3 |
| `$(⏎⏎echo "L=$LINENO"⏎)` | 4 | **2** | 4 |
| `$(⏎⏎⏎⏎echo "L=$LINENO"⏎)` | 6 | **2** | 6 |
| `$(⏎echo x⏎echo "L=$LINENO"⏎)` | 4 | **3** | 4 |
| `$(echo "L=$LINENO"⏎)` | 2 | 2 | 2 |

The last row is the control and is why this is about the **opener**: with
text after the `$(`, every column answers 2. The third row is what says it
is not a constant offset of one — however many newlines stand between the
opener and the first command, that command is the opener's line there.

The line *after* the substitution is 5 in every column, so nothing is
shifted for the rest of the file; the offset lives inside the body. The
same number shows in a **diagnostic** as well as in `$LINENO`, because
both are read off the one offset the body's runner is given.

Diagnostics value: `SubstitutionBodyStartsAtItsOpenersLine` — bash alone.
It sits beside `BackquotedSubstitutionRestartsLines` because the two are
one question about where a body's lines are counted from, asked of the two
spellings. The older spelling is a third answer again in that shell and is
neither field's; it is measured in #3553.

## What this does not cover

The internal grammar of each form: arithmetic operators and their
precedence, the parameter-expansion operator set, and what a
`${x/pat/rep}` pattern means. Those are needed by expansion, not by
tokenization, and each is its own document. Recorded here so their
absence is a known gap rather than an oversight.
