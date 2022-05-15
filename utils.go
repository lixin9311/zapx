package zapx

import (
	"context"

	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/metadata"
)

func reportLocationFromEntry(ent zapcore.Entry) reportLocation {
	caller := ent.Caller

	if !caller.Defined {
		return reportLocation{}
	}
	loc := reportLocation{
		filePath:     caller.TrimmedPath(),
		lineNumber:   caller.Line,
		functionName: caller.Function,
	}

	return loc
}

func sourceLocationFromEntry(ent zapcore.Entry) sourceLocation {
	caller := ent.Caller

	if !caller.Defined {
		return sourceLocation{}
	}
	loc := sourceLocation{
		file:     caller.TrimmedPath(),
		line:     caller.Line,
		function: caller.Function,
	}

	return loc
}

func extractRequestID(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		reqIDs, ok := md[RequestIDMetadataKey]
		if ok && len(reqIDs) > 0 {
			return reqIDs[0]
		}
	}
	return ""
}
