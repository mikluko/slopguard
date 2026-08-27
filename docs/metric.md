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

## What `survived` actually means

`survived` was defined as a comment "kept through many readings of a file
somebody was editing". **That is false for the typical row and the claim is
withdrawn.**

The label counts commits touching the *file*. Checked at the comment's own lines
with `git log -L`, a row recorded at 72 file-edits has one commit that ever
touched its line; rows at 267 have two and three. Survived comments start at
median line 359 and p90 line 1,843, so most of them sit far from where the file's
churn happens.

`survived` therefore means **still present**, and nothing stronger. Until
exposure is re-derived at the hunk level — later commits whose diff touches the
comment's line range — no FPR figure here can be described as a rate on prose
people actively valued.

## Baselines, which must be run and mostly were not

A number with no baseline says nothing, and the first version named four
baselines and ran none of them. Measured since:

| rule | recall | FPR | note |
|---|---|---|---|
| `\b(TODO\|FIXME\|XXX\|HACK)\b` | 0.149 | 0.009 | beats the tool on both axes |
| `trailing` boolean from the parser | 0.130 | 0.053 | sits on the shipped ROC curve |
| fire on every JavaScript comment | 0.106 | 0.036 | same recall, 22% fewer nudges |
| the shipped build | 0.110 | 0.047 | |
| the shipped build, model off | 0.100 | 0.046 | |
| fire on length alone | ≈ chance | | AUC ≈ 0.5 |
| fire at random on 4.9% of rows | 0.049 | 0.049 | |

The tool beats chance decisively: z = 6.2 unstratified, z = 7.3 stratified by
language, z = 3.8 after a worst-case clustering correction, lift 2.25 to 2.54.
**It does not beat three one-bit baselines at its shipped operating point.**

Two of those three are corpus artifacts and dissolve with the repairs below: the
JavaScript baseline is half date-fns, which contributes 246 deleted rows and zero
survived, and `trailing` rides a position skew created by harvesting the two
labels under different eligibility rules. **The marker baseline is not an
artifact.** It is real, and the answer to it is that markers are a tracked-debt
convention that Google's C++ and Java guides mandate, so the tool is deliberately
not trying to catch them. That answer is only honest if marker rows are excluded
from `deleted` rather than counted as recall the tool failed to earn.

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
