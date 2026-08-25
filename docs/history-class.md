# Why the history class never fires

`history` is the only one of the four classes that fires zero times on the
held-out set. Its threshold is finite (+0.340 at the required precision of 0.85)
but sits above every real change-event comment: the highest-scoring held-out
history row reaches +0.221.

The cause is not the threshold and not the fitting method. It is that the
contract/change-event distinction moves a sentence vector about a quarter as far
as its subject matter does, so the distinction does not survive being averaged
into a centroid.

All numbers below are measured on the 114 held-out comments, fitted from the 271
in `corpus.go` plus `mined.go`, unless stated otherwise.

## Baseline

| class | labelled | fired | precision | recall |
| --- | --- | --- | --- | --- |
| contract | 75 | 75 left alone | | 1.000 |
| history | 10 | 0 | - | 0.000 |
| compat | 8 | 4 | 1.000 | 0.500 |
| tautology | 15 | 10 | 1.000 | 0.667 |
| narrative | 6 | 1 | 1.000 | 0.167 |

## The three hypotheses

### 1. History-shaped contracts collapse the direction — refuted

Dropping all eleven seeded entries (`the stop method is no longer necessary to
help the garbage collector` and its neighbours) moves the history cut from
+0.340 to +0.338. History still fires zero times, at every required precision
from 0.70 to 0.90. The change is not free: compat precision falls from 1.000 to
0.800 at p=0.70, and tautology loses a true positive at p=0.85.

They are not what holds the cut up. Ranking the whole fitting corpus along the
history direction, precision reaches 0.900 at rank 9 and never returns to 0.85
below it, so rank 9's score becomes the cut. What breaks precision above rank 9
is ordinary contract prose, and what keeps it broken below is the compat class,
whose exemplars occupy ranks 16 through 57 densely.

```
rank  score    label      text
   2  +0.372   contract   do not change anything here, a refresh may be in progress
   9  +0.340   history    previously created by the deployment unit that is now dormant   <- the cut
  10  +0.324   contract   changing this would orphan existing installations
  12  +0.309   contract   the version was capped for compatibility, ...                   <- a seeded one
  16  +0.278   compat     still referenced by the deprecated entry point
```

Only one seeded entry appears in the top fifteen. Removing them changes nothing
because the two entries above the cut are not seeds.

### 2. Two modes in one class — refuted

Splitting history into change narration and provisional statement (`for now`,
`not automated yet`, `temporarily disabled`) gives 7 exemplars and 4 held-out
rows to the second mode.

| fit | history | provisional | contract left alone |
| --- | --- | --- | --- |
| p=0.70 | 1/3, precision 0.333 | 1/1 | 71/75 |
| p=0.80 | 0/0 | 1/1 | 74/75 |
| p=0.85 | 0/0 | 1/1 | 75/75 |
| p=0.90 | 0/0 | 0/0 | 75/75 |

History fires at all only at p=0.70, at precision 0.333, and costs four contract
false positives. Combining the split with hypothesis 1 gives the same result.
The two modes are not what is wrong.

### 3. Not in the embedding — right in substance, wrong as posed

The distinction *is* encoded. Twenty matched pairs, the same claim phrased as
contract and as change event, project onto the fitted history direction with a
mean gap of +0.172 and 19 of 20 in the right direction. The tautology control,
eight pairs measured the same way, gives +0.182 and 8 of 8. By that measure the
history axis is exactly as real as the tautology axis.

What the same measurement shows is why it is useless:

```
cosine similarity                        mean
matched pair, same claim both ways      0.597
two history comments, different topics  0.148
two contract lines, different topics    0.121
random pair from the fitting corpus     0.090
```

Reframing a claim from contract to change event moves it from 1.0 to 0.597.
Changing the subject moves it to about 0.13. Subject matter is roughly four
times the signal that framing is, so in a class whose members span unrelated
subjects the framing component cancels under averaging and the centroid lands on
whatever topic the exemplars happen to share.

That is directly observable. Fitting the history direction from two disjoint
samples of real history comments gives two directions that barely agree:

| class | cos(direction from mined, direction from held-out) |
| --- | --- |
| history | +0.317 |
| narrative | +0.303 |
| compat | +0.575 |
| tautology | +0.647 |

Cross-fitted both ways, the honest ceiling for history is precision 1.000 at
recall 1/10 (mined to held-out) and 2/10 (held-out to mined), and those cuts are
chosen on the test set. With the shipped direction, held-out history precision
never reaches 0.70 at any cut whatsoever.

The three classes that work have a lexical register of their own: an imperative
verb and an object, `kept for compatibility`, an enumeration of steps. That
register is a large component of the vector and it survives averaging. History
has no register. Its members are ordinary sentences about ordinary subjects,
marked only by tense and by where their truth condition sits, and neither is
something a sentence-similarity model was trained to make large.

## What else was measured

**The in-sample oracle is a trap.** A direction fitted from the held-out labels
themselves reaches precision 1.000 at recall 8/10 on those same rows. It does
not survive cross-validation, which is the +0.317 above.

**The hand-written exemplars are actively harmful.** The 21 in `corpus.go` are
terse and first-person-plural (`reverted the change from last week`); real
change-event comments are long, third-person, and carry their subject matter.

| history direction fitted from | cos against a fit from real comments |
| --- | --- |
| the 21 hand-written | +0.220 and +0.189 |
| compat's 13 hand-written | +0.623 and +0.584 |
| tautology's 20 hand-written | +0.527 and +0.643 |

Fitted from the hand-written set alone, history never reaches 0.70 precision at
any cut. Rewriting the 21 into the register real comments use moves the
direction from cos +0.301 to +0.384 against the oracle and lifts recall from
0.000 to 0.700, but precision only reaches 0.636 and four contract rows fire.
Adding the rewrite on top of the existing 21 rather than replacing them is worse
than either alone: precision 0.250.

The four rows it wrongly fires on say what the direction has actually learned:

```
the runtime offers no cross-operation transaction, so a mid-batch error can
    leave a partial write for version control to roll back
the database is written first, transactionally, and the tree after: a failed
    file removal leaves the record consistent
dropping a column is the same table rebuild the engine forces on any schema change
in a dry run nothing was mutated and the changed-file list is empty
```

All four are contracts about state changing over time. The direction separates
sentences that are *about change* from sentences that are not, which is not the
distinction the class needs: a change-event comment is one whose truth condition
sits in the repository's past, and that is a property of reference rather than of
topic.

**The fitting method is not the limit.** L2 logistic regression with the
positives reweighted, on the same corpus, gives history no held-out cut reaching
0.70 precision and a cross-fit agreement of +0.319 at lambda 0.01 and +0.353 at
lambda 0.10 — the centroid difference's +0.317, within noise.

## Verdict

No hypothesis yields a history class firing at precision 0.85 or better with any
recall, so nothing was changed. The corpus is not the fault, the split is not the
fault, and the fitting method is not the fault: a class distinguished by tense
and reference rather than by register cannot be recovered from
all-MiniLM-L6-v2 embeddings by a single direction.

`narrative` has a milder form of the same disease and is worth watching: its
cross-fit agreement is +0.303, and its held-out precision of 1.000 rests on one
firing.

What would be worth trying next, in rough order of cost: a model that encodes
tense and reference rather than topic similarity (a natural-language-inference
or reranker cross-encoder scoring the comment against `this sentence describes a
change that was made` and against `this sentence states what the code
guarantees`); or accepting that this class needs the diff, which the hook has
access to and the embedding does not.
