package signal

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

var signalMap = map[string]os.Signal{
	"SIGTERM": syscall.SIGTERM,
	"SIGINT":  syscall.SIGINT,
	"SIGKILL": syscall.SIGKILL,
}

func ParseSignal(
	name string,
) (os.Signal, error) {
	sig, ok := signalMap[strings.ToUpper(name)]
	if !ok {
		return nil, fmt.Errorf("unknown signal: %s", name)
	}
	return sig, nil
}
