#!/usr/bin/env python3
"""
Inject dummy segmentation tasks ke RabbitMQ untuk uji konektivitas
worker PC-A & PC-B tanpa harus melalui frontend / API Master Go.

Setiap dummy chunk = 256x256 float32 zeros (262144 bytes raw, ~349528 char base64).
Worker akan tetap melakukan inference UNETR seperti biasa, tapi datanya dummy.

Tujuan:
  - Verifikasi 3 consumer (PC-A GPU0, PC-A GPU1, PC-B GPU0) terdaftar di queue
  - Verifikasi pull-based work-stealing dengan prefetch_count=1
  - Verifikasi ACK + Redis write berjalan normal

Contoh pemakaian:

  python inject_dummy_tasks.py --count 12 --mode homogen

  python inject_dummy_tasks.py \
      --count 6 \
      --mode heterogen \
      --queue segmentation_tasks_heterogen \
      --host 100.110.113.24

  python inject_dummy_tasks.py --count 1 --task-id manual_smoke_test
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import sys
import time

import numpy as np
import pika


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Publish dummy segmentation chunks ke RabbitMQ untuk uji distributed worker.",
    )
    parser.add_argument(
        "--count",
        type=int,
        default=12,
        help="Jumlah dummy chunk yang dipublish (default: 12).",
    )
    parser.add_argument(
        "--mode",
        choices=["homogen", "heterogen", "legacy"],
        default="homogen",
        help="Cluster mode label di payload (default: homogen).",
    )
    parser.add_argument(
        "--queue",
        default=None,
        help=(
            "Nama queue tujuan. Default: auto-pilih sesuai --mode "
            "(segmentation_tasks_homogen, segmentation_tasks_heterogen, segmentation_tasks)."
        ),
    )
    parser.add_argument(
        "--host",
        default=os.getenv("RABBITMQ_HOST", "localhost"),
        help="Host RabbitMQ. Dari laptop Master pakai 'localhost' (port 5674).",
    )
    parser.add_argument(
        "--port",
        type=int,
        default=int(os.getenv("RABBITMQ_PORT", "5674")),
        help="Port AMQP RabbitMQ (default: 5674).",
    )
    parser.add_argument(
        "--user",
        default=os.getenv("RABBITMQ_USER", "brainnav"),
        help="Username RabbitMQ (default: brainnav).",
    )
    parser.add_argument(
        "--password",
        default=os.getenv("RABBITMQ_PASS", "BrainNav_Secure_2025!"),
        help="Password RabbitMQ (default dari env atau hardcoded).",
    )
    parser.add_argument(
        "--vhost",
        default=os.getenv("RABBITMQ_VHOST", "brainnav_vhost"),
        help="Virtual host RabbitMQ (default: brainnav_vhost).",
    )
    parser.add_argument(
        "--task-id",
        default=None,
        help="Override task_id (default: auto generate dummy_<timestamp>).",
    )
    parser.add_argument(
        "--chunk-size",
        type=int,
        default=256,
        help="Sisi chunk persegi (default: 256 — sesuai pipeline real).",
    )
    parser.add_argument(
        "--declare-queue",
        action="store_true",
        help="Declare queue durable=true sebelum publish (aman dijalankan berulang).",
    )
    return parser.parse_args()


def resolve_queue(mode: str, override: str | None) -> str:
    if override:
        return override
    return {
        "homogen": "segmentation_tasks_homogen",
        "heterogen": "segmentation_tasks_heterogen",
        "legacy": "segmentation_tasks",
    }[mode]


def build_dummy_chunk_b64(chunk_size: int) -> tuple[str, int]:
    """Buat base64 dummy chunk = chunk_size x chunk_size float32 zeros."""
    arr = np.zeros((chunk_size, chunk_size), dtype=np.float32)
    raw = arr.tobytes()
    encoded = base64.b64encode(raw).decode("utf-8")
    return encoded, len(encoded)


def main() -> int:
    args = parse_args()
    queue_name = resolve_queue(args.mode, args.queue)
    base_task_id = args.task_id or f"dummy_test_{int(time.time())}"
    chunk_b64, b64_len = build_dummy_chunk_b64(args.chunk_size)

    creds = pika.PlainCredentials(args.user, args.password)
    params = pika.ConnectionParameters(
        host=args.host,
        port=args.port,
        virtual_host=args.vhost,
        credentials=creds,
        heartbeat=30,
        blocked_connection_timeout=10,
    )

    print(
        f"Connecting to RabbitMQ at {args.host}:{args.port} (vhost={args.vhost}) ...",
        flush=True,
    )
    try:
        conn = pika.BlockingConnection(params)
    except pika.exceptions.AMQPConnectionError as exc:
        print(f"[ERROR] Failed to connect: {exc}", file=sys.stderr)
        print(
            "Pastikan: (1) Master RabbitMQ container running, (2) port 5674 reachable, "
            "(3) credential & vhost benar.",
            file=sys.stderr,
        )
        return 1

    channel = conn.channel()
    if args.declare_queue:
        channel.queue_declare(queue=queue_name, durable=True)
        print(f"Queue '{queue_name}' declared (durable=true).")

    print(
        f"Publishing {args.count} dummy chunks to queue '{queue_name}' "
        f"(mode={args.mode}, chunk={args.chunk_size}x{args.chunk_size}, base64_len={b64_len}) ...",
        flush=True,
    )

    for i in range(args.count):
        published_at_ns = time.time_ns()
        payload = {
            "task_id": base_task_id,
            "chunk_id": i,
            "chunk_data": chunk_b64,
            "chunk_shape": [args.chunk_size, args.chunk_size],
            "position": {
                "x": 0,
                "y": 0,
                "width": args.chunk_size,
                "height": args.chunk_size,
                "overlap_x": 0,
                "overlap_y": 0,
            },
            "total_chunks": args.count,
            "retry_count": 0,
            "cluster_mode": args.mode,
            "published_at": published_at_ns,
            "chunk_size": args.chunk_size,
            "overlap_size": 0,
        }

        channel.basic_publish(
            exchange="",
            routing_key=queue_name,
            body=json.dumps(payload),
            properties=pika.BasicProperties(
                delivery_mode=2,  # persistent message
                content_type="application/json",
            ),
        )
        print(f"  [{i + 1:>3}/{args.count}] task={base_task_id} chunk={i}")

    conn.close()
    print(
        f"\nDone. {args.count} dummy tasks published to '{queue_name}'.\n"
        f"Verify with:\n"
        f"  docker exec brainnav-rabbitmq rabbitmqctl list_queues -p {args.vhost}\n"
        f"  docker exec brainnav-rabbitmq rabbitmqctl list_consumers -p {args.vhost}\n"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
