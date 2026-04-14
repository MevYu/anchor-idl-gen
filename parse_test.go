package main

import (
	"path/filepath"
	"testing"
)

func TestLoadIDL_Minimal(t *testing.T) {
	idl, err := LoadIDL(filepath.Join("testdata", "minimal.json"))
	if err != nil {
		t.Fatal(err)
	}
	if idl.Metadata.Name != "counter" {
		t.Errorf("metadata.name = %q", idl.Metadata.Name)
	}
	if len(idl.Instructions) != 1 {
		t.Fatalf("instructions len = %d", len(idl.Instructions))
	}
	ix := idl.Instructions[0]
	if ix.Name != "increment" {
		t.Errorf("instruction name = %q", ix.Name)
	}
	if len(ix.Discriminator) != 8 {
		t.Errorf("discriminator len = %d", len(ix.Discriminator))
	}
	if len(ix.Accounts) != 2 {
		t.Errorf("accounts len = %d", len(ix.Accounts))
	}
	if !ix.Accounts[0].Writable {
		t.Error("counter account should be writable")
	}
	if !ix.Accounts[1].Signer {
		t.Error("authority account should be signer")
	}
	if len(ix.Args) != 1 || ix.Args[0].Name != "amount" {
		t.Errorf("args mismatch: %+v", ix.Args)
	}

	if len(idl.Accounts) != 1 || idl.Accounts[0].Name != "Counter" {
		t.Errorf("accounts mismatch: %+v", idl.Accounts)
	}

	if len(idl.Types) != 2 {
		t.Fatalf("types len = %d", len(idl.Types))
	}
	counter := idl.Types[0]
	if counter.Name != "Counter" || counter.Type.Kind != "struct" {
		t.Errorf("counter type mismatch")
	}
	if len(counter.Type.Fields) != 7 {
		t.Fatalf("counter fields len = %d", len(counter.Type.Fields))
	}
	// Spot-check a few field shapes.
	if counter.Type.Fields[0].Type.Primitive != "u64" {
		t.Errorf("count type mismatch")
	}
	if counter.Type.Fields[1].Type.Primitive != "pubkey" {
		t.Errorf("authority type mismatch")
	}
	if counter.Type.Fields[2].Type.Vec == nil || counter.Type.Fields[2].Type.Vec.Primitive != "u64" {
		t.Errorf("history type mismatch")
	}
	if counter.Type.Fields[4].Type.Array == nil || counter.Type.Fields[4].Type.Array.Length != 32 {
		t.Errorf("label type mismatch")
	}
	if counter.Type.Fields[5].Type.Option == nil || counter.Type.Fields[5].Type.Option.Primitive != "u64" {
		t.Errorf("last type mismatch")
	}
	if counter.Type.Fields[6].Type.Defined != "Direction" {
		t.Errorf("direction type mismatch")
	}

	direction := idl.Types[1]
	if direction.Name != "Direction" || direction.Type.Kind != "enum" {
		t.Errorf("direction type mismatch")
	}
	if len(direction.Type.Variants) != 2 {
		t.Errorf("direction variants len = %d", len(direction.Type.Variants))
	}

	if len(idl.Errors) != 1 || idl.Errors[0].Code != 6000 {
		t.Errorf("errors mismatch")
	}
}

func TestLoadIDL_NotFound(t *testing.T) {
	if _, err := LoadIDL("does-not-exist.json"); err == nil {
		t.Fatal("expected error")
	}
}
