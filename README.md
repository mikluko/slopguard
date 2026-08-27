# slopguard

A Claude Code `PostToolUse` hook that reads the comments a write just added and says which of them belong somewhere
else: in the commit message, in a test, or nowhere.

## TL;DR

**Why.** An agent writes comments that read fine today and mislead after the next commit: notes about what changed,
restatements of the line below, prose walking through what a test should have asserted. Nobody re-checks a comment, so
they rot in place and the next reader believes them. slopguard reads every comment a write adds and names the ones whose
claim belongs elsewhere, at the moment it is cheapest to move. It objects and never blocks.

```
line 3  padded documentation: cut "This function takes a value and returns a value.", which says nothing the
        signature does not
line 7  restates what the code already says: the line below is the documentation
```

**Try it on a repository, wiring nothing.** Given paths instead of a hook payload it judges those files and prints what
it finds, one finding per line:

```sh
brew install mikluko/tap/slopguard
cd <a repo>
git ls-files -z | xargs -0 slopguard -v
```

`-v` prints each comment with the line under it, which is what you need in order to say whether a finding is right:

```
internal/store/lifecycle_test.go:63	echo	0.950	restates what the code already says: the line below is the documentation
	| // Reason/until are postponed-only.
	| {name: "ready with reason", from: Item{Status: str(StatusDraft)}, status: StatusReady, ...},
```

A file in a language it does not read produces nothing, so passing the whole index is safe. The sweep writes nothing and
remembers nothing; it is the same judgment the hook makes, without the hook. For scale, the Go standard library gives
142 findings over 4,065 files, all of them commented-out code.

**What ships is `leftover` alone.** `echo`, `hollow`, `tautology` and `compat` are behind `SLOPGUARD_WIDER=1`, off by
default. On a corpus of comments other people deleted or kept, `leftover` catches 20 and nudges 5 where the whole set
catches 21 and nudges 25: one extra catch for twenty extra false positives. The two semantic classes are also what
loads the 86 MB model, which is most of a second per write. `docs/metric.md` carries the measurement and the argument
for why this is a default rather than a deletion.

**Wire it.** Add the `Write|Edit|MultiEdit` matcher under [Configure](#configure) to `~/.claude/settings.json` and
restart the session.

## Why this is not a lint rule

None of it is about syntax. What separates `// the caller has already bounded v, so this cannot overflow` from
`// multiply it by two` is what the sentence is doing, and the only way to tell is to read it. So slopguard reads it:
tree-sitter finds the comments, and a sentence embedding model decides what each one is by how it compares against a
corpus of comments that earned their place and a corpus of comments that did not.

`PostToolUse` fires after the write, the finding comes back as context, and what the agent does about it is the agent's
business.

## What it says something about

| Class              | What fires it                                                    |
|--------------------|------------------------------------------------------------------|
| restatement        | `// close the connection` above `conn.Close()`                   |
| self-justification | `// kept for backwards compatibility`                            |
| commented-out code | a comment that parses as source the compiler would take          |
| padded documentation | a sentence past the first that says nothing the signature does |

The first two are read by the model. The last two are structural, and so is half of restatement: a comment whose content
words are already spelled by the identifiers on the line below it is a restatement on the evidence, with no model
involved.

Change-event comments — `// we now use the pooled client` — were the original point of this tool and are not in it,
because the class could not be made to work. [docs/limits.md](docs/limits.md) says what was tried.

## What it leaves alone

Everything that states a contract. A precondition, an invariant, a failure mode, a cost the signature cannot show, a
constraint enforced somewhere the reader cannot see, a reason a non-obvious choice was made. The negative corpus is drawn
from the Go standard library, the DOOM sources and Django, plus the register of Kubernetes and Terraform configuration,
and it includes the hard cases:

```
the stop method is no longer necessary to help the garbage collector
kept for binary compatibility and exported only for the type descriptors
returns every durable the consumer no longer runs
```

All three spell a change-event marker and all three are contracts. A phrase list flags every one of them, which is why
there is a model here at all.

Machine-readable comments are skipped outright: shebangs, build constraints, `//go:generate`, `//nolint`,
`// Deprecated:`, `# noqa`, `# type:`, `// eslint-disable`, and their neighbours.

A licence notice is exempt from every rule. Some of them ask in their own text to be preserved.

A comment sharing a line with code is exempt from the rules that read it against that line. The words it shares with the
line are what the note is about rather than evidence it repeats one, and the notation those notes use is the notation the
commented-out-code rule would otherwise read as source:

```
num -= old.cap - old.len // preserve memory of old[old.len:old.cap]
x2 := Sqrt(x1)           // x2 = sqrt(1 - x*x)
B2 = 696219795           // (664-0.03306235651)*2**20
```

The cost is `i++ // increment i`, which nothing else here catches.

## Languages

Go, Python, JavaScript, TypeScript, TSX, Rust, C, C++, Java, Ruby, PHP, shell, YAML and Terraform, plus `Dockerfile`,
`Containerfile` and `Makefile` by name rather than by extension. A file nothing here reads produces nothing, and so does
a file that fails to parse — a broken tree is not evidence of a comment.

Two languages are read as something other than themselves. Helm templates are read as the YAML they become, so a manifest
opening with `{{- if }}` keeps its comments; a `.tpl` file is deliberately not mapped, since those are mostly
`{{ define }}` bodies and calling them YAML would be a false claim of coverage. A commented-out Dockerfile instruction is
told from prose by its case, because `# copy the buffer first` parses as a perfectly good `COPY`.

## Install

```sh
brew install mikluko/tap/slopguard
```

The formula depends on `onnxruntime` and pulls it in. To build instead:

```sh
git clone https://github.com/mikluko/slopguard && cd slopguard && go build .
brew install onnxruntime
```

Either way the binary dlopens the library at run time, looking in Homebrew's prefix on both architectures, Linuxbrew's,
and the two places a Linux package manager puts it.

## Configure

Add a `Write|Edit|MultiEdit` matcher to `PostToolUse` in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit|MultiEdit",
        "hooks": [
          {
            "type": "command",
            "command": "slopguard",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

Use an absolute path if sessions start without the Homebrew prefix on `PATH`. Restart the session to load it.

| Variable                        | Effect                                                                               |
|---------------------------------|--------------------------------------------------------------------------------------|
| `SLOPGUARD_ONNXRUNTIME_LIBRARY` | where to dlopen ONNX Runtime from, when it is in none of the usual places            |
| `SLOPGUARD_NO_MODEL`            | turns the semantic pass off, leaving the structural rules                            |
| `SLOPGUARD_STATE`               | where the per-session memory of what has already been said lives; empty turns it off |
| `SLOPGUARD_LOG`                 | a file every finding is appended to, one JSON object per line                        |

The last two write to disk. `SLOPGUARD_STATE` defaults to `slopguard` under the user cache directory, and files there
that have gone a day without a write are removed — only files this program wrote, whose names are a single base-36
number, so pointing it at a directory of your own will not empty it.

It loads a native library into its own process and is not a sandbox.

## Contract

Reads the hook payload on stdin, writes a `PostToolUse` result on stdout, always exits 0. A write it does not object to
produces no output at all.

It fails open. A payload it cannot decode, a tool other than `Write`, `Edit` or `MultiEdit`, an extension with no
grammar, a file that is gone or is not a regular file or is over 2 MB, replacement text it cannot locate in the file, and
a source that does not parse all yield silence, because none of them is evidence of a comment.

Without ONNX Runtime it does not go silent, it goes stupid: the structural rules still run, and a phrase list stands in
for the one model class it can approximate. That fallback judges differently from the model in both directions, and
`SLOPGUARD_NO_MODEL=1` is how to see what it says.

A finding names the line and the rule, at most three per write, strongest first:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PostToolUse",
    "additionalContext": "Edit store.go before your next step: these comments it just gained say things that belong elsewhere.\n\n  line 42  restates what the code already says: the line below is the documentation\n\nPer line: if the claim still binds the next editor, restate it as the symbol's contract or as a test. If it only records this change, cut it and carry it into the commit message. What is judged is where the claim lives, not which words carry it, so rewording is not a fix. If a line is right where it is, keep it and say so in one line."
  },
  "systemMessage": "slopguard: 1 comment in store.go to reconsider"
}
```

A write with nothing for the model to read returns in single-digit to low-teens milliseconds; one that reaches the model
pays 115 to 145 ms, most of it opening the ONNX session.

## Limits

- Recall is modest by construction: held out, restatement reaches about half and self-justification a quarter, at the
  precision the thresholds are fitted for. **A comment this tool passes over is not a comment it approves of.**
- Change-event comments are not caught. Neither is implementation narrative.
- A step comment inside a long function is a finding, and plenty of engineers would defend it. On the Go standard library
  that is the largest class the tool has, 623 of 1034.
- Rust documentation never reaches the padding rule, so a sweep reporting nothing on Rust is reporting that the rule did
  not run.
- A finding is remembered when it is named, not when it is acted on: ignoring a nudge silences it for the session.
- Generated files are not skipped unless they carry a `//go:generate` marker.

[docs/limits.md](docs/limits.md) has the rest, including the shapes it gets wrong and the classes that were tried and
dropped.

## Development

```sh
go test ./...
```

Given file paths instead of a payload, the tool judges those files and prints what it finds, one finding per line. `-v`
prints the comment and the line under it as well, which is what a sweep has to show before anyone can say whether it is
right:

```sh
slopguard -v $(git ls-files '*.go')
```

The tables are the specification: `internal/rule/rule_test.go` for what fires per language, with
`internal/rule/python_test.go` and `internal/rule/yaml_test.go` beside it for the two languages whose prose parses, and
`internal/model/semantic_test.go` for what the model must and must not recognise. A false positive or a missed comment is
a row added there first.

Tests that judge text skip when ONNX Runtime is absent; the parsing and structural tests run either way, so a checkout
without it still exercises the language table, the commented-out-code rules and the identifier echo.

`go test ./internal/model -v -run TestStability` reports how far each class's direction moves when it is fitted from one
half of its own examples rather than the other. It fails below cos 0.45. That is the measurement the deleted change-event
class failed at +0.32 while looking healthy from every other angle.

[docs/design.md](docs/design.md) is how the judgment works and how the thresholds were fitted.

## License

The source is MIT. The binary is not only the source: it embeds all-MiniLM-L6-v2, which is Apache 2.0, so every build
redistributes Apache 2.0 material and carries that licence with it. The text is in
`internal/model/assets/LICENSE.apache-2.0` and `internal/model/assets/PROVENANCE` is the notice, with the revision and
the checksums to verify what was embedded.
