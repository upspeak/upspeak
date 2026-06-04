package realtime

import "net/http"

// Authenticator authorises a WebSocket upgrade request. The returned identity
// scopes per-user connection limits. The default implementation permits every
// request; real token or identity checks replace it in a later phase.
type Authenticator interface {
	Authenticate(r *http.Request) (identity string, err error)
}

// allowAllAuthenticator permits every upgrade and assigns a single shared
// identity. It is the default until real authentication exists.
type allowAllAuthenticator struct{}

// Authenticate always succeeds with a fixed identity.
func (allowAllAuthenticator) Authenticate(_ *http.Request) (string, error) {
	return "local", nil
}
