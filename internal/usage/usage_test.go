package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordAndRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local)
	tr := Open(path)
	tr.now = func() time.Time { return now }

	tr.Record(true, time.Second)    // awake and used
	tr.Record(true, 10*time.Minute) // awake, idle
	tr.Record(false, 0)             // asleep: ignored
	now = now.AddDate(0, 0, -2)
	tr.Record(true, 0)
	tr.Flush() // as the app quits or the computer sleeps

	reopened := Open(path)
	reopened.now = func() time.Time { return time.Date(2026, 10, 14, 23, 0, 0, 0, time.Local) }
	days := reopened.Recent(3)
	want := []Day{{"2026-10-12", 30, 30}, {"2026-10-13", 0, 0}, {"2026-10-14", 60, 30}}
	for i := range want {
		if days[i] != want[i] {
			t.Errorf("day %d = %+v, want %+v", i, days[i], want[i])
		}
	}
}

// Record batches writes; Flush saves the rest, and an empty Flush doesn't
// touch the file (including when every sample was asleep).
func TestSavesEveryFewSamples(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	tr := Open(path)
	tr.now = func() time.Time { return time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local) }
	saved := func() int { return Open(path).days["2026-10-14"].Awake }
	sample := int(Interval.Seconds())

	tr.Record(false, 0)
	tr.Flush()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("an asleep sample wrote a file: %v", err)
	}
	for range saveEvery - 1 {
		tr.Record(true, 0)
	}
	if got := saved(); got != 0 {
		t.Fatalf("saved %d s before the batch was complete", got)
	}
	tr.Record(true, 0)
	if got, want := saved(), saveEvery*sample; got != want {
		t.Fatalf("saved %d s after a batch, want %d", got, want)
	}
	tr.Record(true, 0)
	tr.Flush()
	if got, want := saved(), (saveEvery+1)*sample; got != want {
		t.Errorf("saved %d s after Flush, want %d", got, want)
	}
	if err := os.WriteFile(path, []byte("marker"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr.Flush()
	if data, err := os.ReadFile(path); err != nil || string(data) != "marker" {
		t.Errorf("an empty Flush wrote again: %q, %v", data, err)
	}
}

func TestFlushRetriesFailedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	if err := os.Mkdir(path, 0o755); err != nil { // a directory can't be overwritten
		t.Fatal(err)
	}
	tr := Open(path)
	tr.Record(true, 0)
	tr.Flush()
	if tr.unsaved != 1 {
		t.Fatal("a failed write discarded the pending sample")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	tr.Flush()
	if tr.unsaved != 0 || len(Open(path).days) != 1 {
		t.Fatal("the pending sample wasn't saved on retry")
	}
}

func TestKeepsSixtyDays(t *testing.T) {
	tr := Open(filepath.Join(t.TempDir(), "usage.json"))
	day := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	for i := range 70 {
		tr.now = func() time.Time { return day.AddDate(0, 0, i) }
		tr.Record(true, 0)
	}
	if len(tr.days) != keepDays {
		t.Fatalf("kept %d days, want %d", len(tr.days), keepDays)
	}
	if _, ok := tr.days["2026-01-01"]; ok {
		t.Error("the oldest day was kept")
	}
}
