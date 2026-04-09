// gen_large generates testdata/large.n4l: a valid N4L file with exactly 10,000 lines
// used as the benchmark fixture for ParseFile performance testing.
//
// Structure:
//   - One chapter header (line 1)
//   - 2,500 node+ditto pairs, each node followed by one ditto arrow line
//   - :: context operator every 100 nodes (resetting context)
//   - One _sequence_ context block of 20 nodes at nodes 1000–1019
//
// Run: go run ./internal/content/testdata/gen_large/
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func main() {
	_, file, _, _ := runtime.Caller(0)
	outPath := filepath.Join(filepath.Dir(file), "..", "large.n4l")

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	// Line 1: chapter header
	fmt.Fprintln(w, "- large benchmark chapter")

	lineCount := 1
	nodeIdx := 0

	// We target 10,000 lines total.
	// Layout per 100-node block (100 nodes):
	//   1 ":: context N ::" reset line  → 1 line
	//   100 nodes × 2 lines each        → 200 lines
	//   total per block                 → 201 lines
	// Except the _sequence_ block at nodes 1000–1019 which has an extra ":: _sequence_ ::" and ":: ::" line.

	inSequence := false
	sequenceEmitted := false

	for nodeIdx < 2500 && lineCount < 10000 {
		// Every 100 nodes emit a context reset
		if nodeIdx%100 == 0 {
			if lineCount >= 10000 {
				break
			}
			fmt.Fprintf(w, ":: context%d\n", nodeIdx/100)
			lineCount++
		}

		// Emit _sequence_ block for nodes 1000–1019
		if nodeIdx == 1000 && !sequenceEmitted {
			if lineCount >= 10000 {
				break
			}
			fmt.Fprintln(w, "+:: _sequence_")
			lineCount++
			inSequence = true
		}
		if nodeIdx == 1020 && inSequence {
			if lineCount >= 10000 {
				break
			}
			fmt.Fprintln(w, "-:: _sequence_")
			lineCount++
			inSequence = false
			sequenceEmitted = true
		}

		// Node line
		if lineCount >= 10000 {
			break
		}
		fmt.Fprintf(w, "Concept number %d explains an important idea\n", nodeIdx)
		lineCount++
		nodeIdx++

		// Ditto line
		if lineCount >= 10000 {
			break
		}
		fmt.Fprintf(w, "  (e.g.) example for concept %d\n", nodeIdx-1)
		lineCount++
	}

	// Pad to exactly 10,000 lines if needed (should not be needed with the math above)
	for lineCount < 10000 {
		fmt.Fprintf(w, "Padding node %d\n", lineCount)
		lineCount++
	}

	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "flush error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s (%d lines)\n", outPath, lineCount)
}
