package ingest

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func gzipLines(t *testing.T, lines ...string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(strings.Join(lines, "\n") + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func lengthBetween(min, max int) func([]byte) bool {
	return func(b []byte) bool { return len(b) >= min && len(b) <= max }
}

func collect(t *testing.T, src *Source) [][]any {
	t.Helper()
	var rows [][]any
	for src.Next() {
		v, err := src.Values()
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, v)
	}
	if err := src.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	return rows
}

func TestSourceFiltersAndTrims(t *testing.T) {
	in := gzipLines(t, "SUPER100", "ABC", "HAPPYHRS\r", "  FIFTYOFF  ", "WAYTOOLONGCODE", "")

	src, err := NewSource(in, 2, lengthBetween(8, 10))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	got := collect(t, src)
	want := []string{"SUPER100", "HAPPYHRS", "FIFTYOFF"}
	if len(got) != len(want) {
		t.Fatalf("got %d rows %v, want %v", len(got), got, want)
	}
	for i, row := range got {
		if row[0] != want[i] {
			t.Errorf("row %d code = %q, want %q", i, row[0], want[i])
		}
		if row[1] != int16(2) {
			t.Errorf("row %d sourceID = %v, want 2", i, row[1])
		}
	}
}

func TestSourceSkipsLinesLongerThanBuffer(t *testing.T) {
	in := gzipLines(t, "SUPER100", strings.Repeat("X", bufSize*2), "FIFTYOFF")

	src, err := NewSource(in, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	got := collect(t, src)
	if len(got) != 2 || got[0][0] != "SUPER100" || got[1][0] != "FIFTYOFF" {
		t.Fatalf("got %v, want [SUPER100 FIFTYOFF]", got)
	}
}

func TestSourceScanReportsLineNumbers(t *testing.T) {
	in := gzipLines(t, "SUPER100", "ABC", "", strings.Repeat("X", bufSize*2), "FIFTYOFF")

	src, err := NewSource(in, 1, lengthBetween(8, 10))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	type hit struct {
		line string
		no   int64
	}
	var got []hit
	for src.Scan() {
		got = append(got, hit{string(src.Bytes()), src.LineNumber()})
	}
	if err := src.Err(); err != nil {
		t.Fatal(err)
	}
	want := []hit{{"SUPER100", 1}, {"FIFTYOFF", 5}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNewSourceRejectsNonGzip(t *testing.T) {
	if _, err := NewSource(strings.NewReader("SUPER100\n"), 1, nil); err == nil {
		t.Fatal("expected error for non-gzip input")
	}
}
