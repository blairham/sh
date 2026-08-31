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

Vector field: `ArithFloat` (default false).

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

Vector fields: `ArithLeadingZeroIsOctal` (default true) and
`ArithExplicitBase` (default true, false for `posix`).

## Operators

Precedence follows C, highest first. Measured spot-checks are unanimous:
`1+2*3` is 7, `(1+2)*3` is 9, `2*3%4` is 2.

    ++  --                    increment, decrement   (not POSIX; absent from dash)
    +   -   ~   !             unary
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

Vector fields: `ArithIncDec` and `ArithComma`, both default true, both
false for `posix`.

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
