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

### A leading zero the *lexer* never sees

Everything above is about a literal written into the expression. A value
read out of a **variable** is a second reader, and one shell's two answers
differ. Measured 2026-09-11 with `-c`:

| `k=010` | `$((k))` | `$((010))` |
| --- | --- | --- |
| dash | 8 | 8 |
| bash 5.3.15, bash 3.2 | 8 | 8 |
| ksh93u+ | **10** | 8 |
| zsh 5.9.2 | 10 | 10 |

ksh93 is the only column whose two answers differ. Its arithmetic lexer
reads `010` as octal and a value dereferenced into the expression does not
go through that lexer; zsh's two agree because nothing there makes a
leading zero octal at all, and the rest because both readers are octal.

It is the value's **leading numeral** and not the whole of it:

| value | ksh93u+ | the same text as a literal |
| --- | --- | --- |
| `010` | 10 | 8 |
| `0010` | 10 | 8 |
| `09` | 9 | 9 |
| `010+1` | **11** | 9 |
| `1+010` | 9 | 9 |
| `010#5` | 5 | error |
| `-010` | -8 | -8 |
| `" 010"` | 8 | 8 |
| `0x10` | 16 | 16 |

So the numeral has to *begin* the value, with no sign and nothing in front
of it, and the rest of the expression stays octal.

Semantics axis: `ArithStoredValueReadsALeadingZeroAsDecimal` — ksh93 yes,
dash and bash no, asked only where `ArithLeadingZeroIsOctal` is yes so zsh
is never asked. It is the other half of the split
`IntegerAssignmentReadsALeadingZeroAsDecimal` records at the integer
attribute's assignment (#1270); the two are separate axes because a single
answer routed through the evaluator would move `$((010))` with them, and
that is 8 in the shell that splits.

## The base the answer is *written* in

Everything above is about the base a literal is *read* in. One shell in
the panel also has a way to say the base its answer is written in, and it
is the same characters read the other way round: `$(( [#16] 255 ))` is
`16#FF`, which `$(( 16#FF ))` reads back as 255.

Measured 2026-09-12 on zsh 5.9.2, against bash 5.3.15, bash 3.2.57, bash
as `sh`, ksh93u+ and dash — every one of which reads the `[` as an operand
it cannot have:

| probe | zsh 5.9.2 | everything else |
| --- | --- | --- |
| `$(( [#16] 255 ))` | `16#FF` | arithmetic syntax error |
| `$(( [##16] 255 ))` | `FF` | arithmetic syntax error |
| `$(( [#2] 5 ))` | `2#101` | arithmetic syntax error |
| `$(( [#36] 1295 ))` | `36#ZZ` | arithmetic syntax error |
| `$(( [#10] 255 ))` | `255` | arithmetic syntax error |
| `$(( [#16] -255 ))` | `-16#FF` | arithmetic syntax error |
| `$(( [#16] ))` | `16#0` | arithmetic syntax error |

One `#` writes the `base#` in front of the digits and two write the digits
alone; that is the whole of the difference, and it is why both spellings
exist. Base ten writes no mark under either spelling, which is the answer
`typeset -i10` gives as well — a base a shell can spell and a base it
marks are not the same question. A negative keeps its sign outside the
mark, which is the same reading `IntegerBaseNegativeIsTwosComplement`
already records for the integer attribute, and a literal written in one
base is written back in another: `$(( [#16] 0x1f ))` is `16#1F`.

An `_` groups the digits. A bare one is decimal in threes, a number after
it is the group size counted from the right, and `_0` turns grouping off
again:

| probe | zsh 5.9.2 |
| --- | --- |
| `$(( [#_] 1234567 ))` | `1_234_567` |
| `$(( [#_5] 1234567 ))` | `12_34567` |
| `$(( [#16_4] 1048575 ))` | `16#F_FFFF` |
| `$(( [#16_] 1048575 ))` | `16#FF_FFF` |
| `$(( [#16_0] 1048575 ))` | `16#FFFFF` |
| `$(( [#_3] 1234567.5 ))` | `1_234_567.5` |

The last row is the one that says a base and a grouping are separable
questions rather than one: a specifier naming a base truncates a float
(`$(( [#16] 3.5 ))` is `16#3`, and so is `$(( [#10] 3.5 ))` at `3`) where
one naming only a grouping leaves it alone.

### It is lexical, not a prefix operator

The obvious reading — a unary operator over the expression beside it — is
wrong, and three measurements say so independently:

| probe | zsh 5.9.2 | what it rules out |
| --- | --- | --- |
| `$(( 2[#8] ))` | `8#2` | it has to precede its operand |
| `$(( 0 ? [#16] 1 : 2 ))` | `16#2` | it is evaluated |
| `$(( [#16] 255 + [#8] 1 ))` | `8#400` | the outermost one wins |

So it is a token that produces no value, standing wherever a token may,
and the **textually last** one decides. That is how it is implemented: the
specifier is consumed where blanks are, and lifted to the top of the tree
as a single node over the whole expression, rather than being a node where
it was written. A node where it was written could not answer the second
row at all, because nothing evaluates the branch that holds it.

A subscript is untouched and could not collide: its bracket touches the
name in front of it and is read by the name, where this one stands where a
token begins. `$(( a[#8] ))` is the element of `a` under the subscript
`#8` and `$(( a [#8] ))` is `8#0` — one space apart, measured.

It reaches two texts and not three. The answer an expansion produces is
the obvious one; an assignment *inside* the expression stores the
formatted text as well, so `x=5; (( x = [#16] 255 ))` leaves x holding the
six characters `16#FF` and `$(( x ))` reads them back as 255. The
subscript of that same assignment is not formatted —
`typeset -A m; (( [#16] m[255] = 1 ))` stores `16#1` under the key `255` —
which is what tells the value apart from every other text the write
touches.

### And it teaches an integer name nothing

The integer attribute writes a base with the same six characters, so the
two constructs meet. Measured:

| probe | zsh 5.9.2 |
| --- | --- |
| `typeset -i i; (( i = [#16] 255 )); echo $i` | `255` |
| `typeset -i i; (( i = [#16] 0x1f )); typeset -p i` | `typeset -i16 i=31` |
| `typeset -i i; i=$(( [#16] 255 )); typeset -p i` | `typeset -i16 i=255` |
| `typeset -i16 i; (( i = [#8] 255 )); echo $i` | `16#FF` |

So `IntegerBaseComesFromTheValueAssigned` reads the **literal the script
wrote** and not what a format rendered: the second row learns 16 from the
`0x1f` beside it, and the first learns nothing at all. The third is the
boundary — the same characters arriving as the text of an ordinary
assignment do teach a base, because there they *are* the text the name was
given. The fourth says a base the name already holds stands, which it does
for every other route too.

Getting this wrong is silent: reading the base back out of the rendered
text answers `16#FF` for the first row, which is a base the script never
wrote down.

### The three refusals

| probe | zsh 5.9.2 |
| --- | --- |
| `$(( [#37] 5 ))` | `invalid base (must be 2 to 36 inclusive): 37` |
| `$(( [#0] 5 ))` | `invalid base (must be 2 to 36 inclusive): 0` |
| `$(( [# 16] 5 ))` | `bad output format specification` |
| `$(( [#] 5 ))` | `bad output format specification` |
| `$(( [foo] 5 ))` | `bad output format specification` |
| `$(( [16] 255 ))` | `bad base syntax` |

The last two are **one character apart and worded apart**, which is why
the grammar tells them apart rather than calling both a bad specifier: a
bracketed group holding nothing but digits is its own sentence. Neither
opens with `bad math expression:` the way every other arithmetic failure
in that shell does, and neither names the text it refused.

Zero is a base and not the absence of one — `[#0]` is refused where `[#_]`
is fine — so the node carries a flag beside the number rather than reading
0 as "none".

The range is checked **before the expression is evaluated**, which is a
choice with one row against it. `$(( [#37] 1/0 ))` names the base in that
shell and `$(( 1/0 + [#37] 1 ))` names the division, because it reads and
evaluates in one pass and had already divided when it reached the
specifier. Two failures in one expression is the only text that can tell
the orderings apart, so the reading taken here is the one that is right
for the three shapes a script can have, and the fourth is recorded rather
than reproduced.

Grammar flag: `ArithOutputFormat` — core: off; `zsh`: on.

Not a semantics axis, and the range is not a constant in the parser
either: which bases can be spelled is `IntegerBaseDigits`, shared with the
integer attribute because the two write a base the same way and a script
reads one back through the other. `typeset -i16 h=255` and
`$(( [#16] 255 ))` are `16#FF` in the same shell, and they would not have
to be if the two renderers were two.

**Recorded and not reproduced**: `setopt c_bases` rewrites the mark for
bases 8 and 16 — `$(( [#16] 255 ))` becomes `0xFF`, and with
`octal_zeroes` as well `$(( [#8] 8 ))` becomes `010`. That is an option
this implementation does not have, and it moves `typeset -i16` in the same
shell by the same amount, so it belongs to whatever adds the option rather
than here.

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

## Quote characters inside an expression

Measured 2026-09-07 and re-measured 2026-09-10, from a script file with
`n=5`. This is where the panel divides most sharply, and the two quote
characters divide it differently — no shell's answer to one predicts its
answer to the other.

### The double quote

| probe | bash 5.3.15 | bash-as-`sh` | bash 3.2.57 | ksh93u+ | zsh 5.9.2 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| `$(( "1" + 1 ))` | `2` | `2` | error | `2` | `2` | error |
| `$(( "n" + 1 ))` | `6` | `6` | error | `6` | `6` | error |
| `$(( 1 + "2" ))` | `3` | `3` | error | `3` | `3` | error |
| `$(( "" + 7 ))` | `7` | `7` | error | `7` | `7` | error |
| `$(( 1"0" ))` | **`10`** | **`10`** | error | **error** | **error** | error |
| `$(( n"a"me ))` | `0`, the name `name` | same | error | error | error | error |

Four shells read through the quote and two refuse it — and the last two
rows split those four again. bash **removes** the byte from the text
before anything reads it, so two digits with a quote between them are one
number; ksh93 and zsh **skip** it only where a token may begin, so the
quote ends the number and two operands run together. The removal shows up
in the diagnostic too: `$(( "1" "2" ))` is reported against `1 2` in bash
and against the text as written in the other two.

Three readings, all additive — the core refuses, and a dialect that has
one accepts more — so it is `syntax.Dialect.ArithDoubleQuote` and not a
semantics axis, the same call [the character code operator](#the-character-code-operator--zsh-only)
makes.

It is worth more than the spelling suggests. `$(( "$n" + 1 ))` is a shape
that looks defensive and is common, and refusing it fails under bash, ksh
and zsh alike.

### The single quote

| probe | bash 5.3.15 | bash 3.2.57 | bash-as-`sh` | ksh93u+ | zsh 5.9.2 | dash |
| --- | --- | --- | --- | --- | --- | --- |
| `$(( '1' + 1 ))` | error | error | error | **`50`** | `illegal character: '` | error |
| `$(( 'a' ))` | error | error | error | **`97`** | same | error |
| `$(( 'ab' ))` | error | error | error | **error** | same | error |
| `$(( 'a ))` | error | error | error | **`97`** | same | error |
| `$(( '' ))` | error | error | error | **`39`** | same | error |
| `$(( '\101' ))` | error | error | error | **`65`** | same | error |

ksh93 alone reads it as a **character constant**, the way C does: the code
of the character between the quotes, with the escapes `$'…'` decodes. The
closing quote is optional — `'a` is 97 and `''` is 39, the second quote
read as the character with nothing left to close it — which is also why
`'ab'` is a syntax error rather than a multi-character constant: the
reading stops after the `a` and the `b` is left standing where an operator
belongs.

One shell has it and five refuse it, so it is a flag too:
`syntax.Dialect.ArithCharacterConstant`. It builds the same node `##c`
builds, because it asks the same question and two nodes would be two
places for the escape table to drift apart.

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

### What a complaint quotes back

Two questions, and they are answered per dialect rather than per failure: a
parse failure and an evaluation failure carry the same prefix as each other on
the same route. Measured 2026-09-11, `-c`:

| route | bash 5.3.15 | ksh93u+ | zsh 5.9.2 |
| --- | --- | --- | --- |
| `(( 1/0 ))` | `((: 1/0 : division by 0 (error token is "0 ")` | ` 1/0 : divide by zero` | `division by zero` |
| `for (( i=0; i<1/0; i++ ))` | `((: i<1/0: division by 0` | ` i<1/0: divide by zero` | `division by zero` |
| `[[ 1/0 -eq 1 ]]` | `[[: 1/0: division by 0` | `1/0: divide by zero` | `division by zero` |
| `let '1/0'` | `let: 1/0: division by 0` | `let: 1/0: divide by zero` | `division by zero` |
| `x=$(( 1/0 ))` | `1/0 : division by 0` | ` 1/0 : divide by zero` | `division by zero` |

**The construct names itself, in bash alone.** `((: ` and `[[: ` go in front
the way a builtin's name does, on the routes that are *commands*; the expansion
carries no name. ksh93 and zsh name no construct anywhere, and `let` is already
on this footing through the builtin — bash and ksh93 write `let: ` and zsh
writes nothing.

**The expression is quoted as the construct held it.** Blanks included:
`((    1/0   ))` is `   1/0   : divide by zero` in ksh93, spaces and all.
bash skips the *leading* blanks and keeps the rest — `((: 1/0   : ` — so the
question is one-sided rather than a trim.

Diagnostics: `ArithErrorNamesTheConstruct` and `ArithErrorSkipsLeadingSpace`,
both bash alone. zsh names no expression at all, so neither reaches it.

**Not yet matched:** bash's *error token* runs from the failing operand to the
end of the expression — `1/0 ` blames `0 `, `1/0 + 2 ` blames `0 + 2 ` — where
a literal the reader refused blames itself alone (`8#9`). Ours names the
operand without the tail, on every route equally.

### A stray `:` is one shell's one reversal

Every math complaint ksh93 makes is `<expression>: <reason>` — except for a
`:` with no `?` in front of it, where the offending byte comes first and the
expression comes last. Measured 2026-09-12:

| written | ksh93u+ |
| --- | --- |
| `$(( 1 : ))` | ``:: invalid character in expression -  1 : `` |
| `$(( 1 : 2 ))` | ``:: invalid character in expression -  1 : 2 `` |
| `$(( 1 ? 2 : 3 : 4 ))` | ``:: invalid character in expression -  1 ? 2 : 3 : 4 `` |
| `$(( 1 2 ))` | `` 1 2 : arithmetic syntax error`` |
| `$(( 1 @ ))` | `` 1 @ : arithmetic syntax error`` |
| `$(( : 1 ))` | `` : 1 : arithmetic syntax error`` |

The last three are the controls that make it the byte's shape and not
leftover text's. **`:` is the only byte that takes this order**, established
by measuring `$(( 1 <byte> ))` over every printable one: `@`, `$`, `;`, `\`,
`'`, `{`, `]`, `.`, `_` and `~` are all the ordinary shape, `?` has a
sentence of its own in the ordinary order, and `,`, `"`, `#` and `||` are not
failures there at all. A *leading* colon is ordinary too, because that one
wants a value and never reaches the leftover reading.

It applies wherever the colon stands, a complete conditional in front of it
included, and inside a subscript — which is an expression like any other.

The parser gives a stray colon a kind of its own, `ErrArithColonWithoutQuestion`,
by both roads to it: the dialect that reads `:` as a math token
(`ArithColonIsAToken`, zsh) arrives after it has found the second value, and
the rest arrive through leftover text. A dialect with no sentence for the byte
falls back to the one it gives any leftover text, which is what bash and dash
say. Diagnostics: `ArithColonWithoutQuestion` for the reason, zsh; and
`ArithColonWithoutQuestionLine` for the whole reversed line, ksh93 alone.

A variable whose value is not a number splits three ways:

| `x=abc; $((x+1))` | result |
| --- | --- |
| dash | error: `Illegal number: abc` |
| bash | **1** — the value is re-evaluated as an expression, `abc` is unset, so 0 |
| zsh | **1** — same |
| ksh93 | error: `abc: parameter not set`, status 1, script abandoned |

Three answers again, and two of them are quiet. Recorded here rather than
in `semantics.md`'s table for the reason given there: the table's shape is
binary and this is not.

### The chase, and its last step, are two questions

The table above reads as though ksh93 refuses to re-evaluate a name-shaped
value. **It does not.** Measured 2026-09-11 against 93u+ 2012-08-01:

| | bash 5.3 | bash 3.2 | zsh 5.9 | ksh93 | dash |
| --- | --- | --- | --- | --- | --- |
| `y=5; x=y; $((x+1))` | 6 | 6 | 6 | **6** | `Illegal number: y` |
| `z=7; y=z; x=y; $((x+1))` | 8 | 8 | 8 | **8** | `Illegal number: y` |
| `$((nosuch+1))` | 1 | 1 | 1 | 1 | 1 |
| `y=; x=y; $((x+1))` | 1 | 1 | 1 | 1 | 1 |
| `x=abc; $((x+1))` | 1 | 1 | 1 | **error** | `Illegal number: abc` |
| `x=1abc; $((x+1))` | error | error | error | `1abc: arithmetic syntax error` | `Illegal number: 1abc` |

So there are two axes and not one:

- **`ArithNameValueRecurses`** — whether the value is looked up at all.
  Yes in bash, zsh *and ksh93*; dash alone says no, and it says no as far
  as the values lead.
- **`ArithRecursedNameMustBeSet`** — what an unset name *at the end of the
  chase* means. A zero in bash and zsh, a refusal in ksh93. Asked below the
  top only: a name written into the expression itself is a zero in every
  column, which the third row is there to say.

The fourth row is the discriminator between the two readings of ksh93's
refusal. A set-but-empty value is 1 there as everywhere, so what that shell
refuses is an **unset name**, not a value it could not read as a number —
and the refusal is its `set -u` sentence word for word, with nounset off,
fatal in the same way: `||` does not catch it and a subshell dies alone.

The sixth row is the other side of the same distinction. With nothing to
look up, ksh93 reports an `arithmetic syntax error` — the same reason it
gives for an expression it cannot parse — rather than a parameter that is
not set.

### The value is re-read as an *expression*, not as a name

The chase is not a lookup with a name-shaped value as its input. The value
is parsed again as an expression, and a name is simply the expression that
is one operand. Measured 2026-09-11, `-c`:

| | bash 5.3 | ksh93 | zsh 5.9 | dash |
| --- | --- | --- | --- | --- |
| `v=1+1; $(( v * 3 ))` | 6 | 6 | 6 | `Illegal number: 1+1` |
| `a=(1+1); $(( a[0] * 3 ))` | 6 | 6 | 0 — `a[1]` is 6 | no arrays |
| `i=1; v=i++; $(( v ))` | 1, and `i` is 2 | same | same | `Illegal number: i++` |
| `v=q=5; $(( v ))` | 5, and `q` is 5 | same | same | `Illegal number: q=5` |
| `v="3 4"; $(( v ))` | `3 4: arithmetic syntax error in expression` | `3 4: arithmetic syntax error` | ``operator expected at `4' `` | `Illegal number: 3 4` |
| `v=1/0; $(( v ))` | `1/0: division by 0` | `1/0: divide by zero` | `division by zero` | `Illegal number: 1/0` |
| `q=5; v='$q'; $(( v ))` | `$q: ... operand expected` | `$q: arithmetic syntax error` | error | `Illegal number: $q` |

Four facts, in order:

- The operators in the value run, side effects and all, so this is an
  evaluation and not a conversion.
- The fifth and sixth rows say what fails: the re-read expression, worded
  the way a written one is, and blaming the **value** rather than the name
  it came out of.
- The last row says the value is *not expanded* on the way in. It went
  through expansion when it was assigned and does not go through it again,
  so a surviving `$` is an ordinary character in an expression and refused
  as an operand.
- It is the same split as the chase, so it is the same axis —
  `ArithNameValueRecurses`, yes in bash, ksh93 and zsh, no in dash, which
  refuses the value as a number in one sentence.

The bound applies here rather than to the chase alone: reading a value as
an expression puts the whole evaluator inside the value, so `x=x` has to
stop on a count. All three shells that re-read stop it and say so — bash
`expression recursion level exceeded`, ksh93 `recursion too deep`, zsh
`math recursion limit exceeded`.

**Why this is written down rather than merged into the row above.** The
interpreter modeled the chase alone and stood ksh93's `parameter not set`
in as the wording for *any* unreadable value (#1629), which answered the
last row wrongly and the fifth row wrongly in the other direction — a
plausible `1` where the real shell stops the script. A probe that only ever
tries `x=abc` cannot tell the two apart.

## A subscript inside an expression

`a[i]` written inside `$(( ))` is the element `${a[i]}` is, and the
brackets take the same three readings there they take in an expansion:
an expression on an indexed name, a key on a table, and — where the
grammar has them — a flag group that selects.

### The whole-array spelling

`$(( a[*] ))` and `$(( a[@] ))` split the panel three ways on an
*indexed* name, and the three are two axes:

| shell | `a=(3 4 5); $(( a[*] ))` |
| --- | --- |
| bash 5.3.15, as `sh`, 3.2.57 | `a[*]: bad array subscript`, then `0`, status 0 |
| ksh93u+ | `*: arithmetic syntax error`, status 1 |
| zsh 5.9.2 | the slice `3 4 5`, read as an expression and failing as one |

`ArithWholeArraySubscriptIsTheSlice` is zsh's reading and
`ArithWholeArraySubscriptIsReportedAsBad` is the split between the other
two: bash reports and answers zero so the expression survives, ksh93
lets the arithmetic refuse the whole thing.

On an **association** every column is silent and answers zero, because
the brackets are the key `*` and nothing is stored under it — so the
table is consulted before either question is asked.

The spelling has to be exact. `$(( a[ * ] ))` is an arithmetic syntax
error in bash and in zsh alike, so a `*` with a blank beside it is an
expression that will not read rather than the whole-array spelling
(#1978).

### A flag group

Where the grammar has subscript flag groups, an expression reads one too
— it is the same construct in a second position, not a second construct.
Measured on zsh 5.9.2, 2026-09-12:

| written | is |
| --- | --- |
| `a=(10 20 30); $(( a[(r)20] ))` | `20`, the value the search found |
| `$(( a[(i)20] ))` | `2`, the index it found it at |
| `$(( a[(i)99] ))` | `4`, the miss one past the end |
| `$(( a[(r)99] ))` | `0` — the miss reads as nothing, which is zero |
| `$(( a[(e)2] ))` | `20`; a group that selects nothing leaves an ordinary subscript |
| `$(( a[(r)20] + 1 ))` | `21`, an operand like any other |
| `typeset -A m; m[k]=9; $(( m[(k)k] ))` | `9` |
| `s=hello; $(( s[(r)l] ))` | `0` — a character is no number |
| `a=(10 20 30); (( a[(r)20] = 9 ))` | `10 9 30`; the write names the same element |

The answer is the expansion's, joined and then read as a number by the
same rule an element's value is read by — so a group matching several
keys is usually a failure rather than a number, exactly as a slice of
several elements is.

The group's operand is a **literal**: an arithmetic expression is
expanded whole before it is parsed, so there is nothing left in it to
expand and a `$` still in it is a `$`. A letter the group is read with
and this implementation does not carry is refused by name here as it is
in an expansion, rather than becoming part of a key or of an expression
(#1986).

## What this does not cover

Integer width and overflow behavior, which POSIX leaves to the C
implementation and which the panel would answer differently on different
machines. Anything depending on it is unportable by construction, so no
core answer is recorded rather than an arbitrary one being invented.
