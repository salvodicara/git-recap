package main

import (
	"os"
	"strings"
	"testing"
)

// TestReadmeUsageInSync makes CLI/docs drift a test failure: the README's
// Usage block must contain the exact help text the binary prints. Changing a
// flag means updating `usage` AND README.md in the same commit.
func TestReadmeUsageInSync(t *testing.T) {
	b, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), usage) {
		t.Error("README.md Usage section is out of sync with the CLI help — paste the exact `usage` text from main.go into the README's Usage code block")
	}
}
