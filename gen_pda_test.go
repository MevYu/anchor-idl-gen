package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePDA_MatchesGolden(t *testing.T) {
	idl, err := LoadIDL(filepath.Join("testdata", "minimal.json"))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := GeneratePDA(&buf, "counter", idl); err != nil {
		t.Fatal(err)
	}

	goldenPath := filepath.Join("testdata", "golden", "pda.go")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %q: %v", goldenPath, err)
	}

	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("pda output differs from golden.\n--- got ---\n%s\n--- want ---\n%s",
			buf.String(), want)
	}
}

func TestGeneratePDA_NoSeeds(t *testing.T) {
	idl := &IDL{
		Instructions: []InstructionDef{
			{
				Name: "noop",
				Accounts: []InstructionAcc{
					{Name: "user", Signer: true},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := GeneratePDA(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "package p") {
		t.Errorf("missing package declaration: %s", got)
	}
	if strings.Contains(got, "Derive") {
		t.Errorf("unexpected Derive function for no-PDA IDL: %s", got)
	}
}

func TestGeneratePDA_WithConstStringSeed(t *testing.T) {
	idl := &IDL{
		Address: "Prog1111111111111111111111111111111111111111",
		Instructions: []InstructionDef{
			{
				Name: "init",
				Accounts: []InstructionAcc{
					{
						Name: "vault",
						PDA: &PDADef{
							Seeds: []PDASeed{
								{Kind: "const", Value: json.RawMessage(`"vault"`)},
							},
						},
					},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := GeneratePDA(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "DeriveInitVault") {
		t.Errorf("missing DeriveInitVault: %s", got)
	}
	if !strings.Contains(got, `[]byte("vault")`) {
		t.Errorf("missing string seed literal: %s", got)
	}
	if !strings.Contains(got, "FindProgramAddress") {
		t.Errorf("missing FindProgramAddress: %s", got)
	}
}

func TestGeneratePDA_WithConstBytesSeed(t *testing.T) {
	idl := &IDL{
		Address: "Prog1111111111111111111111111111111111111111",
		Instructions: []InstructionDef{
			{
				Name: "init",
				Accounts: []InstructionAcc{
					{
						Name: "vault",
						PDA: &PDADef{
							Seeds: []PDASeed{
								{Kind: "const", Value: json.RawMessage(`[1, 2, 3]`)},
							},
						},
					},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := GeneratePDA(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "{0x01, 0x02, 0x03}") {
		t.Errorf("missing byte-array seed: %s", got)
	}
}

func TestGeneratePDA_WithAccountSeed(t *testing.T) {
	idl := &IDL{
		Address: "Prog1111111111111111111111111111111111111111",
		Instructions: []InstructionDef{
			{
				Name: "init",
				Accounts: []InstructionAcc{
					{
						Name: "record",
						PDA: &PDADef{
							Seeds: []PDASeed{
								{Kind: "const", Value: json.RawMessage(`"rec"`)},
								{Kind: "account", Path: "user"},
							},
						},
					},
					{Name: "user", Signer: true},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := GeneratePDA(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "user solana.PublicKey") {
		t.Errorf("missing user parameter: %s", got)
	}
	if !strings.Contains(got, "user[:]") {
		t.Errorf("missing user seed: %s", got)
	}
}
