package main

import (
	"flag"
	"log/slog"
	"os"
)

type Updater interface {
	Process(releaseTrack string) error
	GetRelease() (string, error)
	GetCommitMessage() (string, error)
}

func main() {
	var (
		releaseTrack      = flag.String("release-track", "", "Release track to follow (e.g., 1 for Go, v1.31 for Envoy)")
		outputVersionFile = flag.String("output-version-file", "", "File to write the version to")
		commitMessageFile = flag.String("commit-message-file", "", "File to write the commit message to")
	)
	flag.Parse()

	if len(flag.Args()) < 1 {
		slog.Error("Component name is required")
		flag.Usage()
		os.Exit(1)
	}
	component := flag.Args()[0]

	if *releaseTrack == "" {
		flag.Usage()
		os.Exit(1)
	}

	var updater Updater

	switch component {
	case "golang":
		updater = &GolangUpdater{}
	case "envoy":
		updater = &EnvoyUpdater{}
	default:
		slog.Error("Unsupported component", "component", component)
		os.Exit(1)
	}

	err := updater.Process(*releaseTrack)
	if err != nil {
		slog.Error("Error processing update", "error", err)
		os.Exit(1)
	}

	latestVersion, err := updater.GetRelease()
	if err != nil {
		slog.Error("Error getting latest version", "error", err)
		os.Exit(1)
	}
	slog.Info("Latest version", "component", component, "releaseTrack", *releaseTrack, "version", latestVersion)

	if *outputVersionFile != "" {
		err = os.WriteFile(*outputVersionFile, []byte(latestVersion), 0644)
		if err != nil {
			slog.Error("Error writing to output version file", "error", err)
			os.Exit(1)
		}
		slog.Info("Wrote latest version to file", "file", *outputVersionFile)
	}

	commitMessage, err := updater.GetCommitMessage()
	if err != nil {
		slog.Error("Error getting commit message", "error", err)
		os.Exit(1)
	}

	if *commitMessageFile != "" {
		err = os.WriteFile(*commitMessageFile, []byte(commitMessage), 0644)
		if err != nil {
			slog.Error("Error writing to commit message file", "error", err)
			os.Exit(1)
		}
		slog.Info("Wrote commit message to file", "file", *commitMessageFile)
	}
}
