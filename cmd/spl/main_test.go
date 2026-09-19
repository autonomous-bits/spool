package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestNewLoggerWritesStructuredJSON(t *testing.T) {
	var output bytes.Buffer

	newLogger(&output).Error("command failed", "error", errors.New("invalid input"))

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode log entry: %v", err)
	}
	if entry["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", entry["level"])
	}
	if entry["msg"] != "command failed" {
		t.Errorf("msg = %v, want command failed", entry["msg"])
	}
	if entry["component"] != "spl" {
		t.Errorf("component = %v, want spl", entry["component"])
	}
	if entry["error"] != "invalid input" {
		t.Errorf("error = %v, want invalid input", entry["error"])
	}
}

func TestBootstrapRootCommand(t *testing.T) {
	var output bytes.Buffer
	command, closeFn := bootstrapRootCommand(&output)
	if command == nil {
		t.Fatal("root command is nil")
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
