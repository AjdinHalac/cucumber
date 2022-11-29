package log

import "context"

type loggerKey struct{}

// FromContext extracts logger from context
func FromContext(ctx context.Context) (Logger, bool) {
	l, ok := ctx.Value(loggerKey{}).(Logger)
	return l, ok
}

// NewContext creates context with logger
func NewContext(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}
