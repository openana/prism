package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// loadConfig reads and parses a <locale>.yaml config file for the given cname.
// It looks in srcDir/cname/<locale>.yaml.
func loadConfig(srcDir, cname, locale string) (*ZDocConfig, error) {
	yamlPath := filepath.Join(srcDir, cname, locale+".yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", yamlPath, err)
	}

	var raw ZDocConfigOnDisk
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", yamlPath, err)
	}

	if raw.Title == "" {
		return nil, fmt.Errorf("config %s: missing required '_' (title) field", yamlPath)
	}
	if len(raw.Block) == 0 {
		raw.Block = []string{"index"}
	}

	// Parse inputs: the raw YAML unmarshals input values as map[string]any.
	// We need to classify each as option, bool, or text.
	inputs := make(map[string]InputDef)
	for name, rawDef := range raw.Input {
		if rawDef == nil {
			continue
		}
		def, err := parseInputDef(name, rawDef)
		if err != nil {
			return nil, fmt.Errorf("config %s, input %q: %w", yamlPath, name, err)
		}
		inputs[name] = def
	}

	return &ZDocConfig{
		CName:  cname,
		Locale: locale,
		Title:  raw.Title,
		Block:  raw.Block,
		Input:  inputs,
		Filter: raw.Filter,
	}, nil
}

// parseInputDef classifies a raw input definition into Option, Bool, or Text.
func parseInputDef(name string, raw any) (InputDef, error) {
	common := InputCommon{Name: name}

	rawMap, ok := raw.(map[string]any)
	if !ok {
		return InputDef{}, fmt.Errorf("expected a map, got %T", raw)
	}

	if title, ok := rawMap["_"].(string); ok {
		common.Title = title
	}
	if note, ok := rawMap["note"].(string); ok {
		common.Note = note
	}

	_, hasOption := rawMap["option"]
	_, hasTrue := rawMap["true"]
	_, hasFalse := rawMap["false"]

	if hasOption {
		return parseOptionInput(common, rawMap)
	}
	if hasTrue || hasFalse {
		return parseBoolInput(common, rawMap)
	}
	// Text input
	return parseTextInput(common, rawMap), nil
}

func parseOptionInput(common InputCommon, rawMap map[string]any) (InputDef, error) {
	optsRaw, ok := rawMap["option"].(map[string]any)
	if !ok {
		return InputDef{}, fmt.Errorf("option field is not a map")
	}

	var options []InputOption
	defaultVal, _ := rawMap["default"].(string)

	// If there's a default and it exists in options, put it first.
	if defaultVal != "" {
		if optRaw, exists := optsRaw[defaultVal]; exists {
			opt, err := parseOptionValue(defaultVal, optRaw)
			if err != nil {
				return InputDef{}, err
			}
			options = append(options, opt)
		}
	}

	for optName, optRaw := range optsRaw {
		if optName == defaultVal {
			continue
		}
		opt, err := parseOptionValue(optName, optRaw)
		if err != nil {
			return InputDef{}, err
		}
		options = append(options, opt)
	}

	return InputDef{
		Common: common,
		Option: options,
	}, nil
}

func parseOptionValue(name string, raw any) (InputOption, error) {
	// nil value means no extra vars and no label override
	if raw == nil {
		return InputOption{
			Label: name,
			Value: name,
			Extra: nil,
		}, nil
	}

	rawMap, ok := raw.(map[string]any)
	if !ok {
		return InputOption{}, fmt.Errorf("option value for %q is not a map", name)
	}

	label := name
	if l, ok := rawMap["_"].(string); ok {
		label = l
	}

	extra := make(map[string]string)
	for k, v := range rawMap {
		if k == "_" {
			continue
		}
		extra[k] = fmt.Sprintf("%v", v)
	}

	return InputOption{
		Label: label,
		Value: name,
		Extra: extra,
	}, nil
}

func parseBoolInput(common InputCommon, rawMap map[string]any) (InputDef, error) {
	b := &InputBool{}
	// Match TypeScript behavior:
	//   true: absent   → boolean true   → "true" string
	//   true: nil      → boolean true   → "true" string
	//   true: <value>  → <value>        → string as-is
	if t, ok := rawMap["true"]; ok {
		if t == nil {
			b.TrueValue = "true"
		} else {
			b.TrueValue = fmt.Sprintf("%v", t)
		}
	} else {
		b.TrueValue = "true"
	}
	if f, ok := rawMap["false"]; ok {
		if f == nil {
			b.FalseValue = "false"
		} else {
			b.FalseValue = fmt.Sprintf("%v", f)
		}
	} else {
		b.FalseValue = "false"
	}
	if d, ok := rawMap["default"].(bool); ok {
		b.Default = d
	}
	return InputDef{
		Common: common,
		Bool:   b,
	}, nil
}

func parseTextInput(common InputCommon, rawMap map[string]any) InputDef {
	t := &InputText{}
	if d, ok := rawMap["default"].(string); ok {
		t.Default = d
	}
	return InputDef{
		Common: common,
		Text:   t,
	}
}

// loadBlock reads a content block file: <cname>/<block>.<locale>.md
func loadBlock(srcDir, cname, block, locale string) (string, error) {
	blockPath := filepath.Join(srcDir, cname, block+"."+locale+".md")
	data, err := os.ReadFile(blockPath)
	if err != nil {
		return "", fmt.Errorf("reading block %s: %w", blockPath, err)
	}
	return string(data), nil
}

// discoverPages walks srcDir and finds all (<cname>, <locale>) pairs
// that have a valid <locale>.yaml config.
func discoverPages(srcDir string) ([]struct{ CName, Locale string }, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("reading source directory: %w", err)
	}

	var pages []struct{ CName, Locale string }
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cname := entry.Name()
		// Skip hidden directories
		if strings.HasPrefix(cname, ".") {
			continue
		}

		cnameDir := filepath.Join(srcDir, cname)
		files, err := os.ReadDir(cnameDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if strings.HasSuffix(name, ".yaml") {
				locale := strings.TrimSuffix(name, ".yaml")
				pages = append(pages, struct{ CName, Locale string }{cname, locale})
			}
		}
	}
	return pages, nil
}
