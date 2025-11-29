package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/sirupsen/logrus"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// ? Watcher is an logging service, focus on an
// ? event sourcing approach to logging.

var (
	w    *Watcher
	once sync.Once
)

type Watcher struct {
	logger *logrus.Logger
}

func NewWatcherService() *Watcher {
	watcherConfig := config.GetWatcher()

	once.Do(func() {
		w = &Watcher{
			logger: initLogger(watcherConfig),
		}
	})

	return w
}

func GetWatcher() *Watcher {
	return w
}

func SetLevel(level logrus.Level) {
	w.logger.SetLevel(level)
}

func GetLevel() logrus.Level {
	return w.logger.GetLevel()
}

func WithFields(fields logrus.Fields) *logrus.Entry {
	return w.logger.WithFields(fields)
}

func WithField(key string, value interface{}) *logrus.Entry {
	return w.logger.WithField(key, value)
}

func GetConfig() config.Watcher {
	return config.GetWatcher()
}

func IsEnabled() bool {
	return config.GetWatcher().Enabled
}

func initLogger(watcherConfig config.Watcher) *logrus.Logger {
	logger := logrus.New()

	level, err := logrus.ParseLevel(watcherConfig.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	if !watcherConfig.Enabled || isTestEnvironment() {
		logger.SetOutput(os.Stderr)
		return logger
	}

	if err := os.MkdirAll(watcherConfig.Folder, 0750); err != nil {
		logger.SetOutput(os.Stderr)
		return logger
	}

	logFile := filepath.Join(watcherConfig.Folder, "quiver.log")

	logger.SetOutput(&lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    watcherConfig.MaxSize,
		MaxAge:     watcherConfig.MaxAge,
		MaxBackups: 3,
		Compress:   watcherConfig.Compress,
		LocalTime:  true,
	})

	logger.SetOutput(os.Stdout)

	return logger
}

func isTestEnvironment() bool {
	for _, arg := range os.Args {
		if strings.Contains(arg, "-test.") || strings.HasSuffix(arg, ".test") {
			return true
		}
	}

	return false
}
