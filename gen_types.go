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
//     a const block of variant constants
//   - enum kinds with at least one data-carrying variant become a
//     sealed interface with one struct per variant, plus an EnumTag()
//     method each for Borsh encoding
//   - type kinds (aliases) become Go type aliases
//
// IDL docs arrays are emitted as Go line comments above the
// generated declaration.
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
func writeType(w *bytes.Buffer, td TypeDef) error {
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

func writeStruct(w *bytes.Buffer, td TypeDef) error {
	name := goName(td.Name)
	writeDocs(w, td.Docs, "")
	fmt.Fprintf(w, "// %s is generated from the Anchor IDL.\n", name)
	fmt.Fprintf(w, "type %s struct {\n", name)
	for _, f := range td.Type.Fields {
		writeDocs(w, f.Docs, "\t")
		gt, err := goTypeFor(f.Type)
		if err != nil {
			return fmt.Errorf("field %s.%s: %w", td.Name, f.Name, err)
		}
		fmt.Fprintf(w, "\t%s %s\n", goName(f.Name), gt)
	}
	fmt.Fprintf(w, "}\n\n")
	return nil
}

func writeEnum(w *bytes.Buffer, td TypeDef) error {
	name := goName(td.Name)
	writeDocs(w, td.Docs, "")

	if enumIsUnit(td.Type) {
		fmt.Fprintf(w, "// %s is generated from the Anchor IDL.\n", name)
		fmt.Fprintf(w, "type %s uint8\n\n", name)
		fmt.Fprintf(w, "const (\n")
		for i, v := range td.Type.Variants {
			fmt.Fprintf(w, "\t%s%s %s = %d\n", name, goName(v.Name), name, i)
		}
		fmt.Fprintf(w, ")\n\n")
		return nil
	}

	// Data-carrying enum: sealed interface + one struct per variant.
	fmt.Fprintf(w, "// %s is a Borsh-tagged union generated from the Anchor IDL.\n", name)
	fmt.Fprintf(w, "// Exactly one of the %s_* structs implements this interface.\n", name)
	fmt.Fprintf(w, "type %s interface {\n", name)
	fmt.Fprintf(w, "\tis%s()\n", name)
	fmt.Fprintf(w, "\t// EnumTag returns the Borsh variant tag (0-based in IDL order).\n")
	fmt.Fprintf(w, "\tEnumTag() uint8\n")
	fmt.Fprintf(w, "}\n\n")

	for i, v := range td.Type.Variants {
		vname := name + "_" + goName(v.Name)
		fmt.Fprintf(w, "// %s is the %q variant of %s.\n", vname, v.Name, name)
		fmt.Fprintf(w, "type %s struct {\n", vname)
		for _, f := range v.Fields {
			writeDocs(w, f.Docs, "\t")
			gt, err := goTypeFor(f.Type)
			if err != nil {
				return fmt.Errorf("enum %s variant %s field %s: %w", td.Name, v.Name, f.Name, err)
			}
			fmt.Fprintf(w, "\t%s %s\n", goName(f.Name), gt)
		}
		fmt.Fprintf(w, "}\n\n")
		fmt.Fprintf(w, "func (*%s) is%s() {}\n", vname, name)
		fmt.Fprintf(w, "func (*%s) EnumTag() uint8 { return %d }\n\n", vname, i)
	}
	return nil
}

func writeAlias(w *bytes.Buffer, td TypeDef) error {
	if td.Type.Alias == nil {
		return fmt.Errorf("type %s: kind=type but no alias target", td.Name)
	}
	name := goName(td.Name)
	gt, err := goTypeFor(*td.Type.Alias)
	if err != nil {
		return fmt.Errorf("alias %s: %w", td.Name, err)
	}
	writeDocs(w, td.Docs, "")
	fmt.Fprintf(w, "// %s is a type alias generated from the Anchor IDL.\n", name)
	fmt.Fprintf(w, "type %s = %s\n\n", name, gt)
	return nil
}
