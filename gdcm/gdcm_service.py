#!/usr/bin/env python3
"""
BrainNav GDCM Service - minimal PNG rendering endpoints
"""
import io
import os
import sys
import json
import logging
import traceback
import shutil
import subprocess
from datetime import datetime
from pathlib import Path
from typing import List, Optional, Tuple
import hashlib
from werkzeug.utils import secure_filename

import numpy as np
from pydicom import dcmread
from flask import Flask, request, jsonify, send_file
import threading
from PIL import Image

# Logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('/app/logs/gdcm-service.log'),
        logging.StreamHandler(sys.stdout)
    ]
)
logger = logging.getLogger('gdcm-service')

app = Flask(__name__)

TRANSFER_SYNTAX_J2K_LOSSLESS = '1.2.840.10008.1.2.4.90'


def _data_dir() -> Path:
    """Get the base data directory"""
    return Path(os.getenv('GDCM_DATA_DIR', '/app/data'))


def _series_dir_candidates(sid: str) -> List[Path]:
    base = _data_dir()
    out: List[Path] = []
    # 1) Direct match on directory name
    # Prefer decompressed cache first
    p_decomp = base / sid / 'decompressed'
    if p_decomp.exists():
        out.append(p_decomp)
    p = base / sid
    if p.exists():
        out.append(p)
    pc = p / 'compressed'
    if pc.exists():
        out.append(pc)
    if out:
        logger.info("[series-scan] Direct match found for sid=%s in: %s", sid, ", ".join(str(p) for p in out))
        return out

    # 2) Fallback: search first-level subdirectories for ones containing DICOM with matching SeriesInstanceUID
    def dir_matches_series(d: Path) -> bool:
        try:
            # Probe a few files to find one with pixels/tags
            files = _collect_dicom_files(d)
            logger.info("[series-scan] Probing directory: %s; files=%d", d, len(files))
            for probe in files[:200]:
                try:
                    ds = dcmread(str(probe), stop_before_pixels=True, force=True)
                    suid = str(getattr(ds, 'SeriesInstanceUID', '')).strip()
                    if suid == sid:
                        logger.info("[series-scan] Match in %s via file %s", d, probe)
                        return True
                except Exception:
                    continue
        except Exception:
            return False
        return False

    # Search in base directory
    if not base.exists():
        logger.warning("[series-scan] No match for sid=%s, base directory not found: %s", sid, base)
        return out
    
    try:
        for child in base.iterdir():
            if not child.is_dir():
                continue
            # Check the folder and an optional 'compressed' subfolder
            candidates = [child, child / 'compressed']
            for cand in candidates:
                if cand.is_dir() and dir_matches_series(cand):
                    out.append(cand)
    except Exception:
        pass
    
    if out:
        logger.info("[series-scan] Fallback match for sid=%s in: %s", sid, ", ".join(str(p) for p in out))
    else:
        logger.warning("[series-scan] No match for sid=%s under %s", sid, base)
    return out


def _collect_dicom_files(dir_path: Path) -> List[Path]:
    files: List[Path] = []
    for root, _dirs, names in os.walk(dir_path):
        for n in names:
            # Many DICOM files are saved without an extension; include those too
            nl = n.lower()
            if nl.endswith(('.dcm', '.dicom')) or ('.' not in n):
                p = Path(root) / n
                # Skip obvious non-files
                try:
                    if p.is_file() and p.stat().st_size > 0:
                        files.append(p)
                except Exception:
                    # Best-effort: ignore unreadable entries
                    continue
    return files


def _sort_by_instance(files: List[Path]) -> List[Path]:
    def key(path: Path):
        try:
            ds = dcmread(str(path), stop_before_pixels=True, force=True)
            inst = int(getattr(ds, 'InstanceNumber', 0) or 0)
            sliceloc = float(getattr(ds, 'SliceLocation', 0.0) or 0.0)
            sop = str(getattr(ds, 'SOPInstanceUID', ''))
            return inst, sliceloc, sop
        except Exception:
            return (0, 0.0, path.name)
    return sorted(files, key=key)


def group_slices_by_position(files: List[Path], max_slices_per_group: int = 50) -> List[List[Path]]:
    """
    Group DICOM files into multiple series based on InstanceNumber or SliceLocation.
    This preserves legacy behavior where 1 folder → multiple series by slice position.
    
    Args:
        files: List of DICOM file paths
        max_slices_per_group: Maximum number of slices per group (default: 50)
    
    Returns:
        List of groups, where each group is a list of DICOM file paths
    """
    if not files:
        return []
    
    # First, sort files by instance number and slice location
    sorted_files = _sort_by_instance(files)
    
    # Group files by consecutive ranges
    groups: List[List[Path]] = []
    current_group: List[Path] = []
    
    for file_path in sorted_files:
        try:
            ds = dcmread(str(file_path), stop_before_pixels=True, force=True)
            inst_num = int(getattr(ds, 'InstanceNumber', 0) or 0)
            
            # Start new group if current group is full
            if len(current_group) >= max_slices_per_group:
                if current_group:
                    groups.append(current_group)
                current_group = [file_path]
            else:
                current_group.append(file_path)
                
        except Exception as e:
            logger.warning(f"[slice-grouping] Failed to read DICOM metadata from {file_path}: {e}")
            # Add to current group anyway
            if len(current_group) >= max_slices_per_group:
                if current_group:
                    groups.append(current_group)
                current_group = [file_path]
            else:
                current_group.append(file_path)
    
    # Add the last group
    if current_group:
        groups.append(current_group)
    
    logger.info(f"[slice-grouping] Split {len(sorted_files)} files into {len(groups)} groups (max {max_slices_per_group} per group)")
    return groups


def _default_wc_ww(ds) -> Tuple[float, float]:
    wc = getattr(ds, 'WindowCenter', None)
    ww = getattr(ds, 'WindowWidth', None)
    if isinstance(wc, (list, tuple)):
        wc = wc[0]
    if isinstance(ww, (list, tuple)):
        ww = ww[0]
    if wc is not None and ww is not None and float(ww) > 1:
        return float(wc), float(ww)
    modality = str(getattr(ds, 'Modality', '')).upper()
    if modality == 'CT':
        return 40.0, 400.0
    if modality in ('MR', 'MRI'):
        return 50.0, 350.0
    return 128.0, 256.0


def _to_png_bytes(arr: np.ndarray, photometric: str) -> bytes:
    arr = np.clip(arr, 0, 255).astype(np.uint8)
    if photometric.upper() == 'MONOCHROME1':
        arr = 255 - arr
    img = Image.fromarray(arr, mode='L')
    bio = io.BytesIO()
    img.save(bio, format='PNG')
    bio.seek(0)
    return bio.read()


def _window_image(pixel_array: np.ndarray, wc: float, ww: float, slope: float, intercept: float) -> np.ndarray:
    arr = (pixel_array.astype(np.float32) * slope) + intercept
    low = wc - (ww / 2.0)
    high = wc + (ww / 2.0)
    arr = (arr - low) * (255.0 / (high - low))
    return arr


def _load_frame_png_from_series(sid: str, frame_index: int, wc: Optional[float], ww: Optional[float]):
    for base in _series_dir_candidates(sid):
        files = _collect_dicom_files(base)
        if not files:
            continue
        files = _sort_by_instance(files)
        try:
            ds0 = dcmread(str(files[0]), force=True)
            px = ds0.pixel_array
            photometric = str(getattr(ds0, 'PhotometricInterpretation', 'MONOCHROME2'))
            slope = float(getattr(ds0, 'RescaleSlope', 1) or 1)
            intercept = float(getattr(ds0, 'RescaleIntercept', 0) or 0)
            if px.ndim == 3:
                f = int(max(0, min(frame_index, px.shape[0] - 1)))
                frame = px[f]
                _wc, _ww = _default_wc_ww(ds0)
                wc_val = float(wc) if wc is not None else _wc
                ww_val = float(ww) if ww is not None else _ww
                windowed = _window_image(frame, wc_val, ww_val, slope, intercept)
                return _to_png_bytes(windowed, photometric), 'image/png'
            f = int(max(0, min(frame_index, len(files) - 1)))
            ds = dcmread(str(files[f]), force=True)
            photometric = str(getattr(ds, 'PhotometricInterpretation', 'MONOCHROME2'))
            slope = float(getattr(ds, 'RescaleSlope', 1) or 1)
            intercept = float(getattr(ds, 'RescaleIntercept', 0) or 0)
            arr = ds.pixel_array
            _wc, _ww = _default_wc_ww(ds)
            wc_val = float(wc) if wc is not None else _wc
            ww_val = float(ww) if ww is not None else _ww
            windowed = _window_image(arr, wc_val, ww_val, slope, intercept)
            return _to_png_bytes(windowed, photometric), 'image/png'
        except Exception:
            logger.exception("Failed to render frame from series %s", sid)
            continue
    raise FileNotFoundError(f"No DICOM files found for series {sid}")


def _load_thumbnail_png(sid: str):
    candidates = _series_dir_candidates(sid)
    for base in candidates:
        files = _collect_dicom_files(base)
        if not files:
            logger.warning("[thumbnail] No dicom files in %s for sid=%s", base, sid)
            continue
        files = _sort_by_instance(files)
        try:
            ds0 = dcmread(str(files[0]), force=True)
            px = ds0.pixel_array
            photometric = str(getattr(ds0, 'PhotometricInterpretation', 'MONOCHROME2'))
            slope = float(getattr(ds0, 'RescaleSlope', 1) or 1)
            intercept = float(getattr(ds0, 'RescaleIntercept', 0) or 0)
            if px.ndim == 3:
                f = int(px.shape[0] // 2)
                frame = px[f]
            else:
                f = int(len(files) // 2)
                ds = dcmread(str(files[f]), force=True)
                photometric = str(getattr(ds, 'PhotometricInterpretation', 'MONOCHROME2'))
                slope = float(getattr(ds, 'RescaleSlope', 1) or 1)
                intercept = float(getattr(ds, 'RescaleIntercept', 0) or 0)
                frame = ds.pixel_array
            wc, ww = _default_wc_ww(ds0)
            windowed = _window_image(frame, wc, ww, slope, intercept)
            return _to_png_bytes(windowed, photometric), 'image/png'
        except Exception:
            logger.exception("[thumbnail] Failed to render for series %s in %s", sid, base)
            continue
    raise FileNotFoundError(f"No DICOM files found for series {sid}")


@app.route('/health', methods=['GET'])
def health_check():
    return jsonify({
        'status': 'healthy',
        'service': 'brainnav-gdcm',
        'version': '1.0.0',
        'timestamp': datetime.utcnow().isoformat()
    })


@app.route('/render-frame', methods=['GET'])
def render_frame():
    try:
        sid = request.args.get('sid', type=str)
        frame = request.args.get('frame', default=0, type=int)
        wc = request.args.get('wc', default=None, type=float)
        ww = request.args.get('ww', default=None, type=float)
        if not sid:
            return jsonify({'error': 'sid is required'}), 400
        data, mime = _load_frame_png_from_series(sid, frame, wc, ww)
        return send_file(io.BytesIO(data), mimetype=mime)
    except FileNotFoundError as e:
        return jsonify({'error': str(e)}), 404
    except Exception as e:
        logger.error("render-frame failed: %s\n%s", e, traceback.format_exc())
        return jsonify({'error': 'render failed'}), 500


@app.route('/thumbnail', methods=['GET'])
def thumbnail():
    try:
        sid = request.args.get('sid', type=str)
        if not sid:
            return jsonify({'error': 'sid is required'}), 400
        data, mime = _load_thumbnail_png(sid)
        return send_file(io.BytesIO(data), mimetype=mime)
    except FileNotFoundError as e:
        return jsonify({'error': str(e)}), 404
    except Exception as e:
        logger.error("thumbnail failed: %s\n%s", e, traceback.format_exc())
        return jsonify({'error': 'thumbnail failed'}), 500


########################
# Upload & Series Ops  #
########################

def _compress_series_internal(sid: str) -> dict:
    """Internal compression logic extracted for reuse (returns result dict)."""
    logger.info(f"\n{'='*80}")
    logger.info(f"[COMPRESSION START] Series ID: {sid}")
    logger.info(f"{'='*80}")
    
    series_dir = _data_dir() / sid
    if not series_dir.exists() or not series_dir.is_dir():
        logger.error(f"[COMPRESSION ERROR] Series directory not found: {series_dir}")
        return {'status': 'error', 'error': 'raw series not found', 'sid': sid}
    
    logger.info(f"[COMPRESSION] Series directory: {series_dir}")
    
    compressed_dir = series_dir / 'compressed'
    compressed_dir.mkdir(parents=True, exist_ok=True)
    logger.info(f"[COMPRESSION] Target directory: {compressed_dir}")
    
    # Determine source directory - check if files are in 'original' subdirectory
    original_dir = series_dir / 'original'
    if original_dir.exists() and original_dir.is_dir():
        raw_dir = original_dir
        logger.info(f"[COMPRESSION] ✅ Using original/ subdirectory as source")
    else:
        raw_dir = series_dir
        logger.info(f"[COMPRESSION] ✅ Using series root as source")
    
    logger.info(f"[COMPRESSION] Source directory: {raw_dir}")
    
    # Collect all DICOM files
    logger.info(f"[COMPRESSION] Collecting DICOM files...")
    all_files = _collect_dicom_files(raw_dir)
    if not all_files:
        logger.error(f"[COMPRESSION ERROR] No DICOM files found in {raw_dir}")
        return {'status': 'error', 'error': 'no DICOM files found', 'sid': sid}
    
    logger.info(f"[COMPRESSION] ✅ Found {len(all_files)} DICOM files")
    
    # Group files by slice position (max 50 per group for legacy compatibility)
    logger.info(f"[COMPRESSION] Grouping files (max 50 per group)...")
    slice_groups = group_slices_by_position(all_files, max_slices_per_group=50)
    logger.info(f"[COMPRESSION] ✅ Created {len(slice_groups)} groups")
    
    total_in = 0
    total_out = 0
    processed = 0
    failed = 0
    group_manifests = []
    
    # Process each group separately
    for group_idx, group_files in enumerate(slice_groups):
        group_id = f"{sid}_group{group_idx}" if len(slice_groups) > 1 else sid
        group_entries = []
        group_size_in = 0
        group_size_out = 0
        
        logger.info(f"\n[COMPRESSION GROUP {group_idx + 1}/{len(slice_groups)}] Processing {len(group_files)} files...")
        logger.info(f"[COMPRESSION GROUP {group_idx + 1}/{len(slice_groups)}] Group ID: {group_id}")
        
        for file_idx, src in enumerate(group_files, 1):
            try:
                # Progress indicator every 10 files
                if file_idx % 10 == 0:
                    logger.info(f"[COMPRESSION GROUP {group_idx + 1}] Progress: {file_idx}/{len(group_files)} files")
                
                if src.stat().st_size == 0:
                    logger.warning(f"[COMPRESSION] ⚠️  Skipping empty file: {src.name}")
                    continue
                
                rel = src.relative_to(raw_dir)
                dst = compressed_dir / rel
                dst.parent.mkdir(parents=True, exist_ok=True)
                
                # Log compression attempt
                logger.debug(f"[COMPRESSION] Compressing: {src.name} → {dst.name}")
                
                # CRITICAL FIX: Use -K for JPEG2000 LOSSLESS (not -J which is lossy!)
                # -K preserves exact pixel values (truly lossless)
                # -J applies lossy compression (introduces artifacts)
                cmd = ["gdcmconv", "-K", str(src), str(dst)]
                try:
                    result = subprocess.run(
                        cmd, 
                        check=True, 
                        stdout=subprocess.PIPE, 
                        stderr=subprocess.PIPE, 
                        timeout=120
                    )
                    logger.debug(f"[COMPRESSION] ✅ gdcmconv success: {src.name}")
                    
                except FileNotFoundError:
                    logger.warning(f"[COMPRESSION] ⚠️  gdcmconv not found, copying: {src.name}")
                    shutil.copy2(src, dst)
                    
                except subprocess.CalledProcessError as e:
                    stderr_msg = e.stderr.decode() if e.stderr else str(e)
                    logger.error(f"[COMPRESSION] ❌ gdcmconv failed for {src.name}: {stderr_msg}")
                    failed += 1
                    continue
                # Check if output was created
                if not dst.exists():
                    logger.error(f"[COMPRESSION] ❌ Output file not created: {dst.name}")
                    failed += 1
                    continue
                
                size_in = src.stat().st_size
                size_out = dst.stat().st_size
                total_in += size_in
                total_out += size_out
                group_size_in += size_in
                group_size_out += size_out
                processed += 1
                
                # Log compression ratio for this file
                file_ratio = (1 - (size_out / size_in)) * 100 if size_in > 0 else 0
                logger.debug(f"[COMPRESSION] File: {src.name} | {size_in:,} → {size_out:,} bytes ({file_ratio:.1f}%)")
                
                try:
                    with open(dst, 'rb') as df:
                        digest = hashlib.sha256(df.read()).hexdigest()
                    ds_meta = dcmread(str(dst), stop_before_pixels=True, force=True)
                    inst_num = int(getattr(ds_meta, 'InstanceNumber', 0) or 0)
                    sop_uid = str(getattr(ds_meta, 'SOPInstanceUID', '')).strip()
                    slice_loc = float(getattr(ds_meta, 'SliceLocation', 0.0) or 0.0)
                    group_entries.append({
                        'path': str(Path('compressed') / rel),
                        'size': size_out,
                        'hash': digest,
                        'instance_number': inst_num,
                        'slice_location': slice_loc,
                        'sop_instance_uid': sop_uid,
                    })
                except Exception as manifest_err:
                    logger.error(f'[COMPRESSION] ⚠️  Manifest generation failed for {dst.name}: {manifest_err}')
                
                # Delete original file after successful compression
                try:
                    src.unlink(missing_ok=True)
                    logger.debug(f"[COMPRESSION] Deleted original: {src.name}")
                except Exception as del_err:
                    logger.warning(f"[COMPRESSION] ⚠️  Failed to delete original {src.name}: {del_err}")
                    
            except Exception as file_err:
                logger.error(f"[COMPRESSION] ❌ Exception processing {src.name}: {file_err}")
                failed += 1
        
        # Create group-specific manifest entry
        group_ratio = None
        if group_size_in > 0 and group_size_out > 0:
            group_ratio = (1 - (group_size_out / group_size_in)) * 100.0
        
        logger.info(f"[COMPRESSION GROUP {group_idx + 1}] ✅ Completed")
        logger.info(f"[COMPRESSION GROUP {group_idx + 1}] Files processed: {len(group_entries)}")
        logger.info(f"[COMPRESSION GROUP {group_idx + 1}] Original size: {group_size_in:,} bytes")
        logger.info(f"[COMPRESSION GROUP {group_idx + 1}] Compressed size: {group_size_out:,} bytes")
        if group_ratio:
            logger.info(f"[COMPRESSION GROUP {group_idx + 1}] Compression ratio: {group_ratio:.2f}%")
        
        group_manifests.append({
            'group_id': group_id,
            'group_index': group_idx,
            'slice_count': len(group_files),
            'files': group_entries,
            'stats': {
                'original_size_bytes': group_size_in,
                'compressed_size_bytes': group_size_out,
                'compression_ratio_percent': group_ratio,
            }
        })
    
    try:
        for dirpath, dirnames, filenames in os.walk(raw_dir, topdown=False):
            if not dirnames and not filenames:
                Path(dirpath).rmdir()
    except Exception:
        pass
    
    ratio = None
    if total_in > 0 and total_out > 0:
        ratio = (1 - (total_out / total_in)) * 100.0
    status = 'success' if failed == 0 else ('partial' if processed > 0 else 'failed')
    
    # Create comprehensive manifest with group information
    manifest = {
        'sid': sid,
        'generated_at': datetime.utcnow().isoformat() + 'Z',
        'transfer_syntax': TRANSFER_SYNTAX_J2K_LOSSLESS,
        'total_groups': len(slice_groups),
        'groups': group_manifests,
        'stats': {
            'original_size_bytes': total_in,
            'compressed_size_bytes': total_out,
            'compression_ratio_percent': ratio,
            'files_processed': processed,
            'files_failed': failed,
        },
    }
    manifest_path = _data_dir() / sid / 'manifest.json'
    try:
        with open(manifest_path, 'w', encoding='utf-8') as mf:
            json.dump(manifest, mf, indent=2)
    except Exception:
        logger.exception('[series-compress] failed writing manifest json at %s', manifest_path)
    result = {
        'status': status,
        'sid': sid,
        'operation': 'compress',
        'total_groups': len(slice_groups),
        'files_processed': processed,
        'files_failed': failed,
        'original_size_bytes': total_in,
        'compressed_size_bytes': total_out,
        'compression_ratio_percent': ratio,
        'transfer_syntax': TRANSFER_SYNTAX_J2K_LOSSLESS,
        'manifest_path': str(manifest_path.relative_to(_data_dir())) if manifest_path.exists() else None,
        'manifest': manifest,
    }
    
    # Final summary
    logger.info(f"\n{'='*80}")
    logger.info(f"[COMPRESSION COMPLETE] Series ID: {sid}")
    logger.info(f"{'='*80}")
    logger.info(f"[COMPRESSION SUMMARY] Status: {status.upper()}")
    logger.info(f"[COMPRESSION SUMMARY] Total groups: {len(slice_groups)}")
    logger.info(f"[COMPRESSION SUMMARY] Files processed: {processed}")
    logger.info(f"[COMPRESSION SUMMARY] Files failed: {failed}")
    logger.info(f"[COMPRESSION SUMMARY] Original size: {total_in:,} bytes ({total_in/1024/1024:.2f} MB)")
    logger.info(f"[COMPRESSION SUMMARY] Compressed size: {total_out:,} bytes ({total_out/1024/1024:.2f} MB)")
    if ratio:
        logger.info(f"[COMPRESSION SUMMARY] Compression ratio: {ratio:.2f}%")
    logger.info(f"[COMPRESSION SUMMARY] Manifest: {manifest_path}")
    logger.info(f"{'='*80}\n")
    # Persist stats for later polling
    try:
        meta_path = compressed_dir / '_series_stats.json'
        with open(meta_path, 'w', encoding='utf-8') as mf:
            json.dump(result, mf, indent=2)
    except Exception:
        logger.exception('[series-compress] failed writing stats json')
    return result


@app.route('/upload', methods=['POST'])
def upload_dicom():
    try:
        f = request.files.get('dicom_file')
        if not f:
            return jsonify({'error': 'dicom_file missing'}), 400
        provided_sid = request.form.get('sid') or request.args.get('sid') or ''
        compress_flag = request.form.get('compress', 'false').lower() == 'true'
        final_flag = request.form.get('final', 'false').lower() == 'true'
        data = f.read()
        if not data:
            return jsonify({'error': 'empty file'}), 400
        sid = provided_sid.strip()
        try:
            ds_meta = dcmread(io.BytesIO(data), stop_before_pixels=True, force=True)
            ds_sid = str(getattr(ds_meta, 'SeriesInstanceUID', '')).strip()
            if not sid:
                sid = ds_sid
        except Exception:
            logger.exception('Failed to read DICOM meta; fallback sid')
        if not sid:
            sid = hashlib.sha256(data).hexdigest()[:32]
        safe_name = secure_filename(f.filename) or f'dicom_{int(datetime.utcnow().timestamp())}.dcm'
        series_dir = _data_dir() / sid
        series_dir.mkdir(parents=True, exist_ok=True)
        out_path = series_dir / safe_name
        with open(out_path, 'wb') as wf:
            wf.write(data)
        file_count = sum(1 for p in series_dir.iterdir() if p.is_file())
        logger.info("[upload] stored file=%s sid=%s compress=%s final=%s count=%d", out_path, sid, compress_flag, final_flag, file_count)
        triggered = False
        if final_flag and compress_flag:
            triggered = True
            threading.Thread(target=_compress_series_internal, args=(sid,), daemon=True).start()
            logger.info("[upload] triggered async compression sid=%s", sid)
        return jsonify({
            'status': 'stored',
            'sid': sid,
            'filename': safe_name,
            'series_dir': str(series_dir),
            'count': file_count,
            'compressed': False,
            'triggered_compression': triggered,
        }), 200
    except Exception as e:
        logger.error('upload failed: %s', e)
        return jsonify({'error': 'upload failed'}), 500

# Stubs for old single-file APIs (kept for backward compatibility detection)
@app.route('/compress', methods=['POST'])
@app.route('/decompress', methods=['POST'])
@app.route('/info', methods=['POST'])
@app.route('/batch-compress', methods=['POST'])
def not_implemented():
    return jsonify({'status': 'not_implemented'}), 501


@app.route('/decompress-for-render', methods=['POST'])
def decompress_for_render():
    """
    Decompress JPEG2000 compressed DICOM files to render-ready format for WebGL/GLSL.
    This is optimized for frontend rendering - decompresses all files in a series.
    
    Request: {"sid": "series_id", "force": false}
    Response: {"status": "success", "files_processed": 120, "render_path": "..."}
    """
    try:
        data = request.get_json(force=True, silent=True) or {}
        sid = str(data.get('sid') or '').strip()
        force_redecompress = data.get('force', False)
        
        if not sid:
            return jsonify({'error': 'sid required'}), 400
        
        compressed_dir = _data_dir() / sid / 'compressed'
        if not compressed_dir.exists():
            return jsonify({'error': 'compressed series not found'}), 404
        
        render_dir = _data_dir() / sid / 'render'
        
        # Check cache
        if render_dir.exists() and not force_redecompress:
            existing_files = [f for f in render_dir.rglob('*') if f.is_file() and f.name != 'manifest.json']
            logger.info('[decompress-for-render] Cache hit: sid=%s (%d files)', sid, len(existing_files))
            return jsonify({
                'status': 'cached',
                'sid': sid,
                'files_processed': len(existing_files),
                'render_path': str(render_dir)
            }), 200
        
        render_dir.mkdir(parents=True, exist_ok=True)
        processed = 0
        failed = 0
        
        logger.info('[decompress-for-render] Decompressing sid=%s', sid)
        
        # Process all files except manifest.json (handles files with/without .dcm extension)
        for src in compressed_dir.rglob('*'):
            if not src.is_file() or src.name == 'manifest.json':
                continue
            
            rel = src.relative_to(compressed_dir)
            dst = render_dir / rel
            dst.parent.mkdir(parents=True, exist_ok=True)
            
            try:
                # Preserve metadata during decompression for rendering
                cmd = ["gdcmconv", "-w", "-X", "-R", str(src), str(dst)]
                subprocess.run(cmd, check=True, timeout=120, capture_output=True)
                processed += 1
            except (FileNotFoundError, subprocess.CalledProcessError):
                # Fallback: copy if gdcmconv fails
                shutil.copy2(src, dst)
                processed += 1
            except Exception as e:
                logger.error('[decompress-for-render] Failed %s: %s', src, e)
                failed += 1
        
        logger.info('[decompress-for-render] Done: %d processed, %d failed', processed, failed)
        
        return jsonify({
            'status': 'success' if failed == 0 else 'partial',
            'sid': sid,
            'files_processed': processed,
            'files_failed': failed,
            'render_path': str(render_dir)
        }), 200
        
    except Exception as e:
        logger.exception('[decompress-for-render] Error')
        return jsonify({'error': str(e)}), 500


if __name__ == '__main__':
    logger.info("Starting BrainNav GDCM Service...")
    app.run(host='0.0.0.0', port=3000, debug=os.getenv('FLASK_DEBUG', 'false').lower() == 'true')


@app.route('/series/compress', methods=['POST'])
def series_compress():
    try:
        data = request.get_json(force=True, silent=True) or {}
        sid = str(data.get('sid') or '').strip()
        if not sid:
            return jsonify({'error': 'sid required'}), 400
        result = _compress_series_internal(sid)
        status = result.get('status')
        if status == 'not_found' or result.get('error') == 'raw series not found':
            return jsonify({'error': 'raw series not found'}), 404
        code = 200 if status == 'success' else 207 if status == 'partial' else 500
        return jsonify(result), code
    except Exception as e:
        logger.exception('series compress failed')
        return jsonify({'error': 'series compress failed'}), 500


@app.route('/series/decompress', methods=['POST'])
def series_decompress():
    try:
        data = request.get_json(force=True, silent=True) or {}
        sid = str(data.get('sid') or '').strip()
        if not sid:
            return jsonify({'error': 'sid required'}), 400
        compressed_dir = _data_dir() / sid / 'compressed'
        if not compressed_dir.exists():
            return jsonify({'error': 'compressed series not found'}), 404
        decomp_dir = _data_dir() / sid / 'decompressed'
        if decomp_dir.exists():
            # Already cached
            return jsonify({'status': 'cached', 'sid': sid, 'operation': 'decompress'}), 200
        decomp_dir.mkdir(parents=True, exist_ok=True)
        processed = 0
        failed = 0
        for root, _dirs, files in os.walk(compressed_dir):
            for fname in files:
                src = Path(root) / fname
                rel = src.relative_to(compressed_dir)
                dst = decomp_dir / rel
                dst.parent.mkdir(parents=True, exist_ok=True)
                try:
                    # Decompress with metadata preservation using gdcmconv -w -X -R
                    cmd = ["gdcmconv", "-w", "-X", "-R", str(src), str(dst)]
                    try:
                        subprocess.run(cmd, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=120)
                    except FileNotFoundError:
                        # Fallback: copy (if compression not applied or tool missing)
                        shutil.copy2(src, dst)
                    except subprocess.CalledProcessError as e:
                        logger.error('[series-decompress] failed %s: %s', src, e)
                        failed += 1
                        continue
                    processed += 1
                except Exception:
                    logger.exception('[series-decompress] exception %s', src)
                    failed += 1
        status = 'success' if failed == 0 else ('partial' if processed > 0 else 'failed')
        return jsonify({
            'status': status,
            'sid': sid,
            'operation': 'decompress',
            'files_processed': processed,
            'files_failed': failed,
        }), (200 if status == 'success' else 207 if status == 'partial' else 500)
    except Exception:
        logger.exception('series decompress failed')
        return jsonify({'error': 'series decompress failed'}), 500


@app.route('/series/stats', methods=['GET'])
def series_stats():
    try:
        sid = request.args.get('sid', type=str, default='').strip()
        if not sid:
            return jsonify({'error': 'sid required'}), 400
        raw_dir = _data_dir() / sid
        comp_dir = _data_dir() / sid / 'compressed'
        decomp_dir = _data_dir() / sid / 'decompressed'
        data = {
            'sid': sid,
            'raw_exists': raw_dir.exists(),
            'compressed_exists': comp_dir.exists(),
            'decompressed_exists': decomp_dir.exists(),
            'status': 'pending',
        }
        # Try load persisted stats
        meta_path = comp_dir / '_series_stats.json'
        if meta_path.exists():
            import json
            try:
                with open(meta_path, 'r') as mf:
                    meta = json.load(mf)
                data.update(meta)
            except Exception:
                logger.exception('[series-stats] failed reading stats json')
        # Derive status
        if data.get('compressed_exists') and data.get('status') in ('success','partial','failed'):
            pass
        elif data.get('compressed_exists'):
            data['status'] = 'compressed'
        elif data.get('raw_exists'):
            data['status'] = 'pending'
        else:
            data['status'] = 'missing'
        return jsonify(data), 200
    except Exception:
        logger.exception('series stats failed')
        return jsonify({'error': 'series stats failed'}), 500


@app.route('/series/manifest', methods=['GET'])
def series_manifest():
    sid = request.args.get('sid', type=str, default='').strip()
    if not sid:
        return jsonify({'error': 'sid required'}), 400
    manifest_path = _data_dir() / sid / 'manifest.json'
    if not manifest_path.exists():
        return jsonify({'error': 'manifest not found'}), 404
    try:
        with open(manifest_path, 'r', encoding='utf-8') as mf:
            manifest = json.load(mf)
        return jsonify({'status': 'success', 'sid': sid, 'manifest': manifest}), 200
    except Exception:
        logger.exception('[series-manifest] failed for sid=%s', sid)
        return jsonify({'error': 'manifest read failed'}), 500


def _is_jpeg2000_compressed(file_path: Path) -> bool:
    """Check if DICOM file is JPEG 2000 compressed"""
    try:
        ds = dcmread(str(file_path), stop_before_pixels=True, force=True)
        transfer_syntax = str(getattr(ds, 'file_meta', None) and getattr(ds.file_meta, 'TransferSyntaxUID', ''))
        # JPEG 2000 Lossless and Lossy Transfer Syntaxes
        j2k_syntaxes = [
            '1.2.840.10008.1.2.4.90',  # JPEG 2000 Lossless
            '1.2.840.10008.1.2.4.91',  # JPEG 2000
        ]
        return transfer_syntax in j2k_syntaxes
    except Exception:
        return False


@app.route('/decompress/file', methods=['POST'])
def decompress_file():
    """
    Decompress a single DICOM file on-demand.
    Useful for frontend to request decompression of specific files.
    
    Request body:
    {
        "sid": "series_id",
        "file_path": "relative/path/to/file.dcm"
    }
    
    Returns:
    {
        "status": "success" | "cached" | "skipped" | "failed",
        "file_path": "decompressed/relative/path/to/file.dcm",
        "message": "optional message"
    }
    """
    try:
        data = request.get_json(force=True, silent=True) or {}
        sid = str(data.get('sid') or '').strip()
        rel_path = str(data.get('file_path') or '').strip()
        
        if not sid:
            return jsonify({'error': 'sid required'}), 400
        if not rel_path:
            return jsonify({'error': 'file_path required'}), 400
        
        # Normalize path
        rel_path = rel_path.replace('\\', '/').strip('/')
        
        # Determine source file location
        compressed_dir = _data_dir() / sid / 'compressed'
        input_dir = _data_dir() / sid
        
        # Try to find the source file
        source_file = None
        if (compressed_dir / rel_path).exists():
            source_file = compressed_dir / rel_path
            base_search_dir = compressed_dir
        elif rel_path.startswith('compressed/'):
            clean_path = rel_path[len('compressed/'):]
            if (compressed_dir / clean_path).exists():
                source_file = compressed_dir / clean_path
                rel_path = clean_path
                base_search_dir = compressed_dir
        elif (input_dir / rel_path).exists():
            source_file = input_dir / rel_path
            base_search_dir = input_dir
        
        if not source_file:
            return jsonify({'error': 'source file not found'}), 404
        
        # Check if already decompressed (cached)
        decomp_dir = _data_dir() / sid / 'decompressed'
        dest_file = decomp_dir / rel_path
        
        if dest_file.exists():
            logger.info('[decompress-file] Cache hit for sid=%s file=%s', sid, rel_path)
            return jsonify({
                'status': 'cached',
                'file_path': f'decompressed/{rel_path}',
                'message': 'File already decompressed and cached'
            }), 200
        
        # Check if file is actually compressed
        if not _is_jpeg2000_compressed(source_file):
            logger.info('[decompress-file] File not JPEG2000 compressed, skipping: %s', source_file)
            return jsonify({
                'status': 'skipped',
                'file_path': rel_path,
                'message': 'File is not JPEG 2000 compressed, no decompression needed'
            }), 200
        
        # Create output directory
        dest_file.parent.mkdir(parents=True, exist_ok=True)
        
        # Decompress using gdcmconv with metadata preservation
        logger.info('[decompress-file] Decompressing sid=%s file=%s', sid, rel_path)
        cmd = ["gdcmconv", "-w", "-X", "-R", str(source_file), str(dest_file)]
        
        try:
            result = subprocess.run(
                cmd, 
                check=True, 
                stdout=subprocess.PIPE, 
                stderr=subprocess.PIPE, 
                timeout=120
            )
            logger.info('[decompress-file] Success: %s -> %s', source_file, dest_file)
            return jsonify({
                'status': 'success',
                'file_path': f'decompressed/{rel_path}',
                'message': 'File decompressed successfully'
            }), 200
            
        except FileNotFoundError:
            # gdcmconv not available, copy file as fallback
            logger.warning('[decompress-file] gdcmconv not found, copying file instead')
            shutil.copy2(source_file, dest_file)
            return jsonify({
                'status': 'success',
                'file_path': f'decompressed/{rel_path}',
                'message': 'File copied (gdcmconv not available)'
            }), 200
            
        except subprocess.CalledProcessError as e:
            logger.error('[decompress-file] gdcmconv failed: %s', e.stderr.decode() if e.stderr else str(e))
            return jsonify({
                'error': 'decompression failed',
                'details': e.stderr.decode() if e.stderr else str(e)
            }), 500
            
        except subprocess.TimeoutExpired:
            logger.error('[decompress-file] Timeout decompressing %s', source_file)
            return jsonify({'error': 'decompression timeout'}), 500
            
    except Exception as e:
        logger.exception('[decompress-file] Unexpected error')
        return jsonify({'error': 'decompress file failed', 'details': str(e)}), 500


# ============================================================================
# PHASE 5: Binary File Decompression Endpoint
# ============================================================================

@app.route('/file/decompress', methods=['POST'])
def decompress_file_binary():
    """
    Decompress a single DICOM file and return raw binary data.
    Used by backend to stream decompressed files to frontend.
    
    Request body:
    {
        "sid": "series_id",
        "filename": "Image-00001"
    }
    
    Returns: Binary DICOM file data
    """
    try:
        data = request.get_json(force=True, silent=True) or {}
        sid = str(data.get('sid') or '').strip()
        filename = str(data.get('filename') or '').strip()
        
        if not sid:
            return jsonify({'error': 'sid required'}), 400
        if not filename:
            return jsonify({'error': 'filename required'}), 400
        
        logger.info('[file-decompress-binary] Request for sid=%s filename=%s', sid, filename)
        
        # Normalize filename
        filename = filename.replace('\\', '/').strip('/')
        
        # Find source file in compressed directory
        compressed_dir = _data_dir() / sid / 'compressed'
        source_file = compressed_dir / filename
        
        if not source_file.exists():
            logger.error('[file-decompress-binary] Source file not found: %s', source_file)
            return jsonify({'error': 'compressed file not found', 'filename': filename}), 404
        
        # Check if already decompressed (cached)
        decomp_dir = _data_dir() / sid / 'decompressed'
        dest_file = decomp_dir / filename
        
        if dest_file.exists():
            logger.info('[file-decompress-binary] 📖 Cache HIT for %s/%s', sid, filename)
            # Read and return cached file
            with open(dest_file, 'rb') as f:
                return send_file(io.BytesIO(f.read()), 
                               mimetype='application/dicom',
                               as_attachment=False,
                               download_name=filename)
        
        logger.info('[file-decompress-binary] 🔄 Cache MISS - Decompressing %s/%s', sid, filename)
        
        # Check if file is actually JPEG2000 compressed
        if not _is_jpeg2000_compressed(source_file):
            logger.info('[file-decompress-binary] File not compressed, returning as-is: %s', source_file)
            # File not compressed, read and return directly
            with open(source_file, 'rb') as f:
                return send_file(io.BytesIO(f.read()),
                               mimetype='application/dicom',
                               as_attachment=False,
                               download_name=filename)
        
        # Create decomp directory
        decomp_dir.mkdir(parents=True, exist_ok=True)
        
        # Decompress using gdcmconv with metadata preservation
        try:
            # CRITICAL FIX: Use explicit uncompressed transfer syntax to preserve metadata
            # -w: Write uncompressed pixel data  
            # -X: Explicit VR Little Endian (preserves all metadata including orientation)
            # -R: Preserve Rescale Slope/Intercept (0028,1052/1053) for correct Hounsfield Unit calculation
            # Without -X and -R, metadata tags may be lost causing incorrect rendering
            # Image Orientation Patient (0020,0037) and Slice Location (0020,1041) are preserved
            cmd = ["gdcmconv", "-w", "-X", "-R", str(source_file), str(dest_file)]
            logger.info('[file-decompress-binary] Running with metadata preservation: %s', ' '.join(cmd))
            
            result = subprocess.run(
                cmd,
                check=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                timeout=120
            )
            
            logger.info('[file-decompress-binary] ✅ Decompression success: %s', dest_file)
            
            # Read decompressed file and return
            with open(dest_file, 'rb') as f:
                decompressed_data = f.read()
                logger.info('[file-decompress-binary] 📦 Returning %d bytes for %s', len(decompressed_data), filename)
                return send_file(io.BytesIO(decompressed_data),
                               mimetype='application/dicom',
                               as_attachment=False,
                               download_name=filename)
                
        except FileNotFoundError:
            # gdcmconv not available, copy file as fallback
            logger.warning('[file-decompress-binary] gdcmconv not found, copying file')
            shutil.copy2(source_file, dest_file)
            with open(dest_file, 'rb') as f:
                return send_file(io.BytesIO(f.read()),
                               mimetype='application/dicom',
                               as_attachment=False,
                               download_name=filename)
                
        except subprocess.CalledProcessError as e:
            logger.error('[file-decompress-binary] gdcmconv failed: %s', 
                        e.stderr.decode() if e.stderr else str(e))
            return jsonify({
                'error': 'decompression failed',
                'details': e.stderr.decode() if e.stderr else str(e)
            }), 500
            
        except subprocess.TimeoutExpired:
            logger.error('[file-decompress-binary] Timeout decompressing %s', source_file)
            return jsonify({'error': 'decompression timeout'}), 500
            
    except Exception as e:
        logger.exception('[file-decompress-binary] Unexpected error')
        return jsonify({'error': 'decompress file failed', 'details': str(e)}), 500


