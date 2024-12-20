package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/bmatcuk/doublestar/v2"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// WaitStep represents a Buildkite Wait Step
// https://buildkite.com/docs/pipelines/wait-step
// We can't use Step here since the value for Wait is always nil
// regardless of whether or not we want to include the key.
type WaitStep struct{}

func (WaitStep) MarshalYAML() (interface{}, error) {
	return map[string]interface{}{
		"wait": nil,
	}, nil
}

func (s Step) MarshalYAML() (interface{}, error) {
	if s.Group == "" {
		type Alias Step
		return (Alias)(s), nil
	}

	label := s.Group
	s.Group = ""
	return Group{Label: label, Steps: []Step{s}}, nil
}

func (n PluginNotify) MarshalYAML() (interface{}, error) {
	return n, nil
}

// PipelineGenerator generates pipeline file
type PipelineGenerator func(steps []Step, plugin Plugin) (*os.File, bool, error)

func uploadPipeline(plugin Plugin, generatePipeline PipelineGenerator) (string, []string, error) {
	diffOutput, err := diff(plugin.Diff)
	if err != nil {
		log.Fatal(err)
		return "", []string{}, err
	}

	if len(diffOutput) < 1 {
		log.Info("No changes detected. Skipping pipeline upload.")
		return "", []string{}, nil
	}

	log.Debug("Output from diff: \n" + strings.Join(diffOutput, "\n"))

	steps, err := stepsToTrigger(diffOutput, plugin.Watch)
	if err != nil {
		return "", []string{}, err
	}

	pipeline, hasSteps, err := generatePipeline(steps, plugin)
	defer os.Remove(pipeline.Name())

	if err != nil {
		log.Error(err)
		return "", []string{}, err
	}

	if !hasSteps {
		// Handle the case where no steps were provided
		log.Info("No steps generated. Skipping pipeline upload.")
		return "", []string{}, nil
	}

	cmd := "buildkite-agent"
	args := []string{"pipeline", "upload", pipeline.Name()}

	if !plugin.Interpolation {
		args = append(args, "--no-interpolation")
	}

	_, err = executeCommand("buildkite-agent", args)

	return cmd, args, err
}

func diff(command string) ([]string, error) {
	log.Infof("Running diff command: %s", command)

	output, err := executeCommand(
		env("SHELL", "bash"),
		[]string{"-c", strings.Replace(command, "\n", " ", -1)},
	)
	if err != nil {
		return nil, fmt.Errorf("diff command failed: %v", err)
	}

	return strings.Fields(strings.TrimSpace(output)), nil
}

func stepsToTrigger(files []string, watch []WatchConfig) ([]Step, error) {
	steps := []Step{}
	var defaultStep *Step

	for _, w := range watch {
		if w.Default != nil {
			defaultStep = &w.Step
			continue
		}

		match := false
		skip := false
		for _, f := range files {
			for _, p := range w.Paths {

				m, err := matchPath(p, f)
				if err != nil {
					return nil, err
				}

				if m {
					log.Printf("matched: %s\n", f)
					match = true
					break
				}
			}

			for _, sp := range w.SkipPaths {

				sm, err := matchPath(sp, f)
				if err != nil {
					return nil, err
				}

				if sm {
					log.Printf("skipped: %s\n", f)
					skip = true
					break
				}
			}
		}

		if match && !skip {
			log.Printf("adding step: %s\n", w.Step.Trigger)
			steps = append(steps, w.Step)
		}
	}

	if len(steps) == 0 && defaultStep != nil {
		steps = append(steps, *defaultStep)
	}

	return steps, nil
}

// matchPath checks if the file f matches the path p.
func matchPath(p string, f string) (bool, error) {
	// If the path contains a glob, the `doublestar.Match`
	// method is used to determine the match,
	// otherwise `strings.HasPrefix` is used.
	if strings.Contains(p, "*") {
		match, err := doublestar.Match(p, f)
		if err != nil {
			return false, fmt.Errorf("path matching failed: %v", err)
		}
		if match {
			return true, nil
		}
	}
	if strings.HasPrefix(f, p) {
		return true, nil
	}
	return false, nil
}

func generatePipeline(steps []Step, plugin Plugin) (*os.File, bool, error) {
	tmp, err := os.CreateTemp(os.TempDir(), "bmrd-")
	if err != nil {
		return nil, false, fmt.Errorf("could not create temporary pipeline file: %v", err)
	}

	yamlSteps := make([]yaml.Marshaler, len(steps))

	for i, step := range steps {
		yamlSteps[i] = step
	}

	if plugin.Wait {
		yamlSteps = append(yamlSteps, WaitStep{})
	}

	for _, cmd := range plugin.Hooks {
		yamlSteps = append(yamlSteps, Step{Command: cmd.Command})
	}

	yamlNotify := make([]yaml.Marshaler, len(plugin.Notify))
	for i, n := range plugin.Notify {
		yamlNotify[i] = n
	}

	pipeline := map[string][]yaml.Marshaler{
		"steps": yamlSteps,
	}

	if len(yamlNotify) > 0 {
		pipeline["notify"] = yamlNotify
	}

	data, err := yaml.Marshal(&pipeline)
	if err != nil {
		return nil, false, fmt.Errorf("could not serialize the pipeline: %v", err)
	}

	// Disable logging in context of go tests.
	if env("TEST_MODE", "") != "true" {
		fmt.Printf("Generated Pipeline:\n%s\n", string(data))
	}

	if err = os.WriteFile(tmp.Name(), data, 0o644); err != nil {
		return nil, false, fmt.Errorf("could not write step to temporary file: %v", err)
	}

	// Returns the temporary file and a boolean indicating whether or not the pipeline has steps
	if len(yamlSteps) == 0 {
		return tmp, false, nil
	} else {
		return tmp, true, nil
	}
}
