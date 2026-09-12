# How source is laid out

The grammar says what parses. This page says how it is written back —
the questions a *formatter* has to answer that a parser never asks, and
what each dialect answers. It is the spec `cmd/shfmt` is written from.

`syntax.Style` names the questions; `dialect/<shell>.Style()` answers
them. That is the same shape as the other three vectors, and for the
same reason: a shell's taste is data, not a branch in the formatter.

## What a formatter promises

Three properties, in order. Everything below is subordinate to them.

1. **Nothing is lost.** Every token is emitted verbatim by its source
   extent, so a word is never respelled: `$x` does not become `${x}`,
   backticks do not become `$( )`, and a here-document body is copied
   byte for byte. Every comment survives. The formatter owns only what
   lies *between* tokens.
2. **Idempotent.** Formatting formatted output changes nothing.
3. **Behavior-preserving, and checked.** The output reparses to the
   input's tree, compared with `syntax.SameProgram`.

Property 1 is why the formatter does not print through `syntax.Print`.
That printer is a *canonicalizer* — it promises the tree, not the
spelling — and this is not a theoretical distinction: #1221 had it
escape a word-leading pattern group into a different program, and #1406
had it drop the `function` keyword, which changes whether a keyword
function's locals leak. Both are fixed. Neither can reach a formatter
that never reconstructs a token in the first place.

## The measured evidence

Probes run 2026-09-09 on this machine, over the shell scripts installed
under `/bin`, `/usr/bin`, `/usr/share`, `/usr/libexec`, `/etc`,
`/opt/homebrew` and this user's `~/.zi` tree — 3,153 candidate files, of
which 1,869 carried a shebang or extension naming a dialect. Counts
below exclude the shipped zsh distribution, for the reason given under
Indentation. Files are
classified by `#compdef`/`#autoload`, then shebang, then extension.
Here-document bodies are excluded from the indentation counts, because
they are content rather than layout.

### Indentation

Votes are per *file*: a file's step is the smallest leading-space width
it uses at least twice.

| dialect | files voting | 2 spaces | 4 spaces | other |
|---|---|---|---|---|
| zsh, `~/.zi` (third-party plugins) | 294 | **285 (97%)** | 9 | 0 |
| bash | 220 | **133 (60%)** | 81 (36%) | 6 |
| posix `#!/bin/sh` | 64 | 22 (34%) | 19 (29%) | 23 |
| ksh | 4 | 2 | 0 | 2 |
| dash | 2 | 0 | 2 | 0 |

A fifth row is deliberately absent. The zsh distribution shipped with
the OS holds 899 more voting files and agrees at 92%, and it is left
out of the evidence rather than counted: `internal/wild`'s denylist
excludes a shell's own distribution, `CLEANROOM.md` puts another
project's files on the red list, and a rule that the *sweep* obeys
should not be one a *spec* quietly leans on. The conclusion does not
need it — `~/.zi` is third-party plugin code, written by people with no
connection to zsh's maintainers, and it answers 97% on its own.

**Two spaces**, and the honest reading of that table is narrower than
the answer:

- **zsh is decided** at 97% of 294 independent files.
- **bash leans that way** (60/36) and the lean is not on its own
  enough. The Google Shell Style Guide settles it: *"Indent 2 spaces.
  No tabs."* Corpus and named standard agree, so bash answers 2.
- **posix, ksh and dash have no measurable signal here.** 64 mixed
  votes, 4 votes and 2 votes decide nothing, and POSIX itself has no
  style guide to appeal to. They take the shared default *because the
  question was asked and came back empty*, not because a per-dialect
  answer was measured and happened to match. Recording that distinction
  is the point of this section: a later measurement can overturn three
  of these rows and none of the first two.

**This page previously claimed the tree's `.editorconfig` agreed, and
it does not.** Its universal block says `indent_size = 2`, but the
shell-specific block overrides exactly that:

    [*.{sh,bash}]
    indent_style = space
    indent_size = 4

The claim was written from the first block without reading the second,
which is the ordinary way to misread an `.editorconfig` — later
sections win, and the shell section is the one that governs shell. So
the corpus and the Google guide say two, this tree's own configuration
says four, and there is a real disagreement here rather than the
consensus this paragraph used to assert.

**The resolution, decided 2026-09-09: `.editorconfig` wins where one
exists, and two spaces is the fallback where none does.** It is the
community's standard mechanism for this exact question, it is what the
incumbent reads, and a formatter that overrode a project's stated
configuration with its own measurement would be wrong about which of
the two is evidence. The measurement decides what to do in the absence
of an instruction; it does not outrank one.

Two consequences worth stating plainly, and the first is narrower than
it first appears. `[*.{sh,bash}]` is a *glob*, so it covers `.sh` and
`.bash` and nothing else — on this tree a `.zsh` file, a `.ksh` file, a
`_name` completion function and an extensionless script with a
`#!/usr/bin/env bash` line all fall through to the universal `[*]`
block and its two spaces:

| file | matches the shell block | effective indent |
|---|---|---|
| `deploy.sh`, `lib.bash` | yes | 4 |
| `plugin.zsh`, `f.ksh` | no | 2 |
| `_mycompletion`, `deploy` | no | 2 |

So the reader will split this tree's shell files between two widths by
extension, which is very likely not what anybody intended when that
block was written. Whether the fix is to widen the glob or to drop the
block is a question for whoever owns the file, not for this page.

The second consequence is structural: `Style.Indent` stops being a
per-dialect answer and becomes a per-*file* one, because
`.editorconfig` is matched by path while a dialect is chosen by
shebang, extension or flag. The two can disagree about one file — a
`#!/bin/bash` script named `deploy` is bash to the dialect and `[*]` to
the configuration — and the configuration wins. The dialect supplies
the default that a project is free to overrule.

**Not implemented yet.** `cmd/shfmt` reads no configuration file today
and uses the two-space default everywhere. This paragraph records a
decision, not behavior, and should be rewritten as description when
the reader lands.

Google's one exception — tab-indented `<<-` here-document bodies —
needs no rule here: bodies are verbatim, so a `<<-` body keeps the tabs
that make it work. 39 posix and 6 bash scripts in the corpus rely on
this.

### `then` and `do`

| dialect | `; then` | `then` alone | `; do` | `do` alone |
|---|---|---|---|---|
| posix | 1006 | 78 | 223 | 77 |
| bash | 1336 | 162 | 198 | 73 |
| zsh | 2478 | 30 | 506 | 32 |
| ksh | 5 | 0 | 0 | 1 |
| dash | 0 | 0 | 0 | 5 |

The header's line wins by 13:1 or better wherever there is a sample,
and the Google guide states it verbatim (`; then` and `; do` on the
line of the `if`/`for`/`while`). Every dialect answers *on the header
line*. ksh and dash again have no sample worth the name.

### The zsh brace short forms

zsh alone spells a body with braces where the other dialects have only
the keyword form:

    if [[ -n $x ]] { … } else { … }
    for f ( a b c ) { … }
    while (( i-- )) { … }
    repeat 3 { … }
    case x { x) … ;; }

Measured occurrences across 335 zsh files: **317** `if … {`, **120**
`} else {`, **61** `for … ( ) {`, and none outside zsh. That last clause was checked
rather than assumed — the same probe reported 40 apparent hits in bash
scripts, and every one of them turned out to be embedded `awk`
(`if (total > 0) {`) inside a quoted program in a single script. Shell
short forms in a bash script: zero.

**The condition has to be self-delimiting.** This was measured wrong the
first time and is worth stating in full, because the mistake is instructive:

    if [[ 1 == 1 ]] { echo a }              parses
    if (( i )) { echo a }                   parses
    if a { b }                              PARSE ERROR
    if [[ a ]] { b } elif [[ c ]] { d }     parses
    for f ( a b c ) { … }                   parses
    for f in a b c { … }                    PARSE ERROR

A bare command as the condition swallows the `{` — nothing says where the
condition stopped — so the brace body is available only after a construct
that ends itself: `[[ … ]]`, `(( … ))`, a subshell. The first probe run here
used `[[ ]]` in every case and concluded the form was general; a later one
with a bare command found the limit. Two probes, one hypothesis, and only the
second could discriminate.

The `for` line is the same shape of fact from the other side: the brace body
and the parenthesized item list are one spelling, not two independent
choices, which is why `cmd/shfmt` writes a `for` header back from its source
extent instead of rebuilding it from Names and Items.

**And there are two brace-body spellings, not one.** The corpus gate found
this after the fact, which is the argument for having it:

| | zsh's clause body | the core's group body |
|---|---|---|
| example | `for f ( a b c ) { … }` | `for i in a b; { … }` |
| item list | parenthesized | `in`, as usual |
| separator before `{` | none | **required** |
| separator before `}` | none | **required** — it is a group |

`for i in a b { echo "$i"; }` is a syntax error in its own right, pinned by
the corpus, and dash refuses the form altogether. So a formatter cannot
normalize the punctuation around a brace body in either direction: it writes
the header back from the source extent, keeps the `;` that ran up to the
brace, and keeps the one before the closing brace when the source had it.
All three were bugs first — the header was squeezed, the separator trimmed,
and the group's terminator dropped — and all three were caught by formatting
the corpus rather than by review.

**They parse to the same tree as the keyword spelling.** `IfClause`
carries no marker distinguishing them, so a formatter that prints from
the tree alone cannot help but rewrite one into the other — and the
rewrite is silent, tree-identical, and lands on roughly five hundred
lines of a real zsh plugin tree.

That makes it a style question rather than a grammar one, and it is the
one question in this document where the dialects genuinely disagree:

- **zsh preserves the spelling the author wrote**, recovered from the
  source bytes rather than from the tree (the printer already reads the
  source for every token; asking it which byte opens the body is the
  same move). This follows the rule below that author structure is
  intent, and it keeps `cmd/shfmt` safe to run across a zsh tree.
- **Every other dialect expands to the keyword form**, which costs
  nothing, because no other dialect can parse a brace body at all.

`Style.BraceShortForm` is the field. Flipping zsh to `ExpandShortForm`
is a one-line change to a dialect package if the preference turns out
to be the other way; it is deliberately not a branch in the printer.

**The `case` is two words rather than one paired construct**, and it is
the one member of the family where the opener and the closer have to be
read back separately. Measured 2026-09-12 on zsh 5.9.2: `case x { … esac`
and `case x in … }` both run there, so a printer that recovered one word
and derived the other from it would rewrite half the mixed spellings. The
brace form is the same silent, tree-identical rewrite the `if` is — a
`CaseClause` carries no marker either — so both words come out of the
source, the opener by looking at the first non-blank byte after the
subject and the closer by looking at the byte before the clause's end.

### Case-arm terminators

| terminator | posix | dash | bash | ksh | zsh |
|---|---|---|---|---|---|
| `;;`  | 1054 | 0 | 798 | 7 | 2177 |
| `;&`  | 0 | 0 | 0 | 3 | 0 |
| `;;&` | 0 | 0 | 0 | 3 | 0 |
| `;\|` | 0 | 0 | 0 | 0 | **77** |

`;|` is zsh's and appears nowhere else, which is a grammar fact the
formatter inherits for free. The zeroes are a limit of *this machine's*
sample and not a claim about any shell: bash has had `;&` and `;;&`
since 4.0 and zsh has `;&`, and the parser accepts each under the
dialect that owns it whether or not a script here happens to use it. A terminator is written back as it was written; the
formatter never substitutes one for another.

### Where `#compdef` sits

**209 of 209** zsh completion files put `#compdef` on the first line —
and 1,004 of 1,004 with the shipped distribution counted, which is not
needed to make the point. It is not a comment to be re-flowed: zsh's autoload machinery
reads it, and a blank line inserted above it turns a completion
function into an ordinary one. The formatter never moves or pads the
first line of a file. Unanimous, so it is a rule rather than a default.

## The style rules

Set 2026-09-07 and unchanged: **pull from the recommended styles,
choose our preferred position where they differ, and fix only real
issues.** A real issue is an inconsistency — drifting indentation,
ragged spacing, a comment column that almost lines up. Deliberate
author structure — line breaks, continuations, grouping — is intent,
and keeping it is the job.

Sources consulted, all spec-level and clean-room safe: the Google Shell
Style Guide, POSIX XCU 2.3 for token recognition, the vendor manuals
for what each dialect spells, and this tree's `.editorconfig` — which
disagrees with the first of those about shell, as the Indentation
section now records.

- **Indent two spaces**, per the table above.
- **`; then` / `; do` on the header's line**, per the table above.
- **Case arms one step in.** A one-line arm stays one line and keeps a
  space before its terminator; an arm the author gave several lines
  keeps them, its terminator at the arm's indent.
- **Trailing comments align in runs.** Consecutive lines at one indent
  that each carry a trailing comment get their `#` in one column, one
  space past the run's longest code line. A lone trailing comment keeps
  a single space. The column is computed, never inherited from the
  author's spaces, so it is consistent by construction.
- **Backslash continuations are the author's.** A list split one
  element per line stays split, one continuation indent in. Joining a
  deliberate 20-element list onto one 500-column line is not
  formatting. 34,081 continuation lines in the zsh corpus alone say how
  much text this rule is responsible for not touching.
- **Broken pipelines and `&&`/`||` chains keep the author's break
  positions**, with the operator at end of line.
- **Words are never respelled** — property 1. Google prefers `"${var}"`
  over `"$var"` and `$( )` over backticks; both are rewrites of the
  author's tokens. A `-simplify` mode is where such transformations
  would belong, and it does not exist.
- **Line length is not enforced.** Google's 80-column rule needs
  wrapping decisions this formatter has deliberately not taken.
- **Blank-line runs cap at one, and none stands directly under an
  opener.** gofmt's rule, adopted.
- **Function spelling is preserved**, all three of `f() { }`,
  `function f { }` and `function f() { }`. This is not taste: #1406
  established that `function f { }` and `f() { }` are different
  declarations, because `TypesetLocalNeedsKeywordFunction` reads
  `FuncDecl.Keyword`, so rewriting one into the other silently leaks a
  function's locals. The formatter normalizes the *spacing* of the
  header and nothing else. ksh refuses `function f() { }` at parse
  time, which is the dialect doing its own job.

## Which fields have one answer today

`Indent`, `ThenOnHeaderLine`, `DoOnHeaderLine`, `AlignTrailingComments`
and `MaxBlankLines` are answered identically by all four dialects right
now. They are still fields, and still asked of each dialect, because
the agreement is *measured* — the tables above are what it rests on —
rather than assumed. A field that every dialect answers the same way is
a question that has been put; an inline constant is a question nobody
can reopen without editing the printer.

`BraceShortForm` is the field that currently discriminates, and it
exists because the measurement found the disagreement.
