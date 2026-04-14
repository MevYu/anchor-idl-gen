package main

import (
	"bytes"
	"fmt"
)

// This file centralises the Borsh encode/decode fragment emitters
// used by instructions, accounts, and events. Each emitter writes
// Go statements that operate on a caller-supplied buffer variable:
//
//   - emitBorshWrite appends bytes to a *bytes.Buffer named bufVar
//   - emitBorshRead  pulls bytes from a *bytes.Reader named readerVar
//     and writes the decoded value into the Go expression val
//
// Both emitters consult typeDefs when they hit a {"defined": X}
// reference so nested structs and enums are expanded inline without
// runtime reflection.

// needsBinary reports whether the generated code references
// encoding/binary. Callers use it to decide whether to import the
// package.
func needsBinary(src string) bool {
	return bytes.Contains([]byte(src), []byte("binary.LittleEndian"))
}

// emitBorshWrite appends Go statements to buf that Borsh-encode val
// (a Go expression of the given IDL type) into the local *bytes.Buffer
// variable named bufVar. indent is prepended to every line.
func emitBorshWrite(buf *bytes.Buffer, bufVar, val string, t TypeRef, indent string, typeDefs map[string]TypeBody) error {
	switch {
	case t.Primitive != "":
		return emitBorshPrimitiveWrite(buf, bufVar, val, t.Primitive, indent)
	case t.Vec != nil:
		fmt.Fprintf(buf, "%s{\n", indent)
		fmt.Fprintf(buf, "%s\tvar _l [4]byte\n", indent)
		fmt.Fprintf(buf, "%s\tbinary.LittleEndian.PutUint32(_l[:], uint32(len(%s)))\n", indent, val)
		fmt.Fprintf(buf, "%s\t%s.Write(_l[:])\n", indent, bufVar)
		fmt.Fprintf(buf, "%s\tfor _, _elem := range %s {\n", indent, val)
		if err := emitBorshWrite(buf, bufVar, "_elem", *t.Vec, indent+"\t\t", typeDefs); err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s\t}\n", indent)
		fmt.Fprintf(buf, "%s}\n", indent)
	case t.Option != nil:
		fmt.Fprintf(buf, "%sif %s == nil {\n", indent, val)
		fmt.Fprintf(buf, "%s\t%s.WriteByte(0)\n", indent, bufVar)
		fmt.Fprintf(buf, "%s} else {\n", indent)
		fmt.Fprintf(buf, "%s\t%s.WriteByte(1)\n", indent, bufVar)
		if err := emitBorshWrite(buf, bufVar, "*"+val, *t.Option, indent+"\t", typeDefs); err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s}\n", indent)
	case t.Array != nil:
		fmt.Fprintf(buf, "%sfor _, _elem := range %s {\n", indent, val)
		if err := emitBorshWrite(buf, bufVar, "_elem", *t.Array.Element, indent+"\t", typeDefs); err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s}\n", indent)
	case t.Defined != "":
		td, ok := typeDefs[t.Defined]
		if !ok {
			return fmt.Errorf("undefined type %q", t.Defined)
		}
		switch td.Kind {
		case "struct":
			for _, f := range td.Fields {
				if err := emitBorshWrite(buf, bufVar, val+"."+goName(f.Name), f.Type, indent, typeDefs); err != nil {
					return fmt.Errorf("field %s of %s: %w", f.Name, t.Defined, err)
				}
			}
		case "enum":
			if enumIsUnit(td) {
				fmt.Fprintf(buf, "%s%s.WriteByte(uint8(%s))\n", indent, bufVar, val)
				return nil
			}
			// Data-carrying enum: encode tag then variant payload.
			fmt.Fprintf(buf, "%s%s.WriteByte(%s.EnumTag())\n", indent, bufVar, val)
			fmt.Fprintf(buf, "%sswitch _v := %s.(type) {\n", indent, val)
			for i, variant := range td.Variants {
				vname := goName(t.Defined) + "_" + goName(variant.Name)
				fmt.Fprintf(buf, "%scase *%s:\n", indent, vname)
				if len(variant.Fields) == 0 {
					fmt.Fprintf(buf, "%s\t_ = _v\n", indent)
				}
				for _, f := range variant.Fields {
					if err := emitBorshWrite(buf, bufVar, "_v."+goName(f.Name), f.Type, indent+"\t", typeDefs); err != nil {
						return fmt.Errorf("variant %s field %s: %w", variant.Name, f.Name, err)
					}
				}
				_ = i
			}
			fmt.Fprintf(buf, "%s}\n", indent)
		default:
			return fmt.Errorf("defined type %q has unsupported kind %q", t.Defined, td.Kind)
		}
	default:
		return fmt.Errorf("empty type reference")
	}
	return nil
}

// emitBorshPrimitiveWrite emits a single Borsh-encode statement for
// a primitive type.
func emitBorshPrimitiveWrite(buf *bytes.Buffer, bufVar, val, prim, indent string) error {
	switch prim {
	case "u8":
		fmt.Fprintf(buf, "%s%s.WriteByte(%s)\n", indent, bufVar, val)
	case "i8":
		fmt.Fprintf(buf, "%s%s.WriteByte(byte(%s))\n", indent, bufVar, val)
	case "u16":
		fmt.Fprintf(buf, "%s{\n%s\tvar _b [2]byte\n%s\tbinary.LittleEndian.PutUint16(_b[:], %s)\n%s\t%s.Write(_b[:])\n%s}\n",
			indent, indent, indent, val, indent, bufVar, indent)
	case "i16":
		fmt.Fprintf(buf, "%s{\n%s\tvar _b [2]byte\n%s\tbinary.LittleEndian.PutUint16(_b[:], uint16(%s))\n%s\t%s.Write(_b[:])\n%s}\n",
			indent, indent, indent, val, indent, bufVar, indent)
	case "u32":
		fmt.Fprintf(buf, "%s{\n%s\tvar _b [4]byte\n%s\tbinary.LittleEndian.PutUint32(_b[:], %s)\n%s\t%s.Write(_b[:])\n%s}\n",
			indent, indent, indent, val, indent, bufVar, indent)
	case "i32":
		fmt.Fprintf(buf, "%s{\n%s\tvar _b [4]byte\n%s\tbinary.LittleEndian.PutUint32(_b[:], uint32(%s))\n%s\t%s.Write(_b[:])\n%s}\n",
			indent, indent, indent, val, indent, bufVar, indent)
	case "u64":
		fmt.Fprintf(buf, "%s{\n%s\tvar _b [8]byte\n%s\tbinary.LittleEndian.PutUint64(_b[:], %s)\n%s\t%s.Write(_b[:])\n%s}\n",
			indent, indent, indent, val, indent, bufVar, indent)
	case "i64":
		fmt.Fprintf(buf, "%s{\n%s\tvar _b [8]byte\n%s\tbinary.LittleEndian.PutUint64(_b[:], uint64(%s))\n%s\t%s.Write(_b[:])\n%s}\n",
			indent, indent, indent, val, indent, bufVar, indent)
	case "u128", "i128":
		fmt.Fprintf(buf, "%s%s.Write(%s[:])\n", indent, bufVar, val)
	case "bool":
		fmt.Fprintf(buf, "%sif %s {\n%s\t%s.WriteByte(1)\n%s} else {\n%s\t%s.WriteByte(0)\n%s}\n",
			indent, val, indent, bufVar, indent, indent, bufVar, indent)
	case "string":
		fmt.Fprintf(buf, "%s{\n%s\tvar _l [4]byte\n%s\tbinary.LittleEndian.PutUint32(_l[:], uint32(len(%s)))\n%s\t%s.Write(_l[:])\n%s\t%s.WriteString(%s)\n%s}\n",
			indent, indent, indent, val, indent, bufVar, indent, bufVar, val, indent)
	case "bytes":
		fmt.Fprintf(buf, "%s{\n%s\tvar _l [4]byte\n%s\tbinary.LittleEndian.PutUint32(_l[:], uint32(len(%s)))\n%s\t%s.Write(_l[:])\n%s\t%s.Write(%s)\n%s}\n",
			indent, indent, indent, val, indent, bufVar, indent, bufVar, val, indent)
	case "pubkey", "publicKey":
		fmt.Fprintf(buf, "%s%s.Write(%s[:])\n", indent, bufVar, val)
	default:
		return fmt.Errorf("unknown primitive type %q", prim)
	}
	return nil
}

// emitBorshRead appends Go statements to buf that Borsh-decode the
// next value from the *bytes.Reader named readerVar into the Go
// lvalue val. indent is prepended to every line.
//
// Helper functions from the generated file (readU32, readString, …)
// are used so the output stays compact; those helpers are emitted by
// writeBorshReadHelpers.
func emitBorshRead(buf *bytes.Buffer, readerVar, val string, t TypeRef, indent string, typeDefs map[string]TypeBody) error {
	switch {
	case t.Primitive != "":
		return emitBorshPrimitiveRead(buf, readerVar, val, t.Primitive, indent)
	case t.Vec != nil:
		fmt.Fprintf(buf, "%s{\n", indent)
		fmt.Fprintf(buf, "%s\tn, err := readU32(%s)\n", indent, readerVar)
		fmt.Fprintf(buf, "%s\tif err != nil { return err }\n", indent)
		inner, err := goTypeFor(*t.Vec)
		if err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s\t%s = make([]%s, n)\n", indent, val, inner)
		fmt.Fprintf(buf, "%s\tfor i := uint32(0); i < n; i++ {\n", indent)
		if err := emitBorshRead(buf, readerVar, val+"[i]", *t.Vec, indent+"\t\t", typeDefs); err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s\t}\n", indent)
		fmt.Fprintf(buf, "%s}\n", indent)
	case t.Option != nil:
		inner, err := goTypeFor(*t.Option)
		if err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s{\n", indent)
		fmt.Fprintf(buf, "%s\ttag, err := %s.ReadByte()\n", indent, readerVar)
		fmt.Fprintf(buf, "%s\tif err != nil { return err }\n", indent)
		fmt.Fprintf(buf, "%s\tif tag == 1 {\n", indent)
		fmt.Fprintf(buf, "%s\t\tvar _tmp %s\n", indent, inner)
		if err := emitBorshRead(buf, readerVar, "_tmp", *t.Option, indent+"\t\t", typeDefs); err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s\t\t%s = &_tmp\n", indent, val)
		fmt.Fprintf(buf, "%s\t} else if tag != 0 {\n", indent)
		fmt.Fprintf(buf, "%s\t\treturn fmt.Errorf(\"borsh option: invalid tag %%d\", tag)\n", indent)
		fmt.Fprintf(buf, "%s\t}\n", indent)
		fmt.Fprintf(buf, "%s}\n", indent)
	case t.Array != nil:
		fmt.Fprintf(buf, "%sfor i := 0; i < %d; i++ {\n", indent, t.Array.Length)
		if err := emitBorshRead(buf, readerVar, val+"[i]", *t.Array.Element, indent+"\t", typeDefs); err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s}\n", indent)
	case t.Defined != "":
		td, ok := typeDefs[t.Defined]
		if !ok {
			return fmt.Errorf("undefined type %q", t.Defined)
		}
		switch td.Kind {
		case "struct":
			for _, f := range td.Fields {
				if err := emitBorshRead(buf, readerVar, val+"."+goName(f.Name), f.Type, indent, typeDefs); err != nil {
					return fmt.Errorf("field %s of %s: %w", f.Name, t.Defined, err)
				}
			}
		case "enum":
			if enumIsUnit(td) {
				fmt.Fprintf(buf, "%s{\n", indent)
				fmt.Fprintf(buf, "%s\tb, err := %s.ReadByte()\n", indent, readerVar)
				fmt.Fprintf(buf, "%s\tif err != nil { return err }\n", indent)
				fmt.Fprintf(buf, "%s\t%s = %s(b)\n", indent, val, goName(t.Defined))
				fmt.Fprintf(buf, "%s}\n", indent)
				return nil
			}
			// Data-carrying enum: dispatch on tag.
			fmt.Fprintf(buf, "%s{\n", indent)
			fmt.Fprintf(buf, "%s\ttag, err := %s.ReadByte()\n", indent, readerVar)
			fmt.Fprintf(buf, "%s\tif err != nil { return err }\n", indent)
			fmt.Fprintf(buf, "%s\tswitch tag {\n", indent)
			for i, variant := range td.Variants {
				vname := goName(t.Defined) + "_" + goName(variant.Name)
				fmt.Fprintf(buf, "%s\tcase %d:\n", indent, i)
				fmt.Fprintf(buf, "%s\t\t_v := &%s{}\n", indent, vname)
				for _, f := range variant.Fields {
					if err := emitBorshRead(buf, readerVar, "_v."+goName(f.Name), f.Type, indent+"\t\t", typeDefs); err != nil {
						return fmt.Errorf("variant %s field %s: %w", variant.Name, f.Name, err)
					}
				}
				fmt.Fprintf(buf, "%s\t\t%s = _v\n", indent, val)
			}
			fmt.Fprintf(buf, "%s\tdefault:\n", indent)
			fmt.Fprintf(buf, "%s\t\treturn fmt.Errorf(\"borsh enum %s: invalid tag %%d\", tag)\n", indent, t.Defined)
			fmt.Fprintf(buf, "%s\t}\n", indent)
			fmt.Fprintf(buf, "%s}\n", indent)
		default:
			return fmt.Errorf("defined type %q has unsupported kind %q", t.Defined, td.Kind)
		}
	default:
		return fmt.Errorf("empty type reference")
	}
	return nil
}

// emitBorshPrimitiveRead emits a single Borsh-decode statement for
// a primitive type.
func emitBorshPrimitiveRead(buf *bytes.Buffer, readerVar, val, prim, indent string) error {
	switch prim {
	case "u8":
		fmt.Fprintf(buf, "%s{\n%s\tb, err := %s.ReadByte()\n%s\tif err != nil { return err }\n%s\t%s = b\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "i8":
		fmt.Fprintf(buf, "%s{\n%s\tb, err := %s.ReadByte()\n%s\tif err != nil { return err }\n%s\t%s = int8(b)\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "u16":
		fmt.Fprintf(buf, "%s{\n%s\tv, err := readU16(%s)\n%s\tif err != nil { return err }\n%s\t%s = v\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "i16":
		fmt.Fprintf(buf, "%s{\n%s\tv, err := readU16(%s)\n%s\tif err != nil { return err }\n%s\t%s = int16(v)\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "u32":
		fmt.Fprintf(buf, "%s{\n%s\tv, err := readU32(%s)\n%s\tif err != nil { return err }\n%s\t%s = v\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "i32":
		fmt.Fprintf(buf, "%s{\n%s\tv, err := readU32(%s)\n%s\tif err != nil { return err }\n%s\t%s = int32(v)\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "u64":
		fmt.Fprintf(buf, "%s{\n%s\tv, err := readU64(%s)\n%s\tif err != nil { return err }\n%s\t%s = v\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "i64":
		fmt.Fprintf(buf, "%s{\n%s\tv, err := readU64(%s)\n%s\tif err != nil { return err }\n%s\t%s = int64(v)\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "u128", "i128":
		fmt.Fprintf(buf, "%sif _, err := io.ReadFull(%s, %s[:]); err != nil { return err }\n",
			indent, readerVar, val)
	case "bool":
		fmt.Fprintf(buf, "%s{\n%s\tb, err := %s.ReadByte()\n%s\tif err != nil { return err }\n%s\t%s = b != 0\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "string":
		fmt.Fprintf(buf, "%s{\n%s\ts, err := readString(%s)\n%s\tif err != nil { return err }\n%s\t%s = s\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "bytes":
		fmt.Fprintf(buf, "%s{\n%s\tb, err := readBytes(%s)\n%s\tif err != nil { return err }\n%s\t%s = b\n%s}\n",
			indent, indent, readerVar, indent, indent, val, indent)
	case "pubkey", "publicKey":
		fmt.Fprintf(buf, "%sif _, err := io.ReadFull(%s, %s[:]); err != nil { return err }\n",
			indent, readerVar, val)
	default:
		return fmt.Errorf("unknown primitive type %q", prim)
	}
	return nil
}

// writeBorshReadHelpers emits the small package-private helpers
// (readU16/readU32/readU64/readString/readBytes) that the read
// emitters call. They live in whichever generated file needs them
// first; see GenerateAccounts and GenerateEvents.
func writeBorshReadHelpers(w *bytes.Buffer) {
	w.WriteString(`// readU16 reads a little-endian uint16 from r.
func readU16(r *bytes.Reader) (uint16, error) {
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b[:]), nil
}

// readU32 reads a little-endian uint32 from r.
func readU32(r *bytes.Reader) (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}

// readU64 reads a little-endian uint64 from r.
func readU64(r *bytes.Reader) (uint64, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b[:]), nil
}

// readString reads a Borsh length-prefixed UTF-8 string from r.
func readString(r *bytes.Reader) (string, error) {
	b, err := readBytes(r)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// readBytes reads a Borsh length-prefixed byte slice from r.
func readBytes(r *bytes.Reader) ([]byte, error) {
	n, err := readU32(r)
	if err != nil {
		return nil, err
	}
	out := make([]byte, n)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}
`)
}

// enumIsUnit reports whether every variant of the enum is a unit
// variant (no fields). Unit enums map to a Go uint8 alias; others
// map to a sealed interface with one struct per variant.
func enumIsUnit(td TypeBody) bool {
	for _, v := range td.Variants {
		if len(v.Fields) > 0 {
			return false
		}
	}
	return true
}
