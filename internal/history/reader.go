package history

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type Reader struct {
	path   string
	shell  string
	logger *zap.Logger
}

func DetectShell() string {
	switch strings.ToLower(filepath.Base(os.Getenv("SHELL"))) {
	case "bash":
		return "bash"
	case "fish":
		return "fish"
	default:
		return "zsh"
	}
}

func DefaultPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zsh_history"), nil
	case "bash":
		return filepath.Join(home, ".bash_history"), nil
	case "fish":
		return filepath.Join(home, ".local", "share", "fish", "fish_history"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q (choose zsh, bash, or fish)", shell)
	}
}

func NewReader(shell, path string, logger *zap.Logger) (*Reader, error) {
	if shell == "" {
		shell = DetectShell()
	}
	if shell != "zsh" && shell != "bash" && shell != "fish" {
		return nil, fmt.Errorf("unsupported shell %q (choose zsh, bash, or fish)", shell)
	}
	if path == "" {
		var err error
		path, err = DefaultPath(shell)
		if err != nil {
			return nil, err
		}
	}
	return &Reader{path: path, shell: shell, logger: logger}, nil
}

func (r *Reader) ReadHistory(limit int) ([]HistoryEntry, error) {
	if limit < 0 {
		return nil, fmt.Errorf("history limit cannot be negative")
	}
	f, err := os.Open(r.path)
	if err != nil {
		return nil, fmt.Errorf("open %s history %q: %w", r.shell, r.path, err)
	}
	defer f.Close()

	var entries []HistoryEntry
	switch r.shell {
	case "zsh":
		entries, err = parseZsh(f)
	case "bash":
		entries, err = parseBash(f)
	case "fish":
		entries, err = parseFish(f)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s history: %w", r.shell, err)
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}

var zshPrefix = regexp.MustCompile("^: ([0-9]+):[0-9]+;(.*)$")

func scanLines(f *os.File, fn func(string)) error {
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for s.Scan() {
		fn(strings.ToValidUTF8(s.Text(), "�"))
	}
	return s.Err()
}

func parseZsh(f *os.File) ([]HistoryEntry, error) {
	var out []HistoryEntry
	var cmd strings.Builder
	var ts int64
	extended := false
	flush := func() {
		if cmd.Len() > 0 {
			out = append(out, HistoryEntry{Timestamp: ts, Command: strings.TrimSpace(cmd.String())})
			cmd.Reset()
			ts = 0
		}
	}
	err := scanLines(f, func(line string) {
		if m := zshPrefix.FindStringSubmatch(line); m != nil {
			flush()
			extended = true
			ts, _ = strconv.ParseInt(m[1], 10, 64)
			cmd.WriteString(m[2])
			return
		}
		if extended && cmd.Len() > 0 && !strings.HasPrefix(line, ": ") {
			appendContinuation(&cmd, line)
			return
		}
		flush()
		extended = false
		if line != "" {
			cmd.WriteString(line)
		}
	})
	flush()
	return out, err
}

func appendContinuation(b *strings.Builder, line string) {
	value := b.String()
	if strings.HasSuffix(value, "\\") {
		b.Reset()
		b.WriteString(strings.TrimSuffix(value, "\\"))
		b.WriteString(line)
		return
	}
	b.WriteByte('\n')
	b.WriteString(line)
}

func parseBash(f *os.File) ([]HistoryEntry, error) {
	var out []HistoryEntry
	var cmd strings.Builder
	var ts int64
	timestamped := false
	flush := func() {
		if cmd.Len() > 0 {
			out = append(out, HistoryEntry{Timestamp: ts, Command: strings.TrimSpace(cmd.String())})
			cmd.Reset()
			ts = 0
		}
	}
	err := scanLines(f, func(line string) {
		if strings.HasPrefix(line, "#") && len(line) > 1 {
			if n, e := strconv.ParseInt(line[1:], 10, 64); e == nil {
				flush()
				timestamped = true
				ts = n
				return
			}
		}
		if cmd.Len() > 0 && (timestamped || strings.HasSuffix(cmd.String(), "\\")) {
			appendContinuation(&cmd, line)
			return
		}
		flush()
		cmd.WriteString(line)
	})
	flush()
	return out, err
}

func parseFish(f *os.File) ([]HistoryEntry, error) {
	var out []HistoryEntry
	var block strings.Builder
	var blocks []string
	err := scanLines(f, func(line string) {
		if strings.HasPrefix(line, "- cmd: ") {
			if block.Len() > 0 {
				blocks = append(blocks, block.String())
				block.Reset()
			}
			block.WriteString(line)
		} else if block.Len() > 0 {
			block.WriteByte('\n')
			block.WriteString(line)
		}
	})
	if err != nil {
		return nil, err
	}
	if block.Len() > 0 {
		blocks = append(blocks, block.String())
	}
	for _, b := range blocks {
		var item struct {
			Cmd  string `yaml:"cmd"`
			When int64  `yaml:"when"`
		}
		yamlText := strings.ReplaceAll(strings.TrimPrefix(b, "- "), "  when:", "when:")
		if err := yaml.Unmarshal([]byte(yamlText), &item); err != nil {
			return nil, fmt.Errorf("invalid Fish entry: %w", err)
		}
		if item.Cmd != "" {
			out = append(out, HistoryEntry{Timestamp: item.When, Command: item.Cmd})
		}
	}
	return out, nil
}
