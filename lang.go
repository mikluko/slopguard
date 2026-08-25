package main

import (
	"path/filepath"
	"strings"
	"unsafe"

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
	// structure names the node kinds that make a cleanly parsed comment code
	// rather than prose, for a language where prose parses as a plain value.
	structure map[string]bool
	// templated reports whether files in this language are commonly written
	// through a template whose actions the grammar cannot read.
	templated bool
}

// lookup returns the language to parse path as, or nil when the extension
// carries no grammar.
func lookup(path string) *language {
	return byExtension[strings.ToLower(filepath.Ext(path))]
}

var (
	golang = &language{
		name:      "go",
		grammar:   tsgo.Language,
		comments:  set("comment"),
		functions: set("function_declaration", "method_declaration", "func_literal"),
		strict:    true,
	}
	python = &language{
		name:      "python",
		grammar:   tspython.Language,
		comments:  set("comment"),
		functions: set("function_definition", "lambda"),
		strict:    true,
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
		structure: set("block_mapping", "block_sequence", "flow_mapping", "flow_sequence"),
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
	".go":     golang,
	".py":     python,
	".pyi":    python,
	".js":     javascript,
	".mjs":    javascript,
	".cjs":    javascript,
	".jsx":    javascript,
	".ts":     typescript,
	".mts":    typescript,
	".cts":    typescript,
	".tsx":    tsx,
	".rs":     rust,
	".sh":     bash,
	".bash":   bash,
	".c":      clang,
	".h":      clang,
	".cc":     cpp,
	".cpp":    cpp,
	".cxx":    cpp,
	".hpp":    cpp,
	".hh":     cpp,
	".java":   java,
	".rb":     ruby,
	".php":    php,
	".yaml":   yaml,
	".yml":    yaml,
	".tf":     hcl,
	".tfvars": hcl,
	".hcl":    hcl,
}

func set(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}
