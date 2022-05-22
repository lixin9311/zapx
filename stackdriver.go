package zapx

import (
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	logKeySlackNotification = "zapx.slack"
	logKeyContextInfo       = "zapx.context"
	logKeyLabelPrefix       = "zapx.label#"
)

type slackBehavior int

const (
	defaultSlack slackBehavior = iota
	enableSlack
	disableSlack
)

// Zap returns a zap logger configured to output logs to stdout and stderr.
func Zap(level zapcore.Level, opts ...Option) *zap.Logger {
	opt := &option{
		slackURL:  "",
		projectID: "",
		service:   "unknown",
		version:   "unknown",
	}
	for _, o := range opts {
		o(opt)
	}
	enabler := zap.NewAtomicLevel()
	enabler.SetLevel(level)
	stdout := zapcore.Lock(os.Stdout)
	enc := zapcore.NewJSONEncoder(StackdriverEncoderConfig)
	core := zapcore.NewCore(enc, stdout, enabler)
	logger := zap.New(core, zap.AddCaller())
	logger = logger.Named(opt.service)
	return logger.WithOptions(zap.WrapCore(
		func(core zapcore.Core) zapcore.Core {
			return &stackdriver{
				projectID:   opt.projectID,
				parent:      core,
				svcCtx:      serviceContext{Service: opt.service, Version: opt.version},
				slackURL:    opt.slackURL,
				errorPraser: opt.errorParser,
			}
		},
	))
}

// StackdriverEncoderConfig is a encoder config for stackdriver.
var StackdriverEncoderConfig = zapcore.EncoderConfig{
	MessageKey:    "message",
	LevelKey:      "severity",
	TimeKey:       "eventTime",
	NameKey:       "logger",
	CallerKey:     "caller",
	StacktraceKey: "stacktrace",
	LineEnding:    zapcore.DefaultLineEnding,
	EncodeLevel: func(lv zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		var s string
		switch lv {
		case zapcore.DebugLevel:
			s = "DEBUG"
		case zapcore.InfoLevel:
			s = "INFO"
		case zapcore.WarnLevel:
			s = "WARNING"
		case zapcore.ErrorLevel:
			s = "ERROR"
		case zapcore.DPanicLevel:
			s = "CRITICAL"
		case zapcore.PanicLevel:
			s = "ALERT"
		case zapcore.FatalLevel:
			s = "EMERGENCY"
		}

		enc.AppendString(s)
	},
	EncodeTime:     zapcore.ISO8601TimeEncoder,
	EncodeDuration: zapcore.SecondsDurationEncoder,
	EncodeCaller:   zapcore.ShortCallerEncoder,
}

type contextInfo struct {
	IsSampled  bool
	TraceID    string
	SpanID     string
	GrpcMethod string
	RequestID  string
}

// serviceContext is the service context for which this error was reported.
type serviceContext struct {
	Service string
	Version string
}

// MarshalLogObject is ObjectMarshaler implementation.
func (s serviceContext) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddString("service", s.Service)
	e.AddString("version", s.Version)
	return nil
}

// stackdriver implements zapcore.Core which alters logger to output logs in
// the form of stackdriver understands. See
// https://cloud.google.com/error-reporting/docs/formatting-error-messages for
// the format details.
type stackdriver struct {
	projectID   string
	parent      zapcore.Core
	svcCtx      serviceContext
	slackURL    string
	errorPraser func(error) (zapcore.ObjectMarshaler, bool)
	slackWG     sync.WaitGroup

	enableSlack bool
	user        string
	fields      []zapcore.Field
}

func (s *stackdriver) Enabled(l zapcore.Level) bool {
	return s.parent.Enabled(l)
}

func (s *stackdriver) With(fields []zapcore.Field) zapcore.Core {
	fs, user, sendSlack, slackURL := s.parseFields(fields)
	newFileds := make([]zapcore.Field, len(fs)+len(s.fields))

	if user == "" {
		user = s.user
	}
	copy(newFileds, s.fields)
	copy(newFileds[len(s.fields):], fs)

	news := &stackdriver{
		parent:      s.parent,
		projectID:   s.projectID,
		svcCtx:      s.svcCtx,
		slackURL:    s.slackURL,
		errorPraser: s.errorPraser,

		user:   user,
		fields: newFileds,
	}

	if slackURL != "" {
		news.slackURL = slackURL
	}
	if sendSlack == disableSlack {
		news.enableSlack = false
	} else if sendSlack == enableSlack {
		news.enableSlack = true
	}

	return news
}

func (s *stackdriver) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if s.Enabled(ent.Level) {
		return ce.AddCore(ent, s)
	}
	return ce
}

func (s *stackdriver) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	if ent.LoggerName != "" && ent.LoggerName != "unknown" {
		ent.Message = ent.LoggerName + ": " + ent.Message
	}
	rloc := reportLocationFromEntry(ent)
	sloc := sourceLocationFromEntry(ent)
	fs := fields

	fs, user, sendSlack, slackURL := s.parseFields(fs, ent.Message)
	fs = append(fs, s.fields...)
	if user == "" {
		user = s.user
	}
	fs = append(fs, zap.Object("logging.googleapis.com/sourceLocation", sloc), zap.Object("serviceContext", s.svcCtx), zap.Object("context", errorReportingContext{reportLocation: rloc, user: user}))
	if sendSlack == enableSlack || (sendSlack == defaultSlack && s.enableSlack) {
		s.slackWG.Add(1)
		go s.sendSlackNotification(slackURL, ent, fs)
	}
	return s.parent.Write(ent, fs)
}

func (s *stackdriver) Sync() error {
	s.slackWG.Wait()
	return s.parent.Sync()
}

func (s *stackdriver) parseFields(fields []zapcore.Field, msg ...string) (fs []zapcore.Field, user string, sendSlack slackBehavior, slackURL string) {
	labels := labels([]zap.Field{})
	for _, f := range fields {
		if strings.HasPrefix(f.Key, logKeyLabelPrefix) {
			key := strings.TrimPrefix(f.Key, logKeyLabelPrefix)
			val := f.String
			labels = append(labels, zap.String(key, val))
			continue
		}
		switch f.Key {

		case "user":
			if f.Type == zapcore.StringType {
				user = f.String
			}
		case "stack_trace":
			if f.Type == zapcore.StringType && len(msg) > 0 {
				f.String = msg[0] + "\n" + f.String
			}
			fs = append(fs, f)
		case logKeyContextInfo:
			if info, ok := f.Interface.(contextInfo); ok {
				if info.IsSampled {
					fs = append(fs,
						zap.Bool("logging.googleapis.com/trace_sampled", true),
						zap.String("logging.googleapis.com/trace", info.TraceID),
						zap.String("logging.googleapis.com/spanId", info.SpanID),
					)
				}
				if info.GrpcMethod != "" {
					fs = append(fs, zap.String("grpc_method", info.GrpcMethod))
				}
				if info.RequestID != "" {
					fs = append(fs, zap.String("request_id", info.RequestID))
				}
			}

		case logKeySlackNotification:
			if f.Type == zapcore.BoolType {
				if f.Integer == 1 {
					sendSlack = enableSlack
					slackURL = s.slackURL
				} else {
					sendSlack = disableSlack
				}
			} else if f.Type == zapcore.StringType {
				sendSlack = enableSlack
				slackURL = f.String
			}
		default:
			// customize error parsing
			if s.errorPraser != nil && f.Type == zapcore.ErrorType {
				if err, ok := f.Interface.(error); ok {
					if obj, ok := s.errorPraser(err); ok {
						fs = append(fs, zap.Object(f.Key, obj))
						break
					}
				}
			}
			fs = append(fs, f)
		}
	}
	if len(labels) != 0 {
		fs = append(fs, zap.Object("logging.googleapis.com/labels", labels))
	}
	return fs, user, sendSlack, slackURL
}
