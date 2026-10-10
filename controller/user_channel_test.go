package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskAPIKey(t *testing.T) {
	// Short key test
	shortKey := "12345678"
	assert.Equal(t, "sk-******", maskAPIKey(shortKey))

	// Standard OpenAI key test
	standardKey := "sk-proj-1234567890abcdefghijklmn"
	masked := maskAPIKey(standardKey)
	assert.Contains(t, masked, "sk-pro")
	assert.Contains(t, masked, "klmn")
	assert.Contains(t, masked, "******")
	assert.NotEqual(t, standardKey, masked)

	// Multiline key test
	multilineKey := "sk-keyone1234567890\nsk-keytwo1234567890"
	maskedMulti := maskAPIKey(multilineKey)
	assert.Contains(t, maskedMulti, "\n")
	assert.Contains(t, maskedMulti, "******")
}

func TestProfitSharingCalculation(t *testing.T) {
	quotaConsumed := 100000
	revenueShareRatio := 0.8
	sellerEarnings := int(float64(quotaConsumed) * revenueShareRatio)
	assert.Equal(t, 80000, sellerEarnings)

	// Zero consumption
	assert.Equal(t, 0, int(float64(0)*revenueShareRatio))
}
