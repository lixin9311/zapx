package zapx

import (
	"context"
	"strings"

	"go.opencensus.io/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	RequestIDMetadataKey = "x-request-id"

	protomarshaler = protojson.MarshalOptions{UseProtoNames: true}
)

func Label(key, val string) zapcore.Field {
	return zap.String(logKeyLabelPrefix+key, val)
}

func Slack(url ...string) zapcore.Field {
	if len(url) > 0 {
		return zap.String(logKeySlackNotification, url[0])
	}
	return zap.Bool(logKeySlackNotification, true)
}

type jsonpbObjectMarshaler struct {
	pb proto.Message
}

func (j *jsonpbObjectMarshaler) MarshalJSON() ([]byte, error) {
	return protomarshaler.Marshal(j.pb)
}

func Proto(key string, val proto.Message) zapcore.Field {
	return zap.Reflect(key, &jsonpbObjectMarshaler{pb: val})
}

// Context constructs a field that carries trace span & grpc method if possible.
func Context(ctx context.Context) zapcore.Field {
	var info contextInfo
	method, _ := grpc.Method(ctx)
	info.GrpcMethod = method
	info.RequestID = extractRequestID(ctx)

	if span := trace.FromContext(ctx); span != nil || !span.SpanContext().IsSampled() {
		sctx := span.SpanContext()
		info.IsSampled = sctx.IsSampled()
		info.TraceID = sctx.TraceID.String()
		info.SpanID = sctx.SpanID.String()
	} else {
		// try x-cloud-trace-context header
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if cloudTraceHeader := md.Get("x-cloud-trace-context"); len(cloudTraceHeader) > 0 {
				h := cloudTraceHeader[0]
				slash := strings.Index(h, `/`)
				if slash != -1 {
					tid, h := h[:slash], h[slash+1:]
					info.TraceID = tid
					// Parse the span id field.
					spanstr := h
					semicolon := strings.Index(h, `;`)
					if semicolon != -1 {
						spanstr, h = h[:semicolon], h[semicolon+1:]
					}
					info.SpanID = spanstr
					if strings.HasPrefix(h, "o=1") {
						info.IsSampled = true
					}
				}
			}
		}
	}

	return zap.Reflect(logKeyContextInfo, info)
}

func Request(req HTTPRequestEntry) zapcore.Field {
	return zap.Object("httpRequest", req)
}

// Metadata constructs a field that carries metadata from context.
func Metadata(ctx context.Context) zapcore.Field {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return zap.Skip()
	}

	return zap.Object("metadata", wmetadata(md))
}

type wmetadata metadata.MD

// MarshalLogObject is ObjectMarshaler implementation.
func (md wmetadata) MarshalLogObject(e zapcore.ObjectEncoder) error {
	for key, vals := range md {
		zap.Strings(key, vals).AddTo(e)
	}
	return nil
}

type errorReportingContext struct {
	reportLocation reportLocation
	user           string
}

func (r errorReportingContext) MarshalLogObject(e zapcore.ObjectEncoder) error {
	if r.user != "" {
		e.AddString("user", r.user)
	}
	e.AddObject("reportLocation", r.reportLocation)
	return nil
}

// sourceLocation is the location in the source code where the decision was
// made to report the error, usually the place where it was logged. For a
// logged exception this would be the source line where the exception is
// logged, usually close to the place where it was caught.
type sourceLocation struct {
	file     string
	line     int
	function string
}

// MarshalLogObject is ObjectMarshaler implementation.
func (r sourceLocation) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddString("file", r.file)
	e.AddInt("line", r.line)
	e.AddString("function", r.function)
	return nil
}

// reportLocation is the location in the source code where the decision was
// made to report the error, usually the place where it was logged. For a
// logged exception this would be the source line where the exception is
// logged, usually close to the place where it was caught.
type reportLocation struct {
	filePath     string
	lineNumber   int
	functionName string
}

// MarshalLogObject is ObjectMarshaler implementation.
func (r reportLocation) MarshalLogObject(e zapcore.ObjectEncoder) error {
	e.AddString("filePath", r.filePath)
	e.AddInt("lineNumber", r.lineNumber)
	e.AddString("functionName", r.functionName)
	return nil
}

type labels []zap.Field

func (r labels) MarshalLogObject(e zapcore.ObjectEncoder) error {
	for _, f := range r {
		f.AddTo(e)
	}
	return nil
}
