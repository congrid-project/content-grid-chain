package main

import (
	"runtime/debug"
	"strings"

	sdkversion "github.com/cosmos/cosmos-sdk/version"

	"content-grid-chain/app"
)

const (
	defaultVersion = "publisher-rewards-v3"
	unknownCommit  = "unknown"
)

func init() {
	configureVersion(debug.ReadBuildInfo())
}

func configureVersion(buildInfo *debug.BuildInfo, buildInfoOK bool) {
	if sdkversion.Name == "" {
		sdkversion.Name = app.AppName
	}
	if sdkversion.AppName == "" || sdkversion.AppName == "<appd>" {
		sdkversion.AppName = "content-grid-d"
	}

	revision, modified := vcsBuildSettings(buildInfo, buildInfoOK)
	if sdkversion.Commit == "" {
		sdkversion.Commit = revision
		if sdkversion.Commit == "" {
			sdkversion.Commit = unknownCommit
		}
	}
	if sdkversion.Version == "" {
		sdkversion.Version = versionFromBuildInfo(buildInfo, buildInfoOK)
		if sdkversion.Version == "" {
			sdkversion.Version = defaultVersion
			if revision != "" {
				sdkversion.Version += "-" + shortCommit(revision)
			}
			if modified {
				sdkversion.Version += "+dirty"
			}
		}
	}
}

func vcsBuildSettings(buildInfo *debug.BuildInfo, ok bool) (revision string, modified bool) {
	if !ok || buildInfo == nil {
		return "", false
	}
	for _, setting := range buildInfo.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = strings.EqualFold(strings.TrimSpace(setting.Value), "true")
		}
	}
	return revision, modified
}

func versionFromBuildInfo(buildInfo *debug.BuildInfo, ok bool) string {
	if !ok || buildInfo == nil {
		return ""
	}
	version := strings.TrimSpace(buildInfo.Main.Version)
	if version == "" || version == "(devel)" {
		return ""
	}
	return version
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
