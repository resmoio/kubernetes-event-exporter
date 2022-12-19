package exporter

import (
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"testing"
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
