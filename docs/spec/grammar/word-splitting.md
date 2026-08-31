# Field splitting

Stage 6 of the pipeline in `expansion.md`. It turns one expansion result
into zero or more fields, using `IFS`.

Citation: POSIX.1-2024 XCU §2.6.5. Panel measurements as in
`../oracle.md`; probes are given so each row can be re-run.

## What is subject to splitting

Only the **unquoted** results of parameter expansion, command
substitution and arithmetic expansion. Never literal text, and never a
quoted expansion.

    echo a b        two fields, because the source has two words —
                    not because anything was split
    x='a b'
    echo $x         one expansion, split into two fields
    echo "$x"       one expansion, not split: one field

This distinction is the entire feature. A shell that splits literal text
would be unable to pass an argument containing a space.

## IFS

`IFS` names the delimiters. Three states, and they are genuinely
different rather than degrees of the same thing:

| state | behavior | all four agree |
| --- | --- | --- |
| unset | default: space, tab, newline | yes |
| set and non-empty | those characters delimit | yes |
| set and **empty** | **no splitting at all** | yes |

    unset IFS; x='a b'; set -- $x    → [a][b]
    IFS=;     x='a b'; set -- $x    → [a b]

An empty `IFS` is not "split on nothing"; it disables the stage. This is
the standard idiom for reading a line whole.

## Whitespace and non-whitespace delimiters differ

The rule that produces almost all confusion. A character in `IFS` is
**IFS whitespace** if it is space, tab or newline; otherwise it is **IFS
non-whitespace**, and the two are treated differently:

- A run of IFS whitespace is **one** delimiter, and leading and trailing
  runs are discarded entirely.
- **Each** IFS non-whitespace character is a delimiter in its own right,
  so two adjacent ones produce an empty field between them.
- An IFS non-whitespace character may be surrounded by IFS whitespace,
  and the whole group is still one delimiter.

Measured, with `dash`, `bash` and `ksh93` agreeing on every row (zsh does
not split parameter expansions at all — see below):

| probe | fields |
| --- | --- |
| `x='a b   c'; set -- $x` | `[a][b][c]` |
| `x='  a  b  '; set -- $x` | `[a][b]` |
| `IFS=:; x='a:b:c'; set -- $x` | `[a][b][c]` |
| `IFS=:; x='a::b'; set -- $x` | `[a][][b]` |
| `IFS=:; x=':a'; set -- $x` | `[][a]` |
| `IFS=:; x='a:'; set -- $x` | `[a]` |
| `IFS=:; x='::'; set -- $x` | 2 fields |
| `IFS=' :'; x='a : b'; set -- $x` | `[a][b]` |
| `IFS=' :'; x='a::b'; set -- $x` | `[a][][b]` |

### The leading/trailing asymmetry

Two rows above are the same construct at opposite ends of the string and
give different answers:

    IFS=:; x=':a'   → [][a]     leading delimiter makes an empty field
    IFS=:; x='a:'   → [a]       trailing delimiter does not

A trailing delimiter is absorbed; a leading one is not. This is not an
implementation quirk — all three splitting shells do it, and POSIX
specifies it — but it is the single most common source of off-by-one
field counts, and any implementation that treats splitting as a
symmetric "split on delimiter" produces `[a][]` for the second row and is
wrong.

## Empty and unset values produce no fields

    x=''; set -- $x     → 0 fields
    x=''; set -- "$x"   → 1 field, empty
    unset u; set -- $u  → 0 fields

An unquoted expansion of an empty value vanishes. Quoting it produces one
empty field. All four shells agree.

## `"$@"` and `"$*"`

Special parameters, and the only place where quoting produces *more* than
one field. All four shells agree on every row here.

| probe | result |
| --- | --- |
| `set -- a b; printf '[%s]' "$@"` | `[a][b]` |
| `set -- a b; printf '[%s]' "$*"` | `[a b]` |
| `set -- a b; IFS=:; printf '[%s]' "$*"` | `[a:b]` |
| `set -- 'a b' c; printf '[%s]' "$@"` | `[a b][c]` |
| `set --; set -- "$@"; echo $#` | `0` |
| `set --; set -- "$*"; echo $#` | `1` |

Three facts an implementation must encode separately:

- `"$@"` expands to **one field per positional parameter**, each retaining
  its own spaces. It is not a string.
- `"$*"` joins with the **first character of IFS**, not with a space. The
  third row is the proof; with `IFS` unset the joiner is a space.
- With no positional parameters, `"$@"` yields **zero** fields while
  `"$*"` yields **one empty** field. This is why `set -- "$@"` is safe on
  an empty list and `set -- "$*"` is not.

## Splitting applies to command substitution too

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `x='a b'; set -- $x; echo $#` | 2 | 2 | 2 | **1** |
| `set -- $(printf 'a b'); echo $#` | 2 | 2 | 2 | **2** |
| `set -- $(echo a b); echo $#` | 2 | 2 | 2 | **2** |
| `x='a b'; set -- ${x}; echo $#` | 2 | 2 | 2 | **1** |

**zsh's no-split rule covers parameter expansion only.** It splits an
unquoted command substitution exactly like the other three.

This was measured rather than assumed, and the assumption would have been
wrong: "zsh does not word-split" is the usual summary and it is too
broad. The axis is narrower, and an implementation that models it as one
switch over all of stage 6 produces the wrong answer for
`$(...)` in zsh mode.

zsh spells the opt-in for parameter expansions `${=x}`, which is a syntax
error in the other three:

    zsh:  x='a b'; set -- ${=x}; echo $#   → 2

## Vector fields

| field | default | zsh |
| --- | --- | --- |
| `SplitParamExpansion` | `true` | `false` |
| `SplitCommandSubstitution` | `true` | `true` |

Two fields rather than one, because the panel shows the two moving
independently. Naming them for the behavior rather than for zsh is what
`../semantics.md` requires, and here it is also what keeps the second
field from being silently wrong.
