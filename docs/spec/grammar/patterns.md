# Pattern matching

One language with three consumers: pathname expansion, `case` patterns,
and the pattern operators of parameter expansion (`#`, `##`, `%`, `%%`,
`/`). Described once here rather than three times, which is why
`parameter-expansion.md` and `expansion.md` both stop short of it.

Citation: POSIX.1-2024 XCU §2.13. Panel measurements as in `../oracle.md`.

## The language is one thing; the restrictions come from the context

This is the fact to hold on to, because the same pattern gives different
answers in different places and the pattern is not what changed:

| probe | result |
| --- | --- |
| `case .hidden in *)` | **matches** |
| glob `*` in a directory holding `.hidden` and `vis` | `vis` only |
| `case a/b in a*b)` | **matches** |
| glob `*f` where the only `f` is `s/f` | no match |

Unanimous. In **pathname expansion** two restrictions apply:

- No metacharacter matches `/`. Patterns are matched one path component
  at a time, so `*` cannot cross a directory boundary.
- No metacharacter matches a **leading** period of a component. Only
  leading: `*.b` matches `a.b`, and `.hid` is matched by `.*id` because
  the period is written explicitly.

In `case` and in parameter expansion neither restriction exists, because
there is no filesystem and no components — the subject is a string.

An implementation that puts these rules in the matcher rather than in the
caller will be wrong in two places out of three.

## The metacharacters

    *        any string, including empty
    ?        any single character
    [ … ]    a bracket expression

Everything else is literal. There is no alternation and no repetition in
the core language, and no way to ask for a longer or shorter match —
`${x#pat}` and `${x##pat}` choose that by doubling the *operator*, not by
anything inside the pattern.

## Bracket expressions

| form | meaning | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `[abc]` | any of those characters | yes | yes | yes | yes |
| `[a-z]` | a range | yes | yes | yes | yes |
| `[!abc]` | none of those | yes | yes | yes | yes |
| `[^abc]` | none of those | **no** | yes | yes | yes |
| `[[:digit:]]` | a character class | yes | yes | yes | yes |
| `[a-]` | a trailing `-` is literal | yes | yes | yes | yes |

**`!` is the portable negation; `^` is an extension** that dash does not
have — there it is an ordinary character, so `[^abc]` matches a literal
`^`, `a`, `b` or `c`. Silent again: the pattern still matches things,
just not the things intended.

Semantics axis: `BracketCaretNegates` — dash no, bash, ksh93 and zsh
yes. Unanswered in the core, and the `posix` preset says no, because in
a shell pattern the standard has `!` *replace* `^` in the role it plays
in regular expression notation (XCU §2.13.1), which leaves `^` ordinary.

### The character classes

All twelve POSIX class names are implemented, matched over bytes in the
C locale the corpus runs under (POSIX XCU §9.3.5; measured unanimous:
`pat/character-class`, `pat/the-remaining-character-classes`):

    alnum alpha blank cntrl digit graph lower print punct space upper xdigit

The edges that tell the lookalikes apart are measured too
(`pat/classes-that-overlap-and-differ`): a tab is `blank` and `cntrl`
but not `print`, and a space is `blank` and `print` but not `graph` —
the pair an implementation that aliases `print` to `graph` gets wrong.

**Outside ASCII the classes are the locale's**, and this is where the
panel is least uniform (`pat/a-character-class-outside-ascii`).
Measured under `LC_ALL=C.UTF-8`:

| character | bash 5.3, bash 3.2, zsh | ksh93 | dash |
| --- | --- | --- | --- |
| `é` | alpha alnum lower print graph | alpha | — |
| `É` | alpha alnum upper print graph | alpha | — |
| `日` | alpha alnum print graph | alpha | — |
| `·` | punct print graph | — | — |
| non-breaking space | print space | — | — |

This implementation follows the three that agree. ksh93 answering
`alpha` and nothing else is a partial implementation rather than a
different reading — it agrees with the other three on every ASCII
character and on the one class it does implement — so it is recorded
rather than modeled.

Two answers in that table are not the ones Go's `unicode` tables give
for free. `graph` has to *exclude* a space, which `print` keeps, and the
two differ nowhere else. And **`digit` holds nothing outside ASCII**,
though `alnum` holds the same character: POSIX says the digit class is
the digits 0 through 9 in every locale, and `case ٣ in [[:digit:]]` is a
miss in ksh93, zsh and dash — bash 5.3 and 3.2 call it a digit and are
the ones out. That leaves the bash dialect deviating from bash on one
class, which is #956 rather than something settled here.

**A class name nothing defines matches nothing, silently** — no error,
no diagnostic, the arm simply never fires — in dash, bash 5.3, ksh93
and zsh alike (`pat/an-unknown-character-class`). bash 3.2 alone falls
back to reading the whole thing as ordinary bracket characters, so
`[[:bogus:]]` there matches the two characters `b]` and nothing here
intended — its column dates the behavior rather than vetoing it, per
`../core.md`. This implementation answers with the four: an unknown
class can never match, and nothing says so, which is one more of the
silent divergences this document keeps a list of.

## What a "single character" is

`?` matches one character and a bracket matches one character, and
**what a character is comes from the locale**, exactly as it does for
`${#s}` and `${s:off:len}`. Measured 2026-09-05:

| probe, `s=héllo` | `LC_ALL=C` | `LC_ALL=C.UTF-8` |
| --- | --- | --- |
| `case $s in ?????)` | no match anywhere | matches, and not in dash |
| `${s#???}` | `llo` everywhere | `lo`, and `llo` in dash |
| `${s//?/X}` on `日本語` | nine X everywhere | three X, and nine in dash |
| `case é in [é])` | no match anywhere | matches, and not in dash |
| `case ç in [a-é])` | no match anywhere | matches, and not in dash |
| `case é in [[:alpha:]])` | no match anywhere | matches, and not in dash |

The pattern in the first row is five ASCII bytes and the answer still
moves, which is the shape of the whole question: a `?` says nothing
about an encoding and what it *consumes* is one character of the
subject. So the callers ask about the subject as well as the pattern —
and pathname expansion asks about the names in the directory, which are
its subjects.

Semantics axis `MultibyteEncodingIsHonored`, the same one `../semantics.md`
records for a length: this is that axis reached through a second code
path (#899, #905), and nothing here is decided separately.

Three parts of it are not settled by "step a character instead of a
byte", and each was measured on its own:

- **A range is ranked by code point**, not compared as encoded text.
  `[a-é]` has to hold ç, which is between them as a number and is not
  between them byte for byte.
- **A `*` stops between characters and never inside one.** Measured, and
  this is the one corner where the panel splits: with a raw continuation
  byte written into a bracket, `case héllo in *[\251]llo)` is a hit in
  bash 5.3 and a miss in ksh93 and zsh under a UTF-8 locale. This
  implementation follows the two, which is also the only reading
  consistent with `?` consuming a whole character — a matcher cannot
  count characters in one operator and bytes in the operator beside it.
  There is no corpus row, because the probe is a pattern nobody writes
  and a row would pin a permanent disagreement with one column.
- **The character classes**, below.

## Quoting decides whether text is a pattern at all

    p='a*b'
    case 'a*b' in  $p ) …    →  matches, as a pattern
    case 'a*b' in "$p") …    →  matches, as a literal
    case  axb  in a\*b) …    →  no match; the star was escaped

Unanimous, and it is the per-span quoting from `tokenization.md` reaching
all the way into matching. A quoted or escaped metacharacter is an
ordinary character, so the matcher has to be told which characters were
quoted — it cannot be handed a plain string.

That is the second requirement the tree carries for this stage, after
`parameter-expansion.md`'s: a pattern is a *word*, not a string.

## A pattern operand is expanded before any of that

    v=abcd
    echo ${v#$(echo ab)}          →  cd        (all six)
    echo ${v#`echo ab`}           →  cd        (all six)
    v=2bcd; echo ${v#$((1+1))}    →  bcd       (all six)
    v=$'\tx'; echo ${v#$'\t'}     →  x         (all six)
    case ab in $(echo ab))        →  matches   (all six)
    [[ ab == $(echo ab) ]]        →  true      (the five with [[ ]])

Unanimous, in every position that takes a pattern and in every shell that
has the position at all: the operand is a word, so it goes through word
expansion first and what comes back is the pattern. Nothing here is
special to the pattern operators — it is the same expansion any other
word gets — which is exactly why it is easy to leave out and hard to see
missing. An operand handed to the matcher as its own source text
*matches nothing*, and matching nothing is a legal answer: `${v#…}`
returns the subject and the script carries on.

The `pat/an-operand-out-of-…` corpus cases pin each substitution, and
`pat/a-case-arm-out-of-a-command-substitution` and
`pat/a-condition-operand-out-of-a-command-substitution` pin that `case`
and `[[ ]]` are the same rule rather than exceptions to it.

Then, and only then, the quoting rule above applies to what came back —
and with it one axis. Whether a metacharacter that *arrived from an
expansion* is live is `GlobExpansionResults`, and where the expansion
stood does not change the answer:
`v=abcdabcd; echo ${v##$(echo 'a*a')}` is `bcd` in dash, bash and ksh93
and `abcdabcd` in zsh — the same split as `p='a*a'; ${v##$p}` and as
`x='et*'; echo $x`. One axis, observed in three places.

**Process substitution is three questions, not one**, and the panel
splits differently on each. The rules are in `parameter-expansion.md` and
`conditions.md`; in short:

| where | bash | ksh93 | zsh | dash |
| --- | --- | --- | --- | --- |
| a `${…}` operand | a substitution | text | text | text |
| a `case` arm | a substitution | no parse | a substitution | no parse |
| a condition's operand | a substitution | no parse | refused, at 2 | no `[[ ]]` |

Where it *is* a substitution, the path is the pattern — which never
matches anything a script would have written down, and that is the
measured answer rather than a shortcut. Where it is not, the characters
are pattern text: `<(x)` is a `<` followed by the group `(x)`, so it
matches `<x` in the two shells that read a bare group.

What it is never, anywhere, is the text *inside* the substitution. That
reading made `case x in <(x))` match and `${v#<(x)}` trim a bare `x`,
neither of which any column does, and both silently (#902).

## Extended patterns are not core

    ?(…)  *(…)  +(…)  @(…)  !(…)

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `case abc in @(abc\|xyz))` | error | **error** | `at` | no match |
| `case aaa in +(a))` | error | **error** | `plus` | no match |
| `case b in !(a))` | error | **error** | `not` | no match |

ksh93 has them natively — they are a ksh feature — while bash requires
`shopt -s extglob` and zsh `setopt KSH_GLOB`. Only one panel shell accepts
them as written, so they belong to a dialect and not to the core.

Note the shape of the failures: dash and bash report a **syntax error**,
because `(` is an operator where a pattern was expected, while zsh parses
the pattern and simply does not match. Three behaviors again.

Grammar flag: `ExtendedPattern` — core: off; `ksh`: on. bash's narrower
`ExtendedPatternInCondition` turns them on inside `[[ ]]` and nowhere
else, which is where bash reads them.

### A group may stand for no text at all

One repetition of an arm that matches nothing is nothing, so a group with
such an arm may consume no characters — however it is quantified, and
including not at all. Every shell with groups agrees, so this is the
matcher's rule and not a dialect's.

| probe | bash 5.3 | bash 3.2 | bash-as-`sh` | ksh93 |
| --- | --- | --- | --- | --- |
| `case b in @(\|a)b)` | match | match | match | match |
| `case b in @(*)b)` | match | match | match | match |
| `case "" in @(a\|))` | match | match | match | match |
| `case "" in +(\|a))` | match | match | match | match |
| `v=abc; ${v#@(\|a)}` | `abc` | `abc` | `abc` | `abc` |
| `v=abc; ${v##@(\|a)}` | `bc` | `bc` | `bc` | `bc` |

Measured 2026-09-06, `env -i PATH=/usr/bin:/bin` with a scratch `HOME`
and `shopt -s extglob` on a line of its own — on the same line as the
probe it is not yet in force when the line is parsed, which reports the
whole family as a syntax error and is the shape that made a first reading
of this look like a bash/ksh93 split.

The question is "can an arm match nothing", not "is an arm empty": the
`@(*)b` row is the one that separates them, and a rule written the second
way answers it wrong. It is also not the same rule as `?(…)` and `*(…)`
allowing zero repetitions — those allow it whatever the arms are, where
this allows one repetition that happens to consume nothing.

Four of those six rows answered the opposite way here, silently and in
every dialect, because the matcher tried the group against one character
of the subject and upward. `${v#…}` is where the wrong answer is worst: it
trimmed an `a` that the shells leave alone, so the *value* was wrong
rather than a match being missed (#1083).

## A numeric range is one dialect's, and it reaches the lexer

    <->      any number
    <n-m>    a number from n to m
    <n->     from n upward
    <-m>     up to m

zsh alone has them. They match a run of digits whose **value** falls in
the range, so leading zeros belong to the run and not to the number:
`[[ 007 = <1-10> ]]` matches, because 007 is 7.

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `[[ 1 = <-> ]]` | no `[[ ]]`; opens `-` | error | error | `0` |
| `case 42 in <->)` | error | error | error | `num` |
| `echo <->` | redirection | error | opens `-` | the numbered files |

This is the one pattern construct that cannot be left to the matcher.
`<` is a redirection operator everywhere a word may stand, so the *lexer*
has to decide, and the shape is the whole of the disambiguation —
measured 2026-09-05 on zsh 5.9.2, `<` then digits then `-` then digits
then `>` and nothing else. `echo <1` reads the file `1` there;
`echo <a-b>`, `echo <1-2-3>` and `echo <-->` are parse errors. Digits in
front stop being a descriptor when the operator turns out to be a
pattern: `echo 2<->` is one word, not a redirection of descriptor 2.

Once the word has parsed the range is expansion like any other — an
unmatched `<->` gets the same `no matches found` an unmatched `*` gets —
and quoting still decides whether it is a pattern at all: `"<->"` is
four ordinary characters, and so is a `<->` that arrived from an
expansion, since this is also the shell that does not re-read an
expansion's result as a pattern.

Grammar flag: `NumericRangePattern` — core: off; `zsh`: on. The matcher
reads the same flag, the way it reads `PatternAlternation`: what a range
matches is not a grammar question, but which dialects have one is.
Measured: `pat/a-numeric-range-is-any-number`,
`pat/a-numeric-range-has-four-shapes`,
`pat/a-numeric-range-compares-values-not-text`,
`pat/a-quoted-numeric-range-is-four-characters`,
`pat/a-numeric-range-in-a-case-arm`,
`pat/a-numeric-range-against-the-filesystem`.

## Glob qualifiers are one dialect's, and they reach the parser

    *(.)     the regular files among the matches
    *(/)     the directories
    *(@)     the symbolic links, by their own type
    *(^.)    `^` turns the sense of what follows
    *(.,/)   `,` is an or; two qualifiers side by side are an and
    *(N)     a miss is no error and the word is deleted
    *(D)     the hidden names are matched too

zsh alone has them. The list narrows what the pattern in front of it
matched, so it is a **filter over the match set** rather than anything
the matcher does character by character. Measured 2026-09-06 on zsh
5.9.2 against a directory holding `d1/`, `f1`, `f2`, a link `l1` and a
hidden `.dot`.

**The disambiguation is exactly one character.** A group holding a `|`
is the alternation `PatternAlternation` already reads, and a group
without one is a qualifier list:

| written | zsh 5.9.2 |
| --- | --- |
| `echo f(1\|2)` | `f1 f2` — an alternation |
| `echo f(1)` | `unknown file attribute: 1` — a list |
| `echo f*(z\|y)` | an alternation that matched nothing |
| `echo f*(.\|/)` | `bad pattern` — an alternation, badly formed |

Being **last in the word** is a condition and not a convenience:
`echo *(.)x` is `no matches found: *(.)x`, so a group with text after it
is an alternation whatever is in it. And a list makes the field a
pattern on its own — `echo f1(.)` sends a literal name to the
filesystem, where the name alone is no pattern at all.

**A character no qualifier claims is fatal and is named**, which is the
answer the shell itself gives: `echo *(qqq)` is
`unknown file attribute: q`. A space is such a character, and that is
what makes the next part visible.

### The parser has to say which position it is

    echo MY ( x )      two words: `MY` and `( x )`
    ( x )              a subshell running `x`

`(` is an operator wherever a command may begin, so this is the second
pattern construct that cannot be left to the matcher — and
unlike a numeric range it cannot be left to the *lexer* either. **Command
position is the whole of the difference**, measured: `( x )` written
first runs `x` in a subshell, and the same three characters after a word
are one argument, whose group is then read as a qualifier list —
`unknown file attribute:` naming the space inside it. The lexer cannot
tell the two apart on its own, so the parser sets a flag before the token
is read, exactly as it does for a condition and for a pattern operand.

**The group ends the word at a shell operator.** `echo ( a <b )` is
``parse error near `)'`` there, with globbing on *and* off, because the
`<` ended the word and left the `)` with nowhere to go. A `|` is the
exception, a pattern group being allowed to hold an alternation. That is
also what `coproc MY ( cat </dev/null )` runs into, which is why the
complaint names the `)` and not the `(`.

**`setopt no_glob` proves the split is lexical rather than
interpretive.** With globbing off, `echo MY ( x )` *prints* `MY ( x )` —
the same two words, and only what became of the group has changed. So
the grammar half is unconditional under the flag and the qualifier
reading lives in the expansion, after the point where whether a word is
a pattern at all has already been decided. (`set -f` is not that option
in this shell: `set -f; echo *` still lists the directory, and
`set -o noglob` and `setopt no_glob` are the spellings that do not.)

**Where the parentheses came from decides whether they are a group**, the
same way it does for every other metacharacter: `echo "( x )"` is five
characters, `echo *"(.)"` matches nothing, and `p="*(.)"; echo $p` prints
four characters — this being also the shell that does not re-read an
expansion's result as a pattern. `(` and `)` are in the escape set for
that reason.

**An array literal's element is one of those positions**, and it was the
one left out. `files=( (#i)a )` is accepted by that shell and was
``expected ) to close an array assignment`` here, because the assignment
found its closing `)` by *counting* parentheses and a group at the front
of an element opens one that belongs to the word. So does a group on a
later element — `files=( x (#i)a )` — which is a different parser state
and had to be asked separately.

The three parenthesised shapes that stand *after* a pattern were already
right, and that is what says where the boundary was:

    files=( *(-.DN) )                       already parsed
    files=( *~(*/*|.(_backup|git))/* )      already parsed
    files=( *.(zip|tgz) )                   already parsed
    files=( (#i)a )                         did not

Mid-word the lexer folds a group without being told; only at the *front*
of a word does it need to know what position it is in. The fix is the
flag this section is about, set while the elements are read and restored
afterwards — an assignment is read at command position as well as after
a word, and the token after the array is an argument in neither case.

Nothing about matching changes with it: `files=( (#i)a )` parses and the
flag is then an unimplemented one, so the pattern reaches the filesystem
as written and misses. `(#i)` and its neighbours are the `(#q…)` form
listed under "read and not implemented" below (#1053); this was only ever
about the parse (#1149).

Grammar flag: `GlobQualifiers` — core: off; `zsh`: on. The matcher reads
the same flag, the way it reads `PatternAlternation` and
`NumericRangePattern`. Measured:
`pat/a-trailing-group-is-a-list-of-qualifiers`,
`pat/a-qualifier-list-is-not-an-alternation`,
`pat/a-paren-where-an-argument-stands`,
`pat/a-glob-flag-where-an-array-element-begins`,
`pat/a-glob-flag-on-a-later-array-element`,
`pat/a-glob-flag-where-a-loop-item-begins`,
`pat/a-glob-flag-on-a-later-loop-item`,
`pat/a-glob-flag-where-a-select-item-begins`,
`pat/a-glob-flag-where-a-case-arm-begins`,
`pat/a-case-arms-own-paren-in-front-of-a-group`,
`pat/a-case-arms-paren-in-front-of-an-expression`.

**A loop's item and a `case` arm's pattern are the other three
positions**, and they read the flag now too — `for`, `select` and
`foreach` share one item reader, and the parenthesised `for x (…)` list
is the fourth place the same two answers apply:

    for x in (#i)a; do :; done              accepted
    for x in a (#i)b; do :; done            accepted
    select x in (#i)a; do :; done           accepted
    for x ((#i)a); do :; done               *refused* — see below
    case x in (#i)a) :;; esac               accepted
    case x in ((#i)a) :;; esac              accepted
    case x in ((#i)*.zip) :;; esac          accepted

**The `case` arm is the hardest of them, and it splits in two.** The arm
carries an optional `(` of its own, so a group at the front of a pattern
and the arm's own paren are the same character. What tells them apart is
measured, and it is the `#`:

    case x in (a)          the arm's paren, pattern `a`
    case x in ((a|b))      the arm's paren, pattern `(a|b)`
    case x in (#i)a)       *no* arm paren, pattern `(#i)a`
    case x in ((#i)a)      the arm's paren, pattern `(#i)a`

The first two rows and the last differ from the third in nothing else, so
a leading `(` belongs to the pattern exactly when it opens a glob flag —
`Lexer.leadingParenBelongsToTheWord`.

The second half is **not about the flag's spelling at all**, and
`case x in ((a|b))` is the row that says so: two parens where no command
may begin were read as an *arithmetic command*, so the complaint named
something the script had never written. An arm suspends that reading the
way a condition already does — `Lexer.inCaseArm`, and
`case x in ((1))` is the row with no pattern language in it whatsoever.
The suspension belongs to the arm and not to the construct: an arm's
**body** is ordinary commands and `case x in a) ((1));; esac` is an
expression there, which is what a flag left on for the body would have
broken.

That is what `~/.zi/bin/lib/zsh/install.zsh:1580` — `((#i)*.zip)` — was
stopping on. With these positions reading the flag the file parses to
line **2048**, 468 lines further, where a `||` with a line continuation
in front of a block's closing brace is the next thing it wants.

**Two shapes in an arm are still refused**, measured and written down
rather than guessed at: `case x in (a)b)`, where the pattern's group is
followed by more pattern text and the arm's `)` is the second one, and
`case x in (#i*)`, where the flag's own group swallows the arm's paren.
Both are accepted by that shell. Neither is a regression — both were
refused before as well — and neither is derivable from the rule above,
which is why they are named here instead of being answered wrong.

**`;` as an element separator inside an array literal** is the other
thing still outstanding from this family, which that shell and ksh93
accept.

### What is read and not implemented

The type tests `.`, `/` and `@`, the `^` that turns them, the `,` that
unions them, and `N` and `D`. Everything else in that language — the
permission tests `r`, `w` and `x`, `p` and `=` and `%` for the other file
types, the `-` prefix that follows a link before testing, `e` and `+` for
a command's verdict, `o`, `O`, `Y` and `[n,m]` for ordering and counting,
and the `(#q…)` form that needs `extended_glob` — is refused by name with
the shell's own wording rather than answered wrong. `*(x)` here is
`unknown file attribute: x`, where the shell would list the executable
files.

That refusal is what let this set be enumerated exactly, and it is
asserted rather than assumed: see
`TestAnUnknownQualifierIsRefusedByName`.

## Run-time switches over the language

bash lets a script move some of these rules at run time, through `shopt`
— a builtin of bash's alone: dash, ksh93 and zsh all answer it with
"command not found" (measured; the `shopt/` corpus cases record it).
Measured against bash 5.3, each option below does exactly this and
nothing beside it:

| option | measured effect |
| --- | --- |
| `nullglob` | a pattern matching no file expands to **nothing**: `echo zz*zz` prints an empty line, and a command whose every word vanishes runs nothing with status 0 |
| `dotglob` | metacharacters match a leading period; `.` and `..` are still never produced |
| `nocaseglob` | pathname expansion folds case — `a*` finds `Apple` — and `case`/`[[ ]]` stay exact |
| `nocasematch` | `case` and `[[ ]]` fold case — `case A in a)` matches — and pathname expansion and `${x#pat}` stay exact |
| `globstar` | `**` standing alone as a component matches zero or more directory levels: `**/f` finds `f`, `d/f` and `d/e/f`; a trailing `d/**` lists `d/` itself and then everything beneath it; hidden entries are neither listed nor descended into without `dotglob`; a symbolic link is listed and never followed; `a**` and a quoted `**` are ordinary patterns |
| `extglob` | the quantified groups above are read **everywhere**, and read at parse time |

`extglob` is the odd one, because it changes the grammar. bash parses a
line before running any of it, so the option takes effect on the *next*
line: `shopt -s extglob; echo @(x)` on one line is a syntax error and
the same two commands on two lines work, in both directions (measured,
bash 5.3 `-c` with embedded newlines). Inside `[[ ]]` bash reads the
groups whether or not `extglob` is on — which is the
`ExtendedPatternInCondition` flag — and with it on, a group arriving
from an expansion matches as a group in `case` too.

The listing and statuses of the builtin itself: `shopt name` prints the
name padded to twenty columns, a tab, then `on` or `off`, with status 0
only if every named option is on; `-q` is that status with no output;
`-p` prints `shopt -s name` / `shopt -u name`, status as a query; `-s` /
`-u` with no names list what is on / off; an unknown name is
`shopt: name: invalid shell option name`, status 1, and the known names
around it are still switched; `-s` with `-u` is refused, status 1; an
unknown flag prints a usage line, status 2.

`expand_aliases` is the seventh wired name and the only one that is not a
matcher option. It decides whether a word being *parsed* is replaced by
what the alias table holds for it, so the state lives on the runner and
the front end passes the parser a hook that reads it — measured, it is a
switch and not a door: the word expands after `shopt -s`, is a command
not found again after `shopt -u`, and the one-line-late rule `extglob`
has applies to it for the same reason. Two things move it besides the
builtin: the route the program arrived by, which is the dialect's
`syntax.Dialect.ExpandAliases` and sets the base, and **POSIX mode**,
which turns it on for as long as the mode lasts. Leaving the mode
restores the base rather than what was set before entering — measured,
`shopt -s expand_aliases; set -o posix; set +o posix; shopt
expand_aliases` answers `off` in bash 5.3 — which is why the runner keeps
two bits and not one.

**The rest of the table is recognized, not implemented.** bash 5.3 lists
59 names and this table holds all 59 — the same set, and a superset of
bash 3.2's 34 — so nothing here is missing the way zsh's `setopt` names
were (#856). What is missing is behavior: 52 of the 59 are held at the
state this shell is already in, and asking one of them to *move* is
refused out loud with `shopt: name: not implemented`, status 1. 46 refuse
`-s` and six — `extquote`, `globasciiranges`, `globskipdots`,
`interactive_comments`, `promptvars` and `sourcepath` — refuse `-u`,
because the behavior they name is simply how this shell works. Asking for
the state already held is granted in both directions, which is the same
bargain `set +o posix` strikes.

These are run-time states rather than semantics axes, which is why the
core holds them as `interp.MatchOption` values and only the bash dialect
maps names onto them. `failglob` is recorded here and deliberately not
implemented: its miss aborts the rest of the current *line* and then
carries on (measured: `shopt -s failglob` then `echo zz*zz; echo after`
on one line prints neither, and `echo after` on the next line prints),
which is a control-flow shape nothing else needs yet.

## What this does not cover

Collating symbols and equivalence classes (`[[.a.]]`, `[[=a=]]`), which
POSIX defines and which no consumer in this project has needed. Recorded
so their absence is a decision rather than an oversight.
