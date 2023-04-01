package sinks

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
)

type StdoutConfig struct {
	// DeDot all labels and annotations in the event. For both the event and the involvedObject
	DeDot       bool              `yaml:"deDot"`
	Layout map[string]interface{} `yaml:"layout"`
}

func (f *StdoutConfig) Validate() error {
	return nil
}

type Stdout struct {
	writer  io.Writer
	encoder *json.Encoder
	cfg     *StdoutConfig
}

func NewStdoutSink(config *StdoutConfig) (*Stdout, error) {
	logger := log.New(os.Stdout, "", 0)
	writer := logger.Writer()

	return &Stdout{
		writer:  writer,
		encoder: json.NewEncoder(writer),
		cfg:     config,
	}, nil
}

func (f *Stdout) Close() {
	return
}

func (f *Stdout) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
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
