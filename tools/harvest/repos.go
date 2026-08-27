package main

// A Repo is one public repository to harvest, with the licence its text comes
// under. The licence is recorded on every row because the corpus embeds other
// people's prose, and a row nobody can trace to its terms cannot be shipped.
type Repo struct {
	Name    string
	License string
}

// URL is where the repository is cloned from.
func (r Repo) URL() string { return "https://github.com/" + r.Name + ".git" }

// Dir is the directory name the clone is kept under.
func (r Repo) Dir() string {
	out := []byte(r.Name)
	for i, c := range out {
		if c == '/' {
			out[i] = '_'
		}
	}
	return string(out)
}

// defaults is the harvest set: permissively licensed repositories spread across
// the languages slopguard parses, none of them connected to this project.
//
// The spread is deliberate rather than a sample of what is popular. A corpus
// drawn from one language measures that language's comment conventions, and the
// classes are supposed to be about where an explanation belongs rather than
// about how Go spells a doc comment. The weighting favours the languages an
// agent is most often asked to write.
var defaults = []Repo{
	// Go
	{"spf13/cobra", "Apache-2.0"},
	{"sirupsen/logrus", "MIT"},
	{"gin-gonic/gin", "MIT"},
	{"go-chi/chi", "MIT"},
	{"etcd-io/bbolt", "MIT"},
	{"prometheus/client_golang", "Apache-2.0"},
	{"grpc/grpc-go", "Apache-2.0"},

	// Python
	{"pallets/flask", "BSD-3-Clause"},
	{"psf/requests", "Apache-2.0"},
	{"encode/httpx", "BSD-3-Clause"},
	{"pydantic/pydantic", "MIT"},
	{"scrapy/scrapy", "BSD-3-Clause"},

	// TypeScript and JavaScript
	{"axios/axios", "MIT"},
	{"expressjs/express", "MIT"},
	{"date-fns/date-fns", "MIT"},
	{"vuejs/core", "MIT"},

	// Rust
	{"tokio-rs/tokio", "MIT"},
	{"clap-rs/clap", "MIT OR Apache-2.0"},

	// C, Java, Ruby
	{"jqlang/jq", "MIT"},
	{"google/gson", "Apache-2.0"},
	{"sinatra/sinatra", "MIT"},

	// YAML and HCL, which is where the configuration rules are measured
	{"prometheus-community/helm-charts", "Apache-2.0"},
	{"grafana/helm-charts", "Apache-2.0"},
	{"terraform-aws-modules/terraform-aws-vpc", "Apache-2.0"},
}
