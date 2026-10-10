// Package logger provides a global zap-based structured logger for the gateway.
//
// It outputs JSON-formatted logs with configurable level, supports async writing,
// and provides helper functions to inject trace_id/span_id into log entries.
package logger

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// L is the global logger singleton used across the application.
	L *zap.Logger
	// S is the global sugared logger for convenience.
	S *zap.SugaredLogger

	once      sync.Once
	asyncChan chan logEntry
	wg        sync.WaitGroup
	stopCh    chan struct{}
)

type logEntry struct {
	level zapcore.Level
	msg   string
	fields []zap.Field
}

// Init initializes the global logger with the given configuration.
//
// Parameters:
//   - level: log level (debug/info/warn/error)
//   - format: output format (json/console)
//   - output: output destination (stdout or file path)
//   - async: whether to use async writing
func Init(level, format, output string, async bool) {
	once.Do(func() {
		cfg := zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(parseLevel(level))
		cfg.Encoding = format
		cfg.EncoderConfig.TimeKey = "timestamp"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		cfg.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
		cfg.OutputPaths = []string{output}
		cfg.ErrorOutputPaths = []string{"stderr"}

		var err error
		L, err = cfg.Build()
		if err != nil {
			// Fallback to stderr logger
			L = zap.NewExample()
		}
		S = L.Sugar()

		if async {
			startAsyncWriter()
		}
	})
}

// parseLevel converts a string level to zapcore.Level.
func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// startAsyncWriter launches a background goroutine for async log writing.
// This prevents the request path from blocking on disk I/O.
func startAsyncWriter() {
	asyncChan = make(chan logEntry, 10000)
	stopCh = make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case entry := <-asyncChan:
				L.Log(entry.level, entry.msg, entry.fields...)
			case <-stopCh:
				// Drain remaining entries
				for {
					select {
					case entry := <-asyncChan:
						L.Log(entry.level, entry.msg, entry.fields...)
					default:
						return
					}
				}
			}
		}
	}()
}

// Sync flushes any buffered log entries. Should be called on shutdown.
func Sync() {
	if L != nil {
		_ = L.Sync()
	}
	if stopCh != nil {
		close(stopCh)
		wg.Wait()
	}
}

// WithTrace returns a logger with trace_id and span_id fields attached.
func WithTrace(traceID, spanID string) *zap.Logger {
	if L == nil {
		return zap.NewExample()
	}
	return L.With(
		zap.String("trace_id", traceID),
		zap.String("span_id", spanID),
	)
}

// Info logs an info-level message with optional structured data fields.
// data is a map of key-value pairs that will be converted to zap fields.
func Info(msg string, data ...map[string]interface{}) {
	if L != nil {
		L.Info(msg, mapToFields(data...)...)
	}
}

// Warn logs a warn-level message with optional structured data fields.
func Warn(msg string, data ...map[string]interface{}) {
	if L != nil {
		L.Warn(msg, mapToFields(data...)...)
	}
}

// Error logs an error-level message with optional structured data fields.
func Error(msg string, data ...map[string]interface{}) {
	if L != nil {
		L.Error(msg, mapToFields(data...)...)
	}
}

// Debug logs a debug-level message with optional structured data fields.
func Debug(msg string, data ...map[string]interface{}) {
	if L != nil {
		L.Debug(msg, mapToFields(data...)...)
	}
}

// Fatal logs a fatal-level message and exits.
func Fatal(msg string, data ...map[string]interface{}) {
	if L != nil {
		L.Fatal(msg, mapToFields(data...)...)
	}
	os.Exit(1)
}

// mapToFields converts one or more maps to zap.Field slices.
func mapToFields(maps ...map[string]interface{}) []zap.Field {
	fields := make([]zap.Field, 0)
	for _, m := range maps {
		for k, v := range m {
			fields = append(fields, zap.Any(k, v))
		}
	}
	return fields
}
