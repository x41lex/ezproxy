// EzProxy Web API
package api

import (
	"context"
	"errors"
	"ezproxy/handler"
	"fmt"
	"log/slog"
	"net/http"
)

type apiEndpoint struct {
	Endpoint string
	Version  int
	Perms    int
	Method   string
	Desc     string
}

// Base of the response from the API
type baseResponse struct {
	Status int         // HTTP status code
	Data   interface{} // If Status != 200 this is a error string
}

// Web API handler
type WebApi struct {
	auth       *authLookup           // Authentication handler, if nil authentication will be disabled.
	mux        *http.ServeMux        // HTTP Mux
	handler    handler.IProxySpawner // Current proxy spawner
	ctx        context.Context       // Context that cancels this and all WebSockets
	cancelFunc context.CancelFunc    // Cancel this context
	logger     *slog.Logger          // Can be nil
	wsocks     []*wsApi              // Connected WebSockets
	endpoints  []apiEndpoint         // Endpoint info for self documentation
}

func (w *WebApi) hasFilterer() bool {
	for _, v := range w.wsocks {
		if v.ctx.Err() != nil {
			continue
		}
		if v.canFilter {
			return true
		}
	}
	return false
}

// Adds a new auth key, with all permissions needed.
// A error will be returned if authentication is disabled or the key is already in use.
func (w *WebApi) AddAuth(key uint64, perms ...authPerms) error {
	if w.auth == nil {
		return errors.New("auth is disabled")
	}
	return w.auth.addKey(key, perms...)
}

func (wa *WebApi) homePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	// https://markdowntohtml.com/ ? or maybe a JS thing
	data := "<html><h1>EzProxy API</h1>"
	for _, v := range wa.endpoints {
		data += fmt.Sprintf("<a><b>%s</b><p>%s</p><ul><li>Version: %d</li><li>Method : %s</li><li>Permissions:</li><ul>", v.Endpoint, v.Desc, v.Version, v.Method)
		if checkPermission(v.Perms, AuthCanCheckStatus) {
			data += fmt.Sprintf("<li>AuthCanCheckStatus (%d)</li>", AuthCanCheckStatus)
		}
		if checkPermission(v.Perms, AuthCanClose) {
			data += fmt.Sprintf("<li>AuthCanClose (%d)</li>", AuthCanClose)
		}
		if checkPermission(v.Perms, AuthCanUseWebsocket) {
			data += fmt.Sprintf("<li>AuthCanUseWebsocket (%d)</li>", AuthCanUseWebsocket)
		}
		if checkPermission(v.Perms, AuthCanFilter) {
			data += fmt.Sprintf("<li>AuthCanFilter (%d)</li>", AuthCanFilter)
		}
		if checkPermission(v.Perms, AuthCanInject) {
			data += fmt.Sprintf("<li>AuthCanInject (%d)</li>", AuthCanInject)
		}
		if checkPermission(v.Perms, AuthCanMakeKeys) {
			data += fmt.Sprintf("<li>AuthCanMakeKeys (%d)</li>", AuthCanMakeKeys)
		}
		if checkPermission(v.Perms, AuthCanDuplicateKeys) {
			data += fmt.Sprintf("<li>AuthCanDuplicateKeys (%d)</li>", AuthCanDuplicateKeys)
		}
		data += "</ul></ul></a>"
	}
	data += "</html>"
	w.Write([]byte(data))
}

func (w *WebApi) Close() {
	w.cancelFunc()
	for _, v := range w.wsocks {
		v.Close()
	}
}

// Creates a new WebApi instance.
// If useAuth is false then GetNewAuth and AddAuth will both return errors.
func NewWebApi(mux *http.ServeMux, useAuth bool, ph handler.IProxySpawner) *WebApi {
	wa := &WebApi{
		mux:        mux,
		handler:    ph,
		logger:     slog.Default(),
		ctx:        nil,
		cancelFunc: nil,
		wsocks:     make([]*wsApi, 0),
		endpoints:  make([]apiEndpoint, 0),
		auth:       nil,
	}
	if useAuth {
		wa.auth = newAuthLookup()
	} else {
		wa.logger.Info("API Authentication is disabled, not setting up auth")
	}
	c, can := context.WithCancel(context.Background())
	wa.cancelFunc = can
	wa.ctx = c
	// I need to better document this.
	// Write it in MD and convert that to HTML probably.
	// For sure returns need to be documented
	mux.HandleFunc("/", wa.homePage)
	wa.addEndpoint("status", 1, http.MethodGet, wa.epStatus, AuthCanCheckStatus)
	wa.documentEndpoint("status", "Get status of the Proxy Spawner", 1, "GET", int(AuthCanCheckStatus))
	wa.addEndpoint("proxies", 1, http.MethodGet, wa.epProxyList, AuthCanCheckStatus)
	wa.documentEndpoint("proxies", "Get status of all connected clients", 1, "GET", int(AuthCanCheckStatus))
	wa.addEndpoint("inject", 1, http.MethodPost, wa.epInject, AuthCanInject)
	wa.documentEndpoint("inject", "Inject data to a target, send JSON data to inject. (TODO: Better docuemnt)", 1, "POST", int(AuthCanInject))
	wa.addEndpoint("newkey", 1, http.MethodGet, wa.epGetKey, AuthCanMakeKeys)
	wa.documentEndpoint("newkey", "Create a new key with your permissions.", 1, "GET", int(AuthCanMakeKeys))
	wa.addEndpoint("keyinfo", 1, http.MethodGet, wa.epGetAuthValue) // Anyone can use this given they have a valid API key
	wa.documentEndpoint("keyinfo", "Get info about this API key", 1, "GET", 0)
	wa.addEndpoint("socket", 2, http.MethodGet, wa.newWebSocket, AuthCanUseWebsocket)
	wa.documentEndpoint("socket", "Create a new WebSocket (TODO: Document the rest of this)", 2, "GET", int(AuthCanUseWebsocket))
	return wa
}
