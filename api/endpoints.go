package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// Adds a new endpoint to the API, the endpoint will end up as /api/<version>/<name>, require the request be the 'method' specified and require 'perms' permissions if [authLookup] is enabled.
func (a *WebApi) addEndpoint(name string, version int, method string, handler http.HandlerFunc, perms ...authPerms) {
	a.mux.HandleFunc(fmt.Sprintf("/api/%d/%s", version, name), func(w http.ResponseWriter, r *http.Request) {
		a.logger.Debug("Request to endpoint", "EndpointName", name, "Version", version, "Endpoint", fmt.Sprintf("/api/%d/%s", version, name), "RequestedURI", r.RequestURI)
		if r.Method != method {
			a.logger.Debug("Invalid request to endpoint, invalid method", "EndpointName", name, "Version", version, "ExpectedMethod", method, "Method", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(fmt.Sprintf("Method must be %s", method)))
			return
		}
		if a.auth != nil && !a.auth.checkPermission(w, r, perms...) {
			// Other permission data already logged
			a.logger.Debug("Missing permissions on request", "EndpointName", name, "Version", version)
			return
		}
		handler(w, r)
	})
}

func (a *WebApi) documentEndpoint(name string, desc string, version int, method string, perms int) {
	a.endpoints = append(a.endpoints, apiEndpoint{
		Endpoint: name,
		Version:  version,
		Perms:    perms,
		Method:   method,
		Desc:     desc,
	})
}

// Status of a handler, used for /api/1/handler
type handlerStatus struct {
	ConnectionCount int               // Number of connections
	Alive           bool              // Is the handler alive
	BytesSent       uint64            // Number of bytes sent
	MpxAddresses    map[string]string // MpxName -> Proxy address (IP):(PORT)
	ServerAddress   string            // Server address (IP):(PORT)
}

func (a *WebApi) epStatus(w http.ResponseWriter, r *http.Request) {
	data := handlerStatus{
		ConnectionCount: len(a.handler.GetAllProxies()),
		Alive:           a.handler.IsAlive(),
		BytesSent:       a.handler.GetBytesSent(),
		MpxAddresses:    make(map[string]string),
		ServerAddress:   a.handler.GetServerAddr().String(),
	}
	for k, v := range a.handler.GetMpxAddrs() {
		data.MpxAddresses[k] = v.String()
	}
	a.logger.Debug("Sending HandlerStatus")
	writeResponse(w, 200, data)
}

// Status of a proxy, used for /api/1/proxies
type proxyStatus struct {
	Id             int    // Proxy Id
	Alive          bool   // Is the client alive
	Address        string // (IP):(Port) of this client
	Network        string // Network this proxy is connected on
	BytesSent      uint64 // Number of bytes sent
	LastContactAgo int64  // last contact ago in MS
}

func (a *WebApi) epProxyList(w http.ResponseWriter, r *http.Request) {
	data := make([]proxyStatus, 0)
	for _, v := range a.handler.GetAllProxies() {
		data = append(data, proxyStatus{
			Id:             v.GetId(),
			Alive:          v.IsAlive(),
			Address:        v.GetClientAddr().String(),
			Network:        v.GetClientAddr().Network(),
			BytesSent:      v.GetBytesSent(),
			LastContactAgo: v.LastContactTimeAgo().Milliseconds(),
		})
	}
	a.logger.Debug("Sending []ProxyStatus", "Count", len(data))
	writeResponse(w, 200, data)
}

// Inject data, used for /api/1/inject
type injectData struct {
	Id       int    // Proxy ID, set to -1 for all
	Data     []byte // Inject data
	ToClient bool   // Send to client(s)
	ToServer bool   // Send to server(s)
}

func (a *WebApi) epInject(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		// Server error not API error
		a.logger.Warn("Failed to read data from request", "Error", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(data))
		return
	}
	id := &injectData{}
	err = json.Unmarshal(data, id)
	if err != nil {
		// API error
		a.logger.Debug("Got invalid JSON data", "Error", err.Error())
		writeResponse(w, http.StatusBadRequest, "Invalid JSON data")
		return
	}
	// Actually handle it
	if !id.ToClient && !id.ToServer {
		a.logger.Debug("ToClient & ToServer are false")
		writeResponse(w, http.StatusBadRequest, "ToClient and/or ToServer must be true")
		return
	}
	if id.Id == -1 {
		a.logger.Debug("Injecting data to all", "Data", id.Data, "ToClient", id.ToClient, "ToServer", id.ToServer)
		if id.ToClient {
			a.handler.SendToAllClients(id.Data)
		}
		if id.ToServer {
			a.handler.SendToAllServers(id.Data)
		}
		writeResponse(w, 200, "")
		return
	}
	px, err := a.handler.GetProxy(id.Id)
	if err != nil {
		a.logger.Debug("Proxy not found to inject to", "Id", id.Id)
		writeResponse(w, http.StatusNotFound, fmt.Sprintf("proxy not found: %v", err))
		return
	}
	a.logger.Debug("Sending to proxy", "Id", id.Id, "Data", id.Data, "ToClient", id.ToClient, "ToServer", id.ToServer)
	if id.ToClient {
		px.SendToClient(id.Data)
	}
	if id.ToServer {
		px.SendToServer(id.Data)
	}
	writeResponse(w, 200, "")
}

func isValidCreationPerm(currentValue int, userPerms int, desiredPerms int, perm authPerms) (int, bool) {
	// First we check if we even care about this one
	if !checkPermission(desiredPerms, perm) {
		return currentValue, true
	}
	// Then we check if the user has permission to do anything with that
	if !checkPermission(userPerms, perm) {
		// The user can't set this one
		return 0, false
	}
	// Its fine
	return currentValue | int(perm), true
}

func createNewPerms(userPerms int, desiredPerms int) (value int, err error) {
	value = 0
	value, ok := isValidCreationPerm(value, userPerms, desiredPerms, AuthCanCheckStatus)
	if !ok {
		return 0, errors.New("CanCheckStatus")
	}
	value, ok = isValidCreationPerm(value, userPerms, desiredPerms, AuthCanClose)
	if !ok {
		return 0, errors.New("CanClose")
	}
	value, ok = isValidCreationPerm(value, userPerms, desiredPerms, AuthCanUseWebsocket)
	if !ok {
		return 0, errors.New("CanUseWebsocket")
	}
	value, ok = isValidCreationPerm(value, userPerms, desiredPerms, AuthCanFilter)
	if !ok {
		return 0, errors.New("CanFilter")
	}
	value, ok = isValidCreationPerm(value, userPerms, desiredPerms, AuthCanInject)
	if !ok {
		return 0, errors.New("CanInject")
	}
	// We can only create a new key with CanMakeKeys if we have AuthCanDuplicateKeys
	if checkPermission(desiredPerms, AuthCanMakeKeys) {
		if !checkPermission(userPerms, AuthCanDuplicateKeys) {
			return 0, errors.New("CanMakeKeys")
		}
		value |= int(AuthCanMakeKeys)
	}
	value, ok = isValidCreationPerm(value, userPerms, desiredPerms, AuthCanDuplicateKeys)
	if !ok {
		return 0, errors.New("CanInject")
	}
	return value, nil
}

// Key info, used for /api/1/keyinfo
type newKeyData struct {
	Key   string
	Perms int
}

func (a *WebApi) epGetKey(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeResponse(w, http.StatusNoContent, "Authentication is disabled")
		return
	}
	qr := r.URL.Query()
	if !qr.Has("perms") {
		a.logger.Debug("Missing 'perms' field in request to getKey")
		writeResponse(w, http.StatusBadRequest, "requires 'perms' field")
		return
	}
	newPerms, err := strconv.ParseInt(qr.Get("perms"), 0, 64)
	if err != nil {
		a.logger.Debug("Invalid 'perms' field in request to getKey", "Error", err.Error())
		writeResponse(w, http.StatusBadRequest, "invalid perms field")
		return
	}
	key, ourPerms, ok := a.auth.getAuthValues(w, r)
	if !ok {
		a.logger.Error("authentication of both succeeded & failed", "Key", key, "Map", a.auth.keys)
		// The writer has been written to already
		return
	}
	// Now we check each possible perm
	// AuthCanMakeKeys cannot be created.
	validPerms, err := createNewPerms(ourPerms, int(newPerms))
	if err != nil {
		a.logger.Debug("Lacking permission to add to key", "Error", err.Error(), "RequestedPerms", int(newPerms), "OurPerms", ourPerms)
		writeResponse(w, http.StatusForbidden, fmt.Sprintf("lacking permission to add '%s' permission", err.Error()))
		return
	}
	if validPerms == 0 {
		a.logger.Debug("No permissions to add to key, key not being created", "RequestedPerms", int(newPerms), "OurPerms", ourPerms, "ValidPerms", validPerms)
		writeResponse(w, http.StatusBadRequest, "no permissions were added")
		return
	}
	// Set it without reserved bits
	newKey := a.auth.newKeyInt(validPerms)
	a.logger.Info("Created new Authkey", "From", key, "FromPerms", ourPerms, "New", newKey, "NewPerms", validPerms, "RequestedPerms", newPerms)
	writeResponse(w, 200, newKeyData{
		Key:   fmt.Sprintf("%x", newKey),
		Perms: validPerms,
	})
}

type authValue struct {
	Value            int  // Permission value of this key
	CanCheckStatus   bool // AuthCanCheckStatus
	CanClose         bool // AuthCanClose
	CanUseWebsocket  bool // AuthCanUseWebsocket
	CanFilter        bool // AuthCanFilter
	CanInject        bool // AuthCanInject
	CanMakeKeys      bool // AuthCanMakeKeys
	CanDuplicateKeys bool // AuthCanDuplicateKeys
	Admin            bool // AuthAll
}

func (a *WebApi) epGetAuthValue(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeResponse(w, http.StatusNoContent, "Authentication is disabled")
		return
	}
	key, value, ok := a.auth.getAuthValues(w, r)
	if !ok {
		return
	}
	a.logger.Debug("Getting auth key values", "Key", key, "Value", value)
	writeResponse(w, 200, authValue{
		Value:            value,
		CanCheckStatus:   checkPermission(value, AuthCanCheckStatus),
		CanClose:         checkPermission(value, AuthCanClose),
		CanUseWebsocket:  checkPermission(value, AuthCanUseWebsocket),
		CanFilter:        checkPermission(value, AuthCanFilter),
		CanInject:        checkPermission(value, AuthCanInject),
		CanMakeKeys:      checkPermission(value, AuthCanMakeKeys),
		CanDuplicateKeys: checkPermission(value, AuthCanDuplicateKeys),
		Admin:            value == int(AuthAll),
	})
}
