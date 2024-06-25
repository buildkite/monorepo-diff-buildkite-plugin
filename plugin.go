package main

import (
	"encoding/json"
	"errors"
	"strings"

	log "github.com/sirupsen/logrus"
)

const pluginName = "github.com/davidarcher/monorepo-diff"

// Plugin buildkite monorepo diff plugin structure
type Plugin struct {
	Diff     string
	LogLevel string `json:"log_level"`
	Watch    []WatchConfig
}

// WatchConfig Plugin watch configuration
type WatchConfig struct {
	RawPath   interface{} `json:"path"`
	Paths     []string
	Generator string `json:"generator"`
}

// UnmarshalJSON set defaults properties
func (plugin *Plugin) UnmarshalJSON(data []byte) error {
	type plain Plugin

	def := &plain{
		Diff:     "git diff --name-only HEAD~1",
		LogLevel: "info",
	}

	_ = json.Unmarshal(data, def)

	*plugin = Plugin(*def)

	// Path can be string or an array of strings,
	// handle both cases and create an array of paths.
	for i, p := range plugin.Watch {
		switch p.RawPath.(type) {
		case string:
			plugin.Watch[i].Paths = []string{plugin.Watch[i].RawPath.(string)}
		case []interface{}:
			for _, v := range plugin.Watch[i].RawPath.([]interface{}) {
				plugin.Watch[i].Paths = append(plugin.Watch[i].Paths, v.(string))
			}
		}

		p.RawPath = nil
	}

	return nil
}

func initializePlugin(data string) (Plugin, error) {
	log.Debugf("parsing plugin config: %v", data)

	var pluginConfigs []map[string]json.RawMessage

	if err := json.Unmarshal([]byte(data), &pluginConfigs); err != nil {
		log.Debug(err)
		return Plugin{}, errors.New("failed to parse plugin configuration")
	}

	for _, p := range pluginConfigs {
		for key, pluginConfig := range p {
			if strings.HasPrefix(key, pluginName) {
				var plugin Plugin

				if err := json.Unmarshal(pluginConfig, &plugin); err != nil {
					log.Debug(err)
					return Plugin{}, errors.New("failed to parse plugin configuration")
				}

				return plugin, nil
			}
		}
	}

	return Plugin{}, errors.New("could not initialize plugin")
}
