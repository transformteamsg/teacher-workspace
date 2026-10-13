package middleware

import (
	"bytes"
	"encoding/json"
	"testing"
)

// logRecord is one record written by a [slog.JSONHandler].
type logRecord struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Err   string `json:"err"`
}

// decodeLogRecords returns the records in logs, the output of a
// [slog.JSONHandler], in the order they were written.
func decodeLogRecords(t *testing.T, logs *bytes.Buffer) []logRecord {
	t.Helper()

	var records []logRecord
	for line := range bytes.Lines(logs.Bytes()) {
		var record logRecord
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		records = append(records, record)
	}

	return records
}
