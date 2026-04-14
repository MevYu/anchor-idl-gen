package main

import (
	"encoding/json"
	"testing"
)

func TestTypeRef_Primitive(t *testing.T) {
	var tr TypeRef
	if err := json.Unmarshal([]byte(`"u64"`), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Primitive != "u64" {
		t.Errorf("Primitive = %q", tr.Primitive)
	}
}

func TestTypeRef_Vec(t *testing.T) {
	var tr TypeRef
	if err := json.Unmarshal([]byte(`{"vec": "u64"}`), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Vec == nil || tr.Vec.Primitive != "u64" {
		t.Errorf("Vec mismatch: %+v", tr)
	}
}

func TestTypeRef_Option(t *testing.T) {
	var tr TypeRef
	if err := json.Unmarshal([]byte(`{"option": "pubkey"}`), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Option == nil || tr.Option.Primitive != "pubkey" {
		t.Errorf("Option mismatch: %+v", tr)
	}
}

func TestTypeRef_Array(t *testing.T) {
	var tr TypeRef
	if err := json.Unmarshal([]byte(`{"array": ["u8", 32]}`), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Array == nil {
		t.Fatal("Array should not be nil")
	}
	if tr.Array.Element.Primitive != "u8" {
		t.Errorf("element = %q", tr.Array.Element.Primitive)
	}
	if tr.Array.Length != 32 {
		t.Errorf("length = %d", tr.Array.Length)
	}
}

func TestTypeRef_Defined_Anchor030(t *testing.T) {
	var tr TypeRef
	if err := json.Unmarshal([]byte(`{"defined": {"name": "MyStruct"}}`), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Defined != "MyStruct" {
		t.Errorf("Defined = %q", tr.Defined)
	}
}

func TestTypeRef_Defined_Anchor029Legacy(t *testing.T) {
	var tr TypeRef
	if err := json.Unmarshal([]byte(`{"defined": "MyStruct"}`), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Defined != "MyStruct" {
		t.Errorf("Defined = %q", tr.Defined)
	}
}

func TestTypeRef_NestedComposite(t *testing.T) {
	// vec<option<u64>> — nested composites should parse recursively.
	var tr TypeRef
	if err := json.Unmarshal([]byte(`{"vec": {"option": "u64"}}`), &tr); err != nil {
		t.Fatal(err)
	}
	if tr.Vec == nil || tr.Vec.Option == nil || tr.Vec.Option.Primitive != "u64" {
		t.Errorf("nested mismatch: %+v", tr)
	}
}

func TestDiscriminator_Unmarshal(t *testing.T) {
	var d Discriminator
	if err := json.Unmarshal([]byte(`[255, 176, 4, 245, 188, 253, 124, 25]`), &d); err != nil {
		t.Fatal(err)
	}
	if len(d) != 8 {
		t.Fatalf("len = %d", len(d))
	}
	if d[0] != 255 || d[7] != 25 {
		t.Errorf("mismatch: %v", d)
	}
}

func TestDiscriminator_OutOfRange(t *testing.T) {
	var d Discriminator
	if err := json.Unmarshal([]byte(`[256]`), &d); err == nil {
		t.Fatal("expected error for value > 255")
	}
	if err := json.Unmarshal([]byte(`[-1]`), &d); err == nil {
		t.Fatal("expected error for value < 0")
	}
}
