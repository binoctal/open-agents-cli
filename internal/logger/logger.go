package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

const (
	MaxSize    = 10 * 1024 * 1024 // 10MB
	MaxBackups = 7
)

// Log levels
const (
	LevelDebug = iota
	LevelInfo
	LevelWarn
	LevelError
)

var levelNames = map[int]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
}

var levelFromString = map[string]int{
	"debug": LevelDebug,
	"info":  LevelInfo,
	"warn":  LevelWarn,
	"error": LevelError,
}

// Global logger instance
var globalLogger *Logger

// Logger is a rotating file logger with level support
type Logger struct {
	dir     string
	file    *os.File
	size    int64
	level   int
	mu      sync.Mutex
}

// New creates a new rotating logger
func New() (*Logger, error) {
	dir := getLogDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	l := &Logger{dir: dir, level: LevelInfo}
	if err := l.openFile(); err != nil {
		return nil, err
	}

	globalLogger = l
	return l, nil
}

// SetLevel sets the log level from string (debug, info, warn, error)
func (l *Logger) SetLevel(levelStr string) {
	if level, ok := levelFromString[levelStr]; ok {
		l.level = level
	}
}

// SetGlobalLevel sets the global logger level
func SetGlobalLevel(levelStr string) {
	if globalLogger != nil {
		globalLogger.SetLevel(levelStr)
	}
}

// Debug logs at debug level
func Debug(format string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.log(LevelDebug, format, args...)
	}
}

// Info logs at info level
func Info(format string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.log(LevelInfo, format, args...)
	}
}

// Warn logs at warn level
func Warn(format string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.log(LevelWarn, format, args...)
	}
}

// Error logs at error level
func Error(format string, args ...interface{}) {
	if globalLogger != nil {
		globalLogger.log(LevelError, format, args...)
	}
}

func (l *Logger) log(level int, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	line := fmt.Sprintf("[%s] [%s] %s\n", timestamp, levelNames[level], msg)

	l.Write([]byte(line))
}

func (l *Logger) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.size+int64(len(p)) > MaxSize {
		l.rotate()
	}

	n, err = l.file.Write(p)
	l.size += int64(n)
	return
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) openFile() error {
	name := fmt.Sprintf("bridge-%s.log", time.Now().Format("2006-01-02"))
	path := filepath.Join(l.dir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	info, _ := f.Stat()
	l.file = f
	l.size = info.Size()
	return nil
}

func (l *Logger) rotate() {
	l.file.Close()
	l.cleanup()
	l.openFile()
}

func (l *Logger) cleanup() {
	entries, _ := os.ReadDir(l.dir)
	var logs []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".log" {
			logs = append(logs, filepath.Join(l.dir, e.Name()))
		}
	}

	if len(logs) <= MaxBackups {
		return
	}

	sort.Slice(logs, func(i, j int) bool {
		fi, _ := os.Stat(logs[i])
		fj, _ := os.Stat(logs[j])
		return fi.ModTime().Before(fj.ModTime())
	})

	for i := 0; i < len(logs)-MaxBackups; i++ {
		os.Remove(logs[i])
	}
}

// Writer returns an io.Writer for use with log.SetOutput
func (l *Logger) Writer() io.Writer {
	return io.MultiWriter(l, os.Stderr)
}

func getLogDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "open-agents", "logs")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Logs", "open-agents")
	default:
		return filepath.Join(os.Getenv("HOME"), ".open-agents", "logs")
	}
}
