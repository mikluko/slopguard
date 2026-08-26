package main

import (
	"path/filepath"
	"strings"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

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

// language binds a grammar to the node kinds slopguard reads out of its trees.
type language struct {
	name      string
	grammar   func() unsafe.Pointer
	comments  map[string]bool
	functions map[string]bool
	// strict reports whether prose reliably fails to parse as source in this
	// language, which is what makes the commented-out-code rule safe to apply.
	strict bool
	// evidence decides, for a language whose prose parses as source, whether a
	// cleanly parsed comment is code somebody commented out or a sentence that
	// merely reads as code. It is given the comment, the parse of its text, that
	// text, and the file the comment came from.
	evidence func(c comment, parsed *tree_sitter.Node, body, src []byte) bool
	// wrapper puts a fragment where this language's grammar expects statements.
	// Code is commented out from inside a function far more often than from
	// file scope, and the two parse by different rules: tree-sitter-go reads
	// `fmt.Println("x")` at file scope as a conversion and inside a function as
	// a call. Recovery differs too, so an unclosed brace is an error at file
	// scope and a MISSING node in a body.
	//
	// A language without one is parsed bare, which is what every language here
	// did before any of them was measured.
	wrapper func() (prefix, suffix string)
	// legal reports whether a fragment's statements could have been compiled.
	// A grammar is context-free and accepts what the language does not: a bare
	// comparison is a legal parse and an illegal statement, so `// f == g` is a
	// relation somebody wrote down rather than a line they switched off.
	legal func(statements []*tree_sitter.Node, body []byte) bool
	// templated reports whether files in this language are commonly written
	// through a template whose actions the grammar cannot read.
	templated bool
	// docstrings reports whether this language documents with a string in
	// statement position rather than with a comment. Python does, and it is
	// where essentially all of its standing documentation lives.
	docstrings bool
}

// lookup returns the language to parse path as, or nil when nothing here reads
// it. A file with no useful extension is looked up by name, which is the only
// way a Dockerfile or a Makefile is ever identified.
func lookup(path string) *language {
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
	golang = &language{
		name:      "go",
		grammar:   tsgo.Language,
		comments:  set("comment"),
		functions: set("function_declaration", "method_declaration", "func_literal"),
		strict:    true,
		wrapper:   func() (string, string) { return "package p\nfunc _() {\n", "\n}\n" },
		legal:     goLegal,
	}
	// Python prose parses as Python: `in`, `is`, `not`, `and` and `or` are
	// operators, so "# DEBUG=False in production" is a clean parse and was
	// being reported as commented-out code. It is judged by structure instead,
	// like YAML, and needs a statement rather than a bare expression.
	python = &language{
		name:       "python",
		grammar:    tspython.Language,
		comments:   set("comment"),
		functions:  set("function_definition", "lambda"),
		evidence:   pythonCode,
		docstrings: true,
	}
	javascript = &language{
		name:     "javascript",
		grammar:  tsjs.Language,
		comments: set("comment"),
		functions: set("function_declaration", "function_expression", "arrow_function",
			"method_definition", "generator_function", "generator_function_declaration"),
		strict: true,
	}
	typescript = &language{
		name:     "typescript",
		grammar:  tsts.LanguageTypescript,
		comments: set("comment"),
		functions: set("function_declaration", "function_expression", "arrow_function",
			"method_definition", "generator_function", "generator_function_declaration"),
		strict: true,
	}
	tsx = &language{
		name:      "tsx",
		grammar:   tsts.LanguageTSX,
		comments:  typescript.comments,
		functions: typescript.functions,
		strict:    true,
	}
	rust = &language{
		name:      "rust",
		grammar:   tsrust.Language,
		comments:  set("line_comment", "block_comment"),
		functions: set("function_item", "closure_expression"),
		strict:    true,
	}
	bash = &language{
		name:      "bash",
		grammar:   tsbash.Language,
		comments:  set("comment"),
		functions: set("function_definition"),
	}
	clang = &language{
		name:      "c",
		grammar:   tsc.Language,
		comments:  set("comment"),
		functions: set("function_definition"),
		strict:    true,
	}
	cpp = &language{
		name:      "c++",
		grammar:   tscpp.Language,
		comments:  set("comment"),
		functions: set("function_definition", "lambda_expression"),
		strict:    true,
	}
	java = &language{
		name:      "java",
		grammar:   tsjava.Language,
		comments:  set("line_comment", "block_comment"),
		functions: set("method_declaration", "constructor_declaration", "lambda_expression"),
		strict:    true,
	}
	ruby = &language{
		name:      "ruby",
		grammar:   tsruby.Language,
		comments:  set("comment"),
		functions: set("method", "singleton_method", "block", "do_block", "lambda"),
	}
	// yaml carries no functions, so a Kubernetes manifest, a chart, or a values
	// file is judged on its documentation and on the config left commented out.
	yaml = &language{
		name:      "yaml",
		grammar:   tsyaml.Language,
		comments:  set("comment"),
		functions: set(),
		evidence:  yamlConfig,
		templated: true,
	}
	// hcl has no function bodies, so every comment in a Terraform file is judged
	// as documentation of the block it sits in.
	hcl = &language{
		name:      "hcl",
		grammar:   tshcl.Language,
		comments:  set("comment"),
		functions: set(),
		strict:    true,
	}
	// A Dockerfile and a Makefile are named rather than extended, and both are
	// written by agents constantly. Neither has function bodies; a Makefile's
	// recipes are shell, which this does not descend into.
	dockerfile = &language{
		name:      "dockerfile",
		grammar:   tsdocker.GetLanguage,
		comments:  set("comment"),
		functions: set(),
		evidence:  dockerCode,
	}
	makefile = &language{
		name:      "make",
		grammar:   tsmake.GetLanguage,
		comments:  set("comment"),
		functions: set(),
	}
	php = &language{
		name:     "php",
		grammar:  tsphp.LanguagePHP,
		comments: set("comment"),
		functions: set("function_definition", "method_declaration",
			"anonymous_function_creation_expression", "anonymous_function", "arrow_function"),
		strict: true,
	}
)

var byExtension = map[string]*language{
	".go":   golang,
	".py":   python,
	".pyi":  python,
	".js":   javascript,
	".mjs":  javascript,
	".cjs":  javascript,
	".jsx":  javascript,
	".ts":   typescript,
	".mts":  typescript,
	".cts":  typescript,
	".tsx":  tsx,
	".rs":   rust,
	".sh":   bash,
	".bash": bash,
	".c":    clang,
	".h":    clang,
	".cc":   cpp,
	".cpp":  cpp,
	".cxx":  cpp,
	".hpp":  cpp,
	".hh":   cpp,
	".java": java,
	".rb":   ruby,
	".php":  php,
	".yaml": yaml,
	".yml":  yaml,
	// A chart's helpers and a helmfile are YAML written through a template, and
	// the grammar reads them once the actions are blanked. Skipping them by
	// extension left `templates/_helpers.tpl` unread in every chart.
	".tpl":    yaml,
	".gotmpl": yaml,
	".tftpl":  yaml,
	".j2":     yaml,
	".tf":     hcl,
	".tfvars": hcl,
	".hcl":    hcl,
	".mk":     makefile,
}

// byName reads the files that carry their language in their name.
var byName = map[string]*language{
	"Dockerfile":    dockerfile,
	"Containerfile": dockerfile,
	"Makefile":      makefile,
	"makefile":      makefile,
	"GNUmakefile":   makefile,
}

func set(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}
