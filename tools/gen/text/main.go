package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultDirMode = 0o755
	minVal         = 32
	maxVal         = 126
)

func main() {
	bytes := rand.IntN(1024 * 1024)
	count := 1
	outDir := "."

	flag.IntVar(&bytes, "bytes", bytes, "Text file size (byte)")
	flag.IntVar(&count, "count", count, "Number of images to generate")
	flag.StringVar(&outDir, "out", outDir, "Output directory")
	flag.Parse()

	if err := os.MkdirAll(outDir, defaultDirMode); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	for i := range count {
		filename := filepath.Join(outDir, fmt.Sprintf("text_%d_%d.txt", i, bytes))
		err := generateText(filename, bytes)
		if err != nil {
			fmt.Printf("Error generating text %s: %v\n", filename, err)
		} else {
			fmt.Printf("Generated %s\n", filename)
		}
	}
}

func generateText(filename string, bytes int) error {
	sb := strings.Builder{}

	for range bytes {
		r := minVal + rand.Int32N(maxVal-minVal+1)
		sb.WriteRune(r)
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(sb.String())

	return err
}
