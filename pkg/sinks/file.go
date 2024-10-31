package sinks

import (
	"context"
	"io"

	"github.com/bytedance/sonic"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"gopkg.in/natefinch/lumberjack.v2"
)

var sonicAPI = sonic.Config{
	EscapeHTML:       true, // 安全性
	CompactMarshaler: true, // 兼容性
	CopyString:       true, // 正确性
}.Froze()

type FileConfig struct {
	Path       string                 `yaml:"path"`
	Layout     map[string]interface{} `yaml:"layout"`
	MaxSize    int                    `yaml:"maxsize"`
	MaxAge     int                    `yaml:"maxage"`
	MaxBackups int                    `yaml:"maxbackups"`
	DeDot      bool                   `yaml:"deDot"`
}

func (f *FileConfig) Validate() error {
	return nil
}

type File struct {
	writer  io.WriteCloser
	encoder sonic.Encoder
	layout  map[string]interface{}
	DeDot   bool
}

func NewFileSink(config *FileConfig) (*File, error) {
	writer := &lumberjack.Logger{
		Filename:   config.Path,
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
	}

	return &File{
		writer:  writer,
		encoder: sonicAPI.NewEncoder(writer),
		layout:  config.Layout,
		DeDot:   config.DeDot,
	}, nil
}

func (f *File) Close() {
	_ = f.writer.Close()
}

func (f *File) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	if f.DeDot {
		de := ev.DeDot()
		ev = &de
	}
	if f.layout == nil {
		return f.encoder.Encode(ev)
	}

	res, err := convertLayoutTemplate(f.layout, ev)
	if err != nil {
		return err
	}

	return f.encoder.Encode(res)
}
