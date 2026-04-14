package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"strings"
)

// GeneratePDA renders the package's pda.go file content into w.
// For each instruction account that carries a PDA definition in the
// IDL, it emits a Derive<Instruction><Account> helper function that
// wraps solana.FindProgramAddress.
//
// Seeds of kind "const" are inlined as byte-array or string literals.
// Seeds of kind "account" and "arg" become function parameters. Any
// unknown seed kind is left as a TODO comment so the file still
// compiles and can be completed manually.
//
// If no PDA seeds are present across all instructions, GeneratePDA
// still writes a valid package declaration so the output directory
// compiles cleanly.
func GeneratePDA(w io.Writer, pkgName string, idl *IDL) error {
	var body bytes.Buffer

	for _, ix := range idl.Instructions {
		for _, acc := range ix.Accounts {
			if acc.PDA == nil || len(acc.PDA.Seeds) == 0 {
				continue
			}
			if err := emitPDADeriver(&body, idl, ix, acc); err != nil {
				return fmt.Errorf("instruction %s account %s: %w", ix.Name, acc.Name, err)
			}
		}
	}

	bodyStr := body.String()
	needsSolana := strings.Contains(bodyStr, "solana.")

	var header bytes.Buffer
	header.WriteString(fileHeader())
	fmt.Fprintf(&header, "package %s\n\n", pkgName)
	if needsSolana {
		header.WriteString("import (\n")
		header.WriteString("\tsolana \"github.com/cielu/solana-go\"\n")
		header.WriteString(")\n\n")
	}

	raw := append(header.Bytes(), []byte(bodyStr)...)
	formatted, err := format.Source(raw)
	if err != nil {
		return fmt.Errorf("format pda: %w\n--- raw ---\n%s", err, raw)
	}
	_, err = w.Write(formatted)
	return err
}

// emitPDADeriver writes one Derive* function for the given
// instruction account PDA.
func emitPDADeriver(buf *bytes.Buffer, idl *IDL, ix InstructionDef, acc InstructionAcc) error {
	fnName := "Derive" + goName(ix.Name) + goName(acc.Name)
	pda := acc.PDA

	// Collect parameters for "account" and "arg" seeds (deduplicated).
	type param struct {
		name  string
		goTyp string
	}
	var params []param
	seen := map[string]bool{}
	for _, seed := range pda.Seeds {
		switch seed.Kind {
		case "account":
			pname := lowerFirst(goName(seed.Path))
			if !seen[pname] {
				params = append(params, param{pname, "solana.PublicKey"})
				seen[pname] = true
			}
		case "arg":
			pname := lowerFirst(goName(seed.Path))
			if seen[pname] {
				continue
			}
			var argType TypeRef
			for _, arg := range ix.Args {
				if arg.Name == seed.Path {
					argType = arg.Type
					break
				}
			}
			gt, err := goTypeFor(argType)
			if err != nil {
				gt = "[]byte"
			}
			params = append(params, param{pname, gt})
			seen[pname] = true
		}
	}

	// Function signature.
	fmt.Fprintf(buf, "// %s derives the %s PDA for the %s instruction.\n",
		fnName, goName(acc.Name), goName(ix.Name))
	fmt.Fprintf(buf, "func %s(", fnName)
	for i, p := range params {
		if i > 0 {
			buf.WriteString(", ")
		}
		fmt.Fprintf(buf, "%s %s", p.name, p.goTyp)
	}
	buf.WriteString(") (solana.PublicKey, uint8, error) {\n")

	// Seeds slice.
	buf.WriteString("\tseeds := [][]byte{\n")
	for _, seed := range pda.Seeds {
		switch seed.Kind {
		case "const":
			// Try JSON array of integers first, then string.
			var vals []int
			if err := json.Unmarshal(seed.Value, &vals); err == nil {
				buf.WriteString("\t\t{")
				for i, v := range vals {
					if i > 0 {
						buf.WriteString(", ")
					}
					fmt.Fprintf(buf, "0x%02x", v)
				}
				buf.WriteString("},\n")
			} else {
				var s string
				if err2 := json.Unmarshal(seed.Value, &s); err2 == nil {
					fmt.Fprintf(buf, "\t\t[]byte(%q),\n", s)
				} else {
					fmt.Fprintf(buf, "\t\t// TODO: const seed: %s\n", seed.Value)
					buf.WriteString("\t\t[]byte{},\n")
				}
			}
		case "account":
			pname := lowerFirst(goName(seed.Path))
			fmt.Fprintf(buf, "\t\t%s[:],\n", pname)
		case "arg":
			pname := lowerFirst(goName(seed.Path))
			fmt.Fprintf(buf, "\t\t// TODO: Borsh-encode arg %q into seed bytes (param: %s).\n", seed.Path, pname)
			buf.WriteString("\t\t[]byte{},\n")
		default:
			fmt.Fprintf(buf, "\t\t// TODO: unknown seed kind %q\n", seed.Kind)
			buf.WriteString("\t\t[]byte{},\n")
		}
	}
	buf.WriteString("\t}\n")

	// FindProgramAddress call.
	progExpr := "ProgramAddress"
	if idl.Address == "" {
		progExpr = "solana.PublicKey{} /* set program address */"
	}
	fmt.Fprintf(buf, "\treturn solana.FindProgramAddress(seeds, %s)\n", progExpr)
	buf.WriteString("}\n\n")
	return nil
}

// lowerFirst returns s with its first byte lowercased.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
