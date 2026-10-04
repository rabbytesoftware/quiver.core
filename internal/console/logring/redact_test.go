package logring_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/console/logring"
)

func TestIsSensitiveKey(t *testing.T) {
	for _, key := range []string{
		"token", "Token", "access_token", "SECRET", "client_secret", "password", "passwd",
		"Authorization", "apikey", "api_key", "api-key", "private_key", "X-Bearer", "cookie", "credentials", "pairing_code",
	} {
		assert.True(t, logring.IsSensitiveKey(key), key)
	}
	for _, key := range []string{"ns", "took", "retry", "msg", "addr", "device"} {
		assert.False(t, logring.IsSensitiveKey(key), key)
	}
}

func TestRedactValue_PassesNonStringsAndPlainStrings(t *testing.T) {
	assert.Equal(t, int64(3), logring.RedactValue("n", int64(3)))
	assert.Equal(t, "plain", logring.RedactValue("note", "plain"))
	assert.Equal(t, logring.Redacted, logring.RedactValue("token", int64(3)))
}

func TestRedactLine(t *testing.T) {
	cases := map[string]string{
		"install github.com/a/b":                   "install github.com/a/b",
		"install  github.com/a/b   --detach":       "install github.com/a/b --detach",
		"run x --data token=abc":                   "run x --data token=[redacted]",
		"run x --data=token=abc":                   "run x --data=token=[redacted]",
		"run x --data password=hunter2 --data a=b": "run x --data password=[redacted] --data a=b",
		"x --token abc":                            "x --token [redacted]",
		"x --token=abc":                            "x --token=[redacted]",
		"x --api-key=abc y":                        "x --api-key=[redacted] y",
		"info https://user:pw@host/r":              "info https://[redacted]@host/r",
		"x --secret":                               "x --secret",
		"":                                         "",
	}
	for in, want := range cases {
		assert.Equal(t, want, logring.RedactLine(in), in)
	}
}

func TestLevelHelpers(t *testing.T) {
	assert.True(t, logring.IsLevel("warn"))
	assert.False(t, logring.IsLevel("WARN"))
	assert.False(t, logring.IsLevel("trace"))
	assert.Equal(t, "debug", logring.LevelName(-8))
	assert.Equal(t, "error", logring.LevelName(12))
	assert.Equal(t, "warn", logring.LevelName(5))
}
