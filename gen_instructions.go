package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"strings"
)

// GenerateInstructions renders the package's instructions.go file
// content into w. For each entry in idl.Instructions it emits:
//
//   - an 8-byte discriminator variable
//   - a typed builder struct with one field per account and per arg
//   - ProgramID / Accounts / Data methods implementing solana.Instruction
//
// The generated Data() method uses Borsh encoding (little-endian
// integers, 4-byte length-prefixed strings and vecs, option tags).
// Defined-type arguments are encoded by recursively expanding their
// fields from idl.Types; if a referenced type is not found the
// generator returns an error.
//
// If the IDL carries a program address, a ProgramAddress package var
// is emitted so callers can reference it without hard-coding the key.
func GenerateInstructions(w io.Writer, pkgName string, idl *IDL) error {
	typeDefs := make(map[string]TypeBody, len(idl.Types))
	for _, td := range idl.Types {
		typeDefs[td.Name] = td.Type
	}

	var body bytes.Buffer

	if idl.Address != "" {
		fmt.Fprintf(&body, "// ProgramAddress is the on-chain address of the %s program\n", pkgName)
		fmt.Fprintf(&body, "// as recorded in the IDL.\n")
		fmt.Fprintf(&body, "var ProgramAddress = solana.MustPublicKeyFromBase58(%q)\n\n", idl.Address)
	}

	for _, ix := range idl.Instructions {
		name := goName(ix.Name)

		// Discriminator variable.
		fmt.Fprintf(&body, "// %sDiscriminator is the 8-byte Anchor discriminator for the\n", name)
		fmt.Fprintf(&body, "// %s instruction.\n", name)
		fmt.Fprintf(&body, "var %sDiscriminator = [8]byte{", name)
		for i, b := range ix.Discriminator {
			if i > 0 {
				body.WriteString(", ")
			}
			fmt.Fprintf(&body, "0x%02x", b)
		}
		body.WriteString("}\n\n")

		// Builder struct: one field per account, then one per arg.
		fmt.Fprintf(&body, "// %sInstruction builds the %s instruction.\n", name, name)
		fmt.Fprintf(&body, "type %sInstruction struct {\n", name)
		for _, acc := range ix.Accounts {
			var tag string
			switch {
			case acc.Signer && acc.Writable:
				tag = " // signer, writable"
			case acc.Signer:
				tag = " // signer"
			case acc.Writable:
				tag = " // writable"
			}
			fmt.Fprintf(&body, "\t%s solana.PublicKey%s\n", goName(acc.Name), tag)
		}
		for _, arg := range ix.Args {
			gt, err := goTypeFor(arg.Type)
			if err != nil {
				return fmt.Errorf("instruction %s arg %s: %w", ix.Name, arg.Name, err)
			}
			fmt.Fprintf(&body, "\t%s %s\n", goName(arg.Name), gt)
		}
		body.WriteString("}\n\n")

		// ProgramID method.
		fmt.Fprintf(&body, "// ProgramID returns the program address for the %s instruction.\n", name)
		if idl.Address != "" {
			fmt.Fprintf(&body, "func (ix *%sInstruction) ProgramID() solana.PublicKey { return ProgramAddress }\n\n", name)
		} else {
			fmt.Fprintf(&body, "func (ix *%sInstruction) ProgramID() solana.PublicKey { return solana.PublicKey{} }\n\n", name)
		}

		// Accounts method.
		fmt.Fprintf(&body, "// Accounts returns account metas for the %s instruction in\n", name)
		fmt.Fprintf(&body, "// the positional order the program expects.\n")
		fmt.Fprintf(&body, "func (ix *%sInstruction) Accounts() []*solana.AccountMeta {\n", name)
		fmt.Fprintf(&body, "\treturn []*solana.AccountMeta{\n")
		for _, acc := range ix.Accounts {
			fmt.Fprintf(&body, "\t\tsolana.NewAccountMeta(ix.%s, %v, %v),\n",
				goName(acc.Name), acc.Signer, acc.Writable)
		}
		fmt.Fprintf(&body, "\t}\n}\n\n")

		// Data method: discriminator + Borsh-encoded args.
		fmt.Fprintf(&body, "// Data returns the Borsh-encoded instruction payload: 8-byte\n")
		fmt.Fprintf(&body, "// discriminator followed by the instruction arguments.\n")
		fmt.Fprintf(&body, "func (ix *%sInstruction) Data() ([]byte, error) {\n", name)
		fmt.Fprintf(&body, "\tvar buf bytes.Buffer\n")
		fmt.Fprintf(&body, "\tbuf.Write(%sDiscriminator[:])\n", name)
		for _, arg := range ix.Args {
			if err := emitBorshWrite(&body, "ix."+goName(arg.Name), arg.Type, "\t", typeDefs); err != nil {
				return fmt.Errorf("instruction %s arg %s: %w", ix.Name, arg.Name, err)
			}
		}
		fmt.Fprintf(&body, "\treturn buf.Bytes(), nil\n}\n\n")
	}

	bodyStr := body.String()
	needsBinary := strings.Contains(bodyStr, "binary.LittleEndian")

	var header bytes.Buffer
	header.WriteString(fileHeader())
	fmt.Fprintf(&header, "package %s\n\n", pkgName)
	header.WriteString("import (\n")
	header.WriteString("\t\"bytes\"\n")
	if needsBinary {
		header.WriteString("\t\"encoding/binary\"\n")
	}
	header.WriteString("\n\tsolana \"github.com/cielu/solana-go\"\n")
	header.WriteString(")\n\n")

	raw := append(header.Bytes(), []byte(bodyStr)...)
	formatted, err := format.Source(raw)
	if err != nil {
		return fmt.Errorf("format instructions: %w\n--- raw ---\n%s", err, raw)
	}
	_, err = w.Write(formatted)
	return err
}

// emitBorshWrite appends Go statements to buf that Borsh-encode val
// (a Go expression of the given IDL type) into the local bytes.Buffer
// variable named "buf". indent is prepended to each statement line.
func emitBorshWrite(buf *bytes.Buffer, val string, t TypeRef, indent string, typeDefs map[string]TypeBody) error {
	switch {
	case t.Primitive != "":
		return emitBorshPrimitive(buf, val, t.Primitive, indent)
	case t.Vec != nil:
		fmt.Fprintf(buf, "%s{\n", indent)
		fmt.Fprintf(buf, "%svar _l [4]byte\n", indent)
		fmt.Fprintf(buf, "%sbinary.LittleEndian.PutUint32(_l[:], uint32(len(%s)))\n", indent, val)
		fmt.Fprintf(buf, "%sbuf.Write(_l[:])\n", indent)
		fmt.Fprintf(buf, "%sfor _, _elem := range %s {\n", indent, val)
		if err := emitBorshWrite(buf, "_elem", *t.Vec, indent+"\t", typeDefs); err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s}\n", indent)
		fmt.Fprintf(buf, "%s}\n", indent)
	case t.Option != nil:
		fmt.Fprintf(buf, "%sif %s == nil {\n", indent, val)
		fmt.Fprintf(buf, "%s\tbuf.WriteByte(0)\n", indent)
		fmt.Fprintf(buf, "%s} else {\n", indent)
		fmt.Fprintf(buf, "%s\tbuf.WriteByte(1)\n", indent)
		if err := emitBorshWrite(buf, "*"+val, *t.Option, indent+"\t", typeDefs); err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s}\n", indent)
	case t.Array != nil:
		fmt.Fprintf(buf, "%sfor _, _elem := range %s {\n", indent, val)
		if err := emitBorshWrite(buf, "_elem", *t.Array.Element, indent+"\t", typeDefs); err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s}\n", indent)
	case t.Defined != "":
		td, ok := typeDefs[t.Defined]
		if !ok {
			return fmt.Errorf("undefined type %q referenced in instruction arg", t.Defined)
		}
		if td.Kind != "struct" {
			return fmt.Errorf("defined type %q is %q; only struct args are supported for inline encoding", t.Defined, td.Kind)
		}
		for _, f := range td.Fields {
			if err := emitBorshWrite(buf, val+"."+goName(f.Name), f.Type, indent, typeDefs); err != nil {
				return fmt.Errorf("field %s of %s: %w", f.Name, t.Defined, err)
			}
		}
	default:
		return fmt.Errorf("empty type reference")
	}
	return nil
}

// emitBorshPrimitive appends the single Borsh-encoding statement for
// a primitive IDL type into buf.
func emitBorshPrimitive(buf *bytes.Buffer, val, prim, indent string) error {
	switch prim {
	case "u8":
		fmt.Fprintf(buf, "%sbuf.WriteByte(%s)\n", indent, val)
	case "i8":
		fmt.Fprintf(buf, "%sbuf.WriteByte(byte(%s))\n", indent, val)
	case "u16":
		fmt.Fprintf(buf, "%s{\nvar _b [2]byte\nbinary.LittleEndian.PutUint16(_b[:], %s)\nbuf.Write(_b[:])\n}\n", indent, val)
	case "i16":
		fmt.Fprintf(buf, "%s{\nvar _b [2]byte\nbinary.LittleEndian.PutUint16(_b[:], uint16(%s))\nbuf.Write(_b[:])\n}\n", indent, val)
	case "u32":
		fmt.Fprintf(buf, "%s{\nvar _b [4]byte\nbinary.LittleEndian.PutUint32(_b[:], %s)\nbuf.Write(_b[:])\n}\n", indent, val)
	case "i32":
		fmt.Fprintf(buf, "%s{\nvar _b [4]byte\nbinary.LittleEndian.PutUint32(_b[:], uint32(%s))\nbuf.Write(_b[:])\n}\n", indent, val)
	case "u64":
		fmt.Fprintf(buf, "%s{\nvar _b [8]byte\nbinary.LittleEndian.PutUint64(_b[:], %s)\nbuf.Write(_b[:])\n}\n", indent, val)
	case "i64":
		fmt.Fprintf(buf, "%s{\nvar _b [8]byte\nbinary.LittleEndian.PutUint64(_b[:], uint64(%s))\nbuf.Write(_b[:])\n}\n", indent, val)
	case "u128", "i128":
		fmt.Fprintf(buf, "%sbuf.Write(%s[:])\n", indent, val)
	case "bool":
		fmt.Fprintf(buf, "%sif %s {\nbuf.WriteByte(1)\n} else {\nbuf.WriteByte(0)\n}\n", indent, val)
	case "string":
		fmt.Fprintf(buf, "%s{\nvar _l [4]byte\nbinary.LittleEndian.PutUint32(_l[:], uint32(len(%s)))\nbuf.Write(_l[:])\nbuf.WriteString(%s)\n}\n", indent, val, val)
	case "bytes":
		fmt.Fprintf(buf, "%s{\nvar _l [4]byte\nbinary.LittleEndian.PutUint32(_l[:], uint32(len(%s)))\nbuf.Write(_l[:])\nbuf.Write(%s)\n}\n", indent, val, val)
	case "pubkey", "publicKey":
		fmt.Fprintf(buf, "%sbuf.Write(%s[:])\n", indent, val)
	default:
		return fmt.Errorf("unknown primitive type %q", prim)
	}
	return nil
}
