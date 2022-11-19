package exporter

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

func TestValidate_IsCheckingMaxEventAgeSeconds_WhenNotSet(t *testing.T) {
	config := Config{}
	config.Validate()
	assert.True(t, config.MaxEventAgeSeconds == 5)
}

func TestValidate_IsCheckingMaxEventAgeSeconds_WhenThrottledPeriodSet(t *testing.T) {
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	config := Config{
		ThrottlePeriod: 123,
	}
	config.Validate()

	assert.True(t, config.MaxEventAgeSeconds == 123)
	assert.Contains(t, output.String(), "config.maxEventAgeSeconds=123")
	assert.Contains(t, output.String(), "config.throttlePeriod is depricated, consider using config.maxEventAgeSeconds instead")
}

func TestValidate_IsCheckingMaxEventAgeSeconds_WhenMaxEventAgeSecondsSet(t *testing.T) {
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	config := Config{
		MaxEventAgeSeconds: 123,
	}
	config.Validate()
	assert.True(t, config.MaxEventAgeSeconds == 123)
	assert.Contains(t, output.String(), "config.maxEventAgeSeconds=123")
}

func TestValidate_IsCheckingMaxEventAgeSeconds_WhenMaxEventAgeSecondsAndThrottledPeriodSet(t *testing.T) {
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	config := Config{
		ThrottlePeriod:     123,
		MaxEventAgeSeconds: 321,
	}
	config.Validate()
	// should call log.Fatal() so the program will exit
	assert.True(t, false)
}
