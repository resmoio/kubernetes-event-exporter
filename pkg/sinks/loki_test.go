package sinks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"testing"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLoki_Send(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	client := Loki{cfg: &LokiConfig{URL: ts.URL}}

	err := client.Send(context.Background(), &kube.EnhancedEvent{})

	assert.NoError(t, err)
}

func TestLoki_Send_StreamLabelsTemplated(t *testing.T) {
	rr := httptest.NewRecorder()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		rr.Write(result)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	client := Loki{cfg: &LokiConfig{
		URL: ts.URL,
		StreamLabels: map[string]string{
			"app":              "kube-events",
			"object_namespace": "{{ .InvolvedObject.Namespace }}",
		}}}

	ev := &kube.EnhancedEvent{}
	ev.Namespace = "default"
	ev.Reason = "my reason"
	ev.Type = "Warning"
	ev.InvolvedObject.Kind = "Pod"
	ev.InvolvedObject.Name = "nginx-server-123abc-456def"
	ev.InvolvedObject.Namespace = "prod"
	ev.Message = "Successfully pulled image \"nginx:latest\""
	ev.FirstTimestamp = v1.Time{Time: time.Now()}

	err := client.Send(context.Background(), ev)
	assert.NoError(t, err)

	var res LokiMsg
	err = json.Unmarshal(rr.Body.Bytes(), &res)
	assert.NoError(t, err)

	assert.Equal(t, res.Streams[0].Stream["app"], "kube-events", "Non template labels should remain the same")
	assert.Equal(t, res.Streams[0].Stream["object_namespace"], "prod", "Template labels should be templated")
}
