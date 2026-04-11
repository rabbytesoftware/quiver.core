package dto_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepProgressDTOFrom_RunStep(t *testing.T) {
	sp := domainRuntime.StepProgress{
		Index:  0,
		Status: domainRuntime.StepStatusRunning,
		Step:   domainStep.NewRunStep("Install Discord", "echo install", 0, true),
	}
	d := dto.StepProgressDTOFrom(sp)
	assert.Equal(t, 0, d.Index)
	assert.Equal(t, "running", d.Status)
	assert.Equal(t, "Install Discord", d.Title)
	assert.Equal(t, "run", d.Type)
}

func TestStepProgressDTOFrom_FetchStep(t *testing.T) {
	sp := domainRuntime.StepProgress{
		Index:  1,
		Status: domainRuntime.StepStatusCompleted,
		Step:   domainStep.NewFetchStep("Download file", "https://example.com/file", "/tmp/f", 0, false),
	}
	d := dto.StepProgressDTOFrom(sp)
	assert.Equal(t, 1, d.Index)
	assert.Equal(t, "completed", d.Status)
	assert.Equal(t, "Download file", d.Title)
	assert.Equal(t, "fetch", d.Type)
}

func TestRunRecordDTOFrom_IncludesSteps(t *testing.T) {
	r := &domainRuntime.RunRecord{
		Method:    "_install",
		Variables: map[string]string{"KEY": "val"},
		Steps: []domainRuntime.StepProgress{
			{Index: 0, Status: domainRuntime.StepStatusCompleted, Step: domainStep.NewRunStep("Step A", "echo a", 0, false)},
			{Index: 1, Status: domainRuntime.StepStatusRunning, Step: domainStep.NewFetchStep("Step B", "https://x.com", "/tmp/b", 0, false)},
			{Index: 2, Status: domainRuntime.StepStatusPending, Step: domainStep.NewRunStep("Step C", "echo c", 0, false)},
		},
	}
	d := dto.RunRecordDTOFrom(r)
	require.NotNil(t, d)
	require.Len(t, d.Steps, 3)
	assert.Equal(t, "completed", d.Steps[0].Status)
	assert.Equal(t, "Step A", d.Steps[0].Title)
	assert.Equal(t, "running", d.Steps[1].Status)
	assert.Equal(t, "Step B", d.Steps[1].Title)
	assert.Equal(t, "fetch", d.Steps[1].Type)
	assert.Equal(t, "pending", d.Steps[2].Status)
}

func TestRunRecordDTOFrom_NoSteps_EmitsEmptyNotNull(t *testing.T) {
	r := &domainRuntime.RunRecord{Method: "run"}
	d := dto.RunRecordDTOFrom(r)
	require.NotNil(t, d)
	assert.Empty(t, d.Steps)
}
