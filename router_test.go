package cucumber

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouterBasic(t *testing.T) {
	router := NewRouter()
	group := router.Group("/hi", func(c *Context) {})
	group.Use(func(c *Context) {})

	assert.Len(t, group.Handlers, 2)
	assert.Equal(t, "/hi", group.BasePath())

	group2 := group.Group("resho")
	group2.Use(func(c *Context) {}, func(c *Context) {})

	assert.Len(t, group2.Handlers, 4)
	assert.Equal(t, "/hi/resho", group2.BasePath())
}

func TestRouterGroupBasicHandle(t *testing.T) {
	performRequestInGroup(t, "GET")
	performRequestInGroup(t, "POST")
	performRequestInGroup(t, "PUT")
	performRequestInGroup(t, "PATCH")
	performRequestInGroup(t, "DELETE")
	performRequestInGroup(t, "HEAD")
	performRequestInGroup(t, "OPTIONS")
}

func TestRouterGroupInvalidStatic(t *testing.T) {
	router := NewRouter()
	assert.Panics(t, func() {
		router.Static("/path/:param", "/")
	})

	assert.Panics(t, func() {
		router.Static("/path/*param", "/")
	})
}

func TestRouterGroupInvalidStaticFile(t *testing.T) {
	router := NewRouter()
	assert.Panics(t, func() {
		router.StaticFile("/path/:param", "favicon.ico")
	})

	assert.Panics(t, func() {
		router.StaticFile("/path/*param", "favicon.ico")
	})
}

func TestRouterGroupTooManyHandlers(t *testing.T) {
	router := NewRouter()
	middlewares1 := make([]HandlerFunc, 40)
	router.Use(middlewares1...)

	middlewares2 := make([]HandlerFunc, 26)
	router.Use(middlewares2...)

	handler := func(c *Context) {}

	assert.Panics(t, func() {
		router.GET("/", handler)
	})
}

func TestRouterGroupBadMethod(t *testing.T) {
	router := NewRouter()
	assert.Panics(t, func() {
		router.Handle("get", "/")
	})
	assert.Panics(t, func() {
		router.Handle(" GET", "/")
	})
	assert.Panics(t, func() {
		router.Handle("GET ", "/")
	})
	assert.Panics(t, func() {
		router.Handle("", "/")
	})
	assert.Panics(t, func() {
		router.Handle("PO ST", "/")
	})
	assert.Panics(t, func() {
		router.Handle("1GET", "/")
	})
	assert.Panics(t, func() {
		router.Handle("PATCh", "/")
	})
}

func TestRouterMiddlewareGeneralCase(t *testing.T) {
	signature := ""
	opts := NewOptions()

	opts.UseViewEngine = false
	opts.UseRequestLogger = false
	opts.UseSession = false
	opts.UseTranslator = false

	router := NewWithOptions(opts)
	router.Use(func(c *Context) {
		signature += "A"
		c.Next()
		signature += "D"
	})
	router.Use(func(c *Context) {
		signature += "B"
	})
	router.GET("/", func(c *Context) {
		signature += "C"
	})
	// RUN
	w := performRequest(router, "GET", "/")

	// TEST
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ABCD", signature)
}

func TestRouterMiddlewareAbort(t *testing.T) {
	signature := ""
	opts := NewOptions()

	opts.UseViewEngine = false
	opts.UseRequestLogger = false
	opts.UseSession = false
	opts.UseTranslator = false

	router := NewWithOptions(opts)

	router.Use(func(c *Context) {
		signature += "A"
	})
	router.Use(func(c *Context) {
		signature += "C"
		c.Status(http.StatusUnauthorized)
		c.Abort()
		c.Next()
		signature += "D"
	})
	router.GET("/", func(c *Context) {
		signature += " X "
		c.Next()
		signature += " XX "
	})

	// RUN
	w := performRequest(router, "GET", "/")

	// TEST
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "ACD", signature)
}

func TestRouterMiddlewareAbortHandlersChainAndNext(t *testing.T) {
	signature := ""

	opts := NewOptions()

	opts.UseViewEngine = false
	opts.UseRequestLogger = false
	opts.UseSession = false
	opts.UseTranslator = false

	router := NewWithOptions(opts)

	router.Use(func(c *Context) {
		signature += "A"
		c.Next()
		c.Status(http.StatusGone)
		c.Abort()
		signature += "B"

	})
	router.GET("/", func(c *Context) {
		signature += "C"
		c.Next()
	})
	// RUN
	w := performRequest(router, "GET", "/")

	// TEST
	assert.Equal(t, http.StatusGone, w.Code)
	assert.Equal(t, "ACB", signature)
}

func TestRouterMiddlewareFailHandlersChain(t *testing.T) {
	// SETUP
	signature := ""

	opts := NewOptions()

	opts.UseViewEngine = false
	opts.UseRequestLogger = false
	opts.UseSession = false
	opts.UseTranslator = false

	router := NewWithOptions(opts)

	router.Use(func(context *Context) {
		signature += "A"
		context.Abort()
		context.ServeError(http.StatusInternalServerError, errors.New("foo"))
	})
	router.Use(func(context *Context) {
		signature += "B"
		signature += "C"
	})
	// RUN
	w := performRequest(router, "GET", "/")

	// TEST
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "A", signature)
}

func performRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func performRequestInGroup(t *testing.T, method string) {
	opts := NewOptions()

	opts.UsePanicRecovery = false
	opts.UseViewEngine = false
	opts.UseRequestLogger = false
	opts.UseSession = false
	opts.UseTranslator = false

	app := NewWithOptions(opts)
	router := app.Router()
	v1 := router.Group("v1", func(c *Context) {})
	assert.Equal(t, "/v1", v1.BasePath())

	login := v1.Group("/login/", func(c *Context) {}, func(c *Context) {})
	assert.Equal(t, "/v1/login/", login.BasePath())

	handler := func(c *Context) {
		c.Status(http.StatusBadRequest)
		c.Response.WriteString(fmt.Sprintf("the method was %s and index %d", c.Request.Method, c.index))
	}

	switch method {
	case "GET":
		v1.GET("/test", handler)
		login.GET("/test", handler)
	case "POST":
		v1.POST("/test", handler)
		login.POST("/test", handler)
	case "PUT":
		v1.PUT("/test", handler)
		login.PUT("/test", handler)
	case "PATCH":
		v1.PATCH("/test", handler)
		login.PATCH("/test", handler)
	case "DELETE":
		v1.DELETE("/test", handler)
		login.DELETE("/test", handler)
	case "HEAD":
		v1.HEAD("/test", handler)
		login.HEAD("/test", handler)
	case "OPTIONS":
		v1.OPTIONS("/test", handler)
		login.OPTIONS("/test", handler)
	default:
		panic("unknown method")
	}

	w := performRequest(app, method, "/v1/login/test")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "the method was "+method+" and index 3", w.Body.String())

	w = performRequest(app, method, "/v1/test")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "the method was "+method+" and index 1", w.Body.String())
}
