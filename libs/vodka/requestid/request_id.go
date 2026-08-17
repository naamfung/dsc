package requestid

import (
	"dsc/libs/vodka"
	"dsc/libs/vodka/libraries/gommon/random"
	"dsc/libs/vodka/skipper"
)

type (
	// RequestIDConfig defines the config for RequestID middleware.
	RequestIDConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper skipper.Skipper

		// Generator defines a function to generate an ID.
		// Optional. Default value random.String(32).
		Generator func() string
	}
)

var (
	// DefaultRequestIDConfig is the default RequestID middleware config.
	DefaultRequestIDConfig = RequestIDConfig{
		Skipper:   skipper.DefaultSkipper,
		Generator: generator,
	}
)

// RequestID returns a X-Request-ID middleware.
func RequestID() vodka.Handler {
	return RequestIDWithConfig(DefaultRequestIDConfig)
}

// RequestIDWithConfig returns a X-Request-ID middleware with config.
func RequestIDWithConfig(config RequestIDConfig) vodka.Handler {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultRequestIDConfig.Skipper
	}
	if config.Generator == nil {
		config.Generator = generator
	}

	return func(c *vodka.Context) error {
		if config.Skipper(c) {
			return c.Next()
		}

		req := c.Request
		res := c.Response
		rid := req.Header.Get(vodka.HeaderXRequestID)
		if rid == "" {
			rid = config.Generator()
		}
		res.Header().Set(vodka.HeaderXRequestID, rid)

		return c.Next()
	}

}

func generator() string {
	return random.String(32)
}
