# Parameter expansion forms

What the operators inside `${ … }` mean. `substitutions.md` says where the
braces end; this says what is between them.

Citation: POSIX.1-2024 XCU §2.6.2. Panel measurements as in `../oracle.md`.

## One rule explains the four conditionals

    ${x:-word}   ${x:=word}   ${x:?word}   ${x:+word}
    ${x-word}    ${x=word}    ${x?word}    ${x+word}

**The colon extends the test from "unset" to "unset or empty."** That is
the whole difference between the two rows, and it is unanimous:

| probe | unset | empty | set to `S` |
| --- | --- | --- | --- |
| `${x:-D}` | `D` | `D` | `S` |
| `${x-D}` | `D` | *(empty)* | `S` |
| `${x:+A}` | *(empty)* | *(empty)* | `A` |
| `${x+A}` | *(empty)* | `A` | `A` |

The four operators then differ only in what they do when the test fires:

| operator | when the test fires |
| --- | --- |
| `-` | substitute the word |
| `=` | assign the word to the variable, then substitute it |
| `?` | write the word as an error and exit |
| `+` | substitute the word when the test does **not** fire |

`${u:=V}` leaves `u` set to `V` afterwards, so it is an expansion with a
side effect — the only one in this document.

## The word is itself a word

It is expanded, not taken literally:

    unset u; d=DEF
    ${u:-$d}            →  DEF
    ${u:-$(echo sub)}   →  sub
    ${u:-a b}           →  a b

So the AST cannot store it as a string. It is a nested word, which is why
`substitutions.md` keeps the inner text raw rather than flattening it:
whatever parses `${ }` has to parse a word inside it.

## Pattern removal

    ${x#pat}   shortest matching prefix removed
    ${x##pat}  longest matching prefix removed
    ${x%pat}   shortest matching suffix removed
    ${x%%pat}  longest matching suffix removed

Measured with `p=a.b.c`, unanimous:

| probe | result |
| --- | --- |
| `${p#*.}` | `b.c` |
| `${p##*.}` | `c` |
| `${p%.*}` | `a.b` |
| `${p%%.*}` | `a` |
| `${p#x}` | `a.b.c` — a pattern that does not match removes nothing |

The patterns are **glob patterns, not regular expressions**: `${p#[ab]}`
and `${p#?}` both yield `bc`. Doubling the operator is what selects the
longer match; there is no greediness syntax inside the pattern.

## Length, and where it diverges

`${#x}` is the length of the value — `x=abcd` gives 4, unanimously.

`${#@}` and `${#*}` are not unanimous:

| probe, after `set -- p q r` | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `${#@}` | **5** | 3 | 3 | 3 |
| `${#*}` | **5** | 3 | 3 | 3 |

dash gives the length of the joined string `p q r`; the others give the
number of positional parameters. This is the first axis measured where
dash stands alone, and it is a silent one: both answers are plausible
numbers and neither errors.

Vector field: `LengthOfSpecialIsCount` (default true).

## Extensions

None of these are POSIX, and they do not all arrive together.

| form | meaning | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `${x/pat/rep}` | replace first match | **no** | yes | yes | yes |
| `${x//pat/rep}` | replace every match | **no** | yes | yes | yes |
| `${x/#pat/rep}` | replace an anchored prefix | **no** | yes | yes | yes |
| `${x/%pat/rep}` | replace an anchored suffix | **no** | yes | yes | yes |
| `${x:off:len}` | substring | **no** | yes | yes | yes |
| `${x^^}` `${x,,}` | upper- and lower-case | **no** | **yes** | **no** | **no** |
| `${!x}` | indirection | **no** | **yes** | *other* | **no** |
| `${a[i]}` | array element | **no** | yes | yes | yes |

Two rows deserve their own note.

**`${x^^}` is bash alone.** ksh93 reports a syntax error and zsh a bad
substitution. `shell-matrix.md` already recorded it as bash-4-only; this
places it in the bash dialect rather than the core.

**`${!x}` diverges four ways.** With `x=y` and `y=V`, bash yields `V`,
dash and zsh reject it, and **ksh93 yields `x`** — it is not an error
there, it means something else. That is the `&>` failure mode inside an
expansion, and the third measured instance of it.

`${a[i]}` inherits the array-base axis from `semantics.md`: with
`a=(p q r)`, `${a[1]}` is `q` in bash and ksh93 and `p` in zsh.

## Dialect flags

    ParamSubstitution   ${x/pat/rep} and its anchored forms
    ParamSubstring      ${x:off:len}
    ParamCaseChange     ${x^^} ${x,,}     — bash only
    ParamIndirection    ${!x}             — bash only, and not an error elsewhere

All false for `posix`. `ParamCaseChange` and `ParamIndirection` are false
for `core`, because a construct one shell in the panel supports is not a
common denominator.

## What this does not cover

The pattern-matching language itself — what `*`, `?`, `[…]` and the
bracket-expression classes match — is shared with pathname expansion and
with `case`, so it belongs in its own document rather than being described
three times.
