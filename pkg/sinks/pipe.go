package sinks

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
)

type PipeConfig struct {
	Path   string                 `yaml:"path"`
	// DeDot all labels and annotations in the event. For both the event and the involvedObject
	DeDot       bool              `yaml:"deDot"`
	Layout map[string]interface{} `yaml:"layout"`
}

func (f *PipeConfig) Validate() error {
	return nil
}

type Pipe struct {
	writer  io.WriteCloser
	encoder *json.Encoder
	cfg     *PipeConfig
}

func NewPipeSink(config *PipeConfig) (*Pipe, error) {
	mode := os.FileMode(0644)
	f, err := os.OpenFile(config.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return nil, err
	}
	return &Pipe{
		writer:  f,
		encoder: json.NewEncoder(f),
		cfg:     config,
	}, nil
}

func (f *Pipe) Close() {
	_ = f.writer.Close()
}

func (f *Pipe) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	if f.cfg.DeDot {
		de := ev.DeDot()
		ev = &de
	}

	if f.cfg.Layout == nil {
		return f.encoder.Encode(ev)
	}

	res, err := convertLayoutTemplate(f.cfg.Layout, ev)
	if err != nil {
		return err
	}

	return f.encoder.Encode(res)
}
