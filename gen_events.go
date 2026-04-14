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
//     (Anchor 0.30+ events always have a discriminator; 0.29 events
//     use the event name's sighash, which is computed off-chain)
//
// If idl.Events is empty, GenerateEvents still writes a valid Go
// file containing just the package declaration. Callers that want
// to skip emission in that case should check len(idl.Events) first.
func GenerateEvents(w io.Writer, pkgName string, idl *IDL) error {
	var body bytes.Buffer

	for _, e := range idl.Events {
		name := goName(e.Name)
		fmt.Fprintf(&body, "// %s is an event emitted by the program.\n", name)
		fmt.Fprintf(&body, "type %s struct {\n", name)
		for _, f := range e.Fields {
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
	}

	var header bytes.Buffer
	header.WriteString(fileHeader())
	fmt.Fprintf(&header, "package %s\n\n", pkgName)
	if strings.Contains(body.String(), "solana.PublicKey") {
		header.WriteString("import \"github.com/cielu/solana-go\"\n\n")
	}

	raw := append(header.Bytes(), body.Bytes()...)
	formatted, err := format.Source(raw)
	if err != nil {
		return fmt.Errorf("format events: %w\n--- raw ---\n%s", err, raw)
	}
	_, err = w.Write(formatted)
	return err
}
