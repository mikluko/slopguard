# What says the rule got better

slopguard's existing number is per-class precision and recall on a held-out
split of hand-labelled comments, with the threshold set at the lowest score
where the class still fires at 0.85 precision on the fitting half. That is sound
over labels one person assigned, and it stays. What it cannot answer is whether
the taxonomy is right or whether anyone else would have labelled those rows the
same way, so a second evaluation is added beside it rather than replacing it.

## The second evaluation

The mined corpus does not carry class names. It carries two labels neither of
which is this project's opinion: `deleted`, a comment its own author removed in a
commit that kept the code under it, and `survived`, a comment left standing
through at least eight later commits to the same file. So the question it can
answer is not *is this comment a tautology* but the one the tool's bar is
actually about: **does slopguard fire on comments people threw away, and stay
quiet on comments people kept?**

The evaluation is therefore binary and class-agnostic. Any class firing counts
as firing.

## The headline is recall at a fixed false-positive rate, not precision

**Precision is not comparable here and false-positive rate is.** Precision
depends on the ratio of positives to negatives in the corpus, and that ratio is
an artifact of how the harvest happened to run: `survived` rows are plentiful and
`deleted` rows are rare, in whatever proportion the mining thresholds produced.
Change `endured` from eight to four and every precision figure moves without the
tool having changed at all. Recall and false-positive rate are each computed
within one label's rows, so neither moves when the mix does.

False-positive rate is also the quantity the user actually experiences.
[1092](http://127.0.0.1:9876/item/1092) fixes the bar as a false positive rate
that gets the hook disabled, and what that means concretely is the share of
good comments the tool nags about. That share is FPR on `survived`. Precision
answers a different question, "of the things it flagged, how many deserved it",
which nobody is asking while deciding whether to turn the hook off.

So:

- **Primary: recall on `deleted` at FPR 0.02 on `survived`**, with the same pair
  reported at FPR 0.01 and 0.05 so the shape near the operating point is visible.
- **Single number for ranking builds: partial AUC over FPR in [0, 0.05]**,
  normalised to that range. Full AUC is rejected below.
- **Per class: the same pair with only that class's rule enabled.** A class that
  adds no recall at fixed FPR is not paying for its share of the model load, and
  this is the number that says so.

## The noise runs one way, and that is what makes the number usable

Both labels are wrong some of the time, and the two errors are the same error
seen from either side. Some `deleted` rows are good comments removed for reasons
other than their quality. Some `survived` rows are bad comments nobody got round
to deleting.

Call `d` the share of `deleted` rows that are good and `s` the share of
`survived` rows that are bad. Then:

- a good comment mislabelled `deleted` is one slopguard should stay silent on,
  and silence there is scored as a miss, so **measured recall is a lower bound**;
- a bad comment mislabelled `survived` is one slopguard should fire on, and
  firing there is scored as a false positive, so **measured FPR is an upper
  bound**.

Both errors are pessimistic. A build that improves under this metric improved,
and the metric cannot flatter. That property is the reason to accept mined
labels at all, and it is worth more than the precision the noise costs.

**The numbers are reported, never corrected for.** A correction would have to
assume the mislabelling is independent of the score, and it is not: a good
comment that happened to get deleted is exactly the kind that reads as contract,
which is exactly the kind slopguard is built not to fire on. Correcting would
therefore subtract the errors the tool gets right. `d` and `s` are measured by
hand-sampling one hundred rows of each label, restated beside every figure, and
`recall / (1 - d)` may be quoted as an estimate of recall among rows that were
actually judged, marked as an estimate.

`d` is also the ceiling on everything. If a third of `deleted` rows are good
comments, no build reaches recall above about two thirds, and a run reporting
0.6 is near the ceiling rather than mediocre.

## What the corpus cannot see, and what to do about it

The paragraph above says the noise runs one way and is therefore safe. That
holds, but on one class the bound is loose enough to be useless, and the reason
is structural rather than a matter of sample size.

Measured on 11,387 harvested rows, `echo` catches 25 deleted comments and nudges
254 survived ones. Reading those 254 rather than counting them: `Copy
axios.prototype to instance`, `Iterate over object keys`, `Update the Host
header.`, `Set the sequence.`, `Recursively delete all child buckets.`
`tautology` looks the same: `Convert to a byte array.`, `Create a cursor for
iteration.`, `Increment and return the sequence.`

Every one of those is the class Steidl's sixteen developers agreed above 80% was
trivial, and that Jabrayilzade measures at a 31% base rate. They are correct
firings scored as false positives.

**The cause is that deletion and triviality are not the same judgement.** A
comment gets deleted when somebody is actively bothered by it. A trivial comment
bothers nobody: it is cheap to skim, and removing it is a diff nobody wants to
review. So triviality is systematically absent from the `deleted` side and
abundant on the `survived` side, and a build tuned to maximise recall on
`deleted` would be tuned away from the best-attested defect in the literature.

That is a trap rather than a reason to discard the corpus. What follows from it:

- **`survived` means *nobody removed this*, never *this is good*.** Every report
  says it in those words.
- **The corpus is strong evidence in one direction and weak in the other.** It
  measures false positives on prose people actively valued well, because a
  comment kept through many readings of a file somebody was editing is a real
  positive. It measures recall on triviality badly, because triviality does not
  generate deletions.
- **The classes split by which evidence governs them.** `leftover`, and any
  narration or change-event class, produce deletions and are properly judged
  against the mined labels. `echo` and `tautology` target a defect that does not
  produce deletions, and are properly judged against Steidl's validated
  coherence-coefficient criterion and a human pass, with the mined corpus used
  only for the false-positive half.
- **The hundred-row adjudication is therefore the linchpin, not a nicety.** What
  it has to produce is not only `d` and `s` but a split of the `survived` rows
  into prose that states a contract and prose that is merely trivial and was
  left alone. Only the first is a true negative. The second is not evidence in
  either direction and belongs in neither denominator.

Until that split exists, `echo` and `tautology` are reported with their FPR
figures marked as upper bounds that are known to be loose, and no threshold is
moved on the strength of them.

## What has to be beaten

A number with no baseline says nothing. Four, in increasing order of what they
demand:

1. **Fire on nothing.** Recall 0 at FPR 0. Any positive recall beats it, which
   is why it is the floor and not a real competitor.
2. **Fire on length alone**, the `padding` rule as the only signal. The README
   currently claims the length rule is seven of nine findings on a real Go
   corpus, so this is the baseline the semantic classes most need to beat. A
   model that does not beat sentence-counting is not worth ninety milliseconds
   of ONNX session build.
3. **The phrase list**, which already exists as the degraded path when ONNX
   Runtime is absent. It is known to have produced seven false positives out of
   seven on real code, so it should lose badly. If it does not, the model is not
   doing what the exemplars claim.
4. **The shipped build**, which is the comparison every later change is judged
   against.

## Rejected

**Full AUC-ROC.** It averages over operating points nobody would ship. A build
that is excellent at FPR 0.4 and poor at FPR 0.02 outscores one that is good
exactly where the hook runs. Restricting to FPR in [0, 0.05] keeps the
threshold-independence and drops the region that cannot be used.

**F1.** It is a precision-recall composite, so it inherits precision's dependence
on the corpus mix, and it weights a false positive and a false negative equally,
which contradicts the tool's own bar. Those failures are not symmetric here.

**Accuracy.** With `survived` rows outnumbering `deleted` rows by an order of
magnitude, firing on nothing scores above ninety per cent.

**Replacing the hand-labelled held-out set.** The mined corpus cannot say which
class fired, only that something did, so it cannot catch a build where
`tautology` starts firing on `compat` rows at unchanged aggregate numbers. The
two evaluations answer different questions and a build has to hold both:
**no regression on per-class held-out precision, and no regression on recall at
FPR 0.02.** That conjunction is the shipping rule.

**Correcting for label noise.** Argued above.

## Where it runs

Against the tables `internal/model` already has. `TestHeldOut` and
`TestCalibrate` read a labelled table and report against it; this needs one more
table, the mined rows with their `deleted` / `survived` label, and the same
sweep the `slopguard <files...>` path already performs to get a score per
comment. The scoring machinery, the fitted directions and the thresholds are all
unchanged. What is new is a second table and a second report.

One table the repository does not have and this metric needs: the hand-sampled
adjudication of one hundred rows per label that produces `d` and `s`. It is a
one-off human pass, it is small, and without it none of the figures above can be
interpreted.
