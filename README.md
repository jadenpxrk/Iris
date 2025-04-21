# Iris 👁️‍🗨️

Iris is a Go implementation of [Glimpse](https://github.com/seatedro/glimpse), a command-line tool for quickly analyzing codebases, files, Git repositories, and web URLs. It helps LLMs better understand code structure, view content, and read documentation optimized for LLM consumption, and estimates token counts.

[![Go Report Card](https://goreportcard.com/badge/github.com/jadenpxrk/iris)](https://goreportcard.com/report/github.com/jadenpxrk/iris)

## Installation

The recommended way to install `iris` is by downloading a pre-compiled binary for your operating system and architecture from the [Latest Release](https://github.com/jadenpxrk/iris/releases/latest) page.

1.  Download the appropriate archive/binary (e.g., `iris-darwin-arm64`, `iris-linux-amd64`, `iris-windows-amd64.exe`).
2.  **macOS / Linux:**
    - Make the binary executable: `chmod +x ./iris-<os>-<arch>`
    - Move the binary to a location in your system's PATH: `sudo mv ./iris-<os>-<arch> /usr/local/bin/iris` (or another directory like `~/bin` if you prefer).
3.  **Windows:**
    - Place the downloaded `.exe` file in a directory.
    - Add that directory to your system's `PATH` environment variable.

Once installed, you should be able to run `iris` from any terminal.

### Alternative: Using `go install` (Requires Go)

If you have Go installed and configured (`$GOPATH/bin` or `$HOME/go/bin` in your `PATH`), you can install directly from the source:

```bash
go install github.com/jadenpxrk/iris@latest
```

## Usage

The basic command structure is:

```bash
iris [OPTIONS] [PATHS...]
```

- `OPTIONS`: Flags to control filtering, output, token counting, etc.
- `PATHS...`: One or more local file paths, directory paths, Git repository URLs, or web URLs. Defaults to the current directory (`.`) if no paths are provided.

Run `iris --help` to see all available options.

**Examples:**

```bash
# Process the current directory, showing tree and file content
iris .

# Process a specific Go file and copy output to clipboard
iris -o files -c main.go

# Process a remote Git repository (only .go files) and save to output.txt
iris --include="*.go" -f output.txt https://github.com/golang/go.git

# Process a web URL, convert to Markdown, and count tokens
iris https://example.com

# Traverse links on a web page (max depth 1) and output to PDF
iris --traverse-links --link-depth 1 --pdf report.pdf https://example.com

# Interactively select files/directories to process
iris --interactive
```

## Key Features

- **Multiple Input Sources:** Handles local files/directories, Git URLs, and HTTP/HTTPS URLs.
- **Web Traversal:** Fetches web content, converts HTML to Markdown, and optionally follows links (`--traverse-links`, `--link-depth`).
- **Advanced Filtering:**
  - Include/Exclude patterns (`--include`, `--exclude`).
  - Max file size and directory depth (`--max-size`, `--max-depth`).
  - Show hidden files (`--hidden`).
  - Respects `.gitignore` (`--no-ignore` to disable).
  - Language detection via `languages.yml` for filtering (when no `--include` is specified).
- **Flexible Output:**
  - Formats: `tree`, `files`, `both` (`--output`).
  - Destinations: stdout (default), file (`--file`), clipboard (`--clipboard`), PDF (`--pdf`).
  - Syntax highlighting in PDF output.
- **Token Counting:**
  - Supports `tiktoken` (default: `gpt-4o`) and `huggingface` tokenizers (`--tokenizer`, `--model`, `--tokenizer-file`).
  - Parallel processing for speed (`--threads`).
  - Disable token counting (`--no-tokens`).
- **Interactive Mode:** Use a fuzzy finder to select inputs (`--interactive`).
- **Configuration:** Customize defaults via `config.toml` (in `$HOME/.config/iris/` or `.`) or environment variables (`IRIS_*`).

## Configuration

`iris` looks for a configuration file named `config.toml` in the following locations:

1.  Current working directory (`.`)
2.  `$HOME/.config/iris/` (on Linux/macOS)

Settings can also be controlled via environment variables prefixed with `IRIS_` (e.g., `IRIS_MAX_DEPTH=10`). Command-line flags take precedence over environment variables, which take precedence over the configuration file.

See the flags available via `iris --help` for configurable options.

## Language Detection

`iris` can use a `languages.yml` file (placed in the same locations as `config.toml`) to identify file types based on extensions or filenames. This is used for filtering when no explicit `--include` patterns are provided. A sample `languages.yml` is included in the repository.

## Building from Source

Ensure you have Go installed (version 1.21 or later recommended).

```bash
# Clone the repository
git clone https://github.com/jadenpxrk/iris.git
cd iris

# Build the binary
go build .

# Optionally install it to your Go bin path
go install .
```
