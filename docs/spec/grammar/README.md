# Grammar and expansion specs

Per-construct behavioural specs. Each entry follows the shape set out in
`../README.md`: the construct, the core answer, the measured behaviour of
the reference panel, a citation, and the semantics-vector field that
governs it where one does.

Everything here was measured on the panel described in `../oracle.md`
(dash, bash 5.3, ksh93, zsh 5.9.2; macOS arm64, 2026-08-29) or taken
from POSIX.1-2024 XCU §2. Probes are recorded inline so a claim can be
re-run rather than trusted.

## Entries

    tokenization.md    input to tokens, and where quoting is recorded
    commands.md        tokens to commands: precedence and structure
    expansion.md       the expansion pipeline and its ordering
    word-splitting.md  field splitting and IFS

## Reading order

`tokenization.md` first: it produces the words everything else consumes,
and it is where quoting is recorded. Then `expansion.md`, which
establishes the pipeline, and word splitting is one stage within it. The single most important fact in both documents is
that the stages are **ordered**, and that most surprising shell behaviour
is a consequence of that order rather than of any individual stage.
