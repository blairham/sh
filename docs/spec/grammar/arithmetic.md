# Arithmetic

What is inside `$(( … ))` and `(( … ))`. `substitutions.md` and
`commands.md` say where those end and keep the contents raw; this says
what the contents mean.

Citation: POSIX.1-2024 XCU §2.6.4, which defers the operator set and
precedence to ISO C. Panel measurements as in `../oracle.md`.

## Variables need no dollar, and unset is zero

    x=5;  $((x+1))   →  6
          $(($x+1))  →  6      both spellings work
    unset u; $((u+1))  →  1    an unset variable is 0, not an error

Unanimous. A bare name inside arithmetic is a variable reference, which is
why the expression cannot be lexed as ordinary words: `x` there is not a
command and not a filename.

## Integers — until they are not

    $((3/2))  →  1     division truncates

Unanimous, and POSIX says the arithmetic is integer-only. Two panel shells
disagree:

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `$((1.5))` | error | error | **1.5** | **1.5** |

ksh93 and zsh evaluate floating point. A core that promised integers and
silently truncated on those shells, or a core that accepted floats and
failed on the others, would both be wrong; this is a dialect axis.

Grammar flag: `ArithFloat` — core: off; `ksh` and `zsh`: on.

## Numeric bases, where the interesting divergence is

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `$((0x10))` | 16 | 16 | 16 | 16 |
| `$((010))` | 8 | 8 | 8 | **10** |
| `$((0100))` | 64 | 64 | 64 | **100** |
| `$((08))` | error | error | 8 | 8 |
| `$((2#101))` | **error** | 5 | 5 | 5 |

**zsh does not read a leading zero as octal.** `$((0100))` is one hundred
there and sixty-four everywhere else, and nothing warns. It is the worst
shape a divergence can have: a plausible number, silently different, in
code that looks portable — file modes are written this way.

`08` shows the same split from the other side: an invalid octal digit is
an error where octal is read, and just a decimal digit where it is not.
ksh93 sits between, reading `010` as octal but accepting `08`.

dash has no `base#number` form at all.

Grammar flag: `ArithExplicitBase` — core: on; `posix` and `dash`: off.

Semantics axis: `ArithLeadingZeroIsOctal` — dash, bash and ksh93 yes,
zsh no. Unanswered in the core, and it is the axis that decides `0100`.

**There is no grammar flag for it**, and there is nothing for one to do:
a literal is kept as written, so the tree bakes in no answer and the
parser never has the question. A `syntax.Dialect` field of the same name
stood beside the axis and was removed (#564). It was documented as
deliberately inert, which sounds harmless and is not: nothing read it, so
nothing could correct it, and it had already drifted — the doc said "true
everywhere but zsh" while the `zsh` preset, which starts from `Core()`
where it was set, carried true. A field that is unread *and* wrong is
indistinguishable from one whose consumer was lost in a refactor, which
is the confusion #478 spent effort untangling. Grammar flags have a core
default; semantics axes do not; a question with one answer per shell and
no parser consequence is an axis.

`ArithInvalidOctalDigitIsError` is the second half, and the reason one
field could not carry both: `08` is an error in dash and bash, decimal
8 in ksh93, and unreachable in zsh, where nothing made the zero octal.

## Operators

Precedence follows C, highest first. Measured spot-checks are unanimous:
`1+2*3` is 7, `(1+2)*3` is 9, `2*3%4` is 2.

    ++  --                    increment, decrement   (not POSIX; absent from dash)
    +   -   ~   !             unary
    **                        exponentiation        (not POSIX; absent from dash)
    *   /   %                 multiplicative
    +   -                     additive
    <<  >>                    shifts
    <   <=  >   >=            relational
    ==  !=                    equality
    &                         bitwise and
    ^                         bitwise xor
    |                         bitwise or
    &&                        logical and
    ||                        logical or
    ?:                        conditional
    =  *=  /=  %=  +=  -=  <<=  >>=  &=  ^=  |=    assignment
    ,                         sequence              (absent from dash)

A comparison yields 1 or 0, and a logical operator yields 1 or 0 rather
than one of its operands: `$((2 && 3))` is 1, not 3.

**Logical operators short-circuit, and the effect is observable** because
assignment is an operator here:

    x=0; $((0 && (x=9)))  →  0, and x is still 0

So evaluation order is part of the specification, not an implementation
detail.

**Assignment inside an expression is a side effect that escapes**:
`$((x=5))` yields 5 and leaves `x` set to 5, exactly like `${x:=5}`.

Grammar flags: `ArithIncDec` and `ArithComma` — core: on for both;
`posix` and `dash`: off for both.

## Exponentiation

`**` is the one operator C does not supply, so nothing about it follows
from the citation above — all of it is measured. bash, ksh93 and zsh have
it; dash rejects it, blaming the second `*` as a missing operand.

| probe | dash | bash | ksh93 | zsh |
| --- | --- | --- | --- | --- |
| `$((2**10))` | error | 1024 | 1024 | 1024 |
| `$((2**3**2))` | error | 512 | 512 | 512 |
| `$((-2**2))` | error | 4 | 4 | 4 |
| `$((2*3**2))` | error | 18 | 18 | 18 |
| `$((2**-1))` | error | **error**: exponent less than 0 | **0.5** | **0.5** |
| `$((9**0.5))` | error | error | 3 | 3. |

So, in every shell that has it: **right-associative** — `2**3**2` is 2⁹,
not 8² — and it binds **between `*` and unary**, so `2*3**2` is 18 and a
prefix sign belongs to the base: `-2**2` is (−2)², which is 4, where C's
`pow`-style reading would give −4. `0**0` is 1, unanimously among the
three.

**A negative exponent has no integer answer and splits the shells that
parse it**: bash stops the expression with `exponent less than 0`, status
1; ksh93 and zsh answer with a float, 0.5, exactly as they do for `1/2.0`.
Where the dialect has floats the operator is a float operation like any
other — `9**0.5` is 3 — and bash, having no floats, rejects `0.5` as a
literal before the operator is reached.

Overflow is under "what this does not cover" below: bash and zsh wrap at
the word size where ksh93 slides into float, which is the shells' general
disagreement about integer width rather than anything `**` adds.

Grammar flag: `ArithExponent` — core: on; `posix` and `dash`: off.

Semantics axis: `ArithNegativeExponentIsError` — bash yes, ksh93 and zsh
no. Unanswered in the core, and unreachable in `posix` and `dash`, where
the grammar has no `**` to ask about.

## The character code operator — zsh only

zsh reads a leading `#` in an expression as a **character code**, and it
is the one thing here that a reader is most likely to get backwards:
`$((#a))` looks like a length and is not one.

| probe | zsh 5.9.2 | bash 5.3.15 | bash 3.2.57 | bash-as-`sh` | ksh93 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| `b=zebra; $((#b))` | `122` | error | error | error | error | error |
| `a=(1 2); $((#a))` | `49` | error | error | error | error | error |
| `a=(1 2); $(( $#a ))` | `2` | `0a` is not a number | — | — | — | — |
| `$((##a))` | `97` | error | error | error | error | error |
| `$((##))` | `character missing after ##` | error | error | error | error | error |
| `$((16#ff))` | `255` | `255` | `255` | `255` | `255` | error |

The refusal is the same sentence each shell writes for any operand it
cannot read — bash `#b: arithmetic syntax error: operand expected (error
token is "#b")` at 1 and 127 as `sh`, ksh93 `#b: arithmetic syntax error`
at 1, dash `expecting primary: "#b"` at 2 — which is why the flag needs no
diagnostic of its own: with it off the `#` is simply not an operand, and
the ordinary route says so in each dialect's words.

`#a` on the array `(1 2)` is **49**, the code of the `1` that begins the
array's first element. The count is `$(( $#a ))`, which the brace-less
length form makes spellable.

Two spellings, and the operand is read differently by each:

- **`#name`** — the code of the first character of that parameter's
  *value*. The scan takes digits as readily as letters, so `$((#1))` is
  the first character of `$1` and `$((#0))` of the shell's own name.
  It does not recurse the way a bare name in an expression does:
  `b=zebra; c=b; $((#c))` is 98, the `b` that is c's value, where
  `$((c))` would read through to `zebra`.
- **`##c`** — the code of the character written out, with the escapes
  `$'…'` decodes: `$((##\n))` is 10, `$((##\x41))` and `$((##\101))` and
  `$((##\U00000041))` are all 65. A single `#` before a backslash takes
  the next character as itself instead, so `$((#\n))` is 110 — the letter
  n.

Exactly one character: `$((##ab))` is the code of `a` with a `b` left
over, which is then refused as text where an operator belonged. And the
character is a character, not a byte — `$((##é))` is 233. A byte that is
no character at all is its own value, though: `$((##\x80))` is 128 and
`$((##\xff))` is 255, which is also what a parameter holding such a byte
gives.

Everything with no answer is **zero and quiet**: a name never set, a name
holding the empty string, a `#` with no operand at all, and a name written
with a subscript. That last is measured rather than derived —
`a=(xy z); $((#a))` is 120 and `$((#a[1]))` is 0, where `${a[1]}` is `xy`
— and the quiet answer is kept because a refusal would be louder than the
shell a script was written for.

`$((##))` is the one failure the operator has of its own, and zsh words it
as neither an operand nor an operator problem: `bad math expression:
character missing after ##`, naming the doubled spelling whichever was
written.

The `base#digits` literal above is untouched, and could not collide: that
`#` follows digits and is read by the number, where this one stands where
an operand belongs.

Recorded and not reproduced: zsh's key-binding escape notation reaches
this operator too — `$((##^A))` is 1 and `$((##\M-a))` is 225 — and
nothing else in this implementation reads `^X` or `\M-`, so a table for
them would exist for one operator alone. `$(( # ))` is also left out of
the corpus, though it is implemented: the spelling makes bash lose the
closing parenthesis of the whole word, so the row would be about its
scanner rather than about the operator.

Grammar flag: `ArithCharacterCode` — core: off; `zsh`: on.

## `$[expr]` — the older spelling

`$[ … ]` is arithmetic in four of the six and nothing at all in the other
two. Measured 2026-09-06:

| probe | dash | bash 5.3 | bash 3.2 | bash-as-sh | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- | --- |
| `echo $[1+1]` | `$[1+1]` | 2 | 2 | 2 | `$[1+1]` | 2 |
| `echo $[2**10]` | `$[2**10]` | 1024 | 1024 | 1024 | `$[2**10]` | 1024 |
| `echo "$[2+3]"` | `$[2+3]` | 5 | 5 | 5 | `$[2+3]` | 5 |
| `echo '$[1+1]'` | `$[1+1]` | `$[1+1]` | `$[1+1]` | `$[1+1]` | `$[1+1]` | `$[1+1]` |

Where it is not read the `$` is literal and the brackets are a pattern, so
the word stands as its own text when nothing on the filesystem matches —
no diagnostic, in dash and ksh93 alike. That is what makes this the
**additive** kind of difference and therefore a grammar flag: nobody means
something else by the same characters.

Grammar flag: `DollarBracketArith` — core: off; `bash` and `zsh`: on.

Off in the core because the core is the intersection and two members of
the panel do not have it. **Both bash builds have it**, which was worth
measuring rather than assuming: bash's manual has called the form
deprecated for years, and the two builds are separate panel members
precisely so a removal would show up as a split. It has not happened yet.

The construct is arithmetic and nothing else. The expression inside is the
same grammar `$(( … ))` holds — `$[2**10]`, `$[1,2]`, `$[i++]`, a
`base#digits` literal — and a bad one earns the same diagnostic, word for
word and error token for error token
(`arith/a-dollar-bracket-refuses-like-the-other`). It is read wherever a
substitution is read: in a word, inside double quotes, in a `${…}`
operand, and in an unquoted here-document body, and not in single quotes.

Two details of the *scan* rather than of the meaning. The closing `]` is
found past nesting, because a subscript is arithmetic too and `$[a[1]+1]`
answers in both shells that have the form — and it is found past quotes
and backslashes, which is what bash's own diagnostic reports when it
blames `']'+1` for the whole text between the brackets. And the spelling
is carried on the span (`Span.Bracketed`) so that printing writes back
what was read: the two forms are one node to everything that evaluates
them, and normalizing `$[1+1]` into `$((1+1))` would be editing a script
rather than printing it — the same reason `Span.Backquoted` exists.

## Errors

Division by zero is an error in every shell, unanimously, and it is a
runtime error rather than a syntax error — the expression parses.

A variable whose value is not a number splits three ways:

| `x=abc; $((x+1))` | result |
| --- | --- |
| dash | error: `Illegal number: abc` |
| bash | **1** — the value is re-evaluated as an expression, `abc` is unset, so 0 |
| zsh | **1** — same |
| ksh93 | error: `abc: parameter not set` |

Three answers again, and two of them are quiet. Recorded here rather than
in `semantics.md`'s table for the reason given there: the table's shape is
binary and this is not.

## What this does not cover

Integer width and overflow behavior, which POSIX leaves to the C
implementation and which the panel would answer differently on different
machines. Anything depending on it is unportable by construction, so no
core answer is recorded rather than an arbitrary one being invented.
