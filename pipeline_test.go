package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buildkite/bintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockGeneratePipeline(steps []Step, plugin Plugin) (*os.File, bool, error) {
	mockFile, _ := os.Create("pipeline.txt")
	defer mockFile.Close()

	return mockFile, true, nil
}

func TestUploadPipelineCallsBuildkiteAgentCommand(t *testing.T) {
	plugin := Plugin{Diff: "echo ./foo-service", Interpolation: true}

	agent, err := bintest.NewMock("buildkite-agent")
	if err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })
	os.Setenv("PATH", filepath.Dir(agent.Path)+":"+oldPath)

	agent.
		Expect("pipeline", "upload", "pipeline.txt").
		AndExitWith(0)

	cmd, args, err := uploadPipeline(plugin, mockGeneratePipeline)

	assert.Equal(t, "buildkite-agent", cmd)
	assert.Equal(t, []string{"pipeline", "upload", "pipeline.txt"}, args)
	assert.NoError(t, err)

	agent.CheckAndClose(t)
}

func TestUploadPipelineCallsBuildkiteAgentCommandWithInterpolation(t *testing.T) {
	plugin := Plugin{Diff: "echo ./foo-service", Interpolation: false}

	agent, err := bintest.NewMock("buildkite-agent")
	if err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })
	os.Setenv("PATH", filepath.Dir(agent.Path)+":"+oldPath)

	agent.
		Expect("pipeline", "upload", "pipeline.txt", "--no-interpolation").
		AndExitWith(0)

	cmd, args, err := uploadPipeline(plugin, mockGeneratePipeline)

	assert.Equal(t, "buildkite-agent", cmd)
	assert.Equal(t, []string{"pipeline", "upload", "pipeline.txt", "--no-interpolation"}, args)
	assert.NoError(t, err)

	agent.CheckAndClose(t)
}

func TestUploadPipelineCancelsIfThereIsNoDiffOutput(t *testing.T) {
	plugin := Plugin{Diff: "echo"}
	cmd, args, err := uploadPipeline(plugin, mockGeneratePipeline)

	assert.Equal(t, "", cmd)
	assert.Equal(t, []string{}, args)
	assert.Equal(t, nil, err)
}

func TestUploadPipelineWithEmptyGeneratedPipeline(t *testing.T) {
	plugin := Plugin{Diff: "echo ./bar-service"}
	cmd, args, err := uploadPipeline(plugin, generatePipeline)

	assert.Equal(t, "", cmd)
	assert.Equal(t, []string{}, args)
	assert.Equal(t, nil, err)
}

func TestDiff(t *testing.T) {
	want := []string{
		"services/foo/serverless.yml",
		"services/bar/config.yml",
		"ops/bar/config.yml",
		"README.md",
	}

	got, err := diff(`echo services/foo/serverless.yml
services/bar/config.yml

ops/bar/config.yml
README.md`)

	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestDiffWithSubshell(t *testing.T) {
	want := []string{
		"user-service/infrastructure/cloudfront.yaml",
		"user-service/serverless.yaml",
	}
	got, err := diff("echo $(cat e2e/multiple-paths)")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestPipelinesToTriggerGetsListOfPipelines(t *testing.T) {
	want := []string{"service-1", "service-2", "service-4"}

	watch := []WatchConfig{
		{
			Paths: []string{"watch-path-1"},
			Step:  Step{Trigger: "service-1"},
		},
		{
			Paths: []string{"watch-path-2/", "watch-path-3/", "watch-path-4"},
			Step:  Step{Trigger: "service-2"},
		},
		{
			Paths: []string{"watch-path-5"},
			Step:  Step{Trigger: "service-3"},
		},
		{
			Paths: []string{"watch-path-2"},
			Step:  Step{Trigger: "service-4"},
		},
	}

	changedFiles := []string{
		"watch-path-1/text.txt",
		"watch-path-2/.gitignore",
		"watch-path-2/src/index.go",
		"watch-path-4/test/index_test.go",
	}

	pipelines, err := stepsToTrigger(changedFiles, watch)
	assert.NoError(t, err)
	var got []string

	for _, v := range pipelines {
		got = append(got, v.Trigger)
	}

	assert.Equal(t, want, got)
}

func TestPipelinesStepsToTrigger(t *testing.T) {
	testCases := map[string]struct {
		ChangedFiles []string
		WatchConfigs []WatchConfig
		Expected     []Step
	}{
		"service-1": {
			ChangedFiles: []string{
				"watch-path-1/text.txt",
				"watch-path-2/.gitignore",
			},
			WatchConfigs: []WatchConfig{{
				Paths: []string{"watch-path-1"},
				Step:  Step{Trigger: "service-1"},
			}},
			Expected: []Step{
				{Trigger: "service-1"},
			},
		},
		"service-1-2": {
			ChangedFiles: []string{
				"watch-path-1/text.txt",
				"watch-path-2/.gitignore",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"watch-path-1"},
					Step:  Step{Trigger: "service-1"},
				},
				{
					Paths: []string{"watch-path-2"},
					Step:  Step{Trigger: "service-2"},
				},
			},
			Expected: []Step{
				{Trigger: "service-1"},
				{Trigger: "service-2"},
			},
		},
		"extension wildcard": {
			ChangedFiles: []string{
				"text.txt",
				".gitignore",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"*.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
		"extension wildcard in subdir": {
			ChangedFiles: []string{
				"docs/text.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"docs/*.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
		"directory wildcard": {
			ChangedFiles: []string{
				"docs/text.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"**/text.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
		"directory and extension wildcard": {
			ChangedFiles: []string{
				"package/other.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"*/*.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
		"double directory and extension wildcard": {
			ChangedFiles: []string{
				"package/docs/other.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"**/*.txt"},
					Step:  Step{Trigger: "txt"},
				},
			},
			Expected: []Step{
				{Trigger: "txt"},
			},
		},
		"default configuration": {
			ChangedFiles: []string{
				"unmatched/file.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"app/"},
					Step:  Step{Trigger: "app-deploy"},
				},
				{
					Paths: []string{"test/bin/"},
					Step:  Step{Command: "echo Make Changes to Bin"},
				},
				{
					Default: struct{}{},
					Step:    Step{Command: "buildkite-agent pipeline upload other_tests.yml"},
				},
			},
			Expected: []Step{
				{Command: "buildkite-agent pipeline upload other_tests.yml"},
			},
		},
		"skips service-2": {
			ChangedFiles: []string{
				"watch-path/text.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"watch-path"},
					Step:  Step{Trigger: "service-1"},
				},
				{
					Paths:     []string{"watch-path"},
					SkipPaths: []string{"watch-path/text.txt"},
					Step:      Step{Trigger: "service-2"},
				}},
			Expected: []Step{
				{Trigger: "service-1"},
			},
		},
		"skips extension wildcard": {
			ChangedFiles: []string{
				"text.secret.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"*.txt"},
					Step:  Step{Trigger: "service-1"},
				},
				{
					Paths:     []string{"*.txt"},
					SkipPaths: []string{"*.secret.txt"},
					Step:      Step{Trigger: "service-2"},
				}},
			Expected: []Step{
				{Trigger: "service-1"},
			},
		},
		"skips extension wildcard in subdir": {
			ChangedFiles: []string{
				"docs/text.secret.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"**/*.txt"},
					Step:  Step{Trigger: "service-1"},
				},
				{
					Paths:     []string{"**/*.txt"},
					SkipPaths: []string{"docs/*.txt"},
					Step:      Step{Trigger: "service-2"},
				}},
			Expected: []Step{
				{Trigger: "service-1"},
			},
		},
		"step is not included if one of the files is skipped": {
			ChangedFiles: []string{
				"docs/text.secret.txt",
				"docs/text.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					Paths: []string{"**/*.txt"},
					Step:  Step{Trigger: "service-1"},
				},
				{
					Paths:     []string{"**/*.txt"},
					SkipPaths: []string{"docs/*.secret.txt"},
					Step:      Step{Trigger: "service-2"},
				}},
			Expected: []Step{
				{Trigger: "service-1"},
			},
		},
		"fails if not path is included": {
			ChangedFiles: []string{
				"docs/text.txt",
			},
			WatchConfigs: []WatchConfig{
				{
					SkipPaths: []string{"docs/*.secret.txt"},
					Step:      Step{Trigger: "service-1"},
				}},
			Expected: []Step{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			steps, err := stepsToTrigger(tc.ChangedFiles, tc.WatchConfigs)
			assert.NoError(t, err)
			assert.Equal(t, tc.Expected, steps)
		})
	}
}

func TestGeneratePipeline(t *testing.T) {
	steps := []Step{
		{
			Trigger:  "foo-service-pipeline",
			Build:    Build{Message: "build message"},
			SoftFail: true,
			Notify: []StepNotify{
				{Slack: "@adikari"},
			},
		},
		{
			Trigger: "notification-test",
			Command: "command-to-run",
			Notify: []StepNotify{
				{Basecamp: "https://basecamp-url"},
				{GithubStatus: GithubStatusNotification{Context: "my-custom-status"}},
				{Slack: "@someuser", Condition: "build.state === \"passed\""},
			},
		},
		{
			Group:   "my group",
			Trigger: "foo-service-pipeline",
			Build:   Build{Message: "build message"},
		},
	}

	plugin := Plugin{
		Wait: true,
		Notify: []PluginNotify{
			{Email: "foo@gmail.com"},
			{Email: "bar@gmail.com"},
			{Basecamp: "https://basecamp"},
			{Webhook: "https://webhook"},
			{Slack: "@adikari"},
			{GithubStatus: GithubStatusNotification{Context: "github-context"}},
		},
		Hooks: []HookConfig{
			{Command: "echo \"hello world\""},
			{Command: "cat ./file.txt"},
		},
	}

	pipeline, _, err := generatePipeline(steps, plugin)

	require.NoError(t, err)
	defer os.Remove(pipeline.Name())

	got, err := os.ReadFile(pipeline.Name())
	require.NoError(t, err)

	want := `notify:
- email: foo@gmail.com
- email: bar@gmail.com
- basecamp_campfire: https://basecamp
- webhook: https://webhook
- slack: '@adikari'
- github_commit_status:
    context: github-context
steps:
- trigger: foo-service-pipeline
  build:
    message: build message
  soft_fail: true
  notify:
  - slack: '@adikari'
- trigger: notification-test
  command: command-to-run
  notify:
  - basecamp_campfire: https://basecamp-url
  - github_commit_status:
      context: my-custom-status
  - slack: '@someuser'
    if: build.state === "passed"
- group: my group
  steps:
  - trigger: foo-service-pipeline
    build:
      message: build message
- wait: null
- command: echo "hello world"
- command: cat ./file.txt
`

	assert.Equal(t, want, string(got))
}

func TestGeneratePipelineWithNoStepsAndHooks(t *testing.T) {
	steps := []Step{}

	want := `steps:
- wait: null
- command: echo "hello world"
- command: cat ./file.txt
`

	plugin := Plugin{
		Wait: true,
		Hooks: []HookConfig{
			{Command: "echo \"hello world\""},
			{Command: "cat ./file.txt"},
		},
	}

	pipeline, _, err := generatePipeline(steps, plugin)
	require.NoError(t, err)
	defer os.Remove(pipeline.Name())

	got, err := os.ReadFile(pipeline.Name())
	require.NoError(t, err)

	assert.Equal(t, want, string(got))
}

func TestGeneratePipelineWithNoStepsAndNoHooks(t *testing.T) {
	steps := []Step{}

	want := `steps: []
`

	plugin := Plugin{}

	pipeline, _, err := generatePipeline(steps, plugin)
	require.NoError(t, err)
	defer os.Remove(pipeline.Name())

	got, err := os.ReadFile(pipeline.Name())
	require.NoError(t, err)

	assert.Equal(t, want, string(got))
}
