package watcher

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/errors"
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

func Errorf(message string, args ...interface{}) {
	w.logger.Info(fmt.Sprintf(message, args...))
}

// ? Error forces you to use the errors.Error type
func Error(message errors.Error) {
	w.logger.Error(message.Error())
}

// ? Unforeseen forces you to use the errors.Error type
func Unforeseen(message errors.Error) {
	w.logger.Fatal(message.Error())
}

func Unforeseenf(message string, args ...interface{}) {
	w.logger.Fatal(fmt.Sprintf(message, args...))
	panic(fmt.Sprintf(message, args...))
}
