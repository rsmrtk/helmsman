// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentityOfShortensAssumedRole(t *testing.T) {
	id, scope := identityOf("aws", "arn:aws:sts::886312445846:assumed-role/AccountFullAccessRole/14d87478-90d1  region us-east-1")
	assert.Equal(t, "AccountFullAccessRole @ 886312445846", id)
	assert.Equal(t, "region us-east-1", scope)

	id, scope = identityOf("gcp", "me@example.com  project my-proj")
	assert.Equal(t, "me@example.com", id)
	assert.Equal(t, "project my-proj", scope)
}

func TestExplainKnownErrors(t *testing.T) {
	cases := []struct {
		prov, raw, project, region string
		reason, hintHas            string
	}{
		{"aws", "aws: [ERROR]: An error occurred (AccessDeniedException) when calling the ListClusters", "", "us-east-1", "access denied", "helmsman init aws --region <region>"},
		{"aws", "ExpiredToken: The security token included in the request is expired", "", "", "expired", "aws login"},
		{"gcp", "ERROR: Kubernetes Engine API has not been used in project p1 before or it is disabled", "p1", "", "Kubernetes Engine API is disabled in project p1", "gcloud services enable container.googleapis.com --project p1"},
		{"azure", "ERROR: Please run 'az login' to setup account.", "", "", "not signed in to Azure", "az login"},
		{"aws", "aws is not installed", "", "", "aws CLI is not installed", "brew install awscli"},
	}
	for _, c := range cases {
		reason, hint := explain(c.prov, c.raw, c.project)
		assert.Contains(t, reason, c.reason, c.raw)
		assert.Contains(t, hint, c.hintHas, c.raw)
	}
}

func TestExplainUnknownErrorIsShortenedNotLost(t *testing.T) {
	reason, hint := explain("aws", strings.Repeat("weird failure ", 40), "")
	assert.NotEmpty(t, reason)
	assert.LessOrEqual(t, len([]rune(reason)), 120)
	assert.Empty(t, hint)
}

func TestInitUIIsPlainTextOnANonTerminal(t *testing.T) {
	var b bytes.Buffer
	ui := newInitUI(&b, false) // a bytes.Buffer is not a terminal
	assert.False(t, ui.color)
	ui.header(false)
	ui.provider("AWS")
	ui.ok("account", "role @ 1")
	ui.fail("eks", "boom")
	ui.hint("fix it: run: aws login")
	ui.summary(0, 1, false, "orbstack")

	out := b.String()
	assert.NotContains(t, out, "\x1b", "logs and pipes must not receive ANSI codes")
	assert.Contains(t, out, "✓ account")
	assert.Contains(t, out, "✗ eks")
	assert.Contains(t, out, "↳ fix it:")
	assert.Contains(t, out, "\n        aws login\n", "the command sits on its own line")
	assert.Contains(t, out, "Nothing imported: 1 problem above. Fix it")
}

func TestInitUIHonoursNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	assert.False(t, newInitUI(&bytes.Buffer{}, false).color)
}

func TestInitUIPaintsWhenColorIsOn(t *testing.T) {
	ui := &initUI{w: &bytes.Buffer{}, color: true}
	assert.Contains(t, ui.paint(cGreen, "ok"), "\x1b[")
	assert.Empty(t, ui.paint(cGreen, ""), "nothing to color")
}

func TestSummaryVariants(t *testing.T) {
	for want, args := range map[string][3]any{
		"2 cluster(s) imported":    {2, 0, false},
		"Dry run finished":         {0, 0, true},
		"No clusters found":        {0, 0, false},
		"Nothing imported: 3 prob": {0, 3, false},
	} {
		var b bytes.Buffer
		newInitUI(&b, false).summary(args[0].(int), args[1].(int), args[2].(bool), "ctx")
		assert.Contains(t, b.String(), want)
	}
}

func TestSummaryWhenEverythingIsAlreadyThere(t *testing.T) {
	var b bytes.Buffer
	ui := newInitUI(&b, true)
	ui.present = 3
	ui.summary(0, 0, false, "orbstack")
	assert.Contains(t, b.String(), "Everything is already in your kubeconfig")
	assert.Contains(t, b.String(), "3 cluster(s) found, nothing to add")
	assert.NotContains(t, b.String(), "No clusters found", "that would be wrong: they were found")

	b.Reset()
	ui = newInitUI(&b, true)
	ui.summary(0, 0, false, "orbstack")
	assert.Contains(t, b.String(), "No clusters found")
}
