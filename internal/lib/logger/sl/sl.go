package sl

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

const (
	RequestIdLogKey = "request_id"
	OpLogKey        = "operation"
	ErrorLogKey     = "error"
)

func Err(err error) slog.Attr {
	return slog.Attr{
		Key:   ErrorLogKey,
		Value: slog.StringValue(err.Error()),
	}
}

func Op(op string) slog.Attr {
	return slog.Attr{
		Key:   OpLogKey,
		Value: slog.StringValue(op),
	}
}

func ReqId(r *http.Request) slog.Attr {
	reqId := middleware.GetReqID(r.Context())

	return slog.Attr{
		Key:   RequestIdLogKey,
		Value: slog.StringValue(reqId),
	}
}
