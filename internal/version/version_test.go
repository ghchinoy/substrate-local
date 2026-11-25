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

package version

import (
	"strings"
	"testing"
)

func TestStringAndFull(t *testing.T) {
	origVersion, origCommit, origDate := Version, Commit, Date
	t.Cleanup(func() {
		Version, Commit, Date = origVersion, origCommit, origDate
	})

	Version = "dev"
	Commit = "b1f2ca1deadbeef1234"
	Date = ""
	if got := String(); got != "dev+b1f2ca1deadb" {
		t.Errorf("dev String() = %q, want %q", got, "dev+b1f2ca1deadb")
	}
	if IsRelease() {
		t.Error("expected IsRelease()=false for dev build")
	}

	Version = "v0.1.0"
	Commit = "b1f2ca1deadbeef1234"
	Date = "2026-11-25T22:00:00Z"
	if !IsRelease() {
		t.Error("expected IsRelease()=true for v0.1.0")
	}
	if got := String(); got != "v0.1.0" {
		t.Errorf("release String() = %q, want %q", got, "v0.1.0")
	}
	full := Full()
	for _, want := range []string{"substrate-local v0.1.0", "commit=b1f2ca1deadb", "built=2026-11-25T22:00:00Z"} {
		if !strings.Contains(full, want) {
			t.Errorf("Full() = %q, missing %q", full, want)
		}
	}
}
