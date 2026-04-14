package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"strings"
)

// GenerateAccounts renders the package's accounts.go file content
// into w. For each entry in idl.Accounts it emits:
//
//   - an 8-byte Anchor discriminator variable
//   - an UnmarshalXxx function that checks the discriminator,
//     Borsh-decodes the remaining bytes into the struct generated
//     by GenerateTypes, and returns the populated value
//
// It also emits a MatchAccount helper that reports which known
// account type (if any) a raw byte slice begins with, plus the
// small readU16/readU32/readU64/readString/readBytes helpers used
// by the generated decoders (declared here so events.go can share
// them without re-declaring).
func GenerateAccounts(w io.Writer, pkgName string, idl *IDL) error {
	// Build a map of type body lookups so we can Borsh-decode each
	// account using the fields defined in idl.Types.
	typeDefs := make(map[string]TypeBody, len(idl.Types))
	for _, td := range idl.Types {
		typeDefs[td.Name] = td.Type
	}

	var body bytes.Buffer

	// Validate first.
	for _, a := range idl.Accounts {
		if len(a.Discriminator) != 8 {
			return fmt.Errorf("account %s: expected 8-byte discriminator, got %d bytes", a.Name, len(a.Discriminator))
		}
	}

	// Discriminator variables.
	for _, a := range idl.Accounts {
		name := goName(a.Name)
		writeDocs(&body, a.Docs, "")
		fmt.Fprintf(&body, "// %sDiscriminator is the 8-byte Anchor discriminator prefix for a\n", name)
		fmt.Fprintf(&body, "// %s account, used to distinguish it from other accounts owned by\n", name)
		fmt.Fprintf(&body, "// the same program.\n")
		fmt.Fprintf(&body, "var %sDiscriminator = [8]byte{", name)
		for i, b := range a.Discriminator {
			if i > 0 {
				body.WriteString(", ")
			}
			fmt.Fprintf(&body, "0x%02x", b)
		}
		body.WriteString("}\n\n")
	}

	// MatchAccount helper.
	if len(idl.Accounts) > 0 {
		fmt.Fprintf(&body, "// MatchAccount returns the name of the account type whose\n")
		fmt.Fprintf(&body, "// discriminator matches the first 8 bytes of data, or an empty\n")
		fmt.Fprintf(&body, "// string if no known type matches. It does not verify the\n")
		fmt.Fprintf(&body, "// remainder of data; use UnmarshalXxx to fully decode the\n")
		fmt.Fprintf(&body, "// account body.\n")
		fmt.Fprintf(&body, "func MatchAccount(data []byte) string {\n")
		fmt.Fprintf(&body, "\tif len(data) < 8 {\n\t\treturn \"\"\n\t}\n")
		fmt.Fprintf(&body, "\tvar head [8]byte\n\tcopy(head[:], data[:8])\n")
		fmt.Fprintf(&body, "\tswitch head {\n")
		for _, a := range idl.Accounts {
			name := goName(a.Name)
			fmt.Fprintf(&body, "\tcase %sDiscriminator:\n\t\treturn %q\n", name, name)
		}
		fmt.Fprintf(&body, "\t}\n\treturn \"\"\n}\n\n")
	}

	// Decoders.
	var needsDecoder bool
	for _, a := range idl.Accounts {
		td, ok := typeDefs[a.Name]
		if !ok {
			// Without a type definition we can't decode, just skip.
			continue
		}
		if td.Kind != "struct" {
			continue
		}
		needsDecoder = true
		name := goName(a.Name)
		fmt.Fprintf(&body, "// Unmarshal%s decodes the on-chain %s account bytes: the leading\n", name, name)
		fmt.Fprintf(&body, "// 8-byte discriminator is verified and the remainder is Borsh-decoded\n")
		fmt.Fprintf(&body, "// into a new %s value.\n", name)
		fmt.Fprintf(&body, "func Unmarshal%s(data []byte) (*%s, error) {\n", name, name)
		fmt.Fprintf(&body, "\tif len(data) < 8 {\n\t\treturn nil, fmt.Errorf(\"%s: short header (%%d bytes)\", len(data))\n\t}\n", name)
		fmt.Fprintf(&body, "\tif !bytes.Equal(data[:8], %sDiscriminator[:]) {\n", name)
		fmt.Fprintf(&body, "\t\treturn nil, fmt.Errorf(\"%s: discriminator mismatch\")\n\t}\n", name)
		fmt.Fprintf(&body, "\tr := bytes.NewReader(data[8:])\n")
		fmt.Fprintf(&body, "\tout := &%s{}\n", name)
		fmt.Fprintf(&body, "\tif err := decode%s(r, out); err != nil { return nil, err }\n", name)
		fmt.Fprintf(&body, "\treturn out, nil\n")
		fmt.Fprintf(&body, "}\n\n")

		fmt.Fprintf(&body, "// decode%s Borsh-decodes the fields of %s from r.\n", name, name)
		fmt.Fprintf(&body, "func decode%s(r *bytes.Reader, out *%s) error {\n", name, name)
		for _, f := range td.Fields {
			if err := emitBorshRead(&body, "r", "out."+goName(f.Name), f.Type, "\t", typeDefs); err != nil {
				return fmt.Errorf("account %s field %s: %w", a.Name, f.Name, err)
			}
		}
		fmt.Fprintf(&body, "\treturn nil\n}\n\n")
	}

	// Shared Borsh read helpers, emitted only if a decoder was produced.
	if needsDecoder {
		writeBorshReadHelpers(&body)
	}

	bodyStr := body.String()
	needsBytes := strings.Contains(bodyStr, "bytes.")
	needsBinaryPkg := strings.Contains(bodyStr, "binary.LittleEndian")
	needsFmt := strings.Contains(bodyStr, "fmt.")
	needsIO := strings.Contains(bodyStr, "io.ReadFull")
	needsSolana := strings.Contains(bodyStr, "solana.")

	var header bytes.Buffer
	header.WriteString(fileHeader())
	fmt.Fprintf(&header, "package %s\n\n", pkgName)
	if needsBytes || needsBinaryPkg || needsFmt || needsIO || needsSolana {
		header.WriteString("import (\n")
		if needsBytes {
			header.WriteString("\t\"bytes\"\n")
		}
		if needsBinaryPkg {
			header.WriteString("\t\"encoding/binary\"\n")
		}
		if needsFmt {
			header.WriteString("\t\"fmt\"\n")
		}
		if needsIO {
			header.WriteString("\t\"io\"\n")
		}
		if needsSolana {
			header.WriteString("\n\tsolana \"github.com/cielu/solana-go\"\n")
		}
		header.WriteString(")\n\n")
	}

	raw := append(header.Bytes(), []byte(bodyStr)...)
	formatted, err := format.Source(raw)
	if err != nil {
		return fmt.Errorf("format accounts: %w\n--- raw ---\n%s", err, raw)
	}
	_, err = w.Write(formatted)
	return err
}
