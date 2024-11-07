package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"strings"

	log "github.com/sirupsen/logrus"
)

const pluginName = "monorepo-diff"

// Plugin buildkite monorepo diff plugin structure
type Plugin struct {
	Diff          string
	Wait          bool
	LogLevel      string `json:"log_level"`
	Interpolation bool
	Hooks         []HookConfig
	Watch         []WatchConfig
	RawEnv        interface{} `json:"env"`
	Env           map[string]string
	RawNotify     []map[string]interface{} `json:"notify" yaml:",omitempty"`
	Notify        []PluginNotify           `yaml:"notify,omitempty"`
}

// HookConfig Plugin hook configuration
type HookConfig struct {
	Command string
}

// WatchConfig Plugin watch configuration
type WatchConfig struct {
	RawPath     interface{} `json:"path"`
	Paths       []string
	Step        Step        `json:"config"`
	Default     interface{} `json:"default"`
	RawSkipPath interface{} `json:"skip_path"`
	SkipPaths   []string
}

type Group struct {
	Label string `yaml:"group"`
	Steps []Step `yaml:"steps"`
}

// GithubStatusNotification is notification config for github_commit_status
type GithubStatusNotification struct {
	Context string `yaml:"context,omitempty"`
}

// PluginNotify is notify configuration for pipeline
type PluginNotify struct {
	Slack        string                   `yaml:"slack,omitempty"`
	Email        string                   `yaml:"email,omitempty"`
	PagerDuty    string                   `yaml:"pagerduty_change_event,omitempty"`
	Webhook      string                   `yaml:"webhook,omitempty"`
	Basecamp     string                   `yaml:"basecamp_campfire,omitempty"`
	GithubStatus GithubStatusNotification `yaml:"github_commit_status,omitempty"`
	Condition    string                   `yaml:"if,omitempty"`
}

// Notify is Buildkite notification definition
type StepNotify struct {
	Slack        string                   `yaml:"slack,omitempty"`
	Basecamp     string                   `yaml:"basecamp_campfire,omitempty"`
	GithubStatus GithubStatusNotification `yaml:"github_commit_status,omitempty"`
	Condition    string                   `yaml:"if,omitempty"`
}

// Step is buildkite pipeline definition
type Step struct {
	Group     string                   `yaml:"group,omitempty"`
	Trigger   string                   `yaml:"trigger,omitempty"`
	Label     string                   `yaml:"label,omitempty"`
	Build     Build                    `yaml:"build,omitempty"`
	Command   interface{}              `yaml:"command,omitempty"`
	Commands  interface{}              `yaml:"commands,omitempty"`
	Agents    Agent                    `yaml:"agents,omitempty"`
	Artifacts []string                 `yaml:"artifacts,omitempty"`
	RawEnv    interface{}              `json:"env" yaml:",omitempty"`
	Env       map[string]string        `yaml:"env,omitempty"`
	Async     bool                     `yaml:"async,omitempty"`
	SoftFail  interface{}              `json:"soft_fail" yaml:"soft_fail,omitempty"`
	RawNotify []map[string]interface{} `json:"notify" yaml:",omitempty"`
	Notify    []StepNotify             `yaml:"notify,omitempty"`
}

// Agent is Buildkite agent definition
type Agent map[string]string

// Build is buildkite build definition
type Build struct {
	Message string            `yaml:"message,omitempty"`
	Branch  string            `yaml:"branch,omitempty"`
	Commit  string            `yaml:"commit,omitempty"`
	RawEnv  interface{}       `json:"env" yaml:",omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	// Notify  []Notify          `yaml:"notify,omitempty"`
}

// UnmarshalJSON set defaults properties
func (plugin *Plugin) UnmarshalJSON(data []byte) error {
	type plain Plugin

	def := &plain{
		Diff:          "git diff --name-only HEAD~1",
		Wait:          false,
		LogLevel:      "info",
		Interpolation: true,
	}

	_ = json.Unmarshal(data, def)

	*plugin = Plugin(*def)

	parseResult, err := parseEnv(plugin.RawEnv)
	if err != nil {
		return errors.New("failed to parse plugin configuration")
	}

	plugin.Env = parseResult
	plugin.RawEnv = nil

	setPluginNotify(&plugin.Notify, &plugin.RawNotify)

	// Path and SkipPath can be string or an array of strings,
	// handle both cases and create an array of paths on both.
	for i, p := range plugin.Watch {
		if p.Default != nil {
			plugin.Watch[i].Paths = []string{}
			if config, ok := p.Default.(map[string]interface{}); ok && len(config) > 0 {
				// Use the default config directly
				b, _ := json.Marshal(config)
				err := json.Unmarshal(b, &plugin.Watch[i].Step)
				if err != nil {
					return err
				}
			}
		} else if p.RawPath != nil {
			switch p.RawPath.(type) {
			case string:
				plugin.Watch[i].Paths = []string{plugin.Watch[i].RawPath.(string)}
			case []interface{}:
				for _, v := range plugin.Watch[i].RawPath.([]interface{}) {
					plugin.Watch[i].Paths = append(plugin.Watch[i].Paths, v.(string))
				}
			}
		}

		switch p.RawSkipPath.(type) {
		case string:
			plugin.Watch[i].SkipPaths = []string{plugin.Watch[i].RawSkipPath.(string)}
		case []interface{}:
			for _, v := range plugin.Watch[i].RawSkipPath.([]interface{}) {
				plugin.Watch[i].SkipPaths = append(plugin.Watch[i].SkipPaths, v.(string))
			}
		}

		if plugin.Watch[i].Step.Trigger != "" {
			setBuild(&plugin.Watch[i].Step.Build)
		}

		if plugin.Watch[i].Step.RawNotify != nil {
			setNotify(&plugin.Watch[i].Step.Notify, &plugin.Watch[i].Step.RawNotify)
		}

		appendEnv(&plugin.Watch[i], plugin.Env)

		p.RawPath = nil
		p.RawSkipPath = nil
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
			if strings.HasPrefix(getPluginName(key), pluginName) {
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

func setPluginNotify(notifications *[]PluginNotify, rawNotify *[]map[string]interface{}) {
	for _, v := range *rawNotify {
		var notify PluginNotify

		if condition, ok := isString(v["if"]); ok {
			notify.Condition = condition
		}

		if email, ok := isString(v["email"]); ok {
			notify.Email = email
			*notifications = append(*notifications, notify)
			continue
		}

		if basecamp, ok := isString(v["basecamp_campfire"]); ok {
			notify.Basecamp = basecamp
			*notifications = append(*notifications, notify)
			continue
		}

		if webhook, ok := isString(v["webhook"]); ok {
			notify.Webhook = webhook
			*notifications = append(*notifications, notify)
			continue
		}

		if pagerduty, ok := isString(v["pagerduty_change_event"]); ok {
			notify.PagerDuty = pagerduty
			*notifications = append(*notifications, notify)
			continue
		}

		if slack, ok := isString(v["slack"]); ok {
			notify.Slack = slack
			*notifications = append(*notifications, notify)
			continue
		}

		if github, ok := v["github_commit_status"].(map[string]interface{}); ok {
			if context, ok := isString(github["context"]); ok {
				notify.GithubStatus = GithubStatusNotification{Context: context}
				*notifications = append(*notifications, notify)
			}
			continue
		}
	}

	*rawNotify = nil
}

func setNotify(notifications *[]StepNotify, rawNotify *[]map[string]interface{}) {
	for _, v := range *rawNotify {
		var notify StepNotify

		if condition, ok := isString(v["if"]); ok {
			notify.Condition = condition
		}

		if basecamp, ok := isString(v["basecamp_campfire"]); ok {
			notify.Basecamp = basecamp
			*notifications = append(*notifications, notify)
			continue
		}

		if slack, ok := isString(v["slack"]); ok {
			notify.Slack = slack
			*notifications = append(*notifications, notify)
			continue
		}

		if github, ok := v["github_commit_status"].(map[string]interface{}); ok {
			if context, ok := isString(github["context"]); ok {
				notify.GithubStatus = GithubStatusNotification{Context: context}
				*notifications = append(*notifications, notify)
			}
			continue
		}
	}

	*rawNotify = nil
}

func escapeInterpolation(s string) string {
	return strings.ReplaceAll(s, "$", "$$")
}

func setBuild(build *Build) {
	// when defaulting to existing literal values make sure those values
	// don't trigger interpolation with any stray dollar characters.

	if build.Message == "" {
		build.Message = escapeInterpolation(env("BUILDKITE_MESSAGE", ""))
	}

	if build.Branch == "" {
		build.Branch = escapeInterpolation(env("BUILDKITE_BRANCH", ""))
	}

	if build.Commit == "" {
		build.Commit = escapeInterpolation(env("BUILDKITE_COMMIT", ""))
	}
}

// appends top level env to Step.Env and Step.Build.Env
func appendEnv(watch *WatchConfig, env map[string]string) {
	watch.Step.Env, _ = parseEnv(watch.Step.RawEnv)
	watch.Step.Build.Env, _ = parseEnv(watch.Step.Build.RawEnv)

	for key, value := range env {
		if watch.Step.Command != nil || watch.Step.Commands != nil {
			if watch.Step.Env == nil {
				watch.Step.Env = make(map[string]string)
			}

			watch.Step.Env[key] = value
			continue
		}
		if watch.Step.Trigger != "" {
			if watch.Step.Build.Env == nil {
				watch.Step.Build.Env = make(map[string]string)
			}

			watch.Step.Build.Env[key] = value
			continue
		}
	}

	watch.Step.RawEnv = nil
	watch.Step.Build.RawEnv = nil
	watch.RawPath = nil
	watch.RawSkipPath = nil
}

// parse env in format from env=env-value to map[env] = env-value
func parseEnv(raw interface{}) (map[string]string, error) {
	if raw == nil {
		return nil, nil
	}

	if _, ok := raw.([]interface{}); !ok {
		return nil, errors.New("failed to parse plugin configuration")
	}

	result := make(map[string]string)
	for _, v := range raw.([]interface{}) {
		split := strings.Split(v.(string), "=")
		key, value := strings.TrimSpace(split[0]), split[1:]

		// only key exists. set value from env
		if len(key) > 0 && len(value) == 0 {
			result[key] = env(key, "")
		}

		if len(value) > 0 {
			result[key] = strings.TrimSpace(value[0])
		}
	}

	return result, nil
}

func getPluginName(s string) string {
	ref := s
	if strings.HasPrefix(ref, "github.com/") && !strings.Contains(ref, "://") {
		ref = "https://" + ref
	}

	u, err := url.Parse(ref)
	// if URL could not be parsed, assume it is a direct reference
	if err != nil {
		return s
	}

	// remove the org from the path
	_, repo := path.Split(u.Path)
	return repo
}
