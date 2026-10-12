package history

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestReaders(t *testing.T) {
	cases := []struct {
		name, shell, data string
		want              int
	}{
		{"zsh extended", "zsh", ": 1710000000:0;echo one\n: 1710000001:0;echo \\\n two\n", 2},
		{"zsh plain", "zsh", "echo one\necho two\n", 2},
		{"bash timestamps", "bash", "#1710000000\necho one\n#1710000001\necho two\n", 2},
		{"bash plain", "bash", "echo one\necho two\n", 2},
		{"fish", "fish", "- cmd: echo one\n  when: 1710000000\n- cmd: echo two\n  when: 1710000001\n", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history")
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			r, err := NewReader(tc.shell, path, zap.NewNop())
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.ReadHistory(0)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Fatalf("got %d entries, want %d: %#v", len(got), tc.want, got)
			}
		})
	}
}

func TestLimitAndNegative(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	_ = os.WriteFile(path, []byte("a\nb\nc\n"), 0600)
	r, _ := NewReader("bash", path, zap.NewNop())
	got, err := r.ReadHistory(2)
	if err != nil || len(got) != 2 || got[0].Command != "b" {
		t.Fatalf("limit result %#v, %v", got, err)
	}
	if _, err := r.ReadHistory(-1); err == nil {
		t.Fatal("negative limit accepted")
	}
}

func TestBashTimestampedMultiline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	data := "#1710000000\nprintf first\nprintf second\n#1710000001\necho done\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	r, _ := NewReader("bash", path, zap.NewNop())
	got, err := r.ReadHistory(0)
	if err != nil || len(got) != 2 || got[0].Command != "printf first\nprintf second" {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestEmptyAndMissingHistory(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := NewReader("bash", empty, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := r.ReadHistory(0)
	if err != nil || len(entries) != 0 {
		t.Fatalf("empty history: %#v, %v", entries, err)
	}
	missing, err := NewReader("zsh", filepath.Join(t.TempDir(), "missing"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := missing.ReadHistory(0); err == nil {
		t.Fatal("missing history file should fail")
	}
}

func TestDefaultHistoryPathsRespectShellEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HISTFILE", "~/history/custom-shell-history")
	for _, shell := range []string{"zsh", "bash"} {
		got, err := DefaultPath(shell)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, "history", "custom-shell-history")
		if got != want {
			t.Fatalf("%s path = %q, want %q", shell, got, want)
		}
	}
	dataHome := filepath.Join(home, "custom-data")
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("HISTFILE", "")
	got, err := DefaultPath("fish")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataHome, "fish", "fish_history")
	if got != want {
		t.Fatalf("fish path = %q, want %q", got, want)
	}
}

func TestCustomHistoryPathExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	r, err := NewReader("bash", "~/history.txt", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "history.txt"); r.path != want {
		t.Fatalf("path = %q, want %q", r.path, want)
	}
}
