// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

// The links macOS ships at the root, and only those.
//
// All three are symbolic links the operating system installs into `/private`,
// present on every installation, owned by root, and not something a script can
// change. `ls -l /` shows them and has since OS X.
//
// They are here because a resolution answers with the physical path —
// the walk follows /var and /tmp as the links they are, and F_GETPATH answered
// the same way before it —
// so on a Mac *every* access under `/tmp`, `/var` or `/etc` — which is to say
// every temporary file any script writes — comes back spelled differently
// from the name that was asked about. Without this, Elsewhere would report
// every one of them as having resolved somewhere else and a gate that prompts
// would ask a person twice for the same file. That is the crying-wolf
// failure the permission model is most careful about; see
// internal/acp's Escalates for the same argument made about probe actions.
//
// The bar for an entry is deliberately high and is the one internal/policy
// sets for the table it keeps for the rule side: unconditional on the
// platform. A path that is a link on most machines is a machine's
// arrangement, not the platform's, and suppressing a check for one would be
// suppressing it on the say-so of whoever made the link. `/bin -> usr/bin` on
// a merged-/usr Linux is the example that is deliberately absent.
//
// Stated here as well as in internal/policy because the two are separate
// statements about the same operating system rather than one piece of shared
// logic — that package widens a *pattern* when a file is read, this compares
// two *paths* when a file is opened — and each is measured against the
// running system by a test of its own, so neither can drift from the platform
// without failing.
var platformLinks = [][2]string{
	{"/tmp", "/private/tmp"},
	{"/var", "/private/var"},
	{"/etc", "/private/etc"},
}
