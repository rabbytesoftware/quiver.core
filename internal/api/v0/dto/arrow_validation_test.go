package dto_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationResultDTOFrom_Valid(t *testing.T) {
	r := &models.ValidationResult{
		Valid:                true,
		SupportedPlatforms:   []domain.OS{domain.OSLinuxAMD64, domain.OSDarwinARM64},
		UnsupportedPlatforms: []domain.OS{domain.OSWindowsAMD64},
	}

	d := dto.ValidationResultDTOFrom(r)

	assert.True(t, d.Valid)
	require.Len(t, d.SupportedPlatforms, 2)
	assert.Contains(t, d.SupportedPlatforms, domain.OSLinuxAMD64.String())
	assert.Contains(t, d.SupportedPlatforms, domain.OSDarwinARM64.String())
	require.Len(t, d.UnsupportedPlatforms, 1)
	assert.Contains(t, d.UnsupportedPlatforms, domain.OSWindowsAMD64.String())
	assert.Empty(t, d.Errors)
}

func TestValidationResultDTOFrom_Invalid_WithErrors(t *testing.T) {
	r := &models.ValidationResult{
		Valid: false,
		Errors: []models.ValidationError{
			{Field: "lifecycle.install", Rule: "missing_pair", Message: "install requires uninstall"},
			{Field: "targets", Rule: "no_supported_platform", Message: "no matching platform"},
		},
		SupportedPlatforms:   []domain.OS{},
		UnsupportedPlatforms: []domain.OS{domain.OSLinuxAMD64},
	}

	d := dto.ValidationResultDTOFrom(r)

	assert.False(t, d.Valid)
	require.Len(t, d.Errors, 2)
	assert.Equal(t, "lifecycle.install", d.Errors[0].Field)
	assert.Equal(t, "missing_pair", d.Errors[0].Rule)
	assert.Equal(t, "install requires uninstall", d.Errors[0].Message)
	assert.Equal(t, "no_supported_platform", d.Errors[1].Rule)
	assert.Empty(t, d.SupportedPlatforms)
}

func TestValidationResultDTOFrom_NoErrors_NoOmitErrors(t *testing.T) {
	r := &models.ValidationResult{
		Valid:                false,
		Errors:               []models.ValidationError{},
		SupportedPlatforms:   []domain.OS{},
		UnsupportedPlatforms: []domain.OS{},
	}

	d := dto.ValidationResultDTOFrom(r)

	assert.False(t, d.Valid)
	assert.Empty(t, d.Errors)
}

func TestValidationResultDTOFrom_EmptyPlatforms(t *testing.T) {
	r := &models.ValidationResult{
		Valid:                true,
		SupportedPlatforms:   []domain.OS{},
		UnsupportedPlatforms: []domain.OS{},
	}

	d := dto.ValidationResultDTOFrom(r)

	assert.True(t, d.Valid)
	assert.Empty(t, d.SupportedPlatforms)
	assert.Empty(t, d.UnsupportedPlatforms)
}
