package hah

type successStatusOption struct {
	status int
}

func (o successStatusOption) applyScope(cfg *scopeConfig) {
	if o.status != 0 {
		cfg.success.status = o.status
	}
}

// SuccessStatus sets the default HTTP success status for Render,
// RenderWithMeta, and RenderEmpty(..., 0) calls inside a WithResponses
// subtree.
// An explicit Status(r, status) call still overrides this default.
func SuccessStatus(status int) ScopeOption {
	return successStatusOption{status: status}
}
