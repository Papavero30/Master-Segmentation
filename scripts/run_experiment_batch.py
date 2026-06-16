#!/usr/bin/env python3
"""
Run segmentation experiments for multiple slices and export results to CSV.

Example:
  python run_experiment_batch.py --mode homogen --scan-sid scan_xxx --slices 0-99 --access-token <JWT>
"""

from __future__ import annotations

import argparse
import csv
import os
import sys
import time
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Dict, List

import requests
import urllib3


@dataclass
class TaskResult:
    task_id: str
    slice_index: int
    cluster_mode: str
    status: str
    progress: int
    started_at: str
    finished_at: str
    duration_ms: int
    error: str


def parse_slices(spec: str) -> List[int]:
    values = set()
    for segment in spec.split(","):
        segment = segment.strip()
        if not segment:
            continue

        if "-" in segment:
            start_text, end_text = segment.split("-", 1)
            start = int(start_text.strip())
            end = int(end_text.strip())
            if end < start:
                raise ValueError(f"Invalid range: {segment}")
            for num in range(start, end + 1):
                values.add(num)
        else:
            values.add(int(segment))

    ordered = sorted(values)
    if not ordered:
        raise ValueError("No slices parsed from --slices")

    if ordered[0] < 0:
        raise ValueError("Slice indices must be non-negative")

    return ordered


def build_session(base_url: str, token: str, verify_tls: bool) -> requests.Session:
    session = requests.Session()
    session.headers.update(
        {
            "Content-Type": "application/json",
            "Accept": "application/json",
            "Authorization": f"Bearer {token}",
        }
    )
    session.verify = verify_tls
    session.base_url = base_url.rstrip("/")  # type: ignore[attr-defined]
    return session


def submit_segmentation(session: requests.Session, scan_sid: str, slice_index: int, mode: str) -> str:
    url = f"{session.base_url}/segmentation/segment"  # type: ignore[attr-defined]
    payload = {
        "scan_sid": scan_sid,
        "slice_index": slice_index,
        "cluster_mode": mode,
    }

    resp = session.post(url, json=payload, timeout=30)
    if resp.status_code not in (200, 202):
        raise RuntimeError(f"Submit failed [{resp.status_code}] for slice {slice_index}: {resp.text}")

    data = resp.json()
    task_id = data.get("task_id")
    if not task_id:
        raise RuntimeError(f"No task_id in response for slice {slice_index}: {data}")

    return str(task_id)


def poll_task(
    session: requests.Session,
    task_id: str,
    poll_interval_sec: float,
    timeout_sec: int,
) -> Dict:
    url = f"{session.base_url}/segmentation/status/{task_id}"  # type: ignore[attr-defined]
    start = time.time()

    while True:
        if time.time() - start > timeout_sec:
            raise TimeoutError(f"Polling timed out for task {task_id}")

        resp = session.get(url, timeout=30)
        if resp.status_code != 200:
            raise RuntimeError(f"Status failed [{resp.status_code}] for task {task_id}: {resp.text}")

        data = resp.json()
        status = str(data.get("status", "")).lower()

        if status in ("completed", "failed"):
            return data

        time.sleep(poll_interval_sec)


def write_csv(path: Path, rows: List[TaskResult]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)

    with path.open("w", newline="", encoding="utf-8") as csvfile:
        writer = csv.DictWriter(
            csvfile,
            fieldnames=[
                "task_id",
                "slice_index",
                "cluster_mode",
                "status",
                "progress",
                "started_at",
                "finished_at",
                "duration_ms",
                "error",
            ],
        )
        writer.writeheader()
        for row in rows:
            writer.writerow(
                {
                    "task_id": row.task_id,
                    "slice_index": row.slice_index,
                    "cluster_mode": row.cluster_mode,
                    "status": row.status,
                    "progress": row.progress,
                    "started_at": row.started_at,
                    "finished_at": row.finished_at,
                    "duration_ms": row.duration_ms,
                    "error": row.error,
                }
            )


def main() -> int:
    parser = argparse.ArgumentParser(description="Run segmentation batch experiment")
    parser.add_argument("--base-url", default=os.getenv("BRAINNAV_API_BASE", "https://localhost:8444/api"))
    parser.add_argument("--access-token", default=os.getenv("BRAINNAV_ACCESS_TOKEN"), help="JWT access token")
    parser.add_argument("--scan-sid", required=True)
    parser.add_argument("--mode", required=True, choices=["homogen", "heterogen"])
    parser.add_argument("--slices", required=True, help="Slice spec, e.g. 0-99 or 0,2,4-10")
    parser.add_argument("--poll-interval", type=float, default=2.0)
    parser.add_argument("--timeout-per-task", type=int, default=900)
    parser.add_argument("--verify-tls", action="store_true", help="Enable TLS certificate verification")
    parser.add_argument("--output-csv", default="", help="Output CSV file path")

    args = parser.parse_args()

    if not args.access_token:
        parser.error("--access-token is required (or set BRAINNAV_ACCESS_TOKEN)")

    slices = parse_slices(args.slices)

    if not args.verify_tls:
        urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

    session = build_session(args.base_url, args.access_token, args.verify_tls)

    timestamp = datetime.utcnow().strftime("%Y%m%d_%H%M%S")
    output_csv = Path(args.output_csv) if args.output_csv else Path(f"experiment_batch_{args.mode}_{timestamp}.csv")

    print("Batch configuration")
    print(f"- base_url   : {args.base_url}")
    print(f"- scan_sid   : {args.scan_sid}")
    print(f"- mode       : {args.mode}")
    print(f"- slices     : {len(slices)} items ({slices[0]}..{slices[-1]})")
    print(f"- output_csv : {output_csv}")

    rows: List[TaskResult] = []
    success = 0
    failed = 0

    for index, slice_idx in enumerate(slices, start=1):
        started = datetime.utcnow()
        started_ts = started.isoformat() + "Z"

        print(f"\n[{index}/{len(slices)}] Submit slice {slice_idx}")
        try:
            task_id = submit_segmentation(session, args.scan_sid, slice_idx, args.mode)
            print(f"  task_id={task_id}")

            status_payload = poll_task(
                session,
                task_id,
                poll_interval_sec=args.poll_interval,
                timeout_sec=args.timeout_per_task,
            )

            finished = datetime.utcnow()
            duration_ms = int((finished - started).total_seconds() * 1000)
            final_status = str(status_payload.get("status", "unknown"))
            progress = int(status_payload.get("progress", 0))

            if final_status.lower() == "completed":
                success += 1
            else:
                failed += 1

            rows.append(
                TaskResult(
                    task_id=task_id,
                    slice_index=slice_idx,
                    cluster_mode=args.mode,
                    status=final_status,
                    progress=progress,
                    started_at=started_ts,
                    finished_at=finished.isoformat() + "Z",
                    duration_ms=duration_ms,
                    error="",
                )
            )
            print(f"  status={final_status}, progress={progress}, duration_ms={duration_ms}")

        except Exception as err:  # pylint: disable=broad-except
            finished = datetime.utcnow()
            duration_ms = int((finished - started).total_seconds() * 1000)
            failed += 1
            rows.append(
                TaskResult(
                    task_id="",
                    slice_index=slice_idx,
                    cluster_mode=args.mode,
                    status="failed",
                    progress=0,
                    started_at=started_ts,
                    finished_at=finished.isoformat() + "Z",
                    duration_ms=duration_ms,
                    error=str(err),
                )
            )
            print(f"  [ERR] {err}")

    write_csv(output_csv, rows)

    print("\nBatch result")
    print(f"- success : {success}")
    print(f"- failed  : {failed}")
    print(f"- csv     : {output_csv}")

    return 0 if failed == 0 else 2


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("Interrupted")
        sys.exit(130)
