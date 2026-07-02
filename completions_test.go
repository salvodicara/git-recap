package main

import (
	"strings"
	"testing"
)

func TestCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		s, err := completionScript(shell)
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		// Tokens are generated from the CLI's own tables — spot-check both ends.
		// fish declares long options as `-l frontmatter`, so match the bare name.
		for _, want := range []string{"standup", "last-30-days", "html", "frontmatter", "index"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s completion missing %q", shell, want)
			}
		}
	}
	if _, err := completionScript("powershell"); err == nil {
		t.Error("expected error for unsupported shell")
	}
}
