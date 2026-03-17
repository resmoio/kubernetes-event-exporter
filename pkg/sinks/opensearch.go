package sinks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/opensearch-project/opensearch-go/v3"
	"github.com/opensearch-project/opensearch-go/v3/opensearchapi"
	requestsigner "github.com/opensearch-project/opensearch-go/v3/signer/aws"
	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
)

type OpenSearchConfig struct {
	// Connection specific
	Hosts    []string `yaml:"hosts"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	// Indexing preferences
	UseEventID bool `yaml:"useEventID"`
	// DeDot all labels and annotations in the event. For both the event and the involvedObject
	DeDot       bool                   `yaml:"deDot"`
	Index       string                 `yaml:"index"`
	IndexFormat string                 `yaml:"indexFormat"`
	Type        string                 `yaml:"type"`
	TLS         TLS                    `yaml:"tls"`
	Layout      map[string]interface{} `yaml:"layout"`
	AWSService  string                 `yaml:"awsService"` // Leave blank to not do AWS SigV4 signing. Otherwise, set to "es" for Amazon OpenSearch or "aoss" for Amazon OpenSearch Serverless.
}

func NewOpenSearch(cfg *OpenSearchConfig) (*OpenSearch, error) {
	tlsClientConfig, err := setupTLS(&cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("failed to setup TLS: %w", err)
	}
	openSearchConfig := opensearch.Config{
		Addresses: cfg.Hosts,
		Username:  cfg.Username,
		Password:  cfg.Password,
		Transport: &http.Transport{
			TLSClientConfig: tlsClientConfig,
		},
	}
	if cfg.AWSService != "" {
		// Enable AWS SigV4 signing for OpenSearch requests
		signer, err := requestsigner.NewSignerWithService(
			session.Options{SharedConfigState: session.SharedConfigEnable},
			cfg.AWSService,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize AWS signer: %w", err)
		}
		openSearchConfig.Signer = signer
	}
	client, err := opensearchapi.NewClient(opensearchapi.Config{Client: openSearchConfig})
	if err != nil {
		return nil, err
	}

	return &OpenSearch{
		client: client,
		cfg:    cfg,
	}, nil
}

type OpenSearch struct {
	client *opensearchapi.Client
	cfg    *OpenSearchConfig
}

var osRegex = regexp.MustCompile(`(?s){(.*)}`)

func osFormatIndexName(pattern string, when time.Time) string {
	m := osRegex.FindAllStringSubmatchIndex(pattern, -1)
	current := 0
	var builder strings.Builder

	for i := 0; i < len(m); i++ {
		pair := m[i]

		builder.WriteString(pattern[current:pair[0]])
		builder.WriteString(when.Format(pattern[pair[0]+1 : pair[1]-1]))
		current = pair[1]
	}

	builder.WriteString(pattern[current:])

	return builder.String()
}

func (e *OpenSearch) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	var toSend []byte

	if e.cfg.DeDot {
		de := ev.DeDot()
		ev = &de
	}
	if e.cfg.Layout != nil {
		res, err := convertLayoutTemplate(e.cfg.Layout, ev)
		if err != nil {
			return err
		}

		toSend, err = json.Marshal(res)
		if err != nil {
			return err
		}
	} else {
		toSend = ev.ToJSON()
	}

	var index string
	if len(e.cfg.IndexFormat) > 0 {
		now := time.Now()
		index = osFormatIndexName(e.cfg.IndexFormat, now)
	} else {
		index = e.cfg.Index
	}

	req := opensearchapi.IndexReq{
		Body:  bytes.NewBuffer(toSend),
		Index: index,
	}

	if e.cfg.UseEventID {
		req.DocumentID = string(ev.UID)
	}

	_, err := e.client.Index(ctx, req)
	return err
}

func (e *OpenSearch) Close() {
	// No-op
}
