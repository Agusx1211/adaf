package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTextFlag_EmptyFilePathReturnsFlagValue(t *testing.T) {
	got, err := resolveTextFlag("inline value", "")
	if err != nil {
		t.Fatalf("resolveTextFlag() error = %v", err)
	}
	if got != "inline value" {
		t.Fatalf("resolveTextFlag() = %q, want %q", got, "inline value")
	}
}

func TestResolveTextFlag_FilePathReadsFileContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "description.md")
	if err := os.WriteFile(path, []byte("from file\nline 2"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := resolveTextFlag("inline value", path)
	if err != nil {
		t.Fatalf("resolveTextFlag() error = %v", err)
	}
	if got != "from file\nline 2" {
		t.Fatalf("resolveTextFlag() = %q, want %q", got, "from file\nline 2")
	}
}

func TestResolveTextFlag_DashReadsStdin(t *testing.T) {
	origStdin := os.Stdin
	t.Cleanup(func() {
		os.Stdin = origStdin
	})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})

	if _, err := w.WriteString("stdin content"); err != nil {
		t.Fatalf("writing stdin pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing stdin write pipe: %v", err)
	}

	os.Stdin = r
	got, err := resolveTextFlag("inline value", "-")
	if err != nil {
		t.Fatalf("resolveTextFlag() error = %v", err)
	}
	if got != "stdin content" {
		t.Fatalf("resolveTextFlag() = %q, want %q", got, "stdin content")
	}
}

func TestResolveTextFlag_FileTakesPrecedenceOverInlineValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "description.md")
	if err := os.WriteFile(path, []byte("file value"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := resolveTextFlag("inline value", path)
	if err != nil {
		t.Fatalf("resolveTextFlag() error = %v", err)
	}
	if got != "file value" {
		t.Fatalf("resolveTextFlag() = %q, want %q", got, "file value")
	}
}

func TestResolveTextFlag_MissingFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.md")

	_, err := resolveTextFlag("inline value", path)
	if err == nil {
		t.Fatal("resolveTextFlag() error = nil, want non-nil")
	}
}

func TestResolveTextFlag_EmptyFileReturnsEmptyString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := resolveTextFlag("inline value", path)
	if err != nil {
		t.Fatalf("resolveTextFlag() error = %v", err)
	}
	if got != "" {
		t.Fatalf("resolveTextFlag() = %q, want empty string", got)
	}
}

func TestOpenStoreWalksUpToProjectRoot(t *testing.T) {
	projectDir := t.TempDir()
	projectMetaDir := filepath.Join(projectDir, ".adaf")
	if err := os.MkdirAll(projectMetaDir, 0o755); err != nil {
		t.Fatalf("creating project meta dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectMetaDir, "project.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatalf("writing project.json: %v", err)
	}

	nested := filepath.Join(projectDir, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("creating nested directory: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	if err := os.Chdir(nested); err != nil {
		t.Fatalf("changing cwd: %v", err)
	}
	t.Setenv("ADAF_PROJECT_DIR", "")

	s, err := openStore()
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}

	wantRoot := filepath.Join(projectDir, ".adaf")
	if s.Root() != wantRoot {
		t.Fatalf("openStore() root = %q, want %q", s.Root(), wantRoot)
	}
}

func TestOpenStoreFallsBackToCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("changing cwd: %v", err)
	}
	t.Setenv("ADAF_PROJECT_DIR", "")

	s, err := openStore()
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}

	wantRoot := filepath.Join(dir, ".adaf")
	if s.Root() != wantRoot {
		t.Fatalf("openStore() root = %q, want %q", s.Root(), wantRoot)
	}
}
