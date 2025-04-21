package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/monochromegane/go-gitignore"
)

// processLocalPath handles a single local file or directory path.
// It now accepts LoadedLanguageData for filtering.
func processLocalPath(path string, langData *LoadedLanguageData) ([]FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("error accessing path %s: %w", path, err)
	}

	var files []FileInfo

	if info.IsDir() {
		// It's a directory, start walking
		fmt.Printf("Processing directory: %s\n", path) // Placeholder
		// Pass langData to walkDirectory
		files, err = walkDirectory(path, langData)
		if err != nil {
			return nil, err
		}
	} else {
		// It's a single file
		fmt.Printf("Processing file: %s\n", path) // Placeholder
		// Apply filters even for single files, passing langData
		keep, err := shouldKeepFile(path, info, langData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: error checking file %s: %v\n", path, err)
			// Decide if we should error out or just skip
		} else if keep {
			fileInfo := FileInfo{
				Path:  path,
				Size:  info.Size(),
				Mode:  info.Mode(),
				IsDir: false,
			}
			files = append(files, fileInfo)
		} else {
			fmt.Printf("Skipping single file due to filters: %s\n", path)
		}
	}

	return files, nil
}

// parsePatterns splits a comma-separated string of patterns into a slice.
func parsePatterns(patterns string) []string {
	if patterns == "" {
		return nil
	}
	return strings.Split(patterns, ",")
}

// matchesAnyPattern checks if the given name matches any of the provided glob patterns.
func matchesAnyPattern(name string, patterns []string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, name)
		if err != nil {
			return false, fmt.Errorf("invalid glob pattern '%s': %w", pattern, err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

// walkDirectory recursively walks a directory, respecting filters and .gitignore.
// It now accepts LoadedLanguageData for filtering.
func walkDirectory(root string, langData *LoadedLanguageData) ([]FileInfo, error) {
	var files []FileInfo
	var ignoreMatcher gitignore.IgnoreMatcher

	parsedIncludes := parsePatterns(includePatterns)
	parsedExcludes := parsePatterns(excludePatterns)
	// Check if explicit includes were provided. If not, language filtering might apply.
	hasExplicitIncludes := len(parsedIncludes) > 0

	if !noIgnore {
		// TODO: Consider handling nested .gitignore files?
		// go-gitignore primarily works with one .gitignore at the root level of the match.
		// For full git compatibility, might need a more complex walker or library.
		gitIgnorePath := filepath.Join(root, ".gitignore")
		if _, err := os.Stat(gitIgnorePath); err == nil {
			matcher, err := gitignore.NewGitIgnore(gitIgnorePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not parse .gitignore file %s: %v\n", gitIgnorePath, err)
			} else {
				ignoreMatcher = matcher
			}
		}
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: error accessing path %s: %v\n", path, err)
			// Optionally return err to stop walk, or fs.SkipDir for directory errors?
			return nil // Report and continue
		}

		// Skip root directory itself
		if path == root {
			return nil
		}

		// --- Filtering Logic ---
		baseName := d.Name()
		isDir := d.IsDir()

		// 1. Hidden Files/Dirs
		if !showHidden && isHidden(baseName) {
			if isDir {
				return fs.SkipDir
			}
			return nil
		}

		// 2. .gitignore
		// Need the path relative to the gitignore file (usually the root)
		relPathForIgnore, _ := filepath.Rel(root, path)
		if ignoreMatcher != nil && ignoreMatcher.Match(relPathForIgnore, isDir) {
			if isDir {
				return fs.SkipDir
			}
			return nil
		}

		// 3. Max Depth
		relPath, _ := filepath.Rel(root, path)
		currentDepth := countPathSeparators(relPath)
		if maxDepth > 0 && currentDepth >= maxDepth {
			if isDir {
				return fs.SkipDir // Reached max depth, skip this directory
			}
			// If it's a file at max depth, it might still be processed below
		}

		// Apply Include/Exclude/Language Filters
		// If it's a directory, we check excludes but not includes/language yet (allow traversal)
		if isDir {
			// 4a. Exclude Pattern Match (Directories)
			excluded, err := matchesAnyPattern(baseName, parsedExcludes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: error in exclude pattern matching for %s: %v\n", path, err)
				// Decide how to handle pattern errors - skip file or ignore pattern?
			}
			if excluded {
				return fs.SkipDir // Skip excluded directories
			}
			// Allow traversal of non-excluded directories
		} else {
			// Apply full filters to files
			fileName := baseName

			// 4a. Exclude Pattern Match (Files)
			excluded, err := matchesAnyPattern(fileName, parsedExcludes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: error in exclude pattern matching for %s: %v\n", path, err)
				// Decide how to handle pattern errors - skip file or ignore pattern?
			}
			if excluded {
				return nil // Skip excluded files
			}

			// 4b. Include Pattern Match OR Language Match (Files)
			keepFile := false
			if hasExplicitIncludes {
				// If includes are specified, use them
				included, err := matchesAnyPattern(fileName, parsedIncludes)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: error in include pattern matching for %s: %v\n", path, err)
				}
				if included {
					keepFile = true
				}
			} else if langData != nil {
				// If no includes specified AND langData exists, check language
				if _, knownLang := langData.GetLanguageForFile(path); knownLang {
					keepFile = true
				}
			} else {
				// No includes, no langData -> keep all non-excluded files
				keepFile = true
			}

			if !keepFile {
				return nil // Skip files not matching includes or known languages (if applicable)
			}

			// 5. Max Size (apply only to files)
			var fileSize int64
			var fileMode fs.FileMode
			info, err := d.Info()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not get info for %s: %v\n", path, err)
				return nil // Skip file if info error
			}
			fileSize = info.Size()
			fileMode = info.Mode()
			if maxSizeBytes > 0 && fileSize > maxSizeBytes {
				return nil // Skip large files
			}

			// If file passes all filters, add it
			fileInfo := FileInfo{
				Path:  path,
				Size:  fileSize,
				Mode:  fileMode,
				IsDir: false,
			}
			files = append(files, fileInfo)
		}
		// --- End Filtering Logic ---

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory %s: %w", root, err)
	}

	return files, nil
}

// shouldKeepFile checks if a single file (not in a walk) should be kept based on filters.
// It now accepts LoadedLanguageData for filtering.
func shouldKeepFile(path string, info fs.FileInfo, langData *LoadedLanguageData) (bool, error) {
	baseName := info.Name()

	// Hidden
	if !showHidden && isHidden(baseName) {
		return false, nil
	}

	// Gitignore - less relevant for single file args unless we load a relevant .gitignore?
	// Glimpse probably doesn't apply gitignore to explicit file args.

	// Include/Exclude/Language
	parsedIncludes := parsePatterns(includePatterns)
	parsedExcludes := parsePatterns(excludePatterns)
	hasExplicitIncludes := len(parsedIncludes) > 0

	excluded, err := matchesAnyPattern(baseName, parsedExcludes)
	if err != nil {
		return false, fmt.Errorf("exclude pattern error: %w", err)
	}
	if excluded {
		return false, nil
	}

	keepFile := false
	if hasExplicitIncludes {
		included, err := matchesAnyPattern(baseName, parsedIncludes)
		if err != nil {
			return false, fmt.Errorf("include pattern error: %w", err)
		}
		if included {
			keepFile = true
		}
	} else if langData != nil {
		if _, knownLang := langData.GetLanguageForFile(path); knownLang {
			keepFile = true
		}
	} else {
		keepFile = true // Keep if not excluded and no includes/lang specified
	}
	if !keepFile {
		return false, nil
	}

	// Max Size
	if maxSizeBytes > 0 && info.Size() > maxSizeBytes {
		return false, nil
	}

	return true, nil
}

// isHidden checks if a file path is hidden (starts with '.').
func isHidden(path string) bool {
	// Check for '.' or '..'
	if path == "." || path == ".." {
		return false
	}
	// Check if the actual base name starts with '.'
	baseName := filepath.Base(path)
	return len(baseName) > 0 && baseName[0] == '.'
}

// countPathSeparators counts the number of path separators in a relative path.
func countPathSeparators(path string) int {
	// Normalize path separators for consistency
	path = filepath.ToSlash(path)
	if path == "." || path == "" {
		return 0
	}
	// Don't count trailing slash if present
	return strings.Count(strings.Trim(path, "/"), "/")
}
