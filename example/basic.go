package main

import (
	"errors"
	"time"

	"github.com/lixin9311/zapx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type customError struct {
	err error
}

// MarshalLogObject implements zap.ObjectMarshaler
func (e *customError) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if e == nil {
		enc.AddString("error", "nil")
		return nil
	}
	enc.AddString("error", e.err.Error())
	enc.AddString("message", "custom message")
	return nil
}

func main() {
	logger := zapx.Zap(zapcore.DebugLevel,
		zapx.WithProjectID("example-project"),
		zapx.WithService("example-service"),
		zapx.WithSlackURL("https://slack-webhook"),
		zapx.WithVersion("v0.0.1"),
		zapx.WithErrorParser(func(e error) (zapcore.ObjectMarshaler, bool) {
			return &customError{e}, true
		}),
	)

	zap.ReplaceGlobals(logger)

	// {
	// 	"severity":"DEBUG",
	// 	"eventTime":"2021-09-06T07:10:52.122Z",
	// 	"logger":"example-service",
	// 	"caller":"example/basic.go:64",
	// 	"message":"example-service: hello world",
	// 	"error":{
	// 	  "error":"example error",
	// 	  "message":"custom message"
	// 	},
	// 	"httpRequest":{
	// 	  "requestMethod":"GET",
	// 	  "requestUrl":"http://example.com",
	// 	  "requestSize":"1024",
	// 	  "status":200,
	// 	  "responseSize":"0",
	// 	  "userAgent":"curl",
	// 	  "remoteIp":"8.8.8.8",
	// 	  "referer":"http://example.com",
	// 	  "latency":"1.000000s"
	// 	},
	// 	"logging.googleapis.com/sourceLocation":{
	// 	  "filePath":"example/basic.go:64",
	// 	  "lineNumber":64,
	// 	  "functionName":"main.main"
	// 	},
	// 	"logging.googleapis.com/labels":{
	// 	  "foo-label":"foo",
	// 	  "bar-label":"bar"
	// 	},
	// 	"serviceContext":{
	// 	  "service":"example-service",
	// 	  "version":"v0.0.1"
	// 	}
	// }
	zap.L().Debug("hello world", zap.Error(errors.New("example error")),
		zapx.Label("foo-label", "foo"), zapx.Label("bar-label", "bar"),
		zapx.Request(zapx.HTTPRequestEntry{
			RequestMethod: "GET",
			RequestURL:    "http://example.com",
			Status:        200,
			RequestSize:   1024,
			UserAgent:     "curl",
			RemoteIP:      "8.8.8.8",
			Referer:       "http://example.com",
			Latency:       time.Second,
		}),
	)
}
