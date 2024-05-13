package sinks

import (
	"context"
	"encoding/json"
	"log/syslog"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
)

type SyslogConfig struct {
	SentUpdateEvent bool   `yaml:"sentUpdateEvent,omitempty"`
	Network         string `yaml:"network"`
	Address         string `yaml:"address"`
	Tag             string `yaml:"tag"`
}

type SyslogSink struct {
	sw  *syslog.Writer
	cfg *SyslogConfig
}

func NewSyslogSink(config *SyslogConfig) (Sink, error) {
	w, err := syslog.Dial(config.Network, config.Address, syslog.LOG_LOCAL0, config.Tag)
	if err != nil {
		return nil, err
	}
	return &SyslogSink{sw: w, cfg: config}, nil
}

func (w *SyslogSink) Close() {
	w.sw.Close()
}

func (w *SyslogSink) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	// skip update event
	if ev.IsUpdateEvent && !w.cfg.SentUpdateEvent {
		return nil
	}
	if b, err := json.Marshal(ev); err == nil {
		_, writeErr := w.sw.Write(b)

		if writeErr != nil {
			return writeErr
		}
	} else {
		return err
	}
	return nil
}
