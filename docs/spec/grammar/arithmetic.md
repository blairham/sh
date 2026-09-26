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

### A plain `=` is worth what the target's numeric attribute made of it

An arithmetic assignment's *value* is not the right-hand side. It is the
number the target's numeric attribute converts that side into, so a float
assigned to a name carrying `-i` is the truncated integer — and the
expansion renders **that**, not the float:

| probe (zsh) | value | the name afterwards |
| --- | --- | --- |
| `integer i; float f=3.1415`<br>`$(( i = f * 10000 ))` | `31415` | `31415` |
| the same expression unassigned, `$(( f * 10000 ))` | `31415.` | — |
| `integer i; $(( i = 2.0 ))` | `2` | `2` |
| `integer i; $(( i = -1.5 ))` | `-1` | `-1` |

Truncation is toward zero, which only the negatives can say: `-1.5` is `-1`
and not `-2`. A float the word cannot hold saturates and one that is no
number at all is zero — the conversion an integer context already makes.

**It is the converted number and not the stored text**, and the two rows that
say so hold the numeric type fixed while moving only the rendering:

| probe | value | the name afterwards |
| --- | --- | --- |
| `typeset -i16 a; $(( a = 108 ))` | `108` | `16#6C` |
| `typeset -F3 g; $(( g = 3.14159265358979 ))` | `3.14159265358979` | `3.142` |

So a rule stated as "the value is what was stored" produces those renderings
and is wrong. Moving the *type* is what moves the answer: `integer i` makes
`$(( i = 1.5 ))` `1`, a `float` name makes it `1.5`, and a name with no
numeric attribute leaves it `1.5`.

The truth of `(( ))` is that same value, which is the row no rendering rule
can produce because nothing is printed: `integer i; (( i = 0.5 ))` is
**false** in zsh, and false in ksh93 beside it.

**Only the plain `=`, and that is where the two shells part.** zsh converts
what `=` stores and hands a compound assignment its computed value; ksh93
converts both.

| probe, with `typeset -i n=1` | ksh93 | zsh | `n` afterwards |
| --- | --- | --- | --- |
| `$(( n += 0.5 ))` | `1` | **`1.5`** | `1` in both |
| `$(( n = n + 0.5 ))` | `1` | `1` | `1` in both |

That pair holds the attribute fixed and moves the operator, which is what
says the rule is keyed on the operator and not on "an assignment". The plain
form is shared and implemented; the compound divergence is recorded here and
not modeled (#4606).

bash, dash and BusyBox ash reach none of this: they have no floating point in
arithmetic at all, so there is never a value for an integer attribute to
convert.

Implemented at `numericAttribute` in `interp/arith.go`, measured 2026-09-26
on zsh 5.9.2 and ksh93u+ 2012-08-01.

### And what it **declares** takes its type from the value

The rule above is about the *value* an assignment has. Its neighbor is about
what the assignment leaves behind: an arithmetic assignment to a name that
does not exist declares one in zsh — the axis
`Semantics.ArithmeticAssignmentDeclaresANumber`, which zsh alone answers yes
— and **the value's type says which numeric attribute it gets.** An integer
value declares `integer`; a float value declares a float, at the `F` letter's
default precision.

| probe (zsh), on a name that does not exist | `${(t)xx}` | `typeset -p xx` |
| --- | --- | --- |
| `(( xx = 5 ))` | `integer` | `typeset -i xx=5` |
| `(( xx = 1.5 ))` | `float` | `typeset -F xx=1.5000000000` |
| `(( xx = 1e30 ))` | `float` | `typeset -F xx=1000000000000000019884624838656.0000000000` |

**Two readings agree with that one almost everywhere and both are wrong**, so
the rows that matter are the ones built to part them. It is not "the number
did not come out whole" — `1.0` is whole and declares a float where `3/2` is
whole and declares an integer:

| probe | value | `${(t)xx}` |
| --- | --- | --- |
| `(( xx = 1.0 ))` | `1.` | **`float`** |
| `(( xx = 3/2 ))` | `1` | `integer` |
| `(( xx = 2.0/2 ))` | `1.` | **`float`** |
| `(( xx = 3.0/2 ))` | `1.5` | `float` |

And it is not "a point was written", which takes a pair with no point written
in either — two names differing only in their type:

| probe | `${(t)xx}` |
| --- | --- |
| `float ff=2; (( xx = ff ))` | **`float`** |
| `integer ii=3; (( xx = ii ))` | `integer` |

The same pair from the other side, with `zmodload zsh/mathfunc`, writes a
point in both: `(( xx = int(2.0) ))` is `typeset -i xx=2` and
`(( xx = float(3) ))` is `typeset -F xx=3.0000000000`.

The letter is `F` and not `E`, which is a measurement rather than a default —
`float ee` on its own lists as `typeset -E ee`. **No base comes with a
float**: the integer route learns one from a radix the expression wrote, and
this one does not. `(( xx = [#16] 255.9 ))` is `typeset -F xx=255.9000000000`
where `(( xx = [#16] 255 ))` is `typeset -i16 xx=255`, the `[#16]` reaching
only the expansion's own rendering in the first.

**The operator is not a second noun here**, which is worth saying because it
is one for the rule above: `$(( xx += 1.5 ))` on a name that does not exist
declares a float, exactly as the plain `=` does. Every construct that assigns
inside arithmetic gives the same answer — `(( ))`, `let` and a C-style `for`
header alike.

ksh93 reaches all of this and declares nothing: `(( xx = 1.5 ))` there leaves
a plain scalar `xx=1.5`, because the axis is `No`. bash, dash and BusyBox ash
refuse the float literal outright and never arrive.

Implemented at `declareFloatFromArithmetic` in `interp/arith.go`, measured
2026-09-26 on zsh 5.9.2 and ksh93u+ 2012-08-01 (#4605).

### And `setopt posix_identifiers` turns the declaration off

The axis above is *whether* an arithmetic assignment declares, and zsh has a
name for answering it no. `POSIX_IDENTIFIERS` — off by default, and turned on
without anyone typing it by `emulate sh` and `emulate ksh` — leaves an
ordinary scalar where the declaration would have been. The value stored is
then the expression's own rendering, which is what every other shell's path
already writes.

| probe (zsh), on a name that does not exist | default | under `posix_identifiers` |
| --- | --- | --- |
| `(( xx = 5 ))` | `typeset -i xx=5` | `typeset xx=5` |
| `(( xx = 1.5 ))` | `typeset -F xx=1.5000000000` | `typeset xx=1.5` |
| `(( xx = 1.0/3 ))` | `typeset -F xx=0.3333333333` | `typeset xx=0.33333333333333331` |
| `(( xx = 1e30 ))` | `typeset -F xx=1000000000000000019884624838656.0000000000` | `typeset xx=1e+30` |
| `(( xx = [#16] 255 ))` | `typeset -i16 xx=255` | `typeset xx='16#FF'` |

**The name is still created**; only the attribute is withheld. And the option
reaches only a declaration the assignment would have made, so the rows that
did not move under #4605 do not move here either: `typeset -i ii; (( ii = 5
))` is `typeset -i ii=5` in both states, `float ff; (( ff = 1.5 ))` is
`typeset -E ff=1.500000000e+00` in both, and `(( b[2] = 1.5 ))` writes an
element in both.

**The noun is the declaration and not the identifier**, which has to be said
because the option's name points the other way. `xx` is spelled with nothing
but the characters `POSIX_IDENTIFIERS` permits, so a rule about which
characters may appear in a name cannot reach it — and every row above uses
`xx`. zsh's manual calls the arithmetic effect *another* difference, and the
measurement agrees: `typeset a.b=1` is refused identically in both states. The
option's two other documented effects — which characters an identifier may
hold, and whether `$#name` without braces is the length of `$name` — are
separate behaviors, and the first of them is read when a script is *parsed*
where this one is read when the assignment **runs**. Measured: a function body
written with the option off leaves a scalar when it is called with it on, and
the other direction leaves an integer.

`Semantics.ArithmeticAssignmentDeclaresANumber` read backwards — the option on
is that axis answering `No` — at `posixidentifiers` in `dialect/zsh/setopt.go`.
It was recorded and inert until #4664: the `setopt` succeeded, every surface
that names it reported it back, and the declaration happened in both states.
Measured 2026-09-26 on zsh 5.9.2.

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

### An underscore inside a numeral is a digit separator — zsh only

Measured 2026-09-13, `-c`, against zsh 5.9.2 and bash 5.3.15. `_` is
digit 63 of the base-64 alphabet, so bash reads it into the numeral and
then refuses a digit base ten has no room for; zsh answers a number.

| probe | zsh 5.9.2 | bash 5.3.15 | ksh93u+ |
| --- | --- | --- | --- |
| `$(( 1_ ))` | 1 | `1_: value too great for base` | syntax error |
| `$(( 1_0 ))` | **10** | `1_0: value too great for base` | syntax error |
| `$(( 1_0_0 ))` | 100 | error | error |
| `$(( 1__0 ))` | 10 | error | error |
| `$(( 1_abc ))` | ``operator expected at `abc'`` | error | error |

**`$(( 1_ ))` cannot decide this and `$(( 1_0 ))` can.** One is what a
separator gives and also what a byte the reader threw away would give, so
the row the divergence was noticed on is the one row that answers
nothing. Ten is a separator and nothing else: a digit would be `value too
great for base`, which is what the other columns say, and a numeral that
*ended* at the byte would leave `_0` standing where an operator belongs —
an unset name in arithmetic being zero, `1` followed by a name is an
`operator expected` rather than a 1.

**The rule is: the separator is removed, and then the ordinary rules
apply to what is left.** Every other row follows from that one sentence,
which is why it is worth stating that way round rather than as a list:

| probe | zsh 5.9.2 | what it says |
| --- | --- | --- |
| `$(( 0x1_f ))` | 31 | a radix prefix is unaffected |
| `$(( 2#1_0 ))` | 2 | and so is a `base#` — binary has no digit 63 under any reading |
| `$(( 16#f_f ))` | 255 | |
| `$(( 1_0#5 ))` | 5 | the **base** is read from the cleaned text |
| `$(( 1_#5 ))` | `invalid base …: 1` | which this one says from the other side |
| `setopt octalzeroes; $(( 0_10 ))` | **8** | so the zero the separator uncovers is a prefix |
| `$(( 0x_1 ))` | 1 | one may stand where the first digit would |
| `$(( 2#_10 ))` | 2 | |
| `$(( 1_ ))` | 1 | and a trailing one belongs to the numeral |
| `$(( 1_0.5 ))` | 10.5 | floats, in the fraction … |
| `$(( 1e1_0 ))` | `10000000000.` | … and in the exponent |
| `$(( 1e_2 ))` | `100.` | including in front of the exponent's digits |
| `$(( 1e_ ))` | `operator expected` | which it does not stand in for |
| `$(( _ ))` `$(( _1 ))` | 0 | a **leading** one is a name, as it always was |
| `x=1_0; $(( x ))` | 10 | and a stored value is read by the same rule |

The last two rows are the boundaries. A numeral begins with a digit, so
an underscore in front of one is an identifier and an unset name is zero;
and the rule belongs to reading a numeral rather than to reading a
script, so it reaches a value that was never in the program text.

Grammar flag: `ArithDigitSeparator` — core: off; `zsh`: on. A flag rather
than a semantics axis, asked in the two places that need it — the parser,
where it decides how far the numeral reaches, and the conversion, where a
stored value is read. `ArithFloat` is asked the same way and for the same
reason: two fields could disagree and one cannot.

**One shape is knowingly short of the shell, and it is a wording.** zsh
cleans the token and then reports what is left of the *cleaned* text, so
`$(( 1e_foo ))` blames `efoo` there and `e_foo` here. Both stop in the
same place and both are an `operator expected`.

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

`setopt c_bases` rewrites the mark for bases 8 and 16 — `$(( [#16] 255 ))`
becomes `0xFF`, and with `octal_zeroes` as well `$(( [#8] 8 ))` becomes
`010` — and it moves `typeset -i16` in the same shell by the same amount,
which is the second reason the two constructs share one renderer.
Implemented since #4502 as `Semantics.IntegerBaseMarkIsCSpelled`, written
by zsh's `cbases` entry. **Base eight is the control**: under `c_bases`
alone it is unmoved, as are base 2 and base 36, because only sixteen has a
C literal of its own; eight's is a leading zero and is only C's spelling
in a shell that *reads* one as octal, which is why that half is the axis
and `ArithLeadingZeroIsOctal` together. `$(( [##16] 255 ))` is `FF` in
both states — no mark is written, so there is nothing to respell.

## Operators

Precedence follows C, highest first — in five of the six columns. Measured
spot-checks over the part they all agree about are unanimous: `1+2*3` is 7,
`(1+2)*3` is 9, `2*3%4` is 2. Where zsh parts company is the section below
this table.

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

**Off does not mean refused.** Where the increment operators are absent the
doubled sign is not a token the reader then rejects: it is two unary signs,
and the expression evaluates. Measured 2026-09-17 on dash 0.5.12, the one
panel column without them, against BusyBox ash 1.37.0, which has them:

| written, with `x=5` | dash 0.5.12 | BusyBox ash 1.37.0 |
| --- | --- | --- |
| `$(( --x ))` | `5` | `4` |
| `$(( ++x ))` | `5` | `6` |
| `$(( -- -x ))` | `-5` | — |
| `$(( x++ ))` | `arithmetic expression: expecting primary: " x++ "` | `5` |
| `$(( -- ))` | `arithmetic expression: expecting primary: " -- "` | `arithmetic syntax error` |

So a dialect without the operators reaches the ordinary missing-operand
failure through the sign it already has, and needs no refusal of its own. Ours
had one — `-- is not available in this dialect` — which is a sentence about
this parser rather than about the script, and it refused `--x`, which dash
evaluates (#3475).

## zsh binds the shifts and the bitwise operators differently

The ladder above is one of two, and zsh uses the other by default. Its own
manual documents both and names the first `c_precedences`, which is as
explicit as a disagreement between shells gets: the order below is a
decision rather than a bug.

Two rungs move, and nothing else does. The shifts go to the **tightest**
binary level, above `*` as well as above `+`; and `&`, `^` and `|` go above
`**`, keeping their order relative to each other.

    <<  >>                    shifts
    &                         bitwise and
    ^                         bitwise xor
    |                         bitwise or
    **                        exponentiation
    *   /   %                 multiplicative
    +   -                     additive
    <   <=  >   >=            relational
    ==  !=                    equality
    &&                        logical and
    ||  ^^                    logical or, logical xor

Measured 2026-09-15 from a script file, `env -i` with a scratch HOME:

| probe | zsh | bash | ksh93 | dash |
| --- | --- | --- | --- | --- |
| `$(( 1 << 2 + 1 ))` | **5** | 8 | 8 | 8 |
| `$(( 1 + 2 << 1 ))` | **5** | 6 | 6 | 6 |
| `$(( 1 << 2 * 2 ))` | **8** | 16 | 16 | 16 |
| `$(( 16 >> 1 + 1 ))` | **9** | 4 | 4 | 4 |
| `$(( 1 < 2 & 1 ))` | **0** | 1 | 1 | 1 |
| `$(( 6 \| 1 + 1 ))` | **8** | 6 | 6 | 6 |
| `$(( 2 ** 1 \| 3 ))` | **8** | 3 | 3 | error |
| `$(( 2 \| 1 ** 3 ))` | **27** | 3 | 3 | error |

Parenthesized, every column agrees — `$(( 1 << (2 + 1) ))` is 8 and
`$(( (1 < 2) & 1 ))` is 1 everywhere — which is what says this is precedence
and not a broken operator.

The last two rows are the ones a reading of the shift rows alone would miss.
`**` is *looser* than the bitwise operators here, so `2 ** 1 | 3` groups as
`2 ** (1 | 3)`. Associativity does not move with the rungs: `**` is
right-associative and everything else left-associative under both orders,
and `2 ** 3 ** 2` is 512 in every column that has the operator.

`^^` is zsh's logical XOR and shares a level with `||` in this order — see
the section below, which is where the operator itself is recorded.

Grammar flag: `ArithPrecedence` — core, `posix`, `bash`, `ksh`, `dash` and
`ash`: `ArithPrecedenceAsInC`; `zsh`: `ArithPrecedenceShiftsAndBitwiseBind‑
Tighter`.

**And the option is a run-time one, which the parse is arranged around.**
`setopt c_precedences` changes what an expression answers on a line the
parser has already read, and changes it inside a function whose body was
read before the option was touched — measured on zsh 5.9.2. So an
expression is read again when it runs while the option is in charge, the
same way one holding a `$` already is. `interp.Runner.SetArithPrecedence`
is what the option moves.

## `^^` — the logical exclusive-or, zsh alone

`^^` is 1 where exactly one operand is true and 0 otherwise, the same 1-or-0
result every other logical operator yields. Nobody writes one by accident and
no script that runs under bash can contain one, but the spelling is already
*taken*: `^` is bitwise xor, so `a ^^ b` in every other column is an xor whose
right operand is missing.

Measured 2026-09-18 on zsh 5.9.2, `env -i` with `LC_ALL=C`:

| written | zsh | and the contrast |
| --- | --- | --- |
| `$(( 1 ^^ 1 ))` | 0 | |
| `$(( 1 ^^ 0 ))` | 1 | |
| `$(( 0 ^^ 1 ))` | 1 | |
| `$(( 0 ^^ 0 ))` | 0 | |
| `$(( 5 ^^ 3 ))` | **0** | `$(( 5 ^ 3 ))` is 6 — the truth, not the bits |
| `$(( 2 ^^ 0 ))` | 1 | |
| `$(( 1.5 ^^ 2.5 ))` | **0** | truth is the value's, not the kind's |
| `$(( 0 ^^ (y=9) ))` | 1, and `y` is 9 | both operands evaluated |
| `$(( 1 ^^ (y=9) ))` | 0, and `y` is 9 | nothing to short-circuit |

**Where it sits takes four probes and not one.** Three readings survive the
obvious `$(( 1 || 0 ^^ 1 ))`, which is 0 under all of them:

| written | default | `c_precedences` | what it rules out |
| --- | --- | --- | --- |
| `$(( 1 || 0 ^^ 1 ))` | 0 | **1** | tighter than `\|\|` by default |
| `$(( 1 ^^ 1 \|\| 1 ))` | 1 | 1 | looser than `\|\|` — that reading gives 0 |
| `$(( 1 \|\| 1 ^^ 1 ))` | **0** | 1 | the two ladders, in one row |
| `$(( 1 ^^ 0 && 0 ))` | 1 | 1 | sharing a rung with `&&` — that gives 0 |

So by default it is **on the `||` rung, left-associative**, and under
`setopt c_precedences` it takes a rung of its own **between `||` and `&&`**.

There is an assignment spelling, `^^=`, and it is the one compound assignment
here that **assigns nothing**:

    x=5; echo $(( x ^^= 1 )); echo $x     0 then 5
    x=5; echo $(( x ||= 0 )); echo $x     1 then 1
    x=5; echo $(( x &&= 0 )); echo $x     0 then 0
    x=5; echo $(( x ^= 1 ));  echo $x     4 then 4

It binds as an assignment does — `x=1; $(( x ^^= 1 || 1 ))` is 0, which is
`x ^^ (1 || 1)` — and where the left side cannot be a target it is still the
exclusive-or of what precedes it: `x=0; $(( 1 || x ^^= 1 ))` is 0, which is
`(1 || x) ^^ 1`.

Grammar flag: `ArithLogicalXor` — `zsh` only. With it off, `a ^^ b` is the
bitwise operator running out of operand, which is what every other column
reports.

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

### A group that will not close is an arithmetic failure, not a parse one

A parenthesised sub-expression that reads a complete value and then meets text
it can use for neither an operator nor a close — `(echo a)`, where `echo` is
the value and `a` is neither — is reported the way a division by zero is: the
expression quoted back, the ordinary arithmetic status, and the script carrying
on wherever a failed `(( ))` is not fatal. No shell in the panel calls it a
syntax error.

Measured 2026-09-17, `env -i` with `LC_ALL=C` over a script file, `echo B; echo
$(( (echo a) )); echo A`:

| column | written | status |
| --- | --- | ---: |
| bash 5.3.20 | ``(echo a) : missing `)' (error token is "a) ")`` | 1 |
| zsh 5.9.2 | ``bad math expression: operator expected at `a) '`` | 1 |
| ksh93u+ 2012-08-01 | `` (echo a) : arithmetic syntax error`` | 1 |
| dash 0.5.12 | `arithmetic expression: expecting ')': " (echo a) "` | 2 |
| BusyBox ash 1.37.0 | `arithmetic syntax error` | 2 |

Two columns have a sentence for the unclosed group that they give no other
leftover: the same shells write `arithmetic syntax error in expression` and
`expecting EOF` for `$(( (1)x ))`, where the group *did* close. The other
three say for both exactly what they say for any text an expression could not
use.

Diagnostics: `ArithMissingCloseParen`, worded in bash and dash; empty falls
back to `ArithOperatorExpected`, which is what a dialect without a sentence of
its own would have said had the group closed.

The status is the half a caller can read. Ours wrote the parser's own prose —
`expected ) in arithmetic`, a sentence about a production — and refused the
text as a *parse* failure, which took the file down before anything ran: the
dialect that ends a script over a failed `(( ))` ended it at a syntax error's
3 where its reference ends it at the arithmetic 1, and a script trapping on 1
could not tell this from an unmatched quote (#3071).

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

### And neither is a subscript's expanded text

The same rule reaches the other text that arrives already expanded. A
subscript and a substring's range are **words**, expanded by their caller
before anything arithmetic happens, so a `$` still standing in the result
is an ordinary character there too. Measured 2026-09-15 from a script file,
with `k` holding the six characters `$(echo 1)`:

| | bash 5.3.20 | bash 3.2.57 | ksh93u+ | zsh 5.9.2 |
| --- | --- | --- | --- | --- |
| `k='$(echo 1)'; a[$k]=V` | `$(echo 1): ... operand expected` | same, `syntax error` | `arithmetic syntax error` | invents an index |
| `k='$i'; i=2; a[$k]=V` | `$i: ... operand expected` | same | `arithmetic syntax error` | ``operator expected at `i' `` |
| ``k='`echo 1`'`` | operand expected | same | `arithmetic syntax error` | operand expected |
| `k='$((1))'` | operand expected | same | `arithmetic syntax error` | ``operator expected at `((1))' `` |
| `k='1+1'; a[$k]=V` | element 2 | element 2 | element 2 | element 2 |
| `k='i'; i=2; a[$k]=V` | element 2 | element 2 | element 2 | element 2 |
| `w='$(echo 1)'; ${x:$w:2}` | operand expected | same | `arithmetic syntax error` | operand expected |

The last two rows are what makes this a *reading* rather than a refusal:
the text is still an expression and a **name** in it is still resolved,
because resolving a name is the evaluator's job and not a second round of
expansion. Only the expansion is spent.

**None of the four runs the command.** That is the half that mattered: a
subscript is a place data flows into — `a[$key]=…` with `key` read from a
file, an argument or the environment is ordinary script text — and this
implementation performed the substitution a second time and executed it,
in every dialect, assigning the element the output named (#3047).

zsh is the one column not reproduced. It neither refuses the text nor
evaluates it but invents an index from it: `k='${i}'` assigns element
**17366** of a four-element array, and `w='$'` as a range offset expands to
nothing at status 0. Ours gives the operand sentence its own bash and ksh
give, which is the answer the other three shells agree on; matching zsh's
number would be reproducing garbage.

**The boundary.** A subscript that arrives as *text* rather than as a word
— a builtin's operand, a reference resolved at run time — has **not** been
expanded yet, and is: `unset 'a[$i]'`, `read 'v[${#v}+1]'` and
`v='x[$(echo 2)]'` all keep their `$` through word expansion because the
quotes protect it, so the expansion happens when the subscript is read. It
is once either way, which is the whole of the rule (#1852).

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

`||=` and `&&=`, the two logical compound assignments zsh has beside `^^=`.
Both of those **store** — `x=5; $(( x ||= 0 ))` leaves `x` at 1, measured
2026-09-18 — which is the half `^^=` does not do, and neither is read here.
