#!/usr/bin/env python3
"""
Stress test for prism's /api/index endpoint.

Orchestrates:
  1. Two mock-nginx-index servers (deterministic random responses)
  2. prism built with -tags debug (pprof enabled)
  3. A test config.yaml pointing prism at the mocks
  4. Concurrent HTTP load against /api/index?path=<path>
  5. Metrics collection (latency percentiles, throughput, errors)
  6. Clean shutdown + pprof file collection

Usage:
  python3 test/stress-index/stress-index.py [--interval 30] [--concurrency 10]
"""

import argparse
import concurrent.futures
import itertools
import json
import os
import pathlib
import random
import shutil
import signal
import statistics
import string
import subprocess
import sys
import tempfile
import textwrap
import threading
import time
import urllib.request
from typing import Optional

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

PROJECT_ROOT = pathlib.Path(__file__).resolve().parent.parent.parent
MOCK_BINARY = PROJECT_ROOT / "test" / "mock-nginx-index" / "mock-nginx-index"
MOCK_SOURCE = PROJECT_ROOT / "test" / "mock-nginx-index"
PRISM_SOURCE = PROJECT_ROOT / "cmd" / "prism"

DEFAULT_MOCK_PORT_A = 9090
DEFAULT_MOCK_PORT_B = 9091
DEFAULT_PRISM_LISTEN = "127.0.0.1:8081"
DEFAULT_PRISM_PORT = 8081
DEFAULT_INTERVAL = 30
DEFAULT_CONCURRENCY = 10
DEFAULT_OUTPUT_DIR = PROJECT_ROOT / "stress-results"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def rand_path(rng: random.Random, depth: int = 0) -> str:
    """Generate a random path segment for stress testing."""
    if depth <= 0:
        depth = rng.randint(1, 4)
    parts = []
    for _ in range(depth):
        length = rng.randint(3, 12)
        segment = "".join(rng.choices(string.ascii_lowercase + string.digits, k=length))
        parts.append(segment)
    return "/" + "/".join(parts) + "/"


def generate_paths(hosts: list[str], count: int, seed: int = 42) -> list[str]:
    """Generate a reproducible list of query paths across hosts."""
    rng = random.Random(seed)
    paths = []
    for _ in range(count):
        host = rng.choice(hosts)
        subpath = rand_path(rng)
        paths.append(host + subpath)
    return paths


def build_test_config(mock_port_a: int, mock_port_b: int) -> str:
    """Generate a test config.yaml as a string."""
    return textwrap.dedent(f"""\
    log:
      level: warn
      output: stderr

    access_log:
      level: error
      output: stderr

    http:
      listen: "{DEFAULT_PRISM_LISTEN}"
      proto_header: "X-Forwarded-Proto"
      tcp_keepalive: true

    index:
      cache_ttl: 5m
      cache_max_bytes: 64MB

    sync_status:
      cache_ttl: 60s
      fetch_timeout: 1s

    hosts:
    - name: mock1
      fqdn: 127.0.0.1
      index:
        driver: nginx
        nginx:
          timeout: 3s
          base_url: http://127.0.0.1:{mock_port_a}/api/index/
      sync_status:
        driver: tunasync
        ttl: 60s
        tunasync:
          endpoint: http://127.0.0.1:1/tunasync.json
      mirrors:
      - name: mock1-root
        desc: "Mock Host 1"
        type: rsync
        url_prefix: /mock1/
        real_url_prefix: /mock1/
        help:
          mode: "off"

    - name: mock2
      fqdn: 127.0.0.1
      index:
        driver: nginx
        nginx:
          timeout: 3s
          base_url: http://127.0.0.1:{mock_port_b}/api/index/
      sync_status:
        driver: tunasync
        ttl: 60s
        tunasync:
          endpoint: http://127.0.0.1:1/tunasync.json
      mirrors:
      - name: mock2-root
        desc: "Mock Host 2"
        type: rsync
        url_prefix: /mock2/
        real_url_prefix: /mock2/
        help:
          mode: "off"
    """)


def http_get(url: str, timeout: float = 5.0) -> tuple[int, float]:
    """Perform an HTTP GET and return (status_code, elapsed_seconds)."""
    start = time.monotonic()
    try:
        with urllib.request.urlopen(url, timeout=timeout) as resp:
            # consume body
            _ = resp.read()
            return resp.status, time.monotonic() - start
    except urllib.error.HTTPError as e:
        return e.code, time.monotonic() - start
    except Exception:
        return 0, time.monotonic() - start


def health_check(url: str, timeout: float = 2.0, retries: int = 30, interval: float = 0.5) -> bool:
    """Poll a URL until it responds 200 or retries exhausted."""
    for _ in range(retries):
        try:
            with urllib.request.urlopen(url, timeout=timeout) as resp:
                if resp.status == 200:
                    return True
        except Exception:
            pass
        time.sleep(interval)
    return False


def kill_process(proc: subprocess.Popen, name: str) -> None:
    """Gracefully terminate a process, then force-kill if needed."""
    if proc is None or proc.poll() is not None:
        return
    print(f"  Stopping {name} (pid={proc.pid})...")
    proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        print(f"  Force-killing {name}...")
        proc.kill()
        proc.wait()


# ---------------------------------------------------------------------------
# Metrics
# ---------------------------------------------------------------------------


class MetricsCollector:
    """Thread-safe metrics aggregator."""

    def __init__(self):
        self._lock = threading.Lock()
        self.latencies: list[float] = []
        self.status_counts: dict[int, int] = {}
        self.total_requests = 0
        self.errors = 0  # non-2xx or connection errors

    def record(self, status: int, latency: float) -> None:
        with self._lock:
            self.total_requests += 1
            self.latencies.append(latency)
            self.status_counts[status] = self.status_counts.get(status, 0) + 1
            if status < 200 or status >= 300:
                self.errors += 1

    def snapshot(self) -> dict:
        with self._lock:
            total = self.total_requests
            if total == 0:
                return {"total_requests": 0}

            lats = sorted(self.latencies)
            p50_idx = int(len(lats) * 0.50)
            p95_idx = int(len(lats) * 0.95)
            p99_idx = int(len(lats) * 0.99)

            return {
                "total_requests": total,
                "errors": self.errors,
                "error_rate": self.errors / total if total else 0,
                "status_counts": dict(self.status_counts),
                "latency": {
                    "min_ms": round(lats[0] * 1000, 3),
                    "max_ms": round(lats[-1] * 1000, 3),
                    "mean_ms": round(statistics.mean(lats) * 1000, 3),
                    "p50_ms": round(lats[min(p50_idx, len(lats) - 1)] * 1000, 3),
                    "p95_ms": round(lats[min(p95_idx, len(lats) - 1)] * 1000, 3),
                    "p99_ms": round(lats[min(p99_idx, len(lats) - 1)] * 1000, 3),
                },
            }


# ---------------------------------------------------------------------------
# Stress runner
# ---------------------------------------------------------------------------


class StressRunner:
    """Manages the full stress-test lifecycle."""

    def __init__(
        self,
        interval: int,
        concurrency: int,
        mock_port_a: int,
        mock_port_b: int,
        prism_port: int,
        output_dir: pathlib.Path,
        paths_file: Optional[str],
        path_count: int,
    ):
        self.interval = interval
        self.concurrency = concurrency
        self.mock_port_a = mock_port_a
        self.mock_port_b = mock_port_b
        self.prism_port = prism_port
        self.prism_listen = f"127.0.0.1:{prism_port}"
        self.output_dir = output_dir
        self.paths_file = paths_file
        self.path_count = path_count

        self._mock_a: Optional[subprocess.Popen] = None
        self._mock_b: Optional[subprocess.Popen] = None
        self._prism: Optional[subprocess.Popen] = None
        self._tmpdir: Optional[tempfile.TemporaryDirectory] = None
        self._stop_event = threading.Event()
        self._metrics = MetricsCollector()

        # Override prism listen if port differs from default
        if prism_port != DEFAULT_PRISM_PORT:
            self.prism_listen = f"127.0.0.1:{prism_port}"

    # -- Process management --------------------------------------------------

    def _build_mock(self) -> pathlib.Path:
        """Ensure mock-nginx-index binary exists."""
        if MOCK_BINARY.exists():
            return MOCK_BINARY
        print(f"Building mock-nginx-index...")
        subprocess.run(
            ["go", "build", "-o", str(MOCK_BINARY), "."],
            cwd=str(MOCK_SOURCE),
            check=True,
        )
        return MOCK_BINARY

    def _build_prism(self) -> pathlib.Path:
        """Build prism with debug tags, return binary path."""
        binary = self.output_dir / "prism-debug"
        print(f"Building prism (debug) -> {binary}...")
        subprocess.run(
            ["go", "build", "-tags", "debug", "-o", str(binary), "./cmd/prism/"],
            cwd=str(PROJECT_ROOT),
            check=True,
        )
        return binary

    def _start_mock(self, name: str, port: int) -> subprocess.Popen:
        binary = self._build_mock()
        proc = subprocess.Popen(
            [str(binary), "-port", str(port)],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        print(f"  Started {name} on port {port} (pid={proc.pid})")
        return proc

    def _start_prism(self, binary: pathlib.Path) -> subprocess.Popen:
        tmp_path = pathlib.Path(self._tmpdir.name)  # type: ignore[union-attr]
        config_path = tmp_path / "config.yaml"
        config_path.write_text(self._config_yaml)
        proc = subprocess.Popen(
            [str(binary), "-config", str(config_path)],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            cwd=str(tmp_path),
        )
        print(f"  Started prism on {self.prism_listen} (pid={proc.pid})")
        return proc

    # -- Setup / teardown ----------------------------------------------------

    def setup(self) -> None:
        """Build binaries, write config, start all servers."""
        self.output_dir.mkdir(parents=True, exist_ok=True)

        # Build prism with debug tags
        prism_binary = self._build_prism()

        # Generate test config
        self._config_yaml = build_test_config(self.mock_port_a, self.mock_port_b)

        # Temp directory for prism's working dir (where pprof files land)
        self._tmpdir = tempfile.TemporaryDirectory(prefix="prism-stress-")

        # Start mock servers
        print("Starting mock nginx index servers...")
        self._mock_a = self._start_mock("mock1", self.mock_port_a)
        self._mock_b = self._start_mock("mock2", self.mock_port_b)

        # Health-check mocks
        if not health_check(f"http://127.0.0.1:{self.mock_port_a}/test/"):
            raise RuntimeError("mock1 failed to start")
        if not health_check(f"http://127.0.0.1:{self.mock_port_b}/test/"):
            raise RuntimeError("mock2 failed to start")
        print("  Mock servers healthy.")

        # Start prism
        print("Starting prism...")
        self._prism = self._start_prism(prism_binary)

        # Health-check prism
        if not health_check(f"http://{self.prism_listen}/api/ping"):
            raise RuntimeError("prism failed to start")
        print("  Prism healthy.")

        # Verify index endpoint works
        status, _ = http_get(f"http://{self.prism_listen}/api/index?path=/mock1/")
        if status != 200:
            raise RuntimeError(f"prism /api/index returned {status}")
        print("  /api/index verified.")

    def teardown(self, collect_profiles: bool = True) -> None:
        """Stop all servers gracefully."""
        print("\nShutting down...")
        # Stop prism first (SIGINT triggers deferred pprof writes)
        kill_process(self._prism, "prism")
        self._prism = None

        # Collect pprof files after prism has exited and flushed buffers
        if collect_profiles:
            self._collect_pprof()

        kill_process(self._mock_a, "mock1")
        self._mock_a = None
        kill_process(self._mock_b, "mock2")
        self._mock_b = None

        if self._tmpdir:
            self._tmpdir.cleanup()
            self._tmpdir = None

    # -- Stress test ---------------------------------------------------------

    def _worker(self, paths_cycle: itertools.cycle) -> None:
        """Worker thread: hit /api/index in a loop until stopped."""
        base_url = f"http://{self.prism_listen}/api/index?path="
        while not self._stop_event.is_set():
            path = next(paths_cycle)
            url = base_url + path
            status, latency = http_get(url)
            self._metrics.record(status, latency)

    def run(self) -> dict:
        """Run the stress test and return metrics snapshot."""
        # Load or generate paths
        if self.paths_file:
            with open(self.paths_file) as f:
                paths = [line.strip() for line in f if line.strip()]
        else:
            paths = generate_paths(
                ["/mock1", "/mock2"],
                self.path_count,
            )
        print(f"Using {len(paths)} unique paths.")
        paths_cycle = itertools.cycle(paths)

        print(f"\nStarting stress test: {self.interval}s, {self.concurrency} workers")
        print(f"Target: http://{self.prism_listen}/api/index?path=...\n")

        start_time = time.monotonic()

        with concurrent.futures.ThreadPoolExecutor(
            max_workers=self.concurrency
        ) as executor:
            futures = [
                executor.submit(self._worker, paths_cycle)
                for _ in range(self.concurrency)
            ]

            # Progress reporting
            try:
                last_reported = start_time
                while (elapsed := time.monotonic() - start_time) < self.interval:
                    time.sleep(1)
                    if time.monotonic() - last_reported >= 5:
                        snap = self._metrics.snapshot()
                        rate = (
                            snap["total_requests"] / elapsed
                            if elapsed > 0
                            else 0
                        )
                        print(
                            f"  [{elapsed:.0f}s] {snap['total_requests']} req, "
                            f"{rate:.1f} req/s, {snap['errors']} err"
                        )
                        last_reported = time.monotonic()
            except KeyboardInterrupt:
                print("\n  Interrupted — stopping workers...")

            self._stop_event.set()

            # Wait for all workers to finish
            concurrent.futures.wait(futures, timeout=10)

        elapsed = time.monotonic() - start_time
        metrics = self._metrics.snapshot()
        metrics["duration_sec"] = round(elapsed, 2)
        metrics["throughput_req_per_sec"] = round(
            metrics["total_requests"] / elapsed, 1
        ) if elapsed > 0 else 0
        metrics["concurrency"] = self.concurrency

        return metrics

    # -- Report --------------------------------------------------------------

    def report(self, metrics: dict) -> None:
        """Print a summary and write full JSON report."""
        print("\n" + "=" * 60)
        print("STRESS TEST RESULTS")
        print("=" * 60)

        if metrics["total_requests"] == 0:
            print("  No requests completed.")
            return

        print(f"  Duration:           {metrics['duration_sec']}s")
        print(f"  Concurrency:        {metrics['concurrency']}")
        print(f"  Total requests:     {metrics['total_requests']}")
        print(f"  Throughput:         {metrics['throughput_req_per_sec']} req/s")
        print(f"  Errors:             {metrics['errors']} "
              f"({metrics['error_rate']*100:.2f}%)")
        print()
        print("  Status distribution:")
        for code, count in sorted(metrics["status_counts"].items()):
            label = "connection error" if code == 0 else f"HTTP {code}"
            print(f"    {label:>20s}: {count}")

        lat = metrics["latency"]
        print()
        print("  Latency (ms):")
        print(f"    min: {lat['min_ms']:>10.3f}")
        print(f"    max: {lat['max_ms']:>10.3f}")
        print(f"    mean:{lat['mean_ms']:>10.3f}")
        print(f"    p50: {lat['p50_ms']:>10.3f}")
        print(f"    p95: {lat['p95_ms']:>10.3f}")
        print(f"    p99: {lat['p99_ms']:>10.3f}")

        print("=" * 60)

        # Write JSON report
        report_path = self.output_dir / "report.json"
        report_path.write_text(json.dumps(metrics, indent=2))
        print(f"\nFull report: {report_path}")

    # -- Pprof collection ----------------------------------------------------

    def _collect_pprof(self) -> None:
        """Copy pprof files from the temp working directory to output.

        Must be called AFTER prism has exited (so deferred writes have flushed).
        """
        if not self._tmpdir:
            return
        tmp_path = pathlib.Path(self._tmpdir.name)
        for pattern in ["*-cpu.pprof", "*-mem.pprof"]:
            for f in tmp_path.glob(pattern):
                dest = self.output_dir / f.name
                shutil.copy2(f, dest)
                size = dest.stat().st_size
                print(f"  Collected profile: {dest} ({size} bytes)")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Stress test prism's /api/index endpoint",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=textwrap.dedent("""\
            Examples:
              %(prog)s --interval 30 --concurrency 10
              %(prog)s --interval 60 --concurrency 50 --paths-file paths.txt
              %(prog)s --interval 10 --concurrency 4 --output-dir ./results
        """),
    )
    parser.add_argument(
        "--interval", "-t",
        type=int,
        default=DEFAULT_INTERVAL,
        help=f"Stress test duration in seconds (default: {DEFAULT_INTERVAL})",
    )
    parser.add_argument(
        "--concurrency", "-c",
        type=int,
        default=DEFAULT_CONCURRENCY,
        help=f"Number of concurrent workers (default: {DEFAULT_CONCURRENCY})",
    )
    parser.add_argument(
        "--mock-port-a",
        type=int,
        default=DEFAULT_MOCK_PORT_A,
        help=f"Port for mock server A (default: {DEFAULT_MOCK_PORT_A})",
    )
    parser.add_argument(
        "--mock-port-b",
        type=int,
        default=DEFAULT_MOCK_PORT_B,
        help=f"Port for mock server B (default: {DEFAULT_MOCK_PORT_B})",
    )
    parser.add_argument(
        "--prism-port",
        type=int,
        default=DEFAULT_PRISM_PORT,
        help=f"Port for prism (default: {DEFAULT_PRISM_PORT})",
    )
    parser.add_argument(
        "--output-dir", "-o",
        type=pathlib.Path,
        default=DEFAULT_OUTPUT_DIR,
        help=f"Output directory for reports and profiles (default: {DEFAULT_OUTPUT_DIR})",
    )
    parser.add_argument(
        "--paths-file", "-f",
        type=str,
        default=None,
        help="File with paths to query (one per line). If omitted, random paths are generated.",
    )
    parser.add_argument(
        "--path-count", "-n",
        type=int,
        default=200,
        help="Number of random paths to generate (default: 200)",
    )
    args = parser.parse_args()

    runner = StressRunner(
        interval=args.interval,
        concurrency=args.concurrency,
        mock_port_a=args.mock_port_a,
        mock_port_b=args.mock_port_b,
        prism_port=args.prism_port,
        output_dir=args.output_dir,
        paths_file=args.paths_file,
        path_count=args.path_count,
    )

    # Handle Ctrl-C gracefully during setup
    def _sig_handler(signum, frame):
        print("\nInterrupted during setup — cleaning up...")
        runner.teardown()
        sys.exit(1)

    signal.signal(signal.SIGINT, _sig_handler)

    try:
        runner.setup()

        # Restore default SIGINT for stress phase (KeyboardInterrupt in run())
        signal.signal(signal.SIGINT, signal.SIG_DFL)

        metrics = runner.run()
        runner.report(metrics)
    except Exception as e:
        print(f"\nERROR: {e}", file=sys.stderr)
        sys.exit(1)
    finally:
        runner.teardown()


if __name__ == "__main__":
    main()
