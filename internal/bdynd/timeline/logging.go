package timeline

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

type contextKey string

const requestIDKey contextKey = "bdynd-timeline-request-id"

// WithRequestID attaches a stable operation id to a context so long-running
// upload, restore, repack, and prune steps can be correlated in logs.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return requestID
}

type LogLevel string

const (
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

type LogEvent struct {
	Time      time.Time
	Level     LogLevel
	RequestID string
	Op        string
	BlockID   string
	Message   string
}

type Logger interface {
	Log(ctx context.Context, event LogEvent)
}

type NopLogger struct{}

func (NopLogger) Log(context.Context, LogEvent) {}

type TextLogger struct {
	Out io.Writer
	Now func() time.Time
}

func (l TextLogger) Log(ctx context.Context, event LogEvent) {
	if l.Out == nil {
		return
	}
	if event.Time.IsZero() {
		if l.Now != nil {
			event.Time = l.Now()
		} else {
			event.Time = time.Now().UTC()
		}
	}
	if event.Level == "" {
		event.Level = LevelInfo
	}
	if event.RequestID == "" {
		event.RequestID = RequestID(ctx)
	}
	parts := []string{
		event.Time.UTC().Format(time.RFC3339),
		"level=" + string(event.Level),
	}
	if event.RequestID != "" {
		parts = append(parts, "request_id="+quoteLogValue(event.RequestID))
	}
	if event.Op != "" {
		parts = append(parts, "op="+quoteLogValue(event.Op))
	}
	if event.BlockID != "" {
		parts = append(parts, "block_id="+quoteLogValue(event.BlockID))
	}
	if event.Message != "" {
		parts = append(parts, "msg="+quoteLogValue(event.Message))
	}
	_, _ = fmt.Fprintln(l.Out, strings.Join(parts, " "))
}

func quoteLogValue(value string) string {
	if value == "" {
		return "\"\""
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '"'
	}) < 0 {
		return value
	}
	return fmt.Sprintf("%q", value)
}
