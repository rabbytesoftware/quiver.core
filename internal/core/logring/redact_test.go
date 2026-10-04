package logring_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

func TestRedact_IsSensitiveKey_ClassifiesKeys(t *testing.T) {
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

func TestRedact_RedactValue_PassesNonStringsAndPlainStrings(t *testing.T) {
	assert.Equal(t, int64(3), logring.RedactValue("n", int64(3)))
	assert.Equal(t, "plain", logring.RedactValue("note", "plain"))
	assert.Equal(t, logring.Redacted, logring.RedactValue("token", int64(3)))
}

func TestRedact_RedactLine_ReplacesSecretLookingValues(t *testing.T) {
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

func TestLevel_ParseLevel_MapsNamesAndFallsBackToInfo(t *testing.T) {
	assert.Equal(t, slog.LevelDebug, logring.ParseLevel("debug"))
	assert.Equal(t, slog.LevelWarn, logring.ParseLevel("warn"))
	assert.Equal(t, slog.LevelWarn, logring.ParseLevel("WARNING"))
	assert.Equal(t, slog.LevelError, logring.ParseLevel("error"))
	assert.Equal(t, slog.LevelInfo, logring.ParseLevel("info"))
	assert.Equal(t, slog.LevelInfo, logring.ParseLevel("bogus"))
}

func TestLevel_LevelName_MapsSlogLevelsToWireNames(t *testing.T) {
	assert.Equal(t, "debug", logring.LevelName(slog.LevelDebug-4))
	assert.Equal(t, "debug", logring.LevelName(slog.LevelDebug))
	assert.Equal(t, "info", logring.LevelName(slog.LevelInfo))
	assert.Equal(t, "warn", logring.LevelName(slog.LevelWarn+1))
	assert.Equal(t, "error", logring.LevelName(slog.LevelError+4))
}

func TestLevel_IsLevel_AcceptsOnlyTheFourWireNames(t *testing.T) {
	for _, name := range []string{"debug", "info", "warn", "error"} {
		assert.True(t, logring.IsLevel(name), name)
	}
	for _, name := range []string{"WARN", "trace", "", "warning"} {
		assert.False(t, logring.IsLevel(name), name)
	}
}

func TestRedact_RedactValue_BearerCredentialsAnyCase(t *testing.T) {
	for _, in := range []string{"Bearer abc", "BEARER abc", "bEaReR abc", "sent bearer  abc.def"} {
		got, _ := logring.RedactValue("hdr", in).(string)
		assert.NotContains(t, got, "abc", in)
		assert.Contains(t, got, logring.Redacted, in)
	}
	assert.Equal(t, "no secrets here", logring.RedactValue("hdr", "no secrets here"))
	assert.Equal(t, "https://host/path", logring.RedactValue("url", "https://host/path"))
}
