package zapx

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap/zapcore"
)

// HTTPRequestEntry is information about the HTTP request associated with this
// log entry. See
// https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#HttpRequest
type HTTPRequestEntry struct {
	Request       *http.Request
	RequestMethod string
	RequestURL    string
	RequestSize   int64
	Status        int
	ResponseSize  int64
	UserAgent     string
	RemoteIP      string
	Referer       string
	Latency       time.Duration
}

func (e *HTTPRequestEntry) method() string {
	if e.RequestMethod != "" {
		return e.RequestMethod
	}
	if e.Request == nil {
		return ""
	}
	return e.Request.Method
}

func (e *HTTPRequestEntry) url() string {
	if e.RequestURL != "" {
		return e.RequestURL
	}
	if e.Request == nil {
		return ""
	}

	uri := e.Request.RequestURI

	// Requests using the CONNECT method over HTTP/2.0 must use
	// the authority field (aka r.Host) to identify the target.
	// Refer: https://httpwg.github.io/specs/rfc7540.html#CONNECT
	if e.Request.ProtoMajor == 2 && e.Request.Method == "CONNECT" {
		uri = e.Request.Host
	}
	if uri == "" {
		uri = e.Request.URL.RequestURI()
	}
	return uri
}

func (e *HTTPRequestEntry) userAgent() string {
	if e.UserAgent != "" {
		return e.UserAgent
	}
	if e.Request == nil {
		return ""
	}
	return e.Request.UserAgent()
}

func (e *HTTPRequestEntry) referer() string {
	if e.Referer != "" {
		return e.Referer
	}
	if e.Request == nil {
		return ""
	}
	return e.Request.Referer()
}

func (e *HTTPRequestEntry) remoteIP() string {
	if e.RemoteIP != "" {
		return e.RemoteIP
	}
	if e.Request == nil {
		return ""
	}

	if fwd := e.Request.Header.Get("X-Forwarded-For"); fwd != "" {
		s := strings.Index(fwd, ", ")
		if s == -1 {
			s = len(fwd)
		}
		return fwd[:s]
	}

	ip, _, err := net.SplitHostPort(e.Request.RemoteAddr)
	if err != nil {
		ip = e.Request.RemoteAddr
	}
	return ip
}

// MarshalLogObject is ObjectMarshaler implementation.
func (t HTTPRequestEntry) MarshalLogObject(e zapcore.ObjectEncoder) error {
	if t.RequestMethod == "" {
		t.RequestMethod = "POST"
	}
	e.AddString("requestMethod", t.method())
	addNonEmpty(e, "requestUrl", t.url())
	addNonEmpty(e, "requestSize", strconv.FormatInt(t.RequestSize, 10))
	if t.Status != 0 {
		e.AddInt("status", t.Status)
	}
	addNonEmpty(e, "responseSize", strconv.FormatInt(t.ResponseSize, 10))
	addNonEmpty(e, "userAgent", t.userAgent())
	addNonEmpty(e, "remoteIp", t.remoteIP())
	addNonEmpty(e, "referer", t.referer())
	if t.Latency != 0 {
		e.AddString("latency", fmt.Sprintf("%fs", t.Latency.Seconds()))
	}
	return nil
}

func addNonEmpty(e zapcore.ObjectEncoder, key, val string) {
	if val != "" {
		e.AddString(key, val)
	}
}
