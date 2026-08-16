package component_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
)

func TestFields_Render_AlignsLabels(t *testing.T) {
	th := newTestTheme(t)
	fields := []component.Field{
		{Label: "Name", Value: "repo", Set: true},
		{Label: "Description", Value: "a thing", Set: true},
	}

	got := component.Fields("", fields, th)

	assert.Equal(t, ""+
		"Name         repo\n"+
		"Description  a thing\n", got)
}

func TestFields_Render_OmitsAbsentButMarksEmpty(t *testing.T) {
	th := newTestTheme(t)
	fields := []component.Field{
		{Label: "Name", Value: "repo", Set: true},
		{Label: "License", Value: "", Set: true},
		{Label: "Tags", Value: "", Set: false},
	}

	got := component.Fields("", fields, th)

	assert.Contains(t, got, "License")
	assert.Contains(t, got, "—", "set-but-empty must be visible")
	assert.NotContains(t, got, "Tags", "absent fields are omitted")
}

func TestFields_Render_TitleIsPrefixed(t *testing.T) {
	th := newTestTheme(t)
	fields := []component.Field{{Label: "A", Value: "b", Set: true}}

	got := component.Fields("Lifecycle", fields, th)

	assert.Equal(t, "Lifecycle\nA  b\n", got)
}

func TestFields_Render_AllAbsentRendersNothing(t *testing.T) {
	th := newTestTheme(t)

	got := component.Fields("Title", []component.Field{{Label: "A", Set: false}}, th)

	assert.Equal(t, "", got, "a title alone must not be rendered")
}

func TestFields_Render_NoFieldsRendersNothing(t *testing.T) {
	assert.Equal(t, "", component.Fields("", nil, newTestTheme(t)))
}

func TestFields_Render_WidthTracksTheLongestShownLabel(t *testing.T) {
	th := newTestTheme(t)
	fields := []component.Field{
		{Label: "A", Value: "1", Set: true},
		{Label: "AVeryLongHiddenLabel", Value: "2", Set: false},
	}

	got := component.Fields("", fields, th)

	assert.Equal(t, "A  1\n", got, "hidden labels must not widen the column")
}
