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
| `IFS=:; x='a:'; set -- $x` | `[a]` — but see the axis below |
| `IFS=:; x='::'; set -- $x` | 2 fields |
| `IFS=' :'; x='a : b'; set -- $x` | `[a][b]` |
| `IFS=' :'; x='a::b'; set -- $x` | `[a][][b]` |

### The leading/trailing asymmetry

Two rows above are the same construct at opposite ends of the string and
give different answers:

    IFS=:; x=':a'   → [][a]     leading delimiter makes an empty field
    IFS=:; x='a:'   → [a]       trailing delimiter does not

A trailing delimiter is absorbed; a leading one is not. It is the single
most common source of off-by-one field counts, and any implementation
that treats splitting as a symmetric "split on delimiter" produces
`[a][]` for the second row.

POSIX.1-2024 2.6.5 specifies the absorbing reading — "once the input is
empty, the candidate shall become an output field if and only if it is
not empty" — and dash, bash, bash 3.2, bash as `sh` and ksh93 all
implement it. **zsh does not**, and that is
`Semantics.TrailingSeparatorEndsAField`: the trailing separator delimits
there, so every such split has one more field. It cannot be seen through
a parameter expansion in zsh as it comes, because that is not split at
all; asking it needs `setopt shwordsplit`, the `${=spec}` flag, an
unquoted command substitution — which every shell splits — or `read`.

Measured 2026-09-07 with `IFS=:`, printing the count with the fields:

| value | five shells | zsh |
| --- | --- | --- |
| `a:` | `1 [a]` | `2 [a][]` |
| `a::` | `2 [a][]` | `3 [a][][]` |
| `a:b:` | `2 [a][b]` | `3 [a][b][]` |
| `:a:` | `2 [][a]` | `3 [][a][]` |
| `:` | `1 []` | `2 [][]` |
| `` | 0 fields | 0 fields |

Three things bound it, and each is a row a wrong fix gets wrong. A
**leading** separator delimits in all six, so the asymmetry is at the
tail alone. **Whitespace** is absorbed at both ends in zsh as well —
`' a '` is one field everywhere — so this is the non-whitespace half of
the rule and nothing else. That last sentence is about an *expansion*;
`read` answers it the other way and has an axis of its own, below. And it is the closing **run** that decides
rather than the last byte: with `IFS=' :'`, `'a: '` is two fields in zsh
and `'a  '` is one, so the trailing space does not hide the colon in
front of it.

An **escaped** separator is data and is not part of the run at all, which
is why the mask `read` carries reaches this question: `IFS=:; read -A a`
on `a\:` fills one element `a:` in zsh where `a:` fills two.

The empty value is the row that says the answer adds a field rather than
keeping an edge: an unquoted empty expansion is no field at all in all
six, zsh included, so nothing is opened where there was no separator.
That is also what separates this from the edge-keeping rule a **quoted**
`${=spec}` follows, which keeps both edges unconditionally and makes `''`
one empty field — see `docs/spec/grammar/parameter-expansion.md`. Where
that rule is in force the field behind the last separator is already
there and this question is not asked.

## `read`'s last name takes the remainder of the line

`read` feeds the splitter above, and it uses what comes out two ways. An
array target — `read -a`, `read -A` — takes the fields **as fields**. A
list of names does not: each name but the last takes its field, and the
**last name takes the remainder of the line**, which is the input text
from where its own field began, separators and all.

    IFS=:; printf 'a:b:c\n' | { read -r x y; }    → x=a  y=b:c

The remainder is text, so it is not the remaining fields put back
together. Joining them on a space is the answer that looks right — the
right number of words, the right words, status 0, nothing said — and it
is wrong in all six shells:

    printf 'root:x:0:0:Root User:/root:/bin/sh\n' |
        { IFS=: read -r user rest; }

    all six   rest=x:0:0:Root User:/root:/bin/sh
    a rejoin  rest=x 0 0 Root User /root /bin/sh

`while IFS=: read -r user rest; do …; done < /etc/passwd` is the shape
that reaches it, and anything that splits `$rest` on `:` again finds one
field where there were six. Measured 2026-09-07 across the panel; the
rows are `ifs/read-remainder-*` in `docs/spec/measurements.md`.

### What comes off the end

The closing run of IFS **whitespace**, and nothing else. Three
measurements bound it, and each is one a plausible trim gets wrong:

| probe | remainder |
| --- | --- |
| `IFS=': '; 'a:b:c  '` | `b:c` — the spaces are IFS whitespace |
| `IFS=':'; 'a:b:c  '` | `b:c  ` — the same spaces are ordinary text |
| `IFS=':'; 'a:b:c::'` | `b:c::` — a closing *separator* run is kept |

So the trim is IFS's question rather than `unicode.IsSpace`'s, and it is
the whitespace half of IFS rather than all of it. Leading IFS whitespace
never reaches the remainder at all: it is discarded by the splitter
before the first field starts, which is why `read -r line` on `'  a b  '`
is `a b`.

An **escaped** IFS whitespace character at the end is where the panel
parts company, five to one — bash, bash 3.2, bash as `sh`, ksh93 and zsh
trim it, dash keeps it — and that is #1360 rather than an answer here.

### It is a remainder only past the count

With one field per name the last name takes its own **field**, not the
remainder, and the two differ exactly when the line ends in a separator:

    IFS=:; printf 'a:b:\n' | { read -r x y; }
    five shells  y=b        zsh  y=b:

That is `Semantics.TrailingSeparatorEndsAField` again and not a second
rule. In zsh the trailing separator opens a third field, three fields
against two names is a remainder, and the remainder keeps the colon; in
the other five the separator is absorbed, two fields meet two names, and
`y` is the field `b`. The same fact reaches a single name — `'a:'` read
into one name is `a` in five and `a:` in zsh — and a line of nothing but
separators, where `'::'` into two names leaves the second empty in five
and `:` in zsh.

`read`'s names therefore ask the tail question **at that count and
nowhere else**. Past it the extra empty field changes no value, because
the remainder is the same text under either answer; short of it the name
the extra field would fill is the empty string it was going to be given
anyway. An array target has no such boundary and asks at every count.

### `read` into an array has two edges of its own

The rule above is measured on an expansion. `read` filling an **array**
answers two more questions, and one shell answers the whitespace half of
the rule the *opposite* way there — same shell, same `IFS`, two answers,
which is why they cannot be one field.

Measured 2026-09-07 with a here-string and the element count read back.
The letter differs by shell — `-A` in ksh93 and zsh, `-a` in bash, and
dash has neither — so the corpus rows pick it at run time.

| line | bash 5.3 / 3.2 / as-`sh` | ksh93u+ | zsh 5.9.2 |
| --- | --- | --- | --- |
| `a  ` | `1 [a]` | `1 [a]` | `2 [a][]` |
| `a	` | `1 [a]` | `1 [a]` | `2 [a][]` |
| ` a ` | `1 [a]` | `1 [a]` | `2 [a][]` |
| `a: ` with `IFS=' :'` | `1 [a]` | `1 [a]` | `2 [a][]` |
| ` a` | `1 [a]` | `1 [a]` | `1 [a]` |
| `` (empty) | 0 elements | `1 []` | `1 []` |
| `   ` | 0 elements | `1 []` | `1 []` |

**`Semantics.ReadTrailingWhitespaceEndsAField`** is the first four rows:
a closing run of IFS whitespace opens an element of its own in zsh's
`read`. Through an expansion the same shell absorbs it — `x=' a '; set --
${=x}` is one field — so this is a second field and not the one above.
A *leading* run is still absorbed, and it is the run rather than the
byte: `a::` with `IFS=:` is three elements in zsh and not four.

**`Semantics.ReadNoFieldsIsOneEmptyElement`** is the last two: a line
that splits into nothing leaves one empty element in ksh93 and zsh and
none in bash. ksh93 sides with zsh here and with bash above, which is
what makes them two questions rather than one.

**The order is load-bearing**, and a line of nothing but whitespace is
the only shape that shows it: that line has a closing whitespace run
*and* no fields, so a shell answering yes to both could leave two
elements. zsh leaves one — the closing run is asked first, and the
second question is not put once a field is there.

Neither question is asked of a list of **names**. The last name takes the
remainder of the line and the closing run of IFS whitespace comes off
that remainder anyway, so `read x` on `'a  '` is `a` and `read x y` on
`'a b  '` is `a` and `b` under either answer. An axis reported where it
decides nothing is a refusal a script cannot act on.

## Empty and unset values produce no fields

    x=''; set -- $x     → 0 fields
    x=''; set -- "$x"   → 1 field, empty
    unset u; set -- $u  → 0 fields

An unquoted expansion of an empty value vanishes. Quoting it produces one
empty field. All four shells agree.

## `$@` and `$*` unquoted

Without quotes the two are the same expansion in every shell measured,
with one shape excepted below — and *which* expansion is a dialect's
answer, not a fact.

bash, bash 3.2 and bash as `sh` make the list one string first, joining
the elements on the **first character of `IFS`**, and split that. zsh,
ksh93 and dash split each element on its own and never join. The two
readings agree under a whitespace `IFS`, which is why the difference is
easy to miss.

Measured with `IFS=:`, printing the field count with the fields:

| parameters | bash | ksh93 | dash | zsh | zsh + `shwordsplit` |
| --- | --- | --- | --- | --- | --- |
| `x "" y` | `3 [x][][y]` | `2 [x][y]` | `2 [x][y]` | `2 [x][y]` | `3 [x][][y]` |
| `x y ""` | `2 [x][y]` | `3 [x][y][]` | `2 [x][y]` | `2 [x][y]` | `3 [x][y][]` |
| `"x:" y` | `3 [x][][y]` | `2 [x][y]` | `2 [x][y]` | `2 [x:][y]` | `3 [x][][y]` |
| `":x" y` | `3 [][x][y]` | `3 [][x][y]` | `3 [][x][y]` | `2 [:x][y]` | `3 [][x][y]` |

Every one of bash's answers is the scalar split of the joined string —
`x::y`, `x:y:`, `x::y`, `:x:y` — which is what says it joins rather than
that it has a second rule about empty elements. The rule has three faces
and they must not be modeled separately:

- an empty element **between** others survives the join, because two
  separators meet and the field between them is a field;
- an empty element at the **end** does not, because the join puts a
  trailing separator there and a trailing delimiter is absorbed (see the
  leading/trailing asymmetry above);
- an element that **ends in a separator** makes an empty field with
  nothing empty anywhere, because its separator meets the join's.

zsh is the shell that shows the join is real: with its splitting off it
answers `[x:][y]` where no arrangement of the splitting answer can, since
joining would give one field and it gives two.

The excepted shape is ksh93's, and it is the second row: a **trailing**
empty element is one field for `$@` there and none for `$*`, so ksh93 is
the only shell in the panel where the two unquoted spellings part. It is
also the only row neither reading explains — dropping the empty element
gives two fields and keeping it gives three for both spellings, and
ksh93 gives three for one and two for the other.

Two guards. There is nothing to join with when `IFS` is set and empty —
`IFS=""; set -- x y` is two fields for `$@` and `$*` alike in all four,
where joining would leave one — and nothing to join when the list has one
element. And the splitting answer stands in front of the join: with
splitting off there is nothing to undo it, and zsh gives one field per
element rather than one field.

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
