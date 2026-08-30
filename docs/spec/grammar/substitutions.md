# Substitutions

`$(…)`, `` `…` ``, `$((…))` and `${…}` — specifically **where they end**,
which is what the lexer needs to know. What is *inside* them is a later
document: this one is about delimiting, because a word's boundaries
depend on it and nothing else can be tokenized until they are right.

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

`${x:-y}` and its relatives already lex as one word today, because braces
are not word delimiters and nothing inside them is either. That is
accidental rather than correct: a `}` inside a quoted section of the
expansion would end it early.

The delimiting rule is the same as for `$( )` — track quoting, stop at
the matching close — and the **operators inside** (`:-`, `#`, `%`, `/`,
`:offset:length`, `[index]`) are a separate specification this document
does not attempt. The lexer only has to find the end.

## What this does not cover

The internal grammar of each form: arithmetic operators and their
precedence, the parameter-expansion operator set, and what a
`${x/pat/rep}` pattern means. Those are needed by expansion, not by
tokenization, and each is its own document. Recorded here so their
absence is a known gap rather than an oversight.
