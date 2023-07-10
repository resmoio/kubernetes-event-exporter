package sinks

import (
	"context"
	"sort"

	"github.com/resmoio/kubernetes-event-exporter/pkg/kube"
	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
)

type SlackConfig struct {
	Token      string            `yaml:"token"`
	Channel    string            `yaml:"channel"`
	Message    string            `yaml:"message"`
	Color      string            `yaml:"color"`
	Footer     string            `yaml:"footer"`
	Title      string            `yaml:"title"`
	AuthorName string            `yaml:"author_name"`
	Fields     map[string]string `yaml:"fields"`
}

type SlackSink struct {
	cfg    *SlackConfig
	client *slack.Client
}

func NewSlackSink(cfg *SlackConfig) (Sink, error) {
	return &SlackSink{
		cfg:    cfg,
		client: slack.New(cfg.Token),
	}, nil
}

func (s *SlackSink) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	channel, err := GetString(ev, s.cfg.Channel)
	if err != nil {
		return err
	}

	message, err := GetString(ev, s.cfg.Message)
	if err != nil {
		return err
	}

	options := []slack.MsgOption{slack.MsgOptionText(message, true)}
	if s.cfg.Fields != nil {
		fields := make([]slack.AttachmentField, 0)
		for k, v := range s.cfg.Fields {
			fieldText, err := GetString(ev, v)
			if err != nil {
				return err
			}

			fields = append(fields, slack.AttachmentField{
				Title: k,
				Value: fieldText,
				Short: false,
			})
		}

		sort.SliceStable(fields, func(i, j int) bool {
			return fields[i].Title < fields[j].Title
		})

		// make slack attachment
		slackAttachment := slack.Attachment{}
		slackAttachment.Fields = fields
		if s.cfg.AuthorName != "" {
			slackAttachment.AuthorName, err = GetString(ev, s.cfg.AuthorName)
			if err != nil {
				return err
			}
		}
		if s.cfg.Color != "" {
			slackAttachment.Color = s.cfg.Color
		}
		if s.cfg.Title != "" {
			slackAttachment.Title, err = GetString(ev, s.cfg.Title)
			if err != nil {
				return err
			}
		}
		if s.cfg.Footer != "" {
			slackAttachment.Footer, err = GetString(ev, s.cfg.Footer)
			if err != nil {
				return err
			}
		}

		options = append(options, slack.MsgOptionAttachments(slackAttachment))
	}

	_ch, _ts, _text, err := s.client.SendMessageContext(ctx, channel, options...)
	log.Debug().Str("ch", _ch).Str("ts", _ts).Str("text", _text).Err(err).Msg("Slack Response")
	return err
}

func (s *SlackSink) Close() {
	// No-op
}
