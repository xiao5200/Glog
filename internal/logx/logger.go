package logx

import (
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	mu      sync.RWMutex
	log     *zap.Logger
	logFile *os.File
)

// Init 初始化日志，输出到文件与 stderr。
func Init(logPath string, debug bool) error {
	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	cfg.OutputPaths = []string{"stderr"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	var file *os.File
	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		file = f
	}

	options := []zap.Option{zap.AddCallerSkip(1)}
	if file != nil {
		// 自行持有文件句柄，以便 Close 时真正关闭文件（zap.Sync 不会关闭）。
		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(cfg.EncoderConfig),
			zapcore.AddSync(file),
			cfg.Level,
		)
		options = append(options, zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(core, fileCore)
		}))
	}

	l, err := cfg.Build(options...)
	if err != nil {
		if file != nil {
			_ = file.Close()
		}
		return err
	}

	mu.Lock()
	oldLog := log
	oldFile := logFile
	log = l
	logFile = file
	mu.Unlock()

	if oldLog != nil {
		_ = oldLog.Sync()
	}
	if oldFile != nil {
		_ = oldFile.Close()
	}
	return nil
}

func get() *zap.Logger {
	mu.RLock()
	l := log
	mu.RUnlock()
	if l == nil {
		return zap.NewNop()
	}
	return l
}

func Debug(msg string, fields ...zap.Field) { get().Debug(msg, fields...) }
func Info(msg string, fields ...zap.Field)  { get().Info(msg, fields...) }
func Warn(msg string, fields ...zap.Field)  { get().Warn(msg, fields...) }
func Error(msg string, fields ...zap.Field) { get().Error(msg, fields...) }
func Fatal(msg string, fields ...zap.Field) { get().Fatal(msg, fields...) }

// Sync 刷新缓冲。
func Sync() {
	mu.RLock()
	l := log
	mu.RUnlock()
	if l != nil {
		_ = l.Sync()
	}
}

// Close 刷新并关闭日志文件。关闭后日志调用会静默丢弃。
func Close() {
	mu.Lock()
	l := log
	f := logFile
	log = nil
	logFile = nil
	mu.Unlock()

	if l != nil {
		_ = l.Sync()
	}
	if f != nil {
		_ = f.Close()
	}
}
