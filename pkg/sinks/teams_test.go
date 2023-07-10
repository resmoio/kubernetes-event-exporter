package sinks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/stretchr/testify/assert"
)

func TestTeams_Send(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	client := Teams{cfg: &TeamsConfig{Endpoint: ts.URL}}

	err := client.Send(context.Background(), &kube.EnhancedEvent{})

	assert.NoError(t, err)
}

func TestTeams_Send_WhenTeamsReturnsRateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Webhook message delivery failed with error: Microsoft Teams endpoint returned HTTP error 429 with ContextId tcid=0"))
	}))
	defer ts.Close()
	client := Teams{cfg: &TeamsConfig{Endpoint: ts.URL}}

	err := client.Send(context.Background(), &kube.EnhancedEvent{})

	assert.ErrorContains(t, err, "rate limited")
}
