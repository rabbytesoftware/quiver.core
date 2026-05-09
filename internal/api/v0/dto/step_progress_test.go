package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func TestStepProgressDTOFrom_RunStep(t *testing.T) {
	sp := domainRuntime.StepProgress{
		Index:  0,
		Status: domainRuntime.StepStatusRunning,
		Step:   domainStep.NewRunStep("Install Discord", "echo install", false, "", true),
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
		Step:   domainStep.NewFetchStep("Download file", "https://example.com/file", "/tmp/f", "", "", false),
	}
	d := dto.StepProgressDTOFrom(sp)
	assert.Equal(t, 1, d.Index)
	assert.Equal(t, "completed", d.Status)
	assert.Equal(t, "Download file", d.Title)
	assert.Equal(t, "fetch", d.Type)
}

func TestRunRecordDTOFrom_IncludesSteps(t *testing.T) {
	r := &domainRuntime.Execution{
		Method:    "_install",
		Variables: map[string]string{"KEY": "val"},
		Steps: []domainRuntime.StepProgress{
			{Index: 0, Status: domainRuntime.StepStatusCompleted, Step: domainStep.NewRunStep("Step A", "echo a", false, "", false)},
			{Index: 1, Status: domainRuntime.StepStatusRunning, Step: domainStep.NewFetchStep("Step B", "https://x.com", "/tmp/b", "", "", false)},
			{Index: 2, Status: domainRuntime.StepStatusPending, Step: domainStep.NewRunStep("Step C", "echo c", false, "", false)},
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
	r := &domainRuntime.Execution{Method: "run"}
	d := dto.RunRecordDTOFrom(r)
	require.NotNil(t, d)
	assert.Empty(t, d.Steps)
}

func TestStepProgressDTOFrom_FailedStep_IncludesError(t *testing.T) {
	errMsg := "hdiutil: no mountable file systems"
	sp := domainRuntime.StepProgress{
		Index:  1,
		Status: domainRuntime.StepStatusFailed,
		Step:   domainStep.NewRunStep("Mount DMG", "hdiutil attach", false, "", true),
		Error:  &errMsg,
	}
	d := dto.StepProgressDTOFrom(sp)
	assert.Equal(t, "failed", d.Status)
	require.NotNil(t, d.Error)
	assert.Equal(t, errMsg, *d.Error)
}

func TestStepProgressDTOFrom_NilStep(t *testing.T) {
	sp := domainRuntime.StepProgress{
		Index:  0,
		Status: domainRuntime.StepStatusPending,
		Step:   nil,
	}
	d := dto.StepProgressDTOFrom(sp)
	assert.Equal(t, 0, d.Index)
	assert.Equal(t, "pending", d.Status)
	assert.Equal(t, "", d.Title)
	assert.Equal(t, "", d.Type)
}

func TestReturnDTOFrom_IncludesSteps(t *testing.T) {
	errMsg := "permission denied"
	r := &domainRuntime.Return{
		Method:  "_install",
		Outcome: domainRuntime.ExecutionOutcomeFailed,
		Steps: []domainRuntime.StepProgress{
			{Index: 0, Status: domainRuntime.StepStatusCompleted, Step: domainStep.NewRunStep("Step A", "echo a", false, "", false)},
			{Index: 1, Status: domainRuntime.StepStatusFailed, Step: domainStep.NewRunStep("Step B", "echo b", false, "", true), Error: &errMsg},
		},
	}
	d := dto.ReturnDTOFrom(r)
	require.NotNil(t, d)
	require.Len(t, d.Steps, 2)
	assert.Equal(t, "completed", d.Steps[0].Status)
	assert.Nil(t, d.Steps[0].Error)
	assert.Equal(t, "failed", d.Steps[1].Status)
	require.NotNil(t, d.Steps[1].Error)
	assert.Equal(t, errMsg, *d.Steps[1].Error)
}
