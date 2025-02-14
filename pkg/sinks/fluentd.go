package sinks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fluent/fluent-logger-golang/fluent"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/rs/zerolog/log"
)

// FluentConfig holds the configuration for sending events to Fluent Bit over TCP
type FluentConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Tag  string `yaml:"tag"`
	// DeDot all labels and annotations in the event. For both the event and the involvedObject
	DeDot      bool                   `yaml:"deDot"`
	Layout     map[string]interface{} `yaml:"layout"`
	BufferSize int                    `yaml:"bufferSize"`
}

func NewFluent(cfg *FluentConfig) (*Fluent, error) {
	// Connection specific
	logger, err := fluent.New(fluent.Config{
		FluentHost:  cfg.Host,
		FluentPort:  cfg.Port,
		BufferLimit: cfg.BufferSize,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Fluent Bit logger: %w", err)
	}

	return &Fluent{
		logger: logger,
		tag:    cfg.Tag,
		cfg:    cfg,
	}, nil
}

type Fluent struct {
	logger *fluent.Fluent
	tag    string
	cfg    *FluentConfig
}

func (f *Fluent) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	// Sends events to Fluentd/Fluent Bit with INPUT type tcp
	var toSend map[string]interface{}

	if f.cfg.DeDot {
		de := ev.DeDot()
		ev = &de
	}
	if f.cfg.Layout != nil {
		res, err := convertLayoutTemplate(f.cfg.Layout, ev)
		if err != nil {
			return fmt.Errorf("failed to convert layout template: %w", err)
		}
		toSend = res
	} else {
		jsonData := ev.ToJSON()
		if err := json.Unmarshal(jsonData, &toSend); err != nil {
			return fmt.Errorf("failed to unmarshal JSON data: %w", err)
		}
	}

	err := f.logger.Post(f.tag, toSend)
	if err != nil {
		log.Error().Msgf("Failed to send event to Fluent Bit: %v", err)
		return fmt.Errorf("failed to send event: %w", err)
	}

	return nil
}

func (f *Fluent) Close() {
	if err := f.logger.Close(); err != nil {
		log.Error().Msgf("Failed to close Fluent Bit logger: %v", err)
	}
}
