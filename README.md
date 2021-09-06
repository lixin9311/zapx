# Zapx [![GoDoc][godoc image]][godoc]

Zapx provides a [zap][zap] logger with [Stackdriver format][stackdriver] output.
It aims to integreate with GRPC and Opensensus.
It also comes with extra features. Such as:

- Slack Notification
- Custom Error endocer
- Extract tracing & GRPC information from context
- Stackdriver Label
- Extract Request ID from context metadata

## Usage

```shell
go get -u github.com/lixin9311/zapx
```

```go
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

```

See [https://pkg.go.dev/github.com/lixin9311/zapx][godoc] to view the documentation.

## Contributing

- I would like to keep this library as small as possible.
- Please don't send a PR without opening an issue and discussing it first.
- If proposed change is not a common use case, I will probably not accept it.

[godoc]: https://pkg.go.dev/github.com/lixin9311/zapx
[godoc image]: https://pkg.go.dev/github.com/lixin9311/zapx?status.png
[zap]: https://github.com/uber-go/zap
[stackdriver]: https://cloud.google.com/logging/docs/structured-logging
