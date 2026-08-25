# slopguard

A Claude Code `PostToolUse` hook that reads the comments a write just added and
says which of them belong somewhere else: in the commit message, in a test, or
nowhere.

## Why

An agent writes comments the way it writes prose, and most of them decay. A line
saying *we now use the pooled client* parses only for a reader who saw the
previous version, and that reader does not exist: the diff shipped with the
commit message, where the sentence belonged. A line saying *increment the
counter* over `n++` was never true of anything but the line below it. A
walkthrough of how a function works is a test that was written as prose instead.

None of that is a lint rule, because none of it is about syntax. What separates
`// the caller has already bounded v, so this cannot overflow` from `// multiply
it by two` is what the sentence is doing, and the only way to tell is to read
it. So slopguard reads it: tree-sitter finds the comments, and a sentence
embedding model decides what each one is by how it compares against a corpus of
comments that earned their place and a corpus of comments that did not.

It objects and never blocks. `PostToolUse` fires after the write, the finding
comes back as context, and what the agent does about it is the agent's business.

## What it says something about

| Class | What fires it |
| --- | --- |
| change event | `// we now use a pooled client instead of dialing per request` |
| self-justification | `// kept for backwards compatibility` |
| restatement | `// close the connection` above `conn.Close()` |
| narration | `// first we parse the input, then we walk the tree` |
| commented-out code | a comment that parses as source, or as YAML config |
| length | documentation running past five sentences |

The first four are decided by the model. The last two are structural: a comment
that parses cleanly as code in its own language, and a sentence count.

## What it leaves alone

Everything that states a contract. A precondition, an invariant, a failure mode,
a cost the signature cannot show, a constraint enforced somewhere the reader
cannot see, a reason a non-obvious choice was made. The negative corpus is drawn
from the Go standard library, the DOOM sources and Django, plus the register of
Kubernetes and Terraform configuration, and it includes the hard cases:

```
the stop method is no longer necessary to help the garbage collector
kept for binary compatibility and exported only for the type descriptors
returns every durable the consumer no longer runs
```

All three spell a change-event marker and all three are contracts. A phrase list
flags every one of them, which is why there is a model here at all.

Machine-readable comments are skipped outright: shebangs, build constraints,
`//go:generate`, `//nolint`, `// Deprecated:`, `# noqa`, `# type:`,
`// eslint-disable`, and their neighbours.

## Languages

Go, Python, JavaScript, TypeScript, TSX, Rust, C, C++, Java, Ruby, PHP, shell,
and YAML. A file whose extension carries no grammar produces nothing, and so
does a file that fails to parse — a broken tree is not evidence of a comment.

YAML is judged as configuration: a stacked comment that parses as a mapping or a
sequence is config left behind. Helm templates are a known gap. A `{{- if }}` at
column zero makes the YAML grammar drop every comment in the file, so
`templates/*.yaml` is unguarded while `values.yaml` and `Chart.yaml` are read
normally.

## Install

```sh
brew install mikluko/tap/slopguard
```

The formula depends on `onnxruntime`. From source, `go install
github.com/mikluko/slopguard@latest` builds against whatever
`/opt/homebrew/lib/libonnxruntime.dylib` is installed.

## Configure

Add a `Write|Edit|MultiEdit` matcher to `PostToolUse` in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit|MultiEdit",
        "hooks": [
          { "type": "command", "command": "slopguard", "timeout": 10 }
        ]
      }
    ]
  }
}
```

Use an absolute path if sessions start without the Homebrew prefix on `PATH`.
Restart the session to load it.

Two environment variables: `SLOPGUARD_ONNXRUNTIME_LIBRARY` points at the shared
library when it is not in Homebrew's prefix, and `SLOPGUARD_NO_MODEL` turns the
semantic pass off, leaving the structural rules and a phrase list.

## Contract

Reads the hook payload on stdin, writes a `PostToolUse` result on stdout, always
exits 0. A write it does not object to produces no output at all.

It fails open, in every direction. A payload it cannot decode, a tool other than
`Write`, `Edit` or `MultiEdit`, an extension with no grammar, a file that is
gone or is not a regular file or is over 2 MB, replacement text it cannot locate
in the file, a source that does not parse, a missing ONNX Runtime: all of them
yield silence, because none of them is evidence of a comment.

A finding names the line and the rule, at most three per write, strongest first:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PostToolUse",
    "additionalContext": "slopguard read the comments this write added to internal/store.go.\n\n  line 42  restates what the code already says: the line below is the documentation\n\nTake each line with Edit on internal/store.go before the next step. …"
  },
  "systemMessage": "slopguard: 1 comment in internal/store.go to reconsider"
}
```

## How the judgment works

Each comment is cut into sentences, and each sentence is embedded with
all-MiniLM-L6-v2 (see `assets/PROVENANCE`). A sentence scores against each class
and against the contract corpus as the mean of its two nearest exemplars, minus
what an unrelated sentence scores against that same set — the correction matters
because affinity grows with the size of the set, and the contract corpus is
several times the size of any one class. A class fires when its corrected score
clears that class's floor and beats the contract score by a margin.

Sitting inside a function body lowers the floor and moves nothing else. Prose is
harder to justify there, but a line pointing at a constraint enforced elsewhere
still earns its place, and letting position erode the contract comparison would
disable the negative class exactly where the tool objects most.

The exemplar vectors are computed at build time into `assets/exemplars.bin`, so
an invocation embeds only the sentences in front of it. Re-embedding the corpus
on every hook call was most of what a run used to cost.

## Known limits

- A change event phrased as a statement about a value reads as configuration
  documentation. `# this was bumped from 3 when the disruption budget changed`
  is not caught.
- A five-word fragment carries too little for an embedding to place.
- Python docstrings are strings rather than comments, and are not read yet.
- Helm templates, as above.
- `.tf` and `.hcl` are not wired yet.

## Calibration

The class floors are set per class, above the highest false positive that class
produced on a labelled set of real comments. The labelled set is in
`semantic_test.go` and the exemplars are in `corpus.go`; `go test -v -run
TestScores` prints the score of every labelled sentence against every class,
which is what a floor is chosen from.

Measured on 33 files of a production Go service: 48 findings before the corpus
and the scoring were calibrated, 11 after. On 84 YAML files of a Flux
repository, 1 finding across 107 comment lines.

Editing an exemplar changes every score, so `assets/exemplars.bin` carries a
fingerprint of the corpus it was built from and `go test` fails when the two
disagree. Regenerate with `go test -run TestExemplarAsset -update`.

## Development

```sh
go test ./...
```

The tables are the specification: `scan_test.go` for what fires per language,
`semantic_test.go` for what the model must and must not recognise. A false
positive or a missed comment is a row added there first.

Tests touching the model skip when ONNX Runtime is absent.

## License

MIT
