package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateEvents_TypedStructs(t *testing.T) {
	idl := &IDL{
		Events: []EventDef{
			{
				Name:          "Incremented",
				Discriminator: []byte{0xaa, 0xbb, 0xcc, 0xdd, 0x11, 0x22, 0x33, 0x44},
				Fields: []FieldDef{
					{Name: "by", Type: TypeRef{Primitive: "pubkey"}},
					{Name: "amount", Type: TypeRef{Primitive: "u64"}},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := GenerateEvents(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "type Incremented struct") {
		t.Errorf("missing Incremented struct: %s", got)
	}
	if !strings.Contains(got, "By     solana.PublicKey") && !strings.Contains(got, "By solana.PublicKey") {
		t.Errorf("By field mismatch: %s", got)
	}
	if !strings.Contains(got, "Amount uint64") {
		t.Errorf("Amount field missing: %s", got)
	}
	if !strings.Contains(got, "IncrementedDiscriminator = [8]byte{0xaa, 0xbb, 0xcc, 0xdd, 0x11, 0x22, 0x33, 0x44}") {
		t.Errorf("IncrementedDiscriminator mismatch: %s", got)
	}
	if !strings.Contains(got, `import "github.com/cielu/solana-go"`) {
		t.Errorf("solana import missing for pubkey-bearing event: %s", got)
	}
}

func TestGenerateEvents_NoEvents(t *testing.T) {
	idl := &IDL{}
	var buf bytes.Buffer
	if err := GenerateEvents(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "package p") {
		t.Errorf("missing package declaration: %s", got)
	}
	if strings.Contains(got, "struct") {
		t.Errorf("unexpected struct emission for empty IDL: %s", got)
	}
}

func TestGenerateEvents_NoDiscriminator(t *testing.T) {
	// Events without a discriminator (older IDLs) should still
	// produce a usable struct; the *Discriminator var is simply
	// omitted.
	idl := &IDL{
		Events: []EventDef{
			{
				Name: "Legacy",
				Fields: []FieldDef{
					{Name: "x", Type: TypeRef{Primitive: "u64"}},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := GenerateEvents(&buf, "p", idl); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "type Legacy struct") {
		t.Errorf("missing Legacy struct: %s", got)
	}
	if strings.Contains(got, "LegacyDiscriminator") {
		t.Errorf("unexpected discriminator emission for legacy event: %s", got)
	}
}
