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

It runs **before** parameter expansion, which is why the second row
produces two fields rather than one: the braces are resolved against the
literal text `$a,2`, and only then does `$a` become `1`. A brace range
whose endpoints are variables therefore does not work — the range is
already gone by the time the variable exists.

Brace expansion is purely textual: it does not consult the filesystem and
never fails. An unmatched or malformed brace is left alone.

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

Vector fields: `GlobExpansionResults` (default true) and
`GlobNoMatchIsError` (default false).

## 8. Quote removal

The quote characters that survived stages 1-7 are removed. Nothing else
happens: by this point every decision that depended on quoting has
already been made, which is Invariant 2 restated from the far end.
