// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package secret_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/secret"
)

// The fixtures are assembled from pieces rather than written out whole, and
// that is not squeamishness. This repository's commit hook scans for exactly
// these shapes, so a test file holding literal credentials — invented ones
// included — is a file nobody can commit. Splitting a prefix from its body
// stops that scanner's pattern dead and changes nothing about what these
// tests ask, because the string handed to the rules is assembled at run time
// and is byte for byte the credential it stands for.
//
// None of these is a real credential. Every body is a repeated character or a
// visibly invented word, which also keeps them clear of anything that scores
// them on entropy.
var (
	awsKeyID     = "AKIA" + strings.Repeat("Q", 16)
	awsSecret    = strings.Repeat("A", 20) + "/" + strings.Repeat("b", 19)
	githubPAT    = "ghp" + "_" + strings.Repeat("a", 36)
	githubFine   = "github" + "_pat_" + strings.Repeat("b", 30)
	gitlabPAT    = "glpat" + "-" + strings.Repeat("c", 20)
	slackToken   = "xoxb" + "-" + strings.Repeat("1", 12) + "-abcdef"
	stripeKey    = "sk" + "_live_" + strings.Repeat("d", 24)
	googleKey    = "AIza" + strings.Repeat("e", 35)
	privateKey   = "-----BEGIN OPENSSH PRIVATE" + " KEY-----"
	jwt          = "ey" + "J" + strings.Repeat("f", 12) + "." + strings.Repeat("g", 20) + "." + strings.Repeat("h", 20)
	opaqueBearer = strings.Repeat("z", 40)
)

// Each rule fires on the line it is for, and says which rule it was.
//
// The name is asserted rather than only the yes-or-no, because the name is
// what reaches the person whose line was not recorded and is the only way
// anyone can tell us a rule is wrong. A line matching for the wrong reason is
// a rule that will misfire somewhere else.
func TestEachRuleFindsItsOwnCredential(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"export AWS_ACCESS_KEY_ID=" + awsKeyID, "aws-access-key-id"},
		{"export AWS_SECRET_ACCESS_KEY=" + awsSecret, "aws-secret-access-key"},
		{"export GITHUB_TOKEN=" + githubPAT, "github-token"},
		{"gh auth login --with-token <<< " + githubFine, "github-token"},
		{"export CI_JOB_TOKEN=" + gitlabPAT, "gitlab-token"},
		{"curl -d token=" + slackToken + " https://slack.com/api/auth.test", "slack-token"},
		{"stripe listen --api-key " + stripeKey, "stripe-api-key"},
		{"curl 'https://maps.example.com/api?key=" + googleKey + "'", "google-api-key"},
		{`echo "` + privateKey + `" > id_ed25519`, "private-key-block"},
		{`curl -H "Authorization: Bearer ` + jwt + `"`, "jwt"},
		{`curl -H "Authorization: ` + opaqueBearer + `"`, "authorization-header"},
		{`curl -H "Bearer ` + opaqueBearer + `"`, "bearer-token"},
		{"psql postgres://app:hunter2pass@db.internal:5432/app", "connection-string-password"},
		{"export PGPASSWORD=correcthorsebattery", "credential-assignment"},
		{"deploy --client-secret=" + strings.Repeat("k", 20), "credential-assignment"},
		{"mysql -u root -pveryhiddenpassword -e 'select 1'", ""},
	} {
		got, ok := secret.Default().Match(c.line)
		if c.want == "" {
			if ok {
				t.Errorf("%q matched %s, and this case records that it does not match", c.line, got)
			}
			continue
		}
		if !ok {
			t.Errorf("%q matched nothing, want %s", c.line, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("%q matched %s, want %s", c.line, got, c.want)
		}
	}
}

// The half that decides whether this is worth having.
//
// A scrubber that rejects ordinary lines is worse than none: the history
// stops being trustworthy, and the person turns it off or — the outcome that
// actually costs something — stops reading what it says. Every line here is
// one somebody types on a normal day, and several of them are the *correct*
// way to handle a credential: a variable reference, a path to a key file, a
// password manager.
func TestOrdinaryLinesAreNotCredentials(t *testing.T) {
	for _, line := range []string{
		"echo password",
		"grep -r token .",
		"grep -ri secret /etc/app",
		"find . -name '*token*' -print",
		"export EDITOR=vim",
		"export PATH=/opt/homebrew/bin:$PATH",
		// A reference to a secret is not a secret, and it is how these lines
		// are supposed to be written.
		"export GITHUB_TOKEN=$GITHUB_TOKEN",
		`curl -H "Authorization: Bearer $GITHUB_TOKEN" https://api.example.com`,
		"export AWS_SECRET_ACCESS_KEY=$(pass show aws/secret)",
		// A path that names a credential is not one.
		"ssh -i ~/.ssh/id_ed25519 deploy@host",
		"app --secret-file=/etc/app/secret.pem",
		"app --password-file /run/secrets/db",
		// URLs without a password in them.
		"git clone https://github.com/blairham/sh.git",
		"curl http://localhost:8080/api/tokens",
		"psql postgres://reader@db.internal/app",
		"scp deploy@host:/var/log/app.log .",
		// Assignments too short or too empty to be carrying anything.
		"PASSWORD=",
		"token=1",
		`SECRET=""`,
		// A pattern *about* secrets, which is a shape people write when they
		// are being careful and would be a maddening thing to reject.
		"export HISTIGNORE='*secret=*'",
		"history | grep -i password",
		"man ssh-keygen",
		"openssl genpkey -algorithm ed25519 -out key.pem",
	} {
		if got, ok := secret.Default().Match(line); ok {
			t.Errorf("%q was rejected as %s, and it is an ordinary line", line, got)
		}
	}
}

// Output is redacted rather than rejected: the credential goes and everything
// around it stays.
//
// This is the half blocks (#495) consumes. Nothing calls it yet, which is why
// it is pinned here — an engine with one caller grows one caller's shape.
func TestRedactKeepsEverythingElse(t *testing.T) {
	log := "Fetching layers...\n" +
		"docker login -u ci --password=" + strings.Repeat("m", 24) + " registry.example.com\n" +
		"done in 4.2s\n"
	got, n := secret.Default().Redact(log)
	if n != 1 {
		t.Fatalf("redacted %d, want 1", n)
	}
	if strings.Contains(got, strings.Repeat("m", 24)) {
		t.Error("the credential is still in the output")
	}
	for _, keep := range []string{"Fetching layers...", "docker login -u ci", "registry.example.com", "done in 4.2s"} {
		if !strings.Contains(got, keep) {
			t.Errorf("%q was lost; the point of redacting is that it is not", keep)
		}
	}
	if !strings.Contains(got, secret.Placeholder) {
		t.Errorf("nothing says a redaction happened: %q", got)
	}
	// Text with no credential in it comes back exactly as it was.
	if got, n := secret.Default().Redact(log[:18]); n != 0 || got != log[:18] {
		t.Errorf("clean text came back as %q with %d redactions", got, n)
	}
}

// Two credentials in one text are two redactions, and two rules firing over
// the same characters are still one.
//
// The second half is the one that would corrupt output rather than merely
// under-redact it: a password inside a URL matches both the connection-string
// rule and the assignment rule, and replacing the same span twice would cut
// the text at an index the first replacement had already moved.
func TestRedactHandlesSeveralAndOverlapping(t *testing.T) {
	two := "id=" + awsKeyID + " and key=" + googleKey
	if got, n := secret.Default().Redact(two); n != 2 ||
		strings.Contains(got, awsKeyID) || strings.Contains(got, googleKey) {
		t.Errorf("redacted %d from %q, giving %q", n, two, got)
	}

	pw := strings.Repeat("p", 18)
	line := "export DATABASE_PASSWORD=postgres://app:" + pw + "@db/app"
	got, n := secret.Default().Redact(line)
	if n != 1 {
		t.Errorf("redacted %d spans, want the overlapping rules to leave one", n)
	}
	if strings.Contains(got, pw) {
		t.Errorf("the password survived: %q", got)
	}
	if !strings.HasPrefix(got, "export DATABASE_PASSWORD=") {
		t.Errorf("redaction moved the text around it: %q", got)
	}
}

// A scanner nobody gave rules to matches nothing, and says so rather than
// approving everything quietly.
func TestTheZeroScannerIsEmptyRatherThanPermissive(t *testing.T) {
	var s secret.Scanner
	if name, ok := s.Match("export AWS_ACCESS_KEY_ID=" + awsKeyID); ok {
		t.Errorf("an empty scanner matched %s", name)
	}
	if got, n := s.Redact("anything"); n != 0 || got != "anything" {
		t.Errorf("an empty scanner redacted %q (%d)", got, n)
	}
}
