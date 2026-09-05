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

## Six contexts where an unquoted expansion is exempt

The rule above is about the *expansion*. Whether the stage runs at all is
also about the **position the word is in**, and six positions never split
whatever they hold. This is unanimous — every shell in the panel that has
the construct answers the same way — which makes it different in kind
from the axes at the foot of this file.

| context | probe with `IFS=:` and `x='a:b'` | result |
| --- | --- | --- |
| an assignment's value | `y=$x` | `$y` is `a:b`, one value |
| a `[[ ]]` operand | `[[ $x = "a:b" ]]` | true |
| a `case` subject | `case $x in "a:b") …` | the arm is taken |
| a here-string | `cat <<< $x` | the line is `a:b` — *but see below* |
| a here-document body | `$x` in an unquoted-delimiter body | the line is `a:b` |
| arithmetic text | `n='1+2'; IFS=+; echo $(( $n ))` | `3`, not a syntax error |

Measured 2026-09-05 against bash 5.3.15, bash 3.2.57, bash-as-sh, dash,
ksh93u+ 2012-08-01 and zsh 5.9.2; corpus rows
`expand/an-assignment-value-is-not-split`,
`expand/an-assignment-value-from-a-command-is-not-split`,
`cond/no-field-splitting-inside`, `pat/case-subject-is-not-split`,
`expand/a-here-string-is-not-split`,
`expand/a-heredoc-body-is-not-split`,
`expand/arithmetic-text-is-not-split`. dash abstains on two rows for lack
of the construct — it has neither `[[ ]]` nor `<<<` — and agrees on the
other four.

**One exception, and it is dated rather than disputed: bash 3.2 splits a
here-string.** `x='a:b:c'; IFS=:; cat <<< $x` writes `a b c` there — the
word is split into three fields and rejoined on a space — where bash 5.3,
ksh93 and zsh all write `a:b:c`. Quoting the word (`<<< "$x"`) gives
`a:b:c` in bash 3.2 too, so the split is the whole of the difference.
By the rule in `../core.md`, bash 3.2 dates a construct rather than
vetoing it, so the exemption stands and the row records when it arrived.

**Unanimity is what makes these exemptions and not axes**, and it is
load-bearing rather than an observation. The splitting axis
`SplitParamExpansion` is deliberately *unanswered* in the core, and an
unanswered axis refuses rather than guessing (`../semantics.md`). If
these positions consulted it, the bare core would refuse `x=$two` and
`[[ $two = "a b" ]]` — questions no shell in the panel answers
differently. So the exemption is expressed as a property of the context,
which asks nobody, and the axis stays for the places the panel genuinely
disagrees.

### The redirection target is a seventh position, on its own axis

    x='a b'; echo hi > $x

bash reads the target as an ordinary word, finds two fields, and refuses:
`$x: ambiguous redirect`, status 1. dash, ksh93 and zsh take the unsplit
text and create a file named `a b`, status 0. So this position is a
disagreement, not an exemption, and it gets an axis of its own —
`RedirectTargetIsAnOrdinaryWord`, true in bash alone — rather than
joining the six above.

It also needs *both* readings of the same word, and they cannot be
derived from one another: splitting is quoting-aware, so `"$e"` with a
space is one field where `$e` is two — the unsplit text is not the fields
joined, and the fields are not the text split. Nor may the word be
expanded twice, because `> $(f)` would run `f` twice. One pass produces
both views and the axis picks between them, which is why the fields view
splits unconditionally instead of consulting the splitting axis: the
question there is which *reading* applies, and it is asked exactly where
the two readings differ.

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

## Semantics axes

| axis | dash | bash | ksh93 | zsh | core |
| --- | --- | --- | --- | --- | --- |
| `SplitParamExpansion` | yes | yes | yes | **no** | unanswered |
| `SplitCommandSubstitution` | yes | yes | yes | yes | **yes** |

Two axes rather than one, because the panel shows the two moving
independently. Naming them for the behavior rather than for zsh is what
`../semantics.md` requires, and here it is also what keeps the second
axis from being silently wrong.

The `core` column is why the pair is worth reading together.
`SplitCommandSubstitution` is one of the two axes `CoreSemantics()`
answers at all, and it can be answered precisely because the panel is
unanimous; `SplitParamExpansion` is left unspecified and refused,
because zsh disagrees and no boolean is both split and not-split. That
is the general rule and not a quirk of word splitting — see
`../README.md` on how an entry names the field that governs it.
