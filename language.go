package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LanguageInfo holds details about a specific programming/markup language.
// We only include fields relevant for file detection for now.
type LanguageInfo struct {
	Type         string   `yaml:"type"` // e.g., programming, data, markup
	Extensions   []string `yaml:"extensions"`
	Filenames    []string `yaml:"filenames"`
	Interpreters []string `yaml:"interpreters"`
	// Add other fields like color, language_id later
	// TODO: maybe if I feel like it
}

// LanguageMap maps language names (e.g., "Go") to their details.
type LanguageMap map[string]LanguageInfo

// LoadedLanguageData holds the parsed language map and provides helper methods.
type LoadedLanguageData struct {
	Langs        LanguageMap
	extensionMap map[string]string // Map extension (e.g., ".go") to language name ("Go")
	filenameMap  map[string]string // Map filename (e.g., "Makefile") to language name ("Makefile")
}

// loadLanguageData attempts to load and parse languages.yml.
func loadLanguageData() (*LoadedLanguageData, error) {
	// Look for languages.yml in standard config paths
	configPaths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		configPaths = append(configPaths, filepath.Join(home, ".config", "iris"))
	}
	configPaths = append(configPaths, ".") // Current directory

	var langFilePath string
	for _, p := range configPaths {
		testPath := filepath.Join(p, "languages.yml")
		if _, err := os.Stat(testPath); err == nil {
			langFilePath = testPath
			break
		}
	}

	if langFilePath == "" {
		return nil, fmt.Errorf("languages.yml not found in standard config locations")
	}

	fmt.Printf("Loading language definitions from: %s\n", langFilePath)
	yamlFile, err := os.ReadFile(langFilePath)
	if err != nil {
		return nil, fmt.Errorf("error reading language file %s: %w", langFilePath, err)
	}

	var langs LanguageMap
	err = yaml.Unmarshal(yamlFile, &langs)
	if err != nil {
		return nil, fmt.Errorf("error parsing language file %s: %w", langFilePath, err)
	}

	// Build lookup maps for faster matching
	data := &LoadedLanguageData{
		Langs:        langs,
		extensionMap: make(map[string]string),
		filenameMap:  make(map[string]string),
	}

	for langName, info := range langs {
		for _, ext := range info.Extensions {
			// Ensure extension includes the dot and is lowercase for consistent matching
			lowerExt := strings.ToLower(ext)
			if data.extensionMap[lowerExt] == "" { // Avoid overwriting if multiple languages claim same ext
				data.extensionMap[lowerExt] = langName
			}
		}
		for _, fname := range info.Filenames {
			// Match filenames case-sensitively? Linguist often does.
			if data.filenameMap[fname] == "" {
				data.filenameMap[fname] = langName
			}
		}
	}

	fmt.Printf("Loaded %d languages with %d extensions and %d specific filenames.\n", len(data.Langs), len(data.extensionMap), len(data.filenameMap))
	return data, nil
}

// GetLanguageForFile determines the language for a given path based on loaded data.
func (ld *LoadedLanguageData) GetLanguageForFile(filePath string) (string, bool) {
	if ld == nil {
		return "", false // No language data loaded
	}

	baseName := filepath.Base(filePath)
	ext := strings.ToLower(filepath.Ext(baseName))

	// 1. Check exact filename match first (higher precedence)
	if lang, ok := ld.filenameMap[baseName]; ok {
		return lang, true
	}

	// 2. Check extension match
	if ext != "" {
		if lang, ok := ld.extensionMap[ext]; ok {
			return lang, true
		}
	}

	// Could add interpreter matching here if needed

	return "", false // No match found
}
