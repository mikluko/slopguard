package main

import (
	"fmt"
	"sort"
	"testing"
)

// The held-out table is mined from repositories that had no part in tuning:
// none of its comments contributed an exemplar to corpus.go, a row to
// semantic_test.go, or a floor. Each text is one comment's prose with the
// markers stripped and the project's own names generalised, so what is measured
// is the reading rather than the vocabulary of any one codebase. It is the
// generalisation check the calibration set cannot be: the same sentences that
// set a floor cannot also report what that floor costs.
var labelled = []struct {
	text  string
	class string
}{
	// --- history: change-event explanation ---
	{"a self-closing tag used to be classified as an open tag, which routed everything after it into the wrong bucket for the rest of the stream", "history"},
	{"temporarily disabled: only the session-started events are sent", "history"},
	{"only the onboarding events are enabled for now", "history"},
	{"for now we inherit the caller's own provider and model", "history"},
	{"skip the other content types for now", "history"},
	{"a real run failed mid-batch on exactly this, and the dry run at the time did not catch it", "history"},
	{"this check is what closes the gap the earlier dry run left", "history"},
	{"the audit finding from the dry-run review closes that", "history"},
	{"the per-kind partitions the previous schema enforced as column constraints live here now", "history"},
	{"several stems ran past the old forty-character cap, so this change raises it to seventy-two", "history"},
	{"the bug caught against the real corpus, where an unrelated path was wrongly rewritten onto a mapped number", "history"},
	{"the membership probe used to hang off the provider, which is why the shape here looks indirect", "history"},
	{"resolving attendance used to cost a second call, so the count rides along now", "history"},
	{"it used to be hidden behind the closed key, which meant nobody saw it", "history"},
	{"the total is no longer maintained by hand: it now comes from the read", "history"},
	{"the ordering is alphabetical after the known ids, though currently there are none", "history"},
	{"re-minting the descendants after a rotation is not automated yet", "history"},
	{"the per-entity floors are not stamped on the wire yet; that is a later step", "history"},
	{"stamp the record's own historical date rather than today, which is the gap this closes", "history"},
	{"the loop that used to write through the logger directly is the same line it is now", "history"},

	// --- compat: the symbol justified by its own history ---
	{"legacy alias kept for callers that have not migrated their import path yet", "compat"},
	{"this transport is no longer supported and is kept only for config file compatibility", "compat"},
	{"the key is kept under its old name for backward compatibility with persisted user settings", "compat"},
	{"the timeout is optional for compatibility, though new configurations should set it", "compat"},
	{"fetch the raw files; kept for the spec resolver", "compat"},
	{"whether this provider is retained only for compatibility", "compat"},
	{"the doctor path still needs the legacy client, so keep that lookup contained here", "compat"},
	{"handle both the legacy environment block and the newer secrets block", "compat"},
	{"migrate the old reasoning content to thinking, dropping the invalid legacy blocks", "compat"},
	{"the vendor name is kept here for backwards compatibility with existing user directories", "compat"},
	{"handle the legacy encodings as well as the current one", "compat"},
	{"the boolean and null forms are accepted for compatibility with the upstream file format", "compat"},
	{"this type is exposed for migration tooling that reads the upstream format directly", "compat"},
	{"the legacy bool form", "compat"},
	{"the old role name is mapped onto the current one", "compat"},
	{"this alias is only here until the callers are moved over", "compat"},

	// --- tautology: restates the code below it ---
	{"convert to YAML for storage", "tautology"},
	{"write the contents using the same file handle", "tautology"},
	{"wait for all threads to complete", "tautology"},
	{"print the final values for debugging", "tautology"},
	{"read the file directly", "tautology"},
	{"add the new directory at the beginning", "tautology"},
	{"remove the directory if it already exists", "tautology"},
	{"add to the appropriate list", "tautology"},
	{"check the permission levels", "tautology"},
	{"serialize to JSON", "tautology"},
	{"convert to an array and sort by date", "tautology"},
	{"read the response stream", "tautology"},
	{"save to the downloads directory", "tautology"},
	{"get the lock and update the values", "tautology"},
	{"write all the values", "tautology"},
	{"log the decoded update content", "tautology"},
	{"log the decoded outcome", "tautology"},
	{"log the error if present", "tautology"},
	{"collect the contiguous change ranges", "tautology"},
	{"merge the groups that are close enough to be in the same hunk", "tautology"},
	{"render the item's inline content and any nested blocks", "tautology"},
	{"add a single blank line between blocks, but not after the last block", "tautology"},
	{"Name returns the tool name", "tautology"},
	{"Description returns the tool description", "tautology"},
	{"an Unsubscriber represents something that can be unsubscribed", "tautology"},
	{"supersede the old issuance and record the new one", "tautology"},
	{"find the drafts and sent folder ids", "tautology"},
	{"submit the message for delivery", "tautology"},
	{"upload the blob if content was provided", "tautology"},
	{"resolve the sender identity", "tautology"},

	// --- narrative: a walkthrough of how it works ---
	{"it resolves an optional template, unions the template's grants with the explicit flags, reconciles the validity window, mints with the library, and persists both the new record and the issuance", "narrative"},
	{"it keeps the anchor key, advances the counter, re-mints the self-signed token, and then re-issues every live credential beneath it", "narrative"},
	{"it swaps the allowlist entry from the old identifier to the new one, marks the superseded issuance revoked, records the new issuance, and re-mints each live credential beneath it", "narrative"},
	{"allocate a record per entry in ascending order, place each body with its cross-references rewritten, resolve the status, and derive the edges from the frontmatter", "narrative"},
	{"the handler receives the event, resolves the identity, and writes the result back onto the response", "narrative"},
	{"it waits for the config file to exist, then starts the process, and restarts it on exit until the context is cancelled", "narrative"},
	{"we build the request, send it, read the stream line by line, and assemble the accumulated partial content at the end", "narrative"},
	{"first we acquire the lock, then we drain the queue and release it", "narrative"},
	{"step one resolves the file id to a path, step two downloads the raw bytes", "narrative"},
	{"the parser walks the node tree, removes the blockquote elements in place, and returns the flattened text", "narrative"},
	{"this works by iterating over every entry and applying the transform to each one", "narrative"},
	{"the loop reads each response, filters out the empty ones, and appends the rest to the slice", "narrative"},

	// --- contract prose: leave alone ---
	{"implementations must be safe for concurrent use and may be backed by memory or shared storage", ""},
	{"a non-nil error rejects the request as unauthenticated", ""},
	{"names are asserted by their issuer, and nothing at issuance checks uniqueness", ""},
	{"bearer credentials are replayable until they expire, so pair them with transport security and a short window", ""},
	{"it does not check expiry or membership; the layer above adds those so callers get precise errors", ""},
	{"running after possession is proven means an expensive validator is never triggered by a party that cannot sign", ""},
	{"the nonce is retained for twice the skew window, the longest a replay could still land inside a valid timestamp", ""},
	{"bumping the counter in a fresh anchor token revokes everything minted in earlier ones at once", ""},
	{"a token not self-signed by the anchor poisons the verifier, so every request fails rather than silently skipping policy", ""},
	{"prove possession of the subject key before running any consumer-supplied hook", ""},
	{"an anchor token does not combine with a keyring: the entries carry the policy", ""},
	{"a missing extension yields the zero value and false", ""},
	{"retrieval never requires registration; this only moves malformed-extension failures to authentication time", ""},
	{"these are proofs of origin, never credentials: possession grants nothing", ""},
	{"the two headers carry the chain detached from the token itself, so no single header grows large", ""},
	{"overrides the time source, for tests", ""},
	{"header field names carrying the credential on each request", ""},
	{"an unknown kind is treated as unresolved: an unclassifiable blocker never silently stops blocking", ""},
	{"an empty source list means any status in the vocabulary", ""},
	{"kinds without a close rule never close", ""},
	{"the registry is the only authority on which statuses a kind may carry and how it may move between them", ""},
	{"a relation absent from the map denies both roles", ""},
	{"it is a variable rather than a literal so tests can register synthetic entries; the binary's own set is fixed at startup", ""},
	{"an unknown kind is an error: the registry is the vocabulary", ""},
	{"the callers read this to gate the operation rather than hardcoding the kind", ""},
	{"matching alone is not sufficient to rewrite: the full old filename must agree with the mapped record", ""},
	{"every check here runs identically whether this is a dry run or not, so a dry run fails on exactly what a real run would", ""},
	{"the rewrite is keyed by the map, never by the pattern", ""},
	{"a reference whose number is outside the map is left untouched", ""},
	{"the records are ordered by ascending number, which is what makes the allocation deterministic", ""},
	{"validation runs before any mutation, so a malformed input fails closed without burning identifiers", ""},
	{"the runtime offers no cross-operation transaction, so a mid-batch error can leave a partial write for version control to roll back", ""},
	{"embedded in a longer path; leave it untouched", ""},
	{"the allocation already lands on the initial status; there is nothing further to do", ""},
	{"is postponed-only metadata", ""},
	{"a closed entry requires a reason of exactly resolved or will-not-fix", ""},
	{"an open entry carries neither a reason nor a closed date: both land at close", ""},
	{"the replacement runs in reverse order so that earlier replacements do not invalidate later indices", ""},
	{"every replacement is matched against the original content, not incrementally", ""},
	{"all edits are validated against the original content before any mutation is written", ""},
	{"a validation failure reports the failing index and leaves the file unchanged", ""},
	{"it returns the validated matches sorted by index", ""},
	{"the second return value is the matched path, the third reports whether any decision was found", ""},
	{"a nil decision removes the entry", ""},
	{"missing or malformed files are non-fatal and return an empty map with a nil error", ""},
	{"user overrides replace the defaults for each action entirely", ""},
	{"returns the keys bound to an action, or nil if none", ""},
	{"write under an exclusive lock; the file may be written by concurrent processes", ""},
	{"if the migrated identifier already exists in the config, the older entry is discarded", ""},
	{"the resolution order is the command-line override, then the nearest saved decision, then the fallback", ""},
	{"the returned source indicates which rule applied", ""},
	{"it is tolerant of the timestamped object form by extracting the decision field", ""},
	{"the caller owns the send side of the channel: it must only receive from it and must never close it", ""},
	{"this side is the sole sender on the channel, so it closes it on teardown", ""},
	{"the caller can detect teardown via a receive returning the zero value", ""},
	{"the channel is the one delivery writes into; the mirror goroutine is its sole reader", ""},
	{"stop delivery first so nothing new lands in the buffer", ""},
	{"a version tag, then the timestamp bound to a hash of the request", ""},
	{"the ordering is the anchor first, then the account, then the user", ""},
	{"the database is written first, transactionally, and the tree after: a failed file removal leaves the record consistent", ""},
	{"resolution runs file-relative first, then relative to the register root", ""},
	{"the deterministic order is the built-in dimensions, then the named extensions in the order they were added", ""},
	{"the value must be valid JSON, the name must not collide with a built-in, and a duplicate name is rejected", ""},
	{"a bare value is a host; otherwise the value is a list of dimensioned clauses", ""},
	{"the payload is returned byte for byte, so the wire claim is exactly what was provided", ""},
	{"reject a value that is blank, or that carries a clause separator without the dimensioned form", ""},
	{"a name set more than once is an error", ""},
	{"the built-in dimensions union; the named extensions are keyed by name", ""},
	{"single writer by construction: one connection, held for life", ""},
	{"adding an edge closes a cycle if and only if the target already reaches the source", ""},
	{"the header region ends at the first band heading", ""},
	{"only the public fields reach the rendered config", ""},
	{"hidden directories are not descended into", ""},
	{"probe every endpoint concurrently; serial probing would take as long as the slowest one", ""},
	{"log only the state transitions, not every round", ""},
	{"this maps to a Latin letter, and the transliterator drops it, losing the country", ""},
	{"the trigger's own close counts as the first application", ""},
	{"dropping a column is the same table rebuild the engine forces on any schema change", ""},
	{"no credential is needed here, so the parent's pre-run hook is overridden", ""},
	{"defaults to the package default when empty", ""},
	{"the writer the status line is printed to", ""},
	{"each check carries its own handler", ""},
	{"poll the measurement endpoint until the measurement leaves the in-progress state", ""},
	{"the body is truncated at the configured size rather than failing the check", ""},
	{"it evaluates the condition tree against the input and returns whether it succeeded", ""},
	{"the routing flags are converted into the routes the subscriber binds", ""},
	{"a broken tree is not evidence of a comment", ""},
	{"the resolver supplies the signed token for a public key, and raises when it has none", ""},
	{"a custom check that runs after the chain is verified and possession proven", ""},
	{"the extension is decoded from both tokens before the handler runs", ""},
	{"the container version is emitted as a header line and checked on parse, independent of the wire version", ""},
	{"the reference implementation accepts ASCII digits with an optional sign only, so reject the wider forms this language would take", ""},
	{"the validity window is long enough for delivery latency and drift, short enough to bound capture exposure", ""},
	{"validate the option-carried claims first, so option errors surface before the expiry check", ""},
	{"an empty audience is a no-op", ""},
	{"enforcement is keyed on the presence of the option, not its value", ""},
	{"attribute the failure before evicting: only a chainless token leaves the cached entry as the suspect", ""},
	{"a self-contained token verified on its own, so the cached chain was stale and is dropped", ""},
	{"the receiver does not know our chain: retransmit once with the chain detached alongside the same still-valid token", ""},
	{"drain the response so the connection can be reused", ""},
	{"development-only allowlist that accepts everything", ""},
	{"decides whether an issued credential is still accepted", ""},
	{"the label falls back to the public key when the token carries no name", ""},
	{"a consumer trusting several domains must not assume the names are unique across them", ""},
	{"the pinned public key remains the trust anchor", ""},
	{"an identifier below a nested resource no longer resolves against the root's definitions", ""},
	{"changing this would orphan existing installations", ""},
	{"unknown models have no canonical record, so the request must not pin a default", ""},
	{"the newer models reject the older shape and require the adaptive one", ""},
	{"broader scopes are searched after the project root, and a nearer name shadows a farther one", ""},
	{"the editor is chosen from the dedicated variable, then the visual one, then the generic one", ""},
	{"one hung execution must not block the other sessions", ""},
	{"this data feeds the request waterfall", ""},
	{"the host entry is removed from the header map", ""},
	{"the transport owns this field and fills it from the incoming request; a mismatch fails the signature", ""},
	{"the identity is always present; the delegated user is nil for account-level requests", ""},
	{"requests without a signature pass only when the effective token is a bearer one", ""},
	{"the validators run in registration order and the first error wins", ""},
	{"registering the type only moves malformed payload failures earlier", ""},
	{"without the resolver such requests are rejected", ""},
	{"the signatures are checked here, and the trust is established per request", ""},
	{"this is the display view of a record for the show and list output", ""},
	{"it fails if the store already holds a live anchor", ""},
	{"the identifier is the allowlist key the server verifies against", ""},
	{"depositing the identifier in the allowlist is the command layer's job", ""},
	{"the prior generation is preserved as a retained row", ""},
	{"the allowlist keys on the account identifier, so minting a user generation does not touch it", ""},
	{"a token with no extensions is only ever minted through the explicit opt-in", ""},
	{"the caller confirms before calling", ""},
	{"returns what would fall and what would be revoked, without changing anything", ""},
	{"the store holds exactly one anchor, so this is the store's own name", ""},
	{"the identifier is a content hash over the counter, so a re-issued token has a new one", ""},
	{"scaffolding rather than records: never imported", ""},
	{"the file name must match the four-digit number and the slug shape", ""},
	{"the parser is idempotent on an already-valid stem: it only bites when the stem is too long", ""},
	{"this only rejects the unrecognised rather than translating anything", ""},
	{"the runtime renders its own banner, so an imported hand banner would duplicate it", ""},
	{"content without such a banner is returned unchanged", ""},
	{"the substitution never adds or removes lines, so both sides always carry the same line count", ""},
	{"in a dry run nothing was mutated and the changed-file list is empty", ""},
	{"computed once so the two paths never compute it differently", ""},
	{"the amender must be numbered later, or the graph could cycle", ""},
	{"it does not use the general-purpose renderer; the implementation is written directly against the styling primitives", ""},
	{"tables are rendered as literal text", ""},
	{"the description is a heading level or a list depth", ""},
	{"the mode reports whether the tool may run alongside others", ""},
	{"every replacement must match a unique, non-overlapping region of the original", ""},
	{"if two changes affect the same block, merge them into one", ""},
	{"the current working directory is used when the argument is empty", ""},
	{"exposed so a caller can iterate over it", ""},
	{"the empty event is the operator-driven move; every other event names the behavior whose cascade writes a status", ""},
}

// classOf names the class a verdict's reason belongs to, or "" for the zero
// verdict. One reason string belongs to exactly one class, so the reverse
// lookup is total.
func classOf(reason string) string {
	if reason == "" {
		return ""
	}
	for _, c := range classes {
		if c.reason == reason {
			return c.name
		}
	}
	return "unknown"
}

// contractFloor is the share of contract prose the tool must leave alone.
//
// Measured on the 75 contract rows held out: 75 of 75 above a declaration, 75
// of 75 at the lower threshold a comment inside a function body meets, and 25
// of 25 when those rows are read three to a comment, which is the unit a real
// doc comment arrives in. The floor is one miss below all three, so a change
// costing a single false positive on prose this tool has never seen fails here
// rather than in somebody's editor.
//
// Only this direction is asserted. Everything else the test measures is
// logged, because the recall a re-tune should aim for is a judgement nobody has
// made yet.
const contractFloor = 0.986

func TestHeldOut(t *testing.T) {
	skipWithoutRuntime(t)

	// Half of this table was moved into mined.go to fit the thresholds, and
	// those rows are no longer held out. Filtering here rather than deleting
	// them keeps one copy of the labelling: move a row into mined.go and it
	// leaves the evaluation set by that fact alone.
	inCorpus := map[string]bool{}
	for _, texts := range mined {
		for _, text := range texts {
			inCorpus[text] = true
		}
	}
	live := map[string]bool{"": true}
	for _, c := range classes {
		live[c.name] = true
	}
	var heldout []struct{ text, class string }
	retired := 0
	for _, r := range labelled {
		switch {
		case inCorpus[r.text]:
		case !live[r.class]:
			// Labelled for a class the tool no longer has. The rows stay in the
			// table because the labelling is what a future attempt at that
			// class would be measured against.
			retired++
		default:
			heldout = append(heldout, r)
		}
	}
	t.Logf("%d rows labelled for a class this build does not carry", retired)

	comments := make([][]string, len(heldout))
	for i, r := range heldout {
		comments[i] = split(r.text)
	}
	reasons := judge(comments, make([]float64, len(heldout)))

	names := []string{""}
	for _, c := range classes {
		names = append(names, c.name)
	}
	index := map[string]int{}
	for i, n := range names {
		index[n] = i
	}

	confusion := make([][]int, len(names))
	for i := range confusion {
		confusion[i] = make([]int, len(names))
	}
	// The verdict says which side of the floor a comment fell on; the scores
	// say by how much, which is the difference between a floor set too high
	// and an exemplar set that does not reach the wording at all.
	texts := make([]string, len(heldout))
	for i, r := range heldout {
		texts[i] = r.text
	}
	vectors, err := embedAll(texts)
	if err != nil {
		t.Fatal(err)
	}

	type miss struct {
		text, want, got        string
		score, floor, contract float64
	}
	var misses []miss
	for i, r := range heldout {
		got := classOf(reasons[i].reason)
		w, ok := index[r.class]
		if !ok {
			t.Fatalf("row %d carries an unknown label %q", i, r.class)
		}
		g, ok := index[got]
		if !ok {
			t.Fatalf("%q was judged %q, which is not a class", r.text, got)
		}
		confusion[w][g]++
		if got != r.class {
			// Score the label's own class where there is one, otherwise the
			// class that fired: both answer "how close was this call".
			named := r.class
			if named == "" {
				named = got
			}
			m := miss{text: r.text, want: r.class, got: got}
			for ci, c := range classes {
				if c.name == named {
					m.score = dot(vectors[i], fitted.directions[ci])
					m.floor = fitted.thresholds[ci]
				}
			}
			misses = append(misses, m)
		}
	}

	t.Logf("held-out set: %d comments from repositories that took no part in tuning", len(heldout))
	t.Logf("%-11s %8s %8s %8s %8s %8s", "class", "labelled", "fired", "correct", "prec", "recall")
	for wi, want := range names {
		labelled, correct := 0, confusion[wi][wi]
		for _, n := range confusion[wi] {
			labelled += n
		}
		fired := 0
		for gi := range names {
			fired += confusion[gi][wi]
		}
		name := want
		if name == "" {
			name = "(contract)"
		}
		t.Logf("%-11s %8d %8d %8d %8s %8s", name, labelled, fired, correct,
			ratio(correct, fired), ratio(correct, labelled))
	}

	t.Log("confusion (row: label, column: verdict)")
	header := fmt.Sprintf("%-11s", "")
	for _, n := range names {
		if n == "" {
			n = "-"
		}
		header += fmt.Sprintf("%11s", n)
	}
	t.Log(header)
	for wi, want := range names {
		if want == "" {
			want = "(contract)"
		}
		row := fmt.Sprintf("%-11s", want)
		for gi := range names {
			row += fmt.Sprintf("%11d", confusion[wi][gi])
		}
		t.Log(row)
	}

	// Worst first: a false positive that cleared the floor by a wide margin,
	// or a positive that fell short of it by one, is what a re-tune has to
	// answer for. distance is signed the same way in both directions.
	sort.Slice(misses, func(i, j int) bool {
		return distance(misses[i].score, misses[i].floor, misses[i].contract, misses[i].want) >
			distance(misses[j].score, misses[j].floor, misses[j].contract, misses[j].want)
	})
	for _, m := range misses {
		want, got := m.want, m.got
		if want == "" {
			want = "(contract)"
		}
		if got == "" {
			got = "(contract)"
		}
		t.Logf("miss  want %-10s got %-10s score %+.3f short of %.3f by %.3f  %s",
			want, got, m.score, m.floor, m.floor-m.score, m.text)
	}

	// Only the false-positive rate is asserted: prose wrongly nudged costs an
	// edit against documentation that was already right, and a missed comment
	// costs a nudge nobody sees.
	//
	// It is asserted three times, because most comments do not take the path
	// the first pass measures. A comment inside a function body is judged at a
	// lower threshold, and a real doc comment is several sentences rather than
	// the one a labelled row holds.
	buried := make([]float64, len(heldout))
	for i := range buried {
		buried[i] = allowance(buriedBias, len(comments[i]))
	}
	check := func(name string, left, contract int) {
		rate := float64(left) / float64(contract)
		t.Logf("%-26s contract prose left alone %d/%d = %.3f", name, left, contract, rate)
		if rate < contractFloor {
			t.Errorf("%s: contract prose left alone %d/%d = %.3f, want at least %.3f",
				name, left, contract, rate, contractFloor)
		}
	}
	for _, pass := range []struct {
		name    string
		verdict []verdict
	}{
		{"above a declaration", reasons},
		{"inside a function body", judge(comments, buried)},
	} {
		contract, left := 0, 0
		for i, r := range heldout {
			if r.class != "" {
				continue
			}
			contract++
			if pass.verdict[i].reason == "" {
				left++
			}
		}
		check(pass.name, left, contract)
	}

	// A row is one sentence and a doc comment is several, each of which gets
	// its own draw against the same threshold, so the rate above is measured
	// in a unit the hook never sees. Reading the contract rows three to a
	// comment is what the tool actually faces.
	var sentences []string
	for _, r := range heldout {
		if r.class == "" {
			sentences = append(sentences, r.text)
		}
	}
	var docs [][]string
	for at := 0; at < len(sentences); at += 3 {
		docs = append(docs, sentences[at:min(at+3, len(sentences))])
	}
	bias := make([]float64, len(docs))
	for i := range bias {
		bias[i] = allowance(buriedBias, len(docs[i]))
	}
	left := 0
	for _, v := range judge(docs, bias) {
		if v.reason == "" {
			left++
		}
	}
	check("three to a comment", left, len(docs))
}

// distance is how badly a miss missed, along the class direction and in the
// units its threshold is set in. A false positive is ranked by how far past
// the threshold it went, a missed positive by how far short it fell.
func distance(score, floor, contract float64, want string) float64 {
	if want == "" {
		return score - floor
	}
	return floor - score
}

// ratio renders a rate, or a dash where the denominator is zero.
func ratio(part, whole int) string {
	if whole == 0 {
		return "-"
	}
	return fmt.Sprintf("%.3f", float64(part)/float64(whole))
}
