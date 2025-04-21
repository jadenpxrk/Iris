package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
)

// processWebURLRecursive fetches content from a starting URL, converts it to Markdown,
// finds links, and recursively processes them up to maxDepth.
// It keeps track of visited URLs to avoid loops.
func processWebURLRecursive(startURL string, currentDepth, maxDepth int, visited map[string]bool) ([]FileInfo, error) {
	// Clean URL to avoid re-visiting due to fragments or slight variations
	parsedURL, err := url.Parse(startURL)
	if err != nil {
		return nil, fmt.Errorf("invalid start URL %s: %w", startURL, err)
	}
	parsedURL.Fragment = "" // Ignore fragments
	cleanURL := parsedURL.String()

	if currentDepth > maxDepth {
		fmt.Printf("Max depth (%d) reached, not processing: %s\n", maxDepth, cleanURL)
		return nil, nil
	}
	if visited[cleanURL] {
		fmt.Printf("Already visited, skipping: %s\n", cleanURL)
		return nil, nil
	}

	visited[cleanURL] = true
	fmt.Printf("Processing web URL (Depth %d): %s\n", currentDepth, cleanURL)

	// --- Fetch and Process Current URL ---
	res, err := http.Get(cleanURL)
	if err != nil {
		// Log error but continue traversal if possible? Or stop?
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch URL %s: %v\n", cleanURL, err)
		return nil, nil // Skip this URL and its links on fetch error
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch URL %s: status code %d\n", cleanURL, res.StatusCode)
		return nil, nil // Skip this URL
	}

	// Check content type - only parse HTML
	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "text/html") {
		fmt.Printf("Skipping non-HTML content type (%s) for URL: %s\n", contentType, cleanURL)
		return nil, nil
	}

	// Read the response body
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read response body from %s: %v\n", cleanURL, err)
		return nil, nil // Skip this URL
	}
	// --- End Fetch ---

	// --- Convert Current Page to Markdown ---
	converter := md.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(string(bodyBytes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to convert HTML to Markdown for %s: %v\n", cleanURL, err)
		// Create FileInfo with raw HTML or skip?
		// Let's skip creating FileInfo if conversion fails, but still parse for links below.
	}

	var currentFiles []FileInfo
	if err == nil { // Only add FileInfo if conversion was successful
		fileInfo := FileInfo{
			Path:    cleanURL, // Use the cleaned URL
			Content: []byte(markdown),
			Size:    int64(len(markdown)),
			IsDir:   false,
		}
		currentFiles = append(currentFiles, fileInfo)
		fmt.Printf("Finished processing web URL: %s (Markdown size: %d bytes)\n", cleanURL, fileInfo.Size)
	}
	// --- End Conversion ---

	// --- Find and Process Links (if not at max depth) ---
	if currentDepth < maxDepth {
		// Use goquery to parse the original HTML body bytes
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(bodyBytes)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse HTML for link extraction from %s: %v\n", cleanURL, err)
		} else {
			doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
				link, exists := s.Attr("href")
				if !exists || link == "" || strings.HasPrefix(link, "#") || strings.HasPrefix(strings.ToLower(link), "mailto:") || strings.HasPrefix(strings.ToLower(link), "javascript:") {
					return // Skip empty, fragment, mailto, or javascript links
				}

				// Resolve the link relative to the current page's URL
				resolvedURL, err := parsedURL.Parse(link)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not resolve relative link '%s' on page %s: %v\n", link, cleanURL, err)
					return
				}

				// Only process HTTP/HTTPS URLs
				if resolvedURL.Scheme == "http" || resolvedURL.Scheme == "https://" {
					// Recursively process the resolved link
					linkedFiles, _ := processWebURLRecursive(resolvedURL.String(), currentDepth+1, maxDepth, visited)
					// We ignore the error from the recursive call to continue processing other links
					currentFiles = append(currentFiles, linkedFiles...)
				}
			})
		}
	}
	// --- End Link Processing ---

	return currentFiles, nil
}

// processWebURL remains as a simple, non-recursive entry point if needed,
// but the main logic will likely call processWebURLRecursive directly.
func processWebURL(url string) (FileInfo, error) {
	visited := make(map[string]bool)
	results, err := processWebURLRecursive(url, 0, 0, visited) // Call recursive with maxDepth 0
	if err != nil {
		return FileInfo{}, err
	}
	if len(results) == 0 {
		// This might happen if the initial URL fetch failed or conversion failed
		// Return an error consistent with previous behavior?
		return FileInfo{}, fmt.Errorf("failed to process web URL %s (no content generated)", url)
	}
	return results[0], nil // Return the first result (the page itself)
}
