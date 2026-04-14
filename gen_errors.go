package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
)

// GenerateErrors renders the package's errors.go file content into
// w. It emits one ProgramError sentinel per entry in idl.Errors
// plus an ErrorByCode helper that maps a numeric code back to its
// sentinel.
//
// The generated ProgramError type is local to the output package
// so it does not leak the generator's concept into the root
// solana package. Callers typically assert against the sentinel
// values with errors.Is, or inspect the numeric Code field.
func GenerateErrors(w io.Writer, pkgName string, idl *IDL) error {
	var body bytes.Buffer

	// Always emit the ProgramError type, even when the IDL has no
	// errors: downstream code may want to pattern-match on it, and
	// an empty switch in ErrorByCode is cheap.
	fmt.Fprintf(&body, "// ProgramError is an Anchor program error code declared in the IDL.\n")
	fmt.Fprintf(&body, "type ProgramError struct {\n")
	fmt.Fprintf(&body, "\tCode int\n")
	fmt.Fprintf(&body, "\tName string\n")
	fmt.Fprintf(&body, "\tMsg  string\n")
	fmt.Fprintf(&body, "}\n\n")

	fmt.Fprintf(&body, "// Error implements the error interface.\n")
	fmt.Fprintf(&body, "func (e *ProgramError) Error() string {\n")
	fmt.Fprintf(&body, "\treturn fmt.Sprintf(%q, e.Name, e.Code, e.Msg)\n", "%s (code %d): %s")
	fmt.Fprintf(&body, "}\n\n")

	for _, e := range idl.Errors {
		name := goName(e.Name)
		fmt.Fprintf(&body, "// Err%s corresponds to the Anchor error code %d.", name, e.Code)
		if e.Msg != "" {
			fmt.Fprintf(&body, " %s", e.Msg)
		}
		body.WriteString("\n")
		fmt.Fprintf(&body, "var Err%s = &ProgramError{Code: %d, Name: %q, Msg: %q}\n\n",
			name, e.Code, e.Name, e.Msg)
	}

	fmt.Fprintf(&body, "// ErrorByCode returns the ProgramError matching the given numeric\n")
	fmt.Fprintf(&body, "// code, or nil if the code is not declared in the program's IDL.\n")
	fmt.Fprintf(&body, "func ErrorByCode(code int) *ProgramError {\n")
	if len(idl.Errors) > 0 {
		fmt.Fprintf(&body, "\tswitch code {\n")
		for _, e := range idl.Errors {
			fmt.Fprintf(&body, "\tcase %d:\n\t\treturn Err%s\n", e.Code, goName(e.Name))
		}
		fmt.Fprintf(&body, "\t}\n")
	}
	fmt.Fprintf(&body, "\treturn nil\n}\n")

	var header bytes.Buffer
	header.WriteString(fileHeader())
	fmt.Fprintf(&header, "package %s\n\n", pkgName)
	header.WriteString("import \"fmt\"\n\n")

	raw := append(header.Bytes(), body.Bytes()...)
	formatted, err := format.Source(raw)
	if err != nil {
		return fmt.Errorf("format errors: %w\n--- raw ---\n%s", err, raw)
	}
	_, err = w.Write(formatted)
	return err
}
