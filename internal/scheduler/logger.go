package scheduler

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/hibiken/asynq"
)

// AsynqLogger routes asynq's internal logs (worker + scheduler) through slog
// so they land in the same JSON log / log file as the rest of the app.
type AsynqLogger struct {
	log *slog.Logger
}

func NewAsynqLogger(log *slog.Logger) *AsynqLogger {
	return &AsynqLogger{log: log.With(slog.String("component", "asynq"))}
}

func (l *AsynqLogger) Debug(args ...interface{}) { l.log.Debug(fmt.Sprint(args...)) }
func (l *AsynqLogger) Info(args ...interface{})  { l.log.Info(fmt.Sprint(args...)) }
func (l *AsynqLogger) Warn(args ...interface{})  { l.log.Warn(fmt.Sprint(args...)) }
func (l *AsynqLogger) Error(args ...interface{}) { l.log.Error(fmt.Sprint(args...)) }

// Fatal matches asynq's expectation that Fatal terminates the process.
func (l *AsynqLogger) Fatal(args ...interface{}) {
	l.log.Error(fmt.Sprint(args...), slog.Bool("fatal", true))
	os.Exit(1)
}

// AsynqLogLevel maps LOG_LEVEL to asynq's level, defaulting to info.
func AsynqLogLevel(level string) asynq.LogLevel {
	var l asynq.LogLevel
	if err := l.Set(level); err != nil {
		return asynq.InfoLevel
	}
	return l
}
