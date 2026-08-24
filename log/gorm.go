package log

import (
	"fmt"
	"log/slog"
)

// GormWriter adapts slog to GORM's Printf-based logging interface.
func GormWriter(logger *slog.Logger) *GormLogWriter {
	return &GormLogWriter{logger: logger}
}

// GormLogWriter is a GORM-compatible structured log writer.
type GormLogWriter struct {
	logger *slog.Logger
}

// Printf implements GORM's logger.Writer interface.
func (w *GormLogWriter) Printf(format string, args ...any) {
	if w != nil && w.logger != nil {
		w.logger.Info("database", "detail", formatMessage(format, args...))
	}
}

func formatMessage(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
