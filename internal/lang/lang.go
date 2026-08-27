// Package lang is the table of languages this tool reads: which grammar parses
// each one, and which node kinds in that grammar are a comment, a function, or
// a docstring.
//
// It is data and holds no judgment. What decides whether a commented-out
// fragment is really code differs per language and lives with the rules, found
// by [Language.Name] — a table holding those predicates would depend on the
// rules, and the rules already depend on the table.
package lang

import (
	"path/filepath"
	"strings"
	"unsafe"

	tsdocker "github.com/alexaandru/go-sitter-forest/dockerfile"
	tsmake "github.com/alexaandru/go-sitter-forest/make"
	tshcl "github.com/tree-sitter-grammars/tree-sitter-hcl/bindings/go"
	tsyaml "github.com/tree-sitter-grammars/tree-sitter-yaml/bindings/go"
	tsbash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
	tsc "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tscpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tsphp "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tsruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Language binds a grammar to the node kinds slopguard reads out of its trees.
type Language struct {
	// Name is the key the per-language predicates are found under. It is the
	// one field a rule reads.
	Name      string
	Grammar   func() unsafe.Pointer
	Comments  map[string]bool
	Functions map[string]bool
	// Strict reports whether prose reliably fails to parse as source in this
	// language, which is what makes the commented-out-code rule safe to apply.
	Strict bool
	// Wrapper puts a fragment where this language's grammar expects statements.
	// Code is commented out from inside a function far more often than from
	// file scope, and the two parse by different rules: tree-sitter-go reads
	// `fmt.Println("x")` at file scope as a conversion and inside a function as
	// a call. Recovery differs too, so an unclosed brace is an error at file
	// scope and a MISSING node in a body.
	//
	// A language without one is parsed bare, which is what every language here
	// did before any of them was measured.
	Wrapper func() (prefix, suffix string)
	// Templated reports whether files in this language are commonly written
	// through a template whose actions the grammar cannot read.
	Templated bool
	// Docstrings reports whether this language documents with a string in
	// statement position rather than with a comment. Python does, and it is
	// where essentially all of its standing documentation lives.
	Docstrings bool
	// Registers reports whether commenting a setting out is how these files
	// publish the settings they accept, which makes a commented-out structure
	// documentation rather than residue.
	Registers bool
}

// Lookup returns the language to parse path as, or nil when nothing here reads
// it. A file with no useful extension is looked up by name, which is the only
// way a Dockerfile or a Makefile is ever identified.
func Lookup(path string) *Language {
	name := filepath.Base(path)
	if lang := byName[name]; lang != nil {
		return lang
	}
	extension := strings.ToLower(filepath.Ext(name))
	if lang := byExtension[extension]; lang != nil {
		return lang
	}
	if formats[extension] {
		return nil
	}
	// A named file keeps its language when it is qualified: Dockerfile.dev and
	// api.Dockerfile are both Dockerfiles, and Makefile.local is a Makefile.
	// An extension that names a format of its own wins first, so Makefile.md is
	// the markdown it is rather than a Makefile with a suffix.
	for _, named := range qualified {
		if strings.HasPrefix(name, named+".") || strings.HasSuffix(name, "."+named) {
			return byName[named]
		}
	}
	return nil
}

// qualified is byName's keys in a fixed order, since a map's is not one.
var qualified = []string{"Dockerfile", "Containerfile", "Makefile", "makefile", "GNUmakefile"}

// formats are the extensions that carry a format this tool does not read. They
// are listed rather than inferred because the alternative is reading a
// Makefile.md as a Makefile, which is what happens when an unknown extension is
// taken for a variant tag.
var formats = map[string]bool{
	".md": true, ".markdown": true, ".rst": true, ".txt": true, ".adoc": true,
	".html": true, ".htm": true, ".xml": true, ".json": true, ".csv": true,
	".lock": true, ".sum": true, ".pdf": true, ".png": true, ".svg": true,
}

var (
	Go = &Language{
		Name:      "go",
		Grammar:   tsgo.Language,
		Comments:  set("comment"),
		Functions: set("function_declaration", "method_declaration", "func_literal"),
		Strict:    true,
		Wrapper:   func() (string, string) { return "package p\nfunc _() {\n", "\n}\n" },
	}
	// Python prose parses as Python: `in`, `is`, `not`, `and` and `or` are
	// operators, so "# DEBUG=False in production" is a clean parse and was
	// being reported as commented-out code. It is judged by structure instead,
	// like YAML, and needs a statement rather than a bare expression.
	Python = &Language{
		Name:       "python",
		Grammar:    tspython.Language,
		Comments:   set("comment"),
		Functions:  set("function_definition", "lambda"),
		Docstrings: true,
	}
	JavaScript = &Language{
		Name:     "javascript",
		Grammar:  tsjs.Language,
		Comments: set("comment"),
		Functions: set("function_declaration", "function_expression", "arrow_function",
			"method_definition", "generator_function", "generator_function_declaration"),
		Strict: true,
	}
	TypeScript = &Language{
		Name:     "typescript",
		Grammar:  tsts.LanguageTypescript,
		Comments: set("comment"),
		Functions: set("function_declaration", "function_expression", "arrow_function",
			"method_definition", "generator_function", "generator_function_declaration"),
		Strict: true,
	}
	TSX = &Language{
		Name:      "tsx",
		Grammar:   tsts.LanguageTSX,
		Comments:  TypeScript.Comments,
		Functions: TypeScript.Functions,
		Strict:    true,
	}
	Rust = &Language{
		Name:      "rust",
		Grammar:   tsrust.Language,
		Comments:  set("line_comment", "block_comment"),
		Functions: set("function_item", "closure_expression"),
		Strict:    true,
	}
	Bash = &Language{
		Name:      "bash",
		Grammar:   tsbash.Language,
		Comments:  set("comment"),
		Functions: set("function_definition"),
	}
	C = &Language{
		Name:      "c",
		Grammar:   tsc.Language,
		Comments:  set("comment"),
		Functions: set("function_definition"),
		Strict:    true,
	}
	CPP = &Language{
		Name:      "c++",
		Grammar:   tscpp.Language,
		Comments:  set("comment"),
		Functions: set("function_definition", "lambda_expression"),
		Strict:    true,
	}
	Java = &Language{
		Name:      "java",
		Grammar:   tsjava.Language,
		Comments:  set("line_comment", "block_comment"),
		Functions: set("method_declaration", "constructor_declaration", "lambda_expression"),
		Strict:    true,
	}
	Ruby = &Language{
		Name:      "ruby",
		Grammar:   tsruby.Language,
		Comments:  set("comment"),
		Functions: set("method", "singleton_method", "block", "do_block", "lambda"),
	}
	// yaml carries no functions, so a Kubernetes manifest, a chart, or a values
	// file is judged on its documentation and on the config left commented out.
	YAML = &Language{
		Name:      "yaml",
		Grammar:   tsyaml.Language,
		Comments:  set("comment"),
		Functions: set(),
		Templated: true,
	}
	// Values is a Helm chart's values.yaml, which is YAML in every respect but
	// one: a chart publishes the settings it accepts by writing them commented
	// out, so a commented structure there is the file doing its job.
	//
	// The name is YAML's, so every predicate keyed on the language still finds
	// it, and only the commented-out-code rule reads the flag. Hand-judged over
	// 24 repositories, values.yaml supplied 51 findings and no true positive;
	// the two the rule gets right in YAML are both CI workflows.
	Values = &Language{
		Name:      "yaml",
		Grammar:   tsyaml.Language,
		Comments:  set("comment"),
		Functions: set(),
		Templated: true,
		Registers: true,
	}
	// hcl has no function bodies, so every comment in a Terraform file is judged
	// as documentation of the block it sits in.
	HCL = &Language{
		Name:      "hcl",
		Grammar:   tshcl.Language,
		Comments:  set("comment"),
		Functions: set(),
		Strict:    true,
	}
	// A Dockerfile and a Makefile are named rather than extended, and both are
	// written by agents constantly. Neither has function bodies; a Makefile's
	// recipes are shell, which this does not descend into.
	Dockerfile = &Language{
		Name:      "dockerfile",
		Grammar:   tsdocker.GetLanguage,
		Comments:  set("comment"),
		Functions: set(),
	}
	Makefile = &Language{
		Name:      "make",
		Grammar:   tsmake.GetLanguage,
		Comments:  set("comment"),
		Functions: set(),
	}
	PHP = &Language{
		Name:     "php",
		Grammar:  tsphp.LanguagePHP,
		Comments: set("comment"),
		Functions: set("function_definition", "method_declaration",
			"anonymous_function_creation_expression", "anonymous_function", "arrow_function"),
		Strict: true,
	}
)

var byExtension = map[string]*Language{
	".go":   Go,
	".py":   Python,
	".pyi":  Python,
	".js":   JavaScript,
	".mjs":  JavaScript,
	".cjs":  JavaScript,
	".jsx":  JavaScript,
	".ts":   TypeScript,
	".mts":  TypeScript,
	".cts":  TypeScript,
	".tsx":  TSX,
	".rs":   Rust,
	".sh":   Bash,
	".bash": Bash,
	".c":    C,
	".h":    C,
	".cc":   CPP,
	".cpp":  CPP,
	".cxx":  CPP,
	".hpp":  CPP,
	".hh":   CPP,
	".java": Java,
	".rb":   Ruby,
	".php":  PHP,
	".yaml": YAML,
	".yml":  YAML,
	// A chart's helpers and a helmfile are YAML written through a template, and
	// the grammar reads them once the actions are blanked. Skipping them by
	// extension left `templates/_helpers.tpl` unread in every chart.
	".tpl":    YAML,
	".gotmpl": YAML,
	".tftpl":  YAML,
	// A .j2 or a .tftpl is usually not YAML at all — a systemd unit, an nftables
	// ruleset, redis.conf. It is read as YAML anyway because `#` opens a comment
	// in all of them and the rule that runs here parses the comment's own text,
	// not the file's. What keeps it honest is that a unit file's `# Description=`
	// reads as a heading and `# description: the egress gateway` reads as prose,
	// so neither becomes a finding. Measured across 114 such files: none did.
	".j2":     YAML,
	".tf":     HCL,
	".tfvars": HCL,
	".hcl":    HCL,
	".mk":     Makefile,
}

// byName reads the files that carry their language in their name.
var byName = map[string]*Language{
	"Dockerfile":    Dockerfile,
	"Containerfile": Dockerfile,
	"Makefile":      Makefile,
	"makefile":      Makefile,
	"GNUmakefile":   Makefile,
	"values.yaml":   Values,
	"values.yml":    Values,
}

func set(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}
