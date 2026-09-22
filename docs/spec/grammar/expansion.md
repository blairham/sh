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

### `.` and `..` in a listing

Whether a pathname expansion's component match may reach the two names
every directory holds. Measured 2026-09-14 in a directory holding
`a.txt`, `.dot` and `sub` (`glob/dot-and-dotdot-in-a-listing`):

| shell | `echo .*` | `echo .*/` |
| --- | --- | --- |
| bash 5.3, bash-as-sh, zsh | `.dot` | no match |
| ksh93, dash, BusyBox ash, bash 3.2 | `. .. .dot` | `../ ./` |

Three of the panel's columns against three, with bash disagreeing with
itself across versions — so the preset that models 5.3 answers no and
nothing here claims to be 3.2. `GlobListsDotAndDotDot`.

The leading-period rule is what keeps the two names out of an ordinary
`*`, so this is the **listing** rather than a hidden-name option: turning
hidden names on is what makes `echo *` show them in the columns that have
them, which is how the rule came to be filed with ksh93's `FIGNORE`
(#2748, and `patterns.md` for the parameter). The `**` descent never
follows them — a `..` descended into climbs out of the tree and does not
stop, and no column does that.

### The second route to the same two names, and why it is not the axis

bash 5.2 added `globskipdots`, which is **on** with nothing said — so the
row above is its default and the option is how a script asks for the two
names back. It is `PeriodPatternListsDotAndDotDot`, a `MatchOption` rather
than an axis, because the one shell that has it switches it while it runs.

What makes it a second mechanism rather than a second reading is that the
two shells reach the names by different rules. Measured 2026-09-17 on
bash 5.3.20 under `LC_ALL=C`, in a directory holding `.a`, `.b`, `vis` and
a `sub/` holding `.x` and `y`, with the option **off**:

| written | bash 5.3.20, `globskipdots` off |
| --- | --- |
| `.*` | `. .. .a .b` |
| `*` | `sub vis` |
| `shopt -s dotglob`, then `*` | `.a .b sub vis` |
| `*/.*` | `sub/. sub/.. sub/.x` |
| `.*/` | `../ ./` |
| `shopt -s globstar`, then `**` | `sub sub/y vis` |

The third row is the one that decides it. In the columns the axis holds,
lifting the leading-period rule is exactly what brings the names into a
`*` — ksh93's `FIGNORE=x; echo *` lists them. Here the rule is lifted,
the names are asked for, and `*` still does not see them: what this option
governs is the listing a component **the pattern wrote with a period** may
match, and nothing else. One rule for both would have made the wrong shell
answer `.` for `*`.

Three smaller facts from the same table. It is the component and not the
pattern, so `*/.*` sees them one level down. The listing is sorted with
them in it. And the `**` descent does not list them, which is the gap the
axis already has and here is the measured answer.

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

### A bare `~` after the script has assigned to `HOME`

One build answers this with a home the script has already replaced, and the
row is written down because the obvious reading of it is wrong.

Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C HOME=/orig`, from
`-c`, `HOME=/h; echo ~`:

| shell | answer |
| --- | --- |
| bash 5.3.20 | `/orig` |
| bash as `sh` | `/orig` |
| bash 3.2.57 | `/h` |
| zsh 5.9.2 | `/h` |
| ksh93u+ | `/h` |
| dash 0.5.12 | `/h` |
| BusyBox ash 1.37.0 | `/h` |
| here | `/h` |

**It is not the home the shell started with**, which is what it looks like
from that one probe and is what #3484 was filed on. It is a **cache that
anything building an environment for a child refreshes**:

    HOME=/h; true; echo ~                      /orig     a builtin
    HOME=/h; export FOO=1; echo ~              /orig
    HOME=/h; ( true ); echo ~                  /orig     a subshell
    HOME=/h; /usr/bin/true; echo ~             /h        an external command
    HOME=/h; echo a | cat >/dev/null; echo ~   /h        a pipeline
    HOME=/h; echo $(true); echo ~              /h        a command substitution
    HOME=/h; echo ~; /usr/bin/true; echo ~     /orig then /h

and it is the tilde alone: a bare `cd` in that shell goes to the current
`$HOME` while `cd ~` on the same line goes to the cached one, and `~user`,
`~+` and `~-` all answer from the database and from `PWD`/`OLDPWD` as
usual.

**What refreshes it is the child's *environment*, not the child**, and that
is the measurement that settles what this is. Re-measured 2026-09-21 on bash
5.3.20, taking the exported attribute off `HOME` rather than changing its
value:

    HOME in env at startup:
      HOME=/h; /usr/bin/true; echo ~                  /h
      HOME=/h; export -n HOME; /usr/bin/true; echo ~  the password database
      unset HOME; /usr/bin/true; echo ~               the password database

    no HOME in env at startup:
      HOME=/h; /usr/bin/true; echo ~                  the password database
      export HOME=/h; /usr/bin/true; echo ~           /h
      HOME=/h; export HOME; /usr/bin/true; echo ~     /h

The second line is the one to read: `export -n` changes nothing whatever
about `$HOME` — the shell still reads `/h` from it — and it moves what `~`
expands to. So the refresh reads the block of `NAME=value` strings the shell
has just built for a child, and falls back to the password database when
that block has no `HOME` in it. Neither "the variable" nor "the value at
startup" can produce that row.

**Recorded and not modeled**, which is a decision and not an omission. Two
readings are available and both are worse than answering `$HOME`:
freezing the value at startup contradicts six of the rows above, and
reproducing the cache means reproducing an invalidation that is incidental
to building a child's environment — a `HOME=/x; cd ~` that goes to the old
home for as long as the script runs only builtins, and a `~` that moves when
a name's export attribute does. It is also one build against its own 3.2, so
it is a candidate upstream regression rather than a family rule. The cost of
not taking it is countable and small: seven lines of one fetched suite file
(#3484), and eight of two more (#4039).

**Three corpus rows hold it, and the split between them is the point.**
`expand/tilde-after-a-home-assignment` asks the word before *and* after a
child, so the row cannot be read as a freeze;
`expand/tilde-after-a-home-assignment-reaches-every-road` asks the same
question of an assignment's value, of the tilde after an unquoted colon in
one, and of a `cd` operand, and every column answers all three alike — which
is why there is one home for the tilde here and not three;
`expand/tilde-with-no-home-at-all` records the second question, what a `~`
is worth with nothing to expand it against, where the panel splits four ways.
`TestABareTildeReadsTheCurrentHomeBeforeAndAfterAChild` and
`TestEveryTildeRoadReadsTheOneCurrentHome` are the Go side.

**This has now been filed twice from the same one-line probe** — #3484 and
#4039 — and both times as a home frozen at startup, because `HOME=/h; echo ~`
and `unset HOME; echo ~` are exactly the two lines a freeze and a stale cache
answer identically. A third reading of the panel starts by putting an
external command in the middle.

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

### `~N`: an entry of the directory stack

A tilde over a **number** names an entry of the directory stack rather
than a home. `~N` and `~+N` count from the top, where slot zero is the
current directory and the pushed entries follow it; `~-N` counts from
the bottom. Measured 2026-09-22 on bash 5.3.20 from a script file, after
`pushd /tmp; pushd /usr` in a scratch directory, so that the stack reads
`/usr /tmp /scratch`:

| word | expands to | |
| --- | --- | --- |
| `~0`, `~+0` | `/usr` | slot zero is `$PWD`, not the first push |
| `~1`, `~+1` | `/tmp` | |
| `~2`, `~+2` | `/scratch` | |
| `~3`, `~+3` | `~3`, `~+3` | past the end, so the word stands as written |
| `~-0` | `/scratch` | the bottom entry |
| `~-1` | `/tmp` | |
| `~01`, `~+01` | `/tmp` | leading zeros are a number like any other |
| `~1a` | `~1a` | a name with a digit in it is not an index |
| `~0/x` | `/usr/x` | the tail after the first slash is the tail |

A number that is past the end is **not** then looked for as a name. That
is the row worth stating on its own: a shell that fell through to the
user database there could be handed a home directory by a system that
happens to have a login called `3`.

The stack is not this package's to keep — `pushd`, `popd` and `dirs` are
a dialect's prelude, and bash's `$DIRSTACK` is a view over the storage
that text owns — so the axis is the *name* of the array to read,
`Semantics.DirectoryStackParameter`, and not a yes/no. Empty is the
whole of "this shell has no numbered tilde": bash says `DIRSTACK`, and
the rest of the panel says nothing, zsh included until its own stack is
measured.

### `~name`: a user, or a directory the shell was told about

Two things wear a `~` and a name, and only one of them is the shell's.

**`~user` is the user database's answer**, and every column in the panel
gives it. It is not a semantics axis and it is not this package's to
read either: `interp.Runner.UserHomeDir` carries it, the binaries wire
it to `os/user` in `driver`, and a Runner with no hook leaves the word
exactly as written — which is the right default for a library embedded
in a program that has no business opening a password file. It is also
what keeps a test off the machine's own users: a test that named a real
one would pass on a laptop and fail on a runner.

**`~name` is a *named directory*** — a table this shell owns outright,
written by `hash -d name=dir` and read back by the tilde, needing
nothing from the operating system. zsh alone has it
(`expand/tilde-naming-a-named-directory`); bash spells `hash -d` and
means *forget one hashed name* by it, which is
`Semantics.HashDefinesANamedDirectory` against
`Semantics.HashForgetsOneName` — one letter, two builtins, and no
dialect has both. The table is also `$nameddirs`, which reads it and
writes it.

The order between them is measured: a **named directory wins**.
`hash -d root=/tmp; print -r -- ~root` is `/tmp` in zsh where the same
line without the assignment is `/var/root`.

What a *miss* costs is still one shell's own and is not reproduced here:
bash, bash 3.2, bash-as-sh, ksh93 and dash leave `~nosuchuser` as
written at status 0, and zsh refuses it — `no such user or named
directory` — ending the line. This shell gives the first answer in every
dialect (`expand/tilde-naming-a-user-with-no-entry`, #2191).

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

### The order the matches come back in

**Byte order**, which is a decision and not the absence of one. Measured
2026-09-15, each shell started with `env -i` and nothing but the row's
own variables, in a directory holding `1digit`, `A_upper`, `Cherry`,
`Z`, `_under`, `a`, `b`, `banana` and `date`:

| set | bash 5.3 | ksh93 | zsh | dash |
| --- | --- | --- | --- | --- |
| nothing at all | collates | bytes | bytes | bytes |
| `LC_ALL=C`, `LANG=C` | bytes | bytes | bytes | bytes |
| `LANG`/`LC_ALL`/`LC_COLLATE` = `en_US.UTF-8` | collates | collates | collates | bytes |
| `LC_COLLATE=C LANG=en_US.UTF-8` | bytes | bytes | bytes | bytes |

So it is a **locale** question and a dialect one at once — dash is the
holdout in every row that has one — `LC_COLLATE=C` is byte order
everywhere, and an **unset locale is not unanimous**: bash reads it as
the system's default and collates where the other three read it as C.
That last row is the one a continuous integration runner is in, so a
script whose output is a sorted glob can answer two ways on one machine
depending on which shell ran it.

The collation itself is not attempted, and `interp/order.go` carries the
three reasons next to the code: the platforms disagree about the same
locale name, no dependency or generated table settles that, and an
approximation was tried and is wrong on an ordinary directory. What this
shell guarantees instead is that the order is decided **once** —
`shellOrder`, reached by a pathname expansion, by the words a glob
qualifier list's modifiers produced, and by the `o` and `O` flags of a
parameter expansion, so the three can never answer differently
(`glob/one-order-reaches-every-surface`, #1675).

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
