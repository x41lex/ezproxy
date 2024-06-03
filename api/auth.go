// Web authentication implementation
package api

import (
	"crypto/rand"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
)

// Authentication permissions
type authPerms int

const (
	AuthCanCheckStatus   authPerms = 1 << 0 // /api/1/status, /api/1/proxies
	AuthCanClose         authPerms = 1 << 1 // api/1/close (Closing proxies) /api/1/socket (Requires authCanUseWebsocket)
	AuthCanUseWebsocket  authPerms = 1 << 2 // /api/1/socket (Listening, not injecting or filtering)
	AuthCanFilter        authPerms = 1 << 3 // /api/1/socket (Filtering, requires authCanUseWebsocket)
	AuthCanInject        authPerms = 1 << 4 // /api/1/inject (Injecting) /api/1/socket (Requires authCanUseWebsocket)
	AuthCanMakeKeys      authPerms = 1 << 5 // /api/1/key (Creating new keys) You can still only create keys with permissions matching your own, minus this one
	AuthCanDuplicateKeys authPerms = 1 << 6 // /api/1/key (Creating new keys) Can create keys matching these permissions including AuthCanMakeKeys

	AuthAll            authPerms = 0xfffffffffffffff // All auth values
	AuthAllButMakeKeys authPerms = 0xfffffffffffffdf // All auth values but make keys
)

func checkPermission(val int, code authPerms) bool {
	return val&int(code) != 0
}

// Key lookup
type authLookup struct {
	keys   map[uint64]int // Key to permissions
	logger *slog.Logger   // Current logger, may be nil
}

// Deprecated: Use [getAuthValues]
//
// Parse a request and get the API key in the 'key' query parameter, if the 'key' query is not found a HTTP 401 error will
// be sent on the ResponseWriter and 'ok' will be 'false', if the 'key' is not a valid hex uint64 integer, a HTTP 400 error will be sent, this does not
// verify the API key is valid.
func (l *authLookup) getKey(w http.ResponseWriter, r *http.Request) (key uint64, ok bool) {
	qr := r.URL.Query()
	if !qr.Has("key") {
		l.logger.Debug("Cant get auth key, missing 'key' query parameter")
		writeResponse(w, http.StatusUnauthorized, "no 'key' query parameter")
		return 0, false
	}
	keyStr := qr.Get("key")
	key, err := strconv.ParseUint(keyStr, 16, 64)
	if err != nil {
		l.logger.Debug("Cant get auth key, invalid 'key' query parameter", "Value", keyStr, "Error", err.Error())
		writeResponse(w, http.StatusBadRequest, "invalid 'key' parameter")
		return 0, false
	}
	l.logger.Debug("Got auth key", "Value", key)
	return key, true
}

// Gets the auth key from the 'key' query parameter and the associated key value, a HTTP 401 will be sent if they key 'query' was not found
// and a 401 if the key is not a valid hex uint64 integer or the key was not found.
func (l *authLookup) getAuthValues(w http.ResponseWriter, r *http.Request) (key uint64, value int, ok bool) {
	key, ok = l.getKey(w, r)
	if !ok {
		return 0, 0, false
	}
	if value, found := l.keys[key]; found {
		return key, value, true
	}
	// You could change this to prevent leaking that the API key doesn't exist, but with 1^64-1 options I doubt it matters.
	writeResponse(w, http.StatusUnauthorized, "unknown api key")
	l.logger.Debug("Unknown api key", "Key", key)
	return 0, 0, false
}

// Creates a new random key with a permission bitfield, if after a thousand random keys one unique one was not found this program
// will panic.
func (l *authLookup) newKeyInt(perms int) uint64 {
	for i := 0; i != 1000; i++ {
		v, err := rand.Int(rand.Reader, big.NewInt(int64(0x7fffffffffffffff)))
		if err != nil {
			// Panic - something seriously wrong
			panic(err)
		}
		val := uint64(v.Int64())
		// If we can add the key we found a good one and
		// can return it, otherwise it already exists
		err = l.addKeyInt(val, perms)
		if err == nil {
			return val
		}
	}
	panic("failed to generate new key")
}

// Checks if the request has the desired permsisions, if not a 400, 401 or 403 error code will be written depending on
// why the key is invalid and false will be returned.
func (l *authLookup) checkPermission(w http.ResponseWriter, r *http.Request, requires ...authPerms) bool {
	// We do it manually so we can use diffrent status codes based on state.
	qr := r.URL.Query()
	if !qr.Has("key") {
		writeResponse(w, http.StatusUnauthorized, "missing 'key' parameter")
		return false
	}
	keyStr := qr.Get("key")
	key, err := strconv.ParseUint(keyStr, 16, 64)
	if err != nil {
		writeResponse(w, http.StatusUnauthorized, "invalid 'key' parmeter")
		return false
	}
	value, found := l.keys[key]
	if !found {
		writeResponse(w, http.StatusUnauthorized, "unknown api key")
		return false
	}
	for _, p := range requires {
		if !checkPermission(value, p) {
			writeResponse(w, http.StatusForbidden, "invalid permissions")
			return false
		}
	}
	return true
}

// Adds a key with permissions, if the key already exists a error will be returned.
func (l *authLookup) addKey(key uint64, perms ...authPerms) error {
	if _, found := l.keys[key]; found {
		l.logger.Debug("Attempted to add key that already exists", "Key", key)
		return errors.New("key already exists")
	}
	pVal := 0
	for _, v := range perms {
		pVal |= int(v)
	}
	return l.addKeyInt(key, pVal)
}

// Adds a key with permissions, if the key already exists a error will be returned.
func (l *authLookup) addKeyInt(key uint64, perms int) error {
	if _, found := l.keys[key]; found {
		return errors.New("key already exists")
	}
	// Maybe remove reserved bits?
	l.keys[key] = perms
	l.logger.Debug("Adding auth key", "Key", key, "Perms", perms)
	return nil
}

// Create a new AuthLookup
func newAuthLookup() *authLookup {
	return &authLookup{
		keys:   make(map[uint64]int),
		logger: slog.Default(),
	}
}
