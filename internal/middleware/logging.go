package middleware

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"time"

	"discobox/internal/types"
	"net/http"
)

// loggingResponseWriter captures response details for logging
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
	started    time.Time
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if lrw.statusCode == 0 {
		lrw.statusCode = http.StatusOK
	}
	n, err := lrw.ResponseWriter.Write(b)
	lrw.bytes += n
	return n, err
}

func (lrw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := lrw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("response writer does not support hijacking")
}

// AccessLogging creates access logging middleware
func AccessLogging(logger types.Logger) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer
			lrw := &loggingResponseWriter{
				ResponseWriter: w,
				started:        start,
			}

			// Get request details
			path := r.URL.Path
			if r.URL.RawQuery != "" {
				path = path + "?" + r.URL.RawQuery
			}

			// Process request
			next.ServeHTTP(lrw, r)

			// Log the request
			duration := time.Since(start)

			logger.Info("request",
				"method", r.Method,
				"path", path,
				"status", lrw.statusCode,
				"duration", duration,
				"bytes", lrw.bytes,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"referer", r.Referer(),
			)
		})
	}
}

// StructuredLogger provides structured logging with context
type StructuredLogger struct {
	logger types.Logger
	fields []any
}

// NewStructuredLogger creates a structured logger
func NewStructuredLogger(logger types.Logger) *StructuredLogger {
	return &StructuredLogger{
		logger: logger,
		fields: make([]any, 0),
	}
}

// With adds fields to the logger
func (sl *StructuredLogger) With(fields ...any) *StructuredLogger {
	newFields := make([]any, len(sl.fields)+len(fields))
	copy(newFields, sl.fields)
	copy(newFields[len(sl.fields):], fields)

	return &StructuredLogger{
		logger: sl.logger,
		fields: newFields,
	}
}

// Middleware returns logging middleware
func (sl *StructuredLogger) Middleware() types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Generate request ID if not present
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = generateRequestID()
				r.Header.Set("X-Request-ID", requestID)
			}

			// Add request ID to response
			w.Header().Set("X-Request-ID", requestID)

			// Create logger with request context
			reqLogger := sl.With(
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
			)

			// Log request start
			reqLogger.logger.Debug("request started", reqLogger.fields...)

			// Wrap response writer
			lrw := &loggingResponseWriter{
				ResponseWriter: w,
				started:        start,
			}

			// Process request
			next.ServeHTTP(lrw, r)

			// Log request completion
			duration := time.Since(start)
			fields := append(reqLogger.fields,
				"status", lrw.statusCode,
				"duration_ms", duration.Milliseconds(),
				"bytes", lrw.bytes,
			)

			// Log based on status code
			if lrw.statusCode >= 500 {
				reqLogger.logger.Error("request failed", fields...)
			} else if lrw.statusCode >= 400 {
				reqLogger.logger.Warn("request error", fields...)
			} else {
				reqLogger.logger.Info("request completed", fields...)
			}
		})
	}
}

// RequestLogger adds a logger to the request context
func RequestLogger(logger types.Logger) types.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add logger to request context
			ctx := r.Context()
			ctx = ContextWithLogger(ctx, logger)

			// Add request ID if not present
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = generateRequestID()
				r.Header.Set("X-Request-ID", requestID)
			}

			// Create request-specific logger
			reqLogger := logger.With("request_id", requestID)
			ctx = ContextWithLogger(ctx, reqLogger)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
}

// Context keys for logging
type contextKey string

const loggerKey contextKey = "logger"

// ContextWithLogger adds a logger to the context
func ContextWithLogger(ctx context.Context, logger types.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// LoggerFromContext retrieves a logger from the context
func LoggerFromContext(ctx context.Context) types.Logger {
	if logger, ok := ctx.Value(loggerKey).(types.Logger); ok {
		return logger
	}
	return nil
}
