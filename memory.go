package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A comment already named once is not named again in the same session.
//
// The agent's answer to a nudge is an edit, that edit re-enters the hook, and
// the rewritten comment is inside the new text. Without a memory the same line
// can be nudged until the agent finds the one move that reliably ends it, which
// is deleting the comment — the behaviour the nudge's own wording argues
// against. What is remembered is the comment's prose rather than its line, so
// an edit above it does not make it a new site.
const (
	// memoryEnv points the memory somewhere else, and empty turns it off.
	memoryEnv = "SLOPGUARD_STATE"
	// stale is how long a session's memory outlives its last write.
	stale = 24 * time.Hour
	// remembered bounds one session's file. A session that nudges more than
	// this has bigger problems than repeating itself.
	remembered = 4096
)

// spoken returns the keys this session has already been nudged about for path,
// and a function that records the ones about to be named. It fails open in
// both directions: an unreadable store nudges as if nothing were remembered,
// and an unwritable one forgets.
func spoken(session, path string) (map[uint64]bool, func([]finding)) {
	seen := map[uint64]bool{}
	file := store(session)
	if file == "" {
		return seen, func([]finding) {}
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
	return seen, func(findings []finding) {
		if len(findings) == 0 || lines > remembered {
			return
		}
		handle, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		defer handle.Close()
		for _, f := range findings {
			handle.WriteString(strconv.FormatUint(prefix, 36) + " " + strconv.FormatUint(f.key, 36) + "\n")
		}
	}
}

// store names this session's memory file, creating the directory and sweeping
// out the sessions that have gone quiet. It returns "" when there is nowhere to
// keep one, or when the memory is switched off.
func store(session string) string {
	dir, ok := os.LookupEnv(memoryEnv)
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
	if session == "" {
		session = "unnamed"
	}
	return filepath.Join(dir, strconv.FormatUint(site(session), 36))
}

// sweep removes the memories of sessions that have not written in a day.
func sweep(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < stale {
			continue
		}
		os.Remove(filepath.Join(dir, entry.Name()))
	}
}
