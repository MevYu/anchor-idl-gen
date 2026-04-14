package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
)

// GenerateAccounts renders the package's accounts.go file content
// into w. It emits one 8-byte discriminator variable per entry in
// idl.Accounts and a MatchAccount helper that reports which known
// account type (if any) a raw byte slice starts with.
//
// The struct layout of each account type lives in idl.Types under
// the same Name and is emitted by GenerateTypes, so accounts.go
// carries only the discriminator plus the match helper. A richer
// zero-copy Unmarshal will land in a follow-up once the IDL
// serialisation rules are fully implemented for every field
// shape.
func GenerateAccounts(w io.Writer, pkgName string, idl *IDL) error {
	var body bytes.Buffer

	for _, a := range idl.Accounts {
		if len(a.Discriminator) != 8 {
			return fmt.Errorf("account %s: expected 8-byte discriminator, got %d bytes", a.Name, len(a.Discriminator))
		}
		name := goName(a.Name)
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

	if len(idl.Accounts) > 0 {
		fmt.Fprintf(&body, "// MatchAccount returns the name of the account type whose\n")
		fmt.Fprintf(&body, "// discriminator matches the first 8 bytes of data, or an empty\n")
		fmt.Fprintf(&body, "// string if no known type matches. It does not verify the\n")
		fmt.Fprintf(&body, "// remainder of data; callers should decode further with the\n")
		fmt.Fprintf(&body, "// matched type's Unmarshal function (generated in a follow-up).\n")
		fmt.Fprintf(&body, "func MatchAccount(data []byte) string {\n")
		fmt.Fprintf(&body, "\tif len(data) < 8 {\n\t\treturn \"\"\n\t}\n")
		fmt.Fprintf(&body, "\tvar head [8]byte\n\tcopy(head[:], data[:8])\n")
		fmt.Fprintf(&body, "\tswitch head {\n")
		for _, a := range idl.Accounts {
			name := goName(a.Name)
			fmt.Fprintf(&body, "\tcase %sDiscriminator:\n\t\treturn %q\n", name, name)
		}
		fmt.Fprintf(&body, "\t}\n\treturn \"\"\n}\n")
	}

	var header bytes.Buffer
	header.WriteString(fileHeader())
	fmt.Fprintf(&header, "package %s\n\n", pkgName)

	raw := append(header.Bytes(), body.Bytes()...)
	formatted, err := format.Source(raw)
	if err != nil {
		return fmt.Errorf("format accounts: %w\n--- raw ---\n%s", err, raw)
	}
	_, err = w.Write(formatted)
	return err
}
