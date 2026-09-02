package timeline

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestRequestIDRoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), " restore-123 ")
	if got := RequestID(ctx); got != "restore-123" {
		t.Fatalf("request id=%q", got)
	}
	if got := RequestID(WithRequestID(context.Background(), " ")); got != "" {
		t.Fatalf("empty request id=%q", got)
	}
}

func TestTextLoggerIncludesRequestAndBlockFields(t *testing.T) {
	var out bytes.Buffer
	logger := TextLogger{
		Out: &out,
		Now: func() time.Time { return time.Date(2026, 9, 2, 1, 2, 3, 0, time.UTC) },
	}
	ctx := WithRequestID(context.Background(), "req-1")
	logger.Log(ctx, LogEvent{Level: LevelWarn, Op: "repack", BlockID: "archive-1", Message: "block ready"})
	line := out.String()
	for _, want := range []string{
		"2026-09-02T01:02:03Z",
		"level=warn",
		"request_id=req-1",
		"op=repack",
		"block_id=archive-1",
		"msg=\"block ready\"",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q missing %q", line, want)
		}
	}
}

func TestNopLoggerDoesNothing(t *testing.T) {
	NopLogger{}.Log(context.Background(), LogEvent{Message: "ignored"})
}
