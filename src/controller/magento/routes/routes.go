package routes

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/faradey/madock/v3/src/command"
	cliHelper "github.com/faradey/madock/v3/src/helper/cli"
	"github.com/faradey/madock/v3/src/helper/cli/arg_struct"
	"github.com/faradey/madock/v3/src/helper/cli/attr"
	"github.com/faradey/madock/v3/src/helper/cli/fmtc"
	"github.com/faradey/madock/v3/src/helper/configs"
	"github.com/faradey/madock/v3/src/helper/docker"
	"github.com/faradey/madock/v3/src/helper/logger"
)

type detectedRoute struct {
	ScopeType       string `json:"scope_type"`
	ScopeCode       string `json:"scope_code"`
	Host            string `json:"host"`
	PathPrefix      string `json:"path_prefix"`
	MageRunCode     string `json:"mage_run_code"`
	MageRunType     string `json:"mage_run_type"`
	StripPathPrefix bool   `json:"strip_path_prefix"`
	SourceURL       string `json:"source_url"`
}

type detectedRoutesResult struct {
	Routes   []detectedRoute `json:"routes"`
	Warnings []string        `json:"warnings"`
}

func init() {
	command.Register(&command.Definition{
		Aliases:  []string{"magento:routes:generate", "m:r:g"},
		Handler:  Execute,
		Help:     "Generate nginx/routes from Magento env.php base URLs",
		Category: "magento",
		ArgsType: new(arg_struct.ControllerMagentoRoutesGenerate),
	})
}

func Execute() {
	args := attr.Parse(new(arg_struct.ControllerMagentoRoutesGenerate)).(*arg_struct.ControllerMagentoRoutesGenerate)

	projectConf := configs.GetCurrentProjectConfig()
	if projectConf["platform"] != "magento2" {
		fmtc.WarningIconLn("This command is supported only for Magento 2 projects")
		return
	}

	projectName := configs.GetProjectName()
	service, user, workdir := cliHelper.GetEnvForUserServiceWorkdir("php", "www-data", projectConf["workdir"])
	envFile := resolveEnvFilePath(workdir, args.File)

	result, err := detectMagentoRoutes(projectName, projectConf, service, user, workdir, envFile)
	if err != nil {
		logger.Fatal(err)
	}

	for _, warning := range result.Warnings {
		fmtc.WarningIconLn(warning)
	}

	routes, warnings, err := buildGeneratedRoutes(result.Routes, projectConf, args.HostCode)
	if err != nil {
		logger.Fatal(err)
	}
	for _, warning := range warnings {
		fmtc.WarningIconLn(warning)
	}

	if len(routes) == 0 {
		fmtc.WarningIconLn("No path-based Magento routes were found")
		fmtc.ToDoLn("Ensure app/etc/env.php contains store or website base URLs with path prefixes")
		return
	}

	fmt.Println("")
	fmtc.TitleLn("Generated Magento Path Routes")
	for _, route := range routes {
		fmtc.PrintKeyValue(route.ID, route.HostRef+" "+route.PathPrefix+" -> "+route.MageRunType+":"+route.MageRunCode)
	}

	if args.DryRun {
		fmt.Println("")
		fmtc.InfoIconLn("Dry run: config.xml was not changed")
		return
	}

	activeScope := projectConf["activeScope"]
	if activeScope == "" {
		activeScope = "default"
	}
	if err := configs.ReplaceGeneratedNginxRoutes(projectName, activeScope, routes); err != nil {
		logger.Fatal(err)
	}

	fmt.Println("")
	fmtc.SuccessIconLn(fmt.Sprintf("Applied %d generated route(s) to scope %s", len(routes), activeScope))
	fmtc.ToDoLn("Run madock rebuild")
}

func detectMagentoRoutes(projectName string, projectConf map[string]string, service, user, workdir, envFile string) (detectedRoutesResult, error) {
	containerName := docker.GetContainerName(projectConf, projectName, service)
	commandArgs := []string{"php", "/var/www/scripts/php/magento-routes.php", workdir, envFile}
	cmd, err := docker.PrepareContainerExec(containerName, user, false, commandArgs...)
	if err != nil {
		return detectedRoutesResult{}, err
	}

	output, runErr := cmd.CombinedOutput()
	docker.NotifyExecDone(containerName, commandArgs, runErr)
	if runErr != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = runErr.Error()
		}
		return detectedRoutesResult{}, fmt.Errorf("unable to extract Magento routes from %s: %s", envFile, message)
	}

	var result detectedRoutesResult
	if err := json.Unmarshal(output, &result); err != nil {
		return detectedRoutesResult{}, fmt.Errorf("unable to parse Magento route data: %w", err)
	}

	return result, nil
}

func resolveEnvFilePath(workdir, input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return strings.TrimRight(workdir, "/") + "/app/etc/env.php"
	}
	if strings.HasPrefix(input, "/") {
		return input
	}
	return strings.TrimRight(workdir, "/") + "/" + strings.TrimLeft(strings.TrimPrefix(input, "./"), "/")
}

func buildGeneratedRoutes(detected []detectedRoute, projectConf map[string]string, hostCodeOverride string) ([]configs.NginxRoute, []string, error) {
	hostByName := make(map[string]string)
	hostByCode := make(map[string]string)
	for _, host := range configs.GetHosts(projectConf) {
		hostName := strings.ToLower(strings.TrimSpace(host["name"]))
		hostCode := strings.TrimSpace(host["code"])
		hostByName[hostName] = hostCode
		hostByCode[hostCode] = hostName
	}

	if len(hostByCode) == 0 {
		return nil, nil, fmt.Errorf("no nginx/hosts are configured for the current project")
	}

	hostCodeOverride = strings.TrimSpace(hostCodeOverride)
	if hostCodeOverride != "" {
		if _, ok := hostByCode[hostCodeOverride]; !ok {
			return nil, nil, fmt.Errorf("unknown nginx host code: %s", hostCodeOverride)
		}
	}

	sort.SliceStable(detected, func(i, j int) bool {
		if len(detected[i].PathPrefix) != len(detected[j].PathPrefix) {
			return len(detected[i].PathPrefix) > len(detected[j].PathPrefix)
		}
		if detected[i].MageRunType != detected[j].MageRunType {
			return routeTypePriority(detected[i].MageRunType) < routeTypePriority(detected[j].MageRunType)
		}
		if detected[i].Host != detected[j].Host {
			return detected[i].Host < detected[j].Host
		}
		return detected[i].MageRunCode < detected[j].MageRunCode
	})

	var warnings []string
	seenByHostAndPath := make(map[string]configs.NginxRoute)
	generated := make([]configs.NginxRoute, 0, len(detected))

	for _, suggestion := range detected {
		pathPrefix := strings.TrimSpace(suggestion.PathPrefix)
		if pathPrefix == "" || pathPrefix == "/" {
			continue
		}

		mageRunType := strings.TrimSpace(suggestion.MageRunType)
		if mageRunType == "" {
			if suggestion.ScopeType == "stores" {
				mageRunType = "store"
			} else {
				mageRunType = "website"
			}
		}

		mageRunCode := strings.TrimSpace(suggestion.MageRunCode)
		if mageRunCode == "" {
			mageRunCode = strings.TrimSpace(suggestion.ScopeCode)
		}
		if mageRunCode == "" {
			continue
		}

		hostRef := hostCodeOverride
		if hostRef == "" {
			hostName := strings.ToLower(strings.TrimSpace(suggestion.Host))
			hostRef = hostByName[hostName]
			if hostRef == "" {
				warnings = append(warnings, fmt.Sprintf("Skipped %s:%s for host %s because nginx/hosts has no matching entry. Use --host-code to force one host.", mageRunType, mageRunCode, suggestion.Host))
				continue
			}
		}

		key := hostRef + "|" + pathPrefix
		if existing, ok := seenByHostAndPath[key]; ok {
			warnings = append(warnings, fmt.Sprintf("Skipped duplicate route %s:%s for %s%s because %s:%s already owns that path.", mageRunType, mageRunCode, hostRef, pathPrefix, existing.MageRunType, existing.MageRunCode))
			continue
		}

		route := configs.NginxRoute{
			ID:              configs.BuildGeneratedNginxRouteID(mageRunType, mageRunCode),
			HostRef:         hostRef,
			HostName:        hostByCode[hostRef],
			PathPrefix:      pathPrefix,
			MageRunCode:     mageRunCode,
			MageRunType:     mageRunType,
			StripPathPrefix: suggestion.StripPathPrefix,
			Enabled:         true,
		}
		generated = append(generated, route)
		seenByHostAndPath[key] = route
	}

	sort.SliceStable(generated, func(i, j int) bool {
		if generated[i].HostRef != generated[j].HostRef {
			return generated[i].HostRef < generated[j].HostRef
		}
		if len(generated[i].PathPrefix) != len(generated[j].PathPrefix) {
			return len(generated[i].PathPrefix) > len(generated[j].PathPrefix)
		}
		return generated[i].ID < generated[j].ID
	})

	return generated, warnings, nil
}

func routeTypePriority(routeType string) int {
	switch strings.ToLower(strings.TrimSpace(routeType)) {
	case "store":
		return 0
	case "website":
		return 1
	default:
		return 2
	}
}
