package configs

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/faradey/madock/v3/src/helper/paths"
)

// NginxRoute describes an optional path-based routing rule layered on top of
// the default host-based Magento routing.
type NginxRoute struct {
	ID              string
	HostRef         string
	HostName        string
	PathPrefix      string
	MageRunCode     string
	MageRunType     string
	StripPathPrefix bool
	Enabled         bool
}

// HostRegex returns the host value escaped for use inside an nginx regex.
func (r NginxRoute) HostRegex() string {
	return regexp.QuoteMeta(r.HostName)
}

// PathRegex returns the path prefix escaped for use inside an nginx regex.
func (r NginxRoute) PathRegex() string {
	return regexp.QuoteMeta(r.PathPrefix)
}

// GetNginxRoutes returns fully resolved path-based route definitions.
//
// Route config lives under nginx/routes/<route_id>/... and is intentionally
// additive: incomplete route definitions are ignored until all required fields
// are present, so users can configure them incrementally via config:set.
func GetNginxRoutes(data map[string]string) []NginxRoute {
	hostsByCode := make(map[string]string)
	for _, host := range GetHosts(data) {
		hostsByCode[host["code"]] = host["name"]
	}

	routesByID := make(map[string]*NginxRoute)
	for _, key := range SortMap(data) {
		if !strings.Contains(key, "/routes/") {
			continue
		}

		parts := strings.Split(key, "/")
		routeIndex := -1
		for i, part := range parts {
			if part == "routes" {
				routeIndex = i
				break
			}
		}
		if routeIndex == -1 || len(parts) <= routeIndex+2 {
			continue
		}

		routeID := parts[routeIndex+1]
		field := strings.Join(parts[routeIndex+2:], "/")
		route := routesByID[routeID]
		if route == nil {
			route = &NginxRoute{
				ID:              routeID,
				Enabled:         true,
				StripPathPrefix: true,
			}
			routesByID[routeID] = route
		}

		value := strings.TrimSpace(data[key])
		switch field {
		case "enabled":
			if value != "" {
				route.Enabled = isTruthy(value)
			}
		case "host_ref":
			route.HostRef = value
		case "path_prefix":
			route.PathPrefix = normalizePathPrefix(value)
		case "mage_run_code":
			route.MageRunCode = value
		case "mage_run_type":
			route.MageRunType = value
		case "strip_path_prefix":
			if value != "" {
				route.StripPathPrefix = isTruthy(value)
			}
		}
	}

	var routes []NginxRoute
	for _, route := range routesByID {
		if !route.Enabled || route.HostRef == "" || route.PathPrefix == "" || route.PathPrefix == "/" {
			continue
		}

		hostName, ok := hostsByCode[route.HostRef]
		if !ok || hostName == "" {
			continue
		}

		route.HostName = hostName
		if route.MageRunCode != "" && route.MageRunType == "" {
			route.MageRunType = strings.TrimSpace(data["nginx/run_type"])
		}

		if route.MageRunCode == "" && route.MageRunType == "" && !route.StripPathPrefix {
			continue
		}

		routes = append(routes, *route)
	}

	sort.SliceStable(routes, func(i, j int) bool {
		if len(routes[i].PathPrefix) != len(routes[j].PathPrefix) {
			return len(routes[i].PathPrefix) > len(routes[j].PathPrefix)
		}
		if routes[i].HostName != routes[j].HostName {
			return routes[i].HostName < routes[j].HostName
		}
		return routes[i].ID < routes[j].ID
	})

	return routes
}

func normalizePathPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if len(prefix) > 1 {
		prefix = strings.TrimSuffix(prefix, "/")
	}
	return prefix
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// BuildGeneratedNginxRouteID creates a stable auto-managed route identifier.
func BuildGeneratedNginxRouteID(mageRunType, mageRunCode string) string {
	base := "auto_" + strings.ToLower(strings.TrimSpace(mageRunType)) + "_" + strings.ToLower(strings.TrimSpace(mageRunCode))
	base = regexp.MustCompile(`[^a-z0-9_]+`).ReplaceAllString(base, "_")
	base = regexp.MustCompile(`_+`).ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		return "auto_route"
	}
	if !strings.HasPrefix(base, "auto_") {
		return "auto_" + base
	}
	return base
}

// ReplaceGeneratedNginxRoutes replaces only auto-managed nginx/routes/auto_* rules
// in the selected scope. Manually maintained route IDs remain untouched.
func ReplaceGeneratedNginxRoutes(projectName, activeScope string, routes []NginxRoute) error {
	if activeScope == "" {
		activeScope = "default"
	}

	configPath := GetCurrentProjectConfigPath(projectName)
	if !paths.IsFileExist(configPath) {
		return fmt.Errorf("project config file not found: %s", configPath)
	}

	config := ParseXmlFile(configPath)
	scopePrefix := "scopes/" + activeScope + "/nginx/routes/"
	for key := range config {
		if strings.HasPrefix(key, scopePrefix+"auto_") {
			delete(config, key)
		}
	}

	sort.SliceStable(routes, func(i, j int) bool {
		if routes[i].HostRef != routes[j].HostRef {
			return routes[i].HostRef < routes[j].HostRef
		}
		if len(routes[i].PathPrefix) != len(routes[j].PathPrefix) {
			return len(routes[i].PathPrefix) > len(routes[j].PathPrefix)
		}
		return routes[i].ID < routes[j].ID
	})

	for _, route := range routes {
		routeID := route.ID
		if routeID == "" {
			routeID = BuildGeneratedNginxRouteID(route.MageRunType, route.MageRunCode)
		}
		routePrefix := scopePrefix + routeID + "/"
		config[routePrefix+"host_ref"] = route.HostRef
		config[routePrefix+"path_prefix"] = normalizePathPrefix(route.PathPrefix)
		config[routePrefix+"mage_run_code"] = route.MageRunCode
		config[routePrefix+"mage_run_type"] = route.MageRunType
		config[routePrefix+"strip_path_prefix"] = fmt.Sprintf("%t", route.StripPathPrefix)
	}

	if !saveProjectConfig(configPath, config) {
		return fmt.Errorf("unable to save generated routes to %s", configPath)
	}

	CleanCache()
	return nil
}
