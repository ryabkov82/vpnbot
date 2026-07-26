#!/usr/bin/env python3
"""Deterministic patcher for SHM tg_payments_webapp template brand routing.

Inserts VPNBOT_TG_PAYMENT_ROUTING_VERSION=1 so Telegram WebApp launch params
brand_id / yookassa_ps are applied only when the payment URL's ps matches the
brand YooKassa pay system. CryptoCloud and other systems stay unmodified.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path
from typing import List, Optional, Sequence

TARGET_VERSION = 1
MARKER = f"VPNBOT_TG_PAYMENT_ROUTING_VERSION={TARGET_VERSION}"
MARKER_PREFIX = "VPNBOT_TG_PAYMENT_ROUTING_VERSION="

BEGIN_PROPS = "        // BEGIN VPNBOT_TG_PAYMENT_PROPS"
END_PROPS = "        // END VPNBOT_TG_PAYMENT_PROPS"
BEGIN_INIT = "            // BEGIN VPNBOT_TG_PAYMENT_INIT"
END_INIT = "            // END VPNBOT_TG_PAYMENT_INIT"
BEGIN_PAY = "            // BEGIN VPNBOT_TG_PAYMENT_ROUTING"
END_PAY = "            // END VPNBOT_TG_PAYMENT_ROUTING"

BLOCK_PAIRS = (
    (BEGIN_PROPS, END_PROPS),
    (BEGIN_INIT, END_INIT),
    (BEGIN_PAY, END_PAY),
)

# Exact unique anchors from production tg_payments_webapp.
ANCHOR_ACK_EMAIL_PROP = "        ackEmail        : false,"
ANCHOR_ACK_EMAIL_GET = "            let ack_email = urlParams.get('ack_email');"
ANCHOR_AUTH_URL = (
    "            let xhrURL = new URL("
    "'{{ config.api.url }}/shm/v1/telegram/webapp/auth');"
)
ANCHOR_PAYSYSTEMS = (
    "            xhr.open('GET', "
    "'{{ config.api.url }}/shm/v1/user/pay/paysystems');"
)
ANCHOR_MAKE_PAYMENT = "        makePayment(shm_url, internal) {"
ANCHOR_INTERNAL_IF = "            if ( internal ) {"
ANCHOR_INTERNAL_OPEN = "                xhr.open('GET', shm_url + amount);"
ANCHOR_EXTERNAL_OPEN = (
    "                Telegram.WebApp.openLink( "
    "shm_url + amount + '&email=' +email, { try_instant_view: false } );"
)
ANCHOR_REMOVE_PAYMENT = "        removePayment(id,name) {"

MODERN_INTERNAL_OPEN = "                xhr.open('GET', paymentHref);"
MODERN_EXTERNAL_OPEN = (
    "                paymentURL.searchParams.set('email', email || '');\n"
    "                paymentHref = paymentURL.toString();\n"
    "                Telegram.WebApp.openLink( paymentHref, "
    "{ try_instant_view: false } );"
)

VERSION_RE = re.compile(r"VPNBOT_TG_PAYMENT_ROUTING_VERSION=([0-9]+)")


class PatchError(Exception):
    """Fatal patcher error."""


def _count(haystack: str, needle: str) -> int:
    return haystack.count(needle)


UPSTREAM_UNIQUE = (
    ("ackEmail property", ANCHOR_ACK_EMAIL_PROP),
    ("ack_email query read", ANCHOR_ACK_EMAIL_GET),
    ("telegram webapp auth URL", ANCHOR_AUTH_URL),
    ("paysystems API URL", ANCHOR_PAYSYSTEMS),
    ("makePayment function", ANCHOR_MAKE_PAYMENT),
    ("internal payment branch", ANCHOR_INTERNAL_IF),
    ("internal shm_url+amount", ANCHOR_INTERNAL_OPEN),
    ("external openLink concat", ANCHOR_EXTERNAL_OPEN),
    ("removePayment function", ANCHOR_REMOVE_PAYMENT),
)


def require_unique_anchors(source: str) -> None:
    missing: List[str] = []
    duplicated: List[str] = []
    for label, anchor in UPSTREAM_UNIQUE:
        n = _count(source, anchor)
        if n == 0:
            missing.append(label)
        elif n > 1:
            duplicated.append(f"{label} ({n})")
    if missing or duplicated:
        parts: List[str] = []
        if missing:
            parts.append("missing: " + ", ".join(missing))
        if duplicated:
            parts.append("duplicated: " + ", ".join(duplicated))
        raise PatchError(
            "refusing to patch: template anchors are not unique/exact ("
            + "; ".join(parts)
            + ")"
        )


def detect_marker_version(source: str) -> Optional[int]:
    versions = VERSION_RE.findall(source)
    if not versions:
        return None
    uniq = sorted({int(v) for v in versions})
    if len(uniq) != 1:
        raise PatchError(
            f"refusing to patch: conflicting routing markers: {uniq}"
        )
    return uniq[0]


def _strip_one_block(source: str, begin_m: str, end_m: str) -> str:
    begin = source.find(begin_m)
    end = source.find(end_m)
    if begin < 0 and end < 0:
        return source
    if begin < 0 or end < 0:
        raise PatchError(
            "refusing to patch: incomplete VPNBOT managed block markers"
        )
    if source.count(begin_m) != 1 or source.count(end_m) != 1:
        raise PatchError(
            "refusing to patch: duplicated VPNBOT managed block markers"
        )
    if end < begin:
        raise PatchError("refusing to patch: END marker before BEGIN marker")
    end_line = end + len(end_m)
    if end_line < len(source) and source[end_line] == "\n":
        end_line += 1
    return source[:begin] + source[end_line:]


def strip_managed_blocks(source: str) -> str:
    result = source
    for begin_m, end_m in BLOCK_PAIRS:
        result = _strip_one_block(result, begin_m, end_m)
    if MARKER_PREFIX in result:
        raise PatchError(
            "refusing to patch: routing marker present without "
            "known BEGIN/END block boundaries"
        )
    return result


def _require_exact_pair(source: str, begin_m: str, end_m: str, label: str) -> None:
    bc = source.count(begin_m)
    ec = source.count(end_m)
    if bc == 0 and ec == 0:
        raise PatchError(f"refusing to patch: {label} managed block missing")
    if bc != 1 or ec != 1:
        raise PatchError(
            f"refusing to patch: {label} managed block not unique "
            f"(begin={bc}, end={ec})"
        )


def require_complete_v1(source: str) -> None:
    for begin_m, end_m, label in (
        (BEGIN_PROPS, END_PROPS, "PROPS"),
        (BEGIN_INIT, END_INIT, "INIT"),
        (BEGIN_PAY, END_PAY, "ROUTING"),
    ):
        _require_exact_pair(source, begin_m, end_m, label)
    if MARKER not in source:
        raise PatchError("refusing to patch: VERSION=1 marker missing")
    if "ShmPayApp.brandID" not in source:
        raise PatchError("refusing to patch: brandID wiring missing")
    if "yookassaPaySystem" not in source:
        raise PatchError("refusing to patch: yookassaPaySystem wiring missing")
    if MODERN_INTERNAL_OPEN not in source:
        raise PatchError("refusing to patch: modern internal open missing")
    if "paymentURL.searchParams.set('email'" not in source:
        raise PatchError("refusing to patch: searchParams email missing")


def build_props_block() -> str:
    return "\n".join(
        [
            BEGIN_PROPS,
            f"        // {MARKER}",
            "        brandID         : null,",
            "        yookassaPaySystem : null,",
            END_PROPS,
        ]
    ) + "\n"


def build_init_block() -> str:
    return "\n".join(
        [
            BEGIN_INIT,
            f"            // {MARKER}",
            "            // Managed by vpnbot — do not edit by hand.",
            "            // Never accept return_url from the launch query string.",
            "            let brand_id = (urlParams.get('brand_id') || '').trim();",
            "            let yookassa_ps = (urlParams.get('yookassa_ps') || '').trim();",
            "            if (brand_id) {",
            "                if (!yookassa_ps || "
            "!/^[a-z0-9][a-z0-9_-]*$/i.test(yookassa_ps)) {",
            "                    Telegram.WebApp.showAlert("
            '"Ошибка: Некорректная конфигурация оплаты");',
            "                    Telegram.WebApp.close();",
            "                    return;",
            "                }",
            "                if (!/^[a-z0-9][a-z0-9_-]*$/i.test(brand_id)) {",
            "                    Telegram.WebApp.showAlert("
            '"Ошибка: Некорректная конфигурация оплаты");',
            "                    Telegram.WebApp.close();",
            "                    return;",
            "                }",
            "                ShmPayApp.brandID = brand_id;",
            "                ShmPayApp.yookassaPaySystem = yookassa_ps;",
            "            } else {",
            "                ShmPayApp.brandID = null;",
            "                ShmPayApp.yookassaPaySystem = null;",
            "            }",
            END_INIT,
        ]
    ) + "\n"


def build_pay_block() -> str:
    return "\n".join(
        [
            BEGIN_PAY,
            f"            // {MARKER}",
            "            // Contract: shm_url + amount, then searchParams.",
            "            var rawPaymentURL = shm_url + amount;",
            "            var paymentURL;",
            "            try {",
            "                paymentURL = new URL("
            "rawPaymentURL, window.location.origin);",
            "            } catch (e) {",
            "                Telegram.WebApp.showAlert("
            '"Ошибка: Некорректный URL оплаты");',
            "                return;",
            "            }",
            "            var actualPs = paymentURL.searchParams.get('ps');",
            "            if (ShmPayApp.brandID) {",
            "                if (!ShmPayApp.yookassaPaySystem) {",
            "                    Telegram.WebApp.showAlert("
            '"Ошибка: Некорректная конфигурация оплаты");',
            "                    return;",
            "                }",
            "                if (actualPs === ShmPayApp.yookassaPaySystem) {",
            "                    paymentURL.searchParams.set("
            "'brand_id', ShmPayApp.brandID);",
            "                }",
            "            }",
            "            var paymentHref = paymentURL.toString();",
            END_PAY,
        ]
    ) + "\n"


def _insert_after_line(base: str, anchor: str, block: str, label: str) -> str:
    needle = anchor + "\n"
    if _count(base, needle) != 1:
        raise PatchError(f"refusing to patch: {label} line+newline not unique")
    out = base.replace(needle, needle + block, 1)
    if out == base:
        raise PatchError(f"internal error: failed to insert {label}")
    return out


def _replace_exact(base: str, old: str, new: str, label: str) -> str:
    if _count(base, old) != 1:
        raise PatchError(f"refusing to patch: {label} not unique for replace")
    out = base.replace(old, new, 1)
    if out == base:
        raise PatchError(f"internal error: failed to replace {label}")
    return out


def _postcheck(patched: str) -> None:
    require_complete_v1(patched)
    if _count(patched, ANCHOR_AUTH_URL) != 1:
        raise PatchError("post-check failed: auth URL changed")
    if _count(patched, ANCHOR_PAYSYSTEMS) != 1:
        raise PatchError("post-check failed: paysystems URL changed")
    if "shm_url + amount + '&email='" in patched:
        raise PatchError("post-check failed: legacy email concat remains")
    if "xhr.open('GET', shm_url + amount)" in patched:
        raise PatchError("post-check failed: legacy internal concat remains")

    # Only the managed comment may mention return_url.
    for line in patched.splitlines():
        if "return_url" in line.lower() and "Never accept return_url" not in line:
            raise PatchError(
                f"post-check failed: unexpected return_url reference: {line!r}"
            )

    idx_init = patched.find(BEGIN_INIT)
    idx_auth = patched.find(ANCHOR_AUTH_URL)
    idx_pay = patched.find(BEGIN_PAY)
    idx_internal = patched.find(MODERN_INTERNAL_OPEN)
    idx_external = patched.find("Telegram.WebApp.openLink( paymentHref,")
    if min(idx_init, idx_auth, idx_pay, idx_internal, idx_external) < 0:
        raise PatchError("post-check failed: order anchors missing")
    if not (idx_init < idx_auth < idx_pay < idx_internal < idx_external):
        raise PatchError("post-check failed: invalid block order")


def build_v1_on_upstream(base: str) -> str:
    require_unique_anchors(base)

    patched = _insert_after_line(
        base, ANCHOR_ACK_EMAIL_PROP, build_props_block(), "props"
    )
    patched = _insert_after_line(
        patched, ANCHOR_ACK_EMAIL_GET, build_init_block(), "init"
    )

    if _count(patched, ANCHOR_INTERNAL_IF) != 1:
        raise PatchError("refusing to patch: internal branch not unique")
    idx = patched.find(ANCHOR_INTERNAL_IF)
    # Ensure we insert inside makePayment (after email validation).
    make_idx = patched.find(ANCHOR_MAKE_PAYMENT)
    if make_idx < 0 or idx < make_idx:
        raise PatchError(
            "refusing to patch: internal branch is not inside makePayment"
        )
    patched = patched[:idx] + build_pay_block() + patched[idx:]

    patched = _replace_exact(
        patched, ANCHOR_INTERNAL_OPEN, MODERN_INTERNAL_OPEN, "internal open"
    )
    patched = _replace_exact(
        patched, ANCHOR_EXTERNAL_OPEN, MODERN_EXTERNAL_OPEN, "external openLink"
    )

    _postcheck(patched)
    return patched


def _restore_legacy_opens(source: str) -> str:
    """Reverse makePayment open-line replacements left after block strip."""
    out = source
    if MODERN_INTERNAL_OPEN in out:
        out = _replace_exact(
            out, MODERN_INTERNAL_OPEN, ANCHOR_INTERNAL_OPEN, "restore internal"
        )
    if MODERN_EXTERNAL_OPEN in out:
        out = _replace_exact(
            out, MODERN_EXTERNAL_OPEN, ANCHOR_EXTERNAL_OPEN, "restore external"
        )
    return out


def apply_patch(source: str) -> str:
    version = detect_marker_version(source)

    if version is not None and version != TARGET_VERSION:
        raise PatchError(
            f"refusing to patch: unsupported routing marker version {version} "
            f"(need {TARGET_VERSION})"
        )

    if version is None:
        if any(
            m in source
            for m in (BEGIN_PROPS, BEGIN_INIT, BEGIN_PAY, MARKER_PREFIX)
        ):
            raise PatchError(
                "refusing to patch: partial/unmanaged VPNBOT markers without "
                "a single known version"
            )
        return build_v1_on_upstream(source)

    require_complete_v1(source)
    base = strip_managed_blocks(source)
    base = _restore_legacy_opens(base)
    return build_v1_on_upstream(base)


def patch_file(source_path: Path, output_path: Path) -> str:
    try:
        source = source_path.read_text(encoding="utf-8")
    except OSError as exc:
        raise PatchError(f"cannot read source template {source_path}: {exc}") from exc

    patched = apply_patch(source)
    version = detect_marker_version(source)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(patched, encoding="utf-8")

    if patched == source:
        return f"already applied: {MARKER} (identical)"
    if version == 1:
        return f"updated: regenerated {MARKER}"
    return f"patched: inserted {MARKER}"


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Insert vpnbot brand_id routing into SHM tg_payments_webapp"
    )
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args(argv)
    try:
        status = patch_file(args.source, args.output)
    except PatchError as exc:
        print(f"patch_template: {exc}", file=sys.stderr)
        return 1
    print(f"patch_template: {status}")
    print(f"patch_template: wrote {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
