package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadIDL reads and parses an Anchor IDL JSON file from disk.
func LoadIDL(path string) (*IDL, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read idl %q: %w", path, err)
	}
	var idl IDL
	if err := json.Unmarshal(data, &idl); err != nil {
		return nil, fmt.Errorf("parse idl %q: %w", path, err)
	}
	return &idl, nil
}
