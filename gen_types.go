package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"strings"
)

// GenerateTypes renders the package's types.go file content into w.
// It emits one Go type per entry in idl.Types:
//
//   - struct kinds become Go struct types with one field per IDL
//     field, using goName to convert the field name
//   - enum kinds with unit-only variants become a uint8 alias plus
//     a const block of variant constants. Data-carrying enum
//     variants are not yet supported and produce a clear error.
//
// The output is post-processed through go/format.Source so the
// result is gofmt-clean and can be diffed byte-for-byte against
// a checked-in golden file.
func GenerateTypes(w io.Writer, pkgName string, idl *IDL) error {
	var body bytes.Buffer
	for _, td := range idl.Types {
		if err := writeType(&body, td); err != nil {
			return err
		}
	}

	var header bytes.Buffer
	header.WriteString(fileHeader())
	fmt.Fprintf(&header, "package %s\n\n", pkgName)

	// Determine imports from the body text. At this stage only
	// solana.PublicKey triggers an import; future additions
	// (encoding/binary, etc.) will add more detection.
	if strings.Contains(body.String(), "solana.PublicKey") {
		header.WriteString("import \"github.com/cielu/solana-go\"\n\n")
	}

	raw := append(header.Bytes(), body.Bytes()...)
	formatted, err := format.Source(raw)
	if err != nil {
		return fmt.Errorf("format generated types: %w\n--- raw ---\n%s", err, raw)
	}
	_, err = w.Write(formatted)
	return err
}

// writeType dispatches on the kind of a TypeDef and emits the
// matching Go declaration into w.
func writeType(w io.Writer, td TypeDef) error {
	switch td.Type.Kind {
	case "struct":
		return writeStruct(w, td)
	case "enum":
		return writeEnum(w, td)
	case "type":
		return writeAlias(w, td)
	default:
		return fmt.Errorf("type %s: unsupported kind %q", td.Name, td.Type.Kind)
	}
}

func writeStruct(w io.Writer, td TypeDef) error {
	fmt.Fprintf(w, "// %s is generated from the Anchor IDL.\n", goName(td.Name))
	fmt.Fprintf(w, "type %s struct {\n", goName(td.Name))
	for _, f := range td.Type.Fields {
		gt, err := goTypeFor(f.Type)
		if err != nil {
			return fmt.Errorf("field %s.%s: %w", td.Name, f.Name, err)
		}
		fmt.Fprintf(w, "\t%s %s\n", goName(f.Name), gt)
	}
	fmt.Fprintf(w, "}\n\n")
	return nil
}

func writeEnum(w io.Writer, td TypeDef) error {
	for _, v := range td.Type.Variants {
		if len(v.Fields) > 0 {
			return fmt.Errorf("enum %s variant %s carries data, which is not yet supported by anchoridl-gen", td.Name, v.Name)
		}
	}
	name := goName(td.Name)
	fmt.Fprintf(w, "// %s is generated from the Anchor IDL.\n", name)
	fmt.Fprintf(w, "type %s uint8\n\n", name)
	fmt.Fprintf(w, "const (\n")
	for i, v := range td.Type.Variants {
		fmt.Fprintf(w, "\t%s%s %s = %d\n", name, goName(v.Name), name, i)
	}
	fmt.Fprintf(w, ")\n\n")
	return nil
}

func writeAlias(w io.Writer, td TypeDef) error {
	if td.Type.Alias == nil {
		return fmt.Errorf("type %s: kind=type but no alias target", td.Name)
	}
	gt, err := goTypeFor(*td.Type.Alias)
	if err != nil {
		return fmt.Errorf("alias %s: %w", td.Name, err)
	}
	fmt.Fprintf(w, "// %s is a type alias generated from the Anchor IDL.\n", goName(td.Name))
	fmt.Fprintf(w, "type %s = %s\n\n", goName(td.Name), gt)
	return nil
}
