package reqx

const DefaultMaxBodyBytes int64 = 1 << 20

type bindBodyConfig struct {
	maxBodyBytes       int64
	allowUnknownFields bool
	allowEmptyBody     bool
}

type bindValuesConfig struct {
	allowUnknownFields bool
}

type bindConfig struct {
	body   bindBodyConfig
	query  bindValuesConfig
	header bindValuesConfig
}

// BindOption 自定义统一绑定行为。
type BindOption func(*bindConfig)

// BindBodyOption 自定义 body 绑定行为。
type BindBodyOption = BindOption

// BindQueryParamsOption 自定义 query 绑定行为。
type BindQueryParamsOption = BindOption

// BindHeadersOption 自定义 header 绑定行为。
type BindHeadersOption = BindOption

func defaultBindConfig() bindConfig {
	return bindConfig{
		body: bindBodyConfig{
			maxBodyBytes:       DefaultMaxBodyBytes,
			allowUnknownFields: true,
			allowEmptyBody:     true,
		},
		query: bindValuesConfig{
			allowUnknownFields: true,
		},
		header: bindValuesConfig{
			allowUnknownFields: true,
		},
	}
}

func applyBindOptions(opts ...BindOption) bindConfig {
	cfg := defaultBindConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

func WithMaxBodyBytes(limit int64) BindOption {
	return func(cfg *bindConfig) {
		if limit > 0 {
			cfg.body.maxBodyBytes = limit
		}
	}
}
