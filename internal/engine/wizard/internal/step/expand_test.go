package step_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

const (
	testWorkDir     = "/home/user/.quiver/vault/repo"
	testInstallPath = "/home/user/.quiver/vault/repo"
	testRef         = "v1.2.0"
	testExportKey   = "quiver.test/quiver-test/tool-exporter.EXPORTED_BIN"
	testExportValue = "/home/user/.quiver/vault/tool-exporter/quiver-exporter-bin"
)

func expandVars() map[string]string {
	return map[string]string{
		"WORKDIR":      testWorkDir,
		"INSTALL_PATH": testInstallPath,
		"REF":          testRef,
		"EMPTY":        "",
		"DOLLAR":       "$HOME",
		"VAR":          "quiver-value",
		"A":            "alpha",
		"B":            "beta",
		testExportKey:  testExportValue,
	}
}

// TestRequest_Expand_Table asserts byte-exact output for every construct a
// manifest command can contain: the ones Quiver owns and the ones the shell
// owns.
func TestRequest_Expand_Table(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want string
	}{
		// Quiver's own syntax — replaced with the value.
		{
			name: "workdir builtin",
			in:   "cd ${WORKDIR}",
			want: "cd " + testWorkDir,
		},
		{
			name: "install path builtin",
			in:   "${INSTALL_PATH}/bin/tool",
			want: testInstallPath + "/bin/tool",
		},
		{
			name: "ref builtin",
			in:   "https://example.test/download/${REF}/asset.tgz",
			want: "https://example.test/download/" + testRef + "/asset.tgz",
		},
		{
			name: "namespaced export",
			in:   "test -n ${" + testExportKey + "}",
			want: "test -n " + testExportValue,
		},
		{
			name: "token is the entire string",
			in:   "${WORKDIR}",
			want: testWorkDir,
		},
		{
			name: "adjacent tokens",
			in:   "${A}${B}",
			want: "alphabeta",
		},
		{
			name: "repeated token",
			in:   "${A} and ${A} and ${A}",
			want: "alpha and alpha and alpha",
		},
		{
			name: "token inside double quotes",
			in:   `test "${A}" = "alpha"`,
			want: `test "alpha" = "alpha"`,
		},
		{
			name: "token inside single quotes",
			in:   "printf '${A}'",
			want: "printf 'alpha'",
		},
		{
			name: "token resolving to an empty value",
			in:   "echo [${EMPTY}]",
			want: "echo []",
		},
		{
			name: "value containing a dollar is not re-expanded",
			in:   "echo ${DOLLAR}",
			want: "echo $HOME",
		},
		{
			name: "token inside command substitution is still Quiver's",
			in:   "echo $(basename ${WORKDIR})",
			want: "echo $(basename " + testWorkDir + ")",
		},

		// Unknown Quiver references — left verbatim so typos stay visible.
		{
			name: "unknown token left verbatim",
			in:   "echo ${WORKDIRR}",
			want: "echo ${WORKDIRR}",
		},
		{
			name: "unknown token is the entire string",
			in:   "${NOPE}",
			want: "${NOPE}",
		},
		{
			name: "unknown token between known ones",
			in:   "${A}${NOPE}${B}",
			want: "alpha${NOPE}beta",
		},
		{
			name: "empty name left verbatim",
			in:   "echo ${}",
			want: "echo ${}",
		},
		{
			name: "unterminated token left verbatim",
			in:   "echo ${WORKDIR",
			want: "echo ${WORKDIR",
		},
		{
			name: "unterminated token after a resolved one",
			in:   "${A} ${B",
			want: "alpha ${B",
		},

		// The shell's syntax — handed through byte for byte.
		{
			name: "bare HOME",
			in:   "echo $HOME",
			want: "echo $HOME",
		},
		{
			name: "bare PATH",
			in:   "PATH=$PATH:/opt/bin",
			want: "PATH=$PATH:/opt/bin",
		},
		{
			name: "positional parameter list",
			in:   `run "$@"`,
			want: `run "$@"`,
		},
		{
			name: "positional parameter",
			in:   "echo $1",
			want: "echo $1",
		},
		{
			name: "exit status",
			in:   "test $? -eq 0",
			want: "test $? -eq 0",
		},
		{
			name: "command substitution",
			in:   "echo $(uname -s)",
			want: "echo $(uname -s)",
		},
		{
			name: "backtick substitution",
			in:   "echo `uname -s`",
			want: "echo `uname -s`",
		},
		{
			name: "shell default-value expansion",
			in:   "echo ${VAR:-default}",
			want: "echo ${VAR:-default}",
		},
		{
			name: "shell alternate-value expansion",
			in:   "echo ${VAR:+set}",
			want: "echo ${VAR:+set}",
		},
		{
			name: "shell length expansion",
			in:   "echo ${#VAR}",
			want: "echo ${#VAR}",
		},
		{
			name: "process id",
			in:   "echo $$",
			want: "echo $$",
		},
		{
			name: "escaped dollar",
			in:   `echo \$HOME`,
			want: `echo \$HOME`,
		},
		{
			name: "lone escaped dollar",
			in:   `echo \$`,
			want: `echo \$`,
		},
		{
			name: "bare name matching a known variable",
			in:   "echo $WORKDIR",
			want: "echo $WORKDIR",
		},
		{
			name: "shell variable assignment",
			in:   "x=1; echo $x",
			want: "x=1; echo $x",
		},

		// Degenerate inputs.
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "no dollar at all",
			in:   "echo hello world",
			want: "echo hello world",
		},
		{
			name: "lone open brace",
			in:   "echo ${",
			want: "echo ${",
		},
		{
			name: "lone close brace",
			in:   "echo }",
			want: "echo }",
		},
	}

	req := wizstep.Request{Vars: expandVars()}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, req.Expand(tc.in))
		})
	}
}

// TestRequest_Expand_NilVars asserts a request carrying no variables leaves
// every reference verbatim rather than blanking it.
func TestRequest_Expand_NilVars(t *testing.T) {
	req := wizstep.Request{}

	assert.Equal(t, "echo ${WORKDIR} $HOME", req.Expand("echo ${WORKDIR} $HOME"))
}

// TestRequest_Expand_ShellExpansionWrappingKnownName asserts the shell's
// ${VAR:-default} form stays whole even when VAR is a name Quiver knows: the
// lookup key is the entire brace body, not the identifier inside it.
func TestRequest_Expand_ShellExpansionWrappingKnownName(t *testing.T) {
	req := wizstep.Request{Vars: map[string]string{"VAR": "quiver-value"}}

	assert.Equal(t, "${VAR:-fallback}", req.Expand("${VAR:-fallback}"))
}
