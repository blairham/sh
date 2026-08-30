# Conditions: `[[ … ]]`

The last construct the parser was treating as an ordinary command.
`commands.md` says where it sits in the grammar and why the *parser*
rather than the lexer reinterprets `<` and `>` inside it; this says what
the operators mean.

Absent from dash, where `[[ a < b ]]` is a command named `[[` with a
redirection that opens the file `b` — see `commands.md`. Everything below
is measured on bash, ksh93 and zsh.

## It is a compound command, not a builtin

That single fact explains most of the behaviour, and it is why `[[ … ]]`
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
there, applied in a second place — so the axis is one behaviour, not two.

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

Vector field: `RegexQuotingMakesLiteral` (default true, matching bash).

## Unary and logical operators

    -n s   -z s                     non-empty, empty
    -e f   -f f   -d f   -r f
    -w f   -x f   -s f              file tests
    !                               negation
    &&     ||                       conjunction, disjunction
    ( … )                           grouping

`&&` and `||` inside `[[ ]]` join *conditions*, not commands, and `( )`
groups conditions rather than starting a subshell. That is another
consequence of it being parsed rather than executed, and it is why the
parser needs its own production for the inside rather than reusing the
one for lists.

## What this does not cover

`-v`, `-o`, and the file-comparison operators `-nt`, `-ot`, `-ef`, which
exist in some of the panel and are not needed by anything here yet.
Recorded so their absence is a decision.
