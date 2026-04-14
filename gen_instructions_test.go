package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateInstructions_MatchesGolden(t *testing.T) {
	idl, err := LoadIDL(filepath.Join("testdata", "minimal.json"))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := GenerateInstructions(&buf, "counter", idl); err != nil {
		t.Fatal(err)
	}

	goldenPath := filepath.Join("testdata", "golden", "instructions.go")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %q: %v", goldenPath, err)
	}

	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("instructions output differs from golden.\n--- got (%d bytes) ---\n%s\n--- want (%d bytes) ---\n%s",
			buf.Len(), buf.String(), len(want), want)
	}
}

func TestGenerateInstructions_DiscriminatorAndAccounts(t *testing.T) {
	idl := &IDL{
		Address: "Test1111111111111111111111111111111111111111",
		Instructions: []InstructionDef{
			{
				Name:          "initialize",
				Discriminator: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
				Accounts: []InstructionAcc{
					{Name: "state", Writable: true},
					{Name: "payer", Signer: true, Writable: true},
				},
				Args: []FieldDef{
					{Name: "value", Type: TypeRef{Primitive: "u64"}},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := GenerateInstructions(&buf, "myprog", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "InitializeDiscriminator = [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}") {
		t.Errorf("missing discriminator: %s", got)
	}
	if !strings.Contains(got, "type InitializeInstruction struct") {
		t.Errorf("missing builder struct: %s", got)
	}
	if !strings.Contains(got, "Value uint64") {
		t.Errorf("missing Value arg field: %s", got)
	}
	if !strings.Contains(got, "ProgramAddress") {
		t.Errorf("missing ProgramAddress var: %s", got)
	}
	if !strings.Contains(got, "func (ix *InitializeInstruction) ProgramID()") {
		t.Errorf("missing ProgramID method: %s", got)
	}
	if !strings.Contains(got, "func (ix *InitializeInstruction) Accounts()") {
		t.Errorf("missing Accounts method: %s", got)
	}
	if !strings.Contains(got, "func (ix *InitializeInstruction) Data()") {
		t.Errorf("missing Data method: %s", got)
	}
}

func TestGenerateInstructions_AccountMetaFlags(t *testing.T) {
	idl := &IDL{
		Instructions: []InstructionDef{
			{
				Name:          "transfer",
				Discriminator: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Accounts: []InstructionAcc{
					{Name: "from", Signer: true, Writable: true},
					{Name: "to", Writable: true},
					{Name: "program", Signer: false, Writable: false},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := GenerateInstructions(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "solana.NewAccountMeta(ix.From, true, true)") {
		t.Errorf("from: expected signer=true writable=true: %s", got)
	}
	if !strings.Contains(got, "solana.NewAccountMeta(ix.To, false, true)") {
		t.Errorf("to: expected signer=false writable=true: %s", got)
	}
	if !strings.Contains(got, "solana.NewAccountMeta(ix.Program, false, false)") {
		t.Errorf("program: expected signer=false writable=false: %s", got)
	}
}

func TestGenerateInstructions_BorshEncoding(t *testing.T) {
	idl := &IDL{
		Instructions: []InstructionDef{
			{
				Name:          "setVal",
				Discriminator: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Args: []FieldDef{
					{Name: "amount", Type: TypeRef{Primitive: "u64"}},
					{Name: "label", Type: TypeRef{Primitive: "string"}},
					{Name: "flag", Type: TypeRef{Primitive: "bool"}},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := GenerateInstructions(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "binary.LittleEndian.PutUint64") {
		t.Errorf("expected u64 LE encoding: %s", got)
	}
	if !strings.Contains(got, "buf.WriteString") {
		t.Errorf("expected string WriteString call: %s", got)
	}
	if !strings.Contains(got, "buf.WriteByte") {
		t.Errorf("expected bool WriteByte call: %s", got)
	}
}

func TestGenerateInstructions_NoInstructions(t *testing.T) {
	var buf bytes.Buffer
	if err := GenerateInstructions(&buf, "p", &IDL{}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "package p") {
		t.Errorf("missing package declaration: %s", got)
	}
}
