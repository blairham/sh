# The formatter

`cmd/shfmt` lays a script back out. This page is why it is built the way
it is; `docs/spec/style.md` is what it decides, and
`docs/spec/grammar/comments.md` is the one lexical rule it needed that
the parser does not provide.

## What a formatter promises

Three properties, in order of importance:

1. **Nothing is lost.** Every comment, every word spelled exactly as
   written, every here-document byte for byte. The output parses to the
   input's tree, plus nothing, minus nothing.
2. **Idempotence.** Formatting formatted output changes nothing.
3. **One layout.** Indentation, keyword placement, operator spacing and
   statement separation are the formatter's; everything else is the
   author's.

## Why it does not print through `syntax.Print`

`syntax/print.go` already renders a tree as source, and #1401 opened by
observing that this is most of a formatter. It is not, and the gap is
property 1.

That printer is a **canonicalizer**. It promises the *tree*, not the
spelling — `$x` and `${x}` are one node, and so are `if a; then b; fi`
and the same thing over four lines. That is exactly right for what it
was built for: `type f` shows a function's body and `export -f` carries
one through the environment, and both want the tree said again.

A formatter rewrites a file its author will read next, so it may not
respell anything. And the difference has bitten twice, in the way that
matters most:

- **#1221** — a word-leading pattern group printed back escaped, so
  `echo (v5|v6)` became `echo \(v5\|v6\)`: a program that runs, exits 0,
  and does something else.
- **#1406** — the `function` keyword was dropped, so `function f { … }`
  came back as `f() { … }`. Those are different declarations —
  `TypesetLocalNeedsKeywordFunction` reads `FuncDecl.Keyword` — so the
  rewrite silently leaks a function's locals.

Both are fixed. Neither could ever have reached a formatter that does
not rebuild a token in the first place, and that is the argument for the
design rather than a reason to distrust the printer. **Nobody re-reads a
formatter's diff character by character**, which is what makes this the
one place to prefer the construction that cannot fail over the one that
has been fixed.

## The three load-bearing decisions

**Tokens are emitted verbatim by source extent.** Every node carries
`Pos`/`End` with byte offsets. Words, assignments, redirect targets,
arithmetic and condition text, here-document bodies — all are sliced
from the input, never reconstructed. The formatter owns only what lies
*between* tokens. This is what makes property 1 cheap instead of
heroic: the bytes that could be mangled are never touched.

**Comments are recovered by span subtraction.** The parser discards
them — nothing that *runs* a script needs them. `docs/spec/grammar/
comments.md` establishes, measured across the panel, that a comment is
exactly a `#` outside every word and here-document extent, through end
of line. So subtracting node extents from the source is a complete
recovery, with no lexer support. The printer re-attaches by line: a
comment sharing a line with a statement trails it, a comment on its own
line stands before the next statement at its indent.

**Blank lines are recovered from positions.** Adjacent statements whose
line numbers differ by more than one had blank lines between them; the
formatter keeps one. Same-line statement runs (`a; b`) stay on one line
where the source had one — the author's own paragraphing, which the
parser's line numbers carry in full.

## The dialect decides the layout

`syntax.Style` is the fourth vector beside `Dialect`, `Semantics` and
`Diagnostics`, and `dialect/<shell>.Style()` answers it.

It is thin, because most of what a formatter decides is not a shell's
to decide — an indent width is not a property of zsh. It earns its place
on one question, and that question is a real one: zsh alone spells a
body with braces, and `if cond { … }` parses to *exactly* the tree
`if cond; then … fi` parses to. No AST field separates them. A formatter
printing from the tree therefore rewrites one spelling into the other
with nothing reporting that it did, across some five hundred lines of a
real zsh plugin tree.

Which spelling comes back can only be a stated preference. So it is
data in a dialect package rather than a branch in the printer, and
flipping it is one word in one file — which is where a taste belongs.

## Verification

Tiered, mirroring the substrate's own instruments:

1. **Same program**: format, reparse, compare with `syntax.SameProgram`.
   Promoted out of `syntax_test` for this, as #1401 proposed, so the
   printer's round trip and the formatter's gate cannot drift apart on
   what "the same program" means.
2. **Idempotence**: format twice, compare bytes.
3. **Comment conservation**: the multiset of comment texts in equals the
   multiset out.
4. **Behavioral equality**: original and formatted run identically under
   real bash and zsh. A tier no other formatter tests, and the
   substrate's oracle panel is why it is available.
5. **The corpus**, under two styles — some 2,300 cases written to pin
   behavior by people not thinking about layout, so nothing in it was
   chosen to please a printer.
6. **The wild sweep** (`make fmt-wild`) — every script installed on the
   machine, held to 1–3.

Tier 5 is the one that pays. It found four bugs on its first run that
review had not: an invented `case` terminator, a here-document body
gaining a byte at end of file, an unreflowable newline inside a pattern
list, and the fact that the brace body has two spellings whose
punctuation cannot be normalized in either direction.

It is also worth recording how tier 1 was wrong before it was right.
The sweep originally compared two *canonical prints* — and a
canonicalizer erases exactly the differences a formatter introduces, so
scripts that differed printed alike. It reported "925 scripts, zero
failures". Under `SameProgram` the same population reported 47. The
instrument was the thing at fault, and a green result from it meant
nothing.

## What is deliberately not here

- **No line wrapping.** Google's 80-column rule needs wrapping
  decisions that have not been taken. Continuations the author wrote
  are kept exactly.
- **No `-simplify`.** Rewriting `"$var"` to `"${var}"` or backticks to
  `$( )` is a tree change, which is a different promise from
  formatting. If it is ever wanted it is a separate mode, off by
  default.
- **Few options.** The incumbent's philosophy is right about this:
  options multiply cost. An option must name the dialect fact or the
  corpus evidence that demands it, and `docs/spec/style.md` records
  which fields currently have one answer across all four dialects and
  on what measurement.
