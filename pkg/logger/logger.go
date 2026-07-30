package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

type contextKey string

const (
	SessionIDKey     contextKey = "session_id"
	CorrelationIDKey contextKey = "correlation_id"
	AgentIDKey       contextKey = "agent_id"
)

// Config configures Logger initialization.
type Config struct {
	Level    slog.Level
	Format   string // "text" or "json"
	Output   io.Writer
	FilePath string
}

// Logger wraps slog.Logger with framework helper methods.
type Logger struct {
	slogLogger *slog.Logger
	levelVar   *slog.LevelVar
}

var (
	defaultLogger *Logger
	once          sync.Once
	mu            sync.RWMutex
)

// InitFromEnv initializes the global logger based on environment variables:
// - LOG_LEVEL: debug, info, warn, error (default: info)
// - LOG_FORMAT: text, json (default: text)
// - LOG_FILE: optional filepath to append log entries
func InitFromEnv() *Logger {
	levelStr := os.Getenv("LOG_LEVEL")
	formatStr := os.Getenv("LOG_FORMAT")
	filePath := os.Getenv("LOG_FILE")

	level := parseLevel(levelStr)
	format := strings.ToLower(strings.TrimSpace(formatStr))
	if format == "" {
		format = "text"
	}

	cfg := Config{
		Level:    level,
		Format:   format,
		FilePath: filePath,
	}

	l, err := New(cfg)
	if err != nil {
		// Fallback to basic text stdout logger if file creation fails
		l, _ = New(Config{Level: level, Format: "text", Output: os.Stdout})
	}
	SetDefault(l)
	return l
}

// New creates a new Logger instance.
func New(cfg Config) (*Logger, error) {
	levelVar := &slog.LevelVar{}
	levelVar.Set(cfg.Level)

	var writers []io.Writer
	if cfg.Output != nil {
		writers = append(writers, cfg.Output)
	} else {
		writers = append(writers, os.Stdout)
	}

	if cfg.FilePath != "" {
		f, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("logger: failed to open log file %q: %w", cfg.FilePath, err)
		}
		writers = append(writers, f)
	}

	var mw io.Writer
	if len(writers) == 1 {
		mw = writers[0]
	} else {
		mw = io.MultiWriter(writers...)
	}

	opts := &slog.HandlerOptions{
		Level: levelVar,
	}

	var handler slog.Handler
	if strings.ToLower(cfg.Format) == "json" {
		handler = slog.NewJSONHandler(mw, opts)
	} else {
		handler = slog.NewTextHandler(mw, opts)
	}

	slogger := slog.New(handler)
	return &Logger{
		slogLogger: slogger,
		levelVar:   levelVar,
	}, nil
}

// SetDefault sets the default package-level logger.
func SetDefault(l *Logger) {
	mu.Lock()
	defer mu.Unlock()
	defaultLogger = l
	slog.SetDefault(l.slogLogger)
}

// Default returns the default global logger.
func Default() *Logger {
	mu.RLock()
	l := defaultLogger
	mu.RUnlock()

	if l != nil {
		return l
	}

	once.Do(func() {
		l = InitFromEnv()
	})
	return l
}

// SetLevel updates the logger's log level dynamically.
func (l *Logger) SetLevel(level slog.Level) {
	if l != nil && l.levelVar != nil {
		l.levelVar.Set(level)
	}
}

// With returns a new Logger with additional key-value attributes.
func (l *Logger) With(args ...any) *Logger {
	if l == nil {
		l = Default()
	}
	return &Logger{
		slogLogger: l.slogLogger.With(args...),
		levelVar:   l.levelVar,
	}
}

// WithComponent creates a child logger with a component attribute.
func (l *Logger) WithComponent(component string) *Logger {
	return l.With(slog.String("component", component))
}

// WithAgent creates a child logger with an agent_id attribute.
func (l *Logger) WithAgent(agentID string) *Logger {
	return l.With(slog.String("agent_id", agentID))
}

// WithSession creates a child logger with a session_id attribute.
func (l *Logger) WithSession(sessionID string) *Logger {
	return l.With(slog.String("session_id", sessionID))
}

// Debug logs at Debug level.
func (l *Logger) Debug(msg string, args ...any) {
	if l == nil {
		l = Default()
	}
	l.slogLogger.Debug(msg, args...)
}

// Info logs at Info level.
func (l *Logger) Info(msg string, args ...any) {
	if l == nil {
		l = Default()
	}
	l.slogLogger.Info(msg, args...)
}

// Warn logs at Warn level.
func (l *Logger) Warn(msg string, args ...any) {
	if l == nil {
		l = Default()
	}
	l.slogLogger.Warn(msg, args...)
}

// Error logs at Error level.
func (l *Logger) Error(msg string, args ...any) {
	if l == nil {
		l = Default()
	}
	l.slogLogger.Error(msg, args...)
}

// DebugContext logs at Debug level with context attributes if present.
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	if l == nil {
		l = Default()
	}
	l.slogLogger.DebugContext(ctx, msg, append(args, extractCtxAttrs(ctx)...)...)
}

// InfoContext logs at Info level with context attributes if present.
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	if l == nil {
		l = Default()
	}
	l.slogLogger.InfoContext(ctx, msg, append(args, extractCtxAttrs(ctx)...)...)
}

// WarnContext logs at Warn level with context attributes if present.
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	if l == nil {
		l = Default()
	}
	l.slogLogger.WarnContext(ctx, msg, append(args, extractCtxAttrs(ctx)...)...)
}

// ErrorContext logs at Error level with context attributes if present.
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	if l == nil {
		l = Default()
	}
	l.slogLogger.ErrorContext(ctx, msg, append(args, extractCtxAttrs(ctx)...)...)
}

// Package-level global shortcuts

func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	Default().DebugContext(ctx, msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	Default().InfoContext(ctx, msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	Default().WarnContext(ctx, msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	Default().ErrorContext(ctx, msg, args...)
}

func With(args ...any) *Logger {
	return Default().With(args...)
}

func WithComponent(component string) *Logger {
	return Default().WithComponent(component)
}

func WithAgent(agentID string) *Logger {
	return Default().WithAgent(agentID)
}

func WithSession(sessionID string) *Logger {
	return Default().WithSession(sessionID)
}

func parseLevel(str string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(str)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func extractCtxAttrs(ctx context.Context) []any {
	if ctx == nil {
		return nil
	}
	var attrs []any
	if sess, ok := ctx.Value(SessionIDKey).(string); ok && sess != "" {
		attrs = append(attrs, slog.String("session_id", sess))
	} else if sess, ok := ctx.Value("session_id").(string); ok && sess != "" {
		attrs = append(attrs, slog.String("session_id", sess))
	}
	if corr, ok := ctx.Value(CorrelationIDKey).(string); ok && corr != "" {
		attrs = append(attrs, slog.String("correlation_id", corr))
	} else if corr, ok := ctx.Value("correlation_id").(string); ok && corr != "" {
		attrs = append(attrs, slog.String("correlation_id", corr))
	}
	if agent, ok := ctx.Value(AgentIDKey).(string); ok && agent != "" {
		attrs = append(attrs, slog.String("agent_id", agent))
	} else if agent, ok := ctx.Value("agent_id").(string); ok && agent != "" {
		attrs = append(attrs, slog.String("agent_id", agent))
	}
	return attrs
}
