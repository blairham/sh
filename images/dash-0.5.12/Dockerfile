# SPDX-FileCopyrightText: 2026 Blair Hamilton
# SPDX-License-Identifier: Apache-2.0

# An unpatched upstream dash 0.5.12, because no distribution ships one.
#
# `make suite`'s dash column is graded against `esc=4 pipefail-listed=n` — the
# fingerprint its own AgainstProbe asks for, which upstream 0.5.12 and Apple's
# dash-16 both answer and which every Debian- and Ubuntu-derived build answers
# differently. See images/README.md.

FROM debian:sid-slim@sha256:ec3fa4e0b2987ae47be353f56854191e300261915470e40165b1c906d22a65db AS build

# The tarball is pinned by content, not by URL: this exact archive is what
# deb.debian.org carries as dash_0.5.12.orig.tar.gz and what upstream's own
# host serves as dash-0.5.12.tar.gz, byte for byte (measured 2026-09-20).
ARG DASH_URL=http://deb.debian.org/debian/pool/main/d/dash/dash_0.5.12.orig.tar.gz
ARG DASH_SHA256=6a474ac46e8b0b32916c4c60df694c82058d3297d8b385b74508030ca4a8f28a

RUN set -eu; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl gcc libc6-dev make; \
    rm -rf /var/lib/apt/lists/*; \
    curl -fsSLo /tmp/dash.tar.gz "$DASH_URL"; \
    echo "$DASH_SHA256  /tmp/dash.tar.gz" | sha256sum -c -; \
    mkdir -p /tmp/src; \
    tar -xzf /tmp/dash.tar.gz -C /tmp/src --strip-components=1; \
    cd /tmp/src; \
    ./configure --prefix=/usr; \
    make -j"$(nproc)"; \
    make install DESTDIR=/out

FROM debian:sid-slim@sha256:ec3fa4e0b2987ae47be353f56854191e300261915470e40165b1c906d22a65db

# Over the distribution's own, deliberately: /bin/sh is a symlink to /bin/dash
# here, so the whole image speaks the build this column names rather than
# holding two dashes and leaving which one is found to a lookup order.
COPY --from=build /out/usr/bin/dash /bin/dash

# The column's own AgainstProbe, run at build time. An image that does not
# answer it is not this reference, and finding that out here is cheaper than
# finding it out from a WRONG BUILD banner on a runner.
RUN set -eu; \
    got=$(/bin/dash -c 'e=$(printf "a\eZ"); case $(set -o) in *pipefail*) p=y ;; *) p=n ;; esac; printf "esc=%s pipefail-listed=%s\n" "${#e}" "$p"'); \
    [ "$got" = "esc=4 pipefail-listed=n" ] || { echo "this build answers $got" >&2; exit 1; }
