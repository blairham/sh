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
**IFS whitespace** if it is a space character — always space, tab and
newline, and in two of the seven columns the vertical tab, the form feed
and the carriage return as well, which is the axis at the foot of this
section — otherwise it is **IFS non-whitespace**, and the two are treated
differently:

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

### Which characters are the whitespace half

Space, tab and newline are the whitespace half in every column. POSIX
defines the half as the characters of `IFS` that are also **white-space
characters**, and in the POSIX locale that class holds the vertical tab,
the form feed and the carriage return as well — so the standard's own
reading is wider than the three it is usually quoted as.

Two columns take the wider reading and four take the narrower one.
Measured 2026-09-22 under `LC_ALL=C`, `IFS` set to the vertical tab
alone (`setopt shwordsplit` for zsh, which otherwise splits nothing):

| probe | bash 5.3, ksh93 | bash 3.2, dash, zsh, BusyBox ash |
| --- | --- | --- |
| `v=$'a\v\vb'; set -- $v` | `[a][b]` | `[a][][b]` |
| `v=$'\va\v'; set -- $v` | `[a]` | `[][a]` |

The carriage return and the form feed answer exactly as the vertical tab
does in every column, so this is the character *class* and not one
character. bash 3.2 sitting with the narrower group is what makes this a
change within bash rather than a bash-versus-the-rest split.

`interp.Semantics.IFSWhitespaceIsEverySpaceCharacter` is the axis, and it
decides the half in one place for every stage that splits: word
splitting, `read`'s fields, the remainder `read`'s last name takes, and
`[[:IFSSPACE:]]`. A separator set holding none of the three characters at
issue never puts the question, which is the default `IFS` and every
`IFS=:` a script has ever written.

BusyBox ash answers it **twice**, and the record says so rather than
folding the two together: its splitter gives `[a][][b]` above while its
own `read` merges the run. One shell, two notions; the axis holds the
splitter's.
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
parts company, and three ways rather than two.
`Semantics.ReadTrailingEscapedSeparator` is the answer.

Without `-r` a backslash makes the character after it data, and `read`
carries that as a mask into the splitter; the trim at the tail does not
agree about whether the mask reaches it. Measured 2026-09-12 from a
script file with the default IFS:

| line, `read x y` | dash | bash 5.3.15 | ksh93u+, zsh 5.9.2 |
| --- | --- | --- | --- |
| `a b c\ ` | `b c ` | `b c` | `b c` |
| `a b\ c\ ` | `b c ` | `b c ` | `b c` |
| `a x b\ \ ` | `x b  ` | `x b` | `x b` |
| `a b \ ` | `b` | `b` | `b` |

Row two is what makes it three answers. The escaped space in the middle
joins `b` and `c` into one field, so the line holds one field per name and
the last name takes its own field — bash has no remainder to trim there
and keeps the space, while ksh93 and zsh trim the last name's value
whichever way it was reached.

Row four says dash's reading is about the **field** and not about the
character: an escaped space with a separator in front of it is a field of
its own, is not content, and goes in every column. Row three is the same
claim from the other side — two escaped spaces closing a field that *has*
content stay, both of them.

So, stated once each:

- **dash** — the remainder ends where the last field with content of its
  own ends, so a field's escaped trailing whitespace survives and a field
  made of nothing but escaped separators does not;
- **bash** — the trim ignores the mask, and only a value that took a
  remainder is trimmed;
- **ksh93 and zsh** — the trim ignores the mask for the last name's value
  however it was reached.

Two neighboring questions are unanimous and the axis does not carry them.
A **non-whitespace** separator is never trimmed, escaped or not: with
`IFS=:`, `a:b:c\:` gives `b:c:` in all six, and so does `a:b:c:`. And a
non-default *whitespace* IFS splits exactly as the default one does, so
the answer is about the trim and not about which character it takes.

The rows are `read/an-escaped-separator-closing-a-remainder` and
`read/an-escaped-separator-closing-a-field` (#1360).

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

### An empty *element* of a list leaves its field boundary behind

The rows above are a scalar, where there is one field and it goes. A list —
`$@`, `$*`, `${a[@]}`, a bare array name — has one field per element, and an
element that is empty is a field like any other: it is removed only when
nothing in the word joined any text to it. Measured 2026-09-25 against zsh
5.9.2, bash 5.3.20, ksh93u+ 2012 and dash 0.5.12, with the count printed
beside the fields:

| written | every shell | |
| --- | --- | --- |
| `set -- '' 2; x$@y` | `2` `[x]` `[2y]` | the empty element closed the `x` |
| `set -- 1 ''; x$@y` | `2` `[x1]` `[y]` | and opened the `y` at the other end |
| `set -- '' ''; x$@y` | `2` `[x]` `[y]` | one at each end, two fields |
| `set -- ''; x$@y` | `1` `[xy]` | **one** element takes both sides |
| `set -- '' 2 ''; x$@y` | `3` `[x]` `[2]` `[y]` | |
| `set -- '' '' 2; x$@y` | `2` `[x]` `[2y]` | an interior one shows nothing |
| `set -- '' 2; $@` | `1` `[2]` | nothing beside it, nothing to keep |
| `set -- '' 2; $@y` | `1` `[2y]` | |
| `set -- '' 2; x$@` | `2` `[x]` `[2]` | |
| `set -- '' 2; ""$@` | `2` `[]` `[2]` | a quoted null is not an empty element |

**The rule is keyed on the field and not on the element.** Rows four and
three are the pair that says so: one empty element is one field, and one
field takes the text on *both* sides of the expansion at once, where two
empty elements are two fields and the second is where the `y` goes. A rule
stated about the element — "a dropped empty element at an edge closes the
field there" — answers row four `[x][y]` and agrees with this one
everywhere else, including on every row that has no text beside the
expansion, which is most of them.

**A field the *splitter* made is not a field an element made**, and the two
part under a non-whitespace `IFS`. `IFS=:; set -- ':b' c; $@` is `[][b][c]`
in bash, ksh93 and dash: the null a leading separator writes is kept, where
the null an empty element makes is not. Only where the list is joined before
it is split — `Semantics.UnquotedListJoinsOnIFS`, below — is there no element
left to be empty, and then every null is the splitter's: `IFS=:; set -- ''
c; $@` is `[][c]` in the column that joins and `[c]` in the three that do
not.

**The removal is the word's and not the expansion's**, because what decides
it is what reaches the field afterwards. It therefore happens once, at the
end of the word, and reaches every route that builds one: a flag group's
result in the one grammar that has them — `x${(o)a}y` and `x${(s.:.)v}y`
alike — the distribution `${^spec}` and `RC_EXPAND_PARAM` ask for, where the
word is produced once per element and an empty element's copy is the word
with nothing added to it, and a redirection's target, which is a word with
two readings and so two places the boundary shows: zsh opens two files where
`a=('' 2); >x${a[@]}y` names them, bash calls more than one word an ambiguous
redirect, and ksh93 writes the single file `x 2y` its words view joins to.

A context that keeps no fields is the exception, and it is not one: there
the caller joins what comes back, so an empty element is a *separator*
rather than a word. `set -- a '' b; x=$@` is `a  b` — two spaces — in all
four.

### An element that splits away to *nothing* leaves its boundary too

The section above is an element whose **value** is empty. This one is an
element whose value is nothing but separators, which splits into **no field
at all** — a different thing arriving at the same place, and it has no field
to mark. What it leaves is the boundary, which is what the *scalar* path has
recorded since a value of blanks was measured: `v=' '; x${v}y` is `[x] [y]`
in every column that splits.

Measured 2026-09-26 against bash 5.3.20, bash 3.2.57, ksh93u+ 2012, dash
0.5.12, BusyBox ash 1.37.0 and zsh 5.9.2 under `shwordsplit` — the columns
that split a list — with the count printed beside the fields, and unanimous
on every row:

| written | every splitting shell | |
| --- | --- | --- |
| `set -- ' ' 2; x$@y` | `2` `[x]` `[2y]` | the blank element closed the `x` |
| `set -- 2 ' '; x$@y` | `2` `[x2]` `[y]` | and opened the `y` at the far end |
| `set -- ' '; x$@y` | `2` `[x]` `[y]` | **one** element, and both edges |
| `set -- '  ' 2; x$@y` | `2` `[x]` `[2y]` | a run of them is one boundary |
| `set -- ' ' ' ' 2; x$@y` | `2` `[x]` `[2y]` | and so is a run of elements |
| `set -- ' ' 2 ' '; x$@y` | `3` `[x]` `[2]` `[y]` | |
| `set -- 2 ' ' 3; x$@y` | `2` `[x2]` `[3y]` | in the middle it shows nothing |
| `set -- ' a ' 2; x$@y` | `3` `[x]` `[a]` `[2y]` | an element that splits to one |
| `set -- ' ' 2; $@` | `1` `[2]` | nothing beside it, nothing to keep |
| `set -- ' ' 2; $@y` | `1` `[2y]` | |
| `set -- ' ' 2; x$@` | `2` `[x]` `[2]` | |
| `IFS=:; set -- ':'; x$@y` | `2` `[x]` `[y]` | the non-whitespace spelling |
| `IFS=; set -- ' ' 2; x$@y` | `2` `[x ]` `[2y]` | nothing splits, so no boundary |

**The rule is keyed on the *list* and not on the element.** Rows six and
seven are the pair that says so, and they hold the element fixed and move
it: the same blank element is a boundary at an end of the list and is
nothing in the middle, because an element in the middle is already a field
away from its neighbors and has nothing left to separate. So what is
recorded is the first element's opening edge and the last element's closing
one, which is exactly the pair the scalar path reads — and a rule stated
about the element, closing the field wherever a blank element stood, answers
row seven `[x2] [] [3y]` while agreeing with this one everywhere above it.

**A blank element is not an empty one**, and the two sections part on their
one-element row: `set -- ''; x$@y` is the single word `xy` and `set -- ' ';
x$@y` is `[x] [y]`. One empty element is one *field*, and one field takes
the text on both sides at once; one blank element is no field and two
boundaries. A reading that folded them together — dropping the blank element
as though it were empty — gets every other row of both tables right.

**A field the splitter made is still not a field an element made.** The
boundary is a boundary and never a field, so it adds nothing for the removal
above to take away: `IFS=:; set -- ':b' c; $@` is `[][b][c]` as before, and
`IFS=:; set -- ':'; x$@y` is `[x] [y]` rather than `[x] [] [y]` — the
splitter's null absorbs the `x` and the closing edge opens the `y`.

**It reaches a redirection's target**, which is the same word read two ways
and so two places the boundary shows: with `set -- ' ' 2` and the target
`x$@y`, zsh 5.9.2 opens two files, bash 5.3.20 calls it an ambiguous
redirect — which is what more than one word means there — and ksh93u+ and
dash write the single file their words view joins to.

## A separator closes the field beside it, with nothing to show for it

Measured 2026-09-16, script files under `env -i`, one bracketed field per
argument:

    v=" "; printf "[%s]" a${v}b        → [a][b]
    v=" b"; printf "[%s]" a$v          → [a][b]
    v="b "; printf "[%s]" ${v}c        → [b][c]
    printf "[%s]" a$(printf " ")b      → [a][b]
    v=""; printf "[%s]" a${v}b         → [ab]
    v=" "; printf "[%s]" $v            → []

bash 5.3.20, ksh93u+ 2012, dash 0.5.12 and BusyBox ash 1.37.0 answer every
row alike, and zsh answers the command-substitution row with them — it does
not split a parameter expansion at all, which is the row above it. So the
separator is a **boundary in its own right**: splitting a value that is
nothing but separators produces no field, and the text on either side of it
still stops sharing one. The empty value is the control that says it is the
separator doing it rather than the expansion having produced nothing.

### The two edges are not one question

A *non-whitespace* separator at the edge is the same boundary reached by a
different route, and the two ends of the value answer it differently.
Measured 2026-09-16 with `IFS=:`:

    v=":";  printf "[%s]" a${v}b       → [a][b]
    v="b:"; printf "[%s]" ${v}c        → [b][c]
    v="::"; printf "[%s]" a${v}b       → [a][][b]
    v=":";  printf "[%s]" a${v}""      → [a][]
    v=":b"; printf "[%s]" a${v}        → [a][b]
    v=":a:"; printf "[%s]" x${v}y      → [x][a][y]
    v=":";  printf "[%s]" ${v}b        → [][b]
    v="b:"; printf "[%s]" $v           → [b]
    v=":";  printf "[%s]" a${v}        → [a]

bash 5.3.20, ksh93u+ 2012 and dash 0.5.12 answer every row alike, and BusyBox
ash 1.37.0 and zsh 5.9.2 join them on the command-substitution spelling of
each — the last two rows excepted, which is the trailing-separator axis
of *The leading/trailing asymmetry* above.

At the **leading** end the splitter has already written the empty field the
separator asks for, so the boundary is in the fields it returned: `":b"` is
`["", "b"]`, the empty joins whatever text preceded the expansion and `b`
opens a field of its own. At the **closing** end the splitter absorbs the
delimiter and writes nothing, whitespace or not — so the boundary has to be
recorded beside the split, exactly as it is for a whitespace separator, and
the last two rows are what says the field it closes is opened only by
something arriving to go in it.

The one reading that writes a closing field is zsh's, where a trailing
non-whitespace separator ends a field rather than being absorbed. The
boundary is therefore read off the **split** rather than off the text: where
the split wrote that field there is nothing left open, and where it absorbed
the delimiter there is. Recording it from the text in both readings would
give zsh one field too many.

## The word a `-` or `+` substitutes is split like the result it is

    set -- a b; v=x
    printf "[%s]" ${v:+p q}            → [p][q]      one field in zsh
    printf "[%s]" ${nope:-p q}         → [p][q]
    printf "[%s]" ${v:+"$@" "$@"}      → [a][b][a][b]
    IFS=: printf "[%s]" ${v:+"$@" "$@"} → [a][b a][b] in every column
    printf "[%s]" ${v:+"p q"}          → [p q]
    printf "[%s]" ${v:+p }             → [p]

The word substitutes into an unquoted expansion's result, so what is written
in it separates fields exactly as the same text in a value would. The lists
inside it keep their own boundaries and that part is unanimous — the `IFS=:`
row is what separates the two, since there the blank between the two `"$@"`
groups is not a separator and every column writes three fields. Quoted text
in the word is not split, and a separator at either end of the word opens no
field of its own.

zsh does not split an unquoted expansion, so it does not split this either:
the axis is `SplitParamExpansion` and not one of its own.

### A quoted empty word behind that separator is an axis

    set -- a b; v=x
    printf "[%s]" ${v:+p ""}      bash [p][]     dash [p][]     ash [p][]     ksh93 [p]
    printf "[%s]" ${v:+"$@" ""}   bash [a][b][]  dash [a][b][]  ash [a][b][]  ksh93 [a][b]

It is the one span carrying no text that still has to open the field the
separator asked for, and ksh93 declines. `EmptyQuotesAfterASeparatorAreAField`
carries it; zsh cannot reach the question, since nothing there splits the
word in the first place.

## A value's backslash does not quote the separator behind it

    IFS=:
    v='a\:b'
    set -- $v
    printf 'n=%d len1=%d f1=[%s] f2=[%s]\n' "$#" "${#1}" "$1" "$2"

    bash 5.3.15, bash 3.2.57, bash-as-sh, dash, ksh93u+
                        n=2 len1=2 f1=[a\] f2=[b]

Two things at once, and each is the other's guard. The separator **still
separates** — a backslash that arrived in a *value* is an ordinary
character and quotes nothing for this stage — and the backslash **stays
in the field**, because no shell performs quote removal on the result of
an expansion. So the answer is two fields and a first field of two
characters, and a fix that got one of them alone would answer `n=1` or
`f1=[a]`.

**The length is the assertion.** A probe printing only the first field
reads as a quoting artifact of whatever is displaying it, which is how
this went unnoticed: this implementation answered `len1=3`, a backslash
longer than the value, for every dialect that splits.

It is not about the separator being non-whitespace. `IFS=' '` with
`v='a\ b'` splits into the same two fields with the same one backslash,
and that is the form an ordinary script reaches with `IFS` left alone.

The neighboring shapes, measured the same way and all unanimous:

| value | `IFS` | fields | first field |
| --- | --- | --- | --- |
| `a\:b` | `:` | 2 | `a\`, 2 characters |
| `a\ b` | ` ` | 2 | `a\`, 2 |
| `a\\:b` | `:` | 2 | `a\\`, 3 — the pair is not halved |
| `a\bc:d` | `:` | 2 | `a\bc`, 4 — an ordinary character behind it |
| `\:b` | `:` | 2 | `\`, 1 |
| `a\:` | `:` | 1 | `a\`, 2 — the trailing separator is still absorbed |
| `a\:b c` | ` :` | 3 | `a\`, 2 |

zsh answers every row with the value whole, because it does not split a
parameter expansion at all; with `setopt shwordsplit` — or through
`${=v}` — it splits exactly as the five above do, count included. So the
rule is unanimous and the only dialect question here is the one `IFS`
already has: *whether* the result is split.

Measured 2026-09-12 against bash 5.3.15, bash 3.2.57, bash-as-`sh`, dash,
ksh93u+ and zsh 5.9.2, from a script file under
`env -i PATH=/usr/bin:/bin`. Corpus rows
`ifs/a-value-backslash-before-a-separator` and its six neighbors.

**Why an implementation gets this wrong.** The fields an expansion
produces are not carried as text: they are carried in an escaped form
where a mark is a backslash *and the byte behind it*, which is how a
field remembers that a `*` in a value is not a pattern. `a\:b` therefore
reaches the splitter as `a`, `\\`, `\:`, `b`. A splitter that walks that
form one byte at a time cuts at the `:` — correctly — and leaves the `\`
that marked it on the end of the field in front, where the unescape reads
it as a marked backslash. The cure is that the mark belongs to the byte
behind it: cutting at a marked separator takes its mark with it (#2212).

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

### The boundary between two elements is a delimiter in two of the columns

The table above is three columns of one question — whether the list is
joined — and it hides a second one, because dash's column there is also
BusyBox ash's and the two of them read the **gap between two elements**
differently from everybody else. In dash 0.5.12 and BusyBox ash 1.37.0
that gap is itself a delimiter of the ordinary splitting rule, counting
as IFS *whitespace*, so a separator written beside it joins that one
delimiter rather than writing a field of its own.

Measured 2026-09-26 against `/opt/homebrew/bin/bash` 5.3.20,
`/bin/bash` 3.2.57, `/bin/ksh` 93u+ 2012-08-01, `/bin/dash` 0.5.12,
`/opt/homebrew/bin/zsh` 5.9.2 under `shwordsplit` and BusyBox 1.37.0 in
the alpine digest `internal/oracle.Panel` pins, with the count printed
beside the fields — `IFS=:` throughout, and the word `x$@y`:

| parameters | bash | ksh93 | zsh + `shwordsplit` | dash, ash |
| --- | --- | --- | --- | --- |
| `b ':'` | `3 [xb][][y]` | `3 [xb][][y]` | `3 [xb][][y]` | `2 [xb][y]` |
| `b ':' c` | `4 [xb][][][cy]` | `3 [xb][][cy]` | `4 [xb][][][cy]` | `2 [xb][cy]` |
| `a ':' b ':'` | `6` | `5 [xa][][b][][y]` | `6` | `3 [xa][b][y]` |
| `'b:' ':c'` | `4` | `3 [xb][][cy]` | `4` | `3 [xb][][cy]` |
| `':' ':'` | `4` | `3 [x][][y]` | `4` | `3 [x][][y]` |
| `a b` | `2 [xa][by]` | `2 [xa][by]` | `2 [xa][by]` | `2 [xa][by]` |

**The rule is a sentence with one noun in it: the *boundary* is IFS
whitespace.** Read the word as its text with a blank standing between
adjacent elements and split it with the ordinary rule — a run of IFS
whitespace, at most one non-whitespace separator, a run of IFS
whitespace — and every dash and ash row falls out of it.

Three other readings answer the first row the same way and part on the
rows below, which is what makes those rows the ones to keep:

- The **element**-keyed reading, that a separator at the edge of an
  element merges into the boundary beside it, answers rows four and five
  two fields. Both are three: one delimiter takes at most one
  non-whitespace separator, so the second starts another and the empty
  field between them stands.
- The boundary as an ordinary IFS **separator** rather than whitespace
  answers row one three fields, two separators in a row making an empty
  field between them.
- The **join** above answers row one three fields too, for that reason
  with the separator written rather than implied.

And the last row is the half the others cannot show: a boundary with no
separator anywhere near it still cuts, which is what makes it a
delimiter rather than something that only appears beside one. The pair
that says so holds the characters fixed and moves the boundary —
`set -- a b` is two fields where `set -- ab` is one, and `set -- b ':'`
is two where `set -- 'b::'` is three.

**Under a whitespace `IFS` all the readings coincide**, because a
boundary and a run of blanks are the same delimiter either way — so
nothing in this is visible to a script that leaves `IFS` alone. With
`IFS` set and empty nothing splits at all and the elements are one field
each in every column.

This is not the **empty element** of the two sections above. An empty
element makes no field under this reading and a removable null under the
other, and the word is the same either way: `IFS=:; set -- b '' c; x$@y`
is `[xb] [cy]` in both. Where they meet is an empty element standing
*beside* a separator element, and there only this question moves the
word: `IFS=:; set -- b ':' '' c; x$@y` is `[xb] [cy]` in dash and ash and
`[xb] [] [cy]` in ksh93.

The axis is `UnquotedListBoundaryIsIFSWhitespace`, and it is asked only
where the two readings give different fields — which needs two elements
at least and a character of `IFS` at the edge of one of them.

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
| `EmptyQuotesAfterASeparatorAreAField` | yes | yes | **no** | unreachable | yes |
| `UnquotedListJoinsOnIFS` | no | **yes** | no | no | unanswered |
| `UnquotedListBoundaryIsIFSWhitespace` | **yes** | no | no | no | unanswered |

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
