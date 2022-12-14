package cucumber

// Route represents a request route's specification which
// contains method and path and its handler.
type Route struct {
	Method        string
	Path          string
	HandlersChain HandlersChain
	HandlerName   string
	HandlerFunc   HandlerFunc
}

// Routes defines a Route array.
type Routes []Route
