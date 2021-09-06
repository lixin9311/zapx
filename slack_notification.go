package zapx

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/lixin9311/backoff/v2"
	"github.com/slack-go/slack"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/grpclog"
	"gopkg.in/yaml.v2"
)

var levelColorMap = map[zapcore.Level]string{
	zapcore.DebugLevel: "#2196F3",
	zapcore.InfoLevel:  "#9E9E9E",
	zapcore.WarnLevel:  "#FF9800",
	zapcore.ErrorLevel: "#D50000",
	zapcore.FatalLevel: "#D50000",
	zapcore.PanicLevel: "#D50000",
}

type retryableError interface {
	Retryable() bool
}

type slackRetrier struct {
	bo  *backoff.Backoff
	max int
}

func (r *slackRetrier) Retry(n int, err error) (time.Duration, bool) {
	if n >= r.max {
		return 0, false
	}
	rateErr := &slack.RateLimitedError{}
	if errors.As(err, &rateErr) {
		return rateErr.RetryAfter, true
	} else if rerr, ok := err.(retryableError); ok {
		if !rerr.Retryable() {
			grpclog.Errorf("zapx: failed to post slack notification: %v", err)
			return 0, false
		}
	} // else retry
	dur := r.bo.Backoff(n)
	return dur, true
}

var defaultRetrier = &slackRetrier{max: 10}

func (s *stackdriver) sendSlackNotification(slackurl string, ent zapcore.Entry, fields []zapcore.Field) {
	defer s.slackWG.Done()
	if slackurl == "" {
		return
	}
	color, ok := levelColorMap[ent.Level]
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	enc := &slackEncoder{}
	for _, field := range fields {
		field.AddTo(enc)
	}
	enc.sort()
	head := slack.SectionBlock{
		Type: slack.MBTSection,
		Text: &slack.TextBlockObject{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*%s*\n%s", ent.Message, ent.Caller.String()),
		},
		Fields: []*slack.TextBlockObject{
			{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*%s*\n%s", "Service", s.svcCtx.Service),
			},
			{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*%s*\n%s", "Version", s.svcCtx.Version),
			},
			{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*%s*\n%s", "Time", ent.Time.Format(time.RFC3339)),
			},
		},
	}
	if enc.ErrField != nil {
		head.Fields = append(head.Fields, enc.ErrField)
	}
	attachment := slack.Attachment{
		Color: color,
		// Footer: fmt.Sprintf("reported at %s by %s"+ent.Time.Format(time.RFC3339), ent.LoggerName),
	}
	if len(enc.Fields) != 0 {
		section := slack.SectionBlock{
			Type:   slack.MBTSection,
			Fields: enc.Fields,
		}
		attachment.Blocks = slack.Blocks{
			BlockSet: []slack.Block{
				head,
				slack.NewDividerBlock(),
				section,
			},
		}
	} else {
		attachment.Blocks = slack.Blocks{
			BlockSet: []slack.Block{
				head,
			},
		}
	}

	payload := &slack.WebhookMessage{
		Attachments: []slack.Attachment{attachment},
	}

	send := func(ctx context.Context) error {
		return slack.PostWebhookContext(ctx, slackurl, payload)
	}

	if err := backoff.Invoke(ctx, send, defaultRetrier.Retry); err != nil {
		grpclog.Infof("zapx: failed to post slack notification after 10 retries: %v", err)
	}
}

type slackEncoder struct {
	Fields   []*slack.TextBlockObject
	ErrField *slack.TextBlockObject
}

func (enc *slackEncoder) sort() {
	sort.Slice(enc.Fields, func(i, j int) bool {
		return enc.Fields[i].Text < enc.Fields[j].Text
	})
}

func (enc *slackEncoder) addField(key string, field *slack.TextBlockObject) {
	if key == "error" {
		enc.ErrField = field
	} else {
		enc.Fields = append(enc.Fields, field)
	}
}

func (enc *slackEncoder) AddArray(key string, value zapcore.ArrayMarshaler) error {
	buf, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\n```%s```", key, buf),
	})
	return nil
}
func (enc *slackEncoder) AddObject(key string, value zapcore.ObjectMarshaler) error {
	if key == "serviceContext" {
		return nil
	}
	buf, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	if string(buf) == "{}\n" || string(buf) == "{}" || string(buf) == "" {
		return nil
	}
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\n```%s```", key, buf),
	})
	return nil
}

// Built-in types.
func (enc *slackEncoder) AddBinary(key string, value []byte) {
	buf := base64.StdEncoding.EncodeToString(value)
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\n%s", key, buf),
	})
}
func (enc *slackEncoder) AddByteString(key string, value []byte) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\n%s", key, value),
	})
}
func (enc *slackEncoder) AddBool(key string, value bool) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\n%t", key, value),
	})
}
func (enc *slackEncoder) AddComplex128(key string, value complex128) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\ncomplex128=%f", key, value),
	})
}
func (enc *slackEncoder) AddComplex64(key string, value complex64) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\ncomplex64=%f", key, value),
	})
}
func (enc *slackEncoder) AddDuration(key string, value time.Duration) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\n%s", key, value.String()),
	})
}
func (enc *slackEncoder) AddFloat64(key string, value float64) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nfloat64=%f", key, value),
	})
}
func (enc *slackEncoder) AddFloat32(key string, value float32) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nfloat32=%f", key, value),
	})
}
func (enc *slackEncoder) AddInt(key string, value int) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nint=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddInt64(key string, value int64) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nint64=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddInt32(key string, value int32) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nint32=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddInt16(key string, value int16) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nint16=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddInt8(key string, value int8) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nint8=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddString(key, value string) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\n%s", key, value),
	})
}
func (enc *slackEncoder) AddTime(key string, value time.Time) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\ntime=%s (%d)", key, value.Local().Format(time.RFC3339), value.Unix()),
	})
}
func (enc *slackEncoder) AddUint(key string, value uint) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nuint=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddUint64(key string, value uint64) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nuint64=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddUint32(key string, value uint32) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nuint32=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddUint16(key string, value uint16) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nuint16=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddUint8(key string, value uint8) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nuint8=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddUintptr(key string, value uintptr) {
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\nuintptr=0x%x (%d)", key, value, value),
	})
}
func (enc *slackEncoder) AddReflected(key string, value interface{}) error {
	buf, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	enc.addField(key, &slack.TextBlockObject{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*%s*\n```%s```", key, buf),
	})
	return nil
}

func (enc *slackEncoder) OpenNamespace(key string) {}
