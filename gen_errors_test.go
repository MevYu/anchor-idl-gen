package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateErrors_MatchesGolden(t *testing.T) {
	idl, err := LoadIDL(filepath.Join("testdata", "minimal.json"))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := GenerateErrors(&buf, "counter", idl); err != nil {
		t.Fatal(err)
	}

	goldenPath := filepath.Join("testdata", "golden", "errors.go")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %q: %v", goldenPath, err)
	}

	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("errors output differs from golden.\n--- got (%d bytes) ---\n%s\n--- want (%d bytes) ---\n%s",
			buf.Len(), buf.String(), len(want), want)
	}
}

func TestGenerateErrors_EmptyIDL(t *testing.T) {
	// With no errors, the file still has a valid ProgramError type
	// and an ErrorByCode helper that always returns nil.
	idl := &IDL{}
	var buf bytes.Buffer
	if err := GenerateErrors(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "type ProgramError struct") {
		t.Errorf("missing ProgramError type: %s", got)
	}
	if !strings.Contains(got, "func ErrorByCode(") {
		t.Errorf("missing ErrorByCode helper: %s", got)
	}
	if strings.Contains(got, "Err") && !strings.Contains(got, "Error") {
		t.Errorf("unexpected sentinel emission for empty IDL")
	}
}
