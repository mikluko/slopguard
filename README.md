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
| restatement | `// close the connection` above `conn.Close()` |
| self-justification | `// kept for backwards compatibility` |
| narration | `// first we parse the input, then we walk the tree` |
| commented-out code | a comment that parses as source, or as YAML config |
| length | documentation running past five sentences |

The first three are read by the model. The last two are structural, and so is
half of restatement: a comment whose content words are already spelled by the
identifiers on the line below it is a restatement on the evidence, with no model
involved.

## What it does not catch, and why

A change-event explanation — `// we now use the pooled client`, `// this fixes
the nil panic` — belongs in the commit message, and catching one was the
original point of this tool. It is not in here, because it could not be made to
work.

The distinction is present in the embedding. Across twenty pairs stating one
claim first as contract and then as change event, the change-event member
projects further along the fitted direction in nineteen. But framing moves a
sentence about a quarter as far as its subject does, so averaging a class of
comments about unrelated subjects cancels the framing and leaves the topic:
directions fitted from two disjoint halves of real comments agree at cos +0.32
for this class, against +0.58 for self-justification and +0.65 for restatement.
An L2 logistic head over the same vectors reaches the same +0.32 and no
held-out threshold at even 0.70 precision. Exemplars rewritten in the register
real comments use get to 0.70 recall at 0.64 precision, and every false positive
is a contract about state that changes over time — what the direction learns is
*is about change*, not *its truth condition is in the past*.

A phrase list is worse than nothing. `// returns every durable the consumer no
longer runs` is a contract that spells a marker, and across 33 files of a
production service every one of the seven findings a phrase list produced was
that shape.

So the tool says nothing about change-event comments rather than guessing at
them. The labelled examples are kept in `heldout_test.go` for whatever tries
next.

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
all-MiniLM-L6-v2 (see `assets/PROVENANCE`). Each class owns a direction —
`unit(centroid(class) - centroid(contract))`, fitted from the labelled corpus —
and a sentence fires the class whose score along that direction clears its
threshold by the most.

Listing exemplars and taking the nearest was tried first and does not
generalise: on comments from repositories that took no part in tuning it nudged
prose seven times, missed every change-event comment, caught 3 of 30
restatements, and every false positive outscored every true positive. A set of
exemplars is a set of points, and the thing being recognised is a direction.

Sitting inside a function body lowers the threshold and moves nothing else.
Prose is harder to justify there, but a line pointing at a constraint enforced
elsewhere still earns its place.

The directions are fitted at build time into `assets/head.bin`, six kilobytes,
so an invocation embeds only the sentences in front of it. Re-embedding the
corpus on every hook call was most of what a run used to cost.

## Known limits

- Change-event comments, as above.
- A five-word fragment carries too little for an embedding to place.
- Helm templates whose comments sit inside a block action.
- Recall is modest by construction. Held out, the classes catch between a sixth
  and two thirds of what they should, at the precision the thresholds are fitted
  for. A comment this tool passes over is not a comment it approves of.

## Calibration

Thresholds are fitted, not chosen. Each class gets the lowest score at which it
still fires at 0.85 precision on the fitting corpus, and 0.85 itself is measured:
`go test -v -run TestCalibrate` fits at 0.70 through 0.95 and reports what each
costs on comments the fit never saw. Below 0.85 the tool starts nudging prose
that was right; above it, recall falls and nothing is bought.

The corpus is in `corpus.go` (hand-written) and `mined.go` (harvested and
labelled). Half of the harvest fits; the other half is held out in
`heldout_test.go` and nothing fits from it. `go test -v -run TestHeldOut` reports
precision and recall per class against that half, and fails if the share of
contract prose left alone drops.

Measured on 114 held-out comments: contract prose left alone 75 of 75, with
restatement at recall 0.67, self-justification 0.50 and narration 0.17, each at
precision 1.0. On 33 files of a production Go service, 9 findings where the
first working version produced 48. On 9934 YAML files of an infrastructure
repository, 104 findings across 15552 comment lines.

Editing the corpus changes every score, so `assets/head.bin` carries a
fingerprint of the text it was fitted from and `go test` fails when the two
disagree. Refit with `go test -run TestHeadAsset -update`.

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
