package exporter

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func readConfig(t *testing.T, yml string) Config {
	var cfg Config
	err := yaml.Unmarshal([]byte(yml), &cfg)
	if err != nil {
		t.Fatal("Cannot parse yaml", err)
	}
	return cfg
}

func Test_ParseConfig(t *testing.T) {
	const yml = `
route:
  routes:
    - drop:
        - minCount: 6
          apiVersion: v33
      match:
        - receiver: stdout
receivers:
  - name: stdout
    stdout: {}
`

	cfg := readConfig(t, yml)

	assert.Len(t, cfg.Route.Routes, 1)
	assert.Len(t, cfg.Route.Routes[0].Drop, 1)
	assert.Len(t, cfg.Route.Routes[0].Match, 1)
	assert.Len(t, cfg.Route.Routes[0].Drop, 1)

	assert.Equal(t, int32(6), cfg.Route.Routes[0].Drop[0].MinCount)
	assert.Equal(t, "v33", cfg.Route.Routes[0].Drop[0].APIVersion)
	assert.Equal(t, "stdout", cfg.Route.Routes[0].Match[0].Receiver)
}


func TestValidate_IsCheckingMaxEventAgeSeconds_WhenNotSet(t *testing.T) {
	config := Config{}
	err := config.Validate()
	assert.True(t, config.MaxEventAgeSeconds == 5)
	assert.NoError(t, err)
}

func TestValidate_IsCheckingMaxEventAgeSeconds_WhenThrottledPeriodSet(t *testing.T) {
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	config := Config{
		ThrottlePeriod: 123,
	}
	err := config.Validate()

	assert.True(t, config.MaxEventAgeSeconds == 123)
	assert.Contains(t, output.String(), "config.maxEventAgeSeconds=123")
	assert.Contains(t, output.String(), "config.throttlePeriod is depricated, consider using config.maxEventAgeSeconds instead")
	assert.NoError(t, err)
}

func TestValidate_IsCheckingMaxEventAgeSeconds_WhenMaxEventAgeSecondsSet(t *testing.T) {
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	config := Config{
		MaxEventAgeSeconds: 123,
	}
	err := config.Validate()
	assert.True(t, config.MaxEventAgeSeconds == 123)
	assert.Contains(t, output.String(), "config.maxEventAgeSeconds=123")
	assert.NoError(t, err)
}

func TestValidate_IsCheckingMaxEventAgeSeconds_WhenMaxEventAgeSecondsAndThrottledPeriodSet(t *testing.T) {
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	config := Config{
		ThrottlePeriod:     123,
		MaxEventAgeSeconds: 321,
	}
	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, output.String(), "cannot set both throttlePeriod (depricated) and MaxEventAgeSeconds")
}

func TestValidate_MetricsNamePrefix_WhenEmpty(t *testing.T) {
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	config := Config{}
	err := config.Validate()
	assert.NoError(t, err)
	assert.Equal(t, "", config.MetricsNamePrefix)
	assert.Contains(t, output.String(), "metrics name prefix is empty, setting config.metricsNamePrefix='event_exporter_' is recommended")
}

func TestValidate_MetricsNamePrefix_WhenValid(t *testing.T) {
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	validCases := []string{
		"kubernetes_event_exporter_",
		"test_",
		"test_test_",
		"test::test_test_",
		"TEST::test_test_",
		"test_test::1234_test_",
	}

	for _, testPrefix := range validCases {
		config := Config{
			MetricsNamePrefix: testPrefix,
		}
		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, testPrefix, config.MetricsNamePrefix)
		assert.Contains(t, output.String(), "config.metricsNamePrefix='" + testPrefix + "'")
	}
}

func TestValidate_MetricsNamePrefix_WhenInvalid(t *testing.T) {
	output := &bytes.Buffer{}
	log.Logger = log.Logger.Output(output)

	invalidCases := []string{
		"no_tracing_underscore",
		"__reserved_",
		"::wrong_",
		"13245_test_",
	}

	for _, testPrefix := range invalidCases {
		config := Config{
			MetricsNamePrefix: testPrefix,
		}
		err := config.Validate()
		assert.Error(t, err)
		assert.Equal(t, testPrefix, config.MetricsNamePrefix)
		assert.Contains(t, output.String(), "config.metricsNamePrefix should match the regex: ^[a-zA-Z][a-zA-Z0-9_:]*_$")
	}
}
