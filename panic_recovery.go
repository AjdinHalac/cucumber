package cucumber

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PanicRecovery returns a middleware that recovers from any panics and serves error response
func PanicRecovery() HandlerFunc {
	return func(c *Context) {
		defer func() {
			if err := recover(); err != nil {

				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				if brokenPipe {
					c.Error(err.(error))
					c.Abort()
				} else {
					c.ServeError(http.StatusInternalServerError, err.(error))
				}
			}
		}()
		c.Next()
	}
}

// NewUnaryPanicRecovery creates  interceptor to protect a process from aborting by panic and return Internal error as status code
func NewUnaryPanicRecovery(opts Options) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = status.Errorf(codes.Internal, "panic: %v", r)
			}
		}()

		return handler(ctx, req)
	}
}
