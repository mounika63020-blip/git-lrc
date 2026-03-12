#!/usr/bin/env python3
"""Delete unnecessary Backblaze B2 file versions under a prefix.

By default this script is dry-run and only prints planned deletions.
Use --apply to execute b2_delete_file_version calls.

Environment variables required:
- B2_KEY_ID
- B2_APP_KEY
- B2_BUCKET_ID

Example dry-run:
  /bin/python scripts/b2_release_cleanup.py --prefix lrc/

Example apply:
  /bin/python scripts/b2_release_cleanup.py --prefix lrc/ --apply
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from collections import defaultdict
from datetime import datetime, timezone
from typing import DefaultDict, Dict, List, Optional

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


def plan_deletions(versions: List[Dict]) -> List[Dict]:
    grouped: DefaultDict[str, List[Dict]] = defaultdict(list)
    for v in versions:
        file_name = v.get("fileName", "")
        if not file_name:
            continue
        grouped[file_name].append(v)

    deletions: List[Dict] = []
    for file_name, rows in grouped.items():
        sorted_rows = sorted(rows, key=lambda x: int(x.get("uploadTimestamp", 0)), reverse=True)

        keep_one_upload = False
        for row in sorted_rows:
            action = row.get("action")
            if action == "upload" and not keep_one_upload:
                keep_one_upload = True
                continue

            if action in {"hide", "start"} or (action == "upload" and keep_one_upload):
                deletions.append(row)

    return deletions


def delete_version(api_url: str, token: str, file_name: str, file_id: str) -> None:
    url = f"{api_url}/b2api/v2/b2_delete_file_version"
    resp = requests.post(
        url,
        headers={"Authorization": token},
        json={"fileName": file_name, "fileId": file_id},
        timeout=30,
    )
    if resp.status_code != 200:
        raise RuntimeError(
            f"b2_delete_file_version failed for {file_name} ({file_id}) "
            f"status={resp.status_code}: {resp.text}"
        )


def to_iso(ts_millis: Optional[int]) -> Optional[str]:
    if not ts_millis:
        return None
    return datetime.fromtimestamp(ts_millis / 1000, tz=timezone.utc).isoformat()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Prune unnecessary B2 file versions under a prefix")
    parser.add_argument("--prefix", default="lrc/", help="Prefix to prune (default: lrc/)")
    parser.add_argument(
        "--output",
        default="security_issues/b2-release-cleanup-plan.json",
        help="Path to write deletion plan/report",
    )
    parser.add_argument(
        "--apply",
        action="store_true",
        help="Execute deletions (default is dry-run)",
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
        planned = plan_deletions(versions)

        report = {
            "generated_at": datetime.now(timezone.utc).isoformat(),
            "prefix": args.prefix,
            "bucket_id": bucket_id,
            "mode": "apply" if args.apply else "dry-run",
            "total_versions_seen": len(versions),
            "planned_deletions": [
                {
                    "fileName": v.get("fileName"),
                    "fileId": v.get("fileId"),
                    "action": v.get("action"),
                    "uploadTimestamp": v.get("uploadTimestamp"),
                    "uploadTime": to_iso(v.get("uploadTimestamp")),
                }
                for v in planned
            ],
        }

        os.makedirs(os.path.dirname(args.output), exist_ok=True)
        with open(args.output, "w", encoding="utf-8") as f:
            json.dump(report, f, indent=2)
            f.write("\n")

        print(f"Wrote {args.output}")
        print(f"Planned deletions: {len(planned)}")

        if not args.apply:
            print("Dry-run only. Re-run with --apply to delete planned versions.")
            return 0

        for item in planned:
            file_name = item.get("fileName")
            file_id = item.get("fileId")
            if not file_name or not file_id:
                continue
            delete_version(api_url, token, file_name, file_id)

        print(f"Deleted versions: {len(planned)}")
        return 0
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
