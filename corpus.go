package main

// The exemplars are the specification of taste. The four classes were distilled
// from comments an agent wrote into working repositories, preferring the ones a
// human deleted within weeks; the contract set was distilled from the Go
// standard library, the DOOM sources, and Django, whose comments have aged
// well, and from the infrastructure register the classes kept misreading.
//
// Sets are kept near the same size on purpose: the score is the mean of a
// vector's two closest exemplars, which grows with the size of the set it is
// measured against, so an unbalanced class wins by cardinality alone. The
// contract set is the exception, being the one every class is measured against.

var classes = []class{
	{
		name:   "history",
		reason: "reads as change-event explanation: it belongs in the commit message, which ships beside the diff",
		floor:  0.15,
		exemplars: []string{
			"we now use a pooled client instead of dialing per request",
			"this previously caused the liveness probe to time out",
			"it used to allocate a buffer on every call",
			"previously this pointed at the upstream mirror",
			"the retry logic was temporarily disabled because it blocked",
			"for now we only test that the basic functionality is preserved",
			"the module was renamed, so update the callers",
			"previously created by the deployment unit that is now dormant",
			"the rule this used to write is superseded by the ones above",
			"retained for now until the follow-up lands",
			"accepted temporarily while both versions run side by side",
			"bumped the timeout because the build server is slower than a laptop",
			"removed the cache, the store is fast enough now",
			"this fixes the panic reported after the last release",
			"switched from a map to a slice in this refactor",
			"the value was fifty milliseconds before the load test",
			"changed from a map because ordering matters",
			"this used to be handled by the old parser",
			"added to fix the flaky test on the build server",
			"moved here from the utility package",
			"reverted the change from last week",
		},
	},
	{
		name:   "compat",
		reason: "justifies the symbol by its own history: state the contract instead, as a Deprecated: note if that is what it is",
		floor:  0.22,
		exemplars: []string{
			"kept for backwards compatibility with older callers",
			"this name is kept for backwards compatibility with older clients",
			"legacy field, do not use it in new code",
			"the old environment variable is still accepted for compatibility",
			"left in place so the rollout can be reverted in one step",
			"this alias exists only for clients that have not migrated",
			"retained for the previous configuration format",
			"still referenced by the deprecated entry point",
			"left in place so that old callers keep working",
			"this only exists for the migration",
			"not used any more, left in for safety",
			"old name, kept so that nothing breaks",
			"here for compatibility with the first version of the client",
		},
	},
	{
		name:   "tautology",
		reason: "restates what the code already says: the line below is the documentation",
		floor:  0.30,
		exemplars: []string{
			"increment the counter",
			"close the connection",
			"create the client",
			"build the result",
			"parse the response body",
			"check for a nil error",
			"set up the mock",
			"getting the main parameters",
			"loop over the items",
			"return the result",
			"set the flag to true",
			"save it to the database",
			"handle the error",
			"start the server",
			"append it to the slice",
			"multiply it by two",
			"wait for the goroutine to finish",
			"open the connection",
			"send the message",
			"read the file",
		},
	},
	{
		name:   "narrative",
		reason: "narrates how the code works: a doc states the contract, and the walkthrough belongs in a test",
		floor:  0.20,
		exemplars: []string{
			"this wrapper does two things: first it flattens, then it formats",
			"traverses the message by reflection and substitutes every placeholder",
			"the handler receives the event, computes a time, and writes it",
			"step one observes the source, step two extracts the path",
			"the loop reads each entry, filters it, and appends to the slice",
			"first we acquire the lock, then we drain the channel",
			"first we parse the input, then we walk the tree and collect the results",
			"this works by iterating over every element and applying the transform",
			"here we build the request, send it, and then handle the response",
			"we start by allocating the buffer, after which each chunk is copied in",
			"we check the cache, and if it misses we go to the database",
			"what happens here is that the client opens a connection and keeps it alive",
			"the algorithm walks the graph depth first, marking each node as it goes",
			"this function first validates the input and then writes it to disk",
		},
	},
}

// contract is what every class is measured against: prose that states what a
// symbol or a setting promises, which is the shape slopguard must leave alone.
var contract = []string{
	// The contract of a symbol, in the register of the Go standard library.
	"reports whether the element at i must sort before the element at j",
	"the zero value is ready to use and must not be copied after first use",
	"it panics if the count is negative or the result would overflow",
	"implementations must not retain the buffer passed to this method",
	"the behavior of close after the first call is undefined",
	"callers should process the returned bytes before considering the error",
	"makes one call to len and order n log n calls to less and swap",
	"the returned slice is valid only until the next call on this reader",
	"the sort is not guaranteed to be stable",
	"makes no guarantee about the order in which the callback is invoked",
	"the duration must be greater than zero, otherwise this panics",
	"a value stored here may be dropped automatically at any time",
	"safe for concurrent use by multiple goroutines",
	"keys must be comparable, since they are used as map keys",
	"stop does not close the channel, so readers see no spurious final value",
	"the returned cancel function must be called on every path or the child leaks",
	"returns nil when there is nothing to do",
	"blocks until the context is cancelled",
	"only the first match is returned",
	"returns the number of bytes written",
	"reports whether the path exists",

	// A constraint the caller or the next editor must hold, in the terse
	// register of the DOOM sources.
	"the caller must compute the distance first; this reads it and does not set it",
	"purely informative: nothing is modified except things picked up",
	"the box is widened by the maximum radius because objects are bucketed by origin",
	"do not set the state here, the action routines cannot run yet",
	"these are not sorted, so two adjacent ones may be crossed in either order",
	"twenty adjoining sectors is the ceiling; past that the list is truncated",
	"do not change anything here, a refresh may be in progress",
	"bail out at the end of the array rather than overflow",
	"if this flag is set, the move is legal within the recorded bounds",

	// The register of a Python docstring.
	"must be implemented by subclasses to initialize the wrapped object",
	"returns a tuple of the object and whether it was created",
	"the matched values must be hashable; they are used as dictionary keys",
	"locks the row so a concurrent update blocks until this save completes",
	"raises an error if the expression cannot be used in a where clause",
	"converts a single-argument method into a property cached on the instance",
	"a primary key cannot be null, so one member decides whether the row exists",
	"an unchecked box is a valid value, so the form field is not required",
	"virtual fields cannot be deferred and are ignored rather than rejected",
	"backfills the column for rows written before the constraint existed",
	"the reverse migration is a no-op: the data cannot be reconstructed",
	"this task is idempotent, because the queue may deliver it twice",
	"the field is nullable because the import leaves it empty",
	"an explicit ordering is required for pagination to be stable",

	// History that is contract: a wire format, a compatibility guarantee, or a
	// name that must keep existing. These are the hardest negatives, and every
	// one of them is drawn from a doc comment that has aged well.
	"the stop method is no longer necessary to help the garbage collector",
	"this is no longer returned by anything in this package, so do not compare against it",
	"kept for binary compatibility and exported only for the type descriptors",
	"the version was capped for compatibility, and an extension negotiates it instead",
	"the growth rate has always been doubling, which callers rely on",
	"this input is tolerated rather than rejected, because old call sites pass it",
	"returns every durable the consumer no longer runs",
	"names the single consumer used before the split, which still holds messages",
	"the label domain carried by nodes the older modules built",
	"falls back to the legacy identifier when the current one is absent",
	"this is a historical artifact whose shape the wire format freezes",

	// The register of infrastructure configuration.
	"the number of pods to run",
	"raising this above four needs a node pool change",
	"three replicas because the disruption budget requires two available",
	"do not raise this above the node's memory, which runs two pods",
	"ingress is disabled by default",
	"image pull secrets for the private registry",
	"default values for this chart",
	"cpu limits apply per container",
	"the node selector for the worker pods",
	"set to true to enable the sidecar",
	"must be a valid DNS name",
	"the maximum number of retries before giving up",
	"empty means every namespace",
	"resources requested by each pod",
	"the service account the pods run as",
	"the storage class used for the volume claim",
	"managed by terraform, and overwritten on the next apply",
	"generated by the build, so edits here are lost",
	"kept in sync with the values in the terraform module",
	"apply this before the deployment: the pods mount it at startup",
	"see the runbook before changing this",
	"the value must match the variables in the infrastructure repository",
	"values are in seconds",

	// A constraint enforced somewhere the reader cannot see, which is what
	// earns a line inside a function body.
	"these fields are enforced by the operator and stay unset here",
	"matches the source and destination pair in the terraform module",
	"the server requires the target to live in the same stream",
	"the two trailing wildcards cover the partition suffix",
	"this must stay false: fact gathering runs before the connection is up",
	"the caller has already bounded this, so it cannot overflow",
	"order is significant, callers index by position",
	"a command that does not parse is allowed",
	"generic over the enum type: both packages share the value names",
	"renders the enum as a short lowercase string for human-facing output",
}
