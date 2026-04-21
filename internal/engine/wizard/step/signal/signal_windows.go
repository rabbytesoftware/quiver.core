//go:build windows

package signal

import "os"

var signalMap = map[string]os.Signal{
	"graceful":  os.Interrupt,
	"kill":      os.Kill,
	"interrupt": os.Interrupt,
}
