// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package secret

// The table.
//
// Every entry is written from a published fact about the credential's format:
// a vendor's documented prefix and length, the PEM armor that is part of the
// container, or the syntax of the thing the credential is written into — an
// HTTP header, a URL's authority, a shell assignment. Nothing here was copied
// from another scanner; the shapes are facts and the expression is ours.
//
// Two properties are kept deliberately, and a new rule has to keep them:
//
//   - **A rule says why it matched.** The name reaches the person whose line
//     was not recorded, and it is the only way anyone can report that a rule
//     is wrong.
//
//   - **A rule matches the *literal* credential, never a reference to one.**
//     `TOKEN=$SECRET` and `Authorization: Bearer $TOKEN` are the correct way
//     to write those lines, and a scrubber that punishes them teaches people
//     to stop using it. Every value pattern here excludes `$` at the front
//     for that reason, which is also why they are patterns over the value
//     rather than over the name.

// ruleSource is one entry before compilation, kept separate so the table
// reads as data.
type ruleSource struct {
	name    string
	pattern string
}

// The character classes the general rules share.
//
// valueHead is what a credential may begin with, and its whole job is to
// exclude two things: `$`, which makes the value a reference to a secret
// rather than a secret, and `/`, which makes it a path — `--secret-file=/etc/x`
// names a file and is not itself one.
//
// valueTail is everything up to whatever ends a word in shell. A slash is
// allowed here: base64 is full of them, and stopping at the first one would
// redact half a credential and leave the rest in the file.
const (
	valueHead = `[A-Za-z0-9._+=~-]`
	valueTail = `[^\s"'\x60;|&$<>()]`
)

var defaultRules = []ruleSource{
	// AWS names its key ids by prefix: four letters saying what kind of key
	// it is, then sixteen more of the same alphabet. Twenty upper-case
	// characters in that shape are not a word anyone typed by accident.
	{"aws-access-key-id", `\b(?:AKIA|ASIA|ABIA|ACCA)[A-Z0-9]{16}\b`},

	// The matching secret has no prefix of its own — forty characters of
	// base64 look like anything else forty characters long — so this rule
	// asks the *line* what the value is rather than the value itself.
	{"aws-secret-access-key", `(?i)aws_secret_access_key\s*[=:]\s*["']?([A-Za-z0-9/+=]{40})`},

	// GitHub's tokens are prefixed by type, and the fine-grained ones by
	// `github_pat_`. Both are long enough that the prefix plus the length is
	// the whole of the identification.
	{"github-token", `\b(?:gh[pousr]_[A-Za-z0-9]{36,255}|github_pat_[A-Za-z0-9_]{22,255})\b`},

	// GitLab uses one prefix family for personal, runner, deploy and other
	// token kinds, each followed by the token proper.
	{"gitlab-token", `\bgl(?:pat|rt|dt|soat|ptt|cbt)-[A-Za-z0-9_-]{20,}`},

	// Slack's tokens carry the workspace and the kind in the prefix.
	{"slack-token", `\bxox[abprse]-[A-Za-z0-9-]{10,}`},

	// Stripe's live and test keys, secret and restricted alike.
	{"stripe-api-key", `\b[sr]k_(?:live|test)_[A-Za-z0-9]{16,}\b`},

	// A Google API key is a fixed prefix and a fixed length.
	{"google-api-key", `\bAIza[A-Za-z0-9_-]{35}\b`},

	// PEM armor. The word between BEGIN and PRIVATE varies by key type and
	// some are wrapped as a BLOCK, so the middle is left open — but it is
	// upper case and short, which keeps this from matching prose about
	// private keys.
	//
	// A whole key does not fit on a command line, and it does not need to:
	// `echo "-----BEGIN …" > id_rsa` puts the first line there, and the
	// first line is the tell.
	{"private-key-block", `-----BEGIN [A-Z0-9 ]{0,32}PRIVATE KEY(?: BLOCK)?-----`},

	// A JSON Web Token is three base64url segments separated by dots, and it
	// always starts `eyJ` because that is what `{"` encodes to. It is a
	// bearer credential wherever it appears.
	{"jwt", `\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`},

	// `Authorization: <scheme> <credential>`, with the scheme optional
	// because the header is written both ways. The value stops at a quote or
	// a space, and may not start with `$`.
	{"authorization-header", `(?i)\bauthorization\s*:\s*(?:[A-Za-z]+\s+)?([^\s"'$][^\s"']{7,})`},

	// The scheme on its own, which is how it is usually written on a command
	// line: `curl -H "Bearer …"`. Sixteen characters is short for a bearer
	// token and long for a word, which is what keeps this off prose.
	{"bearer-token", `(?i)\bbearer\s+([A-Za-z0-9._~+/-]{16,}=*)`},

	// A password in a URL's authority — `scheme://user:password@host`, the
	// shape a database connection string takes. The password may not be
	// empty, may not be `$VAR`, and stops at the `@`.
	{"connection-string-password", `(?i)\b[a-z][a-z0-9+.-]*://[^\s/:@]+:([^\s/@$][^\s/@]{2,})@`},

	// The general case, and the only rule that can be wrong about an
	// ordinary line, so it is the most constrained: a name that *contains*
	// one of these words — `AWS_SECRET_ACCESS_KEY` and `--api-key` both do,
	// and neither has the word at a boundary — then `=`, then a value of at
	// least eight characters that is neither a variable reference nor a
	// path.
	//
	// Eight is the length at which `token=1` and `password=x` stop being
	// matched, which are the assignments a person writes while testing
	// something rather than the ones that carry a credential.
	{
		"credential-assignment",
		`(?i)[A-Za-z0-9_.-]*(?:password|passwd|secret|token|api_?key|access_?key|credential)` +
			`[A-Za-z0-9_.-]*\s*=\s*["']?(` + valueHead + valueTail + `{7,})`,
	},
}
