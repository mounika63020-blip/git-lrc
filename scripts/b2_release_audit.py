#!/usr/bin/env python3
"""Audit Backblaze B2 file versions for release artifacts under a prefix.

Environment variables required:
- B2_KEY_ID
- B2_APP_KEY
- B2_BUCKET_ID

Example:
  /bin/python scripts/b2_release_audit.py --prefix lrc/ --output security_issues/b2-release-audit.json
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from typing import Dict, List, Optional

import requests

B2_API_BASE = "https://api.backblazeb2.com"


def require_env(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        raise RuntimeError(f"missing required environment variable: {name}")
    return value


def authorize(key_id: str, app_key: str) -> Dict:
    url = f"{B2_API_BASE}/b2api/v2/b2_authorize_account"
    resp = requests.get(url, auth=(key_id, app_key), timeout=30)
    if resp.status_code != 200:
        raise RuntimeError(f"B2 authorize failed ({resp.status_code}): {resp.text}")
    return resp.json()


def list_file_versions(
    api_url: str,
    token: str,
    bucket_id: str,
    prefix: str,
    start_file_name: Optional[str] = None,
    start_file_id: Optional[str] = None,
    max_file_count: int = 1000,
) -> Dict:
    url = f"{api_url}/b2api/v2/b2_list_file_versions"
    body = {
        "bucketId": bucket_id,
        "prefix": prefix,
        "maxFileCount": max_file_count,
    }
    if start_file_name:
        body["startFileName"] = start_file_name
    if start_file_id:
        body["startFileId"] = start_file_id

    resp = requests.post(
        url,
        headers={"Authorization": token},
        json=body,
        timeout=60,
    )
    if resp.status_code != 200:
        raise RuntimeError(f"b2_list_file_versions failed ({resp.status_code}): {resp.text}")
    return resp.json()


def collect_versions(api_url: str, token: str, bucket_id: str, prefix: str) -> List[Dict]:
    versions: List[Dict] = []
    next_file_name: Optional[str] = None
    next_file_id: Optional[str] = None

    while True:
        page = list_file_versions(
            api_url=api_url,
            token=token,
            bucket_id=bucket_id,
            prefix=prefix,
            start_file_name=next_file_name,
            start_file_id=next_file_id,
            max_file_count=1000,
        )
        versions.extend(page.get("files", []))

        next_file_name = page.get("nextFileName")
        next_file_id = page.get("nextFileId")
        if not next_file_name:
            break

    return versions


def summarize(versions: List[Dict]) -> Dict:
    by_action: Dict[str, int] = {}
    by_file_name: Dict[str, int] = {}

    for row in versions:
        action = row.get("action", "unknown")
        by_action[action] = by_action.get(action, 0) + 1

        file_name = row.get("fileName", "")
        if file_name:
            by_file_name[file_name] = by_file_name.get(file_name, 0) + 1

    multi_version_files = [name for name, count in by_file_name.items() if count > 1]

    return {
        "total_versions": len(versions),
        "actions": by_action,
        "unique_file_names": len(by_file_name),
        "multi_version_file_names": len(multi_version_files),
        "multi_version_samples": sorted(multi_version_files)[:20],
    }


def to_iso(ts_millis: Optional[int]) -> Optional[str]:
    if not ts_millis:
        return None
    return datetime.fromtimestamp(ts_millis / 1000, tz=timezone.utc).isoformat()


def normalize_rows(versions: List[Dict]) -> List[Dict]:
    rows: List[Dict] = []
    for v in versions:
        rows.append(
            {
                "fileName": v.get("fileName"),
                "fileId": v.get("fileId"),
                "action": v.get("action"),
                "uploadTimestamp": v.get("uploadTimestamp"),
                "uploadTime": to_iso(v.get("uploadTimestamp")),
                "contentLength": v.get("contentLength"),
                "contentSha1": v.get("contentSha1"),
            }
        )
    return rows


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Audit B2 versions for release artifacts")
    parser.add_argument("--prefix", default="lrc/", help="Prefix to inspect (default: lrc/)")
    parser.add_argument(
        "--output",
        default="security_issues/b2-release-audit.json",
        help="Output JSON path",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        key_id = require_env("B2_KEY_ID")
        app_key = require_env("B2_APP_KEY")
        bucket_id = require_env("B2_BUCKET_ID")

        auth = authorize(key_id, app_key)
        api_url = auth["apiUrl"]
        token = auth["authorizationToken"]

        versions = collect_versions(api_url, token, bucket_id, args.prefix)
        summary = summarize(versions)

        report = {
            "generated_at": datetime.now(timezone.utc).isoformat(),
            "prefix": args.prefix,
            "bucket_id": bucket_id,
            "summary": summary,
            "versions": normalize_rows(versions),
        }

        os.makedirs(os.path.dirname(args.output), exist_ok=True)
        with open(args.output, "w", encoding="utf-8") as f:
            json.dump(report, f, indent=2)
            f.write("\n")

        print(f"Wrote {args.output}")
        print(json.dumps(summary, indent=2))
        return 0
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
