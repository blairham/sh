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

### A backslash escapes every character, or only a metacharacter

A backslash in a pattern marks the next character as ordinary. What the
panel disagrees about is what happens when the next character was
ordinary already.

**Asking it takes care, because quote removal answers first.** An escape
written in the source is spent before the matcher ever sees it, so a
`case` pattern spelled `bet\a` is the pattern `beta` in all six shells
and says nothing about this. What does ask it is a pattern the matcher
receives with a *raw* backslash in it — a substituted one, in the five
shells that match the result of an expansion, and the same two lines
under `setopt globsubst` in the one that does not:

    p='bet\a'; case beta in $p) … ;; esac

| shell | |
| --- | --- |
| bash 5.3, bash as `sh`, bash 3.2, dash, ksh93 | matches — the backslash is spent |
| zsh 5.9.2 | does not match; `bet\a` matches the five characters |

`Semantics.PatternEscapeReaches` is that answer: the empty string means
every character, and a set means a backslash before anything outside it
is a literal backslash with the character after it standing on its own.
zsh names its pattern metacharacters, measured character by character:

    escaped:      - = ! * ? [ ] ( ) | ^ ~ # < >
    not escaped:  letters, digits, _ . / + : % & @ , " ' space { } $

The backslash itself is in the set — `x\\y` matches one backslash there
and not two — which is what keeps a glob-escaped value literal.

The reachable route in the zsh dialect is a subscript search operand,
which is not a quoting context, so a backslash written in one arrives
raw: `a=('bet\a' beta); ${a[(r)bet\a]}` is the first element there and
would be the second under the other answer. See
`parameter-expansion.md`.

One row is deliberately not modeled. dash escapes every character **but
`^`**: a pattern `x\^y` does not match `x^y` there, where `x\.y` matches
`x.y`. One character of one shell, recorded in the corpus and filed
rather than given a value.

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

**"Unknown" is a per-dialect question, and the roster is what differs.**
Measured 2026-09-10, `[[ $c = [[:NAME:]] ]]` a character at a time:

| name | zsh | bash 5.3 | bash 3.2 | ksh93 | dash | what it holds |
|---|---|---|---|---|---|---|
| `ascii` | yes | yes | yes | no | no | one byte below 0x80 |
| `IDENT` | yes | no | no | no | no | `alnum` and `_` |
| `IFS` | yes | no | no | no | no | the separators `$IFS` names |
| `IFSSPACE` | yes | no | no | no | no | the whitespace among them |
| `WORD` | yes | no | no | no | no | `alnum` and `$WORDCHARS` |
| `INCOMPLETE` | yes | no | no | no | no | a byte that could begin a character |
| `INVALID` | yes | no | no | no | no | a byte that could not |

`ascii` holds `a` and holds neither `é` nor `日` in all three columns that
have it. `IDENT` is the union of `alnum` and the underscore, and it follows
`alnum` outside ASCII — `é`, `日` and `٣` are all in it. `IFS` and `WORD`
read shell state as it stands: `IFS=':x'` puts those two characters in the
first and takes the space out, and `WORDCHARS='@%'` puts `@` and `%` in the
second and takes `-` and `.` out. `INCOMPLETE` is a byte from 0xC2 to 0xF4,
which is where a character could have started; `INVALID` is 0x80 to 0xC1 and
0xF5 upward, which is where none could.

**The names are case-sensitive**: `[[:ident:]]` and `[[:ASCII:]]` match
nothing where `[[:IDENT:]]` and `[[:ascii:]]` match.

That is `Semantics.PatternClasses`, a space-separated roster and not a flag
per name, because nothing here is in dispute — no shell disagrees with
another about what `ascii` means, they differ only over whether the name
exists. Empty is the twelve and nothing else, which is ksh93's answer and
dash's.

`[[:IDENT:]]` is the row that was reachable and wrong. `gitstatus` guards
its argument with `[[ $name != [[:IDENT:]]## ]]`, so a missing name made the
guard fire on every well-formed argument, silently and at status 0 — the
plugin then reported a bad argument and nothing of this shell's said why
(#1721).

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

### A group is not a second language

An expansion written **inside** a parenthesised group is the same expansion
it is anywhere else in a word, and its *value* is the pattern text.
Measured 2026-09-08 on zsh 5.9.2, the only panel shell with bare groups,
and on bash 5.3.15 and ksh93 through the quantified spelling:

    L=wait; [[ wait == ($L) ]]        matches
    L=wait; [[ '$L' == ($L) ]]        does not          ← the discriminator
    L=wait; [[ wait == (${L}) ]]      matches
    L=wait; [[ wait == ($(echo wait)) ]]   matches
    L=wait; [[ wait == @($L) ]]       matches in bash and ksh93
    L=wait; [[ '$L' == @($L) ]]       does not, in either

The second row is the whole of it. A group handed to the matcher as its
own source text matches the two characters `$L`, which is a legal answer:
status 1, nothing said, and a pattern that did not contain what the script
wrote. That was this implementation's answer until #1331, and it is the
single reason a real plugin manager loaded nothing at all — the parser
every one of its commands goes through is one match of this shape, so no
option was ever recognized and every one of them was passed on as a
command name.

The cause was a *copy*. The group scanner grew out of the word scanner and
kept its own list of the constructs a word may hold, with the quoting forms
in it and none of the expansions — so `("b")` was read (#1248) and `($L)`
was not. There is one list now and both scanners read it; see
`Lexer.substitutionSpans`.

Process substitution is the one form the list leaves out, because `<(` and
`>(` are the only members whose first byte is also one of the four
operators that end a word inside a group, and there the operator wins:

    [[ x == (a<(echo x)b) ]]     process substitution … cannot be used here
    print -r -- (a<(echo x)b)    number expected

Whether the metacharacters in the value are then *live* is not a second
question. It is `GlobExpansionResults`, asked where every other
expansion's is:

    L='a|b'; [[ a == ($L) ]]      no match in zsh
    L='a|b'; [[ 'a|b' == ($L) ]]  matches in zsh — the `|` was a character
    L='a|b'; [[ a == (${~L}) ]]   matches: the flag overrides the axis
    L='a|b'; [[ a == @($L) ]]     matches in bash and ksh93, which glob it

The `|` needed one addition to make that true. It is the one
metacharacter a *group* introduces, so a value can only carry a live one
where a group came from somewhere else — which nothing could reach while
an expansion inside a group was still text. Measured in a directory
holding `ice.zsh`, `other.zsh` and one file named `ice|other.zsh`:

    L='ice|other'; print -r -- ($L).zsh      →  ice|other.zsh
    L='ice|other'; print -r -- (${~L}).zsh   →  ice.zsh other.zsh

**One divergence is recorded and not implemented.** ksh93 globs a `*` out
of a value and refuses to read an extended group out of one, where bash
reads both:

    L='ice|other'; echo @($L).zsh      ksh93: @(ice|other).zsh   bash: ice.zsh other.zsh
    L='@(ice|other)'; echo $L.zsh      ksh93: @(ice|other).zsh   bash: ice.zsh other.zsh

So `GlobExpansionResults` has a third value there — yes for the ordinary
metacharacters, no for the extended constructs — and this implementation
gives bash's answer in ksh93's dialect on both rows. The second of them
answered that way before #1331 and the first now joins it.

### A live bar outside a group is an alternation in one dialect

The same `|`, one level out. In the shell with bare groups a bar that
arrived from a value is an alternation at the **top** of a pattern, not
only inside `( … )` — and again only where it arrived live, since the
written spelling is a parse error:

    [[ a = a|b ]]                      parse error near `|' — zsh 5.9.2 and here

Measured on zsh 5.9.2, 2026-09-08 and again 2026-09-11, each probe in a
script file of its own under `env -i`:

    L='a|b'; [[ a = ${~L} ]]                    matches
    L='a|b'; [[ b = ${~L} ]]                    matches
    L='a|b'; [[ 'a|b' = ${~L} ]]                does not   ← the discriminator
    L='a|b'; case a in ${~L}) …                 takes the arm
    setopt globsubst; L='a|b'; [[ a = $L ]]     matches
    L='a|b'; print -l -- ${~L}                  lists the files `a` and `b`

The third row is the one that separates the two readings: under "the bar is
a character" the value matches its own text, which is what this
implementation answered until #1497 — `[[ a = ${~L} ]]` was a quiet false
while the identical value inside a group was an alternation, because #1331
fixed the group and the two had been measured together.

Three boundaries, each measured:

- **A bracket expression is stepped over.** `L='[a|b]'` matches `a` and
  matches `|`, so the bar between two members is not a split. A walker that
  counted only parentheses answers no to the second.
- **An arm may be empty.** `L='a|'` matches `a` and matches the empty
  string, and `L='|'` matches the empty string.
- **Against the filesystem the split is per component**, because that is
  where a glob matches: `Q='d1|d2'; print -l -- ${~Q}/*` lists `d1/x` and
  `d2/y`, while `P='d1/x|d2/y'` matches no file at all — an arm holding a
  `/` cannot cross a component. In a condition, where there are no
  components, the same value matches the whole string.

A top-level bar also makes a field a **pattern by itself**: `L='a|b'` with
no other metacharacter in it is generated against the filesystem in that
shell, where in every dialect whose bar means something only inside a group
a field holding one is an ordinary word. That is why the bar is composed in
beside `hasUnescapedMeta` rather than counted by it — the same composition
`resultReadsAsPattern` already makes one level up.

`Dialect.PatternTopLevelAlternation` is the answer, and `matchTopLevel` is
where the split is made: at the one entry point every surface's match goes
through, so a condition, a `case`, a trim and a glob all take it.

### Which arm a longest prefix trim takes

`${x##pat}` is spelled "the longest match", and the panel does not agree on
what that means once the pattern holds an alternation whose arms take
different lengths. Measured 2026-09-11, `x=abc`, each probe a `-c` of its
own — the spelling differs by column because the group does, and the answer
does not:

    ${x##(a|ab)}     zsh 5.9.2   bc        the arm written first
    ${x##(ab|a)}     zsh 5.9.2   c
    ${x##@(a|ab)}    bash 5.3    c         the longest arm
    ${x##@(ab|a)}    bash 5.3    c
    ${x##@(a|ab)}    bash 3.2    c
    ${x##@(a|ab)}    ksh93u+     c

The shell that takes the written arm is not taking the *shortest* one, and
these are the rows that say so:

    ${x##(a*|ab)}    zsh 5.9.2   (empty)   the arm still takes as much as it can
    ${x##(a|ab)c}    zsh 5.9.2   (empty)   a later arm, where the rest needs it
    ${x##(|a)}       zsh 5.9.2   abc       an empty arm is an arm
    ${x##*(a|ab)}    zsh 5.9.2   bc        and the order survives a `*` in front

So it is a search order — the arms tried left to right, the first that lets
the whole pattern match kept, and the greediest reading taken inside it —
rather than a second rule about length.

**The other three trims ask nothing**, which is why the axis is worded for
this one. The single `#` takes the shortest match in every column whichever
arm came first, and both suffix trims take the longest:

    ${x#(a|ab)}      zsh 5.9.2   bc
    ${x#(ab|a)}      zsh 5.9.2   bc
    ${x%%(|bc)}      zsh 5.9.2   a         an empty first arm does not stop it
    ${x%(c|bc)}      zsh 5.9.2   ab

`Semantics.LongestPrefixTrimTakesTheWrittenArm` is the answer, and it is
asked **only where the two readings land in different places**: `(ab|a)` —
the arms in decreasing length — has two readings that agree, and a pattern
with no alternation has one. A live top-level bar is read the same way as a
written group, measured through `${~L}`.

The same order decides what a `(#b)` group reports and what the `(M)` flag
keeps, because they are the one match seen from the other side:
`${(M)x##(a|ab)}` is `a` where the trim leaves `bc`, and `${x##(#b)(a|ab)}`
puts `a` in `$match[1]`. The matcher already preferred a written arm for
what it reports; what it could not do was say so through the yes/no question
a trim's candidate search asks it, which is why the arm order is recovered
by resolving the alternations to one arm each and asking the ordinary
question of each resolved pattern, in order (#1918).

Two shapes are deliberately left on the length reading, because an arm
chosen once does not speak for them: a *quantified* group, `@(a|b)` and its
four relatives, whose arm may be taken more than once or not at all, and a
group a closure repeats, `(a|b)#` or `(a|b)(#c2,3)`.

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

### A quantified group reaches the filesystem

`echo @(a|b)` lists the files, and the same flag decides it. Measured
2026-09-07 in a directory holding `a` and `b`, on ksh93u+ 2012-08-01 and
on bash 5.3.15 and 3.2.57 under `shopt -s extglob`:

| written | ksh93 | bash 5.3 | bash 3.2 | zsh |
| --- | --- | --- | --- | --- |
| `echo @(a\|b)` | `a b` | `a b` | `a b` | no match |
| `echo +(a\|b)` | `a b` | `a b` | `a b` | no match |
| `echo !(a)` | `b` | `b` | `b` | `number expected` |
| `echo @a` | `@a` | `@a` | `@a` | `@a` |

zsh reads the `@` as an ordinary character in front of a bare group of
its own, so its group matches nothing and the miss is fatal there — the
same reading its `case` column gives above. The last row is the one that
keeps the rule from being "an `@` is a metacharacter": a quantifier with
no `(` behind it is an ordinary character everywhere.

The predicate that decides whether a field is a pattern at all counts
`*`, `?`, a closed `[`, a range where the dialect has one and a bare `(`
where the dialect has those. It did **not** count a quantified group, so
a field holding one and nothing else never reached the walk — and
`*(a|b)` and `?(a|b)` worked all along and hid it, their quantifier being
a metacharacter in its own right. That is why the gap showed up as three
of the five quantifiers rather than as the construct (#1042). Measured:
`pat/an-extended-pattern-reaches-the-filesystem`.

The `(` has to be the byte **directly after** the quantifier. `@a(b)` is
``syntax error at line 1: `(' unexpected`` in ksh93, so no source can put
such a field in front of the matcher — but a value can, and the predicate
answers about the characters rather than about where they came from.

The same predicate is read a second time, at the **expansion** sites: a
field with a metacharacter in it is escaped where the dialect does not
glob the result of an expansion, and one with none is left alone because
it has nothing to protect. No preset combines the two — both shells with
quantified groups glob such a result, and the shell that does not has no
quantified groups — so that half is asserted against a dialect assembled
for it rather than against a shell. The two questions are independent and
the sites read both.

### The leading-period rule and a group's own period

Whether the rule looks *inside* a group is a **second axis**, and it
splits a shell rather than two shells. Measured the same day in a
directory holding `a` and `.hid`:

| written | bash 5.3 | bash 3.2 | ksh93 |
| --- | --- | --- | --- |
| `echo @(.hid)` | `.hid` | `@(.hid)` | `.hid` |
| `echo @(a\|.hid)` | `.hid a` | `a` | `.hid a` |
| `echo *(.hid)` | `.hid` | `*(.hid)` | `.hid` |
| `echo .@(hid)` | `.hid` | `.hid` | `.hid` |

So bash 5.3 and ksh93 find the hidden name through *any* alternative of
the group, and bash 3.2 through none of them. The last row, whose period
stands outside the group, is what every column agrees on — so the
disagreement is about where the literal period has to be and not about
whether the rule applies at all.

A version difference inside one preset is the shape `${x^^}` has and no
grammar flag answers it, so this is written down rather than guessed at.
**This shell answers as bash 3.2 does.** Measured:
`pat/a-quantifier-does-not-suspend-the-leading-period`.

And `*(.)` is a separate row again: `. ..` in ksh93 and `*(.)` in bash
5.3 and 3.2 alike, the pattern's leading character being the `*` and
`.`/`..` not being entries this walk offers. #1042 was filed expecting
both bashes to answer `. ..`; they do not, and the quantifier is not
exempt from the leading-period rule in either of them. That divergence is
ksh93's alone and is what `pat/a-trailing-group-is-a-list-of-qualifiers`
records in its column.

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
    *(p)     the FIFOs
    *(%)     the device nodes, block and character alike
    *(x)     the ones their owner may execute — and `r` and `w` beside it
    *(E)     the ones their group may execute — with `A` read and `I` write
    *(X)     the ones the world may execute — with `R` read and `W` write
    *(s)     set-user-ID; `S` is set-group-ID and `t` is the sticky bit
    *(f755)  the ones at that mode — `f` is access rights, and takes
             an argument in either of two spellings
    *(u0)    the ones owned by that user; `g` is the group, and `U`
             and `G` are the same two questions about this process
    *(-.)    `-` is not an attribute: it toggles whether what follows
             asks about a link or about what the link points at
    *(^.)    `^` turns the sense of what follows
    *(.,/)   `,` is an or; two qualifiers side by side are an and
    *(N)     a miss is no error and the word is deleted
    *(D)     the hidden names are matched too
    *(.:t)   a `:` ends the qualifiers and opens a list of modifiers,
             applied to every name the pattern reported

zsh alone has them. The list narrows what the pattern in front of it
matched, so it is a **filter over the match set** rather than anything
the matcher does character by character. Measured 2026-09-06 on zsh
5.9.2 against a directory holding `d1/`, `f1`, `f2`, a link `l1` and a
hidden `.dot`, and the permission and mode letters on 2026-09-07 against
the same directory with `x` made executable and a second one holding a
0644 `plain`, a 4755 `suid`, a sticky directory and a FIFO.

**The three permission triples are what the run had to establish**, and
no naming convention predicts them: owner is `r w x`, group is `A I E`
and world is `R W X`. The fixture's modes were 0644 and 0755, so `A`
listing every name and `I` listing none puts that pair on the group
triple, and `E` and `X` agreeing on the 0755 names are separated only by
`W` — world write — being empty where `w` is not.

**A permission letter is a question about the file and not about this
process.** It reads the mode from the same lstat the type tests read, so
a symbolic link answers for itself: `*(x)` lists `l1`, whose target `f1`
is 0600. zsh has separate letters for the effective user's own access
and they are not these; see below.

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

**The whole reading is behind an option**, `BARE_GLOB_QUAL`, on by
default. With it off a trailing group is pattern text again and nothing
about the *word* changes — it is still one word and still a group.
Measured on zsh 5.9.2, 2026-09-10, in a directory holding `AGENTS.md`,
`CLA.md`, `xN` and `xy`:

| written | `BARE_GLOB_QUAL` | `NO_BARE_GLOB_QUAL` |
| --- | --- | --- |
| `echo *.md(N)` | `AGENTS.md CLA.md` | `no matches found: *.md(N)`, 1 |
| `echo x(N)` | nothing — no file `x` | `xN` |
| `echo x(N\|y)` | `xN xy` — an alternation either way | `xN xy` |
| `echo *(#q.)` | the regular files | the regular files |
| `[[ xN == x(N) ]]` | true | true |

Three things to read out of it. The `(#q…)` spelling is **not** gated,
which is what it is for — it is the list wearing a spelling that works
where the bare one has been turned off. A condition is unaffected,
because pathname expansion is the only place a qualifier list is read at
all. And `NO_EXTENDED_GLOB` does not turn it off: `echo *.md(N)` with
only that name unset still expands the qualifier, so the two options in
the agent preamble
`{ shopt -u extglob || setopt NO_EXTENDED_GLOB NO_BARE_GLOB_QUAL; }`
are two separate questions and only this one decides the qualifier.

That preamble is why the option is not a corner a script has to opt
into: it runs in front of **every** command an agent harness issues, so
the off state is the state every such command is expanded in. Being more
permissive than asked is the wrong direction there — the tool turns the
reading off precisely so a generated pattern cannot be reinterpreted
(#1729). The core holds it as `interp.TrailingGroupIsPartOfThePattern`,
named for the state the name turns *off*, because a `MatchOption` starts
at zero and the qualifier reading is the default.

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

**And it is a rule about any group in a word, not about this construct
and not about where the group sits.** A *pattern operand's* group ends at
the same four characters, which needs only bare groups and no qualifiers
at all — measured on zsh 5.9.2, 2026-09-07, each probe in a script file
of its own so the first refusal does not hide the rest:

| written | zsh 5.9.2 |
| --- | --- |
| `[[ $k == (a<b) ]]` | ``parse error near `<'`` |
| `[[ $k == (a>b) ]]` | ``parse error near `>'`` |
| `[[ $k == (a;b) ]]` | ``parse error near `;'`` |
| `[[ $k == (a&b) ]]` | ``parse error near `&'`` |
| `[[ $k == (a\|b) ]]` | matches — the `\|` is the group's |
| `[[ $k == a(b<c) ]]` | ``parse error near `<'`` |
| `[[ $k == a(b;c) ]]` | ``parse error near `;'`` |
| `[[ $k == a(b\|c) ]]` | matches |
| `[[ $k == a(<0-9>) ]]` | matches — the range's `<` never was one |

The last four rows are the correction. This was written as a rule about
a *route* into the scanner and then as a rule about a group **starting**
a word; each was the shape of where the group happened to be measured,
and neither survives asking the middle of a word. Measured:
`cond/a-mid-word-groups-operator-ends-the-word`,
`cond/a-mid-word-groups-operator-may-be-quoted`.

**The four end the word only where they are unquoted.** A backslash or
either quote takes the operator away and the group carries the byte,
which is what quoting does everywhere else in a word:

| written | zsh 5.9.2 |
| --- | --- |
| `[[ '<x' == (\<)* ]]` | matches |
| `[[ '<x' == ("<")* ]]` | matches |
| `[[ '<x' == ('<')* ]]` | matches |
| `[[ 'a;b' == (a\;b) ]]` | matches |
| `[[ 'a)b' == (a\)b) ]]` | matches — the group's own delimiter too |

And the quoting reaches the *matcher*, not only the scan: quoted text
inside a group is literal there as it is anywhere else, so
`[[ b == ("b") ]]` matches and `[[ axb == (a"*"b) ]]` does not. That
half is not one shell's — bash's quantified group is the same body, and
`[[ b == @("b") ]]` matches in bash 5.3.15. Measured:
`cond/a-groups-operator-may-be-quoted`,
`cond/a-groups-quoted-text-is-literal`,
`pat/quoted-text-in-an-extended-pattern-group`,
`pat/a-quoted-operator-in-a-group-reaches-every-route`.

A **regular expression's** operand is the other answer and owns its
operators: `[[ 'a<b' =~ (a<b) ]]` and `[[ 'a;b' =~ (a;b) ]]` both match
in bash 5.3, bash 3.2, bash-as-`sh` and ksh93, and `[[ ab =~ (a<b) ]]`
does not — so those characters are regex text there rather than a
redirection or a terminator. zsh alone refuses the `<` while parsing.

So the two operands differ in nothing a reader can see and the shells
still separate them, which is why both are written down. Measured:
`cond/a-pattern-operands-group-ends-at-an-operator`,
`cond/a-regex-operands-group-keeps-its-operators`.

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
of a word does it need to know what position it is in.

**Argument position has to be given back, and only a *run* can see it.**
The flag is set before a list's first word is read and restored after, and
the token behind the list is read while it is still on — so a `(` there
would be folded into a word. Every spelling of that **parses** either way:
a folded group is a perfectly good word, and `-n` cannot tell the two
apart. `for x in a; do ( echo hi ); done` is the row, and it prints
`unknown file attribute:` instead of `hi` when the flag leaks. Every
parse-only assertion passed a mutant that leaked it (#1161). The fix is the
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
    for x (a (#i)b); do :; done             accepted
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

**And the parenthesised list's own opening paren**, which is a lexing
question rather than a word-position one: `for x ((#i)a)` reaches the
lexer as `((`, is taken for an arithmetic command, and never gets as far
as being a list at all. A *later* element there does read the flag —
`for x (a (#i)b)` — because its paren stands after a word. The suspension
a `case` arm gets would fix it, and it wants a position of its own to be
told about; nothing measured needs it, so it is named here instead.

**`;` as an element separator inside an array literal** is the other
thing still outstanding from this family, which that shell and ksh93
accept.

### Access rights, ownership, and following a link

`f`, `u` and `g` take an **argument**, which is why the list is scanned
by index rather than one character at a time — everything else in it is a
single letter. Measured on zsh 5.9.2, 2026-09-10, against one regular
file per mode: 0000, 0600, 0644, 0664, 0666, 0700, 0755, 1777 and 4755.

    *(f0666)      the file at exactly that mode
    *(f755)       0755 *and* 4755 — three digits compare three digits
    *(f0755)      0755 alone — a fourth digit compares the fourth
    *(f+022)      every bit of 022 set
    *(f-022)      no bit of 022 set
    *(f70?)       owner 7, group 0, and `?` asks nothing of the third
    *(f:g+w:)     the chmod-style spelling, which is what the
                  completion system writes
    *(f:u=rw:)    the owner's bits are exactly rw
    *(f:g+w,o+w:) both clauses hold — a comma inside is an *and*

**`+` is every bit and `-` is no bit.** The alternative reading of `-`
— "not all of them" — agrees with it on every number a file either has
whole or lacks whole, so the probe that separates them is a partial
overlap: `*(f-0700)` lists nothing where a 0666 file is present, and 0666
has two of those three bits.

**The mask is as wide as the number is long**, which is the finding
`f755` listing a 4755 file forced; `?` is the same rule spelled per
digit, and is the only way to leave a *middle* digit out.

**A sub-spec that is a number ends the spec.** `f:u+w,+022:` is read and
`f:+022,u+w:`, `f:755,644:` and `f:u+w,+022,g+w:` are all `invalid mode
specification` — so a number may be the last sub-spec or the only one,
never a middle one.

**The operator may be left out, and leaving it out is `=`** — the same
default the number form has. `f:u:` is the files whose owner bits are all
clear, exactly as `f:u=:` is, and `f:g:` and `f:a:` say the same of their
own classes. A clause with no *class* is a different thing and is
refused: `f:x+w:` is `invalid mode specification`, and `f:+w:` never
reaches the clause reader at all — an operator at the front is the number
form, and `+w` is a number with a `w` left over.

**`s` and `t` are the class's own bit rather than a fourth permission**:
`u+s` is set-user-ID, `g+s` is set-group-ID, `o+t` is the sticky bit, and
`u+t`, `g+t` and `o+s` ask for a bit that class does not have and hold of
everything. **`=` compares the class's whole triple, that bit included** —
`f:u=rwx:` leaves out a 4755 file and `f:u=rwxs:` is what finds it.

**A delimited ownership argument is a *name*, always.** `u:foo:`,
`u[foo]` and `u{foo}` are the same argument, and `u[501]` is
`unknown username '501'` rather than the uid — so the numeric and named
forms do not overlap.

**Any character delimits, an operator included**, which is worth writing
down because the evidence reads exactly like a rule that it does not:
`u+500`, `u-502` and `u=501` are all
`missing delimiter for 'u' glob qualifier`, and `u+bhamilton+` is a list
of files. The complaint is about the missing *partner* and not about the
character — `u+5+` is `unknown username '5'`. The lookup happens while the *list* is read:
`zz*(Nu:nobody-here:)` names the user in a directory where the pattern
matches nothing at all. The two failures are two different sentences, and
that is the shell's doing rather than ours: `unknown username 'x'` names
the name and `unknown group` does not.

**`-` is a toggle and not a flag**, measured with a fixture holding a
regular file, a link to it, a link to a directory and a link to nothing:

| written | zsh 5.9.2 |
| --- | --- |
| `*(N.)` | `f1` |
| `*(N-.)` | `f1 la` — the link to a regular file, followed |
| `*(N--.)` | `f1` again — a second `-` turns it back off |
| `*(N.-@)` | nothing — it applies to what follows it, not to `.` |
| `*(N-@)` | `dangle` alone — see below |
| `*(N-@,@)` | `dangle la ld` — a `,` reads it again from nothing |

**A link whose target cannot be stat'd is treated as a file in its own
right**, which is what `*(N-@)` says: following the two that resolve
reaches a regular file and a directory, and following the dangling one
reaches nothing, so it answers as the link it is.

### `l` is a number, and the shape every numeric qualifier takes

`l` is the file's **link count**, and its argument is a number with an
optional comparison in front of it. Measured on zsh 5.9.2, 2026-09-11,
against a directory holding `dir1/` (2 links), `dir2/` with one
subdirectory (3), `f1` hard-linked to `f1b` (2 each), `g1` (1) and a
symbolic link `lnk` (1):

    *(l1)      g1 lnk                exactly one
    *(l2)      dir1 f1 f1b           exactly two
    *(l+1)     dir1 dir2 f1 f1b      more than one
    *(l-3)     everything but dir2   fewer than three
    *(l0)      no matches found      nothing has none
    *(l+0)     everything            and everything has some

**Neither comparison includes the number itself**: `l-1` matches nothing
where `l1` matches two names.

**The digits end the argument and the next character is a qualifier
again.** `*(l1x)` is the one-link name whose owner may execute it, and
`*(l1.5)` is `unknown file attribute: 5` — the same rule seen through a
character nothing claims.

**A number wider than the type is a count no file can carry rather than
bad input**: `l99999999999999999999` is `no matches found` and
`l-99999999999999999999` lists everything. That is the answer the
ownership argument already gives a uid nothing holds.

**Only an argument with no digits at all is refused, and the sentence is
`number expected`** — `l`, `l+`, `l-`, `lx` and `l 1` alike, which is a
different complaint from `unknown file attribute` and says the letter
was recognized and its argument was not.

**The two spellings of a minus do not collide.** The `-` that follows a
symbolic link is written before the letter and the comparison after it,
so `*(-l1)` asks the target and `*(l-1)` asks for fewer than one. Both
were measured against the same fixture: `lnk` has one link of its own
and points at a name with two, and `*(-l1)` leaves it out.

Where the letter was met: powerlevel10k's `_p9k_prompt_length` writes
`${(%):-$1%$y(l.1.0)}`, and a reader that globbed that text reached this
qualifier. #1695 stopped that expansion being globbed at all, so a
startup no longer arrives here — but the same text written without the
flag still names a number in zsh, and named a file attribute here.

### A list may end in modifiers

A `:` in the list ends the qualifiers and opens the same history-style
modifiers `${x:t}` takes, applied to every name the pattern reported.
Measured against `sub/x.txt` and `sub/y.md`:

    */*(N:t)     x.txt y.md      the tail of each name
    */*(N:t:r)   x y             a chain, applied left to right
    */*(N:e)     md txt          and the answer is *re-sorted*
    */*(N:h1)    sub sub         a count reaches the letter here too
    */*(N:s/x/Q/)                and so does a substitution

**Everything after the first `:` is modifier text**: `*(N:t.)` is the
tails of every name and the `.` asks nothing, where `*(N.:t)` is the
tails of the regular files. **Re-sorting is the measurement's own
finding** — `:e` answers `md txt` for names that arrived in the order
`x.txt y.md`.

**An unrecognized modifier stops the chain and says nothing**, which is
not what the parameter surface does with the same text: `*(N:z)` lists
the names unchanged, `*(N:zt)` does not apply the `t` behind the `z`, and
`*(N:t:X:u)` applies the `t` and not the `u`. Text after a letter *inside*
one segment is ignored rather than being the failure it is in `${x:ha}`:
`*(N:tr)` is the tail alone. A substitution is the exception to the
silence and it is the shell's own — `*(N:s)` is `bad substitution` and
`*(N:s//Q/)` is `no previous substitution`, both fatal.

This is the spelling zi autoloads a plugin's functions with —
`functions/^([_.]*|prompt_*_setup|README*)(D-.N:t)`.

**None of this is a `Semantics` axis.** Every other shell in the panel
refuses `*(` while parsing, so there is no disagreement to answer: the
whole language lives behind the `GlobQualifiers` grammar flag, and adding
to it adds no conditional anywhere.

### What is read and not implemented

The type tests `.`, `/`, `@`, `p` and `%`, the nine permission letters,
`s`, `S` and `t` for the bits outside the permission triples, `f` and its
mode argument, `u`, `g`, `U` and `G` for ownership, `l` and its numeric
argument for the link count, the `-` that follows a link before testing,
the `^` that turns any of them, the `,` that unions them, `N` and `D`,
and the `:` that opens a modifier list.

Everything else in that language — `=` for a socket, the `%b` and `%c`
spellings that separate the two kinds of device, `e` and `+` for a
command's verdict, `d` for a device, `o`, `O`, `Y` and `[n,m]` for
ordering and counting, `a`, `m` and `c` for times, `L` for a size, and
the `(#q…)` form that needs `extended_glob` — is refused by name with the
shell's own wording rather than answered wrong. `*(L+1)` here is
`unknown file attribute: L`, where the shell would list the names above
that size.

`S` and `%` are read and unproven in the positive direction, which is a
fact about what a test process may create rather than about the shell:
both need privileges. What is asserted of them is that they are
*claimed* — the miss names the whole word, not the letter — which is the
half that separates a read letter from an unknown one.

That refusal is what let this set be enumerated exactly, and it is
asserted rather than assumed: see
`TestAnUnknownQualifierIsRefusedByName`, `TestTheLinkCountQualifier`,
`TestALinkCountWithNoNumberIsRefused`,
`TestThePermissionQualifiersReadTheMode`,
`TestTheModeBitQualifiers`, `TestTheAccessRightsQualifier`,
`TestAnUnreadableModeSpecIsRefused`, `TestTheOwnershipQualifiers`,
`TestAnOwnerMayBeNamed`, `TestTheFollowToggleAsksAboutTheTarget` and
`TestAQualifierListMayEndInModifiers`. Measured:
`pat/a-qualifier-list-reads-the-access-rights`,
`pat/a-qualifier-list-reads-the-owner`,
`pat/a-qualifier-list-reads-the-link-count`,
`pat/a-qualifier-list-may-follow-a-link`,
`pat/a-qualifier-list-may-end-in-modifiers`.

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
| `nocasematch` | `case`, `[[ ]]` and the **substitution** operators of parameter expansion fold case — `case A in a)` matches and `v=ABC; ${v//b/X}` is `AXC` — while pathname expansion, the trims `${x#pat}` and `${x%pat}`, and the case-change operator `${x^^pat}` stay exact |
| `globstar` | `**` standing alone as a component matches zero or more directory levels: `**/f` finds `f`, `d/f` and `d/e/f`; a trailing `d/**` lists `d/` itself and then everything beneath it; hidden entries are neither listed nor descended into without `dotglob`; a symbolic link is listed and never followed; `a**` and a quoted `**` are ordinary patterns |
| `extglob` | the quantified groups above are read **everywhere**, and read at parse time |

The seam `nocasematch` draws is *inside* `${ }` rather than around it,
and it is easy to miss for the reason it was missed here: `${x#a}` is the
first parameter expansion anybody reaches for, it stays exact, and it
says nothing at all about the substitution next door. Measured 2026-09-11
on bash 5.3.15 with `v=ABC` and the option on —

| written | answer |
| --- | --- |
| `${v//b/X}` | `AXC` |
| `${v/b/X}` | `AXC` |
| `${v/#a/Y}` | `YBC` |
| `${v/%c/Z}` | `ABZ` |
| `${v#a}` | `ABC` |
| `${v%c}` | `ABC` |
| `${v^^b}` | `ABC` |

— so a probe that used only a trim cannot tell "parameter expansion is
exempt" from "the trims are exempt", and those are different rules.

Three details of the fold, each measured rather than assumed. It is
**symmetric and inside the matcher**: with `v=abc`, `${v//B/X}`,
`${v//[B]/X}` and `${v//b?/X}` are all `aXc`/`aX`, so what folds is the
comparison and not the pattern's text. It does **not** decide what an
`&` in the replacement stands for: `${v//b/<&>}` on `ABC` is `A<B>C`,
the subject's own upper-case B. And `nocaseglob` reaches none of it —
with that option on instead, `${v//b/X}` leaves `ABC` alone.

bash 3.2 does not fold the substitution, which is where the panel's two
bash columns part company; this shell follows the version its bash
dialect is measured against.

### `**` is two questions, and the panel splits on the second

`globstar` above answers both of them at once, which hid the fact that
they are separate until zsh was measured beside it. Measured 2026-09-07,
in a directory holding `ax`, `bx`, `cx/ax` and `cx/dx/ax`:

| pattern | bash 5.3 `-s globstar` | ksh93 `-o globstar` | zsh 5.9.2, no option |
| --- | --- | --- | --- |
| `**/a*` | `ax cx/ax cx/dx/ax` | `ax cx/ax cx/dx/ax` | `ax cx/ax cx/dx/ax` |
| `**` | `ax bx cx cx/ax cx/dx cx/dx/ax` | the same | `ax bx cx` |
| `cx/**` | `cx/ cx/ax cx/dx cx/dx/ax` | `cx/ax cx/dx cx/dx/ax` | `cx/ax cx/dx` |

So the **slashed** form is one behavior in three shells — zero or more
directory levels, the starting directory included — and it needs no
option in zsh, which has no `setopt` name that turns it off. The **bare**
form is not: bash and ksh93 reach every level, and zsh answers what `*`
answers.

A slash is what tells them apart, rather than "nothing follows": `**/`
written at the very end still crosses levels in all three, because the
component after it is empty and *written*. `cx/**/` is `cx/ cx/dx/` in
bash and zsh.

Two run-time switches follow, `StarStarCrossesDirectories` and
`StarStarAloneCrossesDirectories`, and the second is consulted only where
the first is on — a shell that does not read `**` as level-crossing at
all reads it as `*` in every position. bash's `globstar` sets both; the
zsh dialect sets the first and leaves the second off. zsh's
`globstarshort`, which is the name that would move the second, is
recorded and not acted on.

Reading it as one question is what made `**/a*` come back **short at
status 0** in the zsh dialect, with the match in the starting directory
missing and nothing said (#1339).

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
were (#856). What is missing is behavior: most of the rest are held at the
state this shell is already in, and asking one of them to *move* is
refused out loud with `shopt: name: not implemented`, status 1. The ones
that refuse `-u` rather than `-s` — `complete_fullquote`, `extquote`,
`globasciiranges`, `globskipdots`, `histappend`, `interactive_comments`,
`lithist`, `promptvars` and `sourcepath` — refuse it because the behavior
they name is simply how this shell works. Asking for the state already
held is granted in both directions, which is the same bargain `set +o
posix` strikes.

**Three names are recorded instead**, which is the bargain zsh's `setopt`
table already strikes over 151 of its 185 names, arrived at here from the
other end (#1712). `shopt -p` is a **capture surface**: an agent harness
snapshots a shell with it and sources the result back ahead of every later
command, so a state this shell reports wrong is re-applied to every command
it runs, and a state *bash* reported and this shell will not read back is a
complaint on standard error ahead of each of them. Measured 2026-09-10: a
real bash 5.3.15's own `shopt -p`, sourced into this shell, wrote seven
`not implemented` lines.

Three of the seven earn recording on one test — the option decides what a
*completer* offers, and there is nothing else a script can ask it:
`force_fignore` (this shell has no FIGNORE, so no word is ever named and
there is no last resort to leave out), `hostcomplete` (this completer reads
no host list) and `progcomp` (`complete` keeps every spec verbatim and the
completer consults none of them). They report bash's own default, remember
a move, and promise nothing — the state lives in an array under a name no
script can reach, so `(shopt -u progcomp)` stays inside the subshell.

A fourth, `complete_fullquote`, is not recorded and that is the point of
having the category at all: this shell really does backslash every shell
metacharacter in a name it offers, so bash's `on` is this implementation's
own state and it belongs with the rest of what is true. `histappend` and
`lithist` read **on** against bash's off because this shell's history
really does append to the file and really does keep a multi-line entry's
newlines, measured through a terminal — divergences reported honestly
rather than defaults to copy.

The seventh, `patsub_replacement`, was the one left reporting **off**
because the behavior it gates was unbuilt, and reporting it on would have
been the worst kind of lie. It is built now and the name is wired to
`interp.ReplacementAmpersandIsTheMatch` (#1862); the section below is what
it switches.

These are run-time states rather than semantics axes, which is why the
core holds them as `interp.MatchOption` values and only the bash dialect
maps names onto them. `failglob` is recorded here and deliberately not
implemented: its miss aborts the rest of the current *line* and then
carries on (measured: `shopt -s failglob` then `echo zz*zz; echo after`
on one line prints neither, and `echo after` on the next line prints),
which is a control-flow shape nothing else needs yet.

## The ampersand in a replacement, and the option that gates it

bash 5.3 reads an unescaped `&` in a pattern substitution's replacement as
the text the pattern just matched. Measured 2026-09-11 with `v=abc`:

| written | bash 5.3.15 | bash 3.2.57 | ksh93 | zsh 5.9.2 |
| --- | --- | --- | --- | --- |
| `${v/b/[&]}` | `a[b]c` | `a[&]c` | `a[&]c` | `a[&]c` |
| `${v//b/[&]}` | `a[b]c` | `a[&]c` | `a[&]c` | `a[&]c` |
| `${v/b/[\&]}` | `a[&]c` | `a[\&]c` | `a[&]c` | `a[\&]c` |
| `shopt -p patsub_replacement` | `shopt -s …` | invalid name | — | — |

So one shell in the panel, behind an option that shell turns on with
nothing said — which is why this is a `MatchOption` and not a semantics
axis, and why the bash dialect is the only one that sets it. dash has no
operator at all and refuses the line.

**Every match gets its own reading.** `v=abcabc; ${v//b/<&>}` is
`a<b>ca<b>c`, `${v//[ab]/<&>}` is `<a><b>c<a><b>c`, and the `&` stands for
what the *pattern took* rather than for the pattern: `v=aXbXc;
${v/X*X/<&>}` is `a<XbX>c`. The anchored forms read it too —
`v=abcabc; ${v/#a/<&>}` is `<a>bcabc` and `${v/%c/<&>}` is `abcab<c>` —
and a pattern that matches nothing leaves the replacement unread.

**Quoting is what turns the reading off, character by character**, the same
channel that decides whether a `*` in the *pattern* half is a pattern.
Measured with `v=abc`, in both `${ }` and `"${ }"`, all giving `a&c`:
`${v/b/"&"}`, `${v/b/'&'}`, `${v/b/$'&'}`, `${v/b/\&}`. The enclosing
quotes are not asked — `"${v/b/[&]}"` still answers `a[b]c` — because the
question is how the `&` itself was written.

**A replacement that arrived from an expansion is read**, and its quoting
counts the same way. With `r='&'`: `${v/b/$r}` is `abc` and `${v/b/"$r"}`
is `a&c`. So the reading runs after the replacement is expanded, over the
text the expansion produced.

**The escape half is a different rule from the history modifier's.** In the
text an expansion brought, a backslash is an escape only before an `&` or
another backslash and is kept in front of anything else — with `r='[\&]'`
the answer is `a[&]c`, with `r='[\\&]'` it is `a[\b]c`, and with
`r='[\a]'` it is `a[\a]c`. `${name:s/pat/rep}` resolves `\a` to `a`, which
is why `expandAmpersand` takes the rule as an argument rather than there
being two of it.

**With `shopt -u patsub_replacement` none of that happens**: `${v/b/[&]}`
is `a[&]c`, and a `\&` from an expansion keeps its backslash — `r='[\&]'`
answers `a[\&]c` where with the reading on it answers `a[&]c`. A written
`[\&]` answers `a[&]c` in both states, because that backslash is removed
by ordinary quote removal before the replacement is ever read.

## One shell's extended pattern operators, and the option that gates them

zsh has a second pattern language on top of the one above, and every part
of it is behind `setopt extendedglob`. Measured on zsh 5.9.2, 2026-09-07,
under `env -i` with a scratch `HOME` and `ZDOTDIR`.

**With the option off, all of it is ordinary text**, and that is the
measurement that makes the option load-bearing rather than a formality:

| written | option off | option on |
| --- | --- | --- |
| `[[ 'a#' == a# ]]` | matches | does not |
| `[[ aaa == a# ]]` | does not | matches |
| `[[ '#iabc' == (#i)abc ]]` | matches — `(#i)` is a group holding one alternative | does not |
| `[[ ABC == (#i)abc ]]` | does not | matches |
| `[[ abc == ^x* ]]` | does not | matches |
| `[[ '^x' == ^x ]]` | matches | does not |

So the same four characters are a closure or two ordinary ones depending
on a run-time state, which is why this is an `interp.MatchOption`
(`ExtendedPatternOperators`) and not a grammar flag: nothing here reaches
the lexer, and a pattern held in a variable is read the same way as one
written down.

### The four constructs, and how they bind

    pat1~pat2   the exclusion: pat1, minus anything pat2 also matches
    ^pat        the negation: anything the rest of this branch does not match
    item#       the closure: zero or more of the item in front of it
    item##      one or more
    (#…)        a flag group, changing how the rest of the branch reads

Precedence, each measured rather than read off a manual:

- **The exclusion is tighter than `|`**, so a `|` separates whole
  exclusions. `[[ zz == (a*~*b*|zz) ]]` matches, which it could not if the
  `~` took the whole `*b*|zz` as its right side, and `case` reads
  `a~a|b` as `(a~a)|b`: `a` misses, `b` hits, `c` misses.
- **Exclusions chain to the left.** `a*~*b~*c` is `a*` with two things taken
  out of it, each compared with the whole subject: `ad` hits, `adb` and
  `adc` miss.
- **The exclusion is looser than `/`.** `**/x~*bar*` in a directory holding
  `foo/x`, `bar/x` and `baz/sub/x` lists `baz/sub/x` and `foo/x`: the right
  side was compared against the whole path, not against one component. The
  filesystem half of this has a section of its own, below.
- **The negation is tighter than `/`.** `^foo/x` lists `bar/x`, so `^` is
  read inside one component.
- **The negation starts where it stands** and runs to the end of its
  branch: `[[ ab == a^x ]]` matches and `[[ ab == a^b ]]` does not.
- **A closure binds to exactly one item**: `[[ abbb == ab# ]]` matches and
  `[[ abab == ab# ]]` does not. An item is a group, a bracket expression,
  an escaped character, a `?`, a numeric range or one ordinary character —
  `(ab)#`, `[ab]#` and `?#` all match — and a `*` is **not** one: `*#` is
  `bad pattern`, as is a third `#` (`ab###`).
- **The item is the *whole* bracket expression**, `[:class:]` and all. A
  class's own `]` does not end the bracket, so `[[:space:]]##` is
  one-or-more whitespace characters and not one whitespace character
  followed by a repeated `#`. Measured on the trims, where the extent of a
  single match is visible: with `v="  x  "`, `${v##[[:space:]]##}` is
  `x  ` and `${v%%[[:space:]]##}` is `  x`, and with `v=abX`,
  `${v##[[:alpha:]]##}` and `${v##[[:alpha:]]#}` are both empty and
  `${v##[[:alpha:]](#c2)}` is `X`. A range or an enumeration has no inner
  `]` and was never in doubt: `[a-z]##`, `[ab]##` and `?##` quantify the
  same way (#1409).

  **Two spellings cannot see this and must not be used to check it.** A
  global substitution re-applies its pattern until nothing matches, so
  matching one character at a time still removes the whole run —
  `v="  x"; ${v//[[:space:]]##/}` is `x` either way. And the *shortest*
  match of one-or-more is one character, which is the same answer a
  closure that quantified nothing gives: `v=12ab; ${v#[[:digit:]]##}` is
  `2ab` either way. Only `##`, `%%`, `(M)` and a single anchored
  replacement observe the length of one match.

  A `[:` with no `:]` **after** it is not a class, and neither is one whose
  only colon is its own: `[[:space]` is the ordinary bracket holding `[`,
  `:`, `s`, `p`, `a`, `c` and `e`, so `v="cape[X"; ${v##[[:space]##}` is
  `X`; and `[[:]` is the bracket holding `[` and `:`, which is why
  `[[ ":" == [[:] ]]` matches and `[[ x == [[:] ]]` does not in bash 5.3.15
  and zsh 5.9.2 alike. Reading the closing `:]` from the `[` rather than
  from after the `[:` lets one colon do both jobs, which gave the class a
  name running from offset 3 to offset 2 and panicked this shell on every
  surface that matches a pattern.

  `+([[:alpha:]])` is a **different operator** — a quantified group, in
  bash and ksh93 — and is read elsewhere in the matcher. `v=abX;
  ${v##+([[:alpha:]])}` under `shopt -s extglob` is empty in bash 5.3,
  bash 3.2 and ksh93; it has nothing to say about the postfix closure and
  answering it correctly is no evidence about one.
- **A `~` with nothing on one side of it is the character itself**:
  `[[ 'a~' == a~ ]]` matches and `[[ ab == a~ ]]` does not.
- **A `#` with nothing in front of it is the character itself**:
  `p="#foo"; [[ "#foo" == $p ]]` matches.

### The flag groups

Thirteen letters are taken. Enumerated by asking `[[ abc == (#X)abc ]]`
of all 52 letters: `b`, `B`, `e`, `i`, `l`, `m`, `q`, `s`, `u`, `I`, `M`
and `U` are taken bare, and `a` and `c` are taken **only** with a number
after them — `(#a)` and `(#c)` are both `bad pattern` where `(#a1)` and
`(#c1)` are not. `(#)` with an empty body is a no-op that matches. Every
other letter is `bad pattern`.

A flag reaches to the end of the group it stands in, which is why
`[[ ABCd == ((#i)abc)d ]]` matches and `[[ ABCD == ((#i)abc)d ]]` does
not.

**The case flags reach only the literal characters of a pattern**, which
is the finding that separates them from `nocasematch`:

    [[ ABC == (#i)abc ]]              matches
    [[ B == (#i)\b ]]                 matches — an escaped literal folds
    [[ ABC == (#i)?bc ]]              matches
    [[ ABC == (#i)[abc][abc][abc] ]]  does NOT — a bracket does not fold
    [[ ABC == (#i)[[:lower:]]## ]]    does NOT — nor does a class

`(#I)` turns the folding back off (`[[ ABCdef == (#i)abc(#I)def ]]`
matches, `[[ ABCDEF == … ]]` does not) and `(#l)` folds one way only: a
lowercase letter in the pattern matches either case and an uppercase one
matches only itself, so `[[ ABC == (#l)abc ]]` matches and
`[[ abc == (#l)ABC ]]` does not.

`(#c…)` is a closure rather than a flag: `a(#c3)` is `aaa`, `a(#c2,4)` is
two to four, `a(#c2,)` is two or more, `a(#c,3)` is at most three, and
`a(#c,)` is `a#`. It needs an item in front of it — `[[ aaa == (#c3) ]]`
is `bad pattern`.

### Where a match is standing

`(#s)` and `(#e)` are not options at all. Each is a **zero-width
assertion**: it consumes nothing and asks only where in the subject the
match currently stands — `(#s)` that it is at the start, `(#e)` that it
is at the end. Measured on zsh 5.9.2, 2026-09-07, with `extendedglob` on.

An anchor anywhere else is a match that cannot happen, and **not** a
pattern that cannot be read:

    [[ ab == (#s)ab ]]        matches
    [[ ab == ab(#e) ]]        matches
    [[ '' == (#s)(#e) ]]      matches
    [[ ab == a(#s)b ]]        does not — status 1, not the 2 of a bad pattern
    [[ ab == a(#e)b ]]        does not
    [[ ab == (#s)(#e)ab ]]    does not

That distinction is load-bearing, because it is what lets another branch
carry the match:

    [[ ab == (a|(#s))b ]]     matches — by way of the `a` arm
    [[ ab == ((#s)a|b)b ]]    matches
    [[ ab == (a(#e)|a)b ]]    matches
    [[ ab == *(#s)ab ]]       matches — a `*` that consumed nothing is at 0
    [[ ab == ab(#e)* ]]       matches

**Neither may share its flag group.** The body is the letter alone or the
pattern is rejected — `(#is)`, `(#si)`, `(#se)` and `(#ss)` are every one
of them `bad pattern`, where the same letters written as two groups
(`(#i)(#s)`) are fine. A flag group is also not a closable item:
`[[ ab == (#s)# ]]` is `bad pattern`.

**An anchor names the subject, never the piece a surface handed over.**
This is the whole of what the mechanism costs and the reason the matcher
has to carry its position rather than deriving one from the string in
hand: a trim tries the prefixes of its value one at a time, and each of
those trials is at the start of the subject but reaches its end only when
it is the whole of it.

| surface | measured |
| --- | --- |
| `[[ ]]` | `[[ ab == (#s)ab(#e) ]]` matches |
| `case` | `case ab in ((#s)ab)` matches; `(a(#e)b)` does not |
| `${x#pat}` | `x=abcd; ${x#(#s)ab}` is `cd`; `${x#ab(#e)}` is `abcd`; `${x#abcd(#e)}` is empty |
| `${x%pat}` | `${x%cd(#e)}` is `ab`; `${x%(#s)cd}` is `abcd`; `${x%(#s)abcd}` is empty |
| `${x//pat/rep}` | `x=XbXcX; ${x//(#s)X/-}` is `-bXcX`; `${x//X(#e)/-}` is `XbXc-`; `${x//X(#s)/-}` is unchanged |
| `${x:#pat}`, `(M)`, `(R)` | whole element: `a=(ab cb); ${a:#(#s)a*}` is `cb` |
| pathname expansion | **per component**: `**/(#s)a*` lists `ax` and `cx/ax`, `*/(#s)a*` lists `cx/ax`, `*x(#e)` lists the names ending in `x` |

An empty match is a position like any other, so `${x//(#s)/-}` on `abc`
is `-abc` and `${x//(#e)/-}` is `abc-`.

### Reporting where a match landed

`(#b)`, `(#B)`, `(#m)` and `(#M)` need more than a position: each has to
**report** one, into parameters. Measured on zsh 5.9.2, 2026-09-07.

`(#b)` turns the groups of a pattern into backreferences and fills three
arrays; `(#B)` turns that back off. `(#m)` fills three scalars with the
whole match; `(#M)` turns *that* off. The two sets are independent and
compose — `[[ abc == (#m)(#b)(a)b* ]]` fills both.

    [[ abc == (#b)(a)(b)c ]]    match=(a b)   mbegin=(1 2)  mend=(1 2)
    [[ abc == (#b)a(b*) ]]      match=(bc)    mbegin=(2)    mend=(3)
    [[ abc == (#m)a* ]]         MATCH=abc     MBEGIN=1      MEND=3

**The bounds are one-based and the end is the index of the last
character**, so an empty match has an end one below its begin:
`[[ ac == (#b)(a)(b|)c ]]` is `mbegin=(1 2)` and `mend=(1 1)`.

**They are character indices, not byte offsets.** `x=aébc;
${x//(#m)?/<$MATCH:$MBEGIN>}` is `<a:1><é:2><b:3><c:4>`.

**They move with the array base.** Under `ksharrays`, `(#m)` on `abc`
reports `MBEGIN=0 MEND=2` and the arrays count from zero too.

#### Numbering

By **opening parenthesis**, outermost first, counting every group whose
`(` is read while `(#b)` is in effect:

    [[ abc == (#b)((a)(b))c ]]      match=(ab a b)
    [[ abc == ((#b)(x)|(#b)a(b)c) ]]  match=("" b)  — the outer `(` precedes the flag
    [[ abc == (#b)((x)|a(b)c) ]]    match=(abc "" b)  mbegin=(1 -1 2)

**A group that did not participate is the empty string with -1 for both
bounds** — the third row above, where `(x)` is numbered 2 even though the
arm holding it is never taken. That is why a quietly dropped `(#b)` was
never acceptable: empty is a real answer here and reads exactly like a
feature that is absent.

Both switches are **scoped to the group they stand in**, like the case
flags: `[[ abc == (#b)(a)((#B)(b))(c) ]]` reports three groups, not four.
Within one group the last letter wins — `(#bB)` reports none.

#### Which split is reported

The combination of arm and length decides nothing about *whether* a
pattern matches and everything about what it reports:

    [[ abc == (#b)(a|ab)* ]]        match=(a)      — a written arm beats a longer one
    [[ abc == (#b)(ab|a)* ]]        match=(ab)
    [[ ab == (#b)(|a)(b|ab) ]]      match=("" ab)  — including an empty arm
    [[ aabab == (#b)(a*)b ]]        match=(aaba)   — within an arm, as much as it can
    [[ abcabc == (#b)(*)(abc) ]]    match=(abc abc)
    [[ abab == (#b)(ab)# ]]         mbegin=(3)     — a closure reports its *last* repetition

#### Nothing is written unless the pattern asked and matched

    match=(zz); [[ abc == (#b)abc ]]      match is still (zz)  — no group
    match=(zz); [[ abc == (#b)(x)zz ]]    match is still (zz)  — no match
    MATCH=zz;   [[ abc == (#m)xyz ]]      MATCH is still zz

They are ordinary parameters, so `local match mbegin mend` in a function
contains them.

#### Which surfaces report

`[[ ]]`, `case`, `${x#pat}`, `${x%pat}`, `${x/pat/rep}`, `${x:#pat}`, the
`(M)` filter and an `(r)` subscript all report, in **whole-subject**
coordinates: `x=abcd; ${x%(#b)(c)(d)}` gives `mbegin=(3 4)`.

**Pathname expansion reports nothing.** `print -rl -- (#b)(a)*` leaves
`$match` untouched, and so does `(#m)` there.

#### A replacement is expanded once per match

The sharpest consequence, and the one a replacement joined before the scan
cannot express:

    x=abcd; ${x//(#b)(b)(c)/[$match[1]-$match[2]]}   →  a[b-c]d
    x=abcd; ${x//(#m)[bc]/<$MATCH:$MBEGIN:$MEND>}    →  a<b:2:2><c:3:3>d
    x=abcb; ${x//(#b)(b)/Q}                          →  aQcQ, and mbegin=(4)

so each replacement reads what *its own* match wrote, and the parameters
are left holding the last one.

#### Where a misplaced `(#m)` is read differently here

`(#m)` is honored where it is **in effect at the end of the pattern's own
top level** — which is the reading the manual's "the flag applies from
where it stands" gives, and which is what every use of it in a real plugin
tree needs, since all 43 of them write it at the front. Three measured
rows do not fit that reading and are recorded rather than reproduced:

| pattern | zsh | here |
| --- | --- | --- |
| `[[ abc == a(#m)bc ]]` | `MATCH` unset | `MATCH=abc` |
| `[[ abc == (#m)a(#m)bc ]]` | `MATCH` unset | `MATCH=abc` |
| `[[ abc == *(#m)* ]]` | `MATCH=abc` | `MATCH=abc` |

The first two are `(#m)` standing between two runs of ordinary
characters, and the third is the same position with the runs replaced by
stars — so the difference is not the *position* and no rule stated in
terms of one accounts for it. Reproducing it would mean reasoning about
how zsh compiles a literal run, which is a fact about an implementation
rather than about the language, and CLEANROOM.md is what forbids going and
looking. Recorded here so the divergence is a decision.

### A `(#…)` at the end of a component is a flag group, not a qualifier

The two spellings collide at exactly one place — the end of the last
component, which is where a qualifier list is written and where an end
anchor is written. With `extendedglob` on, a trailing group whose body
starts with `#` and is not the `(#q…)` spelling is read as a **flag
group**:

    *x(#i)      lists ax and cx        — a flag group
    *x(#e)      lists ax and cx        — a flag group
    *(#q/)      lists the directories  — a qualifier list
    *(#c1,9)    bad pattern            — the matcher's complaint, not an attribute's

With the option off, the same `*x(#i)` is `unknown file attribute: #`,
which is what says the reading is the option's.

### The status a rejected pattern exits with is the surface's

All four abandon the script rather than failing the match, and the status
differs by where the pattern stood — measured with `(#Z)a`:

| surface | status |
| --- | --- |
| `[[ ]]` | 2 |
| `case` | 0 |
| `${x#pat}` | 1 |
| pathname expansion | 1 |

### A metacharacter that arrived from a value is not one

`#`, `~` and `^` are read as operators only in pattern text written down,
never in the result of an expansion — which is the same answer zsh gives
for `*`, and the reason `${~p}` exists:

    p="a#b"; [[ ab == $p ]]       does not match
    p="a#b"; [[ ab == ${~p} ]]    matches
    p="^x";  [[ ab == $p ]]       does not match
    p="^x";  [[ ab == ${~p} ]]    matches

### What is read and not implemented here

`(#i)`, `(#I)`, `(#l)`, the `#` and `##` closures, `(#c…)`, `^`, `~` and
the `(#q…)` spelling of a qualifier list are implemented, in every surface
that matches a pattern: `[[ ]]`, `case`, `${x#pat}` / `${x%pat}` /
`${x/pat/rep}`, `${x:#pat}` and the `(M)` filter, and pathname expansion.

The two anchors `(#s)` and `(#e)` are implemented on every one of those
surfaces too, and so are `(#b)`, `(#B)`, `(#m)` and `(#M)` with their
parameters. The matcher carries the offset of the piece it is matching
within the subject the caller named, and the offset of the pattern text it
is reading within the pattern — the first is what an assertion about a
position needs, and the second is what tells a group which backreference
it is.

The rest is **refused by name**: `(#a1)` approximate matching, which needs
edit distance and has no use at all in the plugin tree this was measured
against, and `(#u)` / `(#U)`. So a pattern using one says

    <pattern>: the (#a) pattern flag is not implemented

and stops the script. That is deliberate and it is the convention the whole
entry is built on: a flag that was quietly dropped left a script reading a
plausible wrong answer rather than seeing a refusal, which is the failure
mode this project exists to avoid (#1244). `(#b)` was the sharpest case of
it — a dropped one left `$match[1]` **empty**, which is also what a group
that genuinely matched nothing leaves — and it is why the flag kept
refusing until it could fill all three arrays (#1304).


### The exclusion against the filesystem

An exclusion is the one pattern operator that is **looser than `/`**, so
pathname expansion has to take it off the field before it cuts the field
into components. Measured 2026-09-10 against zsh 5.9.2, over a tree holding
`/tmp/gx/keep_a`, `/tmp/gx/keep_c~`, `/tmp/gx/keep_b.zwc`, `/tmp/gx/gxdir/`
and `/tmp/gx/sub/`:

    /tmp/gx/*~*(~|.zwc)     → /tmp/gx/gxdir /tmp/gx/keep_a /tmp/gx/sub
    /tmp/gx/*~*gx*          → nothing: every word holds `gx` in its directory
    cd /tmp/gx; *~*gx*      → keep_a keep_b.zwc keep_c~ sub
    /tmp/gx/*~*x*/deep      → all seven: no word ends `/deep`

The reading those three rows force:

- **The left side is globbed the ordinary way**, one component at a time,
  with no metacharacter crossing a `/`.
- **The right side is matched against the whole word the left side
  produced**, as a plain pattern: `/` is an ordinary character in it, a `*`
  crosses directories, and the leading-period rule does not apply —
  `*(D)`'s `.hidden` is taken out by `~*hidden*`.
- **The word is the one the pattern wrote**, not a cleaned or absolute path.
  `cd /tmp; ./gx/*~./gx/keep_a` takes `keep_a` out and `./gx/*~gx/keep_a`
  does not, and `/tmp/gx/sub/../*` is excluded as `/tmp/gx/sub/../keep_a`.
- **The exclusion comes before the qualifiers.** `/tmp/gx/*~*gxdir*([1])` is
  `/tmp/gx/keep_a`: the list `[1]` counts into has already had the exclusion
  taken out of it. Glob modifiers come after it too — `*~*zwc*(N:e)` runs
  `:e` over what survived.
- **A `~` straight after a `/` leaves the left side's last component
  empty**, and no file is named nothing: `/tmp/gx/sub/` lists `/tmp/gx/sub/`
  and `/tmp/gx/sub/~*zzzz*` lists nothing. The trailing slash that means
  "directories only" is the one at the end of the *word*.
- **An exclusion inside a group is not a top-level one** and stays the
  single component's question: `/tmp/gx/(*~sub)/*` is `/tmp/gx/gxdir/deep`.
- **Everything excluded is a miss**, complained about as `no matches found`
  naming the whole field, `~` and all.

**A `~` on its own does not make a word a pattern.** `keep_a~zzz` prints
those eleven characters where `keep#_a~zzz` globs and `^zzz` lists the
directory, so the exclusion says what to take out of a search rather than
that there is one — it is not in the set that sends a field to the
filesystem, though it is still escaped when an expansion is pasted into
one.

This is the shape every real call site writes, because a script that
searches `$fpath` has a directory to search in: `vcs_info` sweeps it with
`$dir/VCS_INFO_get_data_*~*(~|.zwc)(N)`, and refusing that by name put
eighteen lines on the standard error of one interactive startup (#1719).

## What matching costs, and what is left

This is a backtracking matcher, and the shapes a prompt theme writes —
nested alternation over closures, with bracket expressions inside the arms
— are what make a backtracking matcher expensive. The costs are recorded
here because two of them have been taken out and the third has not, and
the third is a design question rather than an oversight.

The instrument is `BenchmarkReplaceExtendedGlob` in `interp`: one
`${msg//pat/X}` over a real theme's pattern against the 82-byte message it
is applied to, with no shell around it. The reference is real zsh 5.9.2
doing the same substitution, timed by running it 5000 times from a script
and subtracting the startup the same script measures at zero iterations —
**12.4 µs** per substitution on this machine.

The two figures below are interleaved runs of the same benchmark on the
same machine minutes apart, which is the only way to compare them: this
machine runs several graders at once and a figure taken alone is a figure
about the load.

| | per substitution | matchHere calls |
| --- | --- | --- |
| before #1396 | 8 s | — |
| after #1396, before #1398 | 5.2 ms | 159,540 |
| after #1398 | 0.66 ms | 43,269 |
| real zsh 5.9.2 | 0.0124 ms | — |

`make startup`'s rich-rc case is the same work seen from the outside —
forty sourced plugins and this substitution, timed from process start to a
prompt — and it moves with it: `ours-zsh` **31.5 ms to 9.9 ms** against
real `zsh` at 8.4 ms in the same run. That case was written so this number
would be visible rather than inferred, and it now stands at 1.2x real zsh
where it was 3.6x.

**The blowup was #1396's and the constant factor was #1398's.** Three
things came out, in this order and each measured on its own:

1. **The pattern's structure is read once.** Where a group closes, where a
   bracket closes, how far an item reaches, and what a group's arms are
   were all rediscovered by a scan at every position of every trial —
   `splitExclusion` walked the remaining pattern looking for a `~` that
   most patterns do not contain at all. `matchWhere.prepare` works them out
   once for the whole pattern. Every lookup falls back to the scan it
   replaces, because a group body or an arm is a *truncation* of the
   pattern rather than a suffix of it and a closer past the end of the
   piece is not a closer.
2. **A group's split is floored by what follows it.** `matchGroup` tried
   every split of the subject and asked the arms about each, including all
   the splits that hand what follows the group more text than it could
   possibly consume. The commonest case is a group with *nothing* after
   it, where exactly one split is possible and 82 were being tried. It
   does not apply where the group repeats: what follows one repetition of
   a `*` or `+` group is the group as well as the rest, which `+(a)b`
   against `aab` is the row for.
3. **The memo is a flat table rather than a map.** #1396's memo of dead
   ends is what removed the blowup, and the map then became the cost
   rather than the questions it answers — near half the samples in
   hashing, probing and rehashing a table that reaches a hundred thousand
   entries in one substitution. The keys are already one integer each and
   a dead end carries no value, so linear probing over a flat slice does
   the whole job.

Each is switchable from a test — `patternPrepares`, `patternReachBounds`,
`memoThreshold` — so "this changed no answer" is a comparison against the
code rather than a claim about it.

**What is left is about 50x, and it is the number of questions rather than
their cost.** 43,269 calls to `matchHere` for an 82-byte subject is the
remaining gap; zsh cannot be doing more than a few thousand steps in
12 µs. Two multipliers are in it and neither is a constant factor:

- **The substitution enumerates spans.** `${v//pat/X}` wants the longest
  match at each position and finds it by trying every span from the
  longest down, which is 285 whole match attempts for this subject where
  an extent-reporting matcher would make one pass per position. The
  comment on #1398 sets out why that change is not free: first success is
  not longest match, so the exploration order or the early exit has to
  give, and a "faster" matcher that stops finding the same matches is not
  faster.
- **The search re-derives which construct stands at a position.** Reading
  the pattern's *extents* once is what has been done; reading it into a
  compiled form — a node per construct, matched against directly — is what
  has not, and it is what removes the per-step branch chain rather than
  making it cheaper.

Both want their own design note and their own differential run against the
panel, which is why they are not in the change that wrote this section.

## What this does not cover

Collating symbols and equivalence classes (`[[.a.]]`, `[[=a=]]`), which
POSIX defines and which no consumer in this project has needed. Recorded
so their absence is a decision rather than an oversight.
