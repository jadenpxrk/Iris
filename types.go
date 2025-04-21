package main

import "io/fs"

// FileInfo holds information about a processed file.
type FileInfo struct {
	Path       string
	Size       int64
	Mode       fs.FileMode
	Content    []byte // Content might be loaded conditionally based on output format
	TokenCount int    // Populated if token counting is enabled
	IsDir      bool   // Indicates if this is a directory entry
	Error      error  // Stores any error encountered while processing this file/dir
}

// Summary holds aggregated information about the processed items.
type Summary struct {
	TotalFiles  int
	TotalSize   int64
	TotalTokens int
}

// ProcessedItem represents either a FileInfo or a directory structure node.
// This could evolve as we implement tree/file output. For now, FileInfo covers both files and directory entries discovered during traversal.
// We might need a separate DirectoryInfo later if we store nested structures.
