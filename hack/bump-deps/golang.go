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

var filesToPatchWithGolangVersion = []string{
	"Makefile",
	".github/workflows/build_daily.yaml",
	".github/workflows/build_tag.yaml",
	".github/workflows/codeql-analysis.yml",
	".github/workflows/prbuild.yaml",
}

type GolangUpdater struct {
	targetVersion string
	imageHash     string // The image hash for the golang docker container.
}

type GoDevRelease struct {
	Version string `json:"version"`
}

func (u *GolangUpdater) getLatestGoVersion(releaseTrack string) error {
	url := "https://go.dev/dl/?mode=json"
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to get releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var releases []GoDevRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	for _, release := range releases {
		if strings.HasPrefix(release.Version, "go"+releaseTrack) {
			u.targetVersion = release.Version
			slog.Info("Latest version", "component", "golang", "releaseTrack", releaseTrack, "version", u.targetVersion)
			return nil
		}
	}

	return fmt.Errorf("no matching release found for track: %s", releaseTrack)
}

func (u *GolangUpdater) getGolangImageHash() error {
	tag := strings.TrimPrefix(u.targetVersion, "go")
	url := fmt.Sprintf("https://registry.hub.docker.com/v2/repositories/library/golang/tags/%s", tag)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to get releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var tagResponse struct {
		Images []struct {
			Digest       string `json:"digest"`
			Architecture string `json:"architecture"`
		} `json:"images"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tagResponse); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	if len(tagResponse.Images) == 0 {
		return fmt.Errorf("no image found for tag: %s", tag)
	}

	for _, image := range tagResponse.Images {
		if image.Architecture == "amd64" {
			u.imageHash = image.Digest
			slog.Info("Golang image hash", "imageHash", u.imageHash)
			return nil
		}
	}

	return fmt.Errorf("no amd64 image found for tag: %s", tag)
}

func (u *GolangUpdater) updateGoVersion() error {
	pattern1 := regexp.MustCompile(`(BUILD_BASE_IMAGE\s*\?=\s*golang:)[0-9]+\.[0-9]+\.[0-9]+(@sha256:[a-f0-9]{64})`)
	pattern2 := regexp.MustCompile(`(GO_VERSION:\s*)[0-9]+\.[0-9]+\.[0-9]+`)

	for _, filePath := range filesToPatchWithGolangVersion {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		newContent := pattern1.ReplaceAllString(string(content), fmt.Sprintf("${1}%s$2", strings.TrimPrefix(u.targetVersion, "go")))
		newContent = pattern2.ReplaceAllString(newContent, fmt.Sprintf("${1}%s", strings.TrimPrefix(u.targetVersion, "go")))

		err = os.WriteFile(filePath, []byte(newContent), 0644)
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}
	}

	slog.Info("Running go mod tidy to update generated files")
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run go mod tidy: %w", err)
	}

	return nil
}

func (u *GolangUpdater) GetCommitMessage() (string, error) {
	if u.targetVersion == "" {
		return "", fmt.Errorf("target version not set")
	}
	message := fmt.Sprintf("Automated bump for Go %s.\n\nSee the release notes: https://go.dev/doc/devel/release#%s", u.targetVersion, u.targetVersion)
	return message, nil
}

func (u *GolangUpdater) GetRelease() (string, error) {
	if u.targetVersion == "" {
		return "", fmt.Errorf("target version not set")
	}
	return u.targetVersion, nil
}

func (u *GolangUpdater) Process(releaseTrack string) error {
	err := u.getLatestGoVersion(releaseTrack)
	if err != nil {
		return fmt.Errorf("error getting latest version: %w", err)
	}

	err = u.getGolangImageHash()
	if err != nil {
		return fmt.Errorf("error getting image hash: %w", err)
	}

	err = u.updateGoVersion()
	if err != nil {
		return fmt.Errorf("error updating code: %w", err)
	}
	return nil
}
