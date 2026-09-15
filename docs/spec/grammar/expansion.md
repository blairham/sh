# Expansion

How a word becomes zero or more fields.

## The pipeline

Expansion is a fixed sequence of stages. POSIX.1-2024 XCU §2.6 defines
four groups; the shells that extend it insert stages at the ends rather
than in the middle.

    1. brace expansion          (extension — not POSIX; absent in dash)
    2. tilde expansion
    3. parameter expansion      \
    4. command substitution      | performed left to right, in one pass
    5. arithmetic expansion     /
    6. field splitting          (on the unquoted results of 3-5 only)
    7. pathname expansion       (globbing)
    8. quote removal

**Almost every surprising result in this document is a consequence of
that ordering rather than of any single stage.** Two invariants follow
from it and are worth stating before the stages themselves.

## Invariant 1: expansion results are not rescanned

The text produced by stages 3-5 is data. It is subject to stages 6-7 —
splitting and globbing — and to nothing else. It is never re-parsed for
quotes, operators, or further expansions.

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `x='a"b'; set -- $x` | `[a"b]` | `[a"b]` | `[a"b]` | `[a"b]` |
| `x='$HOME'; echo "$x"` | `$HOME` | `$HOME` | `$HOME` | `$HOME` |
| `x='a;b'; set -- $x` | `[a;b]` | `[a;b]` | `[a;b]` | `[a;b]` |

A quote in expanded text is a literal quote. A `$` in expanded text does
not expand. A `;` in expanded text is not a separator. All four shells
agree, so this is **core behavior with no vector field**.

This is why `eval` exists: it is the only way to ask for a second pass,
and it is the whole difference between data and code in a shell.

## Invariant 2: quoting is decided before expansion, not after

Whether a word was quoted is a property of the *source text*, established
at parse time. It cannot be reconstructed from the expanded value — the
same characters behave differently depending on where the quotes were:

| probe | result | why |
| --- | --- | --- |
| `set -- "et*"` | `[et*]` | literal, quoted — no globbing |
| `x='et*'; set -- "$x"` | `[et*]` | expansion quoted — no splitting, no globbing |
| `x='et*'; set -- $x` | `[etc]` | expansion unquoted — split, then globbed |

The parser must therefore record quoting per *span* within a word, not
per word. A word can be partly quoted (`a"b c"d`), and only the unquoted
spans of an expansion result are subject to stages 6-7.

## 1. Brace expansion

Not POSIX, and **absent from dash**:

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `echo {1..3}` | `{1..3}` | `1 2 3` | `1 2 3` | `1 2 3` |
| `a=1; echo {$a,2}` | `{1,2}` | `1 2` | `1 2` | `1 2` |
| `n=3; echo {1..$n}` | `{1..3}` | `{1..3}` | `1 2 3` | `1 2 3` |

It runs **before** parameter expansion, which is why the second row
produces two fields rather than one: the braces are resolved against the
literal text `$a,2`, and only then does `$a` become `1`.

The third row is where that ordering stops being a fact and becomes a
**disagreement**. bash keeps it for a *range* too, so the range is gone by
the time `$n` exists and the word stays literal; zsh and ksh93 expand a
range's endpoints first and read the range afterwards.
`BraceRangeEndpointsExpanded` is that axis, asked only where a range is
written with something to expand in it — a literal `{1..3}` is unanimous
and is never a question. Corpus:
`expand/brace-range-expanded-endpoint`.

Three things follow, all measured (`-quoting` and `-refused`):

- **Quoting hides an endpoint from the brace scanner, not from the
  range.** `{1..'3'}`, `{1.."$n"}` and `{"1"..3}` all count `1 2 3` in
  the two shells that expand endpoints, while `"{1..3}"` — the whole
  word quoted — is a literal everywhere.
- **A comma is a list before it is ever a range.** `{1..3,5}` is the two
  words `1..3` and `5` in bash, ksh93 and zsh alike, so nothing may
  expand a body a range will not read.
- **A range that does not form gives back what it expanded, once.**
  `{1..$(f)}` with a non-numeric `f` runs `f` a single time, and the text
  left behind is neither field-split nor matched: ksh93 splits `$sp` in a
  word of its own and still leaves `{1..$sp}` a single field.

Apart from a range's endpoints it is purely textual: it does not consult
the filesystem and never fails. An unmatched or malformed brace is left
alone.

### Ranges beyond `{1..3}`

Numbers are not the whole of it. Each row below is a corpus case under
`expand/brace-range-…`, and the shells that expand braces at all agree
on the first two:

- **Letters range too**: `{a..e}` is `a b c d e`, either direction,
  counted in code points (`-alphabetic`).
- **A third number strides the range**: `{1..10..3}` is `1 4 7 10`
  (`-stepped`). The step is a **bash 4** addition — bash 3.2 leaves the
  word alone, dated rather than vetoed per `../core.md` — and this
  implementation reads one wherever braces expand at all.
- **A range's elements are data, a list's are text.** zsh's `{=..?}` is
  the three characters `= > ?` with the `?` never matched against a
  filename, while its `{?,x}` matches; bash writes the bracket, the
  backslash and the caret of `{A..z}` out the same way. So a counted
  element is put back quoted and an alternative is not.

The rest is disagreement, and each row is a semantics axis rather than a
core answer:

- **Zero padding** (`-zero-padded`): a leading zero on either endpoint
  pads the whole range to the widest width in bash and zsh — `{01..03}`
  is `01 02 03`, zeros after the sign — and ksh93 strips it.
  `BraceRangePadsToEndpointWidth`.
- **A step's sign** (`-step-sign-and-direction`): bash takes magnitude
  only, the endpoints deciding direction; ksh93 honors the sign and
  yields a single element when it points the wrong way.
  `BraceRangeStepSignHonored`.
- **A negative step in zsh** (`-negative-step-reversal`): zsh *reverses
  its result* — `{1..10..-4}` is `9 5 1`, bash's `1 5 9` backwards, not
  a walk from 10. `BraceRangeNegativeStepReverses`.
- **What a range between two single characters spans**
  (`-between-any-two-characters`, `-alpha-stepped`): zsh counts between
  whatever the two characters are — `{1..x}` is seventy-two words,
  `{α..γ}` is three, `{1...}` is `1 0 / .` — and takes **no step**, so
  `{a..z..2}` is the word as written there where bash and ksh93 count
  `a c e …`. The body is counted in characters rather than cut at its
  first `..`, which is what makes `{....}` the single word `.` and
  `{.....}` the word as written. `BraceCharRangeSpansAnyCharacter`.
- **A `+` in front of a number** (`-endpoint-with-a-plus`): bash and
  ksh93 read `{+1..2}` as `1 2`; zsh takes a `+` anywhere as putting the
  body outside the reading. `BraceRangeNumberMayCarryAPlus`.
- **A range with a component missing** (`-missing-endpoint`,
  `-missing-endpoint-drops-the-braces`, `-zero-step`): three answers to
  one word. bash leaves `{1..}` alone; ksh93 counts the missing *second*
  endpoint from zero and answers `1 0`; zsh takes the **braces off** and
  leaves `1..` standing as ordinary text. The same three part over a
  written step of zero: bash reads it as one and counts `{1..2..0}` as
  `1 2`, ksh93 leaves the word, zsh drops the braces.
  `BraceRangeMissingEndCountsFromZero`, `BraceRangeZeroStepCountsAsOne`
  and `BraceRangeThatCannotBeCounted`.

  The shape that reaches those axes is narrow, and the narrowness is
  measured rather than defensive: the first endpoint is an unsigned run
  of digits and the second and the step may each carry a `-`, so
  `{-1..}`, `{+1..}` and `{1..2..x}` are the word as written in every
  column. A body with **no digit at either end** is left alone
  everywhere too, which is what separates zsh's `{..2..}` — the word —
  from its `{1..2..}`, which is `1..2..`.

The core expands what is unanimous and asks the vector where the answers
part; `interp/brace.go` names the same fields, plus the ordering one
above.

**Core**: present. Dialect `posix` disables it (matching dash).

## 2. Tilde expansion

Applies only to an **unquoted** `~` at the **start of a word**, and in
the value of an assignment.

| probe | all four |
| --- | --- |
| `echo ~` | absolute path |
| `echo "~"` | literal `~` |
| `x=~; echo $x` | absolute path |
| `echo a~` | literal — not at word start |

The assignment case is the one that surprises people: `x=~` expands
because assignment values are a tilde-expansion context, so `PATH=~/bin`
works as intended.

All four agree. **Core behavior, no vector field.**

### `~+` and `~-`

Two named tildes expand to directories rather than to a home: `~+` is
`$PWD` and `~-` is `$OLDPWD`, in bash, ksh93 and zsh — dash keeps both
as written — and only while the variable is set: `unset OLDPWD; echo ~-`
stays literal, except in zsh, which still answers from directory state
of its own (measured: `expand/tilde-plus-and-minus`). The exception is
recorded rather than modeled; the shape shared by all three is
"expand the variable when it is set", and that is what the
`TildePlusMinusExpands` axis provides (no for `posix`; the bash, ksh
and zsh dialects say yes).

A `~user` form needs a user database and is a different question, which
is why the two are not lumped together as "the other tildes".

## 3-5. Parameter, command and arithmetic expansion

Performed left to right in a single pass. The distinction that matters
downstream is not between these three stages but between their results
being **quoted or unquoted**, which decides whether stage 6 sees them.

One asymmetry is measured and is *not* what the names suggest — see
`word-splitting.md`: zsh declines to split the result of a **parameter**
expansion but splits a **command substitution** like every other shell.

## 6. Field splitting

Its own document: `word-splitting.md`.

## 7. Pathname expansion

Each field produced by stage 6 is matched against the filesystem.

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `cd /; x='et*'; set -- $x` | `[etc]` | `[etc]` | `[etc]` | **`[et*]`** |
| `cd /; echo /zzz_no_such*` | pattern | pattern | pattern | **error** |

Two separate zsh divergences here, and conflating them is a mistake:

- **zsh does not glob the result of an expansion.** A literal pattern in
  the source is expanded; a pattern that arrived via `$x` is not. zsh
  spells the opt-in `${~x}`, which is a syntax error in the other three.
- **zsh errors on a pattern that matches nothing**, where the others
  pass the unmatched pattern through unchanged.

Semantics axes: `GlobExpansionResults` (dash, bash and ksh93 yes; zsh
no) and `GlobNoMatchIsError` (dash, bash and ksh93 no; zsh yes). Neither
is answered in the core, so an unqualified core run refuses the glob
rather than picking a side.

## 8. Quote removal

The quote characters that survived stages 1-7 are removed. Nothing else
happens: by this point every decision that depended on quoting has
already been made, which is Invariant 2 restated from the far end.
