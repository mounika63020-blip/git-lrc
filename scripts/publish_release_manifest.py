#!/usr/bin/env python3
"""Publish lrc/latest.json manifest from existing B2 release objects.

This is intended for backfilling manifest support for already-uploaded releases.

Environment variables required:
- B2_KEY_ID
- B2_APP_KEY
- B2_BUCKET_NAME
- B2_BUCKET_ID
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import sys
from datetime import datetime, timezone
from typing import Dict, List, Optional, Tuple
from urllib.parse import quote

import requests

B2_API_BASE = "https://api.backblazeb2.com"
PREFIX = "lrc"
MANIFEST_NAME = "latest.json"
PLATFORM_RE = re.compile(r"^lrc/(v\d+\.\d+\.\d+)/(linux-amd64|linux-arm64|darwin-amd64|darwin-arm64|windows-amd64)/(lrc|lrc\.exe)$")


def require_env(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        raise RuntimeError(f"missing required environment variable: {name}")
    return value


def parse_semver(v: str) -> Tuple[int, int, int]:
    m = re.match(r"^v(\d+)\.(\d+)\.(\d+)$", v)
    if not m:
        raise ValueError(f"invalid semantic version: {v}")
    return int(m.group(1)), int(m.group(2)), int(m.group(3))


def authorize(key_id: str, app_key: str) -> Dict:
    url = f"{B2_API_BASE}/b2api/v2/b2_authorize_account"
    resp = requests.get(url, auth=(key_id, app_key), timeout=30)
    if resp.status_code != 200:
        raise RuntimeError(f"B2 authorize failed ({resp.status_code}): {resp.text}")
    return resp.json()


def list_file_names(api_url: str, token: str, bucket_id: str, prefix: str) -> List[Dict]:
    files: List[Dict] = []
    start_file_name: Optional[str] = f"{prefix}/"

    while True:
        url = f"{api_url}/b2api/v2/b2_list_file_names"
        body = {
            "bucketId": bucket_id,
            "startFileName": start_file_name,
            "prefix": f"{prefix}/",
            "maxFileCount": 1000,
        }
        resp = requests.post(url, headers={"Authorization": token}, json=body, timeout=60)
        if resp.status_code != 200:
            raise RuntimeError(f"b2_list_file_names failed ({resp.status_code}): {resp.text}")

        data = resp.json()
        files.extend(data.get("files", []))

        next_file_name = data.get("nextFileName")
        if not next_file_name:
            break
        start_file_name = next_file_name

    return files


def get_upload_url(api_url: str, token: str, bucket_id: str) -> Dict:
    url = f"{api_url}/b2api/v2/b2_get_upload_url"
    resp = requests.post(
        url,
        headers={"Authorization": token},
        json={"bucketId": bucket_id},
        timeout=30,
    )
    if resp.status_code != 200:
        raise RuntimeError(f"b2_get_upload_url failed ({resp.status_code}): {resp.text}")
    return resp.json()


def upload_manifest(upload_data: Dict, manifest_bytes: bytes, b2_file_name: str) -> None:
    sha1 = hashlib.sha1(manifest_bytes).hexdigest()  # nosec B324 - required by B2 API
    headers = {
        "Authorization": upload_data["authorizationToken"],
        "X-Bz-File-Name": quote(b2_file_name, safe="/"),
        "Content-Type": "application/json",
        "X-Bz-Content-Sha1": sha1,
    }
    resp = requests.post(upload_data["uploadUrl"], headers=headers, data=manifest_bytes, timeout=30)
    if resp.status_code != 200:
        raise RuntimeError(f"manifest upload failed ({resp.status_code}): {resp.text}")


def build_manifest(bucket_name: str, file_rows: List[Dict]) -> Dict:
    releases: Dict[str, Dict] = {}

    for row in file_rows:
        file_name = row.get("fileName", "")
        match = PLATFORM_RE.match(file_name)
        if not match:
            continue

        version = match.group(1)
        platform = match.group(2)
        binary_name = match.group(3)

        releases.setdefault(version, {"platforms": {}})
        releases[version]["platforms"][platform] = {
            "binary": file_name,
            "sha256sums": f"{PREFIX}/{version}/{platform}/SHA256SUMS",
            "sha256": "",
            "binary_name": binary_name,
        }

    if not releases:
        raise RuntimeError("no release binaries found under lrc/ to build manifest")

    latest_version = sorted(releases.keys(), key=parse_semver, reverse=True)[0]
    download_base = f"https://f005.backblazeb2.com/file/{bucket_name}/{PREFIX}"

    return {
        "schema_version": 1,
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "latest_version": latest_version,
        "bucket": bucket_name,
        "prefix": PREFIX,
        "download_base": download_base,
        "releases": releases,
    }


def main() -> int:
    try:
        key_id = require_env("B2_KEY_ID")
        app_key = require_env("B2_APP_KEY")
        bucket_name = require_env("B2_BUCKET_NAME")
        bucket_id = require_env("B2_BUCKET_ID")

        auth = authorize(key_id, app_key)
        api_url = auth["apiUrl"]
        token = auth["authorizationToken"]

        rows = list_file_names(api_url, token, bucket_id, PREFIX)
        manifest = build_manifest(bucket_name, rows)
        manifest_bytes = (json.dumps(manifest, indent=2) + "\n").encode("utf-8")

        upload_data = get_upload_url(api_url, token, bucket_id)
        upload_manifest(upload_data, manifest_bytes, f"{PREFIX}/{MANIFEST_NAME}")

        print(f"Published manifest: {PREFIX}/{MANIFEST_NAME}")
        print(f"Latest version: {manifest['latest_version']}")
        return 0
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
