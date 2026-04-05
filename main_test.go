package main

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	vers "go.admiral.io/cli/internal/version"
)

func TestVersion(t *testing.T) {
	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	for name, tt := range map[string]struct {
		version, commit, date, builtBy string
		out                            vers.Version
	}{
		"all empty": {
			out: vers.Version{
				GitVersion: "devel",
				GitCommit:  "unknown",
				BuildDate:  "unknown",
				BuiltBy:    "unknown",
				GoVersion:  runtime.Version(),
				Compiler:   runtime.Compiler,
				Platform:   platform,
				AsciiArt:   asciiArt,
			},
		},
		"only version": {
			version: "1.2.3",
			out: vers.Version{
				GitVersion: "1.2.3",
				GitCommit:  "unknown",
				BuildDate:  "unknown",
				BuiltBy:    "unknown",
				GoVersion:  runtime.Version(),
				Compiler:   runtime.Compiler,
				Platform:   platform,
				AsciiArt:   asciiArt,
			},
		},
		"version and date": {
			version: "1.2.3",
			date:    "12/12/12",
			out: vers.Version{
				GitVersion: "1.2.3",
				GitCommit:  "unknown",
				BuildDate:  "12/12/12",
				BuiltBy:    "unknown",
				GoVersion:  runtime.Version(),
				Compiler:   runtime.Compiler,
				Platform:   platform,
				AsciiArt:   asciiArt,
			},
		},
		"version, date, built by": {
			version: "1.2.3",
			date:    "12/12/12",
			builtBy: "me",
			out: vers.Version{
				GitVersion: "1.2.3",
				GitCommit:  "unknown",
				BuildDate:  "12/12/12",
				BuiltBy:    "me",
				GoVersion:  runtime.Version(),
				Compiler:   runtime.Compiler,
				Platform:   platform,
				AsciiArt:   asciiArt,
			},
		},
		"complete": {
			version: "1.2.3",
			date:    "12/12/12",
			commit:  "aaaa",
			builtBy: "me",
			out: vers.Version{
				GitVersion: "1.2.3",
				GitCommit:  "aaaa",
				BuildDate:  "12/12/12",
				BuiltBy:    "me",
				GoVersion:  runtime.Version(),
				Compiler:   runtime.Compiler,
				Platform:   platform,
				AsciiArt:   asciiArt,
			},
		},
	} {
		tt := tt
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.out, buildVersion(tt.version, tt.commit, tt.date, tt.builtBy))
		})
	}
}
