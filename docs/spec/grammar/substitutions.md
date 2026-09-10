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

## `$((` is arithmetic, and a nested subshell needs a space

    $((1+2))        →  3       arithmetic
    $( (echo sub) ) →  sub     a subshell inside a substitution

Unanimous. `$((` is taken as the start of arithmetic, so a command
substitution whose first construct is a subshell must be written with a
space. That is the only disambiguation available and it belongs to the
lexer, since by the time the parser sees tokens the choice has been made.

## Backticks

    `echo hi`               →  hi
    `echo \`echo deep\``    →  deep

The older form. It nests only with backslash escaping, which is why the
`$( )` form exists and why this one is worth supporting but never
recommending. Its delimiter is a backtick that is not backslash-escaped.

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

The body is delimited exactly as the parameter form is: a `}` inside
quotes does not close either, and both nest. Grammar flag:
`CurrentShellSubstitution`, on for bash and ksh, off elsewhere including
the core.

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
`/dev/fd/3`, zsh `/dev/fd/11`. This implementation answers with a named
pipe in a temporary directory of its own instead, because `/dev/fd`
requires the descriptor to survive `exec` and clearing Go's
close-on-exec flag leaks it into every later command. All the spec
requires is a path the child can open; the numbers above are recorded so
a reader does not mistake one shell's for the specification.

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
- **zsh's `NULLCMD` / `READNULLCMD`.** The hook itself, which this
  implementation does not have: `<f` writes nothing here where zsh writes
  the file.

## What this does not cover

The internal grammar of each form: arithmetic operators and their
precedence, the parameter-expansion operator set, and what a
`${x/pat/rep}` pattern means. Those are needed by expansion, not by
tokenization, and each is its own document. Recorded here so their
absence is a known gap rather than an oversight.
