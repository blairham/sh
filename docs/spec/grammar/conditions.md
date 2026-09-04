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

Vector field: `TerminalTestRequiresANumber` (bash and dash yes, ksh93
and zsh no), asked only for such an operand.

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

Vector field: `MissingFileIsOlder` (bash and ksh93 yes, dash and zsh
no). One axis for `test`, `[` and `[[ ]]` alike, because every shell
answers its two constructs the same way.

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
file and exist in some of the panel only. The parser refuses them along
with everything else it does not list, which is the rule: an operator is
either implemented or refused at parse, never parsed and then refused at
run time. Recorded so their absence is a decision.
