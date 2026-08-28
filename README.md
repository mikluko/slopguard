# slopguard

A Claude Code `PostToolUse` hook that reads the comments a write just added and says which of them belong somewhere
else: in the commit message, in a test, or nowhere.

## TL;DR

**Why.** An agent comments code out instead of deleting it, and leaves it there. The next reader cannot tell dead code
from a note, git already remembers what was removed, and nobody re-checks a comment. slopguard reads every comment a
write adds and names the ones that are source in disguise, at the moment it is cheapest to delete. It objects and never
blocks.

```
line 42  commented-out code: delete it, or make it real
```

**Try it on a repository, wiring nothing.** Given paths instead of a hook payload it judges those files and prints what
it finds, one finding per line:

```sh
brew install mikluko/tap/slopguard
cd <a repo>
git ls-files -z | xargs -0 slopguard -v
```

`-v` prints each comment with the line under it:

```
internal/store/lifecycle.go:4	leftover	1.000	commented-out code: delete it, or make it real
	| // if item.Status == StatusDraft {
	| return nil
```

It prints the comment's first line and the first line of code under it, which is what you need in order to say whether a
finding is right. The run continues past what is shown.

A file in a language it does not read produces nothing, so passing the whole index is safe. The sweep writes nothing and
remembers nothing; it is the same judgment the hook makes, without the hook. For scale, the Go standard library gives
130 findings over 4,065 files, every one of them from the commented-out-code rule — which is a statement about which
rule fired, not about how many are right. On that particular library, most are not: see below.

**What ships is `leftover` alone.** `echo`, `hollow`, `tautology` and `compat` are behind `SLOPGUARD_WIDER=1`, off by
default. On a corpus of comments other people deleted or kept, `leftover` catches 20 and nudges 1 where the whole set
catches 21 and nudges 20: one extra catch for nineteen extra false alarms. The two semantic classes are also what loads
the 90 MB model, which is a second on the first write that reaches it. The binary is the same size either way, since the
model is embedded at compile time. `docs/metric.md` carries the measurement and the argument for why this is a default
rather than a deletion.

**How often it is right, and where.** Judged by hand in context, precision splits hard by what the code is, and the
average across them describes no actual repository:

| population | roughly right |
|---|---|
| application and library code, tests, config | 6 to 9 in 10 |
| compilers, codegen, crypto and spec implementations | about 1 in 4 |

The Go standard library sits at the bottom of that range and is the honest figure for anyone writing systems code. The
wrong ones are all one thing: notation that is valid source. A compiler pass sketching the code it emits, an RFC or NIST
step written as an assignment, a section banner. Those live where the tool is weakest and are rare where it is strong.

Treat these as a range, not a measurement. The exemptions above were chosen by reading the same hand-judged findings the
figures are computed over, so they are fitted to that sample and no held-out set has been judged; four readers worked on
disjoint findings, so there is no agreement rate behind them either. `docs/metric.md` says what would have to be done to
turn them into an estimate. **Read a finding before acting on it.**

**Wire it.** Add the `Write|Edit|MultiEdit` matcher under [Configure](#configure) to `~/.claude/settings.json` and
restart the session.

## Why this is not a lint rule

A linter reads a comment as a token. Whether `// x = y` is a disabled statement or the notation a spec step is written
in depends on what is around it, so slopguard parses the comment as source in the language of the file it sits in, in
the scope it sits in, and asks whether the tree that comes back is one the compiler would have taken. A comment that
merely looks like code does not survive that; a comment that is code does.

`PostToolUse` fires after the write, the finding comes back as context, and what the agent does about it is the agent's
business.

## What it says something about

| Class                | What fires it                                                    | Default |
|----------------------|------------------------------------------------------------------|---------|
| commented-out code   | a comment that parses as source the compiler would take          | on      |
| restatement          | `// close the connection` above `conn.Close()`                   | off     |
| self-justification   | `// kept for backwards compatibility`                            | off     |
| padded documentation | a sentence past the first that says nothing the signature does   | off     |

Only the first ships. The other three are one variable away — `SLOPGUARD_WIDER=1` — and turning them on loads a 90 MB
sentence embedding model, because two of them are decided by what a sentence means rather than by its shape. They are
off because they were measured: on the mined corpus the four of them together buy one extra catch for nineteen extra
false alarms, and on the Go standard library they are four fifths of everything the tool says.

Change-event comments — `// we now use the pooled client` — were the original point of this tool and are not in it,
because the class could not be made to work. [docs/limits.md](docs/limits.md) says what was tried.

## What it leaves alone

A comment sharing a line with code, whatever it parses as. Code is disabled by commenting the line it is on, which
leaves the comment alone on that line; a comment after a live statement is a note about it, and the notation those notes
use is the notation this rule would otherwise read as source:

```
num -= old.cap - old.len // preserve memory of old[old.len:old.cap]
x2 := Sqrt(x1)           // x2 = sqrt(1 - x*x)
B2 = 696219795           // (664-0.03306235651)*2**20
```

Machine-readable comments are skipped outright: shebangs, build constraints, `//go:generate`, `//nolint`,
`// Deprecated:`, `# noqa`, `# type:`, `// eslint-disable`, Renovate annotations, editor modelines, and their
neighbours.

A licence notice is exempt from every rule. Some of them ask in their own text to be preserved.

Configuration that documents itself is exempt where the idiom is unambiguous. A chart's `values.yaml` publishes the
settings it accepts by writing them commented out, so nothing there is read as residue; elsewhere in YAML, a run
carrying a sentence beside its structure is an option being documented rather than one removed:

```yaml
## Optionally specify an array of imagePullSecrets.
# pullSecrets:
#   - myRegistrKeySecretName
```

## Languages

Go, Python, JavaScript, TypeScript, TSX, Rust, C, C++, Java, Ruby, PHP, shell, YAML and Terraform, plus `Dockerfile`,
`Containerfile` and `Makefile` by name rather than by extension. A file nothing here reads produces nothing.

A file that does not parse is still judged, and this document claimed the opposite for a long time. Declining a file whose
tree carries an error was tried and reverted: an error node means tree-sitter could not parse the file, not that the file
is broken, and it drops 16 of the 168 findings on a sweep of the mined clones — every one C, where the grammar loses its
footing in the preprocessor. What is skipped is a *comment* whose own text does not parse as source, which is a different
test and the one that stops prose being read as code.

Two languages are read as something other than themselves. Helm templates are read as the YAML they become, so a manifest
opening with `{{- if }}` keeps its comments, and `.tpl`, `.gotmpl` and `.tftpl` are mapped the same way — skipping them by
extension left `templates/_helpers.tpl` unread in every chart. A commented-out Dockerfile instruction is told from prose
by its case, because `# copy the buffer first` parses as a perfectly good `COPY`.

`values.yaml` is read as YAML that documents itself, so the commented-out-code rule does not run there at all.

## Install

```sh
brew install mikluko/tap/slopguard
```

Or build it:

```sh
git clone https://github.com/mikluko/slopguard && cd slopguard && go build .
```

The default needs nothing else. ONNX Runtime is required only by `SLOPGUARD_WIDER=1`, which is what loads the model;
without it the wider classes that read a sentence stay quiet and the rest run unchanged. Install it with
`brew install onnxruntime` if you want them. The binary dlopens the library at run time, looking in Homebrew's prefix on
both architectures, Linuxbrew's, and the two places a Linux package manager puts it.

## Configure

Add a `Write|Edit|MultiEdit` matcher to `PostToolUse` in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit|MultiEdit",
        "hooks": [
          {
            "type": "command",
            "command": "slopguard",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

Use an absolute path if sessions start without the Homebrew prefix on `PATH`. Restart the session to load it.

| Variable                        | Effect                                                                               |
|---------------------------------|--------------------------------------------------------------------------------------|
| `SLOPGUARD_WIDER`               | `1` adds restatement, self-justification and padded documentation; off by default    |
| `SLOPGUARD_ONNXRUNTIME_LIBRARY` | where to dlopen ONNX Runtime from, when it is in none of the usual places            |
| `SLOPGUARD_NO_MODEL`            | turns the semantic pass off, leaving the structural rules; a no-op unless `WIDER`    |
| `SLOPGUARD_STATE`               | where the per-session memory of what has already been said lives; empty turns it off |
| `SLOPGUARD_LOG`                 | a file every finding is appended to, one JSON object per line                        |

The last two write to disk. `SLOPGUARD_STATE` defaults to `slopguard` under the user cache directory, and files there
that have gone a day without a write are removed — only files this program wrote, whose names are a single base-36
number, so pointing it at a directory of your own will not empty it.

It loads a native library into its own process and is not a sandbox.

## Contract

Reads the hook payload on stdin, writes a `PostToolUse` result on stdout, always exits 0. A write it does not object to
produces no output at all.

It fails open. A payload it cannot decode, a tool other than `Write`, `Edit` or `MultiEdit`, an extension with no
grammar, a file that is gone or is not a regular file or is over 2 MB, and replacement text it cannot locate in the file
all yield silence, because none of them is evidence of a comment. A source that does not parse is **not** on that list —
see [Languages](#languages).

The default never opens ONNX Runtime, so a machine without it runs the shipped build unchanged. Under
`SLOPGUARD_WIDER=1` a missing runtime does not go silent, it goes stupid: the structural rules still run and a phrase
list stands in for the one model class it can approximate, judging differently from the model in both directions.

A finding names the line and the rule, at most three per write, strongest first:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PostToolUse",
    "additionalContext": "slopguard is a heuristic. Many of its findings are wrong, and in code that implements a specification or generates other code, most are. It parsed these comments in store.go as valid source:\n\n  store.go:42  commented-out code: delete it, or make it real\n\nPer line: if this write commented out live code, delete it — git has it. If it is spec or algebraic notation, a sketch of the code a pass emits, a label naming what a case arm handles, or a section heading, then it is right as written: leave it and carry on, no reply needed. Those four are the measured false positives. Do not reword a comment to satisfy this, and do not act on a comment this write did not author."
  },
  "systemMessage": "slopguard: 1 comment to reconsider — store.go:42"
}
```

One thing it says without hedging: where a single `Edit` turned live code into the comment being reported, the tool
watched that happen and says so. A `MultiEdit` never gets that sentence, because several replacements cannot be tied to
one comment by text alone — one of them can write a copy and another delete it, leaving a payload that vouches for a
comment nobody wrote in that write.

The nudge says it is often wrong, and where. It does not quote a rate: the pooled figure is fitted to the sample the
exemptions were chosen from, and an agent handed a number acts on it. It names the four shapes it gets wrong so the agent
can dismiss one at a glance, and makes leaving a comment alone cost nothing — an agent charged for defending correct code
learns that deleting it is cheaper. The `systemMessage` carries the lines rather than a count, since the human is the
only party who can tell a true finding from a false one.

A write is judged in single-digit to low-teens milliseconds. Under `SLOPGUARD_WIDER=1` one that reaches the model pays
about a second on the first call, most of it opening the ONNX session, and 115 to 145 ms after that.

## Limits

- **A large share of what it says is wrong** — see the table above for how large, and note it is fitted rather than
  estimated. Read the finding; do not act on the count.
- The shapes it gets wrong are notation that is valid source: a compiler pass sketching the code it emits
  (`// hp = &a[0]`), a spec step (`// V = HMAC(K, V)`), a comment naming what a `case` arm handles, a section heading.
  All four are legitimate and all four parse.
- Recall is modest by construction. **A comment this tool passes over is not a comment it approves of.**
- Change-event comments are not caught. Neither is implementation narrative, restatement, or padded documentation —
  the last two only under `SLOPGUARD_WIDER=1`.
- A finding is remembered when it is named, not when it is acted on: ignoring a nudge silences it for the session.
- Generated files are not skipped. A `//go:generate` marker is skipped as a comment, which is not the same thing and
  does not exempt the file it sits in.

[docs/limits.md](docs/limits.md) has the rest, including the shapes it gets wrong and the classes that were tried and
dropped.

## Development

```sh
go test ./...
```

Given file paths instead of a payload, the tool judges those files and prints what it finds, one finding per line. `-v`
prints the comment and the line under it as well, which is what a sweep has to show before anyone can say whether it is
right:

```sh
slopguard -v $(git ls-files '*.go')
```

The tables are the specification: `internal/rule/rule_test.go` for what fires per language, with
`internal/rule/python_test.go` and `internal/rule/yaml_test.go` beside it for the two languages whose prose parses, and
`internal/model/semantic_test.go` for what the model must and must not recognise. A false positive or a missed comment is
a row added there first.

Tests that judge text skip when ONNX Runtime is absent; the parsing and structural tests run either way, so a checkout
without it still exercises the language table, the commented-out-code rules and the identifier echo.
`TestShippedRunsLeftoverAlone` is the one that asserts the default configuration, and it needs no runtime.

`go test ./internal/model -v -run TestStability` reports how far each class's direction moves when it is fitted from one
half of its own examples rather than the other. It fails below cos 0.45. That is the measurement the deleted change-event
class failed at +0.32 while looking healthy from every other angle.

[docs/design.md](docs/design.md) is how the judgment works and how the thresholds were fitted.

## License

The source is MIT. The binary is not only the source: it embeds all-MiniLM-L6-v2, which is Apache 2.0, so every build
redistributes Apache 2.0 material and carries that licence with it. The text is in
`internal/model/assets/LICENSE.apache-2.0` and `internal/model/assets/PROVENANCE` is the notice, with the revision and
the checksums to verify what was embedded.
