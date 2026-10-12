package llm

import (
	"strings"
	"testing"

	"github.com/jaded33/hintai/internal/history"
)

func TestVerifiedMatchesOnlyReturnsHistoryCommands(t *testing.T) {
	entries := []history.HistoryEntry{{Command: "git status"}, {Command: "docker ps"}}
	got := verifiedMatches("H2\nH9\nrm -rf /\nH1", entries)
	if got != "docker ps\ngit status" {
		t.Fatalf("got %q", got)
	}
}

func TestFindPromptIncludesAllEntries(t *testing.T) {
	entries := make([]history.HistoryEntry, 200)
	for i := range entries {
		entries[i].Command = "echo sample"
	}
	prompt := buildFindPromptText("find it", entries)
	if !strings.Contains(prompt, "[H200] echo sample") {
		t.Fatal("history was capped before it reached the prompt")
	}
}
