# Known limits

The ones that change whether to run it are in the README. These are the rest, kept because a limit nobody wrote down is
rediscovered by whoever hits it next.

## Coverage

- `Jenkinsfile` and other Groovy: no grammar wired.
- A five-word fragment carries too little for an embedding to place.
- Rust documentation used not to reach the padding rule, for a reason that turned out to matter far more elsewhere.
  tree-sitter-rust ends a `line_comment` at column zero of the row below, so `adjacent` read every `///` as multi-line
  and no run was ever grouped. The padding rule declining to run was the harmless half. The damaging half was that
  `leftover` then judged each line of a ` ```rust ` doc example on its own and reported the body of a test `cargo test`
  compiles and runs as commented-out code, at score 1.000, which wins the three-finding budget outright. Measured on the
  mined corpus that was 143 of `leftover`'s 298 false positives, a quarter of the tool's whole false-positive rate.
  `written` now asks what row a comment last puts text on rather than where its node ends; removing those 143 cost no
  catches and took the corpus false-positive rate from 0.047 to 0.039.
- Generated files are not skipped. A `//go:generate` marker is, but a table generated without one —
  `syscall/zsysnum_*.go` — is read like anything else.
- The legality check that rules out equations misread as code is wired for Go alone. Every other language still decides
  on a bare parse plus a lexical prefilter, which is what all of them did before any was measured.

## Shapes it gets wrong

- An equation whose left side is a plain identifier is legal Go and is still read as code: `// EM = 0x00 || 0x02 || PS`,
  `// U_n = PRF(password, U_(n-1))`. Separating those from a switched-off assignment needs the identifiers resolved
  against the file, since these resolve to nothing in it. **Tried, measured, reverted.** Requiring one name in the
  comment to occur elsewhere in the file, comments excluded, removes 4 of the 130 findings on the Go standard library —
  three section banners and a `// done (label)` note — and costs no catch on the mined corpus. It also silences
  `// var timeout = 5 * time.Second`, because commenting out the last use of a package takes the import with it, so the
  cleanest true positive there is grounds nothing. Four findings is not worth a systematic miss on that shape.
- A compiler pass sketching the code it emits is the largest single false-positive family, about 29 of the Go standard
  library's findings and nearly all of `cmd/compile/internal/walk`: `// hv1 := 0`, `// hp = &a[0]`. The sketch names the
  variables the pass manipulates, so it grounds every name and reads as legal Go. It fails to *type* check —
  `no new variables on left side of :=` — and that is the only thing measured that separates it. A type checker is two
  orders of magnitude outside a hook's budget and exists for one of the fourteen languages, so this shape stands.
- A comment run that opens with a licence line pardons every line stacked under it, because a run reads as one comment
  and any of its lines can carry the marker.
- A contract stated in the words of its own signature reads as padding when several of them stand together.
  `java.time.zone.ZoneOffsetTransitionRule` documents three enum constants in parallel — "The STANDARD type uses the
  standard offset" — over a method whose parameters are `standardOffset` and `wallOffset`, so every word is one the
  declaration spells. Two findings in 15,605 JDK files, and the shape has no tell beyond being right.
- A commented option under a key that already has values is reported, which on a stock `helm create` scaffold is two
  findings. A commented block indented under a key whose value is an empty collection is documentation rather than
  residue and is spared: `podSecurityContext: {}` over `# fsGroup: 2000` is how a chart shows what a setting takes.
- The same replacement text occurring twice in a file is claimed twice. Only the bytes an edit changed are attributed to
  it, but where those bytes appear more than once there is nothing in the payload that tells the copies apart.

## Judgments somebody will disagree with

- A step comment inside a long function — `// Sort edges.` over `sort.Sort(edges)` — is a finding, and plenty of
  engineers would defend it as what makes a two-hundred-line routine readable. The rule this tool serves reads the urge
  to write one as a signal the block wants to be a function. It fires wherever the comment's words are already spelled by
  the line under it, whatever follows. Under `SLOPGUARD_WIDER=1` on the Go standard library that was the largest class
  the tool had, 623 of 1034; the shipped default runs neither `echo` nor `tautology`, so the figure is history rather
  than behaviour. A run of them restating four consecutive trivial lines is the shape the rule is aimed at, and a gate on what
  follows the line reduces that run to one finding, so there is no version of this that flags the run and spares the
  banner.
- The padding rule fires only where its own instruction has a target: not on a file's own documentation, since that is
  already the first home it names, and not in a language with no function, since YAML and HCL have no symbol to document
  and no test to move a claim into.
- A finding is remembered when it is named, not when it is acted on. Ignoring a nudge silences it for the rest of the
  session, rewording earns a fresh one, and deleting the line goes quiet. The cheapest paths to silence are still ignore
  and delete; the wording argues against both, and nothing enforces it.

## Classes that were tried and dropped

Change-event comments — `// we now use the pooled client` — were the original point of the tool and are not in it. The
distinction is present in the embedding but does not survive averaging: directions fitted from two disjoint halves agree
at cos +0.32, against +0.575 and +0.647 for the two classes that shipped. What the direction learns is *is about change*,
not *its truth condition is in the past*, and every false positive is a contract about state that changes over time. A
phrase list is worse: `// returns every durable the consumer no longer runs` is a contract that spells a marker, and
across 33 files of a production service every one of the seven findings a phrase list produced was that shape. The full
diagnosis — three hypotheses, the numbers that refuted each, and the two things worth trying — is in
[history-class.md](history-class.md). The labelled examples are kept in `internal/model/aa_heldout_test.go` for whatever
tries next.

A narration class was dropped more cheaply. Its direction converged, but it fired once on the held-out set, never across
three production repositories, and not on its own training sentence. A class that says almost nothing is carrying risk
for nothing.

Implementation narrative is not caught, and three mechanisms aimed at it were built and reverted.
[narrative-class.md](narrative-class.md) has the measurements.

## Naming

The classes are named `tautology`, `echo`, `compat`, `leftover` and `hollow` everywhere the tests and the sweep print
them. `tautology` and `echo` are the model's reading of restatement and the structural one, they carry the same wording,
and the README counts them together as restatement. `compat` is self-justification, `leftover` is commented-out code, and
`hollow` is padded documentation.
