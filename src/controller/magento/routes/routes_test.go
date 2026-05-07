package routes

import (
	"strings"
	"testing"
)

func TestBuildGeneratedRoutesPrefersStoreForDuplicatePath(t *testing.T) {
	projectConf := map[string]string{
		"nginx/hosts/base/name": "example.test",
	}

	detected := []detectedRoute{
		{ScopeType: "websites", ScopeCode: "sitecl", Host: "example.test", PathPrefix: "/cl/es", MageRunCode: "sitecl", MageRunType: "website", StripPathPrefix: true},
		{ScopeType: "stores", ScopeCode: "storecles", Host: "example.test", PathPrefix: "/cl/es", MageRunCode: "storecles", MageRunType: "store", StripPathPrefix: true},
		{ScopeType: "websites", ScopeCode: "sitecl", Host: "example.test", PathPrefix: "/cl", MageRunCode: "sitecl", MageRunType: "website", StripPathPrefix: true},
		{ScopeType: "stores", ScopeCode: "storefrfr", Host: "missing.test", PathPrefix: "/fr/fr", MageRunCode: "storefrfr", MageRunType: "store", StripPathPrefix: true},
	}

	routes, warnings, err := buildGeneratedRoutes(detected, projectConf, "")
	if err != nil {
		t.Fatalf("buildGeneratedRoutes() error = %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("buildGeneratedRoutes() returned %d routes, want 2", len(routes))
	}
	if routes[0].MageRunCode != "storecles" {
		t.Fatalf("first route MageRunCode = %q, want %q", routes[0].MageRunCode, "storecles")
	}
	if routes[0].PathPrefix != "/cl/es" {
		t.Fatalf("first route PathPrefix = %q, want %q", routes[0].PathPrefix, "/cl/es")
	}
	if routes[1].MageRunCode != "sitecl" {
		t.Fatalf("second route MageRunCode = %q, want %q", routes[1].MageRunCode, "sitecl")
	}

	joinedWarnings := strings.Join(warnings, "\n")
	if !strings.Contains(joinedWarnings, "Skipped duplicate route website:sitecl") {
		t.Fatalf("expected duplicate warning, got %q", joinedWarnings)
	}
	if !strings.Contains(joinedWarnings, "missing.test") {
		t.Fatalf("expected missing host warning, got %q", joinedWarnings)
	}
}

func TestBuildGeneratedRoutesWithHostCodeOverride(t *testing.T) {
	projectConf := map[string]string{
		"nginx/hosts/base/name": "local.test",
	}

	detected := []detectedRoute{
		{ScopeType: "stores", ScopeCode: "storedeen", Host: "www.example.com", PathPrefix: "/de/en", MageRunCode: "storedeen", MageRunType: "store", StripPathPrefix: true},
	}

	routes, warnings, err := buildGeneratedRoutes(detected, projectConf, "base")
	if err != nil {
		t.Fatalf("buildGeneratedRoutes() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(routes) != 1 {
		t.Fatalf("buildGeneratedRoutes() returned %d routes, want 1", len(routes))
	}
	if routes[0].HostRef != "base" {
		t.Fatalf("HostRef = %q, want %q", routes[0].HostRef, "base")
	}
	if routes[0].HostName != "local.test" {
		t.Fatalf("HostName = %q, want %q", routes[0].HostName, "local.test")
	}
}
