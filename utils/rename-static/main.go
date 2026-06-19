package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	filePath := flag.String("file", "", "Path to the file to rename (required)")
	hashLength := flag.Int("n", 8, "Number of hash characters to include")
	hashType := flag.String("hash", "sha256", "Hash algorithm to use (md5, sha1, sha256, sha512)")

	flag.Parse()

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "Error: -file parameter is required.")
		flag.Usage()
		os.Exit(1)
	}

	if *hashLength <= 0 {
		fmt.Fprintln(os.Stderr, "Error: -n (length) must be greater than 0.")
		os.Exit(1)
	}

	file, err := os.Open(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var hasher hash.Hash
	switch strings.ToLower(*hashType) {
	case "md5":
		hasher = md5.New()
	case "sha1":
		hasher = sha1.New()
	case "sha256":
		hasher = sha256.New()
	case "sha512":
		hasher = sha512.New()
	default:
		fmt.Fprintf(os.Stderr, "Error: Unsupported hash type '%s'. Choose from md5, sha1, sha256, sha512.\n", *hashType)
		os.Exit(1)
	}

	if _, err := io.Copy(hasher, file); err != nil {
		fmt.Fprintf(os.Stderr, "Error calculating hash: %v\n", err)
		os.Exit(1)
	}

	file.Close()

	fullHash := fmt.Sprintf("%x", hasher.Sum(nil))

	if *hashLength > len(fullHash) {
		*hashLength = len(fullHash)
	}
	shortHash := fullHash[:*hashLength]

	dir := filepath.Dir(*filePath)
	base := filepath.Base(*filePath)
	ext := filepath.Ext(base)                       // e.g., ".txt"
	nameWithoutExt := strings.TrimSuffix(base, ext) // e.g., "report"

	newBase := fmt.Sprintf("%s.%s%s", nameWithoutExt, shortHash, ext)
	newPath := filepath.Join(dir, newBase)

	err = os.Rename(*filePath, newPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error renaming file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully renamed to: %s\n", newPath)
}
