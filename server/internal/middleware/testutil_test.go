package middleware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// decodeLogRecords returns the records in logs, the output of a
// [slog.JSONHandler], in the order they were written, each decoded into a T.
func decodeLogRecords[T any](t *testing.T, logs *bytes.Buffer) []T {
	t.Helper()

	var records []T
	for line := range bytes.Lines(logs.Bytes()) {
		var record T
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		records = append(records, record)
	}

	return records
}

// statusRecordingWriter is an [httptest.ResponseRecorder] that records every
// status passed to WriteHeader, including those the recorder ignores because
// a status was already sent.
type statusRecordingWriter struct {
	*httptest.ResponseRecorder

	statuses []int
}

func (w *statusRecordingWriter) WriteHeader(status int) {
	w.statuses = append(w.statuses, status)
	w.ResponseRecorder.WriteHeader(status)
}

// hijackableWriter is an [httptest.ResponseRecorder] whose connection can be
// hijacked. Hijack hands over conn and records the response headers as they
// were at that moment.
type hijackableWriter struct {
	*httptest.ResponseRecorder

	conn net.Conn

	hijacked       bool
	hijackedHeader http.Header
}

func (w *hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	w.hijackedHeader = w.Header().Clone()

	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}
