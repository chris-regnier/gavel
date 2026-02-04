package input

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHandler_ReadFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "pkg", "foo.go"), []byte("package pkg\n\nfunc Foo() {}\n"), 0644)

	h := NewHandler()
	artifacts, err := h.ReadFiles([]string{
		filepath.Join(dir, "main.go"),
		filepath.Join(dir, "pkg", "foo.go"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}
	if artifacts[0].Path != filepath.Join(dir, "main.go") {
		t.Errorf("unexpected path: %s", artifacts[0].Path)
	}
	if artifacts[0].Content != "package main\n\nfunc main() {}\n" {
		t.Errorf("unexpected content: %q", artifacts[0].Content)
	}
}

func TestHandler_ReadDiff(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\nindex 1234567..abcdefg 100644\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,5 @@\n package main\n\n-func main() {}\n+func main() {\n+\tfmt.Println(\"hello\")\n+}\n"

	h := NewHandler()
	artifacts, err := h.ReadDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Path != "main.go" {
		t.Errorf("expected path 'main.go', got %q", artifacts[0].Path)
	}
	if artifacts[0].Kind != KindDiff {
		t.Errorf("expected kind Diff, got %v", artifacts[0].Kind)
	}
}

func TestHandler_ReadDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b\n"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Hi\n"), 0644)

	h := NewHandler()
	artifacts, err := h.ReadDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) < 2 {
		t.Errorf("expected at least 2 artifacts, got %d", len(artifacts))
	}
}
