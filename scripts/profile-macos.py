#!/usr/bin/env python3
"""Launch an isolated profiling run, timestamp logs, and sample process RSS."""
import argparse
import datetime
import json
import pathlib
import os
import re
import statistics
import subprocess
import sys
import threading
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--seconds', type=float, default=0, help='stop this launched process after N seconds; default: until its windows close')
    parser.add_argument('--sample-after', type=float, default=0, help='exclude frames before N seconds from launch, e.g. a separate scrolling measurement')
    parser.add_argument('--summary', help='write a machine-readable JSON measurement summary')
    parser.add_argument('executable', help='MDFlick executable (not the .app directory)')
    parser.add_argument('documents', nargs='*')
    args = parser.parse_args()
    if sys.platform != 'darwin':
        parser.error('native profiling requires macOS')
    if args.seconds < 0 or args.sample_after < 0:
        parser.error('--seconds and --sample-after must be nonnegative')
    started = time.monotonic()
    output_lock = threading.Lock()
    frames = []
    presentations = []
    requests = []
    closed_heap = []
    memory = []
    forced_stop = False

    def emit(line):
        with output_lock:
            print(f'perf t={time.monotonic()-started:.6f}s {line}', flush=True)

    env = dict(os.environ, MDFLICK_TRACE='1', MYGO_FRAME_STATS='all')
    process = subprocess.Popen([os.path.abspath(args.executable), *map(os.path.abspath, args.documents)], env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    emit(f'launch pid={process.pid} utc={datetime.datetime.now(datetime.timezone.utc).isoformat()}')

    def read_logs():
        for line in process.stdout:
            line = line.rstrip()
            if 'stage=native.open-to-first-present ' in line:
                presentations.append(time.monotonic() - started)
            request = re.search(r'pipeline request=(\d+) stage=native.open-to-first-present elapsed=(\S+)', line)
            if request:
                requests.append({'request': int(request[1]), 'elapsed': request[2], 'observed_seconds_from_launch': time.monotonic()-started})
            closed = re.search(r'memory stage=native.closed .*heap_objects=(\d+) go_retained=(\d+)', line)
            if closed:
                closed_heap.append({'heap_objects': int(closed[1]), 'go_retained': int(closed[2])})
            match = re.search(r'mygo: frame \d+ took ([\d.]+) ms:', line)
            if match and time.monotonic()-started >= args.sample_after:
                frames.append(float(match.group(1)))
            emit(line)

    reader = threading.Thread(target=read_logs, daemon=True)
    reader.start()
    try:
        while process.poll() is None:
            # WebKit XPC processes may be reparented to launchd. Descendants
            # are explicitly a partial sample, never a whole-app memory total.
            rows = subprocess.check_output(['ps', '-axo', 'pid=,ppid=,rss=,comm='], text=True)
            entries = {}
            for row in rows.splitlines():
                fields = row.split(None, 3)
                if len(fields) == 4:
                    pid, parent, rss = map(int, fields[:3])
                    entries[pid] = (parent, rss * 1024, fields[3])
            descendants = {process.pid}
            while True:
                expanded = descendants | {pid for pid, (parent, _, _) in entries.items() if parent in descendants}
                if expanded == descendants:
                    break
                descendants = expanded
            host = entries.get(process.pid, (0, 0, ''))[1]
            child_rss = sum(entries[pid][1] for pid in descendants if pid != process.pid and pid in entries)
            memory.append({'seconds_from_launch': time.monotonic()-started, 'host_rss_bytes': host, 'descendant_rss_bytes': child_rss})
            emit(f'memory host_rss_bytes={host} descendant_rss_bytes={child_rss} descendants={len(descendants)-1}')
            if args.seconds and time.monotonic() - started >= args.seconds:
                forced_stop = True
                emit('stop reason=duration (not an orderly window-close measurement)')
                process.terminate()
                break
            time.sleep(0.5)
        process.wait(timeout=10)
    except KeyboardInterrupt:
        forced_stop = True
        emit('stop reason=interrupt')
        process.terminate()
    finally:
        if process.poll() is None:
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        reader.join(timeout=5)
    frame_summary = None
    if frames:
        ordered = sorted(frames)
        p95 = ordered[max(0, int(len(ordered)*0.95 + 0.999)-1)]
        frame_summary = {'samples': len(frames), 'p50_ms': statistics.median(frames), 'p95_ms': p95, 'max_ms': max(frames)}
        emit(f'frames samples={len(frames)} p50_ms={statistics.median(frames):.3f} p95_ms={p95:.3f} max_ms={max(frames):.3f} (frames after {args.sample_after}s; scrolling requires a separate run)')
    if args.summary:
        summary = {
            'exit_code': process.returncode, 'forced_stop': forced_stop,
            'documents': len(args.documents), 'frames_after_seconds': args.sample_after,
            'frames': frame_summary, 'presentations': requests,
            'host_peak_rss_bytes': max((sample['host_rss_bytes'] for sample in memory), default=0),
            'memory_samples': memory, 'window_close_heap_samples': closed_heap,
            'limits': ['RSS excludes reparented WebKit processes', 'close heap samples do not force GC or prove reclamation', 'presentation is surface submission, not scanout', 'frame labels do not establish that scrolling occurred'],
        }
        target = pathlib.Path(args.summary)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(json.dumps(summary, indent=2)+'\n')
    if args.documents and not presentations:
        emit('error: no document presentation recorded; use a MyGo-built executable with file-association metadata (see docs/performance.md)')
        return 1
    emit(f'exit code={process.returncode}')
    return 0 if process.returncode in (0, -15) else process.returncode


if __name__ == '__main__':
    sys.exit(main())
