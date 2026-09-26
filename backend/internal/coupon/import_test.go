package coupon

import (
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeGzip(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codes.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	if _, err := zw.Write([]byte(strings.Join(lines, "\n") + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func testImporter() *Importer {
	return NewImporter(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultMinFiles)
}

func TestReadSortedFile(t *testing.T) {
	path := writeGzip(t, "SUPER100", "ABC", "HAPPYHRS\r", "  SUPER100  ", "WAYTOOLONGCODE", "", "BIRTHDAY10")

	got, err := testImporter().readSortedFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"HAPPYHRS", "SUPER100", "BIRTHDAY10"} // sorted, deduped
	if d := dec(got); !slices.Equal(d, want) {
		t.Errorf("got %v, want %v", d, want)
	}
}

func TestReadSortedFileRejectsInvalidCharacter(t *testing.T) {
	// Wrong-length lines are skipped, but a valid-length line with a bad
	// character must fail with its line number.
	path := writeGzip(t, "SUPER100", "abc", "HAPPYHRS", "SUPER-10", "FIFTYOFF")

	_, err := testImporter().readSortedFile(context.Background(), path)
	if err == nil {
		t.Fatal("expected error for invalid character")
	}
	for _, want := range []string{"codes.gz", "line 4", `"SUPER-10"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestCapacityHintIsUpperBound(t *testing.T) {
	path := writeGzip(t, "SUPER100", "HAPPYHRS", "BIRTHDAY10")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if hint := capacityHint(f); hint < 3 {
		t.Errorf("capacityHint = %d, want >= 3", hint)
	}
}

func TestParseMinFiles(t *testing.T) {
	for in, want := range map[string]int{"": DefaultMinFiles, "1": 1, "2": 2, "3": 3} {
		if got, err := ParseMinFiles(in); err != nil || got != want {
			t.Errorf("ParseMinFiles(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "4", "-1", "two", " 2", "2.0"} {
		if _, err := ParseMinFiles(in); err == nil {
			t.Errorf("ParseMinFiles(%q) succeeded, want error", in)
		}
	}
}
