// LLM usage: the helpz utility is generated with deepseek-v4-pro and modified manually.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	srcDir := flag.String("src", "", "Source directory containing zdoc documents (e.g., zdoc/global)")
	outDir := flag.String("out", "", "Output directory for generated Go HTML templates")
	flag.Parse()

	if *srcDir == "" || *outDir == "" {
		fmt.Fprintf(os.Stderr, "Usage: helpz-gen -src <source-directory> -out <output-directory>\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Validate source directory
	if info, err := os.Stat(*srcDir); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: source directory %q does not exist or is not a directory\n", *srcDir)
		os.Exit(1)
	}

	if err := generate(*srcDir, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
