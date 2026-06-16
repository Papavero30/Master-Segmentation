# BrainNav Experiment Scripts

Scripts in this folder support post-hoc evaluation and batch experiment execution for segmentation experiments.

## 1) Setup

Install dependencies:

```powershell
Set-Location "d:\MateriKuliahWajib\Tugas Akhir\Implementasi\scripts"
python -m pip install -r requirements.txt
```

Optional environment variables:

- `BRAINNAV_API_BASE` (default: `https://localhost:8444/api`)
- `BRAINNAV_ACCESS_TOKEN` (JWT for protected API)
- `BRAINNAV_REDIS_URL` (default: `redis://localhost:6381/0`)
- `BRAINNAV_PG_DSN` (default: `postgresql://brainnav:brainnav_secure_password_change_me@localhost:5436/brainnav`)

## 2) Run Batch Segmentation

Run multiple slices and export task results to CSV.

```powershell
python run_experiment_batch.py \
  --scan-sid scan_1770291333710_yycgnw4 \
  --mode homogen \
  --slices 0-99 \
  --access-token "<JWT_TOKEN>" \
  --output-csv homogen_0_99.csv
```

For self-signed TLS (default behavior), certificate verification is disabled.

If your environment has a trusted certificate, add `--verify-tls`.

## 3) Evaluate Single Task (Dice + Hausdorff)

```powershell
python evaluate_segmentation.py \
  --task-id seg_abcd1234 \
  --ground-truth "d:\ground_truth\scan_1770291333710_yycgnw4_slice_12.npy" \
  --pred-shape 512x512
```

## 4) Evaluate Batch Tasks

The evaluator fetches completed experiments from PostgreSQL where `dice_score IS NULL`.

Ground-truth files must be named:

- `<scan_sid>_slice_<slice_index>.npy`
- `<scan_sid>_slice_<slice_index>.npz`
- `<scan_sid>_slice_<slice_index>.nii`
- `<scan_sid>_slice_<slice_index>.nii.gz`

Example:

```powershell
python evaluate_segmentation.py \
  --batch \
  --mode heterogen \
  --ground-truth-dir "d:\ground_truth"
```

## 5) SQL Validation Query

After evaluation:

```sql
SELECT task_id, cluster_mode, total_latency_ms, dice_score, hausdorff_distance
FROM segmentation_experiments
ORDER BY submitted_at DESC
LIMIT 20;
```
