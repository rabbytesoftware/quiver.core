package signal

import (
	"fmt"
	"os"
	"strings"
)

func ParseSignal(
	name string,
) (os.Signal, error) {
	if sig, ok := signalMap[name]; ok {
		return sig, nil
	}
	if sig, ok := signalMap[strings.ToUpper(name)]; ok {
		return sig, nil
	}
	return nil, fmt.Errorf("unknown signal: %s", name)
}
