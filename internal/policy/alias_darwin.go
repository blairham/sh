// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy

// The aliases macOS ships, and only those.
//
// All three are symbolic links created by the operating system into
// `/private`, present on every installation, owned by root, and not something a
// script can change. `ls -l /` shows them and has since OS X: `/tmp ->
// private/tmp`, `/var -> private/var`, `/etc -> private/etc`.
//
// Nothing else is here, and the bar is deliberately high: an entry has to be
// unconditional on the platform. A path that is a link on most machines is not
// a platform alias, it is a machine's arrangement, and a table that guessed
// would make a policy mean something different on two boxes — which is worse
// than the hole it closed, because it would be a difference nobody could see in
// the file.
var platformAliases = []alias{
	{from: "/tmp", to: "/private/tmp"},
	{from: "/private/tmp", to: "/tmp"},
	{from: "/var", to: "/private/var"},
	{from: "/private/var", to: "/var"},
	{from: "/etc", to: "/private/etc"},
	{from: "/private/etc", to: "/etc"},
}
