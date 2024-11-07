# Stage 1: Build the Go plugin using Goreleaser
FROM goreleaser/goreleaser:v1.13.1-amd64 as go

WORKDIR /plugin

COPY . .

RUN goreleaser build --rm-dist --skip-validate --config .goreleaser-test.yml

# Stage 2: Lint the plugin using Buildkite plugin-linter
FROM buildkite/plugin-linter:latest as linter

WORKDIR /plugin

# Copy the plugin code from the previous stage to lint it
COPY --from=go /plugin /plugin

# Run the Buildkite plugin linter to validate the plugin
RUN plugin-linter --id buildkite-plugins/monorepo-diff

# Stage 3: Final image for testing the plugin
FROM golang:1.20 as test-stage

WORKDIR /plugin

# Copy the plugin code and artifacts from previous stages
COPY --from=go /plugin /plugin
COPY tests /plugin/tests
COPY hooks /plugin/hooks

# Run the plugin tests
CMD ["go", "test", "-race", "-coverprofile=coverage.out", "-covermode=atomic"]