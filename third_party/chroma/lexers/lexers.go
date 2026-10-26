package lexers

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/alecthomas/chroma/v2"
)

//go:embed embedded
var embedded embed.FS

// GlobalLexerRegistry is the global LexerRegistry of Lexers.
//
// lite: the ~250 embedded XML lexers are parsed on first lookup, not at
// process start (init cost was a fixed per-call tax on every bd command).
var GlobalLexerRegistry = chroma.NewLexerRegistry()

var loadOnce sync.Once

// afterLoad holds init-time tweaks that need the embedded lexers.
var afterLoad []func()

func load() {
	loadOnce.Do(func() {
		paths, err := fs.Glob(embedded, "embedded/*.xml")
		if err != nil {
			panic(err)
		}
		for _, path := range paths {
			GlobalLexerRegistry.Register(chroma.MustNewXMLLexer(embedded, path))
		}
		for _, f := range afterLoad {
			f()
		}
	})
}

// Names of all lexers, optionally including aliases.
func Names(withAliases bool) []string {
	load()
	return GlobalLexerRegistry.Names(withAliases)
}

// Aliases of all the lexers, and skip those lexers who do not have any aliases,
// or show their name instead
func Aliases(skipWithoutAliases bool) []string {
	load()
	return GlobalLexerRegistry.Aliases(skipWithoutAliases)
}

// Get a Lexer by name, alias or file extension.
//
// Note that this if there isn't an exact match on name or alias, this will
// call Match(), so it is not efficient.
func Get(name string) chroma.Lexer {
	load()
	return GlobalLexerRegistry.Get(name)
}

// MatchMimeType attempts to find a lexer for the given MIME type.
func MatchMimeType(mimeType string) chroma.Lexer {
	load()
	return GlobalLexerRegistry.MatchMimeType(mimeType)
}

// Match returns the first lexer matching filename.
//
// Note that this iterates over all file patterns in all lexers, so it's not
// particularly efficient.
func Match(filename string) chroma.Lexer {
	load()
	return GlobalLexerRegistry.Match(filename)
}

// Register a Lexer with the global registry.
func Register(lexer chroma.Lexer) chroma.Lexer {
	return GlobalLexerRegistry.Register(lexer)
}

// Analyse text content and return the "best" lexer..
func Analyse(text string) chroma.Lexer {
	load()
	return GlobalLexerRegistry.Analyse(text)
}

// PlaintextRules is used for the fallback lexer as well as the explicit
// plaintext lexer.
func PlaintextRules() chroma.Rules {
	return chroma.Rules{
		"root": []chroma.Rule{
			{`.+`, chroma.Text, nil},
			{`\n`, chroma.Text, nil},
		},
	}
}

// Fallback lexer if no other is found.
var Fallback chroma.Lexer = chroma.MustNewLexer(&chroma.Config{
	Name:      "fallback",
	Filenames: []string{"*"},
	Priority:  -1,
}, PlaintextRules)
