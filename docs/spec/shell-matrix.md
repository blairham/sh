# Measured feature matrix

Which constructs exist in which shells. **Measured**, not recalled —
each cell is an oracle run on the machine noted below. Re-run rather
than trust this file if the answer matters.

Panel: `dash`, `/bin/sh` (bash 3.2.57 invoked as `sh`), bash 5.3.15,
ksh93 (`/bin/ksh`), zsh 5.9.2. macOS arm64, 2026-08-29.

| feature | dash | bash3.2 | bash5.3 | ksh93 | zsh |
| --- | --- | --- | --- | --- | --- |
| `$((...))` arithmetic | yes | yes | yes | yes | yes |
| `${x#pat}` parameter ops | yes | yes | yes | yes | yes |
| `local` | yes | yes | yes | **no** | yes |
| `typeset` | **no** | yes | yes | yes | yes |
| `$'...'` | **no** | yes | yes | yes | yes |
| `+=` append | **no** | yes | yes | yes | yes |
| `${x:1:2}` substring | **no** | yes | yes | yes | yes |
| `${x/b/X}` substitution | **no** | yes | yes | yes | yes |
| `${x^^}` case conversion | **no** | **no** | yes | **no** | **no** |
| arrays | **no** | yes | yes | yes | yes |
| `[[ ... ]]` | **no** | yes | yes | yes | yes |
| `for ((;;))` | **no** | yes | yes | yes | yes |
| `function f {}` | **no** | yes | yes | yes | yes |
| `<<<` herestring | **no** | yes | yes | yes | yes |
| `<(...)` process subst. | **no** | *see note* | yes | yes | yes |
| `function f { }` | **no** | yes | yes | yes | yes |
| `function f() { }` | **no** | yes | yes | **no** | yes |
| `case` `;&` fallthrough | **no** | **no** | yes | yes | yes |
| `case` `;;&` continue | **no** | **no** | yes | **no** | **no** |

**Note on process substitution.** bash 3.2 *has* it, and loses it when
invoked as `sh`:

    $ /bin/bash -c 'cat <(echo hi)'   →  hi
    $ /bin/sh   -c 'cat <(echo hi)'   →  syntax error near unexpected token `('

Same binary, two languages, selected by argv[0]. This is the clearest
available evidence that a dialect is a *runtime switch*, not a build-time
identity — and it is why this implementation models dialect as state
rather than as a compile-time choice.

`${x^^}` is a bash 4 feature: bash 3.2 lacks it regardless of invocation.

## What this decided

**dash is the sole holdout on 12 of the 15 rows.** The core boundary
therefore hinges entirely on whether dash is in the panel:

- Include dash → the core collapses to roughly POSIX plus `local`.
- Exclude dash → the core is the ksh-family common denominator: arrays,
  `[[ ]]`, `$'...'`, `+=`, substrings, pattern substitution, C-style
  `for`, `function`, herestrings and process substitution.

**The second was chosen.** See `core.md`.

Note the two rows that break any tidy story: `local` is absent from
ksh93 but present in dash, and `typeset` is the reverse. **There is no
portable spelling for a function-local variable across this panel.**
The core provides `local`, and ksh93 is not a compatibility target.
