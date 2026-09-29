package dto

const (
	CodeInvalidArgument    = "INVALID_ARGUMENT"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeNotFound           = "NOT_FOUND"
	CodeInternalError      = "INTERNAL_ERROR"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	CodeEmailNotVerified   = "EMAIL_NOT_VERIFIED"
	CodeAccountDisabled    = "ACCOUNT_DISABLED"
	CodeInvalidToken       = "INVALID_TOKEN"
	CodeTooManyRequests    = "TOO_MANY_REQUESTS"
	CodeConflict           = "CONFLICT"
	CodePayloadTooLarge    = "PAYLOAD_TOO_LARGE"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
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

// 未登录、会话不存在、会话过期、账号被停用，返回的都是这一个响应体——没有参数，
// 就没法不小心让它们长得不一样。区分它们等于告诉探测者他手上的令牌是否真的存在过。
func NewUnauthorized() ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeUnauthorized,
			Message: "请先登录",
		},
	}
}

func NewForbidden(message string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeForbidden,
			Message: message,
		},
	}
}

func NewNotFound(message string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeNotFound,
			Message: message,
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

// 「邮箱不存在」和「口令不对」共用这一个响应体，理由和 NewUnauthorized 一样：
// 区分它们等于给探测者一个查邮箱是否注册过的接口。
func NewInvalidCredentials() ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeInvalidCredentials,
			Message: "邮箱或口令不正确",
		},
	}
}

// 这个响应只可能发给口令已经输对的人，所以它说出「这个邮箱存在」不算泄露——
// 能走到这一步的人本来就知道。
func NewEmailNotVerified() ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeEmailNotVerified,
			Message: "邮箱尚未验证，请先查收验证邮件",
		},
	}
}

func NewAccountDisabled() ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeAccountDisabled,
			Message: "账号已停用",
		},
	}
}

// 验证邮件和重置口令的链接失效时用。不再区分「不存在」「过期」「已用过」——
// 对用户来说要做的事完全一样：重新申请一封。
func NewInvalidToken() ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeInvalidToken,
			Message: "链接无效或已过期，请重新申请",
		},
	}
}

func NewTooManyRequests(message string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeTooManyRequests,
			Message: message,
		},
	}
}

// 请求本身没毛病，只是和已有的数据撞了——比如 slug 已经被占。带上 field
// 是因为这类冲突总能指到表单里的某一格。
func NewConflict(field, message string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeConflict,
			Message: message,
			Field:   field,
		},
	}
}

// 和 INVALID_ARGUMENT 分开，是因为用户要做的事不一样：格式错了要改内容，
// 体积超了要换文件。混在一个码里，前端只能去解析那句提示才知道该说什么。
func NewPayloadTooLarge(message string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodePayloadTooLarge,
			Message: message,
		},
	}
}

// 服务端缺一步配置，重试不会变好，但原因说得出。和 INTERNAL_ERROR 分开是为了让
// 界面能把「这台机器还没配好」和「这次请求本身坏了」分开讲——前者有明确的下一步，
// 后者只能请人稍后再来。
func NewServiceUnavailable(message string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:    CodeServiceUnavailable,
			Message: message,
		},
	}
}
