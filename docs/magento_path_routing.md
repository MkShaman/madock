# Magento Path-Based Routing

This guide covers Magento projects where one domain serves multiple websites or store views by URL path, for example:

- `https://example.test/cl/es/`
- `https://example.test/au/en/`

In this setup, `nginx/hosts` alone is not enough because host-based mapping can choose only one fallback `MAGE_RUN_CODE` per domain.

## Manual Route Rules

Use `nginx/routes` when you need explicit path-based routing.

Example:

```xml
<nginx>
    <hosts>
        <base>
            <name>example.test</name>
        </base>
    </hosts>
    <routes>
        <cl_es>
            <host_ref>base</host_ref>
            <path_prefix>/cl/es</path_prefix>
            <mage_run_code>store_cl_es</mage_run_code>
            <mage_run_type>store</mage_run_type>
            <strip_path_prefix>true</strip_path_prefix>
        </cl_es>
        <au_en>
            <host_ref>base</host_ref>
            <path_prefix>/au/en</path_prefix>
            <mage_run_code>store_au_en</mage_run_code>
            <mage_run_type>store</mage_run_type>
            <strip_path_prefix>true</strip_path_prefix>
        </au_en>
    </routes>
</nginx>
```

Notes:

- `host_ref` points to an existing `nginx/hosts/<code>/name` entry.
- `strip_path_prefix=true` rewrites `/cl/es/...` to `/...` before Magento handles the request.
- Host-only routing remains the fallback when no route matches.

## Automatic Generation From env.php

If `app/etc/env.php` already contains local base URLs with path prefixes, madock can generate route rules automatically.

Preview the generated routes first:

```bash
madock magento:routes:generate --dry-run
```

Apply the generated routes to the current madock scope:

```bash
madock magento:routes:generate
madock rebuild
```

When `env.php` hostnames do not match your current `nginx/hosts` values, force all generated routes to use one configured host code:

```bash
madock magento:routes:generate --host-code=base
madock rebuild
```

Optional flags:

- `--dry-run`: show the generated routes without changing `config.xml`
- `--host-code=<code>`: attach all generated routes to one existing nginx host code
- `--file=<path>`: read a non-standard env.php path inside the container

## Managed vs Manual Routes

The generator writes only `nginx/routes/auto_*` entries. Manual route IDs stay untouched.

That means you can combine both approaches safely:

- keep special-case manual routes in `nginx/routes/custom_*`
- regenerate `nginx/routes/auto_*` from Magento whenever base URLs change

## Recommended Workflow

1. Make sure `nginx/hosts` contains the domain you want to serve locally.
2. Ensure `app/etc/env.php` contains the correct local base URLs.
3. Run `madock magento:routes:generate --dry-run` and verify the result.
4. Run `madock magento:routes:generate`.
5. Run `madock rebuild`.