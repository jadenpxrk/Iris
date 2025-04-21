package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	fuzzyfinder "github.com/ktr0731/go-fuzzyfinder"
)

// runInteractiveFinder finds files/dirs and uses a fuzzy finder for selection.
func runInteractiveFinder() ([]string, error) {
	// 1. Find candidates: Walk current dir, apply basic filters (hidden, maybe gitignore?)
	// For simplicity, let's start with a basic walk respecting --hidden.
	// We won't apply include/exclude/size here, let the user pick first.
	candidates := []string{}
	root := "." // Start from current directory

	// We need a simplified walk just to get paths for the finder
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Silently ignore errors during candidate finding?
			// Or print warnings?
			// fmt.Fprintf(os.Stderr, "Warning (interactive scan): error accessing %s: %v\n", path, err)
			return nil // Continue walking
		}

		// Skip root
		if path == root {
			return nil
		}

		// Basic Hidden File Filter (respecting flag)
		if !showHidden && isHidden(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// TODO: Optionally add .gitignore filtering here for a cleaner list?
		// Requires loading gitignore from "."

		candidates = append(candidates, path)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error scanning for files/directories: %w", err)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no files or directories found to select from")
	}

	// 2. Run Fuzzy Finder
	idx, err := fuzzyfinder.FindMulti(
		candidates,
		func(i int) string {
			return candidates[i] // Display the path itself
		},
		fuzzyfinder.WithPreviewWindow(func(i, w, h int) string {
			if i == -1 { // No selection yet
				return "Select files or directories to process. Press Tab to multi-select, Enter to confirm."
			}
			// Basic preview: show file type and size
			path := candidates[i]
			info, statErr := os.Stat(path)
			if statErr != nil {
				return fmt.Sprintf("Path: %s\nError getting info: %v", path, statErr)
			}
			fileType := "File"
			if info.IsDir() {
				fileType = "Directory"
			}
			return fmt.Sprintf("Path: %s\nType: %s\nSize: %d bytes", path, fileType, info.Size())
		}),
	)

	if err != nil {
		if err == fuzzyfinder.ErrAbort { // User pressed Esc or Ctrl+C
			fmt.Println("Interactive selection aborted.")
			return nil, nil // Return nil slice and nil error to indicate graceful exit
		}
		return nil, fmt.Errorf("fuzzy finder error: %w", err)
	}

	selectedPaths := make([]string, len(idx))
	for i, index := range idx {
		selectedPaths[i] = candidates[index]
	}

	return selectedPaths, nil
}
