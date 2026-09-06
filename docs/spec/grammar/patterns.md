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

**A class name nothing defines matches nothing, silently** — no error,
no diagnostic, the arm simply never fires — in dash, bash 5.3, ksh93
and zsh alike (`pat/an-unknown-character-class`). bash 3.2 alone falls
back to reading the whole thing as ordinary bracket characters, so
`[[:bogus:]]` there matches the two characters `b]` and nothing here
intended — its column dates the behavior rather than vetoing it, per
`../core.md`. This implementation answers with the four: an unknown
class can never match, and nothing says so, which is one more of the
silent divergences this document keeps a list of.

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
