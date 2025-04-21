package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Node represents an entry in the directory tree structure.
type Node struct {
	Name     string
	Path     string
	IsDir    bool
	Size     int64 // Relevant for files
	Children []*Node
}

// buildTree constructs a hierarchical tree from a flat list of FileInfo.
func buildTree(files []FileInfo, rootPath string) *Node {
	// Normalize the root path
	cleanRootPath := filepath.Clean(rootPath)
	root := &Node{Name: filepath.Base(cleanRootPath), Path: cleanRootPath, IsDir: true}
	nodes := make(map[string]*Node)
	nodes[cleanRootPath] = root

	// Sort files by path to ensure parents are processed before children
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	for _, file := range files {
		cleanPath := filepath.Clean(file.Path)
		parentDir := filepath.Dir(cleanPath)
		baseName := filepath.Base(cleanPath)

		parentNode, exists := nodes[parentDir]
		if !exists {
			// This might happen if a directory itself was filtered out, but not its contents.
			// Or if the input list doesn't represent a fully connected tree.
			// For simplicity, let's assume parents exist or create intermediate ones if necessary.
			// A more robust approach might be needed depending on filtering behavior.
			fmt.Fprintf(os.Stderr, "Warning: Parent node not found for %s, skipping in tree view.\n", cleanPath)
			continue
			// Alternatively, create missing parent nodes recursively upwards?
		}

		node := &Node{
			Name:  baseName,
			Path:  cleanPath,
			IsDir: file.IsDir,
			Size:  file.Size,
		}

		parentNode.Children = append(parentNode.Children, node)
		if file.IsDir {
			nodes[cleanPath] = node // Register directory node for future children
		}
	}

	// Sort children alphabetically within each node
	sortChildren(root)

	return root
}

// sortChildren recursively sorts the children of a node alphabetically.
func sortChildren(node *Node) {
	if !node.IsDir || len(node.Children) == 0 {
		return
	}

	sort.Slice(node.Children, func(i, j int) bool {
		// Optional: Sort directories before files
		// if node.Children[i].IsDir != node.Children[j].IsDir {
		//  return node.Children[i].IsDir // true (dir) comes before false (file)
		// }
		return node.Children[i].Name < node.Children[j].Name
	})

	for _, child := range node.Children {
		sortChildren(child)
	}
}

// printTree generates the string representation of the tree.
func printTree(root *Node) string {
	var builder strings.Builder
	// Print root name separately, then start recursion for children
	builder.WriteString(root.Name)
	builder.WriteString("\n")
	printNode(&builder, root.Children, "")
	return builder.String()
}

// printNode is a helper function for recursively printing tree nodes.
func printNode(builder *strings.Builder, children []*Node, prefix string) {
	for i, node := range children {
		connector := "├── "
		newPrefix := prefix + "│   "
		if i == len(children)-1 {
			connector = "└── "
			newPrefix = prefix + "    "
		}

		builder.WriteString(prefix)
		builder.WriteString(connector)
		builder.WriteString(node.Name)
		// Optionally add size or other info here
		// if !node.IsDir {
		//  builder.WriteString(fmt.Sprintf(" (%d bytes)", node.Size))
		// }
		builder.WriteString("\n")

		if node.IsDir && len(node.Children) > 0 {
			printNode(builder, node.Children, newPrefix)
		}
	}
}

// printFiles generates the string representation for the 'files' output format.
func printFiles(files []FileInfo, includeTokens bool) string {
	var builder strings.Builder
	sort.Slice(files, func(i, j int) bool { // Sort by path for consistent output
		return files[i].Path < files[j].Path
	})

	for _, file := range files {
		if file.IsDir {
			continue // Skip directories for 'files' output format
		}

		builder.WriteString(fmt.Sprintf("File: %s\n", file.Path))
		if includeTokens {
			if file.Error != nil {
				builder.WriteString(fmt.Sprintf("Tokens: Error (%v)\n", file.Error)) // Indicate error during token count
			} else {
				builder.WriteString(fmt.Sprintf("Tokens: %d\n", file.TokenCount))
			}
		}
		builder.WriteString(strings.Repeat("=", 50))
		builder.WriteString("\n")

		// Read file content OR use pre-loaded web content
		var contentToPrint []byte
		var readErr error
		if file.Content != nil { // Use content if already loaded (from web)
			contentToPrint = file.Content
		} else { // Otherwise, read from disk
			contentToPrint, readErr = os.ReadFile(file.Path)
		}

		if readErr != nil {
			// If token counting failed due to read error, file.Error might already be set.
			// We still report the error here in the content section.
			builder.WriteString(fmt.Sprintf("Error reading file: %v\n", readErr))
		} else {
			builder.Write(contentToPrint)
		}

		// Ensure consistent line breaks after content
		if len(contentToPrint) > 0 && contentToPrint[len(contentToPrint)-1] != '\n' {
			builder.WriteString("\n")
		}
		builder.WriteString("\n") // Add blank line between files
	}
	return builder.String()
}
