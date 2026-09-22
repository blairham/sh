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
`case` pattern spelled `bet\a` is the pattern `beta` in all seven shells
and says nothing about this. That unanimity is **outside** a bracket
expression, and it is the control for the axis in the next section —
inside one, BusyBox ash spends nothing. What does ask it is a pattern the matcher
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

| form | meaning | dash | ash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `[abc]` | any of those characters | yes | yes | yes | yes | yes |
| `[a-z]` | a range | yes | yes | yes | yes | yes |
| `[!abc]` | none of those | yes | yes | yes | yes | yes |
| `[^abc]` | none of those | **no** | yes | yes | yes | yes |
| `[[:digit:]]` | a character class | yes | yes | yes | yes | yes |
| `[a-]` | a trailing `-` is literal | yes | yes | yes | yes | yes |

**`!` is the portable negation; `^` is an extension** that dash does not
have — there it is an ordinary character, so `[^abc]` matches a literal
`^`, `a`, `b` or `c`. Silent again: the pattern still matches things,
just not the things intended.

Semantics axis: `BracketCaretNegates` — dash no, BusyBox ash, bash,
ksh93 and zsh yes. Unanswered in the core, and the `posix` preset says
no, because in a shell pattern the standard has `!` *replace* `^` in the
role it plays in regular expression notation (XCU §2.13.1), which leaves
`^` ordinary.

**dash is the sole dissenter and it is still an axis**, which is what
#489 asked. `core.md`'s rule that a lone holdout does not keep a
construct out of the core is about *membership*, and this is not a
construct dash lacks: `[^abc]` parses there and matches a different set.
Identical syntax with two meanings is a conflict, and a conflict is a
field. `semantics.md` has the seven-column measurement and the reasoning
under "A sole dissenter on a *meaning* is still an axis".

The subject used to measure it has to be a letter the class does **not**
name. A caret matches `[^abc]` under both readings — literally, where the
class holds one, and by negation everywhere else — so it decides nothing,
and a probe using it sat in the axis comment for a while proving nothing.
`case z in [^abc])` matches in every column but dash, and `case b in
[^abc])` matches in dash alone, which is the same fact from the other
side: dash is not failing to match, it is matching `{^, a, b, c}`.

### What a refused pattern costs the script

One column calls an unterminated bracket a bad pattern rather than a literal
or a class that matches nothing — `Semantics.UnterminatedBracket` at
`BracketBadPattern` — and what that refusal costs the script is two further
questions. Measured 2026-09-18 on zsh 5.9.2 under `env -i HOME=…
PATH=/usr/bin:/bin LC_ALL=C`, from a script file:

| where the pattern is | what happens |
| --- | --- |
| `case '[a' in ([) …` at the top level | the script ends, exit **1** |
| `[[ '[a' == [ ]]` | the script ends, exit **2** |
| `echo '[a'*` | the script ends, exit **1** |
| inside an `eval` | the `eval` reports 1, the **script carries on** |
| inside a `.` | the dot reports 126, the script carries on |
| inside a function body | the script ends, exit 1 |
| inside `( … )` | the subshell ends, `$?` is **0**, the script carries on |

**It is an error, not a request to stop.** That is what the fourth and fifth
rows say, and it is the same boundary `FatalErrorEndsBorrowedTextOnly`
already draws for every other fatal error: text a special builtin is running
is a boundary in that column and a function body is not. This raised an
unannotated stop, so an `eval` around a bad pattern abandoned the whole
script and everything after the first one was lost (#3398).

**The status is the dialect's fatal one**, except in a condition, which
carries its own 2 — the same number `[[ x == (#Z)a ]]` already gives. It used
to be 0 for both, on a measurement that read the *last* row's `$?` rather
than the script's own exit, so a script whose last act was a bad pattern
reported success to whatever ran it.

The last row is measured and not matched: `$?` after `( case '[a' in ([) … )`
is 0 in real zsh and 1 here, where the refusal writes the fatal status the
five rows above it need. Separating the two would mean a number that only the
boundaries read, and the shell exits on the rows that matter either way.

### A backslash inside a bracket expression

Three readings, and `Semantics.BracketEscape` is the axis. Measured
2026-09-07 through zsh's `${~p}` and again 2026-09-16 against BusyBox
ash 1.37.0 in the digest-pinned Alpine image, under `--init`:

| shell | `[\)]` holds | `[a\-z]` holds |
| --- | --- | --- |
| bash 5.3, as `sh`, bash 3.2, dash, ksh93 | `)` | `a`, `-`, `z` |
| zsh 5.9.2 | `)` and `\` | `a`, `-`, `z`, `\` — no `y` |
| BusyBox ash 1.37.0 | `\` alone | `a`, and the **range** `\`–`z` |

The last row's second cell is what makes it a reading rather than a
refusal: the `-` behind the backslash is still the range operator there,
with the backslash as its left bound, so `a[a\-z]c` matches `abc` and
`a\c` and does **not** match `a-c`. A shell that rejected the pattern
would answer no to all three.

**Both routes reach it in that shell**, which is what the section above
used to deny. Quote removal there keeps the escape before a character
that is a metacharacter *to BusyBox* — measured, `* ? [ ] \ ! ^ -` and
no others, so `( ) | < ~ #` are ordinary — and the matcher's bracket
scan has no escape rule, so the surviving backslash is a member. A
pattern written in the source therefore reaches the matcher with the
backslash still in it, where the other six columns spend it.

This implementation marks quoted text with a backslash, that being the
only channel quoting has, so the set it marks narrows to what the
dialect itself reads wherever a surplus mark would become a surplus
member.

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

#### A backslash that arrived in a value

A **backslash** in such a result is not a metacharacter, but it decides
what happens to the character behind it — and the panel has *three*
readings of it, not two. `Semantics.ValueBackslashInAPattern` is the
answer.

Measured 2026-09-12 from a script file. Two directories, each holding a
name for every reading, because a directory holding none of them prints
the same word whichever rule is in force and cannot tell them apart.

With `a\b` and `a*` present:

    v='a\*'; set -- $v; printf "[%s]" "$@"

    dash, bash 5.3.15, bash-as-`sh`, bash 3.2.57, zsh 5.9.2   [a\*]
    ksh93u+                                                   [a\b]

With `a\bc` and `ab` present:

    v='a\b*'; set -- $v; printf "[%s]" "$@"

    dash, bash 5.3.15, bash-as-`sh`, bash 3.2.57   [ab]
    ksh93u+                                        [a\bc]
    zsh 5.9.2                                      [a\b*]

So, stated once each:

- **quotes what follows** — the character behind the backslash is not a
  metacharacter, and the backslash is **not itself matched**. dash and the
  three bash builds: the pattern is `ab*`.
- **is data, and what follows is disarmed** — the backslash is a character
  of the pattern and what follows it is not live, so the field matches
  what the same text written literally would. zsh, reached through
  `${~spec}` since it globs no expansion result otherwise.
- **is data, and what follows is live** — ksh93.

The first probe cannot tell the first two apart: a quoted metacharacter
and a disarmed one both leave nothing to glob. The second can, which is
why both are in the corpus.

Neither of the reading pairs removes the backslash from the **text**. A
shell performs no quote removal on the result of an expansion, so a
pattern that matches nothing comes back with the backslash in it:
`v='a\b[q]'` is `a\b[q]` in every column.

##### Why the quoting reading needs a symbol of its own

The escaped form spells "this byte was quoted" as a backslash in front of
it, and that has one meaning per byte. The quoting reading needs two at
once: for the match the backslash is a quote and contributes nothing, and
for the restored text it is a backslash.

A marked backslash followed by a marked character cannot carry both,
because that is already the *disarming* reading — and the two are told
apart by the panel. With `a\\bc` present:

    set -- 'a\\b'*      →  a\\bc     in every column
    v='a\\b'; set -- $v*   →  a\b, a\bc   in bash and dash
                           →  a\\bc       in ksh93 and zsh

The same four characters, literal on one line and from a value on the
next, match different names. So provenance is a fact the field has to
carry, and one alphabet of marks cannot carry it.

`valueBackslashMark` is that symbol. It records that a value put a
backslash there and commits to nothing; `globUnescape` turns it back into
a backslash, so the *text* is right under every reading whether the field
is ever globbed or not; and `resolveValueBackslashes` reads it into one of
the three readings at the top of `glob`.

##### Where the reading is chosen

At the field and not at the expansion, which is the second half of what
took this so long. A value is one span of a word, and the metacharacter
that makes the field a pattern may come from another:

    v='a\b'; set -- $v*

is `ab` in bash and `a\bc` in ksh93, and the value alone has nothing live
in it at all. Deciding where the value was escaped would have had to
answer without knowing the word.

The axis is asked where the three readings put different patterns on the
wire, and nowhere else. A field none of them makes a pattern is one
nothing will glob, and all three restore the same word — so a value
carrying a backslash in an ordinary word, which is the shape a script
actually writes, demands no dialect. `GlobExpansionResults` is *read*
rather than asked for the same reason (#1367, #1370).


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

### A tilde at the front of a pattern is expanded

    case $HOME in ~)                      →  matches   (all six)
    x=$HOME/sub; echo "${x#~}"            →  /sub      (all six)
    case $HOME/abc in ~/a*)               →  matches   (all six)
    h=$HOME; [[ $h == ~ ]]                →  true      (the five with [[ ]])

A tilde is expanded before the word becomes a pattern, which is the same
statement as the section above — a pattern operand is a word — said about
the one expansion that is not a substitution. It is unanimous, so it is
core.

Two limits, both unanimous as well. The **tail after the directory stays a
pattern**: `~/a*` is the home directory followed by a live `*`, not a
literal path. And **a tilde expands only where a tilde expands anywhere** —
`case $HOME in ~*)` does not match, because `*` names no user and the
segment stays as written, and `case a$HOME in a~)` does not match, because
the tilde is not at the front of the word. A reading that expanded every
tilde in a pattern gets the second wrong; one that only looked at the first
character gets it wrong too.

It is easy to leave out and the asymmetry is what hides it. The tilde
expands in every *word* position — an operand, an assignment's value, a
redirection target — and a condition's **left** side and every unary test
are words. So `[[ ~ == $h ]]`, `[[ -n ~ ]]` and `[[ -d ~ ]]` were all
right in a shell where `[[ $h == ~ ]]` was false, and the one wrong
position is the one powerlevel10k uses: `prompt_asdf` tells a global tool
version from a local override with `[[ ${files[1]:h} == ~ ]]`, so every
version read as a local override and five prompt segments were drawn that
the real shell does not draw (#2181, #2178).

**What the directory is worth once it is in the pattern** is a second
question, and here the panel divides. Measured with
`HOME=/tmp/p78home/a*b`, both `a*b` and `axxb` present:

| | `case '…/a*b' in ~)` | `case '…/axxb' in ~)` |
| --- | --- | --- |
| zsh, dash | matches | no |
| bash 3.2, ksh93 | matches | matches |
| bash 5.3 | no | no |

So the directory is text in two, a pattern in two, and in bash 5.3 neither
— it keeps the escape character itself in the pattern, which is the
question `ValueBackslashInAPattern` answers rather than this one.
The tilde's own reading is still **not** an axis: the core writes the
directory escaped, which is zsh's and dash's answer, and the tail after
it live. A home directory holding a
metacharacter is the only thing that can tell the readings apart.

A tilde that arrives as a *value* is not a written one and is left alone —
`v='~'; [[ $h == $v ]]` is false in all five — which is the same split
`${~spec}` exists for.

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

**The group's shape is the source's and its leaves are the value's**, in
one dialect. `ExpansionResultSuppliesGroupSyntax` — bash yes, ksh93 no —
is read only where `GlobExpansionResults` says yes, and it says that
`(`, `)` and `|` arriving out of an expansion are three literal
characters while every other metacharacter is live.

Re-measured 2026-09-12 on ksh93u+ 2012-08-01, in a directory holding
`ice.zsh`, `other.zsh` and one file literally named `ice|other.zsh`. That
third file is what makes the claim falsifiable at all: without it, *the
group is text* and *the group is a group whose one branch holds a literal
bar* both leave the word standing unchanged, and the probe set #1499 was
filed with cannot tell them apart.

| written | ksh93 | bash 5.3 |
| --- | --- | --- |
| `L='ice\|other'; echo @($L).zsh` | **`ice\|other.zsh`** | `ice.zsh other.zsh` |
| `B='\|'; echo @(ice${B}other).zsh` | **`ice\|other.zsh`** | `ice.zsh other.zsh` |
| `L=ice; echo @($L\|other).zsh` | `ice.zsh other.zsh` | `ice.zsh other.zsh` |
| `L='ice*'; echo @($L).zsh` | `ice.zsh ice\|other.zsh` | same |
| `L='ic?'; echo @($L).zsh` | `ice.zsh` | same |
| `S='[io]*'; echo $S` | all three | same |
| `L='@(ice\|other)'; echo $L.zsh` | `@(ice\|other).zsh` — no match | `ice.zsh other.zsh` |
| `Q='@'; echo ${Q}(ice\|other).zsh` | `ice.zsh other.zsh` | bash refuses the parse |

Row 1 **matched a file**, so the group is a group and its bar is a
character rather than a choice between two names — #1499 was filed as
*the construct is not re-read*, which is right about row 7 and wrong
about row 1. Row 2 is the same fact with the bar alone coming from a
value, so it is the character's provenance and not the value's shape.
Rows 3 to 6 say everything else out of an expansion is live, inside a
group as much as outside one. Row 7 is where *text* is the right word:
all three characters are dead, so nothing matches. **Row 8 is what
decides it** — with the parentheses written in the source and only the
`@` coming from a value, the group is read.

The condition surface does not do it — `L='a|b'; [[ a == @($L) ]]`
matches in ksh93 as in bash — and it is a different path in this
implementation too: a pattern operand's expansion goes through
`expansionPattern`, which the marking never reaches.

**No corpus row.** A snippet with `@(` in argument position needs
`Dialect.ExtendedPattern` in `corpusDialect()`, and turning it on stops
`pat/an-empty-quantified-group-is-not-a-function-definition` parsing at
all: `a+() { echo fn; }` becomes a quantified group where that case
exists to record it as a function definition. Two constructs, one
spelling, and the corpus already has a row for the other one. The rows
above are pinned by `dialect/ksh`'s own test instead.

**One thing here is still ours alone.** Row 8 does not parse in this
implementation — a group whose `@` comes from an expansion is a syntax
error in the `ksh` dialect — so the row that fixes the rule is one we
record and cannot yet run.

### A `(` opening an expansion's pattern operand is refused in one dialect

`${v#(a)}` does not parse in ksh93. Measured 2026-09-12 with `v=aXb`:

| written | dash | bash 5.3, 3.2, as `sh` | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `${v#(a)}` | `aXb` | `aXb` | **syntax error** | `Xb` |
| `${v%(b)}` | `aXb` | `aXb` | **syntax error** | `aX` |
| `${v/(a)/Z}` | — | `aXb` | **syntax error** | `ZXb` |
| `${v#@(a)}` | `aXb` | `aXb` | `Xb` | `aXb` |
| `${v#\(a\)}` | `aXb` | `aXb` | `aXb` | `aXb` |
| `${u:-(a)}` | `(a)` | `(a)` | `(a)` | `(a)` |

The refusal is at **parse** time and takes the script with it — status 3
there — so it is a grammar flag,
`Dialect.GroupOpeningAPatternOperandIsRefused`, and the one place a flag
here says what a dialect *will not* read rather than what it adds. Four
columns take the text and one refuses it, so the core takes it.

The last three rows are the controls, and each rules out a wider reading:
`@(` is that shell's own spelling of a group and is read, an escaped
parenthesis is an ordinary character, and a **word** operand takes a
leading `(` in every column — so it is the pattern's reader that refuses
and not the brace. A group that does not *open* the operand — `${v#a@(X)}`
— was always read.

Corpus: `pat/a-group-opening-a-pattern-operand`,
`pat/what-a-refused-leading-group-does-not-reach`.

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

#### A written bar is not a live one

The flag was documented here and in the source as reachable *only* from a
value, on the grounds that the written spelling is a parse error. That is
true of a condition — `[[ a = a|b ]]` is `parse error near '|'` in zsh and
here alike — and of a `case` arm, where the bar is the grammar's own
separator and never reaches the matcher. It is false inside a `${…}`: the
braces keep the bar out of the command grammar and it arrives as pattern
text, which is the one place a written bar can be asked about. Measured on
zsh 5.9.2, 2026-09-12, `v=abc`:

    ${v#a|ab}                            abc   ← written, so an ordinary character
    L='a|ab'; ${v#${~L}}                 bc    ← live, so an alternation
    L='a|ab'; ${v#$L}                    abc   ← not live without the flag
    setopt globsubst; ${v#$L}            bc    ← the option is the same answer
    setopt globsubst; ${v#a|ab}          abc   ← and does not reach a written bar
    ${v#(a|ab)}                          bc    ← a written *group* still splits
    w='a|b'; ${w#a|b}                    ''    ← the written bar matches itself

The last two rows are why this cannot be done by escaping every written bar:
inside a group the bar is the group's separator, written in the one spelling
zsh does read. So `markWrittenBars` walks the assembled pattern the way
`topAlternatives` does — past a group, past a bracket expression, past an
escape — and escapes only a depth-zero bar that no value contributed.

**A live bar splits the whole pattern rather than the value it arrived in**,
which is measured rather than assumed and is the reason the escaping is done
to the *pattern* instead of to each span:

    N='x|abc'; ${v#a${~N}}      empty   ← arms `ax` and `abc`, not `a(x|abc)`
    I='ab|x';  ${v#${~I}z}      c       ← arms `ab` and `xz`, not `(ab|x)z`
    E=a; F=ab; ${v#${~E}|${~F}} abc     ← the bar between two live values is written

Read as concatenation the first two would both answer `abc`. The third is
the control from the other side: two live expansions with a written bar
between them do not make that bar live.

This implementation gave a written bar the live reading, so `${v#a|ab}`
trimmed and answered `bc` (#2168).

### Which arm a longest match takes

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

**The other three trims ask nothing**, which is why the axis is not worded
for trims in general. The single `#` takes the shortest match in every column
whichever arm came first, and both unflagged suffix trims take the longest:

    ${x#(a|ab)}      zsh 5.9.2   bc
    ${x#(ab|a)}      zsh 5.9.2   bc
    ${x%%(|bc)}      zsh 5.9.2   a         an empty first arm does not stop it
    ${x%(c|bc)}      zsh 5.9.2   ab

**The substitution asks the same question**, and that is the second operator
rather than a second axis: it takes the longest match at each position
exactly as `##` does, so the arm the matcher would have preferred is decided
before the matcher is consulted there too. Measured 2026-09-12 with `x=abc`,
and every row splits the same way the trim's do:

    ${x//(a|ab)/X}   zsh 5.9.2   Xbc       the arm written first
    ${x//(ab|a)/X}   zsh 5.9.2   Xc
    ${x/(b|bc)/X}    zsh 5.9.2   aXc       and not only at the start
    ${x/(|a)/X}      zsh 5.9.2   Xabc      an empty arm is an arm
    ${x//(|a)/X}     zsh 5.9.2   XaXbXc    the scan still makes progress
    ${x/#(a|ab)/X}   zsh 5.9.2   Xbc       `/#` pins the start, not the end
    ${x//@(a|ab)/X}  bash 5.3    Xc        the longest arm

**The boundary is the shape of the match and not the operator's name.** The
question is there wherever the *longest* match is wanted **and** the end of
that match is free to move. `/%` pins the end, so every match at a given
start is the same length and the arms have nothing to disagree about; zsh's
`(S)` flag asks for the shortest match, which is the minimum over every arm,
so the first arm that matches at all matches exactly there:

    ${x/%(c|bc)/X}      zsh 5.9.2   aX
    ${x/%(bc|c)/X}      zsh 5.9.2   aX
    ${(S)x//(a|ab)/X}   zsh 5.9.2   Xbc
    ${(S)x//(ab|a)/X}   zsh 5.9.2   Xbc

`Semantics.LongestMatchTakesTheWrittenArm` is the answer, and it is asked
**only where the two readings land in different places**: `(ab|a)` — the
arms in decreasing length — has two readings that agree, and a pattern with
no alternation has one. A live top-level bar is read the same way as a
written group, measured through `${~L}`.

The field was called `LongestPrefixTrimTakesTheWrittenArm` while only the
trim consulted it, and the substitution went its own way at status 0 for as
long as the name said the question was the trim's — a differential sweep of
8800 flag × pattern × operator combinations found 76 differences against zsh
5.9.2 and every one of them was this (#2152).

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

### A bare group's alternatives are each a place the pattern begins

The rows above are about a **quantified** group, which is bash's and
ksh93's. The dialect with **bare** groups answers the same question its
own way, and it looks inside. Measured 2026-09-12 on zsh 5.9.2 with
`extendedglob` on, in a directory holding `.hidden` and `plain`:

| written | matched |
| --- | --- |
| `.hidden` | `.hidden` |
| `\.hidden` | `.hidden` — an escaped period is still a period |
| `.(hidden\|x)` | `.hidden` — the period stands outside the group |
| `(.hidden\|plain)` | `.hidden plain` |
| `(x\|.hidden)` | `.hidden` — a later alternative counts |
| `(.\|x)hidden` | `.hidden` — the alternative need not be the whole name |
| `(.hidden)` | `.hidden` — a group of one |
| `((.hidden))` | `.hidden` — and one nested in another |
| `(.h*)` | `.hidden` |
| `(\|.hidden)` | `.hidden` — past an empty alternative |
| `(#i)(.HIDDEN\|x)` | `.hidden` — past a pattern-flag group |
| `(.hidden\|plain)*` | `.hidden plain` |
| `[.]hidden` | *nothing* |
| `?hidden` | *nothing* |
| `*` | `plain` |

So the period has to be a **literal** one the pattern could match first,
and the places a pattern can begin are: the front of it, the front of
every alternative of a group standing there, and whatever follows a
`(#…)` flag group, which matches nothing itself.

The bracket row is what says this is not "could this pattern match a
leading period" — `[.]hidden` plainly could — but "is one written there".

This shell answers all fifteen. It is the bare-group reading only: the
quantified spelling keeps the bash 3.2 answer recorded above, and the two
are separate flags on the matcher for the reason they are separate in the
panel.

**It is load-bearing well beyond dotfile listings.** powerlevel10k builds
one alternation out of every anchor file it knows — `.git`,
`.tool-versions`, `go.mod`, `package.json` and the rest — and asks
`[[ -n $dir/${~MARKER}(#qN) ]]` of each component of the working
directory. A reading that looks only at the pattern's first byte finds
`(` there, refuses every dotfile in the alternation, and answers "no
anchor" for every directory whose marker is a dotfile — which is most of
them (#2119).

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

### A quantified group may be written with no arms at all

`+()`, `@()`, `*()`, `?()` and `!()` are groups, and by the rule above a
group with no arms stands for nothing. Measured 2026-09-12, `shopt -s
extglob` on a line of its own:

| probe | bash 5.3 | ksh93 |
| --- | --- | --- |
| `case z in +()z)` | match | match |
| `case "" in @())` | match | match |
| `[[ pqr == *()pqr ]]` | true | true |
| `echo +()z` | `+()z` | `+()z` |

The empty spelling is worth a rule of its own because it collides with the
one construct a parenthesis after a word means elsewhere:

    a+() { echo fn; }

With the quantified groups **in force** that is not a function definition in
either shell — the `+(` opens a group, which leaves the name `a` standing on
its own in front of a brace group, and both answer with a syntax error
naming the `}`. With them off it defines a function called `a+` and both run
it. So the option changes what a *definition* means and not only what a
pattern means, and the group wins wherever it is read at all.

The bare group of the other dialect is the opposite case and is why this
needs saying: there a parenthesis is opened by nothing in front of it, so
`a()` really is a definition and an empty group has no spelling. Pinned:
`pat/an-empty-quantified-group` and
`pat/an-empty-quantified-group-is-not-a-function-definition`.

### A bracket expression inside a quantified group is the group's

The four characters that end a word where they stand inside a group — `;`,
`<`, `>` and `&`, the list under *A group is not a second language* — are
pattern text inside a bracket expression. Measured the same day:

| probe | bash 5.3 | ksh93 | zsh |
| --- | --- | --- | --- |
| `case "x;y" in x@([;])y)` | match | match | **parse error** |
| `case "x<y" in x@([<])y)` | match | match | **parse error** |
| `case "x&y" in x@([&])y)` | match | match | **parse error** |
| `case "x;y" in x[;]y)` | **error** | **error** | **error** |

The last row is what scopes the rule. The identical brackets *outside* a
group are refused everywhere, so it is the group that protects them rather
than the brackets, and a reading that let brackets protect anywhere would
accept three lines no shell in the panel parses.

zsh's column is a bare group and not a quantified one — it reads the `@` as
an ordinary character — and it refuses the construct in a condition and in a
`case` alike, with and without `extendedglob`. So the reading belongs to the
quantified group, which the character in front of the parenthesis is what
identifies.

The extent of the bracket expression is POSIX XCU 2.13.1's and not "the next
`]`": a `]` first — after the negation, where there is one — is the
character rather than the closer, and `[:class:]`, `[.collating.]` and
`[=equivalence=]` each hold a `]` that closes only themselves. A run that
reaches a newline is not a bracket expression, which is what bounds how much
an unpartnered `[` can take into the group. Pinned:
`pat/a-bracket-expression-inside-a-quantified-group` and
`pat/a-bracket-expression-outside-a-group-does-not-protect`.

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
of an element opens one that belongs to the word. That wording is history
twice over: the counting went in #1149, and the message itself went in
#1162, where the production learned to name the token it found. So does a group on a
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
as written and misses. `(#i)` and its neighbors are the `(#q…)` form
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
    for x ((#i)a); do :; done               accepted
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

**The parenthesised list's own opening paren** used to be the one shape
here that was refused, and it was a lexing question rather than a
word-position one: `for x ((#i)a)` reaches the lexer as `((`, was taken
for an arithmetic command, and never got as far as being a list at all. A
*later* element — `for x (a (#i)b)` — read the flag because its paren
stands after a word.

It needed no position of its own in the end. `((` is ambiguous wherever a
command may begin, and the arithmetic reading is now *tried* and
abandoned where it does not close on an adjacent `))` — see
[commands.md](commands.md#-is-ambiguous-and-the-reading-is-given-up-where-it-does-not-close). The `)` in
`((#i)a)` is followed by `a`, so the reading is given up and the `(` is
the list's, which is exactly what that shell does with it:
`setopt extendedglob; for x ((#i)a); do echo "[$x]"; done` prints `[a]`
on zsh 5.9.2, and without the option the same line is `no matches found:
(#i)a` — a match failure, so the header parsed either way (#3052).

**`;` as an element separator inside an array literal** is the thing
still outstanding from this family, which that shell and ksh93 accept.

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
argument for the link count, the three file times `m`, `a` and `c` with
their unit letter and signed number, the `-` that follows a link before
testing, the `^` that turns any of them, the `,` that unions them, `N`
and `D`, and the `:` that opens a modifier list.

The three times are one qualifier with three clocks behind it, and they
were one letter until #3533: `m` reads the modification time, `a` the
access time and `c` the inode change time, and the unit letter, the sign
and the number are read the same way for each. `[.x.]`-shaped refusals
aside, the letter and the operand are two complaints — measured
2026-09-18 on zsh 5.9.2, `zz*(a)` with no number behind it is `number
expected` where an unimplemented letter is `unknown file attribute: a`,
so a column that had neither said the wrong thing about which half was
wrong. `fs.FileInfo` carries only the modification time, so the other two
are read from the platform's own stat fields, which are spelled
differently on each — a per-GOOS file beside the qualifiers, the shape the
signal table already takes.

Everything else in that language — `=` for a socket, the `%b` and `%c`
spellings that separate the two kinds of device, `e` and `+` for a
command's verdict, `d` for a device, `o`, `O`, `Y` and `[n,m]` for
ordering and counting, `L` for a size, and
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
| `nocasematch` | `case`, both `[[ ]]` operators — the glob `==` and the regexp `=~` — and the **substitution** operators of parameter expansion fold case: `case A in a)` matches, `[[ ABC =~ ^abc$ ]]` matches and `v=ABC; ${v//b/X}` is `AXC`, while pathname expansion, the trims `${x#pat}` and `${x%pat}`, and the case-change operator `${x^^pat}` stay exact |
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

`=~` is the other surface the option reaches, and it is a different
mechanism rather than one more place the same comparison happens: the right operand is a regular
expression, so what folds is the compiled expression and not a letter at
a time. The fold therefore reaches **the whole of it**, which a fold over
the pattern's literal characters would not. Measured 2026-09-13 on bash
5.3.15, and the same answers on bash-as-`sh` and bash 3.2.57, with the
option on —

| written | answer |
| --- | --- |
| `[[ ABC =~ ^abc$ ]]` | matches |
| `[[ abc =~ ^ABC$ ]]` | matches |
| `[[ ABC =~ ^[a-c]+$ ]]` | matches — a range folds |
| `[[ ABC =~ ^[[:lower:]]+$ ]]` | matches — so does a class |
| `[[ abc =~ ^[[:upper:]]+$ ]]` | matches |
| `[[ A =~ ^[^a]$ ]]` | does **not** — the fold precedes the negation |
| `[[ ABC =~ ^(a)(B)c$ ]]` | matches, and `BASH_REMATCH` is `ABC A B` |

The last two are the rows a fold bolted on around the match cannot
reproduce: `[^a]` has to exclude `A` as well, which needs the fold to
happen before the class is complemented, and the captures have to be
spans of the subject as the script wrote it rather than of a folded copy
of it. Which is why this implementation sets the engine's own case flag
on the expression instead (#2622).

**The name is bash's, and zsh's `nocasematch` is not the same feature.**
Measured 2026-09-13 on zsh 5.9.2, with `setopt nocasematch`:

| written | zsh | bash |
| --- | --- | --- |
| `[[ ABC =~ ^abc$ ]]` | matches | matches |
| `[[ ABC == abc ]]` | does not | matches |
| `case A in a)` | no | yes |
| `v=ABC; ${v//b/X}` | `ABC` | `AXC` |

So the shared spelling covers one surface there and every one of them
here, and the core keeps two switches — `MatchFoldsCase` and `RegexFoldsCase` — with
each dialect wiring the name it spells. `nocaseglob`/`caseglob` is a
third thing again and reaches none of these five: it is pathname
expansion alone. ksh93 has `=~` and no option of the kind at all — its
`~(i)` is an inline flag, the way zsh's `(#i)` is — and dash and BusyBox
ash have neither `[[ ]]` nor `=~`.

One cell is measured and not yet answered here: **an explicit `C` or
`POSIX` locale narrows the fold to ASCII**, so `LC_ALL=C` makes
`[[ ÉTÉ =~ ^été$ ]]` fail in bash 5.3.15 and in zsh 5.9.2 where the same
line under a UTF-8 locale matches. The engine's case flag has no locale
to be told about, so ours folds it either way. The glob half of `[[ ]]`
has the same gap pointing the other way — its fold is a byte-wise ASCII
one, so `[[ ÉTÉ == été ]]` is exact here under every locale and matches
in bash under a UTF-8 one. Both are #2644.

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

**The fold stops at a POSIX character class inside a glob bracket**, and
that is a class-versus-range seam rather than "the option does not reach
brackets". Measured 2026-09-14 under `LC_ALL=C`:

| written, with `nocasematch` on | bash 5.3.15 | bash 3.2.57 |
| --- | --- | --- |
| `[[ A == [[:lower:]] ]]` | exact | fold |
| `[[ a == [[:upper:]] ]]` | exact | exact |
| `[[ A == [a-z] ]]` | fold | fold |
| `case A in [[:lower:]])` | exact | fold |
| `v=ABC; ${v//[[:lower:]]/X}` | `ABC` | `ABC` |
| `[[ A =~ ^[[:lower:]]$ ]]` | fold | fold |

The range row folds in every column, so a reading that simply left
brackets alone matches nobody. The `=~` row folds because there the fold
belongs to the compiled expression and never reaches this matcher at all.
bash 3.2 is asymmetric with *itself* on the first two rows — no single
rule about the option explains it — and no preset here is bash 3.2, so
5.3 is the column followed and the older one is recorded.

`nocaseglob` answers the same way on the same seam: with `A` and `b` in
the directory, `[[:lower:]]` yields `b` and `[a-z]` yields `A b`.

Which fold is asking decides it, which is the reason the two are separate
fields in the matcher rather than one. ksh93's inline `~(i)` flag — the
table under *Pattern modifiers* below — folds the class as well as the
range, so an implementation with one switch has to pick a side and will
be wrong for one of the two shells. This one folded the class for every
caller until #2716, which is ksh93's answer given to bash.

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

### What a `**` that stood for nothing is called

The zero-level half of the component has a spelling, and it is not the
one the directory has. Measured 2026-09-13 against bash 5.3.15 under
`shopt -s globstar`, in a tree holding `a/b/c`, `a/f1`, `a/b/f2`,
`a/b/c/f3`, `d/e/f4` and `top`:

| pattern | the zero-level match is called |
| --- | --- |
| `a/**` | `a/` |
| `a/b/**` | `a/b/` |
| `a//**` | `a//` |
| `"a"/**` | `a/` |
| `*/**` | `a` |
| `?/**` | `a` |
| `a/*/**` | `a/b` |
| `a/**/**` | `a` |
| `**/c/**` | `a/b/c` |

So the separator is not a property of the directory: `a/**` and `*/**`
report the same directory two different ways, and the only difference
between the two patterns is a component nobody looked at. **It is kept
exactly where everything ahead of the component was spelled out rather
than described**, and the separators already standing are reported as
written — `a//**` is `a//`, not `a///`. Quoting the prefix does not
change the answer, so the question is about the pattern's
*metacharacters* and not about its source text.

ksh93 gives a second reading and this shell does not implement it: it
drops the zero-level match altogether where the prefix is spelled out,
so `a/b/**` is what lies beneath `a/b` and `a/*/**` is `a/b a/f1 …` —
the mirror image, and it counts a plain file as a zero-level match
where bash keeps only directories. Nothing here can reach it, because
`set -o globstar` is not wired into that dialect.

zsh never reaches the question: its bare `**` does not cross levels at
all, and with a slash behind it the separator comes from the pattern.

### A `**` never enters a link, and two questions follow anyway

Every column with the construct refuses to descend **through** a symbolic
link, which is what keeps a `**` finite on a tree holding a link to its own
ancestor. Two separate questions survive that, and the panel answers them
with different columns. Measured 2026-09-18 in a directory holding `r/x`,
a symlink `s` to `r`, a file `y` and a symlink `up` to the directory itself,
with `a/b/sl` a second symlink to `r`:

| pattern | bash 5.3.20 | ksh93u+ | zsh 5.9.2 |
| --- | --- | --- | --- |
| `**/` | `r/ s/ up/` | `r/ s/ up/` | `r/` |
| `**/x` | `r/x` | `r/x` | `r/x` |
| `./**/x` | `./r/x ./s/x` | `./r/x` | `./r/x` |
| `a/**/x` | `a/b/sl/x` | no match | no match |
| `./**/y` | `./up/y ./y` | `./y` | `./y` |

**Is a linked level one of the names `**/` reports?** bash and ksh93 say yes,
zsh says no — `StarStarSeesLinkedDirectories`, and it is the first two rows.

**May the component *behind* a `**` look inside one?** bash says yes and both
others say no, which is the opposite pairing and is why this is a second
switch rather than the first read twice —
`ComponentBehindStarStarSeesLinkedLevels`. The set the next component is
offered widens; the set the walk enters does not, so the last row is still
finite in every column.

**The column that says yes exempts a `**` that begins the word**, which is
measured rather than chosen: `**/x` and `./**/x` name the same files by every
other rule and are answered differently there, and an absolute spelling,
`**//x`, and `a/**/**/x` all look inside. The exemption is that narrow — the
word's first component, with one separator and a real component behind it —
and modeling the shell means modeling it, since a pattern written either way
is a pattern scripts write (#3176).

### A descent reads a listing, so it holds whatever a listing holds

`Semantics.GlobListsDotAndDotDot` says whether a pathname expansion's listing
holds `.` and `..` beside the entries, and a `**` descent reads one listing
per level. It is the same answer at a second place and not a second answer.
Measured 2026-09-18 with the ignore parameter set so the leading-period rule
is off and the two names are visible at all, in a tree holding `topf`,
`p/pf`, `p/q/qf`, `p/q/w/leaf` and `p/q/w/q/deepq`:

| pattern | the column whose listing holds them |
| --- | --- |
| `**` | `. .. p p/. p/.. p/pf p/q p/q/. p/q/.. p/q/qf … topf` |
| `**/` | `../ ./ p/ p/../ p/./ p/q/ …` |
| `**/qf` | `p/q/qf` |
| `p/**` | `p/. p/.. p/pf p/q p/q/. …` |

**Both names are produced and neither is followed.** That is a rule of its
own rather than the axis: a walk descending into `..` would climb out of the
tree it was given and never stop, and no column does that. The third row is
what says so — neither name is a level the walk entered, so a real component
behind the `**` never arrives through one — and the second is the control,
since both names are directories and the trailing-slash form keeps them.

**The leading-period rule is what keeps them out of an ordinary descent**,
exactly as it keeps them out of an ordinary `*`: with nothing asked, `**` is
`p p/pf p/q p/q/qf topf` in that column too (#3175).

### Nothing takes duplicates out, so a run of `**` is an axis

**A pathname expansion is not a set.** Two `**` components are two
alternatives, each standing for zero or more levels, so a name
reachable by several splits is written once per split. Measured over a
tree of directories `a` and `b` nested three deep:

| pattern | bash 5.3 `-s globstar` | ksh93 `-o globstar` | zsh 5.9.2 |
| --- | --- | --- | --- |
| `**/a/**` | 17 names, `a/a/a` three times | the same | — |
| `**/a/**/` | 17 names | the same | the same |
| `a/**/**/b` | `a/a/b a/b a/b/b` | the same | each of the two deeper ones twice |
| `**/**/` | 14 names, each once | the same | 48 names |

The last row is the axis, and the third is the same axis read from the
other side. **bash and ksh93 read a run of `**` components as one
component**; zsh does not, so its `**/**/` is the cross product and
names a directory three deep four times. `RepeatedStarStarIsOneComponent`
carries it, consulted only where `**` crosses levels at all, and the
collapse takes the separators inside the run with it — `**//**` is `**`
in bash, where an ordinary empty component is reproduced (`cx//*` is
`cx//ax` in all six columns).

The two sections interact and the order is measured. `a/**/**` and
`a/**` list the same names and spell the zero-level one differently, so
what the separator rule reads is the field **as written** — before the
run is collapsed, and before the walk has looked at anything.

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
that refuse `-u` rather than `-s` — `cmdhist`, `complete_fullquote`,
`extquote`, `globasciiranges`, `interactive_comments` and `promptvars` —
refuse it because the behavior they name is simply how this shell works. Asking for
the state already held is granted in both directions, which is the same
bargain `set +o posix` strikes.

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
own state and it belongs with the rest of what is true.

`histappend` and `lithist` sat beside it reading **on** against bash's off,
on the same argument — this shell's history appended to the file and kept a
multi-line entry's newlines, measured through a terminal. Both are switches
now, at bash's own default, and neither moved because the argument was
dropped: the behavior each names was built, so the name is a switch rather
than a state nothing could leave (#4149). `lithist` was the honest half —
this shell really did keep the newlines, which is the option's own side —
and `histappend` was the half only a sharper measurement could settle. bash
with that option **off** appends too, in every case but one: it rewrites the
file from its list where HISTSIZE has trimmed the list below the number of
lines the session added. So the two shells differed in exactly that corner
and the table was describing the wrong half of the option. See
`interp.Runner.HistoryJoinsATypedCommand` and `RewritesTheHistoryFile` for
the rows.

The seventh, `patsub_replacement`, was the one left reporting **off**
because the behavior it gates was unbuilt, and reporting it on would have
been the worst kind of lie. It is built now and the name is wired to
`interp.ReplacementAmpersandIsTheMatch` (#1862); the section below is what
it switches.

These are run-time states rather than semantics axes, which is why the
core holds them as `interp.MatchOption` values and only the bash dialect
maps names onto them. `failglob` is one of them — `interp.UnmatchedPatternIsError`
— and its miss abandons the rest of the current *statement* and then carries
on, which is what `interp.Semantics.FailedExpansionAbandonsTheLine` answers:
measured, `shopt -s failglob` then `echo zz*zz; echo after` on one line
prints neither, and `echo after` on the next line prints.

## A parameter that takes names back out of an expansion

bash has one, spelled `GLOBIGNORE`, and it is not a pattern switch: it is
a parameter whose value is a **colon-separated list of patterns**, and
every word a pathname expansion produced that one of them matches is
taken back out.

Measured on bash 5.3.15 and bash 3.2.57, 2026-09-13, in a directory
holding `a.txt`, `b.txt`, `c.log`, `.dot`, `.hid.txt` and a directory
`sub` that holds `x.txt` and `.y`. The two builds answer alike on every
row below **but one**, marked where it falls; the separator rule and the
fold are 5.x's rather than bash's. No preset here is bash 3.2, so both
differences are recorded and neither is given an axis.

| written | answer |
| --- | --- |
| `GLOBIGNORE='*.txt'; echo *` | `.dot c.log sub` |
| `GLOBIGNORE='a.txt:c.log'; echo *` | `.dot .hid.txt b.txt sub` |
| `GLOBIGNORE='a.txt:'; echo *` | `.dot .hid.txt b.txt c.log sub` |
| `GLOBIGNORE='*.txt'; echo *.txt` | `*.txt` |
| `GLOBIGNORE='*x.txt'; echo */*` | `sub/.y sub/x.txt` — **3.2.57: `sub/.y`** |
| `GLOBIGNORE='*/x.txt'; echo */*` | `sub/.y` |
| `GLOBIGNORE='sub/*'; echo */*` | `*/*` |
| `GLOBIGNORE='sub'; echo */` | `sub/` |
| `GLOBIGNORE='sub/'; echo */` | `*/` |
| `GLOBIGNORE='./a.txt'; echo ./*.txt` | `./.hid.txt ./b.txt` |
| `GLOBIGNORE='a.txt'; echo ./*.txt` | `./.hid.txt ./a.txt ./b.txt` |

Five rules come out of those rows.

**The list is split on colons and an empty element is no pattern**, so a
leading or trailing colon ignores nothing rather than everything.

**Each pattern is matched against the word the expansion produced,
spelled the way the expansion spelled it.** The `./` a pattern wrote in
front of a name is part of the word and has to be part of the ignore
pattern; so is the `/` a trailing-slash pattern wrote behind it.

**A separator in the word has to be matched by a separator in the
pattern.** A `*` here stops at a `/` exactly as one in the expansion's own
pattern does, which is why `*x.txt` does not reach `sub/x.txt`. There is
**no leading-period rule** to go with it — `sub/*` does take `sub/.y` out
— so only one half of the rule the walk itself follows survives here.

This is the row where the builds part: **bash 3.2.57 lets the `*` cross the
separator** and answers `sub/.y`, the same as the row below it. The two rows
are a pair for exactly that reason — 3.2.57 agrees on the second, so its
disagreement is about the separator and not about having the parameter at
all. The corpus carries both, and the golden record holds `bash32`'s own
answer beside `bash`'s.

**A word the patterns left with nothing is a word that matched nothing.**
The pattern stands where no option says otherwise, `nullglob` deletes the
word, and `failglob` refuses it. The operand in those rows matched every
`.txt` name before the filter ran, so this is the parameter's miss and not
the expansion's.

**It reaches pathname expansion alone.** `case`, `[[ ]]` and the pattern
operators of parameter expansion are untouched with it set.

The comparison is under the same fold the expansion is under: on bash
5.3.15, with `nocaseglob` on, an ignore pattern of `*.TXT` takes `a.txt`
out, and with it off it does not. Both halves are the measurement — a
filter that always folded would answer the first and a filter that never
folded the second, so either alone says nothing. **bash 3.2.57 folds
neither way here**, which is the second of the two places the builds part.

### The switch the assignment writes, and why this follows the assignment

A non-null assignment also turns `dotglob` on — the option itself, not a
behavior beside it, which is what these measurements say together:

| written | answer |
| --- | --- |
| `GLOBIGNORE=a; shopt dotglob` | `on` |
| `GLOBIGNORE=a; shopt -u dotglob; echo *` | no hidden names |
| `GLOBIGNORE=a; GLOBIGNORE=; shopt dotglob` | `on` |
| `GLOBIGNORE=; shopt dotglob` | `off` |
| `GLOBIGNORE=a; unset GLOBIGNORE; shopt dotglob` | `off` |
| `shopt -s dotglob; unset GLOBIGNORE; shopt dotglob` | `off` |

So the assignment writes the switch and a script may write it back; the
*unset* writes it the other way whoever set it; and an assignment of
nothing writes neither, which is why a null value is not the same state as
no value at all.

The third and fourth rows are a pair and the fourth is the one that says
anything. A null assignment made where the switch is **already on** is a
row both readings of it answer alike — leaving the switch alone and turning
it on are the same answer there — so it shows only that the assignment was
noticed. The row that starts from the switch off is where the two part.

**A value inherited from the environment does none of this and is not read
at all.** `env GLOBIGNORE='*.txt' bash -c 'echo *'` lists the `.txt`
files and reports `dotglob` off, and assigning the parameter its own value
starts both halves. The facility follows the *assignment* rather than the
value, which is the one place this is a state and not a lookup, and
`interp.Runner.ignoredNamesLive` is that state.

A binding change is an assignment for this purpose: a `local` of the
parameter lasts exactly as long as the call, and the caller's patterns —
and the caller's `dotglob` — come back at the return. A declaration with
**no value** is the unset half rather than the null one, since it leaves
the name unset.

### ksh93 spells it `FIGNORE` and does not answer alike

The facility is two dialects' and not one, which is why the core names it
for what it does — `interp.Semantics.IgnoredNamesVariable`, empty where a
dialect has no such parameter — rather than for bash's spelling.

`GLOBIGNORE` does nothing in ksh93 and `FIGNORE` does nothing in bash; zsh
and dash have neither. Measured on ksh93u+ 2026-09-14 in the same fixture,
the two shells part in **three** places, and each is an axis:

| written | ksh93 | bash 5.3 |
| --- | --- | --- |
| `VAR='*.txt'; echo *` | `. .. .dot c.log sub` | `.dot c.log sub` |
| `VAR='*.txt:*.log'; echo *` | nothing taken out | both kinds gone |
| `VAR='@(*.txt\|*.log)'; echo *` | both kinds gone | — |
| `VAR='*.txt'; echo sub/*` | `sub/x.txt` gone | nothing gone |
| `VAR='sub/x.txt'; echo sub/*` | nothing gone | `sub/x.txt` gone |
| `VAR='sub'; echo */` | `sub/` gone | `sub/` kept |
| `VAR='sub/'; echo */` | `sub/` kept | `sub/` gone |
| `VAR='*.txt' sh -c 'echo *'` | filtered | no effect |
| `VAR=''; echo *` | hidden names shown | no hidden names |

- **The value is one pattern, not a colon-separated list.** No name holds a
  colon, so ksh93's second row takes nothing out; the alternation a script
  wants there is written in the pattern grammar, which is the third row and
  does work. `IgnoredNamesValueIsOnePattern`.
- **The subject is the entry the directory listing gave**, not the word the
  expansion produced: no `./` in front of it and no `/` behind it. The four
  middle rows are the same statement from four sides, and the `sub` / `sub/`
  pair is the sharpest — the two shells are exactly opposite there.
  `IgnoredNamesMatchTheLastComponent`.
- **The facility follows the parameter** rather than latching on an
  assignment: an inherited value works, and a null one still shows the
  hidden names. bash has neither half, because there the hidden-name switch
  is `dotglob` itself and a script may write it back; ksh93 has no such
  option to write. `IgnoredNamesFollowTheParameter`.

The first row's `.` and `..` are **not** this parameter's doing and are a
separate axis: ksh93 lists those two names in every pathname expansion, and
the leading-period rule is what keeps them out of an ordinary `*`. See
`GlobListsDotAndDotDot` in `../grammar/expansion.md` — three columns list
them and three do not, bash disagreeing with itself across versions
(#2748).

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

## Another shell's pattern-modifier prefix, and the one regular expression it has

ksh93 writes a letter set in parentheses after a `~` in front of a pattern,
and the letters say what language the pattern is written in and how it is
compared. It is that shell's **only** spelling for a regular expression —
bash has `=~`, zsh has `=~`, and ksh93 has this — so a ksh script wanting
an ERE has nowhere else to go.

**It is a lexical rule first.** A `(` straight after a `~` belongs to the
word, wherever the word stands and whether or not it is being matched.
Measured on ksh93u+ 2012-08-01, 2026-09-13, `env -i` with a scratch `HOME`,
`-c`:

| written | ksh93 |
| --- | --- |
| `echo ~(E)abc` | `~(E)abc` — an ordinary word, kept as written |
| `x=~(Z)abc; echo "$x"` | `~(Z)abc` — and so is an assignment's value |
| `echo a~(x)b` | `a~(x)b` — mid-word too |
| `[[ abc == ~(E)a.c ]]` | matches |
| `case abc in ~(E)^a.c$)` | matches |
| `s=aXbXc; ${s//~(E)X/-}` | `a-b-c` |

The other five columns refuse the paren while reading the file — bash 5.3.15,
that binary as `sh`, bash 3.2.57, dash and BusyBox ash, unanimously, at parse
time. zsh neither refuses nor honors it: there `~` is the exclusion operator,
so `~(E)abc` is a pattern that reads and does not match. So this is
`syntax.Dialect.TildeGroup`, a grammar flag one dialect holds, and not an axis:
nobody else has a reading of the construct to disagree about.

### And the group turns what follows it into the flavor's text

Reading the group was not the whole of the lexical rule, and the half that
was missing is the half a real ERE needs: two of the three metacharacters
that make an extended regular expression *extended* are also the shell's.
`[[ abcd == ~(E)(ab)cd ]]` was ``syntax error at line 1: `(' unexpected``
here, on a flavor this tree ships.

**It is not that a `[[ ]]` operand reads parentheses as pattern characters.**
ksh93 refuses them bare exactly as this shell does, which is the control that
separates the two readings:

| written | ksh93u+ | ours, before and after |
| --- | --- | --- |
| `[[ abcd == (ab)cd ]]` | ``syntax error … `(' unexpected`` | the same |
| `[[ ab == a\|b ]]` | ``syntax error … `\|' unexpected`` | the same |
| `[[ abcd == @(ab)cd ]]` | matches | the same |

The rule has **two halves with different reaches**. Measured on ksh93u+
2012-08-01, 2026-09-19, each probe its own `env -i PATH=/usr/bin:/bin
LC_ALL=C /bin/ksh -c …` with standard input on `/dev/null`.

**A `(` belongs to the word wherever the word stands**, balanced and
nesting:

| written | ksh93u+ |
| --- | --- |
| `print -r -- ~(E)(ab)cd; echo AFTER` | `~(E)(ab)cd` then `AFTER` |
| `x=~(E)(ab)cd; print -r -- "$x"` | `~(E)(ab)cd` |
| `case abcd in ~(E)(ab)cd) …` | matches, and the arm's own `)` closes it |
| `[[ abcd == ~(E)(a(b))cd ]]` | 0 — so it nests |
| `[[ abcd == x~(E)(ab)cd ]]` | 1, nothing on stderr — mid-word too |

**The shell's other operators stop separating only in the right operand of
a pattern comparison.** Each row carries the subject that does *not* match,
because the status alone cannot tell "the character is the pattern's" from
"the character was the shell's and the list happened to answer 0":

| written | ksh93u+ | what a shell reading would have done |
| --- | --- | --- |
| `[[ zzz == ~(E)ab\|zz ]]` | 0, and `[[ qqq == … ]]` is 1 | a syntax error |
| `[[ "a&b" == ~(E)a&b ]]` | 0, nothing on stderr | `[[ … ]] &` then `b`, so **0 either way** |
| `[[ zzz == ~(E)a&b ]]` | **1**, nothing on stderr | still 0 from the async, plus `b: not found` |
| `[[ "a;b" == ~(E)a;b ]]` | 0, nothing on stderr | `[[ … ]]; b`, so `b: not found` at 127 |
| `[[ "a<b" == ~(E)a<b ]]` | 0, and `[[ zzz == … ]]` is 1 | a redirection |
| `[[ "a>b" == ~(E)a>b ]]` | 0, and `[[ zzz == … ]]` is 1 | a redirection |
| `[[ "a)b" == ~(F)a)b ]]` | 0 | a syntax error |

The `&` and `;` rows are the two worth keeping: a probe that asked only
whether the whole line came back 0 cannot tell them apart. The **subject**
is what separates the hypotheses, not the status.

And the six refusals that put the boundary exactly there, every one of them
a `syntax error` in ksh93u+ as well as here:

| written | why it is the boundary |
| --- | --- |
| `print -r -- ~(E)a;b` | `~(E)a`, then `b: not found` — an ordinary word |
| `print -r -- ~(E)a\|cat` | a pipeline — the `\|` is the shell's |
| `printf '[%s]' ~(N)x; echo` | the `;` ends the command |
| `[[ ~(E)a;b == "a;b" ]]` | the **left** operand |
| `[[ -n ~(E)a;b ]]` | a unary operand |
| `case "a;b" in ~(E)a;b) …` | a `case` arm |

Quoting and expansion are untouched by either half — `~(E)"a b"`,
`~(E)a${v}c` and `~(E)a$(print b)c` behave as they would anywhere, and
`~(E)a'b` is still an unterminated quote. An unquoted blank or newline
still ends the word: `[[ abcd == ~(E)(ab) cd ]]` is ``syntax error at line
1: `cd' unexpected``. So this is a list of characters that stop
*separating*, not a raw-text mode.

**Two measured rows are not modeled, deliberately.**

- `case "a|b" in ~(E)a|b)` matches there, so a `case` arm reads the `|` as
  the pattern's while reading the `;` as the shell's. A `case` arm is not a
  pattern comparison's right operand here, so the `|` stays the arm's
  separator — the narrower reading, and the one that cannot swallow an arm.
- `[[ xabc == x~(E)a.c ]]` matches there and does not here, because a
  `~(…)` group that is not at the head of the word reaches no modifier in
  the matcher. That is the matcher's question rather than the lexer's, and
  it answers the same before this rule and after it.

Grammar: `syntax.Lexer.afterTildeGroup`, set when the group is taken and
cleared at the end of the word (#3808).

### The letters, measured one at a time

Two probes separate the readings. `[[ abc == ~(X)a.c ]]` tells a regular
expression from a glob, since `a.c` describes `abc` only where the `.` is a
metacharacter; `[[ xabcx == ~(X)a.c ]]` tells a **substring** search from a
whole-string one.

| letter | what the probes say |
| --- | --- |
| `E` | an extended regular expression, matching a substring |
| `G` `V` | a **basic** regular expression: `^a.c$` and `a.c` match and `a?c` does not, which is grep's *basic* syntax. Eighteen probes separate the two letters on nothing |
| `X` | the extended one plus a single operator — `&`, a conjunction over the same span. Nine further probes separate it from `E` on nothing |
| `P` | a Perl regular expression: `\d`, `\w`, `\s`, a lazy `.*?` and an inline `(?i)` are each read |
| `A` `B` | each answers as `E` does on every probe written here |
| `F` | a literal string, matching a substring: `~(F)a.c` is the three characters |
| `L` | a literal string as well, and no probe here separates it from `F` |
| `K` | the ksh glob, which is what a pattern with no prefix already is |
| `M` `O` `S` `U` `a` `m` `x` | accepted, and no probe here makes any of them change an answer |
| `p` `s` | the shell glob, the same reading `K` has — re-measured 2026-09-19: `~(p)a?c` and `~(s)a?c` match `abc` while `~(p)a.c` and `~(s)a.c` do not, so the dot is an ordinary character in each and neither is a regular expression |
| `g` | the match takes as much subject as it can from where it begins: `v=aXbXc; ${v#~(g)*X}` is `c` where `${v#*X}` is `bXc`. It is **not** a global replacement — `${v//~(g)X/-}` and `${v/~(g)X/-}` are what they were without it — and it leaves a suffix trim alone, because that trim is pinned at the far end: `${v%~(g)X*}` is `aXb`, the same shortest suffix `${v%X*}` takes |
| `N` | the word is deleted where the pattern names nothing, which is what zsh spells `(N)` and bash needs an option for: `printf "[%s]" ~(N)zz*` writes nothing, and `~(N)zzz` goes too where no file is named that |
| `i` | case-insensitive, and the fold reaches a bracket and a character class as well as a literal — one place further than `nocasematch` reaches, which is measured and is the pairing #2716 is about |
| `l` `r` | left and right anchors, which only a substring flavor can show |
| `+` `-` | turn the letters after them on and off |

`~()` is a group that says nothing, and matches. A letter ksh93 does **not**
have is a pattern that cannot match rather than one that cannot be read:
`[[ abc == ~(Z)abc ]]` and `[[ abc == ~(Z)* ]]` are both status 1 with nothing
on standard error.

`~(G)` is worth naming because the issue that filed this guessed it meant
"glob, said explicitly". It does not; `K` does. The guess would have read
`~(G)a?c` as a glob where that shell reads the `?` as a literal character.

A word carrying a group with a **letter** in it is a pattern for pathname
expansion, whatever else is in it, and that is what makes the letters reach a
name with no glob character:

| written, in a directory holding `a.txt` and `b.txt` | ksh93u+ |
| --- | --- |
| `~(i)a.txt` | `a.txt` |
| `~(E)a.txt` | `a.txt` |
| `~(F)a.txt` | `a.txt` |
| `~(N)a` | deleted — nothing is named `a` |
| `~()a.txt` | `~()a.txt` — an **empty** group does not |
| `~(E)zzz` | `~(E)zzz` — a miss without `N` stands as it was written |

The empty-group row is the control that says this is the letters and not the
parentheses.

The anchors:

| written | `xabcx` | `abcx` | `xabc` | `abc` |
| --- | --- | --- | --- | --- |
| `~(E)a.c` | matches | matches | matches | matches |
| `~(El)a.c` | no | matches | no | matches |
| `~(Er)a.c` | no | no | matches | matches |
| `~(Elr)a.c` | no | no | no | matches |

### What this shell honors, and what it refuses by name

Honored: `E`, `F`, `G`, `K`, `L`, `N`, `P`, `V`, `X`, `g`, `i`, `l`, `p`, `r`,
`s`, the `+`/`-` toggles and the empty group. Every regular-expression flavor
is compiled by Go's `regexp`, which is the engine `=~` already uses here —
`E`, `X` and `P` as written, `G` and `V` translated from basic syntax first.

`A`, `B`, `M`, `O`, `S`, `U`, `a`, `m` and `x` are **refused by name rather
than accepted and ignored** — `<pattern>: the ~(A) pattern modifier is not
implemented`, at status 1. `A` and `B` agreeing with `E` on the probes above
is not evidence that they *are* `E`, and no probe gives the rest anything to
do. A flag taken and dropped is a wrong answer at status 0, which is worse
than a refusal.

### The four regular-expression letters, and what each is

They were all four refused until #3186, on a reading that turned out to be
wrong twice over: that they behaved as `E` did, and that one of them alone
needed something the engine here lacks. Re-measured a pattern at a time on
2026-09-20 with the pattern supplied **through a variable**, since the shell's
own quote removal reaches a written one and a written `&` is its async
operator.

`G` and `V` are a **basic** regular expression, which is the mirror of `E` for
seven operators — a backslashed `(`, `)`, `{`, `}`, `|`, `+` and `?` is the
operator and a bare one is the character:

| written | ksh93u+ | reading |
| --- | --- | --- |
| `a\(b\)c` over `abc` | matches | `\(…\)` groups |
| `a(b)c` over `a(b)c` | matches | a bare paren is the character |
| `a\{3\}` over `aaa` | matches | `\{…\}` is the interval |
| `a{3}` over `a{3}` | matches | a bare brace is the character |
| `a\|b` over `ab` | matches | `\|` alternates |
| `a|b` over `a|b` | matches | a bare bar is the character |
| `a\+b` over `aab` | matches | `\+` repeats |
| `a+b` over `a+b` | matches | a bare plus is the character |
| `ax\?b` over `ab` | matches | `\?` is zero or one |
| `a?b` over `a?b` | matches | a bare question is the character |

The anchors there are positional rather than always live, and one of the four
rows is the one a translation is most likely to get wrong:

| written | ksh93u+ | reading |
| --- | --- | --- |
| `x^y` over `x^y` | matches | a caret inside is the character |
| `x$y` over `x$y` | matches | and so is a dollar inside |
| `\(^a\)b` over `ab` | matches | a caret just past `\(` anchors |
| `\(^a\)b` over `xab` | no | the control for the row above |
| `a\(b$\)` over `ab` | matches | a dollar just before `\)` anchors |
| `a\(b$\)` over `abx` | no | the control |
| `^a\|^b` over `ab` | matches | on the first branch's anchor |
| `^a\|^b` over `b` | **no** | so a caret just past a `\|` is the character |

**`V` is not distinguished from `G` by any of the eighteen probes written**,
those rows included, so the two compile alike here and the pair is recorded
rather than guessed at.

`X` is `E` plus exactly one operator, and finding which one took nine probes
that separate the two letters on nothing — `.` spanning a newline, `(?i)`,
`.*?`, `[[:alpha:]]`, the anchors, groups, alternation, `+` and `\<`, which
neither has:

| written | `~(E)` | `~(X)` |
| --- | --- | --- |
| `a.c&abc` over `abc` | no | **matches** |
| `a.c&axc` over `abc` | no | no — the control |
| `a&b` over `a&b` | matches | no |

So `&` is a conjunction there. Its operands describe the **same span** rather
than the same subject, which is the reading a pair of independent searches
would get wrong:

| written | ksh93u+ | reading |
| --- | --- | --- |
| `a&c` over `abc` | **no** | `a` and `c` are each in the subject and there is no span holding both |
| `(a.c)&(abc)` over `abc` | matches | the control that says the operator works |
| `a.&.b` over `ab` | matches | one span, described twice |
| `ab&b` over `ab` | no | no span is both |
| `^abc&abc$` over `abcabc` | **no** | the anchors are the subject's, not the span's |
| `^ab&ab` over `abab` | matches | the control for the caret half |
| `ab&ab$` over `abab` | matches | and for the dollar half |
| `a&b|c` over `c` | matches | `&` binds **tighter** than `|` |
| `a&(b|c)` over `c` | no | the control that says which way round |

`P` is a Perl regular expression, and everything measured about it is
something Go's `regexp` already reads — so it is the same compile with a wider
escape set rather than a second engine:

| written | ksh93u+ | control |
| --- | --- | --- |
| `a\d` over `a1` | matches | `a\d` over `ab` — no |
| `a\wc` over `abc`, `a\sc` over `a c` | matches | |
| `^a.*?Xb` over `aXbXc` | matches | `^a.*Xb$` over `aXbXc` — no, so the lazy quantifier is doing it |
| `(?i)abc` over `ABC` | matches | |

### What the four refuse, and why that is what let them land

Not the letter — the **construct**, which is the posture #3894 settled for
`E`. Every flavor ksh93 has carries backreferences, `E` included once the
probe is written unescaped, and RE2 has none:

| written | ksh93u+ |
| --- | --- |
| `\(ab\)\1` over `abab` under `~(G)` | **matches** |
| `\(ab\)\1` over `abcd` under `~(G)` | no |
| `\(ab\)cd` over `abcd` under `~(G)` | matches — the control that says the group parses |

So a pattern using one stops with `<pattern>: the \1 backreference is not
implemented` at status 1, and every pattern that does not use one is
answered. The lookarounds are refused the same way, and the basic flavors add
a third: a **one-sided word edge**, which RE2 has no spelling for at all since
its `\b` is both sides at once and narrowing it would need the lookaround it
also lacks.

| written | ksh93u+ | here |
| --- | --- | --- |
| `\<cd` over `ab cd` under `~(G)` | matches | `the \< word edge is not implemented`, at 1 |
| `\<cd` over `abcd` under `~(G)` | no | the control |

### `E` refuses the same two constructs by name, rather than answering no

`E` is **shipped** rather than refused, so the engine underneath it shows
through on any pattern RE2 cannot express — and until #3894 it showed through
as a confident `no` at status 0. Measured on ksh93u+ 2012-08-01, 2026-09-20,
`env -i PATH=/usr/bin:/bin LC_ALL=C` from a script file:

| written | ksh93u+ | here, before | here, now |
| --- | --- | --- | --- |
| `[[ abab == ~(E)(ab)\1 ]]` | **matches** | no, silently, at 0 | refused at 1 |
| `[[ abcd == ~(E)(ab)\1 ]]` | no | no, silently, at 0 | refused at 1 |
| `[[ abc == ~(E)a(?=b)bc ]]` | **matches** | no, silently, at 0 | refused at 1 |
| `[[ abc == ~(E)a(?!b)bc ]]` | no | no, silently, at 0 | refused at 1 |

The second and fourth rows are the controls in the issue that filed this, and
what they show is worth stating plainly: **they agreed by accident.** The
pattern is the same unwritable one in each pair and only the subject differs,
so the `no` was the compile failing rather than the backreference being read.
Both refuse now, and a pair that agreed for the wrong reason becoming a pair
that refuses is the honest form of the same answer.

The refusal is the one `G` already gives — `<pattern>: the \1 backreference is
not implemented` and `<pattern>: the (?= lookaround is not implemented`, at
status 1, naming the construct where the letter refusal names the letter. The
scan runs before the compile and reads what the *engine* would read, so a
backslash behind a backslash and a `(?=` inside a bracket expression are
ordinary text: `~(E)a[(?=]b` matches `a(b` here and there, and `~(E)a\\1b`
matches `a\1b`. Refusing either would trade a silent wrong answer for a loud
one, which is the only real risk the scan carries.

`\1` is refused wherever the digit run continues, because Go reads `\12` as the
**octal** escape for a newline where ksh93 reads a backreference and a `2` — the
one spelling a narrower scan would let through is the one whose silence is
hardest to see.

### A `~(E)` pattern's backslashes are the expression's, and only one of them survives quote removal here

The shell removes a quoting backslash before the pattern is a pattern, and
ksh93 does not do that inside a `~(E)` group: the backslash reaches the
regular-expression engine. Measured the same day:

| written | ksh93u+ | here |
| --- | --- | --- |
| `[[ abc == ~(E)a\.c ]]` | no | **matches** |
| `[[ a.c == ~(E)a\.c ]]` | matches | matches |
| `[[ 5 == ~(E)\d ]]` | matches | **no** |
| `[[ a+b == ~(E)a\+b ]]` | matches | **no** |
| `[[ y == ~(E)\y ]]` | matches | matches |

So ksh93's engine agrees with RE2 about `\d`, `\w`, `\s`, `\.` and `\+`, and
parts from it on `\y` — a literal letter there and a pattern RE2 refuses
outright. Passing every backslash through would fix the first four rows and
break the fifth, so what is modeled is **only the digits**: a `\1` to `\9`
keeps its backslash, because dropping it turns the pattern into the perfectly
ordinary `(ab)1` and hides from the scan the one construct this shell has to
name. The rest of the table is a divergence of its own and wants its own
measurement. `X` and `P` inherit it unchanged.

**And the set ksh93 itself keeps differs by flavor**, which a written probe
cannot see past and which is why every measurement in the section above is
made through a variable. Measured 2026-09-20, each row with the control that
separates the two readings:

| written | ksh93u+ | what it says |
| --- | --- | --- |
| `[[ aXb == ~(E)a\.b ]]` | no | `E` keeps the backslash |
| `[[ aXb == ~(G)a\.b ]]` | **matches** | `G` does not |
| `[[ abcd == ~(E)\(ab\)cd ]]` | no | `E` keeps it here too |
| `[[ abc == ~(G)a\(b\)c ]]` | matches | and `G` keeps this one |
| `[[ aab == ~(G)a\+b ]]` | no | while dropping this one |
| `[[ ab == ~(G)ax\?b ]]` | matches | and keeping this one |

So the extended flavors keep every backslash there and the basic ones keep
only some. What is modeled is narrower than either and reproduces each row
above: the digits for every flavor, and for a **basic** one the characters
whose backslash decides whether they are an operator — `(`, `)`, `|`, `?`,
`*`, `[`, `^`, `<`, `>`. `.`, `+`, `{` and `}` are measured dropped there and
are dropped here.

`X` has one exception of its own, because there `&` is an **operator**:
`[[ "a&b" == ~(X)a\&b ]]` matches and `[[ ab == ~(X)a\&b ]]` does not, so
dropping the backslash would turn a literal into a conjunction. The backslash
is kept through quote removal and undone again when the operands are cut, so
the engine sees the character.

### A regular expression matches a substring, and that reaches the matcher

This is the one property of the construct that the rest of the matcher cannot
express, because every other question it answers is about the piece it was
handed. A condition asks whether the pattern describes the whole subject; a
substitution asks how much of the subject one match takes. The same `~(E)X`
has to answer them differently, so `interp.patternOpts` carries a `whole` flag
that the whole-subject entry points set and the span-choosing ones do not.

Measured both ways: `[[ xabcx == ~(E)a.c ]]` matches, and
`s=aXbXc; ${s//~(E)X/-}` is `a-b-c` rather than `-c`, which is what searching
each candidate span would have given.

### What is not modeled

- **A group standing anywhere but the front of a pattern.** Measured,
  `[[ abc == a~(E)b.? ]]` matches there, so the prefix is really a flag group
  that may appear mid-pattern. Here only a leading one is read.
- **A trim whose flavor searches.** Measured, `s=abc; ${s#~(E)b}` is `ac`
  there: the matched *span* is removed wherever it sits, so `#` and `%` stop
  being prefix and suffix operators. The trims here go on trying prefixes and
  suffixes, so such a pattern does not match and the value comes back whole.
- **A group on a field whose remainder holds a `/`.** The group is the whole
  field's there while the walk matches one component at a time, and the
  reference shell's own answers do not compose: measured 2026-09-19 in a
  directory holding `Sub/C.txt`, ksh93u+ names the file for `~(N)Sub/C.txt`
  and for `~(N)/tmp/…/Sub/C.txt`, and answers the characters as written for
  `~(E)Su./C..xt` and `~(i)/tmp/…/sub/c.txt` — where the flavor and the fold
  would each have matched every component. Such a field behaves here exactly
  as it did before any of these letters was read. `N` is the exception and is
  read whatever the remainder holds, because it is about the **word** rather
  than about matching a component.
- **`${.sh.match}` after an ERE with capture groups.**
- **A `^` or `$` buried inside one operand of a conjunction.** The two at the
  ends of an operand are read against the subject, which is what ksh93 does
  and is what the `^abc&abc$` row above pins; one inside an alternation
  within an operand is read against the span the operator is choosing.

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

**Greedy, and the rows above cannot show it.** Each of those three forces the
split: there is exactly one way to divide the subject that matches at all, so a
shell reading the star either way answers them identically. This tree read
them *shortest* first until #2513 and passed all three. What separates the two
readings is a pattern where **both** divisions match, which is any repetition
followed by something that can take the rest:

    [[ xx93 == (#b)x#(*) ]]         match=(93)     — the closure took both, not neither
    [[ xx93 == (#b)x##(*) ]]        match=(93)
    [[ abc93 == (#b)*(*) ]]         match=("")     — the star took the lot
    [[ xxy == (#b)(x)#(*) ]]        match=(x y)
    [[ xxxxy == (#b)x(#c2,4)(*) ]]  match=(y)      — a counted closure counts up

**And the item is greedy separately from the count.** How many repetitions a
closure takes and how much *one* repetition takes are two orders with two
answers, and the rows above fix only the first: each has a single-unit item,
where the two readings cannot disagree. It takes an alternation whose arms
differ in length to separate them — with equal-length arms both readings claim
the same text:

    [[ ababX == (#b)(ab|a)#(*) ]]   match=(ab X)   — the longer arm, not `a` then a stop
    [[ ééX == (#b)(éé|é)#(*) ]]     match=(éé X)   — and over characters, not bytes

Read greedy on the count but shortest on the item, the first repetition claims
`a`, no further repetition can reach the `b` from there, the closure stops at
one and `(*)` is handed `babX`. Both divisions match, so only what is reported
tells them apart. This tree read the item shortest-first until #2531.

Read shortest first, every left-hand column above matches nothing and the
right-hand one takes the whole subject, which is a correct *match* and a wrong
*report*. It cost a real configuration: `key = value` lines parsed with

    (#b)[[:blank:]]#([^[:blank:]=]##)[[:blank:]]#[=][[:blank:]]#(*)

gave every value a leading blank, because `[[:blank:]]#` matched none of the
space and `(*)` took it. That is the idiom every ini reader in shell uses.

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

## Collating elements and equivalence classes

`[.x.]` and `[=x=]` inside a bracket expression, and the panel answers them
four ways rather than two. Measured 2026-09-18 under `LC_ALL=C`, script files
under `env -i`:

| pattern | bash 5.3.20 | bash 3.2.57 | ksh93u+ | dash 0.5.12 | zsh 5.9.2 | BusyBox ash 1.37.0 |
| --- | --- | --- | --- | --- | --- | --- |
| `[[.a.]]` | `a` | `a` | `a` | `a` | `a]` | *nothing* |
| `[[.a.]x]` | `a` and `x` | `a` and `x` | `a` and `x` | `a` and `x` | *nothing* | `x` |
| `[a[.nosuch.]b]` | `a` and `b` | `a` and `b` | *nothing* | `a` | *nothing* | `a` and `b` |
| `[[.hyphen.]]` | `-` | `-` | *nothing* | *nothing* | `a]`-shaped | *nothing* |
| `[[=space=]]` | a space | *nothing* | *nothing* | *nothing* | — | *nothing* |

**The first row separates every reading.** Where the element is a member,
`[[.a.]]` is the one-member set holding `a`. Where there is no construct the
bracket ends at the element's own `]`, so the same text is the three-member
set `[`, `.`, `a` with a literal `]` behind it. Where the delimiters are read
and no body is ever an element the set is **empty**, which matches nothing at
all — and the second row is what says the delimiters were read rather than the
bracket having failed, since a member written behind the element still counts.

**A body of more than one character is not an element in the C locale**, and
what that does to the bracket around it is the same question an unrecognized
`[:name:]` asks — inert, the scan given up, or the whole bracket emptied. The
third row is that question, and each column answers it there exactly as it
answers a class name it has not got.

**One column looks a longer body up as a name.** `[[.hyphen.]]` is `-` in
bash 5.3 and in bash as `sh`, and in nothing else in the panel. Two things
about the roster are measured rather than derived, and both matter:

- It is **not the standard's charmap aliases**. Of the 87 names of the
  portable character set that can be written in a shell word, 86 are taken
  under both delimiters and `low-line` is refused — that character being
  reached under `underscore`. `NUL` is the 88th and cannot be asked at all,
  since no shell word holds the character.
- It holds **eight names from outside the set** — the control abbreviations
  `BS`, `HT`, `LF`, `VT`, `FF` and `CR`, and `minus` and `dash` for the
  hyphen — while refusing `BEL`, `NL`, `SP`, `XON`, `XOFF`, `left-bracket`,
  `right-bracket`, `underline` and `vertical-bar`. So the abbreviations are a
  chosen set and not a pattern to extend by guessing, and the lookup is
  case-sensitive: `HYPHEN` and `Hyphen` are refused where `hyphen` is taken.

**bash 3.2 reads the names for `[.` and not for `[=`**, which is the same
delimiter split that column already has on an unrecognized body. No preset
here claims that column.

`Semantics.CollatingElements` carries all four readings as a type of its own.
It was an `Answer` until the BusyBox column was measured, and that column is
the shape a boolean could not hold: a shell that reads the delimiters and
finds an element in no body answers neither "the element is a member" nor
"there is no construct", so with a boolean it had to be filed as one of them —
and it had been, as the wrong one (#3378, #3379).

**One neighboring shape is recorded and not implemented.** In the column
that finds no element, a `-` behind such a sub-expression makes a range whose
low bound is the `[` that opened it: `[[.a.]-c]` is the run from `[` to `c`
there and `[[.a.]-_]` is `[ \ ] ^ _`, while `[[.a.]q]` matches no `[` at all,
so the character is a bound and never a member. Every other column answers the
same pattern with an empty bracket and this one follows them.

**A bound that is not an element is the unknown body it is**, which is #3607
and needed no axis of its own: a body that is not an element asks
`Semantics.UnknownCharacterClass`, and it asks it at a range's end as much as
anywhere else. Measured 2026-09-18 under `LC_ALL=C`:

| pattern | subject | bash 5.3.20 | ksh93u+ | dash 0.5.12 |
| --- | --- | --- | --- | --- |
| `[[.nosuch.]-c]` | `c` | n | n | n |
| `[[.nosuch.]-c]` | `-` | n | n | n |
| `[a-[.nosuch.]]` | `b` | Y | n | n |
| `[a-[.nosuch.]]` | `~` | Y | n | n |
| `[a-[.nosuch.]]` | `-` | n | n | n |

Two different wrongs were live and in opposite directions. At the **low** end
the unread element left the scan before the `-` was looked at, so
`[[.nosuch.]-c]` was the two ordinary members `-` and `c` — matched here and
by none of the three shells. At the **high** end the range was built with an
upper bound that had come back empty, so the bracket held every character
above its low end in all three columns, where two of them hold nothing at all.

Under the inert reading the sub-expression contributes nothing and the range
stands with the nothing it holds: an empty low bound ranks above every
character in the set, so the range is empty, and an empty high bound leaves
the range open above its low end. That is bash's answer, row for row,
including the characters past ASCII — and the readings that end the scan or
empty the bracket reach the bracket's own answer instead, which is ksh93's and
dash's *nothing at all*, `a` included.
