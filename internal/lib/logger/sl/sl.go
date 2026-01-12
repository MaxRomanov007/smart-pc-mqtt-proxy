package sl

import "log/slog"

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
