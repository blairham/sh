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
the five characters in every shell in the panel, dash included, and the
reason is that what the construct produces is a *path*: a quoted path is
still a path, so there is nothing for the quoting to change. The lexer
therefore reads the form outside quotes and nowhere else, and that is a
completeness statement rather than an omission.

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

## What this does not cover

The internal grammar of each form: arithmetic operators and their
precedence, the parameter-expansion operator set, and what a
`${x/pat/rep}` pattern means. Those are needed by expansion, not by
tokenization, and each is its own document. Recorded here so their
absence is a known gap rather than an oversight.
