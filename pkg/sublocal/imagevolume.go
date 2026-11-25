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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ImageVolumeMount describes a read-only OCI image volume declared on an
// ActorTemplate (e.g. ghcr.io/google/ax/ax-runner-volume mounted at /opt/ax).
type ImageVolumeMount struct {
	Name      string `yaml:"name"`
	Reference string `yaml:"reference"`
	MountPath string `yaml:"mount_path"`
}

// ImageVolumeResolver pulls and extracts a read-only OCI image volume into a
// local host directory suitable for bind-mounting into actor containers.
type ImageVolumeResolver interface {
	Resolve(ctx context.Context, reference string) (string, error)
}

// OCIImageVolumeResolver pulls OCI image volumes (including FROM scratch images
// such as ghcr.io/google/ax/ax-runner-volume) via go-containerregistry and
// caches their extracted filesystems under CacheDir by (reference, targetArch).
type OCIImageVolumeResolver struct {
	CacheDir   string
	TargetArch string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewOCIImageVolumeResolver creates a resolver caching extracted OCI image
// volumes under cacheDir for linux/<targetArch> (defaulting to runtime.GOARCH).
func NewOCIImageVolumeResolver(cacheDir, targetArch string) *OCIImageVolumeResolver {
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}
	return &OCIImageVolumeResolver{
		CacheDir:   cacheDir,
		TargetArch: targetArch,
		locks:      make(map[string]*sync.Mutex),
	}
}

func (r *OCIImageVolumeResolver) lockFor(key string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.locks == nil {
		r.locks = make(map[string]*sync.Mutex)
	}
	l, ok := r.locks[key]
	if !ok {
		l = &sync.Mutex{}
		r.locks[key] = l
	}
	return l
}

// Resolve returns a cached host directory containing the flattened filesystem
// of reference for linux/<TargetArch>.
func (r *OCIImageVolumeResolver) Resolve(ctx context.Context, reference string) (string, error) {
	refStr := strings.TrimSpace(reference)
	if refStr == "" {
		return "", errors.New("image volume reference is empty")
	}
	arch := r.TargetArch
	if arch == "" {
		arch = runtime.GOARCH
	}
	cacheRoot := r.CacheDir
	if cacheRoot == "" {
		cacheRoot = filepath.Join(os.TempDir(), "substrate-local-image-volumes")
	}
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return "", fmt.Errorf("creating image volume cache dir: %w", err)
	}

	sum := sha256.Sum256([]byte(refStr + "\x00linux/" + arch))
	key := hex.EncodeToString(sum[:12])
	dstDir := filepath.Join(cacheRoot, key)
	readyMarker := dstDir + ".ready"

	l := r.lockFor(key)
	l.Lock()
	defer l.Unlock()

	if _, err := os.Stat(readyMarker); err == nil {
		if st, err := os.Stat(dstDir); err == nil && st.IsDir() {
			return dstDir, nil
		}
	}

	parsedRef, err := name.ParseReference(refStr)
	if err != nil {
		return "", fmt.Errorf("parsing image volume reference %q: %w", refStr, err)
	}

	slog.Info("pulling OCI image volume", "reference", refStr, "platform", "linux/"+arch, "cacheDir", dstDir)
	img, err := remote.Image(parsedRef,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithPlatform(v1.Platform{OS: "linux", Architecture: arch}),
	)
	if err != nil {
		return "", fmt.Errorf("pulling image volume %q (linux/%s): %w", refStr, arch, err)
	}

	tmpDir, err := os.MkdirTemp(cacheRoot, key+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("creating staging dir for image volume %q: %w", refStr, err)
	}
	if err := os.Chmod(tmpDir, 0o755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("setting permissions on image volume staging dir: %w", err)
	}
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	rc := mutate.Extract(img)
	extractErr := extractTarSafe(rc, tmpDir)
	closeErr := rc.Close()
	if extractErr != nil {
		return "", fmt.Errorf("extracting image volume %q: %w", refStr, extractErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("closing image volume stream %q: %w", refStr, closeErr)
	}

	_ = os.RemoveAll(dstDir)
	if err := os.Rename(tmpDir, dstDir); err != nil {
		return "", fmt.Errorf("committing image volume cache dir %q: %w", dstDir, err)
	}
	cleanupTmp = false
	if err := os.WriteFile(readyMarker, []byte(refStr+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("writing image volume ready marker: %w", err)
	}
	return dstDir, nil
}

// extractTarSafe extracts a flattened OCI tar stream into dstDir while
// rejecting path traversal and preserving executable file modes.
func extractTarSafe(r io.Reader, dstDir string) error {
	cleanRoot := filepath.Clean(dstDir)
	tr := tar.NewReader(r)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		name := strings.TrimPrefix(hdr.Name, "/")
		name = filepath.Clean(name)
		if name == "." || name == "" {
			continue
		}
		if name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("illegal path traversal in image volume tar: %q", hdr.Name)
		}

		target := filepath.Join(cleanRoot, name)
		if target != cleanRoot && !strings.HasPrefix(target, cleanRoot+string(filepath.Separator)) {
			return fmt.Errorf("target path escapes root: %q", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			mode := os.FileMode(hdr.Mode) & 0o777
			if mode == 0 {
				mode = 0o755
			}
			if err := os.MkdirAll(target, mode); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			linkDest := hdr.Linkname
			if filepath.IsAbs(linkDest) {
				linkDest = filepath.Join(cleanRoot, strings.TrimPrefix(linkDest, "/"))
			} else {
				linkDest = filepath.Join(filepath.Dir(target), linkDest)
			}
			linkDest = filepath.Clean(linkDest)
			if linkDest != cleanRoot && !strings.HasPrefix(linkDest, cleanRoot+string(filepath.Separator)) {
				return fmt.Errorf("illegal symlink target escaping root: %q -> %q", hdr.Name, hdr.Linkname)
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			linkName := filepath.Clean(strings.TrimPrefix(hdr.Linkname, "/"))
			linkTarget := filepath.Join(cleanRoot, linkName)
			if linkTarget != cleanRoot && !strings.HasPrefix(linkTarget, cleanRoot+string(filepath.Separator)) {
				return fmt.Errorf("illegal hardlink target escaping root: %q -> %q", hdr.Name, hdr.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Link(linkTarget, target); err != nil {
				return err
			}
		}
	}
}
