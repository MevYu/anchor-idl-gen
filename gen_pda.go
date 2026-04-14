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
// Seeds of kind "account" and "arg" become function parameters;
// account seeds are dereferenced directly as the account's public
// key bytes, and arg seeds are Borsh-encoded into a local buffer so
// the derivation produces the same bytes the program sees on-chain.
//
// When the IDL's PDA definition carries a "program" override (i.e.
// the PDA is derived under a program other than the one the
// instruction belongs to), GeneratePDA honours it: a constant program
// address becomes a literal PublicKey, and an account-reference
// program becomes an extra typed parameter.
func GeneratePDA(w io.Writer, pkgName string, idl *IDL) error {
	typeDefs := make(map[string]TypeBody, len(idl.Types))
	for _, td := range idl.Types {
		typeDefs[td.Name] = td.Type
	}

	var body bytes.Buffer

	for _, ix := range idl.Instructions {
		for _, acc := range ix.Accounts {
			if acc.PDA == nil || len(acc.PDA.Seeds) == 0 {
				continue
			}
			if err := emitPDADeriver(&body, idl, ix, acc, typeDefs); err != nil {
				return fmt.Errorf("instruction %s account %s: %w", ix.Name, acc.Name, err)
			}
		}
	}

	bodyStr := body.String()
	needsSolana := strings.Contains(bodyStr, "solana.")
	needsBytes := strings.Contains(bodyStr, "bytes.Buffer")
	needsBinaryPkg := strings.Contains(bodyStr, "binary.LittleEndian")

	var header bytes.Buffer
	header.WriteString(fileHeader())
	fmt.Fprintf(&header, "package %s\n\n", pkgName)
	if needsBytes || needsBinaryPkg || needsSolana {
		header.WriteString("import (\n")
		if needsBytes {
			header.WriteString("\t\"bytes\"\n")
		}
		if needsBinaryPkg {
			header.WriteString("\t\"encoding/binary\"\n")
		}
		if needsBytes || needsBinaryPkg {
			header.WriteString("\n")
		}
		if needsSolana {
			header.WriteString("\tsolana \"github.com/cielu/solana-go\"\n")
		}
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
func emitPDADeriver(buf *bytes.Buffer, idl *IDL, ix InstructionDef, acc InstructionAcc, typeDefs map[string]TypeBody) error {
	fnName := "Derive" + goName(ix.Name) + goName(acc.Name)
	pda := acc.PDA

	// Collect parameters for "account" and "arg" seeds (deduplicated).
	type param struct {
		name  string
		goTyp string
	}
	var params []param
	seen := map[string]bool{}
	addParam := func(name, goTyp string) {
		if seen[name] {
			return
		}
		params = append(params, param{name, goTyp})
		seen[name] = true
	}
	// Track arg types so we can Borsh-encode them in the body.
	argTypes := map[string]TypeRef{}
	for _, seed := range pda.Seeds {
		switch seed.Kind {
		case "account":
			addParam(lowerFirst(goName(seed.Path)), "solana.PublicKey")
		case "arg":
			var argType TypeRef
			for _, arg := range ix.Args {
				if arg.Name == seed.Path {
					argType = arg.Type
					break
				}
			}
			if seed.Type != nil {
				argType = *seed.Type
			}
			gt, err := goTypeFor(argType)
			if err != nil {
				return fmt.Errorf("arg seed %q: %w", seed.Path, err)
			}
			pname := lowerFirst(goName(seed.Path))
			addParam(pname, gt)
			argTypes[pname] = argType
		}
	}
	// Program override may add an extra parameter.
	if pda.Program != nil && pda.Program.Kind == "account" {
		addParam(lowerFirst(goName(pda.Program.Path)), "solana.PublicKey")
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

	// Seed-encoding preamble for arg seeds: one local buffer per unique param.
	argSeedVar := map[string]string{}
	for _, seed := range pda.Seeds {
		if seed.Kind != "arg" {
			continue
		}
		pname := lowerFirst(goName(seed.Path))
		if _, ok := argSeedVar[pname]; ok {
			continue
		}
		seedVar := "_seed_" + pname
		argSeedVar[pname] = seedVar
		fmt.Fprintf(buf, "\tvar %s bytes.Buffer\n", seedVar)
		if err := emitBorshWrite(buf, seedVar, pname, argTypes[pname], "\t", typeDefs); err != nil {
			return fmt.Errorf("encode arg seed %q: %w", seed.Path, err)
		}
	}

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
				continue
			}
			var s string
			if err := json.Unmarshal(seed.Value, &s); err == nil {
				fmt.Fprintf(buf, "\t\t[]byte(%q),\n", s)
				continue
			}
			fmt.Fprintf(buf, "\t\t// unrecognised const seed: %s\n", seed.Value)
			buf.WriteString("\t\t[]byte{},\n")
		case "account":
			pname := lowerFirst(goName(seed.Path))
			fmt.Fprintf(buf, "\t\t%s[:],\n", pname)
		case "arg":
			pname := lowerFirst(goName(seed.Path))
			fmt.Fprintf(buf, "\t\t%s.Bytes(),\n", argSeedVar[pname])
		default:
			fmt.Fprintf(buf, "\t\t// unknown seed kind %q\n", seed.Kind)
			buf.WriteString("\t\t[]byte{},\n")
		}
	}
	buf.WriteString("\t}\n")

	// FindProgramAddress call.
	progExpr, err := pdaProgramExpr(idl, pda)
	if err != nil {
		return err
	}
	fmt.Fprintf(buf, "\treturn solana.FindProgramAddress(seeds, %s)\n", progExpr)
	buf.WriteString("}\n\n")
	return nil
}

// pdaProgramExpr returns the Go expression naming the program ID
// the PDA should be derived under. Precedence: an explicit program
// override in the IDL's PDA definition, then the program's own
// address, then an explanatory placeholder.
func pdaProgramExpr(idl *IDL, pda *PDADef) (string, error) {
	if pda.Program != nil {
		switch pda.Program.Kind {
		case "const":
			// Accept either a base58 string or a byte array.
			var s string
			if err := json.Unmarshal(pda.Program.Value, &s); err == nil && s != "" {
				return fmt.Sprintf("solana.MustPublicKeyFromBase58(%q)", s), nil
			}
			var vals []int
			if err := json.Unmarshal(pda.Program.Value, &vals); err == nil && len(vals) == 32 {
				var b strings.Builder
				b.WriteString("solana.PublicKey{")
				for i, v := range vals {
					if i > 0 {
						b.WriteString(", ")
					}
					fmt.Fprintf(&b, "0x%02x", v)
				}
				b.WriteString("}")
				return b.String(), nil
			}
			return "", fmt.Errorf("pda program: unrecognised const value %s", pda.Program.Value)
		case "account":
			return lowerFirst(goName(pda.Program.Path)), nil
		default:
			return "", fmt.Errorf("pda program: unsupported kind %q", pda.Program.Kind)
		}
	}
	if idl.Address != "" {
		return "ProgramAddress", nil
	}
	return "solana.PublicKey{} /* set program address */", nil
}
