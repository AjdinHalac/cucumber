package cucumber

import (
	"html/template"

	"github.com/AjdinHalac/cucumber/log"
	"github.com/AjdinHalac/cucumber/render/view"
	"github.com/AjdinHalac/cucumber/sessions"
	"google.golang.org/grpc"
)

const (
	defaultEnv     = "development"
	defaultName    = "cucumberApp"
	defaultVersion = "v0.0.0"

	defaultLogLevel = "debug"

	defaultRedirectTrailingSlash  = true
	defaultRedirectFixedPath      = false
	defaultHandleMethodNotAllowed = false
	defaultMaxMultipartMemory     = 32 << 20 // 32 MB

	default404Body = "404 page not found"
	default405Body = "405 method not allowed"

	defaultUseSession  = false
	defaultSessionName = "_cucumber_app_session"

	defaultUseTranslator         = false
	defaultTranslatorLocalesRoot = "locales"
	defaultTranslatorDefaultLang = "en-US"

	defaultUseRequestLogger = true
	defaultUsePanicRecovery = true

	defaultUseViewEngine     = false
	defaultViewsRoot         = "views"
	defaultViewsExt          = ".tpl"
	defaultViewsMasterLayout = "layouts/master"
	defaultViewsPartialsRoot = "partials"
	defaultViewsDisableCache = false

	defaultServeStatic = false
	defaultStaticPath  = "/static"
	defaultStaticDir   = "./public"

	// ControllerPackage holds package name in which controllers can be registered
	defaultControllerPackage = "controllers"
	// ControllerIndex holds controller Index name
	defaultControllerIndex = "Index"
	// ControllerSuffix holds controller naming convention
	defaultControllerSuffix = "Controller"
)

// Options holds cucumber configuration options
type Options struct {
	Env      string
	Name     string
	HTTPAddr string
	GRPCAddr string
	Version  string

	LogLevel string

	RedirectTrailingSlash  bool
	RedirectFixedPath      bool
	HandleMethodNotAllowed bool
	MaxMultipartMemory     int64

	Body404 string
	Body500 string

	UseSession    bool
	SessionName   string
	SessionSecret string

	UseTranslator         bool
	TranslatorLocalesRoot string
	TranslatorDefaultLang string

	UseRequestLogger bool
	UsePanicRecovery bool

	UseViewEngine     bool
	ViewsRoot         string
	ViewsExt          string
	ViewsMasterLayout string
	ViewsPartialsRoot string
	ViewsDisableCache bool

	ServeStatic bool
	StaticPath  string
	StaticDir   string

	Logger            log.Logger
	SessionStore      sessions.Store
	ViewEngine        view.Engine
	Translator        *Translator
	UnaryInterceptors []grpc.UnaryServerInterceptor

	// ControllerPackage holds package name in which controllers can be registered
	ControllerPackage string
	// ControllerIndex holds controller Index name
	ControllerIndex string
	// ControllerSuffix holds controller naming convention
	ControllerSuffix string

	RequestLoggerIgnore []string

	UnaryRequestLoggerIgnore []string

	AppConfig interface{}
}

// NewOptions returns a new Options instance with default configuration
func NewOptions() Options {
	opts := Options{
		Env:                    defaultEnv,
		Name:                   defaultName,
		Version:                defaultVersion,
		LogLevel:               defaultLogLevel,
		RedirectTrailingSlash:  defaultRedirectTrailingSlash,
		RedirectFixedPath:      defaultRedirectFixedPath,
		HandleMethodNotAllowed: defaultHandleMethodNotAllowed,
		MaxMultipartMemory:     defaultMaxMultipartMemory,
		Body404:                default404Body,
		Body500:                default405Body,
		UseSession:             defaultUseSession,
		SessionName:            defaultSessionName,
		UseTranslator:          defaultUseTranslator,
		TranslatorLocalesRoot:  defaultTranslatorLocalesRoot,
		TranslatorDefaultLang:  defaultTranslatorDefaultLang,
		UseRequestLogger:       defaultUseRequestLogger,
		UsePanicRecovery:       defaultUsePanicRecovery,
		UseViewEngine:          defaultUseViewEngine,
		ViewsRoot:              defaultViewsRoot,
		ViewsExt:               defaultViewsExt,
		ViewsMasterLayout:      defaultViewsMasterLayout,
		ViewsPartialsRoot:      defaultViewsPartialsRoot,
		ViewsDisableCache:      defaultViewsDisableCache,
		ServeStatic:            defaultServeStatic,
		StaticPath:             defaultStaticPath,
		StaticDir:              defaultStaticDir,
		ControllerPackage:      defaultControllerPackage,
		ControllerIndex:        defaultControllerIndex,
		ControllerSuffix:       defaultControllerSuffix,
	}

	return opts
}

func optionsWithDefault(opts Options) Options {
	//configure logger
	if opts.Logger == nil {
		opts.Logger = log.New(log.Configuration{
			EnableConsole:     true,
			ConsoleJSONFormat: true,
			ConsoleLevel:      opts.LogLevel,
		})
	}

	//configure session store
	if opts.UseSession && opts.SessionStore == nil {
		if opts.SessionSecret == "" {
			opts.Logger.Warn("SessionSecret configuration key is not set. Your sessions are not safe!")
		}
		opts.SessionStore = sessions.NewCookieStore([]byte(opts.SessionSecret))
	}
	//configure ViewEngine
	if opts.UseViewEngine && opts.ViewEngine == nil {
		partials, err := loadPartials(opts.ViewsRoot, opts.ViewsPartialsRoot, opts.ViewsExt)
		if err != nil {
			opts.Logger.Fatal(err.Error())
		}
		opts.ViewEngine = view.NewHTMLEngine(view.Config{
			Root:         opts.ViewsRoot,
			Ext:          opts.ViewsExt,
			Master:       opts.ViewsMasterLayout,
			Partials:     partials,
			Funcs:        make(template.FuncMap),
			DisableCache: opts.ViewsDisableCache,
			Delims:       view.Delims{Left: "{{", Right: "}}"},
		})
	}

	// configure translator
	if opts.UseTranslator && opts.Translator == nil {
		t, err := NewTranslator(opts.TranslatorLocalesRoot, opts.TranslatorDefaultLang)
		if err != nil {
			opts.Logger.Fatal(err.Error())
		}
		opts.Translator = t
	}

	return opts
}
