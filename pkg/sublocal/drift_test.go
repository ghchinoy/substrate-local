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
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"

	ateapipb "go.ate.dev/substrate/api/ateapi/v1"
)

// intentionallyUnimplementedRPCs enumerates ateapi/v1.ControlServer RPCs that
// substrate-local deliberately leaves unimplemented (cluster-only WorkerPool
// management, multi-tenant RBAC AccessPolicies, egress JWT minting, and
// mutable Actor/Tag updates).
//
// TestControlServerCoverage fails whenever a go.ate.dev/substrate upgrade adds
// a new RPC method that is neither implemented on *Server nor triaged here.
var intentionallyUnimplementedRPCs = map[string]string{
	"UpdateActor":                "AX tasks/actors are immutable in v0.4",
	"UpdateTag":                  "local file tags do not publish to external registries",
	"MintActorJwt":               "egress gateway JWT injection requires full atenet-egress",
	"GetWorker":                  "cluster-only WorkerPool lifecycle",
	"CreateWorker":               "cluster-only WorkerPool lifecycle",
	"UpdateWorker":               "cluster-only WorkerPool lifecycle",
	"DeleteWorker":               "cluster-only WorkerPool lifecycle",
	"DrainWorker":                "cluster-only WorkerPool lifecycle",
	"MarkWorkerAvailable":        "cluster-only WorkerPool lifecycle",
	"MarkWorkerUnavailable":      "cluster-only WorkerPool lifecycle",
	"ListWorkerActorAssignments": "cluster-only WorkerPool lifecycle",
	"GetGlobalAccessPolicy":      "cluster-only RBAC AccessPolicy",
	"CreateGlobalAccessPolicy":   "cluster-only RBAC AccessPolicy",
	"UpdateGlobalAccessPolicy":   "cluster-only RBAC AccessPolicy",
	"GetAtespaceAccessPolicy":    "cluster-only RBAC AccessPolicy",
	"CreateAtespaceAccessPolicy": "cluster-only RBAC AccessPolicy",
	"UpdateAtespaceAccessPolicy": "cluster-only RBAC AccessPolicy",
	"DeleteAtespaceAccessPolicy": "cluster-only RBAC AccessPolicy",
}

// serverDeclaredMethods parses non-test Go files in this package and returns
// every method name declared directly on *Server (excluding promoted methods
// from embedded UnimplementedControlServer).
func serverDeclaredMethods(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing package files: %v", err)
	}
	methods := make(map[string]bool)
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
					continue
				}
				if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
					if id, ok := star.X.(*ast.Ident); ok && id.Name == "Server" {
						methods[fn.Name.Name] = true
					}
				}
			}
		}
	}
	return methods
}

func TestControlServerCoverage(t *testing.T) {
	ifaceType := reflect.TypeOf((*ateapipb.ControlServer)(nil)).Elem()
	declared := serverDeclaredMethods(t)

	var implemented, unimplemented, untriaged, staleAllowlist []string

	seen := make(map[string]bool)
	for i := 0; i < ifaceType.NumMethod(); i++ {
		name := ifaceType.Method(i).Name
		if name == "mustEmbedUnimplementedControlServer" {
			continue
		}
		seen[name] = true

		if declared[name] {
			implemented = append(implemented, name)
			if _, listed := intentionallyUnimplementedRPCs[name]; listed {
				staleAllowlist = append(staleAllowlist, name)
			}
			continue
		}

		if _, listed := intentionallyUnimplementedRPCs[name]; listed {
			unimplemented = append(unimplemented, name)
		} else {
			untriaged = append(untriaged, name)
		}
	}

	for name := range intentionallyUnimplementedRPCs {
		if !seen[name] {
			staleAllowlist = append(staleAllowlist, name+" (removed from ControlServer)")
		}
	}

	slices.Sort(implemented)
	slices.Sort(unimplemented)
	slices.Sort(untriaged)
	slices.Sort(staleAllowlist)

	t.Logf("ateapi/v1.ControlServer coverage: %d/%d RPCs implemented (%d intentionally unimplemented)",
		len(implemented), len(implemented)+len(unimplemented)+len(untriaged), len(unimplemented))

	if len(untriaged) > 0 {
		t.Errorf("new upstream ControlServer RPCs detected (implement on *Server or triage in intentionallyUnimplementedRPCs): %v", untriaged)
	}
	if len(staleAllowlist) > 0 {
		t.Errorf("stale entries in intentionallyUnimplementedRPCs: %v", staleAllowlist)
	}
}
