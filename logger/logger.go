// Package logger preserves the legacy Zap-based logging API.
//
// Deprecated: new services should use github.com/heliantheon/common/log.
package logger

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Log   *zap.Logger
	Sugar *zap.SugaredLogger
)

// Config configures the legacy logger.
type Config struct {
	Format string
	Level  string
	Debug  bool
}

func init() {
	if Log == nil {
		var err error
		Log, err = zap.NewDevelopment()
		if err != nil {
			panic(fmt.Sprintf("init logger failed: %v", err))
		}
		Sugar = Log.Sugar()
	}
}

// InitWithConfig initializes the legacy global logger.
func InitWithConfig(cfg Config) {
	var zapCfg zap.Config
	if strings.EqualFold(cfg.Format, "json") {
		zapCfg = zap.NewProductionConfig()
		zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		zapCfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006/01/02 15:04:05")
	}

	switch strings.ToLower(cfg.Level) {
	case "debug":
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "info":
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	case "warn", "warning":
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error":
		zapCfg.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		if cfg.Debug {
			zapCfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		} else {
			zapCfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		}
	}

	zapCfg.OutputPaths = []string{"stdout"}
	zapCfg.ErrorOutputPaths = []string{"stderr"}

	var err error
	Log, err = zapCfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		panic(err)
	}
	Sugar = Log.Sugar()
}

// Sync flushes the legacy logger.
func Sync() {
	if Log != nil {
		if err := Log.Sync(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "[Logger] sync failed: %v\n", err)
		}
	}
}

// Debug emits a debug record.
func Debug(msg string, fields ...zap.Field) { Log.Debug(msg, fields...) }

// Info emits an informational record.
func Info(msg string, fields ...zap.Field) { Log.Info(msg, fields...) }

// Warn emits a warning record.
func Warn(msg string, fields ...zap.Field) { Log.Warn(msg, fields...) }

// Error emits an error record.
func Error(msg string, fields ...zap.Field) { Log.Error(msg, fields...) }

// Fatal emits a fatal record and exits the process.
func Fatal(msg string, fields ...zap.Field) { Log.Fatal(msg, fields...) }

// Debugf emits a formatted debug record.
func Debugf(template string, args ...any) { Sugar.Debugf(template, args...) }

// Infof emits a formatted informational record.
func Infof(template string, args ...any) { Sugar.Infof(template, args...) }

// Warnf emits a formatted warning record.
func Warnf(template string, args ...any) { Sugar.Warnf(template, args...) }

// Errorf emits a formatted error record.
func Errorf(template string, args ...any) { Sugar.Errorf(template, args...) }

// Fatalf emits a formatted fatal record and exits the process.
func Fatalf(template string, args ...any) { Sugar.Fatalf(template, args...) }

// WithFields returns a child logger with fields.
func WithFields(fields ...zap.Field) *zap.Logger { return Log.With(fields...) }

// GormWriter returns a GORM-compatible legacy log writer.
func GormWriter() *GormLogWriter { return &GormLogWriter{} }

// S returns the legacy sugared logger.
func S() *zap.SugaredLogger { return Sugar }

// L returns the legacy base logger.
func L() *zap.Logger { return Log }

// Exit flushes logs and exits the process.
func Exit(code int) {
	Sync()
	os.Exit(code)
}

// GormLogWriter adapts the legacy sugared logger to GORM.
type GormLogWriter struct{}

// Printf implements GORM's logger.Writer interface.
func (w *GormLogWriter) Printf(format string, args ...any) {
	Sugar.Infof(format, args...)
}
