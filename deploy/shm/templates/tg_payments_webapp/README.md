# SHM `tg_payments_webapp` brand routing overlay

## Why this exists

Telegram balance top-up opens the shared SHM public template:

```text
{api.base_url}/shm/v1/public/tg_payments_webapp?...&brand_id=vff|fc&yookassa_ps=yookassa
```

vpnbot already passes `brand_id` and `yookassa_ps` in the launch URL. Upstream
template ignores them and builds payment links as `shm_url + amount` (external:
also concatenates `&email=`).

This overlay teaches the template to add `brand_id` **only** when the payment
URL’s `ps` matches `yookassa_ps`. CryptoCloud and other systems stay unchanged.
Legacy launches without `brand_id` keep unmodified payment URLs.

## Where the template lives

| Access | Endpoint |
|---|---|
| Admin raw source | `GET/POST {api.base_url}/shm/v1/admin/template/tg_payments_webapp` |
| Public render | `GET {api.base_url}/shm/v1/public/tg_payments_webapp` |

Admin auth: Basic Auth from runtime config `.api.api_login` / `.api.api_pass`.
Update is a full-body `POST` with `Content-Type: text/plain; charset=utf-8`.

Do **not** change template settings via this tooling. Public access is verified
by an actual HTTP 200 on the public endpoint (`allow_public`).

## Marker

`VPNBOT_TG_PAYMENT_ROUTING_VERSION=1`

Managed blocks:

1. **PROPS** — `ShmPayApp.brandID` / `yookassaPaySystem`
2. **INIT** — read/trim/validate launch query (`brand_id`, `yookassa_ps`)
3. **ROUTING** — `shm_url + amount` → `URL` → optional `brand_id` / `email` via `searchParams`

Partial config (`brand_id` set, `yookassa_ps` missing/invalid) fail-closes with
an alert and does not open a payment. `return_url` from the query is never used.

## Commands

```bash
make shm-tg-payments-check      # GET + patch + status (no POST)
make shm-tg-payments-diff       # same + redacted diff
make shm-tg-payments-deploy     # backup, POST, verify, public probe
make shm-tg-payments-rollback BACKUP=/opt/shm/template-backups/tg_payments_webapp.<UTC>.html
```

Or:

```bash
bash scripts/deploy-shm-tg-payments-template.sh check|diff|deploy|rollback
```

Config:

- `CONFIG=/path/to/config.json`, or
- download `root@fr-mrs-1:/opt/bot/config-vff.json`

Backups (SHM host, default `ru-msk-1`):

```text
/opt/shm/template-backups/tg_payments_webapp.<UTC>.html
```

Deploy never prints credentials. On any post-install failure it POSTs the
backup body back and re-checks.

## Updating upstream SHM template

1. Do not hand-edit production over SSH/API.
2. `make shm-tg-payments-check` (or `diff`).
3. If the patcher refuses (anchors), update
   `deploy/shm/templates/tg_payments_webapp/patch_template.py` + tests.
4. Only then `make shm-tg-payments-deploy`.

Always re-run check after any upstream template change.

## Local patcher

```bash
python3 deploy/shm/templates/tg_payments_webapp/patch_template.py \
  --source deploy/shm/templates/tg_payments_webapp/testdata/tg_payments_webapp.upstream.html \
  --output /tmp/tg_payments_webapp.patched.html
```

Fixture: `testdata/tg_payments_webapp.upstream.html` (~12 KB production-shaped source).
