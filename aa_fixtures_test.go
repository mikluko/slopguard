package main

import (
	"testing"

	"github.com/mikluko/slopguard/internal/model"
)

// The one fixture the tests of the command itself need. The prefix sorts it
// above them, which is for the reader rather than the compiler: a file of
// fixtures asserts nothing.

// skipWithoutRuntime skips a test that needs the model, for either reason it can
// be absent: no library to load, or the semantic pass switched off.
//
// Every rule here has to hold without the model — that is a supported way to run
// this tool rather than a degraded one — so a test that needs it skips rather
// than fails.
func skipWithoutRuntime(t *testing.T) {
	t.Helper()
	if why := model.Absent(); why != "" {
		t.Skip(why)
	}
}
