package command_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

func TestSelfSession_Config_IsNeverAvailable(t *testing.T) {
	cfg, err := command.NewSessionForTest("http://127.0.0.1:1", "").Config()

	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "not available in the console")
}

func TestSelfSession_Spinner_RunsTheFunctionAndReturnsItsError(t *testing.T) {
	sess := command.NewSessionForTest("http://127.0.0.1:1", "")
	boom := errors.New("boom")
	ran := false

	err := sess.Spinner(nil, "label", func() error {
		ran = true
		return boom
	})

	assert.True(t, ran)
	assert.ErrorIs(t, err, boom)
	assert.NoError(t, sess.Spinner(nil, "label", func() error { return nil }))
}

func TestSelfSession_IsTTY_IsAlwaysFalse(t *testing.T) {
	assert.False(t, command.NewSessionForTest("http://127.0.0.1:1", "").IsTTY())
}

func TestSelfSession_Client_AttachesTheCallersTokenOnlyWhenThereIsOne(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	defer server.Close()

	for _, token := range []string{"tok", ""} {
		cli, err := command.NewSessionForTest(server.URL, token).Client(context.Background(), nil)
		require.NoError(t, err)
		_, err = cli.ListArrows(context.Background(), nil)
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"Bearer tok", ""}, seen)
}

func TestSelfSession_Client_RejectsAnUnusableAddress(t *testing.T) {
	_, err := command.NewSessionForTest("ftp://nope", "").Client(context.Background(), nil)

	assert.Error(t, err)
}
