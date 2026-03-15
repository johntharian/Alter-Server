package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Level string

const (
	DebugLevel Level = "debug"
	InfoLevel  Level = "info"
	WarnLevel  Level = "warn"
	ErrorLevel Level = "error"
)

var levelPriority = map[Level]int{
	DebugLevel: 0,
	InfoLevel:  1,
	WarnLevel:  2,
	ErrorLevel: 3,
}

type Logger interface {
	Debug(msg string, fields map[string]interface{})
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, fields map[string]interface{})
	Close() error
}

type FileLogger struct {
	file  *os.File
	level Level
	mu    sync.Mutex
}

func NewFileLogger(filePath string, minLevel Level) (*FileLogger, error) {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return &FileLogger{
		file:  f,
		level: minLevel,
	}, nil
}

func (l *FileLogger) log(level Level, msg string, fields map[string]interface{}) {
	if levelPriority[level] < levelPriority[l.level] {
		return
	}

	entry := map[string]interface{}{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"level": level,
		"msg":   msg,
	}

	for k, v := range fields {
		entry[k] = v
	}

	b, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: failed to marshal log entry: %v\n", err)
		return
	}
	b = append(b, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.file.Write(b)
}

func (l *FileLogger) Debug(msg string, fields map[string]interface{}) { l.log(DebugLevel, msg, fields) }
func (l *FileLogger) Info(msg string, fields map[string]interface{})  { l.log(InfoLevel, msg, fields) }
func (l *FileLogger) Warn(msg string, fields map[string]interface{})  { l.log(WarnLevel, msg, fields) }
func (l *FileLogger) Error(msg string, fields map[string]interface{}) { l.log(ErrorLevel, msg, fields) }

func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Global logger instance
var (
	globalLogger Logger
	globalMu     sync.RWMutex
)

func Init(filePath string, level string) error {
	lvl := InfoLevel
	switch level {
	case "debug":
		lvl = DebugLevel
	case "info":
		lvl = InfoLevel
	case "warn":
		lvl = WarnLevel
	case "error":
		lvl = ErrorLevel
	}

	l, err := NewFileLogger(filePath, lvl)
	if err != nil {
		return err
	}

	globalMu.Lock()
	if globalLogger != nil {
		_ = globalLogger.Close()
	}
	globalLogger = l
	globalMu.Unlock()

	return nil
}

func Get() Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLogger
}

func Debug(msg string, fields map[string]interface{}) {
	if l := Get(); l != nil {
		l.Debug(msg, fields)
	}
}

func Info(msg string, fields map[string]interface{}) {
	if l := Get(); l != nil {
		l.Info(msg, fields)
	}
}

func Warn(msg string, fields map[string]interface{}) {
	if l := Get(); l != nil {
		l.Warn(msg, fields)
	}
}

func Error(msg string, fields map[string]interface{}) {
	if l := Get(); l != nil {
		l.Error(msg, fields)
	}
}

func Close() error {
	if l := Get(); l != nil {
		return l.Close()
	}
	return nil
}
