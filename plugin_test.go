package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPluginWithEmptyParameter(t *testing.T) {
	_, err := initializePlugin("[]")

	assert.EqualError(t, err, "could not initialize plugin")
}

func TestPluginWithInvalidParameter(t *testing.T) {
	_, err := initializePlugin("invalid")

	assert.EqualError(t, err, "failed to parse plugin configuration")
}

func TestPluginShouldHaveDefaultValues(t *testing.T) {
	param := `[{
		"github.com/buildkite-plugins/monorepo-diff-buildkite-plugin#commit": {}
	}]`

	got, _ := initializePlugin(param)

	expected := Plugin{
		Diff:     "git diff --name-only HEAD~1",
		LogLevel: "info",
	}

	assert.Equal(t, expected, got)
}

func TestPluginWithValidParameter(t *testing.T) {
	param := ""
	got, err := initializePlugin(param)
	expected := Plugin{}

	assert.EqualError(t, err, "failed to parse plugin configuration")
	assert.Equal(t, expected, got)
}

func TestPluginShouldUnmarshallCorrectly(t *testing.T) {
	param := `[{
		"github.com/buildkite-plugins/monorepo-diff-buildkite-plugin#commit": {
			"diff": "cat ./hello.txt",
			"log_level": "debug",
			"watch": [
				{
					"path": "watch-path-1",
					"generator": "generate-service-1"
				},
				{
					"path": "watch-path-2",
					"generator": "generate-service-2"
				}
			]
		}
	}]`

	got, _ := initializePlugin(param)

	expected := Plugin{
		Diff:     "cat ./hello.txt",
		LogLevel: "debug",
		Watch: []WatchConfig{
			{
				RawPath:   "watch-path-1",
				Paths:     []string{"watch-path-1"},
				Generator: "generate-service-1",
			},
			{
				RawPath:   "watch-path-2",
				Paths:     []string{"watch-path-2"},
				Generator: "generate-service-2",
			},
		},
	}

	assert.Equal(t, expected, got)
}

func TestPluginShouldOnlyFullyUnmarshallItselfAndNotOtherPlugins(t *testing.T) {
	param := `[
		{
			"github.com/example/example-plugin#commit": {
				"env": {
					"EXAMPLE_TOKEN": {
						"json-key": ".TOKEN",
						"secret-id": "global/example/token"
					}
				}
			}
		},
		{
			"github.com/buildkite-plugins/monorepo-diff-buildkite-plugin#commit": {
				"watch": [
					{
						"env": [
							"EXAMPLE_TOKEN"
						],
						"path": [
							".buildkite/**/*"
						],
						"config": {
							"label": "Example label",
							"command": "echo hello world\\n"
						}
					}
				]
			}
		}
	]
	`
	_, err := initializePlugin(param)
	assert.NoError(t, err)
}
