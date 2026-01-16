package connect

import "errors"

var (
	ErrConnectionStopped    = errors.New("connection stopped")
	ErrMissingRequiredScope = errors.New("missing required scope")
)
