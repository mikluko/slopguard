# What says the rule got better

slopguard's existing number is per-class precision and recall on a held-out split
of hand-labelled comments, with the threshold set at the lowest score where the
class still fires at 0.85 precision on the fitting half. What it cannot answer is
whether the taxonomy is right, or whether anyone else would have labelled those
rows the same way, so a second evaluation is added beside it.

**This document has been through one round of independent review and most of its
first version did not survive.** The figures it used to report are withdrawn, the
argument that justified accepting mined labels was circular, and one of the two
labels does not measure what it was defined to measure. What follows is the
corrected design plus a standing account of what is still wrong, because a metric
document that hides its own defects is worth less than no metric at all.

## The second evaluation

The mined corpus carries two labels, neither of which is this project's opinion.
`deleted`: a comment its author removed in a commit that kept the code under it.
`survived`: a comment still present after at least eight later commits to the
same file.

The question it can answer is not *is this comment a tautology* but *does
slopguard fire on comments people threw away and stay quiet on comments people
kept?*

## Withdrawn

The following were reported and are withdrawn. They are listed rather than
deleted so that anyone holding the old numbers knows they were retracted.

**Partial AUC 0.011.** An off-by-one. The trapezoid loop guarded accumulation
with `i > 0` while `previous` was already initialised to the origin, so the
trapezoid from (0, 0) to the lowest-FPR point was discarded. That region is 88%
of the width. Corrected the figure is about 0.056 against 0.025 for a random
classifier. As published it asserted worse-than-chance performance, which the
direct test refutes at z = 6.2.

**"Closing to ninety days raises recall with the false-positive rate unmoved."**
Circular: the window filters deleted rows only, so the false-positive rate is
structurally incapable of moving. The honest comparison is recall 0.110 within
ninety days against 0.058 beyond it, z = 3.60.

**"The semantic classes buy 0.005 of recall for 0.003 of FPR."** That
differenced two threshold tilts rather than turning the model off. Measured
properly with `SLOPGUARD_NO_MODEL=1`, the semantic pass buys 6 catches for 38
nudges: +0.013 recall for +0.003 FPR.

**"`leftover` has more than twice `echo`'s recall."** The ratio interval is
[1.31, 4.64], and pooling `echo` with `tautology` as one defect, which
`docs/limits.md` already does, erases the difference at p = 0.092.

**The terminator fix measured on this corpus.** One row of 12,436, McNemar
p = 1.0. The corpus has no power to detect it in either direction. The
minimal-pair measurement in the commit message is the real evidence.

**Every recall figure, pending the repairs below.** They are not wrong
arithmetic, they are measurements of a corpus whose positive set is 20%
non-defects by this project's own definitions and whose negative set does not
mean what it was defined to mean.

## The headline is recall at a fixed false-positive rate, not precision

This part stands. Precision depends on the ratio of positives to negatives, and
that ratio is an artifact of the mining thresholds: change `endured` from eight
to four and every precision figure moves without the tool changing. Recall and
false-positive rate are each computed inside one label's rows.

False-positive rate is also closer to what the user experiences. The bar is a
false-positive rate that gets the hook disabled, which is the share of good
comments it nags about.

**One caveat that the first version got wrong.** Recall and FPR are invariant to
the ratio *between* labels. They are not invariant to the composition *within* a
label, and the survived pool's composition is set by `-keep`. Removing the four
repositories sitting at the 800-row cap moves FPR from 0.047 to 0.035 without
touching the tool. So any FPR figure must be reported against a stated pool, and
preferably stratified by repository and language.

## The noise does not simply run one way

The first version argued that mislabels are pessimistic in both directions, so a
build that improves under the metric improved and the metric cannot flatter.
**That argument is invalid and is withdrawn.**

Write `d` for the share of `deleted` rows that are good, `f_g` for the rate at
which the tool fires on those, and `r` for its true recall on real defects.
Measured recall is `d·f_g + (1−d)·r`. Measured recall is below true recall **if
and only if `f_g ≤ r`** — that is a property of the tool under test, not of the
labels, and nothing here has established it.

The second corpus contained a case that broke it. Two mechanical date-fns
migrations contributed 158 deleted rows whose comments were long
`@param`/`@returns` JSDoc blocks and `@flow` pragmas, a shape almost absent from
the survived pool, and a build that learned "long tagged doc block" would have
raised measured recall with no rise in measured FPR: better score, worse tool.

That example is history, and this passage asserted it in the present tense for
several corpora after the per-commit cap destroyed it. On the ninth corpus
date-fns contributes 8 deleted rows in total, none of them a JSDoc block, and
only 2 deleted rows anywhere open with `@`. The argument stands without the
example — a filter cannot be assumed to bias against the tool merely because it
removes positives — but the example is no longer evidence for it.

What replaces the claim: **the direction of the bias is unknown and must be
argued per change.** A change that could plausibly be picking up codemod shape,
marker shape, or language shape has to be checked against those strata directly
before its headline movement is believed.

## What `survived` means, after two wrong definitions

It was first defined as a comment kept through many readings of a file somebody
was editing, and counted commits to the **file**. Checked with `git log -L`, a
row recorded at 72 file edits had one commit that ever touched its line. That
version meant only "still present".

The second version counted commits touching the comment's **own line**, over all
history. `git log -L` lists newest-first and the newest commit is the one that
wrote whatever stands there now, so it was counting churn from before the comment
existed: 98.4% of survivors had nothing touch their line after their own text was
written, and half were kept on the strength of a single pre-authorship edit.

The current version counts commits **after the comment was written** that touched
the **code it annotates**. That is the question the label always claimed to ask:
how often did somebody edit the thing being described and leave the description
standing. `seen = 1`, because the window no longer includes the comment's own
authorship.

Both wrong versions passed review once each. The instrument is the part of this
work that has needed the most correction, and it is worth saying that the two
defects were found by review rather than by the tests, which were green
throughout.

## Baselines, which must be run and mostly were not

A number with no baseline says nothing, and the first version named four
baselines and ran none of them. Measured since:

## What ships, and why it is one rule

`leftover` alone. `echo`, `hollow`, `tautology` and `compat` are behind
`SLOPGUARD_WIDER=1` and off by default.

    build                    caught   nudged   recall   FPR      lift
    everything on                21       20    0.093   0.006    8.25
    leftover, four exemptions    20        1    0.088   0.000    15.3

Both rows are the current build on the ninth corpus, which an earlier revision of
this table did not manage: it carried `everything on` at 21 and 25 from before
the exemptions and footed on 231 positives, beside a `leftover` row footed on
227, and called the difference a comparison.

One catch for nineteen false alarms. On the Go standard library the default gives
130 findings over 4,065 files against 678 with everything on, and 1,034 before
this cycle began. User time on one real file: 0.05s against 0.71s,
because the semantic pass loads 90 MB of ONNX before it can say anything.

**It does not shrink the binary, and an earlier version of this document implied
it did.** The model is embedded at compile time and `internal/rule` imports
`internal/model` for its reason strings, so the artifact is 116 MB either way.
What the default buys is the load, which is most of a second on every write, and
twenty of the twenty-five false positives. Dropping the 90 MB as well needs build
tags and a package split, which is a separate change.

**This is a default rather than a deletion, and the reason matters.** The corpus
labels by deletion, a trivial comment bothers nobody, so nobody deletes it:
fifteen of the twenty comments those classes fire on here are trivial by
Steidl's criterion. The corpus cannot see the defect they target, and a reviewer
tested whether it was actively biased against them by rebuilding the harvester
with the exposure gate off. `echo`'s false-positive rate moved 0.07 points. It
is blind, not hostile.

What is measurable is the other side. On real code `echo` is right about two
thirds of the time and its false positives are section headings scored against a
run's first statement. `tautology` is right seven times in twenty-four and fails
systematically above `return`, `break` and `continue`, where there is nothing to
restate, which is the objection every source in `docs/comment-practice.md`
makes. A faithful port of Steidl to the population his metric was validated on,
documentation against the signature, scores below chance.

So the value is unmeasurable and will stay so on this instrument, and the cost is
90 MB, most of a second per write, and four fifths of everything the tool
says. That asymmetry is the whole argument, and it was answerable from the first
round.

## The full pipeline, for comparison

Measured on the seventh corpus, markers excluded, 249 deleted and 3,618
survived:

| | catches | nudges | recall | FPR |
|---|---|---|---|---|
| `leftover` | 20 | 5 | 0.080 | 0.001 |
| `tautology` | 0 | 14 | 0 | 0.004 |
| `echo` | 0 | 6 | 0 | 0.002 |
| `compat` | 1 | 0 | 0.004 | 0 |
| `hollow` | 0 | 0 | 0 | 0 |
| **the shipped build** | **21** | **25** | **0.084** | **0.007** |

Lift over a rule firing at random at the same rate is **7.1**, hypergeometric
z about 10.9. `leftover` alone is **12.4** at z about 15, and supplies 20 of the
21 catches.

Each class is now also measured with the others switched off, and this section
used to claim the two tables agree, so the precedence order was not distorting
the partition. They do not agree, on either corpus. `echo` catches nothing in the
run and one alone; `tautology` takes 13 nudges in the run and 14 alone; the
isolated rows sum to 22 catches against the run's 21. That is exactly the
distortion the isolated table exists to expose, and it was declared absent while
the numbers to refute it were printed beside the claim.

The distortion is small and it runs the way precedence predicts — a class below
`leftover` sees only what `leftover` declined — so nothing else in this document
turns on it. Read the isolated table for what a class costs and the run table for
what it adds.

## The baselines, run at last

On the ninth corpus, over the 3,655 rows the tool could score, with all four
exemptions in:

| rule | recall | FPR | lift |
|---|---|---|---|
| harvest field: `exposure` is zero | 0.656 | 0.000 | **16.10** |
| **the shipped build** | 0.088 | 0.000 | **15.33** |
| marker: TODO, FIXME, XXX, HACK | 0.110 | 0.010 | 6.94 |
| `trailing` | 0.141 | 0.026 | 4.26 |
| text opens with `@` | 0.009 | 0.003 | 2.93 |
| harvest field: `annotates` under 60 bytes | 0.295 | 0.126 | 2.16 |
| harvest field: `annotates` under 80 bytes | 0.480 | 0.225 | 2.00 |
| `doc` | 0.185 | 0.100 | 1.76 |
| harvest field: `annotates` under 100 bytes | 0.577 | 0.329 | 1.68 |
| harvest field: `annotates` under 160 bytes | 0.753 | 0.503 | 1.45 |
| `buried` | 0.617 | 0.428 | 1.40 |
| more than five lines | 0.101 | 0.076 | 1.30 |
| text of 120 bytes or more | 0.366 | 0.374 | 0.98 |
| harvest field: `annotates` truncated at 400 | 0.048 | 0.236 | 0.22 |

All on the same footing, with markers counted on both sides, and every row now
on the rows the tool could actually score. Footed on the loaded set instead, the
tool's own row was computed over 231 positives it had seen 227 of, and one run
printed its recall as 0.088 above this table and 0.087 inside it.

**Read lift as precision, because that is what it is.** With `rate` the share of
all rows a rule fires on,

    rate = (caught + nudged) / (P + N)
    lift = recall / rate = ((P + N) / P) × caught/(caught + nudged)

so it is corpus precision multiplied by `(P+N)/P`, a constant of the mining
thresholds. On this corpus that constant is 16.10.

An earlier revision of this passage wrote the constant as `N/P` and was refuted
by the table twenty lines above it: `N/P` is 15.10, and both `exposure == 0` at
16.10 and the tool at 15.33 sit above it, which no ceiling permits. The
arithmetic below is the same either way; the number is not.

Two consequences, neither of them small. The first is that this document rejected
precision for depending on `endured` and then quoted, in every comparison it
made, precision multiplied by exactly that dependence. The second is that lift
saturates: as false positives go to zero it pins at `(P+N)/P` regardless of how
many positives the rule caught. `exposure == 0` catches 149 at zero cost and
therefore scores exactly 16.10, the ceiling; the tool catches 20 at nearly zero
cost and scores 15.33. That gap is not the tool nearly matching an oracle, it is
both rules pressed against the same wall. Prefer the counts.

**One row above the tool is a field the harvester wrote.** An earlier revision of
this section said two; `annotates` under 60 bytes is sixth. `exposure` is an
artifact of a filter and is discussed below. `annotates` length still separates
the labels, at lift 2.16, because the positive side additionally requires the
annotated code to survive its commit verbatim and long code is likelier to
change, so long-annotated positives are filtered out: median 84 bytes against the
negative class's 158. Standardised to the deleted class's own `annotates`
distribution the tool scores 15.50 against its pooled figure, so the confound is
real and worth about 1%.

**Against the marker regex the comparison is not resolved, in either direction.**
An earlier revision here said the regex edged the tool, 6.85 against 6.40, on a
corpus this is no longer. On the ninth corpus the point estimate reverses: the
tool 15.33 against the regex 6.94, or in counts, 20 catches and 1 false alarm
against 25 and 33.

The interval that was quoted here — a paired gap of +6.11 with a 95% range of
[−0.17, +12.66] — was bootstrapped on the pre-exemption build at lift 13.3
against 7.0, and is left out rather than restated, because nobody has run it on
what ships. What carries over is its shape and its conclusion: clustered by
repository the gap was not distinguishable from zero at 95%, the tool is behind
the regex on recall, and 8 repositories is not enough clusters to settle it
either way. Read the counts.

The answer to the marker rule is still not a number: Google's C++ and Java guides
mandate the form, so a tracked-debt marker is not a misplaced explanation and the
tool is deliberately not chasing it.

Two baselines that beat the whole pipeline on earlier corpora are now near the
floor: `text opens with @` fell from lift 7.2 to 2.93 when a per-commit cap
stopped one repository's JSDoc codemod contributing 119 positives, and `lines >
5` from 2.72 to 1.30. **A baseline that suddenly wins is how a corpus reports its
own contamination**, which is why they are printed on every run.

**Five conclusions have been recorded on five corpora and four are withdrawn.**
That the tool beat chance at z = 6.2 was the first, whose positive class was 70%
admitted by a test that never ran. That it did not beat chance at all, and that
`echo` was a significant negative predictor, was the second, whose negatives were
selected by a gate that reweighted the classes threefold. That `exposure` finally
overlapped the classes was the third, where it was still a zero-error oracle over
44% of the positives. That capping single-label repositories would help was the
fourth, where it concentrated the corpus it was meant to spread. **The only claim
that has held every round is that `echo` and `tautology` catch nothing.**

The one-bit baselines must be re-run here before any is quoted. On earlier
corpora `lines > 5`, `trailing`, a language prior and a `@`-prefix rule all beat
the tool, and the last of those was one repository's codemod, now capped out.

**The marker answer, restated.** Markers are a tracked-debt convention Google's
C++ and Java guides mandate, so the tool is deliberately not chasing them.
Excluding them is not conservative, though: on the ninth corpus 26 deleted rows
carry a marker, the tool catches none of them, and dropping them raises reported
recall from 0.088 to 0.099 on the same 20 catches. So `-markers=false` *flatters*
the tool rather than penalising it. Report both. (An earlier revision put the
count at 89 with recall 2/89, from a corpus four generations back, while its own
defect list two hundred lines below already said 26.)

## The number this corpus cannot produce

Recall and lift are what a corpus labelled by deletion can measure. Precision at
the operating point is not, and precision is what a hook interrupting a write is
paid in.

Two things this section said and had wrong. Precision *is* computable here — it
is `lift × P/(P+N)`, about 0.95 — it is simply meaningless, because it is precision
on a population the hook never sees. And the corpus FPR was said to understate
real-code false positives "by more than an order of magnitude", which cannot be
true: measured over the Go standard library's 146,739 comment runs, 130 findings
put the false-alarm rate below 0.00089 even if every one were wrong, against a
corpus FPR of 0.0003. The gap is a factor of about three, not ten, and the reason
to distrust the corpus figure is not its size.

The reason is the denominator. Roughly four fifths of the positive class is not
commented-out code at all — hand-reading a systematic sample puts commented-out
code at about a fifth of it, tracked-debt markers at an eighth, and prose removed
for reasons that are nobody's defect at about half. `leftover` is built not to
fire on any of that, so recall 0.088 is close to `P(commented-out code | deleted)`
times the rate at which the rule catches one. **Conditional on the subset the
shipped rule addresses, recall is nearer 0.4**, and that is the number a reader
should carry. The headline understates the tool by about five times, for the same
reason the corpus FPR understates its false alarms: both are ratios over a
population chosen by the miner rather than by the deployment.

So it was measured directly. 214 findings over 24 repositories and the Go
standard library, drawn as two strata — every multi-line finding, and an
every-third sample of the single-line ones — and judged in context by four
readers on disjoint packets, blind to each other.

    population                  judged   right   precision
    multi-line                     115      35       0.30
    single-line                     99      43       0.43
    both strata, as judged         214      78       0.36
    the same rows, exemptions in   160      78       0.49

**That last row is not an estimate of anything, and an earlier revision of this
document presented it as one.** The exemptions were chosen by reading these same
214 judgements, and every one of the 54 findings they removed had been judged
wrong — `right` does not move, only `judged`. Fitting a filter to an outcome and
then reporting the filtered rate is a training-set figure. No held-out packet was
drawn, so the honest statement is that on the judged sample the exemptions
removed 54 false positives and no true ones, and that precision on findings
nobody has judged is unmeasured.

Two further defects in how it was pooled. The strata were sampled at different
rates — every multi-line finding, one single-line finding in three — so the naive
pooled figure is wrong by design; it happens to be right here (0.4875 naive
against 0.4884 weighted) only because the exemptions left the two strata almost
equal, and it was off by 3.4 points before them. And the four readers judged
disjoint packets, so nothing was double-judged and there is no agreement rate.

An earlier revision also read `0.28 → 0.49` off the top of this table. The 0.28
was a separate, earlier, standard-library-only pass, and the standard library's
output was byte-identical before and after those three exemptions. Same
population before and after is `0.398 → 0.488`.

The false positives are five shapes, all of them valid source in the language
they sit in: a compiler pass sketching the code it emits, spec or algebraic
notation, a comment naming what a `case` arm handles, a section heading, and an
annotation another tool consumes. Two are now fixed. The annotation families went
with the directive list, and the `case` arm went with a structural test — the
comment opens the arm or sits directly above the label — which removed 12 of 142
findings on the Go standard library and 21 of 189 across the clones, at no cost
in recall. Twenty of those twenty-one were a single Vue file; the twenty-first
was jq's `/*create_pt_key();*/`, which is residue, and the exemption is wrong
about it. It is wrong about three more in the standard library for the same
reason: a comment at the *tail* of an arm has a case label as its next node just
as a label comment does, and the two are the same shape in the tree. Structure
cannot separate them, so this exemption is about 88% right and knowingly so.

The three that stand are the hard ones, and the first two may not be fixable at
all: a pseudocode convention written in the host language's own syntax is not
distinguishable from the syntax. What separates the compiler sketches is that
they fail to *type* check, and a type checker is two orders of magnitude outside
a hook's budget and exists for one of the fourteen languages.

**Precision is bimodal, and the pooled number is nobody's experience.**
Application code, library code and tests run high; compilers, code generators,
crypto and spec implementations run low, because the five shapes above are
concentrated in exactly the second group. The Go standard library is the low end
and is measured: about 35 right of 142 before the case-arm exemption. The high
end is read from clone findings and a pydantic sample, and lands somewhere
between six and nine in ten.

Quote it as that range and no tighter. A previous revision here cited "go-chi 26
findings and about 26 right" as one of two whole-population reads supporting
"near nine in ten". `tmp/clones/go-chi_chi` is an empty clone with no working
tree — its 110 corpus rows are exactly the scorer's "110 missing blob" — so it
produced no findings in any sweep and that read cannot be reproduced. It also
means the corpus is 23 repositories with content rather than 24.

**This is also how a recommendation was refused.** A reviewer, hand-judging the
19 multi-line findings in the standard library at 11 right of 19, proposed
restricting the rule to runs of two or more comment lines. On the whole
population it inverts: multi-line is the *worse* stratum, 0.30 against 0.43,
because it is mostly Helm `values.yaml` and the standard library has none. The
rule would have cost 8 of 20 catches to keep the worse half. With `values.yaml`
exempt the two strata are 0.486 and 0.489, so line count was never the axis. A
sample drawn from one population does not price a rule applied to another.

## Exchangeability, which took three corpora to get close to

`exposure` used to be 0 on every deleted row and at least 2 on every survived
one, because it was computed for survivors alone: a one-line rule on the field
scored recall 1.000 at FPR 0.000. It is now computed for both labels over the
same window, and the classes overlap.

**They overlap because a filter makes them, and this document said otherwise for
two revisions.** `expose` drops every survived row reading 0, so no survived row
can read 0 and `exposure == 0` remains an oracle: recall 0.656 at FPR 0.000, lift
16.10, above the tool on the same corpus and exactly at the ceiling the lift
statistic permits. The earlier claim here — that what
separation remains is inherent, "the label, not a filter" — was wrong. Rebuilt
with the gate off, 32.2% of survived candidates in one repository read 0 and are
discarded.

The window is the other half. Truncating each survived row's history to a length
drawn from the deleted class's own lifetime distribution puts 48.7% of survived
rows at 0, against the deleted class's 65.4%, and takes the oracle's lift from
16.7 to about 1.3. So most of the distributional gap is window asymmetry and none
of the zero is: the zero is arithmetic.

Neither is fixed here. The field ships on every row and defines the negative
label, which is a thing a corpus may not do, and `-matched` conditions only the
positive side. Until it is fixed, no reading of this corpus may use `exposure`,
`CodeFrom` or `CodeTo`, and the tool's lift is not to be compared against a rule
that reads them.

Both labels are now capped per repository, the positive class at a quarter of the
negative one. Before that, one repository was 28% of the positives and the tool
caught 1 of its 250, which moved the measured lift by a quarter on its own.

**Block size is closed, and it was not the explanation.** `leftover` is a
commented-out-code detector and commented-out code is long, so any rule
correlated with block size gets lift for free; on the second corpus 113 of the
116 deleted rows longer than 20 lines were one repository. Measured here:
within every comment-length stratum the lift is 11.7 to 19.3, and within every
annotated-code-length stratum 7.5 to 27.0, row-weighted 17.8 against that build's
pooled 13.3. Conditioning on block size raises the tool's lift rather than
removing it. The figures are the pre-exemption build's, which is what was
measured; the shipped build's pooled lift is 15.33 and the stratification has not
been re-run on it.

**What is open instead** is `annotates` length. The deleted side additionally
requires the annotated code to survive the commit verbatim, and the survived side
has no equivalent test, so long-annotated positives are selectively filtered out:
median 84 bytes against 158. A one-field rule, `annotates` under 60 bytes, scores
lift 2.16. `baselines` used to test two thresholds on that field which were both
dead — under 40 catches nothing, truncation at 400 scores 0.22 — and to miss the
live one; it now sweeps 60, 80, 100 and 160, which is the table above. What is
still open is the cause: apply the survival test to both sides or to neither.

## Comparing two corpora is not a measurement

The fall from recall 0.110 to 0.044 was reported as though the tool had been
re-measured. It had not: the corpus, the build, the deletion window and the
marker treatment all changed at once. Like for like, the same build gives 0.078
on the old corpus and 0.042 on the new.

The only legitimate test is paired, on rows both corpora contain. 281 of the new
deleted rows are in the old one. On those: old build 26 catches, new build 24,
**McNemar exact p = 0.50.** The tool did not change. The drop is entirely a
change in which rows are in the corpus, which is what a repair to the mining
rules should produce.

## What is known to be wrong with the corpus

Stated here rather than in a ticket, because anyone reading a figure needs them.

**Every figure below is on the ninth corpus, 231 deleted and 3,618 survived.** An
earlier revision of this section quoted the second corpus — 1,526 deleted rows —
while presenting itself as current, and two reviews in a row flagged it before it
was rewritten. Four of its bullets described defects the harvester had since
fixed, which is worse than a stale number: a reader is warned off a figure that
is sound and not warned about the one that is not.

- **Markers.** `TODO|FIXME|XXX|HACK` is 26 of 231 deleted rows against 36 of
  3,618 survivors. A share of `deleted` is debt resolution, not a judgement.
- **Exposure.** The largest live defect. See the section above: the gate deletes
  every survived row at zero, so `exposure == 0` remains an oracle, and matching
  the window on both sides takes recall from 0.088 to 0.064.
- **`annotates` length.** The other live one, at lift 2.16. The positive side
  carries a survival test the negative side does not.
- **Repository skew.** The 20 catches come from 8 of 24 repositories, and jq and
  pydantic supply 10 of them. Four of the 20 are one jq commit's two copies of
  the same manual. No repository is at the 800-row cap; the largest contributes
  388.
- **A repository with no content.** `go-chi/chi` cloned empty — its 110 rows are
  exactly the scorer's "110 missing blob" — so the corpus is 23 repositories, and
  any figure quoted as a whole-population read of go-chi is unreproducible.
- **Lifetime.** `lifetime_days` dates the comment by the last touch to its first
  line, so a reflow resets it.
- **Shallow clones.** Five of twenty-four. In vuejs/core the graft boundary is
  the authoring commit for part of the 295 rows it contributes.
- **Non-independence.** Design effect about 2.9, so recall 0.088 is roughly
  [0.022, 0.152] clustered rather than [0.058, 0.132].
- **Resolution.** 20 catches across 8 repositories detects a change of about 30%
  relative and nothing finer, and the false-positive side is one event, so no FPR
  comparison between two builds is possible at all.
- **Population.** Every row is human-written prose in a mature reviewed project.
  The tool judges comments an agent just wrote. The corpus corrects for the
  labeller's taste and not for the population, and no amount of mining reaches
  the second one.

Fixed since, and listed because an earlier revision still warned about them:
codemods (`sweep` caps files per commit and `burst` caps comments per file, and
both are now pinned two-sided), same-day deletions (all 231 rows carry a
lifetime), and the scorer's double counting (one verdict per row).

## What has to hold before a figure is quoted again

Done, and struck rather than deleted so the list is not re-derived from scratch:
`survived` is re-derived from line-local exposure; per-class figures are produced
with the other classes disabled rather than read off a precedence-ordered
partition; the baselines are run, including the marker rule; codemods are capped
per commit and per file, both pinned two-sided.

Still open:

1. Marker rows excluded from `deleted`, or every recall figure reported twice.
2. Both labels harvested under matched eligibility and capped identically, or
   the survived pool reweighted to the deleted pool's repository mix.
3. Uniform clone depth, with graft-terminated blame recorded as a field.
4. Intervals on everything, clustered by repository — the clustering unit is the
   repository rather than the removing commit, since 8 repositories is what the
   catches actually span.
5. The `annotates` survival test applied to both labels or to neither.

## The operating point this document chose does not exist

The design conclusion stands. The numbers it was stated with do not, and the
heading said "this part stands" over both for several corpora.

Sweeping the semantic thresholds moves them only. `echo`, `leftover` and the YAML
carve-out carry no threshold, so they fire identically at every offset and their
combined false-positive rate is a floor. That floor was quoted here as 0.044,
`leftover` 0.024 and `echo` 0.020, with the claim that turning the model off
still lands above 0.02. On the ninth corpus the shipped default's floor is
**0.0003** and the wider build's is **0.0029**, taken as the minimum over the
sweep rather than read off the shipped offset, where it is 0.0058. Either way the
old figure was an order of magnitude high and its bolded claim is refuted by the
corpus this document otherwise reports.

A rule with no threshold cannot be traded off, so the floor moves only by making
one thresholdable or by cutting it. That is a design conclusion no aggregate
number would have shown. The caveat above applies: the floor's *level* is a
composition figure, but its *existence* is a property of the rules.

## The two evaluations answer different questions

The mined corpus records only that something fired, never which class, so it
cannot catch `tautology` starting to fire on `compat` rows at unchanged totals.
The hand-labelled held-out set can, and stays.

That set has its own defect, found in the same review: four calibration tests
read it, and the doc comments on `precision`, `marginWindow`, `clear`, `perDraw`
and `BuriedBias` each say the constant was chosen from those printouts. It is a
selection set, not a test set, and its numbers are validation scores after at
least six decisions. A third split that nothing reads until a build ships is
owed, and it should be carved by project rather than by row.

## Where it runs

`tools/score`, against `internal/corpus/testdata/harvest.jsonl.gz`. `-maxlife`
restricts the deletion window, `-dump <class>` prints the rows a class fired on,
which is how a false-positive rate is read rather than trusted.
