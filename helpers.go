package main

import (
	"context"
	"ezproxy/api"
	"ezproxy/handler"
	"ezproxy/proxy"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	versionExperimental uint32 = 0x80000000
	proxyVersion        uint32 = 0x02_02_01
	apiVersion          uint32 = 0x02_01_02
)

func versionToString(v uint32) string {
	// 0: Bitfield
	//
	//	0: Experimental
	//
	// 1:2: Reserved
	// 2:4: Major version (Major features added, breaking changes)
	// 4:6: Minor version (Minor features added, no breaking changes)
	// 6:8: Revision      (Bug fixes, Performance changes)
	suffix := ""
	if v&versionExperimental != 0 {
		suffix = "e"
	}
	major := v >> 16 & 0xff
	minor := v >> 8 & 0xff
	rev := v & 0xff
	return fmt.Sprintf("%d.%dr%d%s", major, minor, rev, suffix)
}

func getLocalIp() (net.Addr, error) {
	c, err := net.Dial("udp", "192.168.0.254:1234")
	if err != nil {
		return nil, err
	}
	return c.LocalAddr(), nil
}

type ConfigAddress struct {
	Address string `yaml:"Address"` // Leave empty to use local address
	Port    uint16 `yaml:"Port"`
}

func (c *ConfigAddress) IsEmpty() bool {
	return c.Address != "" || c.Port != 0
}

func (c *ConfigAddress) ToString() string {
	if c.Address == "" {
		addr, err := getLocalIp()
		if err != nil {
			slog.Default().Error("Failed to get local IP address", "Error", err.Error())
			panic("Failed to get local IP address")
		}
		ip := strings.Split(addr.String(), ":")[0]
		return fmt.Sprintf("%s:%d", ip, c.Port)
	}
	return fmt.Sprintf("%s:%d", c.Address, c.Port)
}

type ConfigLogging struct {
	Level string `yaml:"Level"`
}

func (c *ConfigLogging) LevelToSlog() slog.Level {
	switch c.Level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		panic(fmt.Sprintf("logging level is unknown: '%s'", c.Level))
	}
}

type ConfigDebug struct {
	Enable bool   `yaml:"Enable"`
	ApiKey uint64 `yaml:"ApiKey"`
}

type ConfigApi struct {
	Enable  bool          `yaml:"Enable"`
	UseAuth bool          `yaml:"UseAuth"`
	Address ConfigAddress `yaml:"Address"`
}

type ConfigLua struct {
	Enable bool   `yaml:"Enable"`
	Path   string `yaml:"Path"`
	Mode   string `yaml:"Mode"`
}

type ConfigData struct {
	ProxyAddress  ConfigAddress `yaml:"ProxyAddress"`
	ServerAddress ConfigAddress `yaml:"ServerAddress"`
	Api           ConfigApi     `yaml:"Api"`
	Logging       ConfigLogging `yaml:"Logging"`
	Lua           ConfigLua     `yaml:"Lua"`
	Debug         ConfigDebug   `yaml:"Debug"`
}

func (c *ConfigData) IsEmpty() bool {
	if !c.ProxyAddress.IsEmpty() {
		return false
	}
	if !c.ServerAddress.IsEmpty() {
		return false
	}
	if !c.Api.Address.IsEmpty() || c.Api.Enable || c.Api.UseAuth {
		return false
	}
	if c.Logging.Level != "" {
		return false
	}
	if c.Debug.Enable || c.Debug.ApiKey != 0 {
		return false
	}
	return true
}

func loadCfg() *ConfigData {
	cfgData, err := os.ReadFile("config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to to open 'config.yaml': %v", err)
		return nil
	}
	cfg := &ConfigData{}
	//err = json.Unmarshal(cfgData, cfg)
	err = yaml.Unmarshal(cfgData, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to unmarshal config: %v", err)
		return nil
	}
	if cfg.IsEmpty() {
		fmt.Fprintf(os.Stderr, "Config is empty\n")
		return nil
	}
	if cfg.Debug.Enable {
		// Print this EVERYWHERE. This is a SERIOUS warning, as having this set will use the ApiKey configured. This is BAD.
		fmt.Fprintf(os.Stderr, "Debug mode is enabled, DO NOT USE THIS IN PRODUCTION.\n")
		fmt.Printf("Debug mode is enabled, DO NOT USE THIS IN PRODUCTION.\n")
	}
	return cfg
}

func setupLogger(cfg *ConfigData) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	logger.Info("Starting EzProxy",
		"ProxyVersion", proxyVersion,
		"ProxyVersionStr", versionToString(proxyVersion),
		"ApiVersion", apiVersion,
		"ApiVersionStr", versionToString(apiVersion),
	)
	file, err := os.OpenFile("log", os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		panic(err)
	}
	logger = slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{
		AddSource: true,
		Level:     cfg.Logging.LevelToSlog(),
	}))
	slog.SetDefault(logger)
}

func setupSpawnerAndApi(cfg *ConfigData) handler.IProxySpawner {
	logger := slog.Default()
	pxAddr, err := net.ResolveTCPAddr("tcp", cfg.ProxyAddress.ToString())
	if err != nil {
		logger.Error("Failed to resolve proxy address", "Error", err.Error(), "Address", cfg.ProxyAddress.ToString())
		return nil
	}
	svAddr, err := net.ResolveTCPAddr("tcp", cfg.ServerAddress.ToString())
	if err != nil {
		logger.Error("Failed to resolve server address", "Error", err.Error(), "Address", cfg.ServerAddress.ToString())
		return nil
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	logger.Debug("Setup proxySpawner", "Server", svAddr.String(), "Proxy", pxAddr.String())
	ps, err := handler.NewProxySpawner(svAddr, pxAddr, ctx, proxy.TcpListener, proxy.UdpListener)
	if err != nil {
		logger.Error("Failed to create ProxySpawner", "Error", err.Error())
		cancel(err)
		return nil
	}
	ps.SetErrorCallback(func(err error, pc handler.IProxyContainer) {
		if pc == nil {
			logger.Error("Spawner error", "Error", err.Error())
		} else {
			logger.Error("Proxy error", "Id", pc.GetId(), "Network", pc.Network(), "Error", err.Error())
		}
	})
	if cfg.Api.Enable {
		web := api.NewWebApi(http.DefaultServeMux, cfg.Api.UseAuth, ps)
		if cfg.Debug.Enable && cfg.Api.UseAuth {
			logger.Info("Adding debug api key", "Key", cfg.Debug.ApiKey)
			err = web.AddAuth(cfg.Debug.ApiKey, api.AuthAll)
			if err != nil {
				logger.Error("Failed to add default admin auth", "Error", err.Error)
				cancel(err)
				return nil
			}
		}
		logger.Debug("Starting API", "Address", cfg.Api.Address.ToString())
		go http.ListenAndServe(cfg.Api.Address.ToString(), nil)
	}
	return ps
}

func run(px handler.IProxySpawner) {
	psCtx := px.GetContext()
	<-psCtx.Done()
}
