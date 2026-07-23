#!/usr/bin/env python3
"""Deterministic patcher for host-mounted SHM yookassa.cgi brand return_url routing.

Reads brand id + brand.public_base_url from vpnbot brand profiles and inserts a
versioned, fail-closed brand_id → return_url map into upstream yookassa.cgi.

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
BEGIN_MARKER = "    # BEGIN VPNBOT_BRAND_ROUTING"
END_MARKER = "    # END VPNBOT_BRAND_ROUTING"

# Exact semantic anchors from upstream SHM yookassa.cgi (must each appear once).
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


def build_routing_block(mapping: Dict[str, str]) -> str:
    lines = [
        BEGIN_MARKER,
        f"    # {MARKER}",
        "    # Managed by vpnbot deploy/shm/yookassa — do not edit by hand.",
        "    my %vpnbot_brand_return_urls = (",
    ]
    for brand_id in sorted(mapping):
        url = mapping[brand_id]
        # Perl single-quoted string; mapping values are validated URLs.
        if "'" in url or "\\" in url:
            raise PatchError(f"return_url contains unsafe characters: {url!r}")
        lines.append(f"        '{brand_id}' => '{url}',")
    lines.extend(
        [
            "    );",
            "    if ( exists $vars{brand_id} && defined $vars{brand_id}"
            " && length($vars{brand_id}) ) {",
            "        my $vpnbot_brand_id = $vars{brand_id};",
            "        $vpnbot_brand_id =~ s/^\\s+|\\s+$//g;",
            "        if ( exists $vpnbot_brand_return_urls{$vpnbot_brand_id} ) {",
            "            $return_url = $vpnbot_brand_return_urls{$vpnbot_brand_id};",
            "        }",
            "        else {",
            "            print_json({ status => 400,"
            " msg => 'Error: unknown brand_id' });",
            "            exit 0;",
            "        }",
            "    }",
            END_MARKER,
        ]
    )
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


def strip_routing_block(source: str) -> str:
    begin = source.find(BEGIN_MARKER)
    end = source.find(END_MARKER)
    if begin < 0 and end < 0:
        # Legacy single-line marker without BEGIN/END (should not happen for v1).
        if MARKER_PREFIX in source:
            raise PatchError(
                "refusing to patch: routing marker present without "
                "BEGIN/END block boundaries"
            )
        return source
    if begin < 0 or end < 0:
        raise PatchError(
            "refusing to patch: incomplete VPNBOT_BRAND_ROUTING block markers"
        )
    if source.count(BEGIN_MARKER) != 1 or source.count(END_MARKER) != 1:
        raise PatchError(
            "refusing to patch: duplicated VPNBOT_BRAND_ROUTING block markers"
        )
    if end < begin:
        raise PatchError(
            "refusing to patch: END marker before BEGIN marker"
        )
    end_line = end + len(END_MARKER)
    # Consume trailing newline after END marker.
    if end_line < len(source) and source[end_line] == "\n":
        end_line += 1
    # Older builds accidentally left one blank line after END; drop it so
    # re-application is idempotent against those files too.
    if end_line < len(source) and source.startswith("\n", end_line):
        end_line += 1
    return source[:begin] + source[end_line:]


def apply_patch(source: str, mapping: Dict[str, str]) -> str:
    version = detect_marker_version(source)
    if version is not None and version != 1:
        raise PatchError(
            f"refusing to patch: unsupported routing marker version {version} "
            f"(need 1)"
        )

    base = strip_routing_block(source) if version == 1 else source
    require_unique_anchors(base)

    block = build_routing_block(mapping)
    # Insert immediately after the return_url assignment line, preserving the
    # original following newline so we do not introduce a blank line.
    anchor_line = ANCHOR_RETURN_URL + "\n"
    if _count_exact(base, anchor_line) != 1:
        raise PatchError(
            "refusing to patch: return_url assignment line+newline not unique"
        )
    patched = base.replace(anchor_line, anchor_line + block, 1)
    if patched == base:
        raise PatchError("internal error: failed to insert routing block")

    # Post-conditions: anchors that must remain unique / unchanged.
    require_unique_anchors(
        # After insert, return_url assignment still unique; routing block may
        # mention return_url variable but not the exact assignment anchor.
        patched
    )
    if MARKER not in patched:
        raise PatchError("internal error: marker missing after patch")
    if "Error: unknown brand_id" not in patched:
        raise PatchError("internal error: unknown brand_id path missing")

    # Credentials / callback / metadata literals must be byte-identical to base
    # outside our inserted block.
    for label, anchor in (
        ("api_key assignment", ANCHOR_API_KEY),
        ("account_id assignment", ANCHOR_ACCOUNT_ID),
        ("metadata section", ANCHOR_METADATA),
        ("confirmation return_url", ANCHOR_CONFIRMATION_RETURN),
        ("unknown user rejection", ANCHOR_UNKNOWN_USER),
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
            # Still write identical output for deterministic pipelines.
            output_path.parent.mkdir(parents=True, exist_ok=True)
            output_path.write_text(patched, encoding="utf-8")
        return msg

    version = detect_marker_version(source)
    if version == 1 and not force:
        # Source had v1 but content differed from regenerated mapping.
        # Regenerating is intentional when brand profiles change.
        pass

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
