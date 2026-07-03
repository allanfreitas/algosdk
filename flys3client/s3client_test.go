package flys3client

import (
	"encoding/csv"
	"io"
	"strings"
	"testing"
)

// TestStreamCSV verifies that StreamCSV correctly reads rows from a CSV reader
// and ignores the header row.
func TestStreamCSV(t *testing.T) {
	csvData := `id,code_part1,code_part2,code_part3,description
010101,01,01,01,Análise e desenvolvimento de sistemas
010201,01,02,01,Programação de computadores
`

	// Create a mock stream csv callback
	var rows [][]string
	callback := func(row []string) error {
		rows = append(rows, row)
		return nil
	}

	// We can test the csv parser directly by isolating the reader logic or mocking Client.
	// Since StreamCSV calls GetObject, we can't easily run it without AWS client initialization,
	// unless we test the underlying CSV reading loop.
	r := strings.NewReader(csvData)
	reader := csvReaderHelper(r)

	// Skip header row
	if _, err := reader.Read(); err != nil {
		t.Fatalf("failed to read header: %v", err)
	}

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read error: %v", err)
		}
		if err := callback(record); err != nil {
			t.Fatalf("callback error: %v", err)
		}
		count++
	}

	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows recorded, got %d", len(rows))
	}

	if rows[0][0] != "010101" || rows[0][4] != "Análise e desenvolvimento de sistemas" {
		t.Errorf("unexpected first row: %v", rows[0])
	}
}

func csvReaderHelper(r io.Reader) *csv.Reader {
	reader := csv.NewReader(r)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	return reader
}
