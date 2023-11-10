package sinks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
	"github.com/rs/zerolog/log"
)

type promtailStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type LokiMsg struct {
	Streams []promtailStream `json:"streams"`
}

type LokiConfig struct {
	Layout       map[string]interface{} `yaml:"layout"`
	StreamLabels map[string]string      `yaml:"streamLabels"`
	TLS          TLS                    `yaml:"tls"`
	URL          string                 `yaml:"url"`
	Headers      map[string]string      `yaml:"headers"`
}

type Loki struct {
	cfg       *LokiConfig
	transport *http.Transport
}

func NewLoki(cfg *LokiConfig) (Sink, error) {
	tlsClientConfig, err := setupTLS(&cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("failed to setup TLS: %w", err)
	}
	return &Loki{cfg: cfg, transport: &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: tlsClientConfig,
	}}, nil
}

func generateTimestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10) + "000000000"
}

func (l *Loki) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	eventBody, err := serializeEventWithLayout(l.cfg.Layout, ev)
	if err != nil {
		return err
	}
	timestamp := generateTimestamp()
	a := LokiMsg{
		Streams: []promtailStream{{
			Stream: l.cfg.StreamLabels,
			Values: [][]string{{timestamp, string(eventBody)}},
		}},
	}
	reqBody, err := json.Marshal(a)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, l.cfg.URL, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	for k, v := range l.cfg.Headers {
		realValue, err := GetString(ev, v)
		if err != nil {
			log.Debug().Err(err).Msgf("parse template failed: %s", v)
			req.Header.Add(k, v)
		} else {
			log.Debug().Msgf("request header: {%s: %s}", k, realValue)
			req.Header.Add(k, realValue)
		}
	}

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if !(resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return errors.New("not successfull (2xx) response: " + string(body))
	}

	return nil
}

func (l *Loki) Close() {
	l.transport.CloseIdleConnections()
}
