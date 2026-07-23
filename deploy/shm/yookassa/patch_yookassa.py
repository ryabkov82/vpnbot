#!/usr/bin/env python3
"""Deterministic patcher for host-mounted SHM yookassa.cgi brand return_url routing.

Reads brand id + brand.public_base_url from vpnbot brand profiles and inserts a
versioned, fail-closed brand_id → return_url map into upstream yookassa.cgi.

VERSION=2:
  - one shared allowlist used by create/payment and vpnbot_route_check;
  - vpnbot_route_check returns the selected public return_url without payment;
  - brand validation still runs before user lookup inside create/payment.

Supports safe upgrade of a complete VERSION=1 overlay to VERSION=2 without
matching upstream adjacency anchors on the already-patched V1 file.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Dict, List, Optional, Sequence, Tuple
from urllib.parse import urlparse

TARGET_VERSION = 2
MARKER = f"VPNBOT_BRAND_ROUTING_VERSION={TARGET_VERSION}"
MARKER_PREFIX = "VPNBOT_BRAND_ROUTING_VERSION="
MARKER_V1 = "VPNBOT_BRAND_ROUTING_VERSION=1"

# V2: shared allowlist + diagnostic action (before create/payment).
BEGIN_SHARED = "    # BEGIN VPNBOT_BRAND_SHARED"
END_SHARED = "    # END VPNBOT_BRAND_SHARED"

# V2: create/payment brand validation (before user lookup).
BEGIN_CREATE_VALIDATE = "    # BEGIN VPNBOT_BRAND_CREATE_VALIDATE"
END_CREATE_VALIDATE = "    # END VPNBOT_BRAND_CREATE_VALIDATE"

# V1+V2: apply computed override after legacy return_url assignment.
BEGIN_APPLY = "    # BEGIN VPNBOT_BRAND_RETURN_APPLY"
END_APPLY = "    # END VPNBOT_BRAND_RETURN_APPLY"

# V1 early block (hash + create validation inside create/payment).
BEGIN_V1_ROUTING = "    # BEGIN VPNBOT_BRAND_ROUTING"
END_V1_ROUTING = "    # END VPNBOT_BRAND_ROUTING"

ALL_BLOCK_PAIRS = (
    (BEGIN_SHARED, END_SHARED),
    (BEGIN_CREATE_VALIDATE, END_CREATE_VALIDATE),
    (BEGIN_V1_ROUTING, END_V1_ROUTING),
    (BEGIN_APPLY, END_APPLY),
)

# Exact semantic anchors from upstream SHM yookassa.cgi (clean upstream only).
ANCHOR_CREATE_BRANCH = (
    "if ( $vars{action} eq 'create' || $vars{action} eq 'payment' ) {"
)
ANCHOR_USER_LOOKUP = "    if ( $vars{user_id} ) {"
ANCHOR_RETURN_URL = "    my $return_url =     $ps_config{return_url};"
ANCHOR_CONFIRMATION_RETURN = (
    "                return_url => $return_url || 'https://www.example.com',"
)
ANCHOR_UNKNOWN_USER = (
    "            print_json({ status => 400, msg => 'Error: unknown user' });"
)
ANCHOR_API_KEY = "    my $api_key =        $ps_config{api_key};"
ANCHOR_ACCOUNT_ID = "    my $account_id =     $ps_config{account_id};"
ANCHOR_METADATA = "        metadata => {"
ANCHOR_YOOKASSA_API = 'HTTP::Request->new( POST => "https://api.yookassa.ru/v3/payments")'

BRAND_ID_RE = re.compile(r"^[a-z0-9][a-z0-9_-]*$")
VERSION_RE = re.compile(r"VPNBOT_BRAND_ROUTING_VERSION=([0-9]+)")


class PatchError(Exception):
    """Fatal patcher error (unknown structure, bad profiles, version conflict)."""


def _count_exact(haystack: str, needle: str) -> int:
    return haystack.count(needle)


UPSTREAM_UNIQUE = (
    ("create/payment branch", ANCHOR_CREATE_BRANCH),
    ("user_id lookup", ANCHOR_USER_LOOKUP),
    ("return_url assignment", ANCHOR_RETURN_URL),
    ("confirmation return_url", ANCHOR_CONFIRMATION_RETURN),
    ("unknown user rejection", ANCHOR_UNKNOWN_USER),
    ("api_key assignment", ANCHOR_API_KEY),
    ("account_id assignment", ANCHOR_ACCOUNT_ID),
    ("metadata section", ANCHOR_METADATA),
)


def require_unique_anchors(source: str) -> None:
    missing: List[str] = []
    duplicated: List[str] = []
    for label, anchor in UPSTREAM_UNIQUE:
        n = _count_exact(source, anchor)
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
            "refusing to patch: CGI anchors are not unique/exact ("
            + "; ".join(parts)
            + ")"
        )


def normalize_public_base_url(raw: str) -> str:
    url = (raw or "").strip()
    if not url:
        raise PatchError("brand.public_base_url is empty")
    parsed = urlparse(url)
    if parsed.scheme not in ("http", "https"):
        raise PatchError(
            f"invalid brand.public_base_url scheme (need http/https): {url!r}"
        )
    if not parsed.netloc:
        raise PatchError(f"invalid brand.public_base_url (no host): {url!r}")
    if " " in url or "\n" in url or "\t" in url:
        raise PatchError(f"invalid brand.public_base_url (whitespace): {url!r}")
    return url.rstrip("/")


def return_url_for_base(public_base_url: str) -> str:
    return f"{normalize_public_base_url(public_base_url)}/payment/return"


def load_brand_profile(path: Path) -> Tuple[str, str]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except OSError as exc:
        raise PatchError(f"cannot read brand profile {path}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise PatchError(f"invalid JSON in brand profile {path}: {exc}") from exc

    if not isinstance(data, dict):
        raise PatchError(f"brand profile {path}: root must be an object")

    brand_id = data.get("id")
    if not isinstance(brand_id, str) or not BRAND_ID_RE.match(brand_id):
        raise PatchError(
            f"brand profile {path}: missing or invalid top-level id"
        )

    brand = data.get("brand")
    if not isinstance(brand, dict):
        raise PatchError(f"brand profile {path}: missing brand object")

    public_base = brand.get("public_base_url")
    if not isinstance(public_base, str):
        raise PatchError(
            f"brand profile {path}: missing brand.public_base_url"
        )

    try:
        return_url = return_url_for_base(public_base)
    except PatchError as exc:
        raise PatchError(f"brand profile {path}: {exc}") from exc

    return brand_id, return_url


def load_brand_mapping(profiles: Sequence[Path]) -> Dict[str, str]:
    if not profiles:
        raise PatchError("at least one brand profile is required")
    mapping: Dict[str, str] = {}
    for path in profiles:
        brand_id, return_url = load_brand_profile(path)
        if brand_id in mapping:
            raise PatchError(f"duplicate brand id in profiles: {brand_id}")
        mapping[brand_id] = return_url
    return mapping


def _mapping_perl_lines(mapping: Dict[str, str]) -> List[str]:
    lines = ["    my %vpnbot_brand_return_urls = ("]
    for brand_id in sorted(mapping):
        url = mapping[brand_id]
        if "'" in url or "\\" in url:
            raise PatchError(f"return_url contains unsafe characters: {url!r}")
        lines.append(f"        '{brand_id}' => '{url}',")
    lines.append("    );")
    return lines


def build_shared_block(mapping: Dict[str, str]) -> str:
    """Shared allowlist + vpnbot_route_check (no payment side effects)."""
    lines = [
        BEGIN_SHARED,
        f"    # {MARKER}",
        "    # Managed by vpnbot deploy/shm/yookassa — do not edit by hand.",
        "    # Shared allowlist for create/payment and vpnbot_route_check.",
    ]
    lines.extend(_mapping_perl_lines(mapping))
    lines.extend(
        [
            "    if ( $vars{action} eq 'vpnbot_route_check' ) {",
            "        my $vpnbot_brand_id;",
            "        if ( exists $vars{brand_id} && defined $vars{brand_id} ) {",
            "            $vpnbot_brand_id = $vars{brand_id};",
            "            $vpnbot_brand_id =~ s/^\\s+|\\s+$//g;",
            "        }",
            "        unless ( defined $vpnbot_brand_id"
            " && length($vpnbot_brand_id)",
            "            && exists"
            " $vpnbot_brand_return_urls{$vpnbot_brand_id} ) {",
            "            print_json({ status => 400,"
            " msg => 'Error: unknown brand_id' });",
            "            exit 0;",
            "        }",
            "        print_json({",
            "            status => 200,",
            "            brand_id => $vpnbot_brand_id,",
            "            return_url =>"
            " $vpnbot_brand_return_urls{$vpnbot_brand_id},",
            "        });",
            "        exit 0;",
            "    }",
            END_SHARED,
        ]
    )
    return "\n".join(lines) + "\n"


def build_create_validate_block() -> str:
    """Validate brand_id before user lookup; compute override URL."""
    lines = [
        BEGIN_CREATE_VALIDATE,
        f"    # {MARKER}",
        "    # Must run before user lookup so unknown brand_id is not masked.",
        "    my $vpnbot_brand_return_url;",
        "    if ( exists $vars{brand_id} && defined $vars{brand_id}"
        " && length($vars{brand_id}) ) {",
        "        my $vpnbot_brand_id = $vars{brand_id};",
        "        $vpnbot_brand_id =~ s/^\\s+|\\s+$//g;",
        "        if ( length($vpnbot_brand_id) ) {",
        "            if ( exists"
        " $vpnbot_brand_return_urls{$vpnbot_brand_id} ) {",
        "                $vpnbot_brand_return_url ="
        " $vpnbot_brand_return_urls{$vpnbot_brand_id};",
        "            }",
        "            else {",
        "                print_json({ status => 400,"
        " msg => 'Error: unknown brand_id' });",
        "                exit 0;",
        "            }",
        "        }",
        "    }",
        END_CREATE_VALIDATE,
    ]
    return "\n".join(lines) + "\n"


def build_apply_block() -> str:
    lines = [
        BEGIN_APPLY,
        f"    # {MARKER}",
        "    $return_url = $vpnbot_brand_return_url"
        " if defined $vpnbot_brand_return_url;",
        END_APPLY,
    ]
    return "\n".join(lines) + "\n"


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
            "refusing to patch: incomplete VPNBOT brand-routing block markers"
        )
    if source.count(begin_m) != 1 or source.count(end_m) != 1:
        raise PatchError(
            "refusing to patch: duplicated VPNBOT brand-routing block markers"
        )
    if end < begin:
        raise PatchError("refusing to patch: END marker before BEGIN marker")
    end_line = end + len(end_m)
    if end_line < len(source) and source[end_line] == "\n":
        end_line += 1
    if end_line < len(source) and source.startswith("\n", end_line):
        end_line += 1
    return source[:begin] + source[end_line:]


def strip_routing_block(source: str) -> str:
    """Remove all known managed VPNBOT brand-routing regions."""
    result = source
    for begin_m, end_m in ALL_BLOCK_PAIRS:
        result = _strip_one_block(result, begin_m, end_m)
    if MARKER_PREFIX in result:
        raise PatchError(
            "refusing to patch: routing marker present without "
            "known BEGIN/END block boundaries"
        )
    return result


def _block_present(source: str, begin_m: str, end_m: str) -> bool:
    return begin_m in source and end_m in source


def _require_exact_block_pair(source: str, begin_m: str, end_m: str, label: str) -> None:
    bc = source.count(begin_m)
    ec = source.count(end_m)
    if bc == 0 and ec == 0:
        raise PatchError(
            f"refusing to patch: {label} managed block missing"
        )
    if bc != 1 or ec != 1:
        raise PatchError(
            f"refusing to patch: {label} managed block not unique "
            f"(begin={bc}, end={ec})"
        )
    if source.find(end_m) < source.find(begin_m):
        raise PatchError(
            f"refusing to patch: {label} END before BEGIN"
        )


def require_complete_v1(source: str) -> None:
    """Fail-closed unless a complete VERSION=1 overlay is present."""
    if BEGIN_SHARED in source or BEGIN_CREATE_VALIDATE in source:
        raise PatchError(
            "refusing to patch: VERSION=1 source has unexpected VERSION=2 blocks"
        )
    _require_exact_block_pair(
        source, BEGIN_V1_ROUTING, END_V1_ROUTING, "VERSION=1 ROUTING"
    )
    _require_exact_block_pair(
        source, BEGIN_APPLY, END_APPLY, "VERSION=1 RETURN_APPLY"
    )
    if MARKER_V1 not in source:
        raise PatchError(
            "refusing to patch: VERSION=1 blocks without VERSION=1 marker"
        )
    if "my %vpnbot_brand_return_urls" not in source:
        raise PatchError(
            "refusing to patch: VERSION=1 missing shared mapping hash"
        )
    if "my $vpnbot_brand_return_url;" not in source:
        raise PatchError(
            "refusing to patch: VERSION=1 missing brand return_url variable"
        )
    if "vpnbot_route_check" in source:
        raise PatchError(
            "refusing to patch: VERSION=1 unexpectedly contains route_check"
        )


def require_complete_v2(source: str) -> None:
    """Fail-closed unless a complete VERSION=2 overlay is present."""
    if BEGIN_V1_ROUTING in source:
        raise PatchError(
            "refusing to patch: VERSION=2 source still has VERSION=1 ROUTING block"
        )
    _require_exact_block_pair(
        source, BEGIN_SHARED, END_SHARED, "VERSION=2 SHARED"
    )
    _require_exact_block_pair(
        source, BEGIN_CREATE_VALIDATE, END_CREATE_VALIDATE,
        "VERSION=2 CREATE_VALIDATE",
    )
    _require_exact_block_pair(
        source, BEGIN_APPLY, END_APPLY, "VERSION=2 RETURN_APPLY"
    )
    if MARKER not in source:
        raise PatchError(
            "refusing to patch: VERSION=2 blocks without VERSION=2 marker"
        )
    if source.count("my %vpnbot_brand_return_urls") != 1:
        raise PatchError(
            "refusing to patch: VERSION=2 must contain exactly one allowlist"
        )
    if "vpnbot_route_check" not in source:
        raise PatchError(
            "refusing to patch: VERSION=2 missing vpnbot_route_check"
        )


def _insert_before_line(base: str, anchor_line: str, block: str, label: str) -> str:
    if _count_exact(base, anchor_line) != 1:
        raise PatchError(
            f"refusing to patch: {label} anchor not unique"
        )
    idx = base.find(anchor_line)
    return base[:idx] + block + base[idx:]


def _insert_after_line(base: str, anchor_line: str, block: str, label: str) -> str:
    needle = anchor_line + "\n"
    if _count_exact(base, needle) != 1:
        raise PatchError(
            f"refusing to patch: {label} line+newline not unique"
        )
    patched = base.replace(needle, needle + block, 1)
    if patched == base:
        raise PatchError(f"internal error: failed to insert {label} block")
    return patched


def _require_v2_order(patched: str) -> None:
    idx_shared = patched.find(BEGIN_SHARED)
    idx_route = patched.find("vpnbot_route_check")
    idx_create = patched.find(ANCHOR_CREATE_BRANCH)
    idx_validate = patched.find(BEGIN_CREATE_VALIDATE)
    idx_user = patched.find(ANCHOR_USER_LOOKUP)
    idx_unknown_user = patched.find("Error: unknown user")
    idx_return = patched.find(ANCHOR_RETURN_URL)
    idx_apply = patched.find(BEGIN_APPLY)
    idx_api = patched.find(ANCHOR_YOOKASSA_API)

    positions = (
        idx_shared, idx_route, idx_create, idx_validate, idx_user,
        idx_unknown_user, idx_return, idx_apply, idx_api,
    )
    if min(positions) < 0:
        raise PatchError("internal error: expected VERSION=2 order anchors missing")

    if not (
        idx_shared
        < idx_route
        < idx_create
        < idx_validate
        < idx_user
        < idx_unknown_user
        < idx_return
        < idx_apply
        < idx_api
    ):
        raise PatchError(
            "internal error: VERSION=2 block order invalid "
            "(route_check must precede create/payment side effects)"
        )

    # route_check body must not contain user lookup / YooKassa API.
    shared_end = patched.find(END_SHARED)
    if shared_end < 0:
        raise PatchError("internal error: SHARED end missing")
    shared_body = patched[idx_shared:shared_end]
    if ANCHOR_USER_LOOKUP in shared_body or "unknown user" in shared_body:
        raise PatchError("internal error: route_check must not look up users")
    if "api.yookassa.ru" in shared_body:
        raise PatchError("internal error: route_check must not call YooKassa API")


def build_v2_on_upstream(base: str, mapping: Dict[str, str]) -> str:
    """Apply VERSION=2 onto a clean upstream-shaped CGI (no managed blocks)."""
    require_unique_anchors(base)
    create_then_user = ANCHOR_CREATE_BRANCH + "\n" + ANCHOR_USER_LOOKUP
    if _count_exact(base, create_then_user) != 1:
        raise PatchError(
            "refusing to patch: create/payment branch does not uniquely "
            "precede user_id lookup on clean CGI"
        )

    shared = build_shared_block(mapping)
    validate = build_create_validate_block()
    apply = build_apply_block()

    patched = _insert_before_line(
        base, ANCHOR_CREATE_BRANCH, shared, "shared/route_check"
    )
    patched = _insert_after_line(
        patched, ANCHOR_CREATE_BRANCH, validate, "create validate"
    )
    patched = _insert_after_line(
        patched, ANCHOR_RETURN_URL, apply, "return_url apply"
    )

    require_unique_anchors(patched)
    _require_v2_order(patched)
    require_complete_v2(patched)

    for label, anchor in (
        ("api_key assignment", ANCHOR_API_KEY),
        ("account_id assignment", ANCHOR_ACCOUNT_ID),
        ("metadata section", ANCHOR_METADATA),
        ("confirmation return_url", ANCHOR_CONFIRMATION_RETURN),
        ("unknown user rejection", ANCHOR_UNKNOWN_USER),
        ("user_id lookup", ANCHOR_USER_LOOKUP),
        ("yookassa api call", ANCHOR_YOOKASSA_API),
    ):
        if _count_exact(patched, anchor) != 1:
            raise PatchError(f"post-check failed for {label}")

    if patched.count("my %vpnbot_brand_return_urls") != 1:
        raise PatchError("internal error: allowlist not unique")

    return patched


def recover_base_from_managed(source: str, version: int) -> str:
    """Strip managed blocks from a validated overlay; do not match V1 as upstream."""
    if version == 1:
        require_complete_v1(source)
    elif version == 2:
        require_complete_v2(source)
    else:
        raise PatchError(f"internal error: recover_base unsupported version {version}")
    base = strip_routing_block(source)
    if MARKER_PREFIX in base:
        raise PatchError(
            "refusing to patch: managed markers remain after strip"
        )
    return base


def apply_patch(source: str, mapping: Dict[str, str]) -> str:
    version = detect_marker_version(source)

    if version is not None and version not in (1, 2):
        raise PatchError(
            f"refusing to patch: unsupported routing marker version {version} "
            f"(supported: 1→{TARGET_VERSION}, {TARGET_VERSION})"
        )

    if version is None:
        # Clean upstream only. Any stray managed markers are fail-closed.
        if any(
            m in source
            for m in (
                BEGIN_SHARED, BEGIN_CREATE_VALIDATE, BEGIN_V1_ROUTING,
                BEGIN_APPLY, MARKER_PREFIX,
            )
        ):
            raise PatchError(
                "refusing to patch: unmanaged/partial VPNBOT markers on "
                "source without a single known version"
            )
        return build_v2_on_upstream(source, mapping)

    # VERSION=1 or VERSION=2: validate managed structure, strip, rebuild V2.
    # Do not require upstream adjacency anchors on the pre-strip overlay.
    base = recover_base_from_managed(source, version)
    return build_v2_on_upstream(base, mapping)


def patch_file(
    source_path: Path,
    profiles: Sequence[Path],
    output_path: Path,
    *,
    force: bool = False,
) -> str:
    try:
        source = source_path.read_text(encoding="utf-8")
    except OSError as exc:
        raise PatchError(f"cannot read source CGI {source_path}: {exc}") from exc

    mapping = load_brand_mapping(profiles)
    patched = apply_patch(source, mapping)

    version = detect_marker_version(source)
    if patched == source:
        msg = f"already applied: {MARKER} (identical)"
        if not force:
            output_path.parent.mkdir(parents=True, exist_ok=True)
            output_path.write_text(patched, encoding="utf-8")
        return msg

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(patched, encoding="utf-8")
    if version == 1:
        return f"upgraded: VERSION=1 → {MARKER}"
    if version == 2:
        return f"updated: regenerated {MARKER}"
    return f"patched: inserted {MARKER}"


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Insert vpnbot brand_id → return_url routing into SHM yookassa.cgi"
        )
    )
    parser.add_argument(
        "--source",
        required=True,
        type=Path,
        help="Path to upstream (or previously patched) yookassa.cgi",
    )
    parser.add_argument(
        "--brand-profile",
        action="append",
        dest="brand_profiles",
        type=Path,
        required=True,
        help="Brand profile JSON (repeatable); uses id + brand.public_base_url",
    )
    parser.add_argument(
        "--output",
        required=True,
        type=Path,
        help="Output path for patched CGI",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Overwrite output even when result is identical to source",
    )
    args = parser.parse_args(argv)

    try:
        status = patch_file(
            args.source,
            args.brand_profiles,
            args.output,
            force=args.force,
        )
    except PatchError as exc:
        print(f"patch_yookassa: {exc}", file=sys.stderr)
        return 1

    print(f"patch_yookassa: {status}")
    print(f"patch_yookassa: wrote {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
