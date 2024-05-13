package sinks

import (
	"context"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
)

type InMemoryConfig struct {
	SentUpdateEvent bool `yaml:"sentUpdateEvent,omitempty"`
	Ref             *InMemory
}

type InMemory struct {
	Events []*kube.EnhancedEvent
	Config *InMemoryConfig
}

func (i *InMemory) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	// skip update event
	if ev.IsUpdateEvent && !i.Config.SentUpdateEvent {
		return nil
	}
	i.Events = append(i.Events, ev)
	return nil
}

func (i *InMemory) Close() {
	// No-op
}
