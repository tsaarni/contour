package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var filesToPatchWithEnvoyVersion = []string{
	"Makefile",
	"cmd/contour/gatewayprovisioner.go",
	"examples/contour/03-envoy.yaml",
	"examples/deployment/03-envoy-deployment.yaml",
}

type EnvoyUpdater struct {
	targetVersion string
}

func (u *EnvoyUpdater) getLatestEnvoyTag(releaseTrack string) error {
	// http https://api.github.com/repos/envoyproxy/envoy/releases | jq -r ".[].tag_name"
	url := "https://api.github.com/repos/envoyproxy/envoy/releases"
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to get releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	for _, release := range releases {
		if strings.HasPrefix(release.TagName, releaseTrack+".") {
			u.targetVersion = release.TagName
			return nil
		}
	}

	return fmt.Errorf("no matching release found for track: %s", releaseTrack)
}

func (u *EnvoyUpdater) updateEnvoyImage() error {

	pattern := regexp.MustCompile(`docker\.io/envoyproxy/envoy:v[0-9]+\.[0-9]+\.[0-9]+`)

	for _, filePath := range filesToPatchWithEnvoyVersion {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		newContent := pattern.ReplaceAllString(string(content), fmt.Sprintf("docker.io/envoyproxy/envoy:%s", u.targetVersion))

		err = os.WriteFile(filePath, []byte(newContent), 0644)
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}
	}

	slog.Info("Running make generate to update generated files")
	cmd := exec.Command("make", "generate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run make generate: %w", err)
	}

	return nil
}

func (u *EnvoyUpdater) GetCommitMessage() (string, error) {
	if u.targetVersion == "" {
		return "", fmt.Errorf("target version not set")
	}
	message := fmt.Sprintf("Automated bump for Envoy %s.\n\nSee the release notes: https://github.com/envoyproxy/envoy/releases/tag/%s", u.targetVersion, u.targetVersion)
	return message, nil
}

func (u *EnvoyUpdater) GetRelease() (string, error) {
	if u.targetVersion == "" {
		return "", fmt.Errorf("target version not set")
	}
	return u.targetVersion, nil
}

func (u *EnvoyUpdater) Process(releaseTrace string) error {
	err := u.getLatestEnvoyTag(releaseTrace)
	if err != nil {
		return fmt.Errorf("error getting latest version: %w", err)
	}
	slog.Info("Latest version", "component", "envoy", "releaseTrack", releaseTrace, "version", u.targetVersion)

	err = u.updateEnvoyImage()
	if err != nil {
		return fmt.Errorf("error updating code: %w", err)
	}
	return nil
}
