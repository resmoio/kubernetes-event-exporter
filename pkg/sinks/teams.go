package sinks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
)

type TeamsConfig struct {
	Endpoint string                 `yaml:"endpoint"`
	Layout   map[string]interface{} `yaml:"layout"`
	Headers  map[string]string      `yaml:"headers"`
}

func NewTeamsSink(cfg *TeamsConfig) (Sink, error) {
	return &Teams{cfg: cfg}, nil
}

type Teams struct {
	cfg *TeamsConfig
}

func (w *Teams) Close() {
	// No-op
}

func (w *Teams) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	event, err := serializeEventWithLayout(w.cfg.Layout, ev)
	if err != nil {
		return err
	}

	var eventData map[string]interface{}
	json.Unmarshal([]byte(event), &eventData)
	output := fmt.Sprintf("Event: %s \nStatus: %s \nMetadata: %s", eventData["message"], eventData["reason"], eventData["metadata"])

	reqBody, err := json.Marshal(map[string]string{
		"summary": "event",
		"text":    string([]byte(output)),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, w.cfg.Endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	for k, v := range w.cfg.Headers {
		req.Header.Add(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	message := string(body)

	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("not 200: %s", message)
	}
	// see: https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/connectors-using?tabs=cURL#rate-limiting-for-connectors
	if strings.Contains(message, "Microsoft Teams endpoint returned HTTP error 429") {
		return fmt.Errorf("rate limited: %s", message)
	}

	return nil
}
