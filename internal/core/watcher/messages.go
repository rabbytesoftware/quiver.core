package watcher

import (
	"fmt"
)

func Debug(message string) {
	w.logger.Debug(message)
}

func Info(message string) {
	w.logger.Info(message)
}

func Warn(message string) {
	w.logger.Warning(message)
}

func Error(message string, args ...interface{}) {
	w.logger.Error(fmt.Sprintf(message, args...))
}

func Unforeseen(message string, args ...interface{}) {
	w.logger.Panic(fmt.Sprintf(message, args...))
}
