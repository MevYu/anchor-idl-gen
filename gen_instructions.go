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
// fields from idl.Types; data-carrying enum arguments are encoded
// via the sealed-interface helpers emitted by GenerateTypes.
//
// Optional instruction accounts use *solana.PublicKey so callers can
// pass nil to omit them; the generated Accounts() slice skips nil
// optional accounts rather than emitting an all-zero meta.
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
		writeDocs(&body, ix.Docs, "")
		fmt.Fprintf(&body, "// %sInstruction builds the %s instruction.\n", name, name)
		fmt.Fprintf(&body, "type %sInstruction struct {\n", name)
		for _, acc := range ix.Accounts {
			writeDocs(&body, acc.Docs, "\t")
			var tag string
			switch {
			case acc.Optional:
				tag = " // optional"
			case acc.Signer && acc.Writable:
				tag = " // signer, writable"
			case acc.Signer:
				tag = " // signer"
			case acc.Writable:
				tag = " // writable"
			}
			typ := "solana.PublicKey"
			if acc.Optional {
				typ = "*solana.PublicKey"
			}
			fmt.Fprintf(&body, "\t%s %s%s\n", goName(acc.Name), typ, tag)
		}
		for _, arg := range ix.Args {
			writeDocs(&body, arg.Docs, "\t")
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
		fmt.Fprintf(&body, "// the positional order the program expects. Optional accounts\n")
		fmt.Fprintf(&body, "// that are nil are skipped.\n")
		fmt.Fprintf(&body, "func (ix *%sInstruction) Accounts() []*solana.AccountMeta {\n", name)
		fmt.Fprintf(&body, "\tmetas := make([]*solana.AccountMeta, 0, %d)\n", len(ix.Accounts))
		for _, acc := range ix.Accounts {
			fieldName := goName(acc.Name)
			if acc.Optional {
				fmt.Fprintf(&body, "\tif ix.%s != nil {\n", fieldName)
				fmt.Fprintf(&body, "\t\tmetas = append(metas, solana.NewAccountMeta(*ix.%s, %v, %v))\n",
					fieldName, acc.Signer, acc.Writable)
				fmt.Fprintf(&body, "\t}\n")
			} else {
				fmt.Fprintf(&body, "\tmetas = append(metas, solana.NewAccountMeta(ix.%s, %v, %v))\n",
					fieldName, acc.Signer, acc.Writable)
			}
		}
		fmt.Fprintf(&body, "\treturn metas\n}\n\n")

		// Data method: discriminator + Borsh-encoded args.
		fmt.Fprintf(&body, "// Data returns the Borsh-encoded instruction payload: 8-byte\n")
		fmt.Fprintf(&body, "// discriminator followed by the instruction arguments.\n")
		fmt.Fprintf(&body, "func (ix *%sInstruction) Data() ([]byte, error) {\n", name)
		fmt.Fprintf(&body, "\tvar buf bytes.Buffer\n")
		fmt.Fprintf(&body, "\tbuf.Write(%sDiscriminator[:])\n", name)
		for _, arg := range ix.Args {
			if err := emitBorshWrite(&body, "buf", "ix."+goName(arg.Name), arg.Type, "\t", typeDefs); err != nil {
				return fmt.Errorf("instruction %s arg %s: %w", ix.Name, arg.Name, err)
			}
		}
		fmt.Fprintf(&body, "\treturn buf.Bytes(), nil\n}\n\n")
	}

	bodyStr := body.String()
	needsBinaryPkg := strings.Contains(bodyStr, "binary.LittleEndian")

	var header bytes.Buffer
	header.WriteString(fileHeader())
	fmt.Fprintf(&header, "package %s\n\n", pkgName)
	header.WriteString("import (\n")
	header.WriteString("\t\"bytes\"\n")
	if needsBinaryPkg {
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
