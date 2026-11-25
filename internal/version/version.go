// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package version holds build-time version metadata stamped via -ldflags.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Populated at build time via -ldflags "-X github.com/ghchinoy/substrate-local/internal/version.<Var>=...".
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// IsRelease reports whether this binary was built with a release version tag.
func IsRelease() bool {
	return Version != "" && Version != "dev"
}

// ShortCommit returns the abbreviated git commit SHA, falling back to Go build info.
func ShortCommit() string {
	c := strings.TrimSpace(Commit)
	if c == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && s.Value != "" {
					c = s.Value
					break
				}
			}
		}
	}
	if len(c) > 12 {
		c = c[:12]
	}
	return c
}

// String returns the concise version string (e.g. "v0.1.0" or "dev+b1f2ca1").
func String() string {
	if IsRelease() {
		return Version
	}
	if c := ShortCommit(); c != "" {
		return "dev+" + c
	}
	return "dev"
}

// Full returns a human-readable version line including platform and optional build date.
func Full() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("substrate-local %s", String()))
	if IsRelease() {
		if c := ShortCommit(); c != "" {
			parts = append(parts, "commit="+c)
		}
	}
	if d := strings.TrimSpace(Date); d != "" {
		parts = append(parts, "built="+d)
	}
	parts = append(parts, fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
	return strings.Join(parts, " ")
}
