package beanbot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubCompactor stands in for the model when Compaction is exercised.
type stubCompactor struct {
	mu    sync.Mutex
	calls int
	given string
	out   string
	err   error
}

func (s *stubCompactor) Compact(_ context.Context, document string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.given = document
	return s.out, s.err
}

func (s *stubCompactor) sawDocument() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.given
}

func (s *stubCompactor) called() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func openTestMemory(t *testing.T, limit int, c Compactor) *Memory {
	t.Helper()

	m, err := OpenMemory(t.TempDir(), limit, 4<<10, c)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMemoryIsDisabledWhenNoDirectoryIsConfigured(t *testing.T) {
	m, err := OpenMemory("", 8<<10, 4<<10, nil)
	if err != nil {
		t.Fatalf("an unset directory means the feature is off, not broken: %v", err)
	}
	if m != nil {
		t.Error("no directory should yield no Memory")
	}
}

func TestAMissingDirectoryIsRefusedRatherThanCreated(t *testing.T) {
	// The mount creates the directory. Creating it here would let a Fly volume
	// that failed to attach look healthy while writing to the container's
	// ephemeral layer — memory that works perfectly until the next deploy.
	dir := filepath.Join(t.TempDir(), "not-mounted")

	if _, err := OpenMemory(dir, 8<<10, 4<<10, nil); err == nil {
		t.Fatal("a missing memory directory should be an error")
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Error("the directory should not have been created")
	}
}

func TestAFileWhereTheDirectoryShouldBeIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory")
	if err := os.WriteFile(path, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := OpenMemory(path, 8<<10, 4<<10, nil); err == nil {
		t.Error("a regular file is not a usable memory directory")
	}
}

func TestAnUnwritableDirectoryIsRefusedAtStartup(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, err := OpenMemory(dir, 8<<10, 4<<10, nil); err == nil {
		t.Error("a read-only memory directory should be an error")
	}
}

func TestWhatIsRecordedIsWhatIsLoaded(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)

	if err := m.Record("123", change{Section: "Traditions", Claim: "Thursday is game night."}); err != nil {
		t.Fatal(err)
	}

	if got := m.Load("123"); !strings.Contains(got, "Thursday is game night.") {
		t.Errorf("got %q", got)
	}
}

func TestAGuildWithNoMemoryYetReadsAsEmpty(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)

	if got := m.Load("123"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestOneGuildCannotReadAnothersMemory(t *testing.T) {
	m := openTestMemory(t, 8<<10, nil)

	if err := m.Record("111", change{Section: "Traditions", Claim: "Thursday is game night."}); err != nil {
		t.Fatal(err)
	}

	if got := m.Load("222"); got != "" {
		t.Errorf("memory bled between guilds: %q", got)
	}
}

func TestAGuildIDThatIsNotASnowflakeIsRefused(t *testing.T) {
	// The guild ID becomes a filename. Discord only ever sends digits, so
	// anything else is either a bug or an attempt to escape the directory.
	m := openTestMemory(t, 8<<10, nil)

	for _, id := range []string{"", "../etc/passwd", "12/34", "abc"} {
		if err := m.Record(id, change{Section: "Traditions", Claim: "x"}); err == nil {
			t.Errorf("guild ID %q should have been refused", id)
		}
		if got := m.Load(id); got != "" {
			t.Errorf("guild ID %q loaded %q", id, got)
		}
	}
}

func TestADisabledMemoryReadsEmptyAndRefusesWrites(t *testing.T) {
	var m *Memory

	if got := m.Load("123"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
	if err := m.Record("123", change{Section: "Traditions", Claim: "x"}); err == nil {
		t.Error("a disabled Memory should refuse to record")
	}
	if err := m.CompactIfNeeded(context.Background(), "123"); err != nil {
		t.Errorf("compacting a disabled Memory should be a no-op, got %v", err)
	}
}

func TestConcurrentRecordsAllSurvive(t *testing.T) {
	// Every Trigger runs in its own goroutine, so two members teaching BeanBot
	// something at once is ordinary. Read-modify-write without a lock loses one.
	m := openTestMemory(t, 1<<20, nil)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.Record("123", change{
				Section: "Traditions",
				Claim:   fmt.Sprintf("fact number %d", i),
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	got := m.Load("123")
	for i := range 20 {
		if !strings.Contains(got, fmt.Sprintf("fact number %d", i)) {
			t.Errorf("entry %d was lost:\n%s", i, got)
		}
	}
}

func TestASmallMemoryIsLeftAlone(t *testing.T) {
	compactor := &stubCompactor{out: "## Traditions\n- squashed\n"}
	m := openTestMemory(t, 8<<10, compactor)

	if err := m.Record("123", change{Section: "Traditions", Claim: "Thursday is game night."}); err != nil {
		t.Fatal(err)
	}
	if err := m.CompactIfNeeded(context.Background(), "123"); err != nil {
		t.Fatal(err)
	}

	if compactor.called() != 0 {
		t.Error("a memory under the limit should not be compacted")
	}
}

func TestAnOversizedMemoryIsReplacedByItsCompaction(t *testing.T) {
	compactor := &stubCompactor{out: "## Traditions\n- Everyone turns up eventually.\n"}
	m := openTestMemory(t, 200, compactor)

	for i := range 20 {
		if err := m.Record("123", change{
			Section: "Traditions",
			Claim:   fmt.Sprintf("a reasonably wordy fact number %d about somebody", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := m.CompactIfNeeded(context.Background(), "123"); err != nil {
		t.Fatal(err)
	}

	if compactor.called() != 1 {
		t.Fatalf("compaction ran %d times, want 1", compactor.called())
	}
	if got := m.Load("123"); got != compactor.out {
		t.Errorf("got %q, want %q", got, compactor.out)
	}
}

func TestAFailedCompactionLeavesTheMemoryIntact(t *testing.T) {
	compactor := &stubCompactor{err: errors.New("the model is having a day")}
	m := openTestMemory(t, 50, compactor)

	if err := m.Record("123", change{Section: "Traditions", Claim: "Thursday is game night, always has been."}); err != nil {
		t.Fatal(err)
	}
	before := m.Load("123")

	if err := m.CompactIfNeeded(context.Background(), "123"); err == nil {
		t.Error("a failed compaction should report the error")
	}

	if got := m.Load("123"); got != before {
		t.Errorf("the memory changed despite a failed compaction:\ngot  %q\nwant %q", got, before)
	}
}

// compactionReturning runs Compaction over an oversized memory with the model
// stubbed to return out, checking the memory survived and reporting the error.
func compactionReturning(t *testing.T, out string) error {
	t.Helper()

	m := openTestMemory(t, 50, &stubCompactor{out: out})
	if err := m.Record("123", change{Section: "Traditions", Claim: "Thursday is game night, always has been."}); err != nil {
		t.Fatal(err)
	}
	before := m.Load("123")

	err := m.CompactIfNeeded(context.Background(), "123")
	if got := m.Load("123"); got != before {
		t.Errorf("the memory was damaged: %q", got)
	}
	return err
}

func TestACompactionThatReturnsNothingIsRejected(t *testing.T) {
	// Compaction overwrites the only copy that exists, so an empty result is
	// the difference between an oversized Memory and no Memory at all.
	for _, out := range []string{"", "   \n"} {
		if err := compactionReturning(t, out); err == nil {
			t.Errorf("compaction returning %q should be rejected", out)
		}
	}
}

func TestACompactionThatWritesUnderPeopleIsRejected(t *testing.T) {
	// The compactor is never shown the Roster, so anything it files there is
	// invented or a topical note it re-filed — and only its topical answer is
	// kept, so either way that text would vanish. Refusing leaves the notes
	// oversized, which the next Trigger retries.
	if err := compactionReturning(t, "## T\n- a\n\n## People\n- x\n"); err == nil {
		t.Error("a compaction that wrote under People should be rejected")
	}
}

func TestACompactionThatGrewTheMemoryIsRejected(t *testing.T) {
	if err := compactionReturning(t, strings.Repeat("longer than the original. ", 50)); err == nil {
		t.Error("a compaction larger than its input has compacted nothing")
	}
}

// gatedCompactor blocks inside Compact until released, so a test can observe
// what the rest of the Memory can do while the model is thinking.
type gatedCompactor struct {
	entered chan struct{}
	release chan struct{}
	out     string
}

func (g *gatedCompactor) Compact(context.Context, string) (string, error) {
	close(g.entered)
	<-g.release
	return g.out, nil
}

func TestCompactionDoesNotBlockSomeoneRecordingSomething(t *testing.T) {
	// Compaction is a model call that can take a minute. Holding the Guild's
	// lock across it would stall any member who asked BeanBot to remember
	// something meanwhile — the very wait Compaction was detached to avoid.
	compactor := &gatedCompactor{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		out:     "## Traditions\n- Everyone turns up eventually.\n",
	}
	m := openTestMemory(t, 20, compactor)

	if err := m.Record("123", change{Section: "Traditions", Claim: "a reasonably wordy fact about somebody or other"}); err != nil {
		t.Fatal(err)
	}

	compaction := make(chan error, 1)
	go func() { compaction <- m.CompactIfNeeded(context.Background(), "123") }()
	<-compactor.entered

	recorded := make(chan error, 1)
	go func() { recorded <- m.Record("123", change{Section: "Traditions", Claim: "Sunday is a roast."}) }()

	select {
	case err := <-recorded:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recording blocked while the model was compacting")
	}

	close(compactor.release)
	if err := <-compaction; err != nil {
		t.Fatal(err)
	}

	// The compaction was computed from a document that no longer exists, so
	// writing it would drop what was recorded meanwhile.
	if got := m.Load("123"); !strings.Contains(got, "Sunday is a roast.") {
		t.Errorf("a stale compaction overwrote a concurrent write: %q", got)
	}
}

// crowdedRoster fills a Guild's Roster with people, which Compaction must never
// be allowed near.
func crowdedRoster(t *testing.T, m *Memory) {
	t.Helper()

	for i := range 20 {
		if err := m.RecordPerson("123", personChange{
			Name:  fmt.Sprintf("Person Number %d", i),
			Claim: fmt.Sprintf("a reasonably wordy note about person number %d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheRosterIsNeverHandedToCompaction(t *testing.T) {
	// Compaction is a model rewrite of the only copy, and "make this smaller"
	// applied to a roster means merging two thin people or dropping whoever has
	// not come up lately. Nothing in the Roster is ever forgotten.
	compactor := &stubCompactor{out: "## Traditions\n- Everyone turns up eventually.\n"}
	m := openTestMemory(t, 200, compactor)

	crowdedRoster(t, m)
	for i := range 10 {
		if err := m.Record("123", change{
			Section: "Traditions",
			Claim:   fmt.Sprintf("a reasonably wordy tradition number %d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := m.CompactIfNeeded(context.Background(), "123"); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(compactor.sawDocument(), "Person Number") {
		t.Errorf("the roster was handed to the model: %q", compactor.sawDocument())
	}
	got := m.Load("123")
	for i := range 20 {
		if !strings.Contains(got, fmt.Sprintf("person number %d", i)) {
			t.Fatalf("compaction lost person %d:\n%s", i, got)
		}
	}
	if !strings.Contains(got, "Everyone turns up eventually.") {
		t.Errorf("the compacted topical notes were not written back: %q", got)
	}
}

func TestABigRosterDoesNotProvokeCompaction(t *testing.T) {
	// The limit caps the topical notes, which ride on every message whole. The
	// Roster does not ride on every message, so it does not count against them.
	compactor := &stubCompactor{out: "## Traditions\n- squashed\n"}
	m := openTestMemory(t, 400, compactor)

	crowdedRoster(t, m)

	if err := m.CompactIfNeeded(context.Background(), "123"); err != nil {
		t.Fatal(err)
	}
	if compactor.called() != 0 {
		t.Error("a roster over the limit should not provoke a compaction")
	}
}

func TestANoteAboutSomebodyDoesNotAbandonACompaction(t *testing.T) {
	// The staleness check exists to protect a concurrent write. A Roster write
	// touches nothing Compaction is rewriting, so abandoning for one would mean
	// an oversized Memory in any server busy enough to need compacting.
	compactor := &gatedCompactor{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		out:     "## Traditions\n- Everyone turns up eventually.\n",
	}
	m := openTestMemory(t, 20, compactor)

	if err := m.Record("123", change{Section: "Traditions", Claim: "a reasonably wordy tradition or other"}); err != nil {
		t.Fatal(err)
	}

	compaction := make(chan error, 1)
	go func() { compaction <- m.CompactIfNeeded(context.Background(), "123") }()
	<-compactor.entered

	if err := m.RecordPerson("123", personChange{Name: "Kate", Claim: "Restores arcade cabinets."}); err != nil {
		t.Fatal(err)
	}

	close(compactor.release)
	if err := <-compaction; err != nil {
		t.Fatal(err)
	}

	got := m.Load("123")
	if !strings.Contains(got, "Everyone turns up eventually.") {
		t.Errorf("the compaction was abandoned over an unrelated roster write: %q", got)
	}
	if !strings.Contains(got, "Restores arcade cabinets.") {
		t.Errorf("the concurrent note about somebody was overwritten: %q", got)
	}
}

func TestCompactionLeavesNoTemporaryFilesBehind(t *testing.T) {
	dir := t.TempDir()
	m, err := OpenMemory(dir, 8<<10, 4<<10, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Record("123", change{Section: "Traditions", Claim: "Thursday is game night."}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "123.md" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected only 123.md, got %v", names)
	}
}
