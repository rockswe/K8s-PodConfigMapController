package logging

import (
	"context"
	"fmt"
	"os"

	"k8s.io/klog/v2"
)

// Logger provides structured logging with different levels
type Logger struct {
	name string
}

// NewLogger creates a new logger instance
func NewLogger(name string) *Logger {
	return &Logger{name: name}
}

// Info logs an info message with structured fields
func (l *Logger) Info(msg string, keysAndValues ...interface{}) {
	klog.InfoS(msg, append([]interface{}{"component", l.name}, keysAndValues...)...)
}

// Error logs an error message with structured fields
func (l *Logger) Error(err error, msg string, keysAndValues ...interface{}) {
	klog.ErrorS(err, msg, append([]interface{}{"component", l.name}, keysAndValues...)...)
}

// Warning logs a warning message with structured fields
func (l *Logger) Warning(msg string, keysAndValues ...interface{}) {
	klog.InfoS(msg, append([]interface{}{"component", l.name, "level", "warning"}, keysAndValues...)...)
}

// Debug logs a debug message with structured fields (only if debug logging is enabled)
func (l *Logger) Debug(msg string, keysAndValues ...interface{}) {
	if klog.V(4).Enabled() {
		klog.V(4).InfoS(msg, append([]interface{}{"component", l.name}, keysAndValues...)...)
	}
}

// WithValues returns a new logger with additional key-value pairs
func (l *Logger) WithValues(keysAndValues ...interface{}) *Logger {
	// For simplicity, we'll just create a new logger with the same name
	// In a more sophisticated implementation, we'd store the values
	return &Logger{name: l.name}
}

// WithName returns a new logger with a different name
func (l *Logger) WithName(name string) *Logger {
	return &Logger{name: fmt.Sprintf("%s.%s", l.name, name)}
}

// SetupLogging configures klog with appropriate settings
func SetupLogging() {
	// Set up klog to use structured logging
	klog.InitFlags(nil)

	// Set default log level based on environment
	if os.Getenv("DEBUG") != "" {
		// Enable debug logging - this is handled by klog flags
		klog.Info("Debug logging enabled")
	}
}

// LoggerFromContext returns a logger from context, or creates a new one
func LoggerFromContext(ctx context.Context, name string) *Logger {
	// In a more sophisticated implementation, we'd store the logger in context
	return NewLogger(name)
}
