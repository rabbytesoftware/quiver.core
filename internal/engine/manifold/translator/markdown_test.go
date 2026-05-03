package translator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractArrowCodeblock_BasicBlock(t *testing.T) {
	input := "# Arrow\n\n```arrow\nname: my-tool\n```\n"
	got, ok := extractArrowCodeblock([]byte(input))
	assert.True(t, ok)
	assert.Equal(t, "name: my-tool", string(got))
}

func TestExtractArrowCodeblock_CRLFLineEndings(t *testing.T) {
	input := "# Arrow\r\n\r\n```arrow\r\nname: my-tool\r\n```\r\n"
	got, ok := extractArrowCodeblock([]byte(input))
	assert.True(t, ok)
	assert.Equal(t, "name: my-tool", string(got))
}

func TestExtractArrowCodeblock_MultilineContent(t *testing.T) {
	input := "```arrow\nname: tool\nversion: v1\n```\n"
	got, ok := extractArrowCodeblock([]byte(input))
	assert.True(t, ok)
	assert.Equal(t, "name: tool\nversion: v1", string(got))
}

func TestExtractArrowCodeblock_NoBlock_ReturnsFalse(t *testing.T) {
	input := "# Just a markdown file\n\nNo code blocks here.\n"
	got, ok := extractArrowCodeblock([]byte(input))
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestExtractArrowCodeblock_OtherFenceIgnored(t *testing.T) {
	input := "```yaml\nname: tool\n```\n"
	got, ok := extractArrowCodeblock([]byte(input))
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestExtractArrowCodeblock_UnclosedBlock_ReturnsFalse(t *testing.T) {
	input := "```arrow\nname: tool\n"
	got, ok := extractArrowCodeblock([]byte(input))
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestExtractArrowCodeblock_EmptyBlock(t *testing.T) {
	input := "```arrow\n```\n"
	got, ok := extractArrowCodeblock([]byte(input))
	assert.True(t, ok)
	assert.Equal(t, "", string(got))
}

func TestExtractArrowCodeblock_FirstBlockWins(t *testing.T) {
	input := "```arrow\nname: first\n```\n\n```arrow\nname: second\n```\n"
	got, ok := extractArrowCodeblock([]byte(input))
	assert.True(t, ok)
	assert.Equal(t, "name: first", string(got))
}

func TestExtractArrowCodeblock_EmptyInput_ReturnsFalse(t *testing.T) {
	got, ok := extractArrowCodeblock([]byte{})
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestExtractQuiverCodeblock_Found(t *testing.T) {
	input := []byte("# Title\n\n```quiver\nschema: quiver@v0\n```\n")
	got, ok := extractQuiverCodeblock(input)
	assert.True(t, ok)
	assert.Contains(t, string(got), "quiver@v0")
}

func TestExtractQuiverCodeblock_NotFound(t *testing.T) {
	input := []byte("# No quiver block here")
	_, ok := extractQuiverCodeblock(input)
	assert.False(t, ok)
}

func TestExtractQuiverCodeblock_ArrowFenceIgnored(t *testing.T) {
	input := []byte("```arrow\nschema: arrow@v0\n```\n")
	_, ok := extractQuiverCodeblock(input)
	assert.False(t, ok)
}

func TestExtractQuiverCodeblock_MultilineContent(t *testing.T) {
	input := []byte("```quiver\nschema: quiver@v0\nmetadata:\n  name: test\n```\n")
	got, ok := extractQuiverCodeblock(input)
	assert.True(t, ok)
	assert.Contains(t, string(got), "metadata:")
}
