# SHM YooKassa CGI brand routing overlay

## Why this exists

SHM’s host-mounted `yookassa.cgi` is shared by all brands (`ps=yookassa`).
vpnbot already sends `brand_id=vff|fc` on create URLs. Upstream CGI ignores
`brand_id` and always uses the single `return_url` from SHM pay-system config.

This overlay inserts a small, versioned routing block so each brand gets its own
post-payment return URL without splitting YooKassa credentials or changing SHM
config / docker-compose.

Managed file on SHM host (`ru-msk-1` by default):

- `/opt/shm/pay_systems/yookassa.cgi`
- mounted into core/spool as `/app/data/pay_systems/yookassa.cgi`

## Brand mapping source

Return URLs are **not** hard-coded in the deploy script. The patcher reads:

- `deploy/brands/vff.json` → `id` + `brand.public_base_url`
- `deploy/brands/fc.json` → `id` + `brand.public_base_url`

and builds:

```text
<public_base_url without trailing slash>/payment/return
```

Behaviour after patch (`VPNBOT_BRAND_ROUTING_VERSION=1`):

| `brand_id` | result |
|---|---|
| `vff` / `fc` | brand-specific return URL |
| unknown non-empty | HTTP 400 `Error: unknown brand_id` |
| absent / empty | legacy `return_url` from SHM yookassa config |

Credentials, callbacks, metadata, and the rest of CGI logic are left untouched.
Do **not** store YooKassa API keys / `account_id` in git.

## Commands

From repo root:

```bash
make shm-yookassa-check      # download + patch + status (no install)
make shm-yookassa-diff       # same + redacted unified diff
make shm-yookassa-deploy     # backup, perl -c, atomic install, probes
make shm-yookassa-rollback BACKUP=/opt/shm/pay_systems/yookassa.cgi.bak.<UTC>
```

Or directly:

```bash
bash scripts/deploy-shm-yookassa.sh check|diff|deploy|rollback
```

Overrides:

- `SHM_HOST` (default `ru-msk-1`)
- `SHM_USER` (default `root`)
- `SHM_DIR` (default `/opt/shm`)
- `SHM_CORE_SERVICE` (default `core`)
- `SHM_YK_BRAND_PROFILES` (optional space-separated profile paths)

Deploy does **not** restart SHM and does **not** edit docker-compose / SHM config.
On any post-install failure it restores the timestamped backup automatically.

Safe probes (always `user_id=-1`, never real payments), against runtime
`api.base_url` (not brand `public_base_url`):

1. `brand_id=vff` → HTTP 400 + `unknown user`
2. `brand_id=fc` → HTTP 400 + `unknown user`
3. invalid `brand_id` → HTTP 400 + `unknown brand_id`
4. no `brand_id` → HTTP 400 + `unknown user` (legacy path)

## Updating upstream SHM CGI

When SHM upgrades `yookassa.cgi`:

1. Do **not** hand-edit production over SSH.
2. Run `make shm-yookassa-check` (or `diff`).
3. If the patcher refuses (missing/duplicated anchors), update
   `deploy/shm/yookassa/patch_yookassa.py` anchors/tests against the new upstream
   shape, then re-check.
4. Only then run `make shm-yookassa-deploy`.

**Always re-run check after any upstream CGI change** — a successful previous
deploy does not guarantee the next upstream file still matches our anchors.

## Local patcher

```bash
python3 deploy/shm/yookassa/patch_yookassa.py \
  --source /path/to/yookassa.cgi \
  --brand-profile deploy/brands/vff.json \
  --brand-profile deploy/brands/fc.json \
  --output /tmp/yookassa.cgi.patched
```

Fixture used by tests: `deploy/shm/yookassa/testdata/yookassa.cgi.upstream`.
