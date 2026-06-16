#!/usr/bin/env python3
"""
Post-hoc evaluator for Dice Similarity Coefficient and Hausdorff Distance.

It reads final masks from Redis key: segmentation:final:{task_id}
and updates segmentation_experiments.dice_score + hausdorff_distance in PostgreSQL.
"""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path
from typing import Iterable, Optional, Sequence, Tuple

import nibabel as nib
import numpy as np
import psycopg2
import redis
from scipy.spatial.distance import directed_hausdorff


def parse_shape(shape_text: str) -> Tuple[int, ...]:
    cleaned = shape_text.lower().replace("x", ",")
    parts = [p.strip() for p in cleaned.split(",") if p.strip()]
    if len(parts) < 2:
        raise ValueError("--pred-shape must contain at least 2 dimensions, e.g. 512x512")
    dims = tuple(int(p) for p in parts)
    if any(d <= 0 for d in dims):
        raise ValueError("--pred-shape dimensions must be positive")
    return dims


def load_ground_truth(path: Path) -> np.ndarray:
    suffixes = [s.lower() for s in path.suffixes]

    if path.suffix.lower() == ".npy":
        return np.load(path)

    if path.suffix.lower() == ".npz":
        data = np.load(path)
        if not data.files:
            raise ValueError(f"No arrays found in npz file: {path}")
        return data[data.files[0]]

    if suffixes[-2:] == [".nii", ".gz"] or path.suffix.lower() == ".nii":
        return nib.load(str(path)).get_fdata()

    raise ValueError(f"Unsupported ground-truth format: {path}")


def binarize(arr: np.ndarray, threshold: float = 0.5) -> np.ndarray:
    return (arr > threshold).astype(np.uint8)


def dice_coefficient(pred: np.ndarray, gt: np.ndarray) -> float:
    pred_bin = binarize(pred)
    gt_bin = binarize(gt)

    pred_sum = int(pred_bin.sum())
    gt_sum = int(gt_bin.sum())

    if pred_sum == 0 and gt_sum == 0:
        return 1.0
    if pred_sum == 0 or gt_sum == 0:
        return 0.0

    intersection = int((pred_bin * gt_bin).sum())
    return (2.0 * intersection) / (pred_sum + gt_sum)


def hausdorff_distance(pred: np.ndarray, gt: np.ndarray) -> float:
    pred_points = np.argwhere(binarize(pred) > 0)
    gt_points = np.argwhere(binarize(gt) > 0)

    if len(pred_points) == 0 and len(gt_points) == 0:
        return 0.0
    if len(pred_points) == 0 or len(gt_points) == 0:
        return float("inf")

    forward = directed_hausdorff(pred_points, gt_points)[0]
    backward = directed_hausdorff(gt_points, pred_points)[0]
    return float(max(forward, backward))


def fetch_result_from_redis(task_id: str, redis_client: redis.Redis, pred_shape: Sequence[int]) -> np.ndarray:
    key = f"segmentation:final:{task_id}"
    payload = redis_client.get(key)
    if payload is None:
        raise ValueError(f"No final result found in Redis for task_id={task_id}")

    expected_size = int(np.prod(pred_shape))
    arr = np.frombuffer(payload, dtype=np.float32)
    if arr.size != expected_size:
        raise ValueError(
            f"Invalid result size for {task_id}: got {arr.size}, expected {expected_size} "
            f"for shape {tuple(pred_shape)}"
        )

    return arr.reshape(tuple(pred_shape))


def update_experiment_scores(task_id: str, dice: float, hd: float, pg_conn) -> None:
    with pg_conn.cursor() as cur:
        cur.execute(
            """
            UPDATE segmentation_experiments
            SET dice_score = %s,
                hausdorff_distance = %s
            WHERE task_id = %s
            """,
            (dice, hd, task_id),
        )
    pg_conn.commit()


def find_ground_truth_file(ground_truth_dir: Path, scan_sid: str, slice_index: int) -> Optional[Path]:
    stem = f"{scan_sid}_slice_{slice_index}"
    candidates = [
        ground_truth_dir / f"{stem}.npy",
        ground_truth_dir / f"{stem}.npz",
        ground_truth_dir / f"{stem}.nii",
        ground_truth_dir / f"{stem}.nii.gz",
    ]

    for candidate in candidates:
        if candidate.exists():
            return candidate

    return None


def load_batch_targets(pg_conn, mode: Optional[str]) -> Iterable[Tuple[str, str, int]]:
    mode_value = (mode or "").strip().lower()

    query = """
        SELECT task_id, scan_sid, slice_index
        FROM segmentation_experiments
        WHERE status = 'completed'
          AND dice_score IS NULL
          AND (%s = '' OR cluster_mode = %s)
        ORDER BY submitted_at ASC
    """

    with pg_conn.cursor() as cur:
        cur.execute(query, (mode_value, mode_value))
        rows = cur.fetchall()

    for task_id, scan_sid, slice_index in rows:
        yield str(task_id), str(scan_sid), int(slice_index)


def evaluate_single(task_id: str, gt_path: Path, pred_shape: Sequence[int], redis_client: redis.Redis, pg_conn) -> None:
    pred = fetch_result_from_redis(task_id, redis_client, pred_shape)
    gt = load_ground_truth(gt_path)

    if gt.shape != pred.shape:
        raise ValueError(f"Shape mismatch for {task_id}: pred={pred.shape}, gt={gt.shape} ({gt_path})")

    dice = dice_coefficient(pred, gt)
    hd = hausdorff_distance(pred, gt)

    print(f"[OK] {task_id}: Dice={dice:.4f}, Hausdorff={hd:.4f}")
    update_experiment_scores(task_id, dice, hd, pg_conn)


def main() -> int:
    parser = argparse.ArgumentParser(description="Evaluate segmentation result(s) with Dice and Hausdorff")

    parser.add_argument("--task-id", help="Evaluate a single task_id")
    parser.add_argument("--ground-truth", help="Path to ground-truth file for single-task mode")

    parser.add_argument("--batch", action="store_true", help="Evaluate all pending completed experiments")
    parser.add_argument("--mode", choices=["homogen", "heterogen", "legacy"], help="Optional mode filter in batch mode")
    parser.add_argument("--ground-truth-dir", help="Directory containing GT files named <scan_sid>_slice_<slice>.npy|npz|nii|nii.gz")

    parser.add_argument("--pred-shape", default="512x512", help="Prediction tensor shape stored in Redis (default: 512x512)")
    parser.add_argument("--redis-url", default=os.getenv("BRAINNAV_REDIS_URL", "redis://localhost:6381/0"))
    parser.add_argument(
        "--pg-dsn",
        default=os.getenv(
            "BRAINNAV_PG_DSN",
            "postgresql://brainnav:brainnav_secure_password_change_me@localhost:5436/brainnav",
        ),
    )

    args = parser.parse_args()

    if not args.task_id and not args.batch:
        parser.error("Choose one mode: --task-id <id> OR --batch")

    if args.task_id and args.batch:
        parser.error("Use only one mode at a time: --task-id OR --batch")

    pred_shape = parse_shape(args.pred_shape)

    redis_client = redis.Redis.from_url(args.redis_url)
    pg_conn = psycopg2.connect(args.pg_dsn)

    try:
        if args.task_id:
            if not args.ground_truth:
                parser.error("--ground-truth is required for single-task mode")
            gt_path = Path(args.ground_truth)
            if not gt_path.exists():
                raise FileNotFoundError(f"Ground-truth file not found: {gt_path}")
            evaluate_single(args.task_id, gt_path, pred_shape, redis_client, pg_conn)
            return 0

        if not args.ground_truth_dir:
            parser.error("--ground-truth-dir is required for --batch mode")

        gt_dir = Path(args.ground_truth_dir)
        if not gt_dir.exists():
            raise FileNotFoundError(f"Ground-truth directory not found: {gt_dir}")

        total = 0
        ok = 0
        skipped = 0
        failed = 0

        for task_id, scan_sid, slice_index in load_batch_targets(pg_conn, args.mode):
            total += 1
            gt_path = find_ground_truth_file(gt_dir, scan_sid, slice_index)
            if gt_path is None:
                skipped += 1
                print(f"[SKIP] {task_id}: ground-truth missing for {scan_sid} slice {slice_index}")
                continue

            try:
                evaluate_single(task_id, gt_path, pred_shape, redis_client, pg_conn)
                ok += 1
            except Exception as err:  # pylint: disable=broad-except
                failed += 1
                print(f"[ERR] {task_id}: {err}")

        print("\nBatch summary")
        print(f"- total   : {total}")
        print(f"- success : {ok}")
        print(f"- skipped : {skipped}")
        print(f"- failed  : {failed}")

        return 0 if failed == 0 else 2
    finally:
        pg_conn.close()


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("Interrupted")
        sys.exit(130)
