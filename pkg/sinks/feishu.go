package sinks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/rs/zerolog/log"
)

// FeishuConfig defines the configuration for Feishu webhook
type FeishuConfig struct {
	WebhookURL string            `yaml:"webhookUrl"` // Feishu bot webhook URL
	Message    string            `yaml:"message"`    // Message text
	Title      string            `yaml:"title"`      // Message card title
	Fields     map[string]string `yaml:"fields"`     // Message fields
	Color      string            `yaml:"color"`      // Card color
}

// FeishuSink implements the Feishu receiver
type FeishuSink struct {
	cfg    *FeishuConfig
	client *http.Client
}

// NewFeishuSink creates a new Feishu sink
func NewFeishuSink(cfg *FeishuConfig) (Sink, error) {
	return &FeishuSink{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Second * 30,
		},
	}, nil
}

// Feishu message card structures
type feishuMessage struct {
	MsgType string            `json:"msg_type"`
	Card    feishuMessageCard `json:"card"`
}

type feishuMessageCard struct {
	Header   feishuCardHeader `json:"header"`
	Elements []feishuElement  `json:"elements"`
	Config   feishuConfig     `json:"config,omitempty"`
}

type feishuCardHeader struct {
	Title    feishuCardText `json:"title"`
	Template string         `json:"template,omitempty"` // Card color, options: blue, wathet, turquoise, green, yellow, orange, red, carmine, violet, purple, indigo, grey
}

type feishuCardText struct {
	Content string `json:"content"`
	Tag     string `json:"tag"`
}

type feishuElement struct {
	Tag      string         `json:"tag"`
	Text     feishuCardText `json:"text,omitempty"`
	Fields   []feishuField  `json:"fields,omitempty"`
	Elements []feishuField  `json:"elements,omitempty"`
}

type feishuField struct {
	Text    feishuCardText `json:"text,omitempty"`
	IsShort bool           `json:"is_short,omitempty"`
}

type feishuConfig struct {
	WideScreenMode bool `json:"wide_screen_mode"`
}

// Send sends a message to Feishu
func (s *FeishuSink) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	message, err := GetString(ev, s.cfg.Message)
	if err != nil {
		return err
	}

	// Build Feishu message
	msg := feishuMessage{
		MsgType: "interactive",
		Card: feishuMessageCard{
			Config: feishuConfig{
				WideScreenMode: true,
			},
		},
	}

	// Set title
	title := message
	if s.cfg.Title != "" {
		title, err = GetString(ev, s.cfg.Title)
		if err != nil {
			return err
		}
	}

	// Set title and color
	msg.Card.Header = feishuCardHeader{
		Title: feishuCardText{
			Content: title,
			Tag:     "plain_text",
		},
	}

	// Set card color
	if s.cfg.Color != "" {
		colorValue, err := GetString(ev, s.cfg.Color)
		if err != nil {
			return err
		}
		msg.Card.Header.Template = colorValue
	} else {
		// Default blue
		msg.Card.Header.Template = "blue"
	}

	// Add message text element
	msg.Card.Elements = append(msg.Card.Elements, feishuElement{
		Tag: "div",
		Text: feishuCardText{
			Content: message,
			Tag:     "plain_text",
		},
	})

	// If fields are configured, add field content
	if s.cfg.Fields != nil && len(s.cfg.Fields) > 0 {
		fields := make([]feishuField, 0)

		// Sort fields to ensure consistent order
		keys := make([]string, 0, len(s.cfg.Fields))
		for k := range s.cfg.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := s.cfg.Fields[k]
			fieldText, err := GetString(ev, v)
			if err != nil {
				return err
			}

			fields = append(fields, feishuField{
				Text: feishuCardText{
					Content: fmt.Sprintf("**%s**: %s", k, fieldText),
					Tag:     "lark_md",
				},
			})
		}

		msg.Card.Elements = append(msg.Card.Elements, feishuElement{
			Tag:      "div",
			Elements: fields,
		})
	}

	// Convert to JSON
	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("error marshaling feishu message: %w", err)
	}

	// Send POST request
	req, err := http.NewRequestWithContext(ctx, "POST", s.cfg.WebhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request to feishu: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feishu webhook returned non-200 status code: %d, body: %s", resp.StatusCode, string(body))
	}

	log.Debug().
		Str("status", resp.Status).
		Str("body", string(body)).
		Msg("Feishu webhook response")

	return nil
}

// Close closes the sink
func (f *FeishuSink) Close() {
	// No actual operation needed
}
