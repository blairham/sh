// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The builtins that do not exist until their module is loaded.
//
// The builtin half of what gatedparameters.go does for parameters, and #4922
// deliberately left it: a missing builtin is told by name at its own call
// site — `command not found: strftime` on the line that wrote it — where a
// missing parameter had no such line, which is why the parameter half was
// the half worth doing first.
//
// # It is not every module builtin, and that is the whole of the measurement
//
// #4997 reported it as "every other builtin a module names", naming
// `zparseopts` and `zstyle` among them. Those two are **there** in a fresh
// shell. Measured 2026-09-28 on zsh 5.9.2 under `-f` from a script file with
// `env -i PATH=/usr/bin:/bin TERM=dumb` and a scratch `HOME`, one `whence -w`
// per shell, over every builtin this shell's own module table names:
//
//	present   zformat zparseopts zregexparse zstyle          zsh/zutil
//	          bindkey vared zle                              zsh/zle
//	          sched                                          zsh/sched
//	          compadd compset                                zsh/complete
//	          comparguments compdescribe compfiles           zsh/computil
//	          compgroups compquote comptags comptry compvalues
//	          echoti                                         zsh/terminfo
//	          echotc                                         zsh/termcap
//	          limit ulimit unlimit                           zsh/rlimits
//
//	absent    strftime                                       zsh/datetime
//	          syserror sysopen sysread sysseek syswrite zsystem   zsh/system
//	          zselect                                        zsh/zselect
//	          zstat                                          zsh/stat
//	          zpty                                           zsh/zpty
//	          zf_chgrp zf_chmod zf_chown zf_ln zf_mkdir      zsh/files
//	          zf_mv zf_rm zf_rmdir zf_sync
//
// **And `zmodload -e` says none of the fourteen modules is loaded**, which is
// the control that says "present" is not "the module is already in". A
// builtin that is there while its module is not is that shell's *autoloadable*
// declaration: the name is a stub in the builtin table and calling it loads
// the module. So the split is not between modules that are loaded and modules
// that are not — every one of them is out — it is between builtins the module
// declares autoloadable and builtins it does not.
//
// Modeling the stub is a separate question and is not what this is: an
// autoloadable builtin here is simply present, which is what it looks like
// from a script until something asks `zmodload -e` about its module. What is
// modeled is the **absence**, because that is the row a script can take the
// wrong arm on: `whence -w strftime` is `none` there and was `builtin` here.
//
// The plain `zsh/files` names — `rm`, `mkdir`, `mv` and the rest — are not
// here because this shell does not register them as builtins at all: measured
// in the same run, `whence -w rm` is `command` in both columns.
// **Cut from `zmodloadFeatures` and checked against it**: a name here that
// that table does not list as a `b:` feature of the same module is a builtin
// nothing could ever bring back, because the load path that restores them is
// the feature selection. See TestEveryGatedBuiltinIsAFeatureOfItsModule.
var zshGatedBuiltins = map[string][]string{
	"zsh/datetime": {"strftime"},
	"zsh/system": {
		"syserror", "sysopen", "sysread", "sysseek", "syswrite", "zsystem",
	},
	"zsh/zselect": {"zselect"},
	"zsh/stat":    {"zstat"},
	"zsh/zpty":    {"zpty"},
	"zsh/files": {
		"zf_chgrp", "zf_chmod", "zf_chown", "zf_ln", "zf_mkdir",
		"zf_mv", "zf_rm", "zf_rmdir", "zf_sync",
	},
}

// withdrawGatedBuiltins takes them out of the table at startup, which is
// where a fresh shell has them.
//
// Withdrawn rather than unregistered, which is the same distinction the
// feature selection already draws: a withdrawn builtin is one this shell
// **has** and is not currently offering, so `zmodload zsh/datetime` still
// sees `b:strftime` as a feature it holds and the load is not refused by the
// gate it is about to open. See interp.Runner.SetBuiltinWithdrawn and
// zmodloadEnforce, which is the other writer of the same state.
func withdrawGatedBuiltins(r *interp.Runner) {
	for _, names := range zshGatedBuiltins {
		for _, name := range names {
			r.SetBuiltinWithdrawn(name, true)
		}
	}
}

// **Nothing puts them back but the load path that was already there.**
// zmodloadEnforce is the other writer of the withdrawn state and a plain
// `zmodload` widens the selection to every feature the module names, so the
// builtins come back by the same road `zmodload -F +b:name` brings one back
// — and a *narrowed* load puts back only what it named, which is the row
// TestTheNamesThatWouldShadowACommandArriveOnlyWhenAsked asserts and which an
// installer of its own broke: `zmodload -F zsh/stat b:stat` restored `zstat`
// as well, because a roster keyed on the module cannot see the selection.
