<?php

$siteRootPath = rtrim($argv[1] ?? '', '/');
$envPath = $argv[2] ?? ($siteRootPath . '/app/etc/env.php');

if ($siteRootPath === '') {
    fwrite(STDERR, "Site root path is required\n");
    exit(1);
}

if ($envPath === '') {
    fwrite(STDERR, "env.php path is required\n");
    exit(1);
}

if (!file_exists($envPath)) {
    fwrite(STDERR, "env.php was not found: {$envPath}\n");
    exit(1);
}

$env = include $envPath;
if (!is_array($env)) {
    fwrite(STDERR, "env.php must return an array\n");
    exit(1);
}

$result = [
    'routes' => [],
    'warnings' => [],
];

$routeMap = [];
$priorities = ['website' => 0, 'store' => 1];

foreach (['websites' => 'website', 'stores' => 'store'] as $scopeType => $mageRunType) {
    if (empty($env['system'][$scopeType]) || !is_array($env['system'][$scopeType])) {
        continue;
    }

    foreach ($env['system'][$scopeType] as $scopeCode => $scopeConfig) {
        $route = buildRoute($scopeType, $scopeCode, $mageRunType, $scopeConfig);
        if ($route === null) {
            continue;
        }

        $routeKey = $route['host'] . '|' . $route['path_prefix'];
        if (isset($routeMap[$routeKey])) {
            $existing = $routeMap[$routeKey];
            $existingPriority = $priorities[$existing['mage_run_type']] ?? -1;
            $currentPriority = $priorities[$route['mage_run_type']] ?? -1;
            if ($currentPriority > $existingPriority) {
                $result['warnings'][] = sprintf(
                    'Overriding %s:%s with %s:%s for %s%s',
                    $existing['mage_run_type'],
                    $existing['mage_run_code'],
                    $route['mage_run_type'],
                    $route['mage_run_code'],
                    $route['host'],
                    $route['path_prefix']
                );
                $routeMap[$routeKey] = $route;
            } elseif ($existing['mage_run_code'] !== $route['mage_run_code'] || $existing['mage_run_type'] !== $route['mage_run_type']) {
                $result['warnings'][] = sprintf(
                    'Skipping %s:%s because %s:%s already uses %s%s',
                    $route['mage_run_type'],
                    $route['mage_run_code'],
                    $existing['mage_run_type'],
                    $existing['mage_run_code'],
                    $route['host'],
                    $route['path_prefix']
                );
            }
            continue;
        }

        $routeMap[$routeKey] = $route;
    }
}

ksort($routeMap);
$result['routes'] = array_values($routeMap);

echo json_encode($result, JSON_UNESCAPED_SLASHES);

function buildRoute(string $scopeType, string $scopeCode, string $mageRunType, $scopeConfig): ?array
{
    if (!is_array($scopeConfig)) {
        return null;
    }

    $url = detectBaseUrl($scopeConfig);
    if ($url === null) {
        return null;
    }

    $parts = parse_url($url);
    if ($parts === false || empty($parts['host'])) {
        return null;
    }

    $pathPrefix = normalizePathPrefix($parts['path'] ?? '/');
    if ($pathPrefix === '/' || $pathPrefix === '') {
        return null;
    }

    return [
        'scope_type' => $scopeType,
        'scope_code' => $scopeCode,
        'host' => strtolower($parts['host']),
        'path_prefix' => $pathPrefix,
        'mage_run_code' => $scopeCode,
        'mage_run_type' => $mageRunType,
        'strip_path_prefix' => false,
        'source_url' => $url,
    ];
}

function detectBaseUrl(array $scopeConfig): ?string
{
    $webConfig = $scopeConfig['web'] ?? [];
    if (!is_array($webConfig)) {
        return null;
    }

    $candidates = [
        $webConfig['secure']['base_link_url'] ?? null,
        $webConfig['secure']['base_url'] ?? null,
        $webConfig['unsecure']['base_link_url'] ?? null,
        $webConfig['unsecure']['base_url'] ?? null,
    ];

    foreach ($candidates as $candidate) {
        if (!is_string($candidate) || trim($candidate) === '') {
            continue;
        }
        if (strpos($candidate, '{{') !== false) {
            continue;
        }
        $parts = parse_url($candidate);
        if ($parts === false || empty($parts['host'])) {
            continue;
        }
        $pathPrefix = normalizePathPrefix($parts['path'] ?? '/');
        if ($pathPrefix !== '/' && $pathPrefix !== '') {
            return $candidate;
        }
    }

    return null;
}

function normalizePathPrefix(string $path): string
{
    $path = '/' . trim($path, '/');
    if ($path === '/' || $path === '/.') {
        return '/';
    }
    return rtrim($path, '/');
}