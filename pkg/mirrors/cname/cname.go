package cname

import (
	_ "embed"

	"github.com/goccy/go-yaml"
)

//go:embed cname.yaml
var cnameYAML []byte
var cnameMap map[string]string // Alias -> cname

func init() {
	var cmap map[string][]string // cname -> []Alias

	err := yaml.Unmarshal(cnameYAML, &cmap)
	if err != nil {
		panic(err)
	}

	cnameMap = make(map[string]string)

	for cname, aliases := range cmap {
		cnameMap[cname] = cname // Add cname self as alias
		for _, alias := range aliases {
			cnameMap[alias] = cname
		}
	}
}

func GetCname(s string) (cname string, ok bool) {
	cname, ok = cnameMap[s]
	return
}

func Cname(s string) (cname string) {
	var ok bool
	cname, ok = cnameMap[s]
	if ok {
		return
	} else {
		return s
	}
}
