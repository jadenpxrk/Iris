package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/jung-kurt/gofpdf"
)

const (
	pdfPageWidth  = 210 // A4 width in mm
	pdfPageHeight = 297 // A4 height in mm
	pdfMargin     = 10  // Margin in mm
	pdfLineHeight = 5   // Line height in mm
	pdfFontSize   = 9   // Reduced font size slightly for better code fit
	pdfTabWidth   = 4   // Number of spaces for a tab
)

// generatePDF takes the collected FileInfo and Summary, generates syntax-highlighted
// PDF output according to the selected format (tree, files, both).
func generatePDF(files []FileInfo, summary Summary, outputFormat string, langData *LoadedLanguageData, outputPath string) error {
	fmt.Printf("Generating PDF output at: %s (Format: %s)\n", outputPath, outputFormat)

	pdf := gofpdf.New("P", "mm", "A4", "") // Portrait, mm, A4, default font dir
	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.SetAutoPageBreak(true, pdfMargin) // Enable auto page breaks
	pdf.AddPage()

	// Select a Chroma style
	// style := styles.Get("monokai") // Example: Monokai
	style := styles.Get("github") // Example: GitHub style
	if style == nil {
		style = styles.Fallback
	}

	// --- Output Tree (if requested) ---
	if outputFormat == "tree" || outputFormat == "both" {
		// For the tree, we don't have syntax highlighting, just print the tree structure.
		// Assuming the first input was a directory if tree format makes sense.
		// Note: This assumes tree generation is still relevant based on input types.
		// We might need to adjust how the tree string is generated/passed.
		// Let's rebuild the tree string here for simplicity.
		treeString := buildTreeString(files) // Need a helper to get tree string

		pdf.SetFont("Courier", "", pdfFontSize)
		pdf.SetTextColor(0, 0, 0)
		pdf.MultiCell(pdfPageWidth-2*pdfMargin, pdfLineHeight, treeString, "", "L", false)
		pdf.Ln(pdfLineHeight)
	}

	// --- Output Files (if requested) ---
	if outputFormat == "files" || outputFormat == "both" {
		// Sort files for consistent output
		sort.Slice(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})

		for _, file := range files {
			if file.IsDir {
				continue
			}

			// Add File Header
			pdf.SetFont("Helvetica", "B", pdfFontSize+1)
			pdf.SetTextColor(0, 0, 0)
			pdf.MultiCell(pdfPageWidth-2*pdfMargin, pdfLineHeight, fmt.Sprintf("File: %s", file.Path), "", "L", false)
			pdf.Ln(pdfLineHeight / 2)

			// Add Token Count if available
			if file.TokenCount > 0 || file.Error != nil { // Assuming TokenCount is populated
				pdf.SetFont("Helvetica", "", pdfFontSize-1)
				tokenStr := ""
				if file.Error != nil {
					tokenStr = fmt.Sprintf("Tokens: Error (%v)", file.Error)
				} else {
					tokenStr = fmt.Sprintf("Tokens: %d", file.TokenCount)
				}
				pdf.MultiCell(pdfPageWidth-2*pdfMargin, pdfLineHeight, tokenStr, "", "L", false)
				pdf.Ln(pdfLineHeight / 2)
			}

			pdf.Line(pdfMargin, pdf.GetY(), pdfPageWidth-pdfMargin, pdf.GetY()) // Separator line
			pdf.Ln(pdfLineHeight / 2)

			// Get content (read again or use pre-loaded if available)
			var content []byte
			var readErr error
			if file.Content != nil {
				content = file.Content
			} else {
				content, readErr = os.ReadFile(file.Path)
			}

			if readErr != nil {
				pdf.SetFont("Courier", "", pdfFontSize)
				pdf.SetTextColor(255, 0, 0) // Red for error
				pdf.MultiCell(pdfPageWidth-2*pdfMargin, pdfLineHeight, fmt.Sprintf("Error reading file: %v", readErr), "", "L", false)
			} else {
				// Perform Syntax Highlighting
				err := writeHighlightedCode(pdf, style, string(content), file.Path, langData)
				if err != nil {
					// Fallback to plain text if highlighting fails?
					fmt.Fprintf(os.Stderr, "Warning: Syntax highlighting failed for %s: %v. Writing plain text.\n", file.Path, err)
					pdf.SetFont("Courier", "", pdfFontSize)
					pdf.SetTextColor(0, 0, 0)
					pdf.MultiCell(pdfPageWidth-2*pdfMargin, pdfLineHeight, string(content), "", "L", false)
				}
			}
			pdf.AddPage() // Start each new file on a new page (or manage breaks better)
		}
	}

	// --- Output Summary ---
	// Ensure we are on the last page or add a new one if needed
	// (Complex page management might be needed if files output is long)
	pdf.SetFont("Helvetica", "B", pdfFontSize+1)
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(pdfLineHeight)
	pdf.MultiCell(pdfPageWidth-2*pdfMargin, pdfLineHeight, "--- Summary ---", "", "L", false)
	pdf.Ln(pdfLineHeight / 2)

	pdf.SetFont("Helvetica", "", pdfFontSize)
	summaryString := fmt.Sprintf("Total files processed: %d\nTotal size: %d bytes", summary.TotalFiles, summary.TotalSize)
	if summary.TotalTokens > 0 { // Assuming token counting wasn't disabled
		summaryString += fmt.Sprintf("\nTotal tokens: %d", summary.TotalTokens)
	}
	// Add failed paths count? summary doesn't store it currently.
	pdf.MultiCell(pdfPageWidth-2*pdfMargin, pdfLineHeight, summaryString, "", "L", false)

	// --- Save PDF ---
	err := pdf.OutputFileAndClose(outputPath)
	if err != nil {
		return fmt.Errorf("failed to save PDF to %s: %w", outputPath, err)
	}

	fmt.Printf("Successfully saved PDF to %s\n", outputPath)
	return nil
}

// buildTreeString creates the simple text representation of the file tree.
// This is a placeholder - ideally uses the same logic as printTree in output.go
func buildTreeString(files []FileInfo) string {
	// This is inefficient - ideally, output.go provides this directly
	// For now, let's just list files as a placeholder
	var builder strings.Builder
	builder.WriteString("File Tree (Placeholder):\n")
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		prefix := "  "
		if file.IsDir {
			prefix = "D "
		}
		builder.WriteString(fmt.Sprintf("%s%s\n", prefix, file.Path))
	}
	return builder.String()
}

// writeHighlightedCode takes code content, analyzes it, and writes it to the PDF with styles.
func writeHighlightedCode(pdf *gofpdf.Fpdf, style *chroma.Style, codeContent, filePath string, langData *LoadedLanguageData) error {
	// 1. Determine the lexer
	lexer := lexers.Analyse(codeContent) // Try analyzing content
	if lexer == nil {
		// Fallback to guessing by filename using langData if available
		if lang, ok := langData.GetLanguageForFile(filePath); ok {
			lexer = lexers.Get(lang) // Chroma might use different names than languages.yml
		}
	}
	if lexer == nil {
		lexer = lexers.Fallback // Default plain text lexer
	}
	lexer = chroma.Coalesce(lexer)

	// 2. Tokenize
	iterator, err := lexer.Tokenise(nil, codeContent)
	if err != nil {
		return fmt.Errorf("tokenization failed: %w", err)
	}

	// 3. Iterate and Write Tokens
	pdf.SetFont("Courier", "", pdfFontSize) // Base font

	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := style.Get(token.Type)
		styleStr := ""
		if entry.Bold == chroma.Yes {
			styleStr += "B"
		}
		if entry.Italic == chroma.Yes {
			styleStr += "I"
		}
		// Underline not directly supported by basic SetFontStyle
		pdf.SetFontStyle(styleStr)

		if entry.Colour.IsSet() {
			// Use Red(), Green(), Blue() which return uint8 (0-255)
			pdf.SetTextColor(int(entry.Colour.Red()), int(entry.Colour.Green()), int(entry.Colour.Blue()))
		} else {
			// Use default text color (e.g., black or style's default)
			fg := style.Get(chroma.Text).Colour
			if fg.IsSet() {
				pdf.SetTextColor(int(fg.Red()), int(fg.Green()), int(fg.Blue()))
			} else {
				pdf.SetTextColor(0, 0, 0) // Fallback to black
			}
		}

		// Handle background color? More complex, requires drawing rects.
		// Let's ignore background for simplicity.

		// Write the token value, handling tabs and newlines
		tokenValue := strings.ReplaceAll(token.Value, "\t", strings.Repeat(" ", pdfTabWidth))
		// gofpdf's Write handles basic line breaks within the cell width
		// Need to manage X, Y position manually for precise control over wrapping/indentation
		// Using Write for simplicity, may have wrapping issues.
		pdf.Write(pdfLineHeight, tokenValue)
	}
	pdf.Ln(-1) // Ensure we move to next line after last token

	return nil
}
