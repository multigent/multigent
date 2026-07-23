package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func readJSONFile(path string, v any) error {
	if path == "" {
		return fmt.Errorf("--file is required")
	}
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	}
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSONFile(path string, v any) error {
	if path == "" || path == "-" {
		return printJSON(v)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
