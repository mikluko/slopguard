# How the judgment works

Each comment is cut into sentences, and each sentence is embedded with all-MiniLM-L6-v2 (see
`internal/model/assets/PROVENANCE`). Each class owns a direction — `unit(centroid(class) - centroid(contract))`, fitted
from the labelled corpus — and a sentence fires the class whose score along that direction clears its threshold by the
most.

Listing exemplars and taking the nearest was tried first and does not generalise: on comments from repositories that took
no part in tuning it nudged prose seven times, missed every change-event comment, caught 3 of 30 restatements, and every
false positive outscored every true positive. A set of exemplars is a set of points, and the thing being recognised is a
direction.

Sitting inside a function body lowers the threshold and moves nothing else. Prose is harder to justify there, but a line
pointing at a constraint enforced elsewhere still earns its place.

## Reading a comment as source

The commented-out-code rule asks more of a parser than a parser gives. A tree-sitter grammar is context-free and Go is
not, so `f == g` and `y0(x) = 1/sqrt(pi) * ...` both parse cleanly and neither is something the compiler would take.
Three shapes the grammar admits and the language refuses rule a fragment out: an expression statement that is not a call
or a receive, an assignment to something not assignable, and a leading label.

The fragment is parsed in statement position rather than at file scope, which is where code gets commented out from, and
the two parse by different rules — `fmt.Println("x")` is a conversion at file scope and a call inside a function. A run
cut out of a larger block leaves braces open, so they are closed before parsing; needing to is evidence rather than a
disqualification.

## Cost

The directions are fitted at build time into `internal/model/assets/head.bin`, three kilobytes, so an invocation embeds
only the sentences in front of it. Re-embedding the corpus on every hook call was most of what a run used to cost: a
write with nothing for the model to read now returns in single-digit to low-teens milliseconds, and one that reaches the
model pays enough more to notice. Opening the ONNX session is a fixed tenth of a second of that; the rest is embedding
each comment run, and only that part grows with the file. The second figure is machine-dependent and varies
several-fold with how many comments the file holds, so [metric.md](metric.md) gives it as an order rather than as a pair;
the pair that used to stand here disagreed with the one in `internal/rule`'s doc, which is what withdrew both. Most of
the floor is process start: the binary is 116 MB, since the model is embedded in it. That is the figure `stat` gives
divided by a million, which is how [metric.md](metric.md) states it too; `ls -lh` says 111, counting in mebibytes.

# Calibration

Thresholds are fitted, not chosen. Each class gets the lowest score at which it still fires at 0.85 precision on the
fitting corpus — over everything above the cut, and among the examples within 0.02 of it, since cumulative precision
alone lets a clean top of the ranking buy slack that is spent entirely at the margin.

Both numbers are measured rather than picked. `go test ./internal/model -v -run TestCalibrate` fits at required
precisions from 0.70 to 0.95, and `-run TestWindow` at margin widths from 0 to 0.08; each reports what that choice costs
on comments the fit never saw. Nothing below 0.85 nudges held-out prose that was right either, so what picks 0.85 is the
other side: at 0.95 the self-justification class stops firing at all and two more comments go unnudged for nothing. The
margin band from 0.01 to 0.02 nudges no held-out prose and still reaches the most true positives; at 0 it reaches five
wrong.

## The corpus

The corpus is in `internal/model/corpus.go` (hand-written) and `internal/model/mined.go` (harvested and labelled). Half
of the harvest fits; the other half is held out in `internal/model/aa_heldout_test.go` and nothing fits from it.
`go test ./internal/model -v -run TestHeldOut` reports precision and recall per class against that half, and fails if the
share of contract prose left alone drops.

One caveat on the held-out numbers, because they are what decides whether this tool is worth running: the held-out half
and the fitting half were split row by row within each source rather than by source, so comments from one file sit on
both sides and the classes see the topics they are tested on. It is an upper bound, and the repository sweeps are the
group-disjoint check beside it.

Editing the corpus changes every score, so `internal/model/assets/head.bin` carries a fingerprint of the text it was
fitted from and `go test` fails when the two disagree. Refit with `go test ./internal/model -run TestHeadAsset -update`.

## What the sweeps found

Measured on 98 held-out comments, each a single sentence: restatement at recall 0.53 and self-justification at 0.25, both
at precision 1.0. Those two are per-sentence rates — a comment of several sentences takes the strongest verdict among
them, so its recall is somewhat higher and is not measured here. Contract prose is left alone 75 of 75 above a
declaration, 75 of 75 at the lower threshold a comment inside a function body meets, and 25 of 25 when those rows are
read three to a comment, which is the unit a real doc comment arrives in. All three are asserted. The first row that
costs a piece of contract prose is twice the shipped tilt, where `TestClear` reports 1 of 75.

On a production Go service, 11 findings across 115 files of `internal`, `app` and `pkg`. On the Go standard library —
`find . -name '*.go' -not -name '*_test.go' -not -path '*/testdata/*' -not -path '*/vendor/*'` under `GOROOT/src`, 4065
files — **678** with every class on, which is what `SLOPGUARD_WIDER=1` asks for: 355 restatement by the model, 172 by the
identifier echo, **130** commented-out code, 21 self-justification, and no padding at all. **The shipped default is the
130.** This paragraph read 1034 / 872 / 142 for several rounds after the exemptions that moved them, while `AGENTS.md`
carried the current figure as an invariant. The largest class is
a step comment inside a long function, which is the shape this tool is pointed at and the shape that library uses most.
On 9934 YAML files and 5313 Terraform files of an infrastructure repository, 32 findings between them. On its own source,
none.

## The rule that counted sentences

A length rule used to fire on documentation running past eight sentences. It added 332 findings to the standard library
sweep and is gone. Length is not what the doctrine asks about: a clause earns its place when removing it changes what the
doc guarantees, and three sentences of padding stay under any threshold while nine that each carry a precondition cross
every one.

It was set against what documentation does rather than what a style guide says it should:
`SLOPGUARD_CORPUS=<repos> go test ./internal/comment -v -run TestLengthDistribution` counts sentences across a corpus,
and in 90,000 comments from four repositories written on purpose, half are one sentence, 95% are four or fewer and 99%
are eight or fewer. Against the Go standard library alone the same test gives 59%, 97% and 99%. Eight flagged that last
percent.

What replaced it asks of each sentence past the first whether it says anything the declaration does not, and on the
standard library none fails. The distribution test is kept because how long documentation actually runs is worth knowing
even once nothing thresholds on it.
