package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/atotto/clipboard"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	// Tokenizer interface defined in tokenizer.go
)

var (
	// Input Sources
	inputPaths []string

	// Filtering
	includePatterns string
	excludePatterns string
	maxSizeBytes    int64
	maxDepth        int
	showHidden      bool
	noIgnore        bool

	// Output
	outputFormat    string
	outputFile      string
	printToStdout   bool // Glimpse uses '-p', but true is the default unless -f or -c is used. Let's clarify behavior later.
	copyToClipboard bool // Glimpse uses '-c'

	// Processing
	numThreads int

	// Token Counting
	disableTokens  bool
	tokenizerType  string
	tokenizerModel string
	tokenizerFile  string

	// Web Specific
	traverseLinks bool
	linkDepth     int

	// PDF Output
	pdfOutputFile string

	// Interactive Mode
	interactiveMode bool

	cfgFile string // Variable to hold potential config file path flag (optional)

	langData *LoadedLanguageData // Global or passed around?
)

// version is the application version, set via ldflags.
var version string = "dev" // Default for local builds

var rootCmd = &cobra.Command{
	Use:   "iris [PATHS...]",
	Short: "Iris is a tool for quickly analyzing codebases, similar to Glimpse.",
	Long: `Iris allows you to process local directories, files, Git repositories,
and web URLs to generate structure views, display content, and count tokens.`,
	Version: version,             // Use the variable here
	Args:    cobra.ArbitraryArgs, // Allow paths to be passed as arguments
	Run: func(cmd *cobra.Command, args []string) {
		// initConfig and language loading are called via cobra.OnInitialize

		// Determine input paths: interactive or command-line args
		var finalInputPaths []string
		var err error
		if interactiveMode {
			finalInputPaths, err = runInteractiveFinder()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Interactive mode error: %v\n", err)
				os.Exit(1)
			}
			if finalInputPaths == nil {
				// User aborted interactive selection
				os.Exit(0)
			}
			fmt.Printf("Processing interactively selected paths: %v\n", finalInputPaths)
		} else {
			// Use command-line arguments
			finalInputPaths = args
			if len(finalInputPaths) == 0 {
				finalInputPaths = []string{"."} // Default to current directory if no paths provided
			}
		}

		// --- Initialize Tokenizer (if needed) ---
		var tokenizer Tokenizer // Use the interface type
		if !disableTokens {
			tokenizer, err = getTokenizer() // Returns the interface (assigns to existing err)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error initializing tokenizer: %v\n", err)
				disableTokens = true
				fmt.Fprintln(os.Stderr, "Token counting disabled due to error.")
			} else {
				// Ensure tokenizer resources are cleaned up if applicable
				defer tokenizer.Close()
			}
		}

		// --- Main Logic ---
		fmt.Println("Iris running...")

		var allFilesMaster []FileInfo // Collect files from all inputs first
		var failedPaths int
		var tempDirsToClean []string // Keep track of temp dirs for cleanup

		// Ensure temporary directories are cleaned up on exit (even if errors occur)
		defer func() {
			for _, dir := range tempDirsToClean {
				fmt.Printf("Cleaning up temporary directory: %s\n", dir)
				_ = os.RemoveAll(dir)
			}
		}()

		for _, input := range finalInputPaths {
			var filesToAppend []FileInfo
			var err error
			currentInput := input

			// Check Web URL FIRST
			if isWebURL(currentInput) {
				// Process web URL (potentially with traversal)
				if traverseLinks {
					fmt.Printf("Starting web traversal from %s (max depth: %d)\n", currentInput, linkDepth)
					visited := make(map[string]bool)
					filesToAppend, err = processWebURLRecursive(currentInput, 0, linkDepth, visited)
				} else {
					var fileInfo FileInfo
					fileInfo, err = processWebURL(currentInput)
					if err == nil {
						filesToAppend = []FileInfo{fileInfo}
					}
				}
			} else if isGitURL(currentInput) {
				// THEN check for Git URL
				tempDir, cloneErr := cloneGitRepo(currentInput)
				if cloneErr != nil {
					fmt.Fprintf(os.Stderr, "Error cloning git repo %s: %v\n", currentInput, cloneErr)
					err = cloneErr // Assign the error to be handled below
				} else {
					tempDirsToClean = append(tempDirsToClean, tempDir)
					currentInput = tempDir // Process the cloned directory path
					// Process the cloned directory as a local path
					filesToAppend, err = processLocalPath(currentInput, langData)
				}
			} else {
				// FINALLY, assume local path
				filesToAppend, err = processLocalPath(currentInput, langData)
			}

			// Handle errors from processing steps
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", input, err)
				failedPaths++
				continue
			}

			allFilesMaster = append(allFilesMaster, filesToAppend...)
		}

		// --- Parallel Token Counting (if enabled) ---
		var processedFiles []FileInfo
		if !disableTokens && tokenizer != nil { // Check if tokenizer was successfully initialized
			numWorkers := numThreads
			if numWorkers <= 0 {
				numWorkers = runtime.NumCPU()
			}
			fmt.Printf("Using %d worker(s) for token counting.\n", numWorkers)

			jobs := make(chan FileInfo, len(allFilesMaster))
			results := make(chan FileInfo, len(allFilesMaster))
			var wg sync.WaitGroup

			// Start workers
			for w := 0; w < numWorkers; w++ {
				wg.Add(1)
				// Pass the Tokenizer interface to the worker
				go tokenWorker(tokenizer, jobs, results, &wg)
			}

			// Send jobs (now includes FileInfo from web URLs)
			filesToProcess := 0
			for _, file := range allFilesMaster {
				if file.IsDir { // Directories don't need token counting
					results <- file
				} else {
					// Send files and web content (as FileInfo) to workers
					jobs <- file
					filesToProcess++
				}
			}
			close(jobs)

			// Wait for workers to finish
			wg.Wait()
			close(results)

			// Collect results
			processedFiles = make([]FileInfo, 0, len(allFilesMaster))
			for res := range results {
				processedFiles = append(processedFiles, res)
			}

		} else {
			// If token counting is disabled, just use the initially collected files
			processedFiles = allFilesMaster
		}
		// --- End Token Counting ---

		// --- Aggregation and Summary (using processedFiles) ---
		var totalFiles int
		var totalSize, totalTokens int64
		for _, file := range processedFiles {
			if !file.IsDir {
				totalFiles++
				totalSize += file.Size
				if !disableTokens {
					totalTokens += int64(file.TokenCount)
				}
			}
		}

		summary := Summary{
			TotalFiles:  totalFiles,
			TotalSize:   totalSize,
			TotalTokens: int(totalTokens),
		}

		// --- Output Generation (using processedFiles) ---
		if pdfOutputFile != "" {
			// Prioritize PDF output if the flag is set
			err = generatePDF(processedFiles, summary, outputFormat, langData, pdfOutputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error generating PDF: %v\n", err)
				// Optionally, print to stdout as fallback?
			}
		} else { // Handle non-PDF output (file, clipboard, stdout)
			// Generate the output string only when not creating PDF
			var outputBuilder strings.Builder
			if outputFormat == "tree" || outputFormat == "both" {
				if len(finalInputPaths) == 1 && isDir(finalInputPaths[0]) { // Check original single input path type
					rootNode := buildTree(processedFiles, finalInputPaths[0])
					outputBuilder.WriteString(printTree(rootNode))
				} else if len(processedFiles) > 0 { // Check if any files were processed
					// If multiple inputs or single file input, show the message
					outputBuilder.WriteString("Tree view generated for single directory input only.\nFiles found:\n")
					sort.Slice(processedFiles, func(i, j int) bool {
						return processedFiles[i].Path < processedFiles[j].Path
					})
					for _, file := range processedFiles {
						outputBuilder.WriteString(fmt.Sprintf("- %s\n", file.Path))
					}
				}
				if outputFormat == "both" {
					outputBuilder.WriteString("\n")
				}
			}
			if outputFormat == "files" || outputFormat == "both" {
				outputBuilder.WriteString(printFiles(processedFiles, !disableTokens))
			}
			// Add summary to the output string
			outputBuilder.WriteString("\n--- Summary ---\n")
			outputBuilder.WriteString(fmt.Sprintf("Total files processed: %d\n", summary.TotalFiles))
			outputBuilder.WriteString(fmt.Sprintf("Total size: %d bytes\n", summary.TotalSize))
			if !disableTokens {
				outputBuilder.WriteString(fmt.Sprintf("Total tokens: %d\n", summary.TotalTokens))
			}
			if failedPaths > 0 {
				outputBuilder.WriteString(fmt.Sprintf("Paths failed to process: %d\n", failedPaths))
			}

			// Declare and assign finalOutput here
			finalOutput := outputBuilder.String()

			// Now handle the destination for the generated string
			if outputFile != "" {
				// Save to text file
				err = os.WriteFile(outputFile, []byte(finalOutput), 0644)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error writing to file %s: %v\n", outputFile, err)
				}
				fmt.Printf("Output saved to %s\n", outputFile)
			} else if copyToClipboard {
				// Copy to clipboard
				err = clipboard.WriteAll(finalOutput)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error writing to clipboard: %v\n", err)
					fmt.Println("\n--- Output (clipboard failed) ---")
					fmt.Println(finalOutput)
				} else {
					fmt.Println("Output copied to clipboard.")
				}
			} else { // Default to stdout
				fmt.Println(finalOutput)
			}
		} // End of non-PDF output handling

		// --- End Main Logic ---
	},
}

func init() {
	// Initialize config first, then languages
	cobra.OnInitialize(initConfig, initLanguages)

	// --- Flag Definitions & Viper Binding ---
	// Optional: Allow specifying config file via flag
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/iris/config.toml)")

	// Filtering
	rootCmd.Flags().StringVarP(&includePatterns, "include", "i", "", `Additional patterns to include (comma-separated, e.g. *.rs,*.go)`)
	viper.BindPFlag("include", rootCmd.Flags().Lookup("include"))
	rootCmd.Flags().StringVarP(&excludePatterns, "exclude", "e", "", "Additional patterns to exclude (comma-separated)")
	viper.BindPFlag("exclude", rootCmd.Flags().Lookup("exclude"))
	viper.BindPFlag("default_excludes", rootCmd.Flags().Lookup("exclude")) // Allow config override via default_excludes
	rootCmd.Flags().Int64VarP(&maxSizeBytes, "max-size", "s", 0, "Maximum file size in bytes (0 for no limit)")
	viper.BindPFlag("max_size", rootCmd.Flags().Lookup("max-size"))
	rootCmd.Flags().IntVar(&maxDepth, "max-depth", 0, "Maximum directory depth to traverse (0 for no limit)")
	viper.BindPFlag("max_depth", rootCmd.Flags().Lookup("max-depth"))
	rootCmd.Flags().BoolVarP(&showHidden, "hidden", "H", false, "Show hidden files and directories")
	viper.BindPFlag("hidden", rootCmd.Flags().Lookup("hidden"))
	rootCmd.Flags().BoolVar(&noIgnore, "no-ignore", false, "Don't respect .gitignore files")
	viper.BindPFlag("no_ignore", rootCmd.Flags().Lookup("no-ignore")) // Use snake_case for viper key

	// Output
	rootCmd.Flags().StringVarP(&outputFormat, "output", "o", "both", "Output format: tree, files, or both")
	viper.BindPFlag("output", rootCmd.Flags().Lookup("output"))
	viper.BindPFlag("default_output_format", rootCmd.Flags().Lookup("output"))
	rootCmd.Flags().StringVarP(&outputFile, "file", "f", "", "Save output to specified file")
	viper.BindPFlag("file", rootCmd.Flags().Lookup("file"))
	rootCmd.Flags().BoolVarP(&printToStdout, "print", "p", false, "Print to stdout (default unless -f, -c, or --pdf used)")
	viper.BindPFlag("print", rootCmd.Flags().Lookup("print"))
	rootCmd.Flags().BoolVarP(&copyToClipboard, "clipboard", "c", false, "Copy output to clipboard")
	viper.BindPFlag("clipboard", rootCmd.Flags().Lookup("clipboard"))

	// Processing
	rootCmd.Flags().IntVarP(&numThreads, "threads", "t", 0, "Number of threads for parallel processing (0 for auto)")
	viper.BindPFlag("threads", rootCmd.Flags().Lookup("threads"))

	// Token Counting
	rootCmd.Flags().BoolVar(&disableTokens, "no-tokens", false, "Disable token counting")
	viper.BindPFlag("no_tokens", rootCmd.Flags().Lookup("no-tokens"))
	rootCmd.Flags().StringVar(&tokenizerType, "tokenizer", "tiktoken", "Tokenizer to use: tiktoken or huggingface")
	viper.BindPFlag("tokenizer", rootCmd.Flags().Lookup("tokenizer"))
	viper.BindPFlag("default_tokenizer", rootCmd.Flags().Lookup("tokenizer"))
	rootCmd.Flags().StringVar(&tokenizerModel, "model", "", "Model name for tokenizer (e.g., gpt-4o, gpt2)")
	viper.BindPFlag("model", rootCmd.Flags().Lookup("model"))
	viper.BindPFlag("default_tokenizer_model", rootCmd.Flags().Lookup("model"))
	rootCmd.Flags().StringVar(&tokenizerFile, "tokenizer-file", "", "Path to local tokenizer file")
	viper.BindPFlag("tokenizer_file", rootCmd.Flags().Lookup("tokenizer-file"))

	// Web Specific
	rootCmd.Flags().BoolVar(&traverseLinks, "traverse-links", false, "Traverse links when processing URLs")
	viper.BindPFlag("traverse_links", rootCmd.Flags().Lookup("traverse-links"))
	rootCmd.Flags().IntVar(&linkDepth, "link-depth", 1, "Maximum depth to traverse links")
	viper.BindPFlag("link_depth", rootCmd.Flags().Lookup("link-depth"))
	viper.BindPFlag("default_link_depth", rootCmd.Flags().Lookup("link-depth"))

	// PDF Output
	rootCmd.Flags().StringVar(&pdfOutputFile, "pdf", "", "Save output as PDF")
	viper.BindPFlag("pdf", rootCmd.Flags().Lookup("pdf"))

	// Interactive Mode
	rootCmd.Flags().BoolVar(&interactiveMode, "interactive", false, "Opens interactive file picker (? for help)")
	viper.BindPFlag("interactive", rootCmd.Flags().Lookup("interactive"))

	// Set Viper defaults (matching Glimpse example where possible)
	viper.SetDefault("max_size", 10485760) // 10MB
	viper.SetDefault("max_depth", 20)
	viper.SetDefault("default_output_format", "both")
	viper.SetDefault("default_tokenizer", "tiktoken")
	viper.SetDefault("default_tokenizer_model", "") // Rely on tokenizer specific defaults
	viper.SetDefault("traverse_links", false)
	viper.SetDefault("default_link_depth", 1)
	viper.SetDefault("default_excludes", []string{
		"**/.git/**",
		"**/target/**",
		"**/node_modules/**",
	})
	// Note: We bind the 'exclude' flag to 'default_excludes' as well,
	// so the config file setting can provide the default value for the flag.
	// If the -e flag is explicitly used, it overrides the config.

	// Set other viper defaults based on flag defaults if needed, though BindPFlag usually handles this.
	viper.SetDefault("hidden", false)
	viper.SetDefault("no_ignore", false)
	viper.SetDefault("threads", 0)
	viper.SetDefault("no_tokens", false)
	viper.SetDefault("interactive", false)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home/.config/iris directory with name "config" (without extension).
		viper.AddConfigPath(filepath.Join(home, ".config", "iris"))
		viper.AddConfigPath(".") // Also look in current directory
		viper.SetConfigName("config")
		viper.SetConfigType("toml")
	}

	viper.AutomaticEnv() // read in environment variables that match IRIS_*
	viper.SetEnvPrefix("IRIS")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	} else {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
			fmt.Fprintln(os.Stderr, "No config file found, using defaults and flags.")
		} else {
			// Config file was found but another error was produced
			fmt.Fprintf(os.Stderr, "Error reading config file: %s\n", err)
		}
	}

	// After loading config, potentially update flag variables if needed?
	// Cobra/Viper binding should handle this - the flag variables like `maxDepth`
	// should now hold the final value from Default < Config < Env < Flag.
	// Example: Update excludePatterns based on combined sources if needed
	// The `default_excludes` from config will set the default for the `exclude` flag.
	// If `-e` is used, it overrides. If neither, the flag default ("" initially) is used.
	// Let's explicitly load the excludes from viper IF the flag wasn't set.
	if !rootCmd.Flags().Changed("exclude") {
		excludePatterns = strings.Join(viper.GetStringSlice("default_excludes"), ",")
	}
	// Similar logic could apply to other defaults if direct variable access is preferred over flags.
}

// initLanguages loads the language definitions.
func initLanguages() {
	var err error
	langData, err = loadLanguageData()
	if err != nil {
		// Log error but don't necessarily fail the program?
		// Language filtering will simply not be applied.
		fmt.Fprintf(os.Stderr, "Warning: Could not load language definitions: %v\n", err)
		fmt.Fprintln(os.Stderr, "Proceeding without language-based filtering.")
		langData = nil // Ensure it's nil if loading failed
	}
}

func main() {
	// initConfig() is called via cobra.OnInitialize(initConfig)
	rootCmd.Execute()
}

// Helper function to check if a path is a directory (used in output generation)
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false // Treat errors as non-directories for safety
	}
	return info.IsDir()
}

// tokenWorker now accepts the Tokenizer interface.
func tokenWorker(tk Tokenizer, jobs <-chan FileInfo, results chan<- FileInfo, wg *sync.WaitGroup) {
	defer wg.Done()
	for file := range jobs {
		if file.IsDir {
			results <- file
			continue
		}

		var content []byte
		var readErr error

		if file.Content != nil { // Use pre-loaded content (from web processing)
			content = file.Content
		} else { // Read from disk for local files/git files
			content, readErr = os.ReadFile(file.Path)
		}

		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: worker could not read file %s: %v\n", file.Path, readErr)
			file.Error = readErr
		} else if len(content) > 0 { // Only count tokens if content is available and read successfully
			// Use the interface method to count tokens
			file.TokenCount = tk.CountTokens(string(content))
		} else {
			file.TokenCount = 0
		}
		results <- file
	}
}

// isWebURL checks if the input string is an HTTP/HTTPS URL.
func isWebURL(input string) bool {
	return strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://")
}
