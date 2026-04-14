package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"strings"
)

// GenerateEvents renders the package's events.go file content into
// w. For each entry in idl.Events it emits:
//
//   - a Go struct with one field per IDL field
//   - an 8-byte Discriminator variable when the IDL supplies one
//   - an UnmarshalXxx function that checks the discriminator (if
//     present) and Borsh-decodes the remainder into the struct
//
// The Borsh read helpers (readU16/readU32/readU64/readString/readBytes)
// are declared in accounts.go; events.go references them but does
// not duplicate the declarations. When the IDL has no accounts —
// and therefore accounts.go would have declared no helpers — events.go
// emits its own copy so the package still compiles.
func GenerateEvents(w io.Writer, pkgName string, idl *IDL) error {
	typeDefs := make(map[string]TypeBody, len(idl.Types))
	for _, td := range idl.Types {
		typeDefs[td.Name] = td.Type
	}

	var body bytes.Buffer

	for _, e := range idl.Events {
		name := goName(e.Name)
		writeDocs(&body, e.Docs, "")
		fmt.Fprintf(&body, "// %s is an event emitted by the program.\n", name)
		fmt.Fprintf(&body, "type %s struct {\n", name)
		for _, f := range e.Fields {
			writeDocs(&body, f.Docs, "\t")
			gt, err := goTypeFor(f.Type)
			if err != nil {
				return fmt.Errorf("event %s field %s: %w", e.Name, f.Name, err)
			}
			fmt.Fprintf(&body, "\t%s %s\n", goName(f.Name), gt)
		}
		fmt.Fprintf(&body, "}\n\n")

		if len(e.Discriminator) == 8 {
			fmt.Fprintf(&body, "// %sDiscriminator is the 8-byte prefix that identifies a\n", name)
			fmt.Fprintf(&body, "// serialised %s event emitted by the program.\n", name)
			fmt.Fprintf(&body, "var %sDiscriminator = [8]byte{", name)
			for i, b := range e.Discriminator {
				if i > 0 {
					body.WriteString(", ")
				}
				fmt.Fprintf(&body, "0x%02x", b)
			}
			body.WriteString("}\n\n")
		}

		// Decoder: Unmarshal wraps a private decode* helper so the
		// Borsh-reader fragments (which assume a single error return)
		// don't need to know about the (*T, error) outer signature.
		fmt.Fprintf(&body, "// Unmarshal%s decodes an emitted %s event.\n", name, name)
		if len(e.Discriminator) == 8 {
			fmt.Fprintf(&body, "// The leading 8-byte discriminator is verified before decoding.\n")
		}
		fmt.Fprintf(&body, "func Unmarshal%s(data []byte) (*%s, error) {\n", name, name)
		offset := 0
		if len(e.Discriminator) == 8 {
			fmt.Fprintf(&body, "\tif len(data) < 8 {\n\t\treturn nil, fmt.Errorf(\"%s: short header (%%d bytes)\", len(data))\n\t}\n", name)
			fmt.Fprintf(&body, "\tif !bytes.Equal(data[:8], %sDiscriminator[:]) {\n", name)
			fmt.Fprintf(&body, "\t\treturn nil, fmt.Errorf(\"%s: discriminator mismatch\")\n\t}\n", name)
			offset = 8
		}
		fmt.Fprintf(&body, "\tr := bytes.NewReader(data[%d:])\n", offset)
		fmt.Fprintf(&body, "\tout := &%s{}\n", name)
		fmt.Fprintf(&body, "\tif err := decode%sEvent(r, out); err != nil { return nil, err }\n", name)
		fmt.Fprintf(&body, "\treturn out, nil\n}\n\n")

		fmt.Fprintf(&body, "// decode%sEvent Borsh-decodes the fields of %s from r.\n", name, name)
		fmt.Fprintf(&body, "func decode%sEvent(r *bytes.Reader, out *%s) error {\n", name, name)
		for _, f := range e.Fields {
			if err := emitBorshRead(&body, "r", "out."+goName(f.Name), f.Type, "\t", typeDefs); err != nil {
				return fmt.Errorf("event %s field %s: %w", e.Name, f.Name, err)
			}
		}
		fmt.Fprintf(&body, "\treturn nil\n}\n\n")
	}

	// If the package has no accounts, accounts.go won't declare the
	// readBorshReadHelpers; emit them here so events.go still compiles.
	if len(idl.Events) > 0 && len(idl.Accounts) == 0 {
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
		return fmt.Errorf("format events: %w\n--- raw ---\n%s", err, raw)
	}
	_, err = w.Write(formatted)
	return err
}
