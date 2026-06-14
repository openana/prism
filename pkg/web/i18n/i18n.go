package i18n

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

//go:embed translations/*.yaml
var translationFS embed.FS

type Locale struct {
	Lang string            // BCP 47 language tag, e.g. "en", "zh-CN"
	keys map[string]string // translation key -> message
}

// T returns the translation for key
func (l *Locale) T(key string, args ...any) string {
	msg, ok := l.keys[key]
	if !ok {
		msg = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

var (
	defaultLocale  *Locale
	localeRegistry = map[string]*Locale{} // lang tag -> locale
)

// load parses a yaml file.
func load(path string) (*Locale, error) {
	data, err := translationFS.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	base := filepath.Base(path)
	lang := strings.TrimSuffix(base, filepath.Ext(base))

	return &Locale{Lang: lang, keys: raw}, nil
}

// Init loads all .yaml files under translations/
// default en
func init() {
	err := func() error {
		entries, err := fs.ReadDir(translationFS, "translations")
		if err != nil {
			return fmt.Errorf("read translations dir: %w", err)
		}

		var first *Locale
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
				continue
			}
			loc, err := load("translations/" + entry.Name())
			if err != nil {
				return err
			}
			localeRegistry[loc.Lang] = loc
			if first == nil {
				first = loc
			}
		}

		if len(localeRegistry) == 0 {
			return fmt.Errorf("no .yaml files found in translations/")
		}

		// EN default
		if en, ok := localeRegistry["en"]; ok {
			defaultLocale = en
		} else {
			return fmt.Errorf("no en.yaml")
		}
		return nil
	}()
	if err != nil {
		panic(err)
	}
}

// en
func Default() *Locale {
	return defaultLocale
}

// Resolve picks the best locale in Accept-Language.
// Returns the default locale if no match is found.
func Resolve(acceptLanguage string) *Locale {
	if acceptLanguage == "" {
		return defaultLocale
	}

	// e.g. "zh-CN,zh;q=0.9,en;q=0.8" -> "zh-CN"
	first := strings.SplitN(acceptLanguage, ",", 2)[0]
	first = strings.TrimSpace(first)
	if idx := strings.IndexByte(first, ';'); idx != -1 {
		first = first[:idx]
	}

	if loc, ok := localeRegistry[first]; ok {
		return loc
	}
	return defaultLocale
}
