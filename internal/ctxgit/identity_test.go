package ctxgit

import "testing"

func TestNamespaceID(t *testing.T) {
	t.Parallel()
	if got := NamespaceID("spool", "idea-1"); got != "spool/idea-1" {
		t.Fatalf("NamespaceID = %q", got)
	}
	if got := NamespaceID("spool", "other/idea-1"); got != "other/idea-1" {
		t.Fatalf("already namespaced = %q", got)
	}
}

func TestFileNameFlatAndSafe(t *testing.T) {
	t.Parallel()
	name, err := FileName("spool/idea-1")
	if err != nil {
		t.Fatalf("FileName: %v", err)
	}
	if name != "spool--idea-1.json" {
		t.Fatalf("FileName = %q", name)
	}
	if _, err := FileName("../escape"); err == nil {
		t.Fatal("expected error for path traversal id")
	}
}

func TestShouldUseLFS(t *testing.T) {
	t.Parallel()
	if shouldUseLFS("notes.json", DefaultLFSThreshold+10, DefaultLFSThreshold) {
		t.Fatal("JSON must stay in plain git")
	}
	if !shouldUseLFS("photo.bin", DefaultLFSThreshold, DefaultLFSThreshold) {
		t.Fatal("binary at threshold should use LFS")
	}
	if !shouldWarnLargeText("schema.toml", LargeTextWarnBytes+1) {
		t.Fatal("large TOML should warn")
	}
}
