package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// isGitURL checks if the input string looks like a Git repository URL.
// Prioritizes .git suffix or git@ prefix.
func isGitURL(input string) bool {
	// Check for common Git URL schemes and the .git suffix
	return strings.HasSuffix(input, ".git") ||
		strings.HasPrefix(input, "git@") // Common SSH format
	// Could add ssh:// but less common for direct user input
	// Don't check for https:// or http:// by default as they are ambiguous
}

// cloneGitRepo clones a Git repository URL into a temporary directory.
// It returns the path to the temporary directory or an error.
func cloneGitRepo(url string) (string, error) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "iris-git-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	fmt.Printf("Cloning Git repository '%s' into '%s'...\n", url, tempDir)

	// Clone the repository
	_, err = git.PlainClone(tempDir, false, &git.CloneOptions{
		URL:      url,
		Progress: os.Stdout, // Show progress during clone
		// Depth: 1, // Optional: shallow clone for faster download if history isn't needed
		ReferenceName: plumbing.HEAD, // Checkout default branch
		SingleBranch:  true,          // Only fetch the default branch
	})

	if err != nil {
		// Attempt cleanup even if clone failed
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to clone repository '%s': %w", url, err)
	}

	fmt.Printf("Finished cloning '%s'.\n", url)
	return tempDir, nil
}
