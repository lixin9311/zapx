package zapx

import (
	"go.uber.org/zap/zapcore"
)

type option struct {
	slackURL    string
	projectID   string
	service     string
	version     string
	errorParser func(error) (zapcore.ObjectMarshaler, bool)
}

type Option func(*option)

// WithSlackURL sets the slack hook url
func WithSlackURL(url string) Option {
	return func(o *option) {
		o.slackURL = url
	}
}

func WithProjectID(id string) Option {
	return func(o *option) {
		o.projectID = id
	}
}

func WithService(name string) Option {
	return func(o *option) {
		o.service = name
	}
}

func WithVersion(ver string) Option {
	return func(o *option) {
		o.version = ver
	}
}

func WithErrorParser(parser func(error) (zapcore.ObjectMarshaler, bool)) Option {
	return func(o *option) {
		o.errorParser = parser
	}
}
