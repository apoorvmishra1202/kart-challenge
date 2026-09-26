// Package ingest streams line-oriented, gzip-compressed data files into
// Postgres. It knows nothing about the meaning of the lines; callers decide
// which lines to keep via a filter.
package ingest

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

const bufSize = 64 * 1024

// Source reads a gzip stream line by line and implements pgx.CopyFromSource,
// emitting one []any{line string, sourceID int16} row per kept line. Memory use
// is bounded by the read buffer regardless of file size.
type Source struct {
	gz       *gzip.Reader
	br       *bufio.Reader
	sourceID int16
	keep     func([]byte) bool
	cur      string
	err      error
}

// NewSource wraps r, which must be gzip-compressed. keep may be nil to accept
// every non-empty line.
func NewSource(r io.Reader, sourceID int16, keep func([]byte) bool) (*Source, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("open gzip stream: %w", err)
	}
	return &Source{
		gz:       gz,
		br:       bufio.NewReaderSize(gz, bufSize),
		sourceID: sourceID,
		keep:     keep,
	}, nil
}

func (s *Source) Next() bool {
	for {
		line, isPrefix, err := s.br.ReadLine()
		if err != nil {
			return s.stop(err)
		}
		if isPrefix {
			// Longer than the buffer: cannot be a valid code, drop the rest of it.
			for isPrefix {
				if _, isPrefix, err = s.br.ReadLine(); err != nil {
					return s.stop(err)
				}
			}
			continue
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 || (s.keep != nil && !s.keep(line)) {
			continue
		}
		// ReadLine's slice is only valid until the next read, so copy it.
		s.cur = string(line)
		return true
	}
}

func (s *Source) stop(err error) bool {
	if err != io.EOF {
		s.err = fmt.Errorf("read line: %w", err)
	}
	return false
}

func (s *Source) Values() ([]any, error) { return []any{s.cur, s.sourceID}, nil }

func (s *Source) Err() error { return s.err }

func (s *Source) Close() error { return s.gz.Close() }
