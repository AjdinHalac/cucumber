package cucumber

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/AjdinHalac/cucumber/di"
	"go.elastic.co/apm/module/apmgrpc"
	"go.elastic.co/apm/module/apmhttp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	ctrlVerRegex = regexp.MustCompile(`V[0-9]`)
)

// App holds fully working application setup
type App struct {
	Options
	container di.Container

	server *grpc.Server
	router *Router
	pool   sync.Pool

	methodNotAllowedHandler HandlerFunc
	unauthorizedHandler     HandlerFunc
	notFoundHandler         HandlerFunc
	errorHandler            HandlerFunc
}

// New returns an App instance with default configuration.
func New() *App {
	return NewWithOptions(NewOptions())
}

// NewWithOptions creates new application instance
// with given Application Options object
func NewWithOptions(opts Options) *App {

	opts = optionsWithDefault(opts)

	// create application router
	r := NewRouter()

	if opts.UseRequestLogger {
		r.Use(RequestLogger())
		opts.UnaryInterceptors = append(opts.UnaryInterceptors, NewUnaryRequestLogger(opts))
	}

	if opts.UsePanicRecovery {
		r.Use(PanicRecovery())
		opts.UnaryInterceptors = append(opts.UnaryInterceptors, NewUnaryPanicRecovery(opts))
	}

	if opts.ServeStatic {
		r.Static(opts.StaticPath, opts.StaticDir)
	}

	srvOpts := []grpc.ServerOption{}
	opts.UnaryInterceptors = append(opts.UnaryInterceptors, apmgrpc.NewUnaryServerInterceptor())
	srvOpts = append(srvOpts, grpc.UnaryInterceptor(ChainUnaryServer(opts.UnaryInterceptors...)))
	srvOpts = append(srvOpts, grpc.StreamInterceptor(apmgrpc.NewStreamServerInterceptor()))

	grpcServer := grpc.NewServer(srvOpts...)

	reflection.Register(grpcServer)

	app := &App{
		Options:   opts,
		router:    r,
		container: di.NewContainer(),
		server:    grpcServer,
	}

	//context pool allocation
	app.pool.New = func() interface{} {
		return app.allocateContext()
	}

	return app
}

// Use appends one or more middlewares onto the Router stack.
func (a *App) Use(middleware ...HandlerFunc) *App {
	a.router.Use(middleware...)
	return a
}

// GET is a shortcut for router.Handle("GET", path, handle)
func (a *App) GET(path string, handler ...HandlerFunc) *App {
	a.router.GET(path, handler...)
	return a
}

// HEAD is a shortcut for router.Handle("HEAD", path, handle)
func (a *App) HEAD(path string, handler ...HandlerFunc) *App {
	a.router.HEAD(path, handler...)
	return a
}

// OPTIONS is a shortcut for router.Handle("OPTIONS", path, handle)
func (a *App) OPTIONS(path string, handler ...HandlerFunc) *App {
	a.router.OPTIONS(path, handler...)
	return a
}

// POST is a shortcut for router.Handle("POST", path, handle)
func (a *App) POST(path string, handler ...HandlerFunc) *App {
	a.router.POST(path, handler...)
	return a
}

// PUT is a shortcut for router.Handle("PUT", path, handle)
func (a *App) PUT(path string, handler HandlerFunc) *App {
	a.router.PUT(path, handler)
	return a
}

// PATCH is a shortcut for router.Handle("PATCH", path, handle)
func (a *App) PATCH(path string, handler ...HandlerFunc) *App {
	a.router.PATCH(path, handler...)
	return a
}

// DELETE is a shortcut for router.Handle("DELETE", path, handle)
func (a *App) DELETE(path string, handler ...HandlerFunc) *App {
	a.router.DELETE(path, handler...)
	return a
}

// Any registers a route that matches all the HTTP methods.
// GET, POST, PUT, PATCH, HEAD, OPTIONS, DELETE, CONNECT, TRACE.
func (a *App) Any(relativePath string, handler ...HandlerFunc) *App {
	a.router.Any(relativePath, handler...)
	return a
}

// Attach another router to current one
func (a *App) Attach(prefix string, router *Router) *App {
	a.router.Attach(prefix, router)
	return a
}

// Register appends one or more values as dependecies
func (a *App) RegisterPackage(value interface{}) *App {
	a.container.Add(value)
	return a
}

// Register appends one or more values as dependecies
func (a *App) Register(value interface{}) *App {

	typ := reflect.TypeOf(value)
	fullSvcName := typ.String()

	if typ.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("Service `%s` has to be pointer", fullSvcName))
	}

	if _, ok := value.(Autowired); ok {
		if a.container.Len() != 0 {
			a.InjectDeps(value)
		}
	}

	if _, ok := value.(Service); ok {
		a.container.Add(value)
	}

	if i, ok := value.(Initer); ok {
		i.Init(a)
	}

	return a
}

// InjectDeps accepts a destination struct and any optional context value(s),
// and injects registered dependencies to the destination object
func (a *App) InjectDeps(dest interface{}, ctx ...reflect.Value) {
	injector := di.Struct(dest, a.container...)
	injector.Inject(dest, ctx...)
}

// RegisterServiceHandler registers a service and its implementation to the gRPC
// server. This must be called before invoking Serve.
func (a *App) RegisterServiceHandler(service interface{}) *App {
	a.Register(service)
	svcProtoRegister, ok := service.(ServiceProtoRegister)
	if !ok {
		panic("Service does not implement ServiceProtoRegister interface")
	}
	svcProtoRegister.RegisterProtoServer(a.server)
	return a
}

// RegisterController registers application controller
func (a *App) RegisterController(ctrl interface{}) *App {

	// set controller route prefix to default
	prefix := "/"
	// set controller version to default
	version := ""

	// check naming convention
	typ := reflect.TypeOf(ctrl)

	// get full controller full name
	fullCtrlName := typ.String()

	// check if controller is pointer
	if typ.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("Controller `%s` has to be pointer", fullCtrlName))
	}
	// remove * from full name
	fullCtrlName = fullCtrlName[1:]

	// check if passed controller is in proper package
	if !strings.HasPrefix(fullCtrlName, a.ControllerPackage) {
		panic(fmt.Sprintf("Controller `%s` has to be in `%s` package", fullCtrlName, a.ControllerPackage))
	}

	//check if passed controller follows naming conventions
	if !strings.HasSuffix(fullCtrlName, a.ControllerSuffix) {
		panic(fmt.Sprintf("Controller `%s` does not follow naming convention", fullCtrlName))
	}

	// get DI injector
	injector := di.Struct(ctrl, a.container...)

	// inject dependencies to controller
	injector.Inject(ctrl)

	// extract controller name from struct
	ctrlName := strings.Replace(fullCtrlName, ".", "", -1)
	ctrlName = strings.TrimPrefix(ctrlName, a.ControllerPackage)
	ctrlName = strings.TrimSuffix(ctrlName, a.ControllerSuffix)

	// extract controller version from name
	version = ctrlVerRegex.FindString(ctrlName)
	if version != "" {
		ctrlName = strings.TrimPrefix(ctrlName, version)
		version = "/" + strings.ToLower(version)
	}

	// assign controller Name to prefix if it is not Index controller
	if ctrlName != a.ControllerIndex {
		prefix = toSnakeCase(ctrlName)
		prefix = fmt.Sprintf("/%s", prefix)
		prefix = strings.ToLower(prefix)
	}

	// check if controller implements versioner
	if v, ok := ctrl.(ControllerVersioner); ok {
		version = v.Version()
	}

	// check if controller implements prefixer
	if p, ok := ctrl.(ControllerPrefixer); ok {
		prefix = p.Prefix()
	}

	// check if controller imlements initer
	if i, ok := ctrl.(Initer); ok {
		i.Init(a)
	}

	path := fmt.Sprintf("%s%s", version, prefix)

	if !strings.HasPrefix(path, "/") {
		panic(fmt.Sprintf("Unable to register controller: `%s`, controller path has to start with `/`. Check Controller `Version()` and `Prefix()` method implementation ", fullCtrlName))
	}

	// log registration for debugging purposes
	a.Logger.Debug(fmt.Sprintf("Registering `%s` with Path: `%s`", fullCtrlName, path))

	ctrlRouter, ok := ctrl.(ControllerRouter)
	if !ok {
		panic(fmt.Sprintf("controller `%s` does not implement ControllerRouter interface", fullCtrlName))
	}

	routes := ctrlRouter.Routes()

	a.router.Attach(path, routes)
	return a
}

// MethodNotAllowedHandler is Handler where message and error can be personalized
// to be in line with application design and logic
func (a *App) MethodNotAllowedHandler(handler HandlerFunc) {
	a.methodNotAllowedHandler = handler
}

// NotFoundHandler is Handler where message and error can be personalized
// to be in line with application design and logic
func (a *App) NotFoundHandler(handler HandlerFunc) {
	a.notFoundHandler = handler
}

// UnauthorizedHandler is handler which is triggered ServeError with 401 status code is called
func (a *App) UnauthorizedHandler(handler HandlerFunc) {
	a.unauthorizedHandler = handler
}

// ErrorHandler is Handler where message and error can be personalized
// to be in line with application design and logic
func (a *App) ErrorHandler(handler HandlerFunc) {
	a.errorHandler = handler
}

func (a *App) Start() {
	a.Logger.Info(fmt.Sprintf("Starting %s version %s...", a.Name, a.Version))

	group := new(errgroup.Group)
	group.Go(func() error { return a.StartHTTP() })
	group.Go(func() error { return a.StartGRPC() })

	a.Logger.Fatal(group.Wait())
}

// StartHTTP the application at the specified address/port and listen for OS
// interrupt and kill signals and will attempt to stop the application gracefully.
func (a *App) StartHTTP() error {
	if a.HTTPAddr == "" {
		return nil
	}

	a.Logger.Info(fmt.Sprintf("Starting HTTP Server at %s", a.HTTPAddr))

	// create http server
	srv := http.Server{
		Handler: apmhttp.Wrap(a),
	}

	// make interrupt channel
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	// listen for interrupt signal
	go func() {
		<-c
		a.Logger.Info("Shutting down application")
		if err := a.stop(); err != nil {
			a.Logger.Error(err.Error())
		}

		if err := srv.Shutdown(context.Background()); err != nil {
			a.Logger.Error(err.Error())
		}
	}()

	srv.Addr = a.HTTPAddr
	if strings.HasPrefix(a.HTTPAddr, "unix:") {
		// create unix network listener
		lis, err := net.Listen("unix", a.HTTPAddr[5:])
		if err != nil {
			return err
		}
		// start accepting incomming requests on listener
		return srv.Serve(lis)
	} else {
		return srv.ListenAndServe()
	}
}

// ServeGRPC the application at the specified address/port and listen for OS
// interrupt and kill signals and will attempt to stop the application gracefully.
func (a *App) StartGRPC() error {
	if a.GRPCAddr == "" {
		return nil
	}

	a.Logger.Info(fmt.Sprintf("Starting GRPC Server at %s", a.GRPCAddr))

	// make interrupt channel
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	// listen for interrupt signal
	go func() {
		<-c
		a.Logger.Info("Shutting down application")
		if err := a.stop(); err != nil {
			a.Logger.Error(err.Error())
		}

		a.server.GracefulStop()
	}()

	if strings.HasPrefix(a.GRPCAddr, "unix:") {
		// create unix network listener
		lis, err := net.Listen("unix", a.GRPCAddr[5:])
		if err != nil {
			return err
		}
		// start accepting incomming requests on listener
		return a.server.Serve(lis)
	} else {
		lis, err := net.Listen("tcp", a.GRPCAddr)
		if err != nil {
			return err
		}
		return a.server.Serve(lis)
	}
}

// Router returns application router instance
func (a *App) Router() *Router {
	return a.router
}

// ServeHTTP conforms to the http.Handler interface.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get context from pool
	c := a.pool.Get().(*Context)
	// reset response writer
	c.writermem.reset(w)
	// set request
	c.Request = r

	// reset context from previous use
	c.reset()

	// handle the request
	a.handleHTTPRequest(c)

	// put back context to pool
	a.pool.Put(c)
}

func (a *App) stop() error {
	return nil
}

// Stop issues interrupt signal
func (a *App) Stop() error {
	// get current process
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	a.Logger.Debug("Stopping....")
	// issue interrupt signal
	return proc.Signal(os.Interrupt)
}

func (a *App) handleHTTPRequest(c *Context) {
	req := c.Request
	httpMethod := req.Method
	path := req.URL.Path

	if root := a.router.trees[httpMethod]; root != nil {
		if handlers, ps, tsr := root.getValue(path); handlers != nil {
			c.handlers = handlers
			c.Params = ps
			c.Next()
			c.writermem.WriteHeaderNow()
			return
		} else if httpMethod != "CONNECT" && path != "/" {
			code := http.StatusMovedPermanently // Permanent redirect, request with GET method
			if httpMethod != "GET" {
				code = http.StatusTemporaryRedirect
			}
			if tsr && a.RedirectTrailingSlash {
				req.URL.Path = path + "/"
				if length := len(path); length > 1 && path[length-1] == '/' {
					req.URL.Path = path[:length-1]
				}
				// logger here
				http.Redirect(c.Response, req, req.URL.String(), code)
				c.writermem.WriteHeaderNow()
				return
			}

			if a.RedirectFixedPath {
				fixedPath, found := root.findCaseInsensitivePath(CleanPath(path), a.RedirectTrailingSlash)
				if found {
					req.URL.Path = string(fixedPath)
					// logger here
					http.Redirect(c.Response, req, req.URL.String(), code)
					c.writermem.WriteHeaderNow()
					return
				}
			}
		}
	}

	if a.HandleMethodNotAllowed {
		if allow := a.router.allowed(path, httpMethod); len(allow) > 0 {
			c.handlers = a.router.Handlers
			c.ServeError(http.StatusMethodNotAllowed, errors.New(default405Body))
			return
		}
	}

	c.handlers = a.router.Handlers
	c.ServeError(http.StatusNotFound, errors.New(default404Body))
}

func (a *App) allocateContext() *Context {
	return &Context{app: a}
}
