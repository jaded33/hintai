package llm

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/jaded33/hintai/internal/history"
)

// buildFindPromptText constructs the prompt for finding history entries.
func buildFindPromptText(query string, historyContext []history.HistoryEntry) string {
	if len(historyContext) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("You are an expert shell history analyzer.\n")
	b.WriteString("The user is searching their shell history for commands based on a description.\n")
	b.WriteString(fmt.Sprintf("User's search query: \"%s\"\n\n", query))
	b.WriteString("Return ONLY the IDs of matching entries, one per line, such as H2. Never invent an ID.\n")
	b.WriteString("If NO entries match, return exactly: 'No relevant commands found.'\n\n")
	b.WriteString(formatHistoryContext("Recent Shell History Context (identify matches by H ID)", historyContext))
	b.WriteString("Matching command(s):\n")
	return b.String()
}

// buildSuggestPromptText constructs the prompt for suggesting commands.
func buildSuggestPromptText(taskDescription, shell string, historyContext []history.HistoryEntry) string {
	if shell == "" {
		shell = history.DetectShell()
	}
	var b strings.Builder

	osHint := "macOS (BSD userland)"
	if runtime.GOOS == "linux" {
		osHint = "Linux (GNU userland)"
	}

	b.WriteString(fmt.Sprintf("You are an AI assistant expert in generating safe commands for %s using %s syntax.\n", osHint, shell))
	b.WriteString(fmt.Sprintf("The user runs the %s shell. Use syntax and built-ins compatible with %s.\n", shell, shell))
	if runtime.GOOS == "darwin" {
		b.WriteString("IMPORTANT: This is macOS. Use BSD-compatible commands. 'find' does not support -printf or -println — use -print instead.\n")
	}
	b.WriteString(fmt.Sprintf("Task: \"%s\"\n\n", taskDescription))
	b.WriteString(formatHistoryContext("Recent Shell History Context", historyContext))
	b.WriteString("Provide multiple alternative commands if they exist, one per line. No explanations unless adding a # warning comment.\n")
	b.WriteString("If the task cannot be safely achieved, return exactly: 'Cannot suggest a command for this task.'\n\n")
	b.WriteString("Suggested Command(s):\n")
	return b.String()
}

func formatHistoryContext(header string, entries []history.HistoryEntry) string {
	if len(entries) == 0 {
		return "No specific user history context provided.\n\n"
	}
	var b strings.Builder
	b.WriteString(header + ":\n")
	for i, e := range entries {
		fmt.Fprintf(&b, "[H%d] %s\n", i+1, e.Command)
	}
	return b.String()
}

func verifiedMatches(response string, entries []history.HistoryEntry) string {
	seen := map[int]bool{}
	var out []string
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "* -")
		line = strings.Trim(line, "`")
		if len(line) > 1 && (line[0] == 'H' || line[0] == 'h') {
			id := strings.TrimLeft(line[1:], "Hh#):. ")
			n, err := strconv.Atoi(id)
			if err == nil && n > 0 && n <= len(entries) && !seen[n] {
				seen[n] = true
				out = append(out, entries[n-1].Command)
			}
		}
	}
	return strings.Join(out, "\n")
}
