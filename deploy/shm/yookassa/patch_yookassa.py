#!/usr/bin/env python3
"""Deterministic patcher for host-mounted SHM yookassa.cgi brand return_url routing.

Reads brand id + brand.public_base_url from vpnbot brand profiles and inserts a
versioned, fail-closed brand_id → return_url map into upstream yookassa.cgi.

Brand validation runs before user lookup inside the create/payment branch so
unknown brand_id is not masked by the unknown-user path (user_id=-1 probes).

Does not embed YooKassa credentials. Refuses unknown CGI structure.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Dict, List, Sequence, Tuple
from urllib.parse import urlparse

MARKER = "VPNBOT_BRAND_ROUTING_VERSION=1"
MARKER_PREFIX = "VPNBOT_BRAND_ROUTING_VERSION="

# Early block: validate brand_id before user lookup; compute override URL.
BEGIN_MARKER = "    # BEGIN VPNBOT_BRAND_ROUTING"
END_MARKER = "    # END VPNBOT_BRAND_ROUTING"

# Late block: apply computed override after legacy return_url assignment.
BEGIN_APPLY_MARKER = "    # BEGIN VPNBOT_BRAND_RETURN_APPLY"
END_APPLY_MARKER = "    # END VPNBOT_BRAND_RETURN_APPLY"

BLOCK_PAIRS = (
    (BEGIN_MARKER, END_MARKER),
    (BEGIN_APPLY_MARKER, END_APPLY_MARKER),
)

# Exact semantic anchors from upstream SHM yookassa.cgi (must each appear once).
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

BRAND_ID_RE = re.compile(r"^[a-z0-9][a-z0-9_-]*$")
VERSION_RE = re.compile(
    r"VPNBOT_BRAND_ROUTING_VERSION=([0-9]+)",
)


class PatchError(Exception):
    """Fatal patcher error (unknown structure, bad profiles, version conflict)."""


def _count_exact(haystack: str, needle: str) -> int:
    return haystack.count(needle)


require_unique = (
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
    for label, anchor in require_unique:
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
    if url != url.strip():
        raise PatchError("brand.public_base_url has unexpected surrounding whitespace")
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


def build_validation_block(mapping: Dict[str, str]) -> str:
    """Validate brand_id and compute override URL before user lookup."""
    lines = [
        BEGIN_MARKER,
        f"    # {MARKER}",
        "    # Managed by vpnbot deploy/shm/yookassa — do not edit by hand.",
        "    # Must run before user lookup so unknown brand_id is not masked.",
        "    my %vpnbot_brand_return_urls = (",
    ]
    for brand_id in sorted(mapping):
        url = mapping[brand_id]
        if "'" in url or "\\" in url:
            raise PatchError(f"return_url contains unsafe characters: {url!r}")
        lines.append(f"        '{brand_id}' => '{url}',")
    lines.extend(
        [
            "    );",
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
            END_MARKER,
        ]
    )
    return "\n".join(lines) + "\n"


def build_apply_block() -> str:
    """Apply computed brand return_url after legacy ps_config assignment."""
    lines = [
        BEGIN_APPLY_MARKER,
        f"    # {MARKER}",
        "    $return_url = $vpnbot_brand_return_url"
        " if defined $vpnbot_brand_return_url;",
        END_APPLY_MARKER,
    ]
    return "\n".join(lines) + "\n"


def detect_marker_version(source: str) -> int | None:
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
        raise PatchError(
            "refusing to patch: END marker before BEGIN marker"
        )
    end_line = end + len(end_m)
    if end_line < len(source) and source[end_line] == "\n":
        end_line += 1
    # Older builds accidentally left one blank line after END; drop it so
    # re-application is idempotent against those files too.
    if end_line < len(source) and source.startswith("\n", end_line):
        end_line += 1
    return source[:begin] + source[end_line:]


def strip_routing_block(source: str) -> str:
    """Remove all managed VPNBOT brand-routing regions (early + apply)."""
    result = source
    for begin_m, end_m in BLOCK_PAIRS:
        result = _strip_one_block(result, begin_m, end_m)
    if MARKER_PREFIX in result:
        raise PatchError(
            "refusing to patch: routing marker present without "
            "known BEGIN/END block boundaries"
        )
    return result


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


def _require_order(patched: str) -> None:
    idx_create = patched.find(ANCHOR_CREATE_BRANCH)
    idx_brand = patched.find("Error: unknown brand_id")
    idx_user_lookup = patched.find(ANCHOR_USER_LOOKUP)
    idx_unknown_user = patched.find("Error: unknown user")
    idx_return = patched.find(ANCHOR_RETURN_URL)
    idx_apply = patched.find(
        "$return_url = $vpnbot_brand_return_url if defined $vpnbot_brand_return_url;"
    )
    if min(idx_create, idx_brand, idx_user_lookup, idx_unknown_user, idx_return, idx_apply) < 0:
        raise PatchError("internal error: expected order anchors missing after patch")
    if not (
        idx_create
        < idx_brand
        < idx_user_lookup
        < idx_unknown_user
        < idx_return
        < idx_apply
    ):
        raise PatchError(
            "internal error: brand validation must run before user lookup "
            "and return_url apply must follow legacy return_url assignment"
        )


def apply_patch(source: str, mapping: Dict[str, str]) -> str:
    version = detect_marker_version(source)
    if version is not None and version != 1:
        raise PatchError(
            f"refusing to patch: unsupported routing marker version {version} "
            f"(need 1)"
        )

    base = strip_routing_block(source) if version == 1 else source
    # Also strip when markers absent but somehow version detected — handled above.
    if version is None and (
        BEGIN_MARKER in source
        or BEGIN_APPLY_MARKER in source
        or MARKER_PREFIX in source
    ):
        base = strip_routing_block(source)

    require_unique_anchors(base)

    # Create branch must immediately precede user lookup in upstream shape.
    create_then_user = ANCHOR_CREATE_BRANCH + "\n" + ANCHOR_USER_LOOKUP
    if _count_exact(base, create_then_user) != 1:
        raise PatchError(
            "refusing to patch: create/payment branch does not uniquely "
            "precede user_id lookup"
        )

    validation = build_validation_block(mapping)
    apply = build_apply_block()

    patched = _insert_after_line(
        base, ANCHOR_CREATE_BRANCH, validation, "brand validation"
    )
    patched = _insert_after_line(
        patched, ANCHOR_RETURN_URL, apply, "return_url apply"
    )

    require_unique_anchors(patched)
    _require_order(patched)

    if MARKER not in patched:
        raise PatchError("internal error: marker missing after patch")
    if "Error: unknown brand_id" not in patched:
        raise PatchError("internal error: unknown brand_id path missing")
    if "my $vpnbot_brand_return_url;" not in patched:
        raise PatchError("internal error: brand return_url variable missing")

    for label, anchor in (
        ("api_key assignment", ANCHOR_API_KEY),
        ("account_id assignment", ANCHOR_ACCOUNT_ID),
        ("metadata section", ANCHOR_METADATA),
        ("confirmation return_url", ANCHOR_CONFIRMATION_RETURN),
        ("unknown user rejection", ANCHOR_UNKNOWN_USER),
        ("user_id lookup", ANCHOR_USER_LOOKUP),
    ):
        if _count_exact(patched, anchor) != 1:
            raise PatchError(f"post-check failed for {label}")

    return patched


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

    if patched == source:
        msg = "already applied: VPNBOT_BRAND_ROUTING_VERSION=1 (identical)"
        if not force:
            output_path.parent.mkdir(parents=True, exist_ok=True)
            output_path.write_text(patched, encoding="utf-8")
        return msg

    version = detect_marker_version(source)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(patched, encoding="utf-8")
    if version == 1:
        return "updated: regenerated VPNBOT_BRAND_ROUTING_VERSION=1"
    return "patched: inserted VPNBOT_BRAND_ROUTING_VERSION=1"


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
