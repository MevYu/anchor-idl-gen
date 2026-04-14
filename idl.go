package main

import (
	"encoding/json"
	"fmt"
)

// IDL is the top-level Anchor IDL structure. It matches the
// Anchor 0.30+ JSON shape with legacy 0.29 fields tolerated where
// they differ harmlessly.
type IDL struct {
	Address      string           `json:"address,omitempty"`
	Metadata     IDLMetadata      `json:"metadata"`
	Name         string           `json:"name,omitempty"`
	Version      string           `json:"version,omitempty"`
	Instructions []InstructionDef `json:"instructions"`
	Accounts     []AccountDef     `json:"accounts"`
	Types        []TypeDef        `json:"types"`
	Events       []EventDef       `json:"events,omitempty"`
	Errors       []ErrorDef       `json:"errors,omitempty"`
}

// IDLMetadata is the Anchor 0.30+ metadata block. Older IDLs
// leave it empty; the parser falls back to the top-level Name
// and Version fields in that case.
type IDLMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Spec    string `json:"spec"`
}

// Discriminator is an Anchor 8-byte instruction or account
// discriminator. It is encoded in the IDL as a JSON array of
// integers (not a base64 string), so we need a custom UnmarshalJSON
// that rejects the byte-slice default behaviour.
type Discriminator []byte

// UnmarshalJSON implements json.Unmarshaler. It accepts a JSON
// array of integers in the range 0..255 and copies them into the
// byte slice.
func (d *Discriminator) UnmarshalJSON(data []byte) error {
	var ints []int
	if err := json.Unmarshal(data, &ints); err != nil {
		return fmt.Errorf("discriminator: %w", err)
	}
	out := make([]byte, len(ints))
	for i, v := range ints {
		if v < 0 || v > 255 {
			return fmt.Errorf("discriminator: byte %d out of range: %d", i, v)
		}
		out[i] = byte(v)
	}
	*d = out
	return nil
}

// InstructionDef describes a single program instruction in the
// IDL: name, 8-byte discriminator, account inputs, and typed
// arguments.
type InstructionDef struct {
	Name          string            `json:"name"`
	Discriminator Discriminator     `json:"discriminator"`
	Accounts      []InstructionAcc  `json:"accounts"`
	Args          []FieldDef        `json:"args"`
	Docs          []string          `json:"docs,omitempty"`
}

// InstructionAcc describes a single account input of an instruction.
type InstructionAcc struct {
	Name     string  `json:"name"`
	Writable bool    `json:"writable,omitempty"`
	Signer   bool    `json:"signer,omitempty"`
	Optional bool    `json:"optional,omitempty"`
	Address  string  `json:"address,omitempty"`
	PDA      *PDADef `json:"pda,omitempty"`
	Docs     []string `json:"docs,omitempty"`
}

// PDADef describes a program-derived address that the instruction
// requires the caller to pass. Seeds may be constants, account
// references, or instruction-arg references.
type PDADef struct {
	Seeds   []PDASeed `json:"seeds"`
	Program *PDASeed  `json:"program,omitempty"`
}

// PDASeed is a single seed in a PDA derivation.
type PDASeed struct {
	Kind  string          `json:"kind"` // "const" | "account" | "arg"
	Value json.RawMessage `json:"value,omitempty"`
	Path  string          `json:"path,omitempty"`
	Type  *TypeRef        `json:"type,omitempty"`
}

// AccountDef describes an on-chain account type. The full struct
// layout lives in the IDL's Types list under the same Name; the
// Accounts list only carries the discriminator so callers can
// match raw account bytes to a type.
type AccountDef struct {
	Name          string        `json:"name"`
	Discriminator Discriminator `json:"discriminator"`
	Docs          []string      `json:"docs,omitempty"`
}

// TypeDef describes a user-defined struct or enum type.
type TypeDef struct {
	Name string   `json:"name"`
	Type TypeBody `json:"type"`
	Docs []string `json:"docs,omitempty"`
}

// TypeBody is the inner shape of a defined type: struct fields,
// enum variants, or a type alias.
type TypeBody struct {
	Kind     string        `json:"kind"` // "struct" | "enum" | "type"
	Fields   []FieldDef    `json:"fields,omitempty"`
	Variants []EnumVariant `json:"variants,omitempty"`
	Alias    *TypeRef      `json:"alias,omitempty"`
}

// FieldDef is a single named field of a struct (or a typed
// instruction argument).
type FieldDef struct {
	Name string  `json:"name"`
	Type TypeRef `json:"type"`
	Docs []string `json:"docs,omitempty"`
}

// EnumVariant is a single variant of an enum type. Unit-only
// variants have an empty Fields slice; data-carrying variants
// use the same FieldDef list as structs.
type EnumVariant struct {
	Name   string     `json:"name"`
	Fields []FieldDef `json:"fields,omitempty"`
}

// EventDef describes an Anchor program event.
type EventDef struct {
	Name          string        `json:"name"`
	Discriminator Discriminator `json:"discriminator,omitempty"`
	Fields        []FieldDef    `json:"fields,omitempty"`
	Docs          []string      `json:"docs,omitempty"`
}

// ErrorDef is a single Anchor program error code.
type ErrorDef struct {
	Code int    `json:"code"`
	Name string `json:"name"`
	Msg  string `json:"msg"`
}

// TypeRef is a polymorphic type reference in the IDL: either a
// primitive name ("u64", "pubkey", ...), a composite wrapper
// ({"vec": <inner>}, {"option": <inner>}, {"array": [<inner>, N]}),
// or a reference to another defined type ({"defined": {"name": ...}}
// in 0.30+, {"defined": "..."} in 0.29).
type TypeRef struct {
	// Exactly one of these is set.
	Primitive string     // e.g. "u64", "pubkey", "bool", "string", "bytes"
	Vec       *TypeRef   // {"vec": <inner>}
	Option    *TypeRef   // {"option": <inner>}
	Array     *ArrayType // {"array": [<inner>, <length>]}
	Defined   string     // {"defined": ...}
}

// ArrayType is the shape of a fixed-size array type.
type ArrayType struct {
	Element *TypeRef
	Length  uint32
}

// UnmarshalJSON implements json.Unmarshaler. The IDL encodes a
// type reference as either a string (for primitives) or an object
// with a single discriminator key; this method handles both.
func (t *TypeRef) UnmarshalJSON(data []byte) error {
	// String form: primitive.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		t.Primitive = s
		return nil
	}
	// Object form: exactly one of vec / option / array / defined.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("type: expected string or object, got %s", data)
	}
	if len(obj) == 0 {
		return fmt.Errorf("type: empty object %s", data)
	}
	for key, val := range obj {
		switch key {
		case "vec":
			inner := &TypeRef{}
			if err := json.Unmarshal(val, inner); err != nil {
				return fmt.Errorf("type: vec: %w", err)
			}
			t.Vec = inner
		case "option":
			inner := &TypeRef{}
			if err := json.Unmarshal(val, inner); err != nil {
				return fmt.Errorf("type: option: %w", err)
			}
			t.Option = inner
		case "array":
			var tuple [2]json.RawMessage
			if err := json.Unmarshal(val, &tuple); err != nil {
				return fmt.Errorf("type: array: %w", err)
			}
			inner := &TypeRef{}
			if err := json.Unmarshal(tuple[0], inner); err != nil {
				return fmt.Errorf("type: array element: %w", err)
			}
			var length uint32
			if err := json.Unmarshal(tuple[1], &length); err != nil {
				return fmt.Errorf("type: array length: %w", err)
			}
			t.Array = &ArrayType{Element: inner, Length: length}
		case "defined":
			// Anchor 0.30+: {"defined": {"name": "Foo"}}
			// Anchor 0.29-: {"defined": "Foo"}
			var named struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(val, &named); err == nil && named.Name != "" {
				t.Defined = named.Name
				continue
			}
			var legacy string
			if err := json.Unmarshal(val, &legacy); err == nil {
				t.Defined = legacy
				continue
			}
			return fmt.Errorf("type: defined: expected string or {name}, got %s", val)
		}
	}
	return nil
}
