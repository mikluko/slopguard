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

The corpus contains a case that breaks it. Two mechanical date-fns migrations
contribute 158 deleted rows whose comments are long `@param`/`@returns` JSDoc
blocks and `@flow` pragmas, a shape almost absent from the survived pool. A build
that learned "long tagged doc block" would raise measured recall with no rise in
measured FPR. It would score better and be worse.

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
    everything on                21       25    0.091   0.007     7.6
    leftover alone               20        5    0.087   0.001    13.3
    leftover, three exemptions   20        1    0.088   0.000    15.3

One catch for twenty false positives. On the Go standard library the default
gives 142 findings over 4,065 files against 690 with everything on, and 1,034
before this cycle began. User time on one real file: 0.05s against 0.71s,
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
86 MB, most of a second per write, and three quarters of everything the tool
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

Each class is now also measured with the others switched off, and the two tables
agree, so the precedence order is not distorting the partition on this corpus.
That was an open worry for three rounds and it is closed by measurement rather
than by argument.

## The baselines, run at last

| rule | recall | FPR | lift |
|---|---|---|---|
| marker: TODO, FIXME, XXX, HACK | 0.126 | 0.010 | **6.85** |
| **the shipped build** | 0.077 | 0.007 | **6.40** |
| `trailing` | 0.186 | 0.026 | 4.90 |
| text opens with `@` | 0.007 | 0.003 | 2.28 |
| `doc` | 0.165 | 0.101 | 1.56 |
| `buried` | 0.607 | 0.434 | 1.36 |
| more than five lines | 0.084 | 0.076 | 1.10 |
| text of 120 bytes or more | 0.312 | 0.375 | 0.84 |

All on the same footing, with markers counted on both sides.

**Against the marker regex the comparison is not resolved, in either direction.**
An earlier revision here said the regex edged the tool, 6.85 against 6.40, on a
corpus this is no longer. On the ninth corpus the point estimate reverses — the
tool 13.3 against the regex 7.0 — but with repositories as the clustering unit
the paired gap is +6.11 lift with a 95% interval of [−0.17, +12.66], and the tool
is behind the regex on recall. So "the tool beats the baselines" is not a finding
at 95%, and neither is its negation. The answer to the marker rule is still not a
number: Google's C++ and Java guides mandate the form, so a tracked-debt marker
is not a misplaced explanation and the tool is deliberately not chasing it.

Two baselines that beat the whole pipeline on earlier corpora are now near the
floor: `text opens with @` fell from lift 7.2 to 2.28 when a per-commit cap
stopped one repository's JSDoc codemod contributing 119 positives, and `lines >
5` from 2.72 to 1.10. **A baseline that suddenly wins is how a corpus reports its
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
Excluding them is not conservative, though: all 89 excluded rows are positives,
none are negatives, and the tool's recall on them is 2/89, worse than its overall
rate. So `-markers=false` slightly *raises* the reported recall. Report both.

## The number this corpus cannot produce

Recall and lift are what a corpus labelled by deletion can measure. Precision at
the operating point is not, and precision is what a hook interrupting a write is
paid in. The corpus samples comment lines that somebody edited; the tool fires on
code nobody has touched, so the corpus's own FPR of 0.001 understates real-code
false positives by more than an order of magnitude.

So it was measured directly. 214 findings over 24 repositories and the Go
standard library, drawn as two strata — every multi-line finding, and an
every-third sample of the single-line ones — and judged in context by four
readers on disjoint packets, blind to each other.

    population                  judged   right   precision
    before this cycle (stdlib)      72      20       0.28
    multi-line                     115      35       0.30
    single-line                     99      43       0.43
    after three exemptions         160      78       0.49

The false positives are five shapes, all of them valid source in the language
they sit in: a compiler pass sketching the code it emits, spec or algebraic
notation, a comment naming what a `case` arm handles, a section heading, and an
annotation another tool consumes. The last of those is fixed; the other four are
not, and the first two are not obviously fixable, since a pseudocode convention
that uses the host language's own syntax is indistinguishable from the syntax.

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
can read 0 and `exposure == 0` remains an oracle: recall 0.654 at FPR 0.000, lift
16.7, above the tool on the same corpus. The earlier claim here — that what
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
annotated-code-length stratum 7.5 to 27.0, row-weighted 17.8 against a pooled
13.3. Conditioning on block size raises the tool's lift rather than removing it.

**What is open instead** is `annotates` length. The deleted side additionally
requires the annotated code to survive the commit verbatim, and the survived side
has no equivalent test, so long-annotated positives are selectively filtered out:
median 84 bytes against 158. A one-field rule, `annotates` under 60 bytes, scores
lift 2.24 [1.64, 2.91]. `baselines` tests two thresholds on that field which are
both dead — under 40 catches nothing, truncation at 400 scores 0.21 — and misses
the live one. Apply the survival test to both sides or to neither.

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

- **Markers.** `TODO|FIXME|XXX|HACK` is 14.9% of young deletions against 0.90% of
  survivors. A share of `deleted` is debt resolution, not a judgement.
- **Codemods.** 298 of 1,526 deleted rows come from ten commits. One date-fns
  commit contributes 134 rows across 134 files. The `burst` guard is per file and
  applies after the other filters, so a repo-wide sweep passes unbounded.
- **Exposure.** As above.
- **Repository skew.** date-fns is 16% of the positives and 0% of the negatives.
  86% of `leftover`'s false positives are Rust and YAML from four repositories at
  the cap.
- **Lifetime.** `lifetime_days` dates the comment by the last touch to its first
  line, so a reflow resets it. The identical pragma removed by one commit carries
  lifetimes from 40 to 2,059 days across sibling files.
- **Same-day deletions dropped.** `omitempty` on a float encodes a zero-day
  lifetime as absent, and the filter then discards those 39 rows. They are the
  tool's best subset at recall 0.231.
- **Shallow clones.** Five of twenty-four. In vuejs/core, 345 rows carry the
  graft boundary as their authoring commit.
- **Double counting.** The scorer iterates comments rather than rows, so a
  trailing comment sharing a start line scores its row twice.
- **Non-independence.** 462 young deletions come from 345 commits; 23% of deleted
  rows share exact text with another. Design effect 2.73, so recall 0.110 is
  [0.072, 0.167] clustered rather than [0.085, 0.142].
- **Population.** Every row is human-written prose in a mature reviewed project.
  The tool judges comments an agent just wrote. The corpus corrects for the
  labeller's taste and not for the population.

## What has to hold before a figure is quoted again

1. `survived` re-derived from line-local exposure.
2. Marker rows excluded from `deleted`, or every recall figure reported twice.
3. Codemods excluded: cap per commit, and drop commits touching many files.
4. Both labels harvested under matched eligibility and capped identically, or
   the survived pool reweighted to the deleted pool's repository mix.
5. Uniform clone depth, with graft-terminated blame recorded as a field.
6. Intervals on everything, clustered by removing commit.
7. Per-class figures produced with the other classes disabled, not read off a
   precedence-ordered partition.
8. The four original baselines plus the marker baseline actually run.

## The operating point this document chose does not exist

This part stands and is the most useful thing the exercise produced.

Sweeping the semantic thresholds moves them only. `echo`, `leftover` and the YAML
carve-out carry no threshold, so they fire identically at every offset and their
combined false-positive rate is a floor: 0.044 on this corpus, of which
`leftover` is 0.024 and `echo` 0.020. **Turning the model off entirely still
lands above FPR 0.02.**

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
