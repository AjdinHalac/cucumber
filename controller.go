package cucumber

// ControllerRouter allows controller to define custom controller routing
//
// Controller is used on app#RegisterController
type ControllerRouter interface {
	Routes() *Router
}

// ControllerPrefixer allows to customize Controller prefix
// for controller action routing
type ControllerPrefixer interface {
	Prefix() string
}

// ControllerVersioner allows controllers versioning
type ControllerVersioner interface {
	Version() string
}
