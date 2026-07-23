# SHM YooKassa CGI brand routing overlay

## Why this exists

SHM’s host-mounted `yookassa.cgi` is shared by all brands (`ps=yookassa`).
vpnbot already sends `brand_id=vff|fc` on create URLs. Upstream CGI ignores
`brand_id` and always uses the single `return_url` from SHM pay-system config.

This overlay inserts a versioned routing block so each brand gets its own
post-payment return URL without splitting YooKassa credentials or changing SHM
config / docker-compose.

Managed file on SHM host (`ru-msk-1` by default):

- `/opt/shm/pay_systems/yookassa.cgi`
- mounted into core/spool as `/app/data/pay_systems/yookassa.cgi`

## Brand mapping source

Return URLs are **not** hard-coded in the deploy script. The patcher reads:

- `deploy/brands/vff.json` → `id` + `brand.public_base_url`
- `deploy/brands/fc.json` → `id` + `brand.public_base_url`

and builds one shared allowlist:

```text
<public_base_url without trailing slash>/payment/return
```

The same allowlist is used by both `create`/`payment` and the diagnostic action.

## VERSION=2 behaviour

Marker: `VPNBOT_BRAND_ROUTING_VERSION=2`

### create / payment (unchanged from VERSION=1 semantics)

| `brand_id` | result |
|---|---|
| `vff` / `fc` | brand-specific return URL applied to `$return_url` |
| unknown non-empty | HTTP 400 `Error: unknown brand_id` (before user lookup) |
| absent / empty | legacy `$ps_config{return_url}` |

### Diagnostic action: `vpnbot_route_check`

Safe, side-effect-free probe of the server-side allowlist. Does **not** create a
payment, look up users, call YooKassa, or read amount/email/description.

```text
GET .../yookassa.cgi?action=vpnbot_route_check&brand_id=vff&ps=yookassa
```

Success (HTTP 200):

```json
{
  "status": 200,
  "brand_id": "vff",
  "return_url": "https://connect.vpn-for-friends.com/payment/return"
}
```

Missing / empty / whitespace / unknown `brand_id` → HTTP 400
`Error: unknown brand_id`.

The response contains only the public route. It never returns API keys,
`account_id`, callbacks, SHM config, or user data. Client-supplied `return_url`
is ignored.

### Block layout

1. **SHARED** (before `create`/`payment`): allowlist + `vpnbot_route_check`
2. **CREATE_VALIDATE** (inside create/payment, before user lookup)
3. **RETURN_APPLY** (after legacy `my $return_url = $ps_config{return_url}`)

## Upgrade VERSION=1 → VERSION=2

Production may already have a complete VERSION=1 overlay. The patcher:

- validates that VERSION=1 managed blocks are complete;
- strips them and rebuilds VERSION=2 from brand profiles;
- does **not** require upstream adjacency anchors on the pre-strip V1 file;
- refuses damaged/partial/duplicated managed blocks and unknown versions.

```bash
make shm-yookassa-check   # reports upgrade available VERSION=1 → VERSION=2
make shm-yookassa-diff    # redacted managed migration only
make shm-yookassa-deploy  # backup, perl -c, atomic install, probes
```

## Commands

```bash
make shm-yookassa-check
make shm-yookassa-diff
make shm-yookassa-deploy
make shm-yookassa-rollback BACKUP=/opt/shm/pay_systems/yookassa.cgi.bak.<UTC>
```

Or:

```bash
bash scripts/deploy-shm-yookassa.sh check|diff|deploy|rollback
```

Overrides: `SHM_HOST` (default `ru-msk-1`), `SHM_USER`, `SHM_DIR`,
`SHM_CORE_SERVICE`, `SHM_YK_BRAND_PROFILES`.

Deploy does **not** restart SHM and does **not** edit docker-compose / SHM config.
On any post-install failure it restores the timestamped backup automatically.

### Safe probes (against runtime `api.base_url`)

Route-check:

1. `brand_id=vff` → HTTP 200 + exact VFF return_url from brand profile
2. `brand_id=fc` → HTTP 200 + exact FC return_url from brand profile
3. invalid `brand_id` → HTTP 400 `unknown brand_id`
4. absent `brand_id` → HTTP 400 `unknown brand_id`

Create (still `user_id=-1`, never real payments):

1. `brand_id=vff` → HTTP 400 `unknown user`
2. `brand_id=fc` → HTTP 400 `unknown user`
3. invalid `brand_id` → HTTP 400 `unknown brand_id`
4. no `brand_id` → HTTP 400 `unknown user` (legacy path)

## Updating upstream SHM CGI

When SHM upgrades `yookassa.cgi`:

1. Do **not** hand-edit production over SSH.
2. Run `make shm-yookassa-check` (or `diff`).
3. If the patcher refuses, update anchors/tests, then re-check.
4. Only then run `make shm-yookassa-deploy`.

**Always re-run check after any upstream CGI change.**

## Local patcher

```bash
python3 deploy/shm/yookassa/patch_yookassa.py \
  --source /path/to/yookassa.cgi \
  --brand-profile deploy/brands/vff.json \
  --brand-profile deploy/brands/fc.json \
  --output /tmp/yookassa.cgi.patched
```

Fixtures:

- `deploy/shm/yookassa/testdata/yookassa.cgi.upstream`
- `deploy/shm/yookassa/testdata/yookassa.cgi.v1`
