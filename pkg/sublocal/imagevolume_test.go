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

package sublocal

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ateapipb "go.ate.dev/substrate/api/ateapi/v1"
)

type fakeImageVolumeResolver struct {
	mu    sync.Mutex
	calls []string
	dirs  map[string]string
}

func (f *fakeImageVolumeResolver) Resolve(_ context.Context, reference string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, reference)
	if d, ok := f.dirs[reference]; ok {
		return d, nil
	}
	return "", fmt.Errorf("unexpected image volume ref %q", reference)
}

func TestImageVolumeMount_CreateTemplateAndResume(t *testing.T) {
	ctx := t.Context()
	volDir := t.TempDir()
	const runnerVolRef = "ghcr.io/google/ax/ax-runner-volume@sha256:a85dc4ac3e4b060d9157e3f80c01463544e79a0e92106bacad1e4f3f3cf5e4d5"

	resolver := &fakeImageVolumeResolver{
		dirs: map[string]string{runnerVolRef: volDir},
	}
	runner := &mockRunner{
		created: make(map[string]bool),
		inspectMap: map[string]*ContainerInspectResult{
			"ate-default-prebuilt-agent": {
				ID: "ate-default-prebuilt-agent",
				Status: struct {
					State    string `json:"state"`
					Networks []struct {
						Hostname    string `json:"hostname"`
						IPv4Address string `json:"ipv4Address"`
						IPv4Gateway string `json:"ipv4Gateway"`
					} `json:"networks"`
				}{
					State: "running",
					Networks: []struct {
						Hostname    string `json:"hostname"`
						IPv4Address string `json:"ipv4Address"`
						IPv4Gateway string `json:"ipv4Gateway"`
					}{{Hostname: "ate-default-prebuilt-agent", IPv4Address: "192.168.64.55/24"}},
				},
			},
		},
	}

	cfg := DefaultConfig()
	cfg.ReadyTimeout = 100 * time.Millisecond
	cfg.Runner = runner
	cfg.ImageVolumeResolver = resolver
	cfg.Templates["default"] = TemplateConfig{
		Atespace: "default",
		Name:     "default",
		Image:    "debian:12",
		Port:     80,
		Volumes:  []string{"/host/ax-runner:/usr/local/bin/ax-runner", "/host/extra:/extra"},
	}

	client, cleanup := startTestControlServer(t, cfg)
	defer cleanup()

	// Create an ActorTemplate matching ax-server --runner-volume-image.
	_, err := client.CreateActorTemplate(ctx, &ateapipb.CreateActorTemplateRequest{
		ActorTemplate: &ateapipb.ActorTemplate{
			Metadata: &ateapipb.ResourceMetadata{Atespace: "default", Name: "ax-prebuilt"},
			Volumes: []*ateapipb.Volume{{
				Name:  "ax-runner",
				Image: &ateapipb.ImageVolumeSource{Reference: runnerVolRef},
			}},
			Containers: []*ateapipb.Container{{
				Name:    "guest",
				Image:   "ghcr.io/agentexecutor/agy-cli@sha256:0d459fee234a47ab9cc56996b65711097e4ac1b18641ec252fdd960aacb74ef6",
				Command: []string{"/opt/ax/ax-runner"},
				VolumeMounts: []*ateapipb.VolumeMount{{
					Name:      "ax-runner",
					MountPath: "/opt/ax",
				}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("CreateActorTemplate failed: %v", err)
	}

	// Verify GetActorTemplate round-trips Volumes and VolumeMounts.
	gotTmpl, err := client.GetActorTemplate(ctx, &ateapipb.GetActorTemplateRequest{
		ActorTemplate: &ateapipb.ObjectRef{Atespace: "default", Name: "ax-prebuilt"},
	})
	if err != nil {
		t.Fatalf("GetActorTemplate failed: %v", err)
	}
	if len(gotTmpl.GetVolumes()) != 1 || gotTmpl.GetVolumes()[0].GetImage().GetReference() != runnerVolRef {
		t.Fatalf("GetActorTemplate Volumes = %v, want single image volume %q", gotTmpl.GetVolumes(), runnerVolRef)
	}
	if len(gotTmpl.GetContainers()) != 1 || len(gotTmpl.GetContainers()[0].GetVolumeMounts()) != 1 ||
		gotTmpl.GetContainers()[0].GetVolumeMounts()[0].GetMountPath() != "/opt/ax" {
		t.Fatalf("GetActorTemplate VolumeMounts = %v, want mount /opt/ax", gotTmpl.GetContainers()[0].GetVolumeMounts())
	}

	// Create & resume an actor using that template.
	if _, err := client.CreateActor(ctx, &ateapipb.CreateActorRequest{
		Actor: &ateapipb.Actor{
			Metadata:      &ateapipb.ResourceMetadata{Atespace: "default", Name: "prebuilt-agent"},
			ActorTemplate: &ateapipb.ObjectRef{Atespace: "default", Name: "ax-prebuilt"},
		},
	}); err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	if _, err := client.ResumeActor(ctx, &ateapipb.ResumeActorRequest{
		Actor: &ateapipb.ObjectRef{Atespace: "default", Name: "prebuilt-agent"},
	}); err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	// Verify runArgs: mounts <volDir>:/opt/ax:ro, sets --entrypoint /opt/ax/ax-runner,
	// keeps non-runner default volumes (/extra), and strips default /usr/local/bin/ax-runner.
	wantMount := fmt.Sprintf("-v %s:/opt/ax:ro", volDir)
	foundRun := false
	for _, call := range runner.runCalls {
		if len(call) > 0 && call[0] == "run" {
			foundRun = true
			joined := strings.Join(call, " ")
			if !strings.Contains(joined, wantMount) {
				t.Errorf("run call missing read-only image volume mount %q:\n  %s", wantMount, joined)
			}
			if !strings.Contains(joined, "--entrypoint /opt/ax/ax-runner") {
				t.Errorf("run call missing '--entrypoint /opt/ax/ax-runner':\n  %s", joined)
			}
			if strings.Contains(joined, ":/usr/local/bin/ax-runner") {
				t.Errorf("run call should not bind-mount default /usr/local/bin/ax-runner when ImageVolumes is set:\n  %s", joined)
			}
			if !strings.Contains(joined, "-v /host/extra:/extra") {
				t.Errorf("run call should preserve non-runner default volumes:\n  %s", joined)
			}
		}
	}
	if !foundRun {
		t.Fatalf("expected a container run call, got %v", runner.runCalls)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != runnerVolRef {
		t.Errorf("resolver calls = %v, want [%q]", resolver.calls, runnerVolRef)
	}
}

func TestExtractTarSafe(t *testing.T) {
	t.Run("extracts_executable_and_symlink", func(t *testing.T) {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		data := []byte("#!/bin/sh\necho ok\n")
		if err := tw.WriteHeader(&tar.Header{
			Name:     "ax-runner",
			Mode:     0o755,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := tw.WriteHeader(&tar.Header{
			Name:     "bin/runner-link",
			Typeflag: tar.TypeSymlink,
			Linkname: "../ax-runner",
		}); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}

		dst := t.TempDir()
		if err := extractTarSafe(&buf, dst); err != nil {
			t.Fatalf("extractTarSafe failed: %v", err)
		}
		st, err := os.Stat(filepath.Join(dst, "ax-runner"))
		if err != nil {
			t.Fatalf("missing extracted ax-runner: %v", err)
		}
		if st.Mode().Perm()&0o111 == 0 {
			t.Errorf("ax-runner mode = %o, want executable", st.Mode().Perm())
		}
		linkTarget, err := os.Readlink(filepath.Join(dst, "bin", "runner-link"))
		if err != nil || linkTarget != "../ax-runner" {
			t.Errorf("symlink = (%q, %v), want ../ax-runner", linkTarget, err)
		}
	})

	t.Run("rejects_path_traversal", func(t *testing.T) {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		_ = tw.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte("no"))
		_ = tw.Close()

		if err := extractTarSafe(&buf, t.TempDir()); err == nil {
			t.Fatal("expected error on path traversal, got nil")
		}
	})
}

func TestOCIImageVolumeResolver_LiveRunnerVolume(t *testing.T) {
	if _, err := exec.LookPath("container"); err != nil {
		t.Skip("Apple container CLI not available, skipping live runner-volume test")
	}
	ctx := t.Context()
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(checkCtx, "container", "list").Run(); err != nil {
		t.Skipf("Apple container daemon not responsive: %v, skipping live test", err)
	}

	const runnerVolRef = "ghcr.io/google/ax/ax-runner-volume@sha256:a85dc4ac3e4b060d9157e3f80c01463544e79a0e92106bacad1e4f3f3cf5e4d5"
	cacheDir := t.TempDir()
	resolver := NewOCIImageVolumeResolver(cacheDir, "")

	pullCtx, pullCancel := context.WithTimeout(ctx, 60*time.Second)
	defer pullCancel()

	dir, err := resolver.Resolve(pullCtx, runnerVolRef)
	if err != nil {
		t.Fatalf("Resolve(%q) failed: %v", runnerVolRef, err)
	}
	runnerPath := filepath.Join(dir, "ax-runner")
	st, err := os.Stat(runnerPath)
	if err != nil {
		t.Fatalf("expected extracted ax-runner at %s: %v", runnerPath, err)
	}
	if st.Size() == 0 || st.Mode().Perm()&0o111 == 0 {
		t.Fatalf("unexpected ax-runner file stats: size=%d mode=%o", st.Size(), st.Mode().Perm())
	}

	// Second Resolve must hit the local cache instantaneously.
	start := time.Now()
	dir2, err := resolver.Resolve(pullCtx, runnerVolRef)
	if err != nil || dir2 != dir {
		t.Fatalf("cached Resolve = (%q, %v), want %q", dir2, err, dir)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("cached Resolve took %v, expected instant cache hit", elapsed)
	}

	// Boot a plain python:3.12-slim microVM mounting the extracted OCI runner volume
	// at /opt/ax:ro with --entrypoint /opt/ax/ax-runner and verify /readyz returns 200 OK.
	cfg := DefaultConfig()
	cfg.SnapshotDir = t.TempDir()
	cfg.ImageVolumeCacheDir = cacheDir
	cfg.ReadyTimeout = 10 * time.Second

	client, serverAddr, cleanup := startTestControlServerWithAddr(t, cfg)
	defer cleanup()

	actorName := fmt.Sprintf("vol-live-%d", time.Now().UnixNano()%100000)
	defer func() {
		_, _ = client.DeleteActor(context.Background(), &ateapipb.DeleteActorRequest{
			Actor: &ateapipb.ObjectRef{Atespace: "default", Name: actorName},
		})
	}()

	if _, err := client.CreateActorTemplate(pullCtx, &ateapipb.CreateActorTemplateRequest{
		ActorTemplate: &ateapipb.ActorTemplate{
			Metadata: &ateapipb.ResourceMetadata{Atespace: "default", Name: "vol-tmpl"},
			Volumes: []*ateapipb.Volume{{
				Name:  "ax-runner",
				Image: &ateapipb.ImageVolumeSource{Reference: runnerVolRef},
			}},
			Containers: []*ateapipb.Container{{
				Name:    "guest",
				Image:   "python:3.12-slim@sha256:5e2bdb1b0d542d5039cffb8181c65930093949194a65c365dfb1b0fe13a4fa5b",
				Command: []string{"/opt/ax/ax-runner"},
				VolumeMounts: []*ateapipb.VolumeMount{{
					Name:      "ax-runner",
					MountPath: "/opt/ax",
				}},
			}},
		},
	}); err != nil {
		t.Fatalf("CreateActorTemplate failed: %v", err)
	}
	if _, err := client.CreateActor(pullCtx, &ateapipb.CreateActorRequest{
		Actor: &ateapipb.Actor{
			Metadata:      &ateapipb.ResourceMetadata{Atespace: "default", Name: actorName},
			ActorTemplate: &ateapipb.ObjectRef{Atespace: "default", Name: "vol-tmpl"},
		},
	}); err != nil {
		t.Fatalf("CreateActor failed: %v", err)
	}
	if _, err := client.ResumeActor(pullCtx, &ateapipb.ResumeActorRequest{
		Actor: &ateapipb.ObjectRef{Atespace: "default", Name: actorName},
	}); err != nil {
		t.Fatalf("ResumeActor failed: %v", err)
	}

	req, err := http.NewRequestWithContext(pullCtx, http.MethodGet, "http://"+serverAddr+"/readyz", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("ate-target-actor", "default/"+actorName)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /readyz via router failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /readyz status = %d, want 200", resp.StatusCode)
	}
	t.Logf("Live plain python:3.12-slim booted /opt/ax/ax-runner from %s and answered /readyz 200 OK", runnerVolRef)
}
