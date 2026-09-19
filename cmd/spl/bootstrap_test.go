package main

import (
	"bytes"
	"testing"
)

func TestBootstrapRootCommandExposesKeepSurface(t *testing.T) {
	command, closeFn := bootstrapRootCommand(&bytes.Buffer{})
	t.Cleanup(func() { _ = closeFn() })
	if _, _, err := command.Find([]string{"query-context"}); err != nil {
		t.Fatalf("find query-context: %v", err)
	}
	if _, _, err := command.Find([]string{"init"}); err == nil {
		t.Fatal("top-level init must be removed")
	}
}
