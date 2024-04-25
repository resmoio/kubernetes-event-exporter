package sinks

import (
	"fmt"
	"testing"
	"time"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/stretchr/testify/assert"
)

func makeTestEvent() *kube.EnhancedEvent {
	ev := &kube.EnhancedEvent{}
	ev.Namespace = "default"
	ev.Type = "Warning"
	ev.InvolvedObject.Kind = "Pod"
	ev.InvolvedObject.Name = "nginx-server-123abc-456def"
	ev.Message = "Successfully pulled image \"nginx:latest\""
	return ev
}

func TestOpenSearch_LegacyTime(t *testing.T) {
	p := "legacy-time-pattern-{2006-01-02}"
	ev := makeTestEvent()
	w := time.Now()

	r, err := osFormatIndexName(p, w, ev)

	assert.Nil(t, err)
	assert.Equal(t, fmt.Sprintf("legacy-time-pattern-%d-%02d-%02d", w.Year(), w.Month(), w.Day()), r)
}

func TestOpenSearch_JustEventInfo(t *testing.T) {
	p := "{{ .Namespace }}-{{ .InvolvedObject.Kind }}-static"
	ev := makeTestEvent()
	w := time.Now()

	r, err := osFormatIndexName(p, w, ev)

	assert.Nil(t, err)
	assert.Equal(t, "default-Pod-static", r)
}

func TestOpenSearch_EventAndTime(t *testing.T) {
	p := "{{ .Namespace }}-{2006-01-02}-{{ .Type }}"
	ev := makeTestEvent()
	w := time.Now()

	r, err := osFormatIndexName(p, w, ev)

	assert.Nil(t, err)
	assert.Equal(t, fmt.Sprintf("default-%d-%02d-%02d-Warning", w.Year(), w.Month(), w.Day()), r)
}

func TestOpenSearch_InvalidEvent(t *testing.T) {
	p := "{{ .NotPresent }}-{2006-01-02}"
	ev := makeTestEvent()
	w := time.Now()

	_, err := osFormatIndexName(p, w, ev)

	assert.ErrorContains(t, err, "can't evaluate field NotPresent")
}
