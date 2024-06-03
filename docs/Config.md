# Configuration

Config is in YAML, the defaults are shown below

```yaml
# Config for EzProxy 2.2r2
# Proxy server address
ProxyAddress:
  # IP Address to use as proxy
  # Leave empty to use local
  Address: &LocalAddress ""
  # Proxy port
  Port: 5554

# Server address
ServerAddress:
  # Server address to connect to as server
  Address: *LocalAddress
  # Server port
  Port: 5555

# Logging info
Logging:
  # Must be "debug", "info", "warn" or "error"
  Level: warn

# API info
Api:
  # Should the API be used, settings this to false disables the API and websocket.
  Enable: true
  # Should authentication be used.
  UseAuth: true
  Address: 
    Address: *LocalAddress
    Port: 8080

# LUA settings - EZP must have been compiled with the 'lua_binding' build tag
# See docs/lua.md
Lua:
  # Should Lua be used
  Enable: false
  # Path of the lua file to execute
  Path:
  # Mode of exection
  #   main: Run EzpMain
  #   callback: Run callbacks on actions
  Mode: main

# Debug settings
Debug:
  # Set to true to enable debugging features
  # This SHOULD NOT be true for production.
  Enable: false
  # Constant API key to set, only used if 'debug' is true
  # default: 0
  ApiKey: 0
```