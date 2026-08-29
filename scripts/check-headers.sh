#!/bin/sh
# Every Go source file carries the two-line SPDX header. See AGENTS.md.
#
# Headers are the mechanism that keeps attribution attached to a file
# that is copied out of this repository; a root LICENSE does not travel
# with it. That is the whole reason this check exists, so it is a
# failure and not a warning.
set -eu

want_id='SPDX-License-Identifier: Apache-2.0'
want_cr='SPDX-FileCopyrightText:'
missing=''

for f in $(git ls-files '*.go'); do
	# Generated files are exempt: the generator's template owns their
	# first lines, and rewriting them on every regeneration is churn.
	if head -5 "$f" | grep -q '^// Code generated .* DO NOT EDIT\.$'; then
		continue
	fi
	if ! head -5 "$f" | grep -qF "$want_id"; then
		missing="$missing  $f (no $want_id)
"
		continue
	fi
	if ! head -5 "$f" | grep -qF "$want_cr"; then
		missing="$missing  $f (no $want_cr)
"
	fi
done

if [ -n "$missing" ]; then
	printf 'Missing SPDX headers:\n%s\n' "$missing"
	printf 'Add to the top of each file:\n\n'
	printf '  // SPDX-FileCopyrightText: 2026 Blair Hamilton\n'
	printf '  // SPDX-License-Identifier: Apache-2.0\n\n'
	exit 1
fi

n=$(git ls-files '*.go' | wc -l | tr -d ' ')
printf 'SPDX headers present on all %s Go files.\n' "$n"
