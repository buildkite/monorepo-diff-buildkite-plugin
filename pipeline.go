package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	"github.com/bmatcuk/doublestar/v2"
	"github.com/buildkite/go-pipeline"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// PipelineGenerator generates pipeline file
type PipelineGenerator func(steps []pipeline.Step, plugin Plugin) (*os.File, bool, error)

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

	p, hasSteps, err := generatePipeline(steps, plugin)
	defer os.Remove(p.Name())

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
	args := []string{"pipeline", "upload", p.Name()}
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

func stepsToTrigger(files []string, watch []WatchConfig) ([]pipeline.Step, error) {
	steps := []pipeline.Step{}

	for _, w := range watch {
		for _, p := range w.Paths {
			for _, f := range files {
				match, err := matchPath(p, f)
				if err != nil {
					return nil, err
				}
				if match {
					out, err := executeCommand("bash", []string{"-c", w.Generator})
					if err != nil {
						return nil, err
					}

					p, err := pipeline.Parse(strings.NewReader(out))
					if err != nil {
						return nil, err
					}

					for _, step := range p.Steps {
						steps = append(steps, step)
					}

					break
				}
			}
		}
	}

	return dedupSteps(steps), nil
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

func dedupSteps(steps []pipeline.Step) []pipeline.Step {
	unique := []pipeline.Step{}
	for _, p := range steps {
		duplicate := false
		for _, t := range unique {
			if reflect.DeepEqual(p, t) {
				duplicate = true
				break
			}
		}

		if !duplicate {
			unique = append(unique, p)
		}
	}

	return unique
}

func generatePipeline(steps []pipeline.Step, plugin Plugin) (*os.File, bool, error) {
	tmp, err := ioutil.TempFile(os.TempDir(), "bmrd-")
	if err != nil {
		return nil, false, fmt.Errorf("could not create temporary pipeline file: %v", err)
	}

	yamlSteps := make([]interface{}, 0)

	for _, step := range steps {
		yamlSteps = append(yamlSteps, step)
	}

	pipeline := map[string]interface{}{
		"steps": yamlSteps,
	}

	data, err := yaml.Marshal(&pipeline)
	if err != nil {
		return nil, false, fmt.Errorf("could not serialize the pipeline: %v", err)
	}

	// Disable logging in context of go tests.
	if env("TEST_MODE", "") != "true" {
		fmt.Printf("Generated Pipeline:\n%s\n", string(data))
	}

	if err = ioutil.WriteFile(tmp.Name(), data, 0644); err != nil {
		return nil, false, fmt.Errorf("could not write step to temporary file: %v", err)
	}

	return tmp, len(yamlSteps) > 0, nil
}
