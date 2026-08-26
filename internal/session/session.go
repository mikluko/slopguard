// Package session remembers what one agent session has already been told, so
// that a comment named once is not named again.
//
// The agent's answer to a nudge is an edit, that edit re-enters the hook, and
// the rewritten comment is inside the new text. Without a memory the same line
// can be nudged until the agent finds the one move that reliably ends it, which
// is deleting the comment — the behaviour the nudge's own wording argues
// against.
//
// What it stores is opaque numbers against a path. What they identify is the
// caller's business, and keeping it that way is what lets this package be about
// a file on disk rather than about comments.
package session

import (
	"bufio"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mikluko/slopguard/internal/prose"
)

const (
	// MemoryEnv points the memory somewhere else, and empty turns it off. It is
	// exported because switching the memory off is a supported way to run the
	// tool, so the name is part of what this package promises.
	MemoryEnv = "SLOPGUARD_STATE"
	// stale is how long a session's memory outlives its last write.
	stale = 24 * time.Hour
	// remembered bounds one session's file. A session that nudges more than
	// this has bigger problems than repeating itself.
	remembered = 4096
)

// Spoken returns the keys this session has already been nudged about for path,
// whether it has been nudged about anything at all, and a function that records
// what is about to be named. It fails open in both directions: an unreadable
// store nudges as if nothing were remembered, and an unwritable one forgets.
//
// The key is the caller's: what a nudge is remembered by has to survive the
// edit that answers it, and only the caller knows what that means. Hashing the
// comment's prose rather than its line is what makes an edit above it the same
// site rather than a new one.
func Spoken(id, path string) (seen map[uint64]bool, spoken bool, record func(keys []uint64)) {
	seen = map[uint64]bool{}
	file := store(id)
	if file == "" {
		return seen, false, func([]uint64) {}
	}
	prefix := site(path)
	lines := 0
	if handle, err := os.Open(file); err == nil {
		scanner := bufio.NewScanner(handle)
		for scanner.Scan() {
			lines++
			at, key, ok := strings.Cut(scanner.Text(), " ")
			if !ok {
				continue
			}
			if n, err := strconv.ParseUint(at, 36, 64); err != nil || n != prefix {
				continue
			}
			if n, err := strconv.ParseUint(key, 36, 64); err == nil {
				seen[n] = true
			}
		}
		handle.Close()
	}
	return seen, lines > 0, func(keys []uint64) {
		if len(keys) == 0 || lines > remembered {
			return
		}
		handle, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		defer handle.Close()
		for _, key := range keys {
			handle.WriteString(strconv.FormatUint(prefix, 36) + " " + strconv.FormatUint(key, 36) + "\n")
		}
	}
}

// site identifies a file by its path, and a session by its id. The rules hash a
// comment's prose the same way to identify the comment; the two share nothing
// but the hash, so each package keeps its own five lines rather than exporting
// one for the other.
func site(text string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(prose.Normalize(text)))
	return h.Sum64()
}

// store names this session's memory file, creating the directory and sweeping
// out the sessions that have gone quiet. It returns "" when there is nowhere to
// keep one, or when the memory is switched off.
func store(id string) string {
	dir, ok := os.LookupEnv(MemoryEnv)
	if ok && dir == "" {
		return ""
	}
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(cache, "slopguard")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	sweep(dir)
	if id == "" {
		id = "unnamed"
	}
	return filepath.Join(dir, strconv.FormatUint(site(id), 36))
}

// sweep removes the memories of sessions that have not written in a day.
//
// It deletes only what this program writes: a regular file whose whole name is
// one base-36 number. SLOPGUARD_STATE is a path a person types, and a sweep
// that trusted the directory would empty whatever they pointed it at.
func sweep(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !ours(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < stale {
			continue
		}
		os.Remove(filepath.Join(dir, entry.Name()))
	}
}

// ours reports whether a name is one this program wrote.
func ours(name string) bool {
	if _, err := strconv.ParseUint(name, 36, 64); err != nil {
		return false
	}
	return true
}
