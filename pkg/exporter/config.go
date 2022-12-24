package exporter

import (
	"errors"
	"strconv"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/resmoio/kubernetes-event-exporter/pkg/sinks"
	"github.com/rs/zerolog/log"
)

// Config allows configuration
type Config struct {
	// Route is the top route that the events will match
	// TODO: There is currently a tight coupling with route and config, but not with receiver config and sink so
	// TODO: I am not sure what to do here.
	LogLevel           string                    `yaml:"logLevel"`
	LogFormat          string                    `yaml:"logFormat"`
	ThrottlePeriod     int64                     `yaml:"throttlePeriod"`
	MaxEventAgeSeconds int64                     `yaml:"maxEventAgeSeconds"`
	ClusterName        string                    `yaml:"clusterName,omitempty"`
	Namespace          string                    `yaml:"namespace"`
	LeaderElection     kube.LeaderElectionConfig `yaml:"leaderElection"`
	Route              Route                     `yaml:"route"`
	Receivers          []sinks.ReceiverConfig    `yaml:"receivers"`
	KubeQPS            float32                   `yaml:"kubeQPS,omitempty"`
	KubeBurst          int                       `yaml:"kubeBurst,omitempty"`
  Kubeconfig         string                    `yaml:"kubeconfig,omitempty"`
}

func (c *Config) Validate() error {
	if err := c.validateDefaults(); err != nil {
		return err
	}

	// No duplicate receivers
	// Receivers individually
	// Routers recursive
	return nil
}

func (c *Config) validateDefaults() error {
	if err := c.validateMaxEventAgeSeconds(); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateMaxEventAgeSeconds() error {
	if c.ThrottlePeriod == 0 && c.MaxEventAgeSeconds == 0 {
		c.MaxEventAgeSeconds = 5
		log.Info().Msg("set config.maxEventAgeSeconds=5 (default)")
	} else if c.ThrottlePeriod != 0 && c.MaxEventAgeSeconds != 0 {
		log.Error().Msg("cannot set both throttlePeriod (depricated) and MaxEventAgeSeconds")
		return errors.New("validateMaxEventAgeSeconds failed")
	} else if c.ThrottlePeriod != 0 {
		log_value := strconv.FormatInt(c.ThrottlePeriod, 10)
		log.Info().Msg("config.maxEventAgeSeconds=" + log_value)
		log.Warn().Msg("config.throttlePeriod is depricated, consider using config.maxEventAgeSeconds instead")
		c.MaxEventAgeSeconds = c.ThrottlePeriod
	} else {
		log_value := strconv.FormatInt(c.MaxEventAgeSeconds, 10)
		log.Info().Msg("config.maxEventAgeSeconds=" + log_value)
	}
	return nil
}
