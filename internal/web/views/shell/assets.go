package shell

// AppCSSHref and DatastarJSHref are shared with internal/httproute's asset
// routes so the <link>/<script> and the route can't drift apart.
const (
	AppCSSHref     = "/static/app.css"
	DatastarJSHref = "/static/datastar.js"
)
