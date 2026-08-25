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
| commented-out code | a comment that parses as source, or as YAML config |
| length | documentation running past five sentences |

The first two are read by the model. The last two are structural, and so is half
of restatement: a comment whose content words are already spelled by the
identifiers on the line below it is a restatement on the evidence, with no model
involved.

A narration class was tried and dropped too, more cheaply than the one below.
Its direction converged, but it fired once on the held-out set, never across
three production repositories, and not on its own training sentence. A class
that says almost nothing is carrying risk for nothing.

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
for this class, against +0.59 for self-justification and +0.69 for restatement.
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
next, and the full diagnosis — three hypotheses, the numbers that refuted each,
and the two things worth trying — is in `docs/history-class.md`.

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
YAML and Terraform. A file nothing here reads produces nothing, and so does a
file that fails to parse — a broken tree is not evidence of a comment.

YAML is judged as configuration: a stacked comment that parses as a mapping or a
sequence is config left behind. Helm templates are read as the YAML they become
— template actions are blanked to spaces of the same width before the parse, so
a manifest opening with `{{- if }}` keeps its comments and its byte offsets. A
`.tpl` file is deliberately not mapped: those are mostly `{{ define }}` bodies,
and calling them YAML would be a false claim of coverage.

`Dockerfile`, `Containerfile` and `Makefile` are read too, by name rather than
by extension. A commented-out Dockerfile instruction is told from prose by its
case: every Dockerfile writes `RUN` and `COPY` in capitals, and no comment
writes prose that way, which matters because `# copy the buffer first` parses as
a perfectly good `COPY`.

## Install

Not published yet, so build it:

```sh
git clone <this repo> && cd slopguard && go build .
brew install onnxruntime
```

The formula in `mikluko/homebrew-tap` is written and waiting on the first
release tag; when that lands, `brew install mikluko/tap/slopguard` is the
install and it depends on `onnxruntime` for you. Either way the binary dlopens
the library at run time, looking in Homebrew's prefix on both architectures,
Linuxbrew's, and the two places a Linux package manager puts it.

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

| Variable | Effect |
| --- | --- |
| `SLOPGUARD_ONNXRUNTIME_LIBRARY` | where to dlopen ONNX Runtime from, when it is in none of the usual places |
| `SLOPGUARD_NO_MODEL` | turns the semantic pass off, leaving the structural rules |
| `SLOPGUARD_STATE` | where the per-session memory of what has already been said lives; empty turns it off |
| `SLOPGUARD_LOG` | a file every finding is appended to, one JSON object per line |

The last two write to disk. `SLOPGUARD_STATE` defaults to `slopguard` under the
user cache directory, and files there that have gone a day without a write are
removed — only files this program wrote, whose names are a single base-36
number, so pointing it at a directory of your own will not empty it.

It loads a native library into its own process and is not a sandbox.

## Contract

Reads the hook payload on stdin, writes a `PostToolUse` result on stdout, always
exits 0. A write it does not object to produces no output at all.

It fails open. A payload it cannot decode, a tool other than `Write`, `Edit` or
`MultiEdit`, an extension with no grammar, a file that is gone or is not a
regular file or is over 2 MB, replacement text it cannot locate in the file, and
a source that does not parse all yield silence, because none of them is evidence
of a comment.

Without ONNX Runtime it does not go silent, it goes stupid: the structural rules
still run, and a phrase list stands in for the one model class it can
approximate. That fallback judges differently from the model in both directions,
and `SLOPGUARD_NO_MODEL=1` is how to see what it says.

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

The directions are fitted at build time into `assets/head.bin`, three kilobytes,
so an invocation embeds only the sentences in front of it. Re-embedding the
corpus on every hook call was most of what a run used to cost: a write with
nothing for the model to read now returns in about 5 ms, and one that reaches it
pays about 90 ms for the ONNX session and 2 ms a sentence after that.

## Known limits

- Change-event comments, as above.
- A five-word fragment carries too little for an embedding to place.
- `Jenkinsfile` and other Groovy: no grammar wired.
- Recall is modest by construction: held out, restatement reaches about half and
  self-justification a quarter, at the precision the thresholds are fitted for.
  A comment this tool passes over is not a comment it approves of.
- A commented block indented under a key whose value is an empty collection is
  documentation, not residue: `podSecurityContext: {}` over `# fsGroup: 2000` is
  how a chart shows what a setting takes, and it is read that way whatever the
  file is called. A commented option under a key that already has values is
  still reported, which on a stock `helm create` scaffold is two findings.
- A finding is remembered when it is named, not when it is acted on. Ignoring a
  nudge silences it for the rest of the session, rewording earns a fresh one, and
  deleting the line goes quiet. The cheapest paths to silence are still ignore
  and delete; the wording argues against both, and nothing enforces it.
- The same replacement text occurring twice in a file is claimed twice. Only the
  bytes an edit changed are attributed to it, but where those bytes appear more
  than once there is nothing in the payload that tells the copies apart.

## Calibration

Thresholds are fitted, not chosen. Each class gets the lowest score at which it
still fires at 0.85 precision on the fitting corpus — over everything above the
cut, and among the examples within 0.02 of it, since cumulative precision alone
lets a clean top of the ranking buy slack that is spent entirely at the margin.

Both numbers are measured rather than picked. `go test -v -run TestCalibrate`
fits at required precisions from 0.70 to 0.95, and `-run TestWindow` at margin
widths from 0 to 0.08; each reports what that choice costs on comments the fit
never saw. Below 0.85 the tool starts nudging prose that was right; above it,
recall falls for nothing. The margin band from 0.01 to 0.02 nudges no held-out
prose at all and still reaches the most true positives.

The corpus is in `corpus.go` (hand-written) and `mined.go` (harvested and
labelled). Half of the harvest fits; the other half is held out in
`heldout_test.go` and nothing fits from it. `go test -v -run TestHeldOut` reports
precision and recall per class against that half, and fails if the share of
contract prose left alone drops.

Measured on 98 held-out comments, each a single sentence: restatement at recall
0.53 and self-justification at 0.25, both at precision 1.0. Those two are
per-sentence rates — a comment of several sentences takes the strongest verdict
among them, so its recall is somewhat higher and is not measured here. Contract
prose is left alone
75 of 75 above a declaration, 74 of 75 at the lower threshold a comment inside a
function body meets, and 25 of 25 when those rows are read three to a comment,
which is the unit a real doc comment arrives in. All three are asserted.

On a production Go service, 27 findings across `internal`, `app` and `pkg`. On
9934 YAML files and 5313 Terraform files of an infrastructure repository, 32
findings between them. On its own source, none.

One caveat on the held-out numbers, because they are what decides whether this
tool is worth running: the held-out half and the fitting half were split row by
row within each source rather than by source, so comments from one file sit on
both sides and the classes see the topics they are tested on. It is an upper
bound, and the repository sweeps above are the group-disjoint check beside it.

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

Tests that judge text skip when ONNX Runtime is absent; the parsing and
structural tests run either way, so a checkout without it still exercises the
language table, the commented-out-code rules and the identifier echo.

`go test -v -run TestStability` reports how far each class's direction moves
when it is fitted from one half of its own examples rather than the other. It
fails below cos 0.45. That is the measurement the deleted change-event class
failed at +0.32 while looking healthy from every other angle.

## License

MIT
