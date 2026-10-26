package styles

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
)

//go:embed *.xml
var embedded embed.FS

// Registry of Styles.
//
// lite: embedded styles are parsed on first Get, not at process start. The map
// holds every embedded name with a nil value until then, so existence checks
// (_, ok := Registry[name]) keep working without parsing 70 XML files.
var Registry = func() map[string]*chroma.Style {
	registry := map[string]*chroma.Style{}
	files, err := fs.ReadDir(embedded, ".")
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		if !file.IsDir() {
			registry[strings.TrimSuffix(file.Name(), ".xml")] = nil
		}
	}
	return registry
}()

var styleMu sync.Mutex

// lookup returns the parsed style for name, parsing the embedded XML once.
func lookup(name string) (*chroma.Style, bool) {
	styleMu.Lock()
	defer styleMu.Unlock()
	style, ok := Registry[name]
	if !ok || style != nil {
		return style, ok
	}
	r, err := embedded.Open(name + ".xml")
	if err != nil {
		panic(err)
	}
	defer r.Close()
	style, err = chroma.NewXMLStyle(r)
	if err != nil {
		panic(err)
	}
	Registry[name] = style
	return style, true
}

// Fallback style. Reassign to change the default fallback style. nil means
// the embedded "swapoff" style (resolved lazily; use GetFallback).
var Fallback *chroma.Style

// GetFallback returns Fallback, or swapoff when Fallback is unset.
func GetFallback() *chroma.Style {
	if Fallback != nil {
		return Fallback
	}
	s, _ := lookup("swapoff")
	return s
}

// Register a chroma.Style.
func Register(style *chroma.Style) *chroma.Style {
	styleMu.Lock()
	defer styleMu.Unlock()
	Registry[style.Name] = style
	return style
}

// Names of all available styles.
func Names() []string {
	styleMu.Lock()
	defer styleMu.Unlock()
	out := []string{}
	for name := range Registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Get named style, or Fallback.
func Get(name string) *chroma.Style {
	if style, ok := lookup(name); ok {
		return style
	}
	return GetFallback()
}
