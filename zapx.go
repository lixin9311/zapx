package zapx

import (
	"context"

	"go.opencensus.io/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var (
	RequestIDMetadataKey = "x-request-id"
)

func Label(key, val string) zapcore.Field {
	return zap.String(logKeyLabelPrefix+key, val)
}

// Context constructs a field that carries trace span & grpc method if possible.
func Context(ctx context.Context) zapcore.Field {
	span := trace.FromContext(ctx)
	sctx := span.SpanContext()
	method, _ := grpc.Method(ctx)
	info := contextInfo{
		IsSampled:  sctx.IsSampled(),
		TraceID:    sctx.TraceID.String(),
		SpanID:     sctx.SpanID.String(),
		GrpcMethod: method,
		RequestID:  extractRequestID(ctx),
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
