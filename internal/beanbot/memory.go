package beanbot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// compactionTimeout bounds the model call that rewrites an oversized Memory,
// so a hung call cannot leave a goroutine parked for the life of the process.
const compactionTimeout = 60 * time.Second

// Compactor shrinks an oversized Memory. *gemini.Prompter satisfies it.
type Compactor interface {
	Compact(ctx context.Context, document string) (string, error)
}

// Memory is BeanBot's long-running record of what it knows about each Guild,
// one markdown file per Guild on a mounted volume.
//
// A nil *Memory is the feature switched off: reads are empty, writes are
// refused, Compaction does nothing. That is what BEANBOT_MEMORY_DIR being unset
// produces, so the rest of BeanBot needs no special case.
type Memory struct {
	dir string
	// limit caps the topical notes, which Compaction rewrites smaller. It does
	// not cap the Roster: nothing there is ever discarded, and it is affordable
	// because only the People a conversation touches are read back.
	limit int
	// budget caps how much Roster one Trigger carries. The Roster itself has no
	// ceiling, so this is the only thing between a server that has discussed
	// four hundred people and a four-hundred-person prompt.
	budget    int
	compactor Compactor

	// locks serialises read-modify-write per Guild. Deliberately per-Guild
	// rather than the single global mutex ADR 0001 removed.
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// OpenMemory prepares the Memory directory. An empty dir disables the feature;
// a dir that is configured but unusable is fatal to the caller.
//
// It deliberately does not create the directory. On Fly the mount creates it,
// and a MkdirAll here would succeed against the container's ephemeral layer
// when the volume failed to attach — Memory that looks perfect until the next
// deploy silently empties it.
func OpenMemory(dir string, limit, budget int, compactor Compactor) (*Memory, error) {
	if dir == "" {
		return nil, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("memory directory %s is not there — is the volume mounted?: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("memory directory %s is not a directory", dir)
	}

	// Statting proves it exists, not that BeanBot may write to it, and the
	// first failed write would otherwise be hours after startup.
	probe, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return nil, fmt.Errorf("memory directory %s is not writable: %w", dir, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return nil, err
	}
	if err := os.Remove(name); err != nil {
		return nil, err
	}

	return &Memory{
		dir:       dir,
		limit:     limit,
		budget:    budget,
		compactor: compactor,
		locks:     map[string]*sync.Mutex{},
	}, nil
}

// Load returns the Guild's Memory, or empty if there is none. A Memory that
// cannot be read is logged and treated as absent: a degraded conversation is
// better than a dead one, which is how a missing Backlog is handled too.
func (m *Memory) Load(guildID string) string {
	if m == nil || guildID == "" {
		return ""
	}

	path, err := m.path(guildID)
	if err != nil {
		log.Printf("loading memory: %v", err)
		return ""
	}
	return m.read(path)
}

// Record applies one change to the Guild's Memory. The read, the merge and the
// write happen under the Guild's lock against the file as it is now — not
// against the snapshot the model was shown — so a concurrent Trigger's entry
// cannot be overwritten by a stale base.
func (m *Memory) Record(guildID string, ch change) error {
	return m.rewrite(guildID, func(current string) (string, error) {
		return applyChange(current, ch)
	})
}

// RecordPerson applies one change to the Guild's Roster, under the same lock
// and against the same live file as Record.
func (m *Memory) RecordPerson(guildID string, ch personChange) error {
	return m.rewrite(guildID, func(current string) (string, error) {
		return applyPersonChange(current, ch)
	})
}

// Merge folds one Person in the Guild's Roster into another.
func (m *Memory) Merge(guildID, from, into string) error {
	return m.rewrite(guildID, func(current string) (string, error) {
		return applyMerge(current, from, into)
	})
}

// rewrite reads, edits and writes the Guild's Memory under its lock, against
// the file as it is now rather than a snapshot the model was shown seconds ago.
func (m *Memory) rewrite(guildID string, edit func(string) (string, error)) error {
	if m == nil {
		return errors.New("i have nowhere to write things down right now")
	}

	path, err := m.path(guildID)
	if err != nil {
		return err
	}

	unlock := m.lock(guildID)
	defer unlock()

	updated, err := edit(m.read(path))
	if err != nil {
		return err
	}
	return writeFileAtomic(path, updated)
}

// Recall is the Memory as it reaches one Trigger: the topical notes whole, the
// People this conversation touches in full, and everybody else by name only.
func (m *Memory) Recall(guildID, backlog string, speakers []namedUser) string {
	if m == nil {
		return ""
	}
	return recall(parseMemory(m.Load(guildID)), backlog, speakers, m.budget)
}

// CompactIfNeeded rewrites the Guild's topical notes smaller once they outgrow
// the limit. It runs detached from the Trigger that provoked it, so nobody waits.
//
// The Roster is deliberately not part of this. Compaction is a model rewrite of
// the only copy that exists, and "make this smaller" pointed at a list of humans
// means merging two thin people into one or dropping whoever has not come up in
// months. Nothing in the Roster is ever forgotten, so nothing in it is ever
// shown to the compactor — which is affordable only because the Roster is no
// longer read back whole on every Trigger.
//
// The model call deliberately happens *outside* the Guild's lock. Holding the
// lock across a call that may take a minute would stall any member who asked
// BeanBot to remember something in the meantime — which is the wait this was
// moved off the reply path to avoid. The cost is that the document may change
// while the model is thinking, so the write re-checks and abandons a compaction
// that has gone stale rather than overwriting an entry recorded since.
func (m *Memory) CompactIfNeeded(ctx context.Context, guildID string) error {
	if m == nil || m.compactor == nil || guildID == "" {
		return nil
	}

	path, err := m.path(guildID)
	if err != nil {
		return err
	}

	before := parseMemory(m.readUnderLock(guildID, path)).topical()
	if len(before) <= m.limit {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, compactionTimeout)
	defer cancel()

	compacted, err := m.compactor.Compact(ctx, before)
	if err != nil {
		return fmt.Errorf("compacting memory for guild %s: %w", guildID, err)
	}

	// Compaction overwrites the only copy that exists. A model that returned
	// nothing, or returned more than it was given, has not compacted anything —
	// keeping an oversized Memory is far cheaper than losing it.
	if strings.TrimSpace(compacted) == "" {
		return fmt.Errorf("compaction of guild %s returned nothing; keeping %d bytes", guildID, len(before))
	}
	if len(compacted) >= len(before) {
		return fmt.Errorf("compaction of guild %s grew %d bytes to %d; keeping the original",
			guildID, len(before), len(compacted))
	}

	// Round-tripped through the parser so whatever shape the model chose is
	// normalised before it becomes the document every later change merges into.
	normalised := parseMemory(compacted)

	// The compactor was shown no People at all, so anything it filed under that
	// heading is either invented or a topical note it decided to re-file. Only
	// the topical part of its answer is kept, so either way that text would be
	// dropped — and dropping a recorded fact is the one thing this must not do.
	// Refusing leaves the notes oversized, which the next Trigger retries.
	if len(normalised.roster.people) > 0 || len(normalised.roster.orphans) > 0 {
		return fmt.Errorf("compaction of guild %s wrote under %q, which is not its to write; keeping the original",
			guildID, rosterHeading)
	}

	unlock := m.lock(guildID)
	defer unlock()

	// Re-read and compare the *topical* notes alone. Somebody recording a note
	// about a person while the model was working has touched nothing this
	// rewrite covers, and abandoning for that would leave any server busy enough
	// to need compacting permanently oversized.
	current := parseMemory(m.read(path))
	if current.topical() != before {
		// Somebody edited what is being rewritten. Writing now would silently
		// drop it; the next Trigger will find the notes still oversized and try
		// again.
		log.Printf("abandoning compaction of guild %s: it changed while the model was working", guildID)
		return nil
	}

	current.preamble, current.sections = normalised.preamble, normalised.sections

	log.Printf("compacted memory for guild %s: %d bytes to %d", guildID, len(before), len(current.topical()))
	return writeFileAtomic(path, current.render())
}

func (m *Memory) readUnderLock(guildID, path string) string {
	unlock := m.lock(guildID)
	defer unlock()
	return m.read(path)
}

func (m *Memory) read(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("reading %s: %v", path, err)
		}
		return ""
	}
	return string(data)
}

// path names the Guild's file. The ID becomes a filename, and Discord only ever
// sends snowflakes, so anything else is a bug or an attempt to escape the
// directory — either way it is refused rather than sanitised.
func (m *Memory) path(guildID string) (string, error) {
	if guildID == "" {
		return "", errors.New("i can only remember things inside a server")
	}
	for _, r := range guildID {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("%q is not a valid server id", guildID)
		}
	}
	return filepath.Join(m.dir, guildID+".md"), nil
}

func (m *Memory) lock(guildID string) func() {
	m.mu.Lock()
	mu, ok := m.locks[guildID]
	if !ok {
		mu = &sync.Mutex{}
		m.locks[guildID] = mu
	}
	m.mu.Unlock()

	mu.Lock()
	return mu.Unlock
}

// writeFileAtomic replaces a file by renaming a complete one over it, so a
// crash or a full disk mid-write cannot leave a Memory half-rewritten.
func writeFileAtomic(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".memory-*")
	if err != nil {
		return err
	}
	name := tmp.Name()

	// Best-effort cleanup: after a successful rename there is nothing at name
	// and the removal fails harmlessly.
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(name)
	}()

	if _, err := tmp.WriteString(content); err != nil {
		return err
	}
	// The rename is atomic, but only over bytes the kernel has actually been
	// given — without this a crash can rename an empty file into place.
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file 0600; the Memory is not a secret and being able
	// to read it over `fly ssh` without sudo is the point of a markdown file.
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}

	return os.Rename(name, path)
}
