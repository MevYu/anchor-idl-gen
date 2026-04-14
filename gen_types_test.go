package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateTypes_MatchesGolden(t *testing.T) {
	idl, err := LoadIDL(filepath.Join("testdata", "minimal.json"))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := GenerateTypes(&buf, "counter", idl); err != nil {
		t.Fatal(err)
	}

	goldenPath := filepath.Join("testdata", "golden", "types.go")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %q: %v", goldenPath, err)
	}

	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("generated output differs from golden. To update:\n"+
			"  go test -run TestGenerateTypes_MatchesGolden -update ./cmd/anchoridl-gen/\n\n"+
			"--- got (%d bytes) ---\n%s\n--- want (%d bytes) ---\n%s",
			buf.Len(), buf.String(), len(want), want)
	}
}

func TestGoTypeFor_Primitives(t *testing.T) {
	cases := map[string]string{
		"u8":        "uint8",
		"u64":       "uint64",
		"i32":       "int32",
		"bool":      "bool",
		"string":    "string",
		"bytes":     "[]byte",
		"pubkey":    "solana.PublicKey",
		"publicKey": "solana.PublicKey",
		"u128":      "[16]byte",
	}
	for prim, want := range cases {
		got, err := goTypeFor(TypeRef{Primitive: prim})
		if err != nil {
			t.Errorf("%s: %v", prim, err)
			continue
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", prim, got, want)
		}
	}
}

func TestGoTypeFor_Composites(t *testing.T) {
	// vec<u64>
	vec := TypeRef{Vec: &TypeRef{Primitive: "u64"}}
	if got, _ := goTypeFor(vec); got != "[]uint64" {
		t.Errorf("vec<u64> = %q", got)
	}
	// option<pubkey>
	opt := TypeRef{Option: &TypeRef{Primitive: "pubkey"}}
	if got, _ := goTypeFor(opt); got != "*solana.PublicKey" {
		t.Errorf("option<pubkey> = %q", got)
	}
	// array<u8, 32>
	arr := TypeRef{Array: &ArrayType{Element: &TypeRef{Primitive: "u8"}, Length: 32}}
	if got, _ := goTypeFor(arr); got != "[32]uint8" {
		t.Errorf("array<u8,32> = %q", got)
	}
	// defined<Foo>
	def := TypeRef{Defined: "foo"}
	if got, _ := goTypeFor(def); got != "Foo" {
		t.Errorf("defined<foo> = %q", got)
	}
	// Nested: vec<option<u64>>
	nested := TypeRef{Vec: &TypeRef{Option: &TypeRef{Primitive: "u64"}}}
	if got, _ := goTypeFor(nested); got != "[]*uint64" {
		t.Errorf("vec<option<u64>> = %q", got)
	}
}

func TestGenerateTypes_DataCarryingEnum(t *testing.T) {
	idl := &IDL{
		Types: []TypeDef{
			{
				Name: "Choice",
				Type: TypeBody{
					Kind: "enum",
					Variants: []EnumVariant{
						{Name: "Nothing"},
						{Name: "Some", Fields: []FieldDef{{Name: "value", Type: TypeRef{Primitive: "u64"}}}},
					},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := GenerateTypes(&buf, "p", idl); err != nil {
		t.Fatalf("generate: %v", err)
	}
	got := buf.String()

	// Sealed interface + per-variant structs with EnumTag().
	for _, want := range []string{
		"type Choice interface",
		"isChoice()",
		"EnumTag() uint8",
		"type Choice_Nothing struct",
		"type Choice_Some struct",
		"Value uint64",
		") isChoice()",
		"EnumTag() uint8 { return 0 }",
		"EnumTag() uint8 { return 1 }",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
