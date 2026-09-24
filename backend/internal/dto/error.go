package dto

const (
	CodeInvalidArgument = "INVALID_ARGUMENT"
	CodeInternalError   = "INTERNAL_ERROR"
)

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

func NewInvalidArgument(field, message string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeInvalidArgument,
			Message: message,
			Field:   field,
		},
	}
}

func NewInternalError() ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeInternalError,
			Message: "服务暂时不可用",
		},
	}
}
