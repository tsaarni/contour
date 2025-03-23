package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"log/slog"
)

// List files to be patched.
var filesToUpdate = []string{
	"Makefile",
	"cmd/contour/gatewayprovisioner.go",
	"examples/contour/03-envoy.yaml",
	"examples/deployment/03-envoy-deployment.yaml",
}

type Release struct {
	TagName string `json:"tag_name"`
}

func main() {
	releaseTrack := flag.String("release-track", "", "Envoy release track to follow (e.g., v1.31)")
	outputVersionFile := flag.String("output-version-file", "", "File to write the version to")
	commitMessageFile := flag.String("commit-message-file", "", "File to write the commit message to")
	flag.Parse()

	if *releaseTrack == "" {
		flag.Usage()
		return
	}

	latestImage, err := getLatestEnvoyTag(*releaseTrack)
	if err != nil {
		slog.Error("Error getting latest Envoy image", "error", err)
		return
	}
	slog.Info("Latest Envoy image", "releaseTrack", *releaseTrack, "image", latestImage)

	err = updateEnvoyImage(latestImage)
	if err != nil {
		slog.Error("Error updating code", "error", err)
		return
	}

	if *outputVersionFile != "" {
		err = os.WriteFile(*outputVersionFile, []byte(latestImage), 0644)
		if err != nil {
			slog.Error("Error writing to output version file", "error", err)
			return
		}
		slog.Info("Wrote latest version to file", "file", *outputVersionFile)
	}

	if *commitMessageFile != "" {
		err = writeCommitMessageFile(*commitMessageFile, latestImage)
		if err != nil {
			slog.Error("Error writing commit message file", "error", err)
			return
		}
		slog.Info("Wrote commit message to file", "file", *commitMessageFile)
	}
}

func getLatestEnvoyTag(releaseTrack string) (string, error) {
	// http https://api.github.com/repos/envoyproxy/envoy/releases | jq -r ".[].tag_name"
	url := "https://api.github.com/repos/envoyproxy/envoy/releases"
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	for _, release := range releases {
		if strings.HasPrefix(release.TagName, releaseTrack+".") {
			return release.TagName, nil
		}
	}

	return "", fmt.Errorf("no matching release found for track: %s", releaseTrack)
}

func updateEnvoyImage(image string) error {
	pattern := regexp.MustCompile(`docker\.io/envoyproxy/envoy:[^"]+`)

	for _, filePath := range filesToUpdate {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		newContent := pattern.ReplaceAllString(string(content), fmt.Sprintf("docker.io/envoyproxy/envoy:%s", image))

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

func writeCommitMessageFile(filePath, image string) error {
	message := fmt.Sprintf("Automated bump of Envoy to %s.\n\nSee the release notes: https://github.com/envoyproxy/envoy/releases/tag/%s", image, image)
	return os.WriteFile(filePath, []byte(message), 0644)
}
