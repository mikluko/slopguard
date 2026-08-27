# slopguard

A `PostToolUse` hook that objects to comments whose claim belongs elsewhere. What it judges, it judges about prose, so
almost every change here is a change to a judgment and has to be measured rather than argued.

## Where a change belongs

Imports run one way and nothing here imports upward:

```
main     -> comment lang rule session
rule     -> comment lang model prose
comment  -> lang prose
model    -> prose
session  -> prose
lang     -> (nothing)
prose    -> (nothing)
```

`comment` turns a file into comments and judges nothing. `rule` decides what to object to. `model` says what it
recognised a sentence as and never what that earns. A change that needs `comment` to know about a finding, or `model` to
know about a rule, is in the wrong package.

`comment.Scan` returns a release beside the comments. A `Comment` is pointers into the parse tree, so reading one after
the release is a use-after-free rather than an empty answer.

## The invariant

Every refactor holds this, and a refactor that moves it is a behaviour change wearing a refactor's commit message:

```sh
cd $(go env GOROOT)/src
find . -name '*.go' -not -name '*_test.go' -not -path '*/testdata/*' -not -path '*/vendor/*' \
  -print0 | xargs -0 -n 300 slopguard > /tmp/sweep.txt
wc -l /tmp/sweep.txt          # 130 over 4,065 files, all leftover
```

Diff the file, not the count: the same total over different lines is not the same behaviour.

The standard library holds no YAML, so it cannot see the exemptions that matter most. The second invariant is a sweep of
the mined clones, where 23 repositories with content and 10 languages give 168 findings; `tools/harvest -clones <dir>`
puts them there, and `go-chi/chi` clones empty, which is why 23 rather than 24. A change that moves the 130 and not the
168, or the other way round, has moved one language and should say which.

Neither number is a score. Hand-judged, the share that is right splits by population rather than averaging: six to nine
in ten on application code, about one in four on compilers and spec implementations, and the figures are fitted to the
sample the exemptions were chosen from rather than estimated on a held-out one. `docs/metric.md` says what that costs.
A change that moves a count has to say which findings it moved, because moving the wrong half and the right half are the
same arithmetic and opposite work.

The tool over its own source returns nothing, which is the cheap version of the same check:

```sh
slopguard *.go internal/*/*.go
```

## Tests

Every test file is one of three shapes, and the name says which:

| Shape | Rule |
|-------|------|
| `x_test.go` | the contract of `x.go`. It asserts, and it fails. |
| `aa_…` | fixtures the other files read. Asserts nothing. |
| `zz_…` | asserts nothing, or asserts only wall-clock. |

The `zz_` rule is a property of the file rather than a plea per file: a run that can only log, or can only fail on a
loaded machine, is a procedure and not a contract. Threshold sweeps, corpus harvests and cost bounds go there. Do not add
an orphan — a `_test.go` with no source beside it and no prefix is the thing this convention exists to prevent.

`internal/model/corpus.go` and `internal/model/mined.go` are data and keep no pair;
`internal/model/exemplars_test.go` and the fingerprint check hold them.

**Tests that judge text skip when ONNX Runtime is absent.** A green run on a machine without it has not exercised the
model, so check `brew list onnxruntime` before believing a passing suite. The structural rules run either way, which is
deliberate: running without the model is a supported mode, not a degraded one.

## The fitted head

`internal/model/assets/head.bin` is fitted from the corpus at build time and carries a fingerprint of the text it was
fitted from. Editing `internal/model/corpus.go` or `internal/model/mined.go` changes every score and fails the build
until refit:

```sh
go test ./internal/model -run TestHeadAsset -update
```

A new class earns its place by surviving `TestStability`, which fits its direction from each half of its own examples and
compares them. Below cos 0.45 the class is learning the subjects of its examples rather than what they have in common.
The change-event class read as healthy from every other angle and died there at +0.32.

## Documentation

### README

It carries the adoption decision and nothing else: what it catches, what it leaves alone, install, configure, the I/O
contract, and the limits that change whether to run it. A reader is deciding, not studying.

The placement rule is the one this tool enforces on comments. A claim belongs where something re-verifies it, and nothing
re-verifies a README. So method, threshold arithmetic, sweep tables and post-mortems on classes that were dropped go to
`docs/`, where a reader goes on purpose and where the neighbouring test prints the same number.

```
recall 0.53 at precision 1.0, held out          docs/, beside the test that prints it
"a comment it passes over is not one it approves of"   README: it changes how you read a silent run
```

- **Every path and command a README cites is a claim that decays.** Check them before shipping, do not assume. The
  package split broke every one of them at once: six paths, four `go test -run` invocations that silently matched
  nothing, and a class table advertising a rule that had been replaced.
- **Say a thing once.** A TL;DR and a Why section that both explain the same decay are one section.
- **One line of real output beats a paragraph.** Put it above the fold; it is the cheapest answer to "what will this do
  to me".
- **A limits list past six bullets is an issue tracker.** Keep the ones that change the decision, move the rest.
- Length is earned by a constraint that forced the design or an approach tried and rejected. Neither is a README's job
  unless a user hits it.

### docs/

| File | Holds |
|------|-------|
| `design.md` | how the judgment works, and how the thresholds were fitted |
| `limits.md` | the long tail: shapes it gets wrong, judgments to disagree with, classes tried and dropped |
| `history-class.md` | why change-event comments are not caught: three hypotheses and the numbers that refuted each |
| `narrative-class.md` | three mechanisms aimed at implementation narrative, built and reverted, with costs |

The last two are the record of a negative result. They exist so the next attempt starts from what was already measured
rather than from the idea, and a class that gets retried has to beat the numbers in them.

## Spec tables

A false positive or a missed comment is a row added to a table before it is a change to a rule:

- `internal/rule/rule_test.go` — what fires per language
- `internal/rule/python_test.go`, `internal/rule/yaml_test.go` — the two languages whose prose parses, each with its own
  rule and its own table
- `internal/model/semantic_test.go` — what the model must and must not recognise
- `internal/model/aa_heldout_test.go` — labelled comments no part of the fit has seen

A case the tool does not answer today stays in the table marked `gap`, with the reason. Deleting it removes the only
record that the behaviour was ever wanted.
