package main

import (
	"fmt"
	"os"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
	hf "github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
)

// Tokenizer is an interface for different tokenizer implementations.
type Tokenizer interface {
	CountTokens(text string) int
	Close() // Add a Close method for potential resource cleanup (like HF tokenizer)
}

// --- Tiktoken Wrapper ---

type TiktokenWrapper struct {
	ttk *tiktoken.Tiktoken
}

func (w *TiktokenWrapper) CountTokens(text string) int {
	if w.ttk == nil {
		return 0
	}
	tokens := w.ttk.EncodeOrdinary(text)
	return len(tokens)
}

func (w *TiktokenWrapper) Close() {
	// No explicit close needed for tiktoken-go
}

// --- HuggingFace (sugarme) Wrapper ---

type HFTokenizerWrapper struct {
	htk *hf.Tokenizer
}

func (w *HFTokenizerWrapper) CountTokens(text string) int {
	if w.htk == nil {
		return 0
	}
	en, err := w.htk.EncodeSingle(text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: HF tokenizer failed to encode text: %v\n", err)
		return 0
	}
	return len(en.Tokens)
}

func (w *HFTokenizerWrapper) Close() {
	// sugarme/tokenizer doesn't seem to have an explicit Close/Free method
}

// --- Tokenizer Loading Logic ---

const defaultTiktokenModel = "gpt-4o" // Default if tokenizer is tiktoken
const defaultHFModel = "gpt2"         // Default if tokenizer is huggingface and no model specified

// getTokenizer returns a tokenizer instance based on flags.
// It returns a Tokenizer interface.
func getTokenizer() (Tokenizer, error) {
	fmt.Printf("Initializing tokenizer (Type: %s, Model: %s, File: %s)\n", tokenizerType, tokenizerModel, tokenizerFile)

	switch strings.ToLower(tokenizerType) {
	case "tiktoken":
		return loadTiktoken()
	case "huggingface":
		return loadHuggingFace()
	default:
		return nil, fmt.Errorf("unsupported tokenizer type: %s. Use 'tiktoken' or 'huggingface'", tokenizerType)
	}
}

func loadTiktoken() (Tokenizer, error) {
	model := tokenizerModel
	if model == "" {
		model = defaultTiktokenModel
		fmt.Printf("No Tiktoken model specified, using default: %s\n", model)
	}

	tke, err := tiktoken.EncodingForModel(model)
	if err != nil {
		fmt.Printf("Warning: Tiktoken model '%s' not found, falling back to default '%s'. Error: %v\n", model, defaultTiktokenModel, err)
		tke, err = tiktoken.EncodingForModel(defaultTiktokenModel)
		if err != nil {
			return nil, fmt.Errorf("failed to get tiktoken encoding for default model '%s': %w", defaultTiktokenModel, err)
		}
	}
	return &TiktokenWrapper{ttk: tke}, nil
}

func loadHuggingFace() (Tokenizer, error) {
	if tokenizerFile != "" {
		// Load from local file
		fmt.Printf("Loading HuggingFace tokenizer from file: %s\n", tokenizerFile)
		ttk, err := pretrained.FromFile(tokenizerFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load tokenizer from file %s: %w", tokenizerFile, err)
		}
		return &HFTokenizerWrapper{htk: ttk}, nil
	} else {
		// Load from Hugging Face Hub
		model := tokenizerModel
		if model == "" {
			model = defaultHFModel
			fmt.Printf("No HuggingFace model specified, using default: %s\n", model)
		}
		fmt.Printf("Loading HuggingFace tokenizer for model: %s (this may download files)\n", model)

		// sugarme/tokenizer uses CachedPath to download/find the tokenizer.json
		// We need the identifier used on the Hub (e.g., "bert-base-uncased")
		configFilePath, err := hf.CachedPath(model, "tokenizer.json")
		if err != nil {
			return nil, fmt.Errorf("failed to get cache path for model %s: %w", model, err)
		}

		ttk, err := pretrained.FromFile(configFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load pretrained tokenizer for model %s (from %s): %w", model, configFilePath, err)
		}
		return &HFTokenizerWrapper{htk: ttk}, nil
	}
}

// countTokens is now a method on the interface wrappers, no longer needed here.
/*
func countTokens(tke *tiktoken.Tiktoken, content []byte) int {
	...
}
*/
