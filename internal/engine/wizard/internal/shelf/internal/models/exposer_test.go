package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClaims(t *testing.T) {
	assert.Equal(t, Claim{Bare: "github.com/u/r"}, NamespaceClaim("github.com/u/r"))
	assert.Equal(t, Claim{Bare: "github.com/u/r", Workdir: "/wd"}, WorkdirClaim("github.com/u/r", "/wd"))
}
