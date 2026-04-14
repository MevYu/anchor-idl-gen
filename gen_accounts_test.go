package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAccounts_MatchesGolden(t *testing.T) {
	idl, err := LoadIDL(filepath.Join("testdata", "minimal.json"))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := GenerateAccounts(&buf, "counter", idl); err != nil {
		t.Fatal(err)
	}

	goldenPath := filepath.Join("testdata", "golden", "accounts.go")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %q: %v", goldenPath, err)
	}

	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("generated output differs from golden.\n\n--- got (%d bytes) ---\n%s\n--- want (%d bytes) ---\n%s",
			buf.Len(), buf.String(), len(want), want)
	}
}

func TestGenerateAccounts_RejectsWrongDiscriminatorLength(t *testing.T) {
	idl := &IDL{
		Accounts: []AccountDef{
			{Name: "Bad", Discriminator: []byte{0x01, 0x02}},
		},
	}
	var buf bytes.Buffer
	if err := GenerateAccounts(&buf, "p", idl); err == nil {
		t.Fatal("expected error for 2-byte discriminator")
	}
}

func TestGenerateAccounts_EmptyAccountsOK(t *testing.T) {
	idl := &IDL{Accounts: nil}
	var buf bytes.Buffer
	if err := GenerateAccounts(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	// With no accounts, no MatchAccount helper is emitted; only the
	// package declaration + header should be present.
	if !bytes.Contains(buf.Bytes(), []byte("package p")) {
		t.Errorf("missing package declaration: %s", buf.String())
	}
	if bytes.Contains(buf.Bytes(), []byte("MatchAccount")) {
		t.Errorf("MatchAccount should not be emitted for empty accounts")
	}
}
