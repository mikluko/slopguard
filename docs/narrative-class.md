# Why implementation narrative is not caught

The padding rule reads a documentation sentence against the declaration it
documents and reports the ones that say nothing it did not. It catches two
shapes and misses a third, and the third is the commonest thing an agent writes:

```
It first opens a transaction, then writes the user row, and finally commits
the transaction.
```

That sentence names a transaction, a row and a commit. It contributes
vocabulary, so every closure test reads it as contributing, and it guarantees
nothing a reader could not see by looking at the four lines below it.

Three mechanisms were built to reach it. All three were measured on the Go
standard library — 4,065 files, no test data, no vendored code — against a rule
that reports zero there. All three were reverted.

## What the reviewer measured first

A reviewer was asked to write seven doc comments in the style coding agents
actually produce, without having read the word lists. The rule reported **none**
of them. They then rephrased three of the rows in `recall_test.go` that do fire,
adding one ordinary word to each — "both of which are optional", "an in-memory
cache" — and **two of the three went silent**.

That is the number this document exists to explain. Six of the seven are
implementation narrative or free-floating rationale; the seventh is a
`Parameters:`/`Returns:` block, which carries no sentence-ending punctuation and
so never splits into more than one piece.

## Attempt 1: phrase frames

Agent padding uses a small number of fixed frames — *is responsible for*, *can
be used to*, *allows for greater*, *under the hood*, *in other words*, *main
entry point*. Matching the frame rather than the words sidesteps the vocabulary
problem entirely, and it caught five of the reviewer's seven.

**Cost: 27 false positives.** The frames are not the agent's. They belong to the
language:

```
the As method is responsible for setting target.        errors
In other words, the representation must be a bijection. encoding/gob
Under the hood, it's a bitmap.                          runtime
```

`is used to` opens hundreds of standard-library sentences. This is the mistake
the repo already made once, where a phrase list read `// returns every durable
the consumer no longer runs` as a change-event marker: a frame does not know who
its subject is. "The caller is responsible for closing the file" is a
precondition and "this function is responsible for closing the file" is padding,
and one phrase carries both.

## Attempt 2: firing on one hollow sentence rather than two

The doctrine puts the burden on the second sentence — "a second is earned by a
precondition, an invariant, a failure mode, or a cost" — so one hollow sentence
past the opener is what it literally asks for, and the two-sentence trigger
gives every doc one free line of padding.

**Cost: 21 false positives.** They are contracts stated in the words of their own
signatures, which is ordinary:

```
It returns the element value e.Value.       container/list
It returns the value and the new offset.    encoding/binary
On return, data[newpivot] = p               sort
Use ExpandString to expand EXPAND_SZ.       syscall
```

A terse contract and a padded comment look identical at one sentence. The second
hollow sentence is what separates them.

## Attempt 3: the body's own vocabulary

The sharpest idea of the three, and the one that failed most instructively. A
narrative sentence retells the implementation, so its words should be the
implementation's words — `Open`, `WriteRow`, `Commit` are three calls in the
body. That is relational, like the signature test, so it is not vulnerable to
the averaging failure that killed the change-event class. It worked on the
constructed case exactly as designed, and left a real contract alone:

```
It first opens a transaction, then writes the user row, and finally commits.
  first(scaffold) open(body) transaction(body) write(scaffold) user(sig)
  row(body) finally(scaffold) commit(body)            -> nothing novel

The delay doubles each time and is capped at one minute, which keeps a
failing dependency from being hammered.
  novel[double capped one keep failing dependency hammered]  -> earns
```

**Cost: 116 false positives**, and every one a contract:

```
It panics if v's kind is not [Bool].                         reflect
It returns every address when the filter is nil.             net
StartTrace returns an error if tracing is already enabled.   runtime/trace
If fd is not a directory, it closes it and returns an error. os
```

The reason is not a tunable threshold. **A contract about what a function does
is stated in the vocabulary of what it does**, and what it does is what the body
implements. There is no lexical separation between describing behaviour, which
is the whole job of a doc comment, and narrating implementation, which is
padding. The difference is whether the reader could have derived it, and that is
not a property of the words.

## What this leaves

The rule keeps the scope a structural test can hold: a sentence that only
respells its signature, and one that only rates the code, at two or more per
comment. Zero findings on 4,065 standard-library files, 1,088 first-party Django
files, and its own source.

The residual is stated in the README's known limits rather than chased. Reaching
it needs something that can ask whether a claim is derivable from the code, not
whether its words are new — which is the entailment test the doctrine actually
specifies, and which nothing here can compute.

One thing that would help and is not a mechanism: a `Parameters:`/`Returns:`
block is invisible because `split` finds no sentence in it. That is a parsing
gap rather than a judgment one, and it is worth closing on its own terms.
