package cucumber

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/AjdinHalac/cucumber/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rs/xid"
)

// RequestLogger returns a middleware that logs all requests on attached router
//
// By default it will log a unique "request_id", the HTTP Method of the request,
// the path that was requested, the duration (time) it took to process the
// request, the size of the response (and the "human" size), and the status
// code of the response.
func RequestLogger() HandlerFunc {
	return func(c *Context) {
		// check if we should ignore given request
		ignoreList := strings.Join(c.app.RequestLoggerIgnore, ",")
		if strings.Contains(ignoreList, c.Request.URL.Path) {
			return
		}
		start := time.Now()

		// check if request ID exists in headers
		requestID := c.RequestID()

		if requestID == "" {
			// generate new RequestID
			guid := xid.New()
			requestID = guid.String()
			// add requestID to header
			c.Request.Header.Add("X-Request-ID", requestID)
		}

		c.Response.Header().Add("X-Request-ID", requestID)

		//c.LogField("request_id", requestID)
		c.LogFields(log.Fields{
			"request_id": requestID,
		})

		//execute next handler in chain
		c.Next()

		c.LogFields(log.Fields{
			"app-version": c.app.Version,
			"status":      c.Response.Status(),
			"method":      c.Request.Method,
			"path":        c.Request.URL.String(),
			"client_ip":   c.ClientIP(),
			"duration":    time.Since(start).String(),
			"size":        c.Response.Size(),
			"human_size":  byteCountDecimal(int64(c.Response.Size())),
			"err_msg":     strings.Join(c.Errors.Errors(), ","),
		})
		c.Logger().Info("request-logger")
	}
}

// NewUnaryRequestLogger creates UnaryInterceptor that logs every request
func NewUnaryRequestLogger(opts Options) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		fullMethodString := info.FullMethod
		startTime := time.Now()
		service := path.Dir(fullMethodString)[1:]
		method := path.Base(fullMethodString)

		ignoreList := strings.Join(opts.UnaryRequestLoggerIgnore, ",")
		if strings.Contains(ignoreList, method) {
			return handler(ctx, req)
		}

		fl := opts.Logger.WithFields(
			log.Fields{
				"system":              "grpc",
				"span.kind":           "server",
				"grpc.server_version": opts.Version,
				"grpc.service":        service,
				"grpc.method":         method,
				"grpc.start_time":     startTime.Format(time.RFC3339),
			},
		)

		if d, ok := ctx.Deadline(); ok {
			fl = fl.WithFields(
				log.Fields{
					"grpc.request.deadline": d.Format(time.RFC3339),
				},
			)
		}

		newCtx := log.NewContext(ctx, fl)

		resp, err := handler(newCtx, req)

		// extract logger from context as it might have additional fields
		if l, ok := log.FromContext(newCtx); ok {
			code := status.Code(err)

			fields := log.Fields{
				"grpc.code":    code.String(),
				"grpc.time_ms": durationToMilliseconds(time.Since(startTime)),
			}

			if err != nil {
				fields["errr"] = err.Error()
			}

			l = l.WithFields(fields)

			logCode(l, code, "finished unary call with code "+code.String())
		}
		return resp, err
	}
}

func durationToMilliseconds(duration time.Duration) float32 {
	return float32(duration.Nanoseconds()/1000) / 1000
}

func logCode(l log.Logger, code codes.Code, msg string) {
	switch code {
	case codes.OK:
		l.Info(msg)
	case codes.Canceled:
		l.Info(msg)
	case codes.Unknown:
		l.Error(msg)
	case codes.InvalidArgument:
		l.Info(msg)
	case codes.DeadlineExceeded:
		l.Warn(msg)
	case codes.NotFound:
		l.Info(msg)
	case codes.AlreadyExists:
		l.Info(msg)
	case codes.PermissionDenied:
		l.Warn(msg)
	case codes.Unauthenticated:
		l.Info(msg)
	case codes.ResourceExhausted:
		l.Warn(msg)
	case codes.FailedPrecondition:
		l.Warn(msg)
	case codes.Aborted:
		l.Warn(msg)
	case codes.OutOfRange:
		l.Warn(msg)
	case codes.Unimplemented:
		l.Error(msg)
	case codes.Internal:
		l.Error(msg)
	case codes.Unavailable:
		l.Warn(msg)
	case codes.DataLoss:
		l.Error(msg)
	default:
		l.Error(msg)
	}
}
