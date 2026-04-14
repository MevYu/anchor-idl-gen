// Command anchoridl-gen generates Go bindings from an Anchor IDL
// JSON file. It is the code generator that gives solana-go its
// Anchor program ecosystem moat: any program whose IDL is published
// can be bound in minutes, with generated code that uses zero
// reflection and fits the rest of solana-go's type conventions
// (typed Instruction builders, typed result structs).
//
// Usage:
//
//	anchoridl-gen -idl path/to/program.json -out programs/foo [-package foo]
//
// The output directory will contain:
//
//	types.go         // defined struct and enum types
//	accounts.go      // account discriminators and match helper
//	instructions.go  // instruction builders (Borsh-encoded Data())
//	errors.go        // error code sentinels
//	events.go        // event structs with discriminators
//	pda.go           // PDA derivation helpers
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var (
		idlPath = flag.String("idl", "", "path to the Anchor IDL JSON file")
		outDir  = flag.String("out", "", "output directory for generated Go code")
		pkgName = flag.String("package", "", "Go package name for generated code (defaults to the IDL program name)")
	)
	flag.Parse()

	if *idlPath == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "usage: anchoridl-gen -idl <path> -out <dir> [-package <name>]")
		os.Exit(2)
	}

	if err := run(*idlPath, *outDir, *pkgName); err != nil {
		fmt.Fprintf(os.Stderr, "anchoridl-gen: %v\n", err)
		os.Exit(1)
	}
}

func run(idlPath, outDir, pkgName string) error {
	idl, err := LoadIDL(idlPath)
	if err != nil {
		return err
	}
	if pkgName == "" {
		pkgName = idl.Metadata.Name
		if pkgName == "" {
			pkgName = idl.Name
		}
	}
	if pkgName == "" {
		return fmt.Errorf("package name not set and IDL has no name/metadata.name")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	fmt.Fprintf(os.Stderr, "anchoridl-gen: loaded IDL %q\n", pkgName)
	fmt.Fprintf(os.Stderr, "  instructions: %d\n", len(idl.Instructions))
	fmt.Fprintf(os.Stderr, "  accounts:     %d\n", len(idl.Accounts))
	fmt.Fprintf(os.Stderr, "  types:        %d\n", len(idl.Types))
	fmt.Fprintf(os.Stderr, "  events:       %d\n", len(idl.Events))
	fmt.Fprintf(os.Stderr, "  errors:       %d\n", len(idl.Errors))

	generators := []struct {
		file string
		fn   func(*os.File) error
	}{
		{"types.go", func(w *os.File) error { return GenerateTypes(w, pkgName, idl) }},
		{"accounts.go", func(w *os.File) error { return GenerateAccounts(w, pkgName, idl) }},
		{"instructions.go", func(w *os.File) error { return GenerateInstructions(w, pkgName, idl) }},
		{"errors.go", func(w *os.File) error { return GenerateErrors(w, pkgName, idl) }},
		{"events.go", func(w *os.File) error { return GenerateEvents(w, pkgName, idl) }},
		{"pda.go", func(w *os.File) error { return GeneratePDA(w, pkgName, idl) }},
	}
	for _, g := range generators {
		if err := emitFile(outDir, g.file, g.fn); err != nil {
			return fmt.Errorf("generate %s: %w", g.file, err)
		}
	}
	return nil
}

// emitFile creates outDir/name and hands it to gen. It takes care
// of closing the file and reporting the write path on stderr.
func emitFile(outDir, name string, gen func(*os.File) error) error {
	path := filepath.Join(outDir, name)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := gen(f); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return nil
}
