# SPDX-FileCopyrightText: 2026 Blair Hamilton
# SPDX-License-Identifier: Apache-2.0

# A zsh 5.9.2, because no image ships one and 5.9 is not a substitute.
#
# `make suite`'s zsh column is graded against `zsh 5.9.2`, and 5.9 answers two
# of that column's own files differently — `kill -L` exists in 5.9.2 and not in
# 5.9, and `kill -l` of an out-of-range number answers the number rather than
# wrapping. See images/README.md.

FROM debian:sid-slim@sha256:ec3fa4e0b2987ae47be353f56854191e300261915470e40165b1c906d22a65db

# No version pin on the package: sid moves, so a pin here would fail the build
# within days, and the thing that is pinned is the published image's digest
# rather than this recipe. What keeps the recipe honest is the assertion —
# whatever is installed has to be the build the column names, or no image is
# published at all.
RUN set -eu; \
    apt-get update; \
    apt-get install -y --no-install-recommends zsh; \
    rm -rf /var/lib/apt/lists/*; \
    v=$(zsh --version); \
    case $v in 'zsh 5.9.2 '*) ;; *) echo "this build reports $v" >&2; exit 1 ;; esac

# The two behaviors the column's files split 5.9.2 from 5.9 on, asked of the
# build that is about to be published rather than of the one that was measured.
RUN set -eu; \
    zsh -c 'kill -L' >/dev/null 2>&1 || { echo 'kill -L is refused: this is a 5.9' >&2; exit 1; }; \
    [ "$(zsh -c 'kill -l 257')" = 257 ] || { echo 'kill -l wraps: this is a 5.9' >&2; exit 1; }
