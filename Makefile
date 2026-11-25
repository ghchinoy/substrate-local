# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

export GOTOOLCHAIN ?= auto

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG := github.com/ghchinoy/substrate-local/internal/version
LDFLAGS := -s -w -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).Date=$(BUILD_DATE)

.PHONY: build run test test-compat vet fmt up down multi-up multi-down status logs reset clean

build:
	mkdir -p bin
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/substrate-local ./cmd/substrate-local

run: build
	./bin/substrate-local --config examples/substrate-local.yaml

test:
	go test -race -v ./...

test-compat:
	./scripts/test-ax-compat.sh

vet:
	go vet ./...

fmt:
	go fmt ./...

up:
	./scripts/ax-stack.sh up

down:
	./scripts/ax-stack.sh down

multi-up:
	./scripts/ax-stack.sh multi-up

multi-down:
	./scripts/ax-stack.sh multi-down

status:
	./scripts/ax-stack.sh status

logs:
	./scripts/ax-stack.sh logs

reset:
	./scripts/ax-stack.sh reset

clean:
	rm -rf bin .tmp
