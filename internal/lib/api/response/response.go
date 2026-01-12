package response

type Response struct {
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

const (
	StatusOK = iota
	StatusBadRequest
	StatusUnauthorized
	StatusForbidden
	StatusInternalError
)

func OK() Response {
	return Response{
		Status: StatusOK,
	}
}

func BadRequest(msg string) Response {
	return Response{
		Status: StatusBadRequest,
		Error:  msg,
	}
}

func Unauthorized(msg string) Response {
	if msg == "" {
		return Response{
			Status: StatusUnauthorized,
			Error:  "Unauthorized",
		}
	}
	return Response{
		Status: StatusUnauthorized,
		Error:  "Unauthorized: " + msg,
	}
}

func Forbidden(msg string) Response {
	if msg == "" {
		return Response{
			Status: StatusForbidden,
			Error:  "Forbidden",
		}
	}
	return Response{
		Status: StatusForbidden,
		Error:  "Forbidden: " + msg,
	}
}

func InternalError() Response {
	return Response{
		Status: StatusInternalError,
		Error:  "Internal error",
	}
}
