package main

// ZDocInputCommon holds fields common to all input types.
type ZDocInputCommon struct {
	Title string `yaml:"_"`    // display label
	Note  string `yaml:"note"` // optional sidenote
}

// OptionValue represents a single option in a dropdown select.
// _ is the display label; all other keys are extra context variables
// injected into the Mustache rendering context when this option is selected.
type OptionValue map[string]string

// ZDocInputOption is a dropdown/select input.
type ZDocInputOption struct {
	ZDocInputCommon `yaml:",inline"`
	Option          map[string]OptionValue `yaml:"option"`
	Default         string                 `yaml:"default"`
}

// ZDocInputBool is a boolean toggle switch.
// True/False hold the values injected when the toggle is on/off.
// If True is nil, the variable is set to true (boolean).
// If True is a string, the variable is set to that string value.
type ZDocInputBool struct {
	ZDocInputCommon `yaml:",inline"`
	True            any  `yaml:"true"`
	False           any  `yaml:"false"`
	Default         bool `yaml:"default"`
}

// ZDocInputText is a free-text input.
type ZDocInputText struct {
	ZDocInputCommon `yaml:",inline"`
	Default         string `yaml:"default"`
}

// ZDocConfigOnDisk mirrors the raw YAML structure on disk.
// Fields may be absent (due to merging of local/global configs).
type ZDocConfigOnDisk struct {
	Title  string         `yaml:"_"`     // display name / title
	Block  []string       `yaml:"block"` // ordered content block names
	Input  map[string]any `yaml:"input"` // raw input definitions (parsed post-unmarshal)
	Filter *ZDocFilter    `yaml:"filter"`
}

// ZDocFilter holds optional filtering rules.
type ZDocFilter struct {
	Scheme string `yaml:"scheme"` // enforced protocol, e.g. "https"
}

// ZDocConfig is the fully-resolved config for a single locale of a distro.
type ZDocConfig struct {
	CName  string              // distro canonical name (directory name)
	Locale string              // language code, e.g. "zh"
	Title  string              // page title
	Block  []string            // ordered content block names
	Input  map[string]InputDef // parsed input definitions
	Filter *ZDocFilter         // optional filter
}

// InputDef is a discriminated union over the three input types.
// Only one of Option, Bool, or Text is non-nil.
type InputDef struct {
	Common InputCommon
	Option []InputOption // populated for select inputs
	Bool   *InputBool    // populated for toggle inputs
	Text   *InputText    // populated for text inputs
}

// InputCommon holds fields shared by all input types in the resolved form.
type InputCommon struct {
	Name  string
	Title string
	Note  string
}

// InputOption represents a single option in a select dropdown (resolved form).
type InputOption struct {
	Label string            // display label
	Value string            // the value for the named variable
	Extra map[string]string // extra context variables injected when selected
}

// InputBool represents a boolean toggle (resolved form).
// TrueValue/FalseValue are always strings (resolved in parseBoolInput).
type InputBool struct {
	TrueValue  string
	FalseValue string
	Default    bool
}

// InputText represents a free-text input (resolved form).
type InputText struct {
	Default string
}

// ZTMPLBlock represents a parsed {ztmpl} fenced code block.
type ZTMPLBlock struct {
	TemplateContent string   // raw Mustache template content
	Lang            string   // code language for syntax highlighting
	Path            string   // target file path
	Append          bool     // whether content appends to path
	Inputs          []string // space-separated list of input variable names
	Global          bool     // whether this is a global menu (content ignored)
	TemplateID      string   // unique id, e.g. "tmpl-0"
	GlobalMenuID    string   // for global menus, e.g. "globalMenu-0"
}

// Page represents a fully assembled output page.
type Page struct {
	CName  string
	Locale string
	Config *ZDocConfig
	Blocks []string     // raw content of each block (in order)
	ZTMPLs []ZTMPLBlock // extracted ztmpl blocks in document order
	HTML   string       // final rendered HTML (post-goldmark, post-ztmpl replacement)
}
