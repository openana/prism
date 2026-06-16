#!/usr/bin/env python3
"""
Stress test for prism's /api/mirrors endpoint.

Orchestrates:
  1. An in-process Python HTTP server serving tunasync.json as a mock tunasync API
  2. prism built with -tags debug (pprof enabled)
  3. A test config.yaml pointing prism at the mock tunasync server
  4. Concurrent HTTP load against /api/mirrors
  5. Metrics collection (latency percentiles, throughput, errors)
  6. Clean shutdown + pprof file collection

Usage:
  python3 test/stress-mirrors/stress-mirrors.py [--interval 30] [--concurrency 10]
"""

import argparse
import concurrent.futures
import datetime
import json
import os
import pathlib
import shutil
import signal
import statistics
import subprocess
import sys
import tempfile
import textwrap
import threading
import time
import urllib.request
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Optional

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

PROJECT_ROOT = pathlib.Path(__file__).resolve().parent.parent.parent
PRISM_SOURCE = PROJECT_ROOT / "cmd" / "prism"
TUNAYNC_JSON = pathlib.Path(__file__).resolve().parent / "testdata" / "tunasync.json"

DEFAULT_TUNAYNC_PORT = 9090
DEFAULT_PRISM_LISTEN = "127.0.0.1:8081"
DEFAULT_PRISM_PORT = 8081
DEFAULT_INTERVAL = 30
DEFAULT_CONCURRENCY = 10

# ---------------------------------------------------------------------------
# Mirror names used in config (subset of what tunasync.json returns).
# Each host gets its own subset so both host fetches contribute mirrors.
# ---------------------------------------------------------------------------

MOCK_MIRRORS_HOST1 = [
    ("alpine", "Alpine Linux"),
    ("archlinux", "Arch Linux"),
    ("centos", "CentOS"),
    ("debian", "Debian"),
    ("fedora", "Fedora"),
    ("gentoo-portage", "Gentoo Portage"),
    ("gnu", "GNU"),
    ("manjaro", "Manjaro"),
    ("openwrt", "OpenWrt"),
    ("ubuntu", "Ubuntu"),
]

MOCK_MIRRORS_HOST2 = [
    ("centos-vault", "CentOS Vault"),
    ("deepin", "deepin"),
    ("epel", "EPEL"),
    ("kali-images", "Kali Images"),
    ("raspbian", "Raspbian"),
    ("rocky", "Rocky Linux"),
    ("ros", "ROS"),
    ("ros2", "ROS 2"),
    ("ubuntu-ports", "Ubuntu Ports"),
    ("vim", "Vim"),
]


def _mirrors_yaml(mirrors: list[tuple[str, str]], url_prefix_base: str) -> str:
    """Generate YAML lines for a list of (name, desc) mirror entries.

    Returns content with 0 base-indent so it can be indented uniformly by
    the caller via ``textwrap.indent``.
    """
    lines = []
    for name, desc in mirrors:
        lines.append(
            f"- name: {name}\n"
            f'  desc: "{desc}"\n'
            f"  type: rsync\n"
            f"  url_prefix: {url_prefix_base}/{name}/\n"
            f"  real_url_prefix: {url_prefix_base}/{name}/\n"
            f"  help:\n"
            f'    mode: "off"'
        )
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


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


def health_check(
    url: str,
    timeout: float = 2.0,
    retries: int = 30,
    interval: float = 0.5,
) -> bool:
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


def timestamp() -> str:
    """Return a compact ISO-8601 timestamp for directory naming."""
    return datetime.datetime.now().strftime("%Y%m%dT%H%M%S")


# ---------------------------------------------------------------------------
# Mock tunasync HTTP server (in-process, daemon thread)
# ---------------------------------------------------------------------------


class _TunasyncHandler(BaseHTTPRequestHandler):
    """Serve tunasync.json for every GET request."""

    tunasync_data: bytes = b"[]"

    def do_GET(self) -> None:
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(self.tunasync_data)))
        self.end_headers()
        self.wfile.write(self.tunasync_data)

    def log_message(self, format, *args):
        # Suppress access logs
        pass


class TunasyncMockServer:
    """A mock tunasync HTTP server running in a daemon thread."""

    def __init__(self, port: int, data_path: pathlib.Path):
        self._port = port
        self._data_path = data_path
        self._server: Optional[HTTPServer] = None
        self._thread: Optional[threading.Thread] = None

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self._port}/tunasync.json"

    def start(self) -> None:
        # Load tunasync JSON
        data = self._data_path.read_bytes()

        # Create a custom handler class with the pre-loaded data
        handler = type(
            "_TunasyncHandler",
            (_TunasyncHandler,),
            {"tunasync_data": data},
        )

        self._server = HTTPServer(("127.0.0.1", self._port), handler)
        self._thread = threading.Thread(target=self._server.serve_forever, daemon=True)
        self._thread.start()
        print(f"  Started mock tunasync on port {self._port}")

    def stop(self) -> None:
        if self._server is not None:
            self._server.shutdown()
            self._server.server_close()
            self._server = None
        if self._thread is not None:
            self._thread.join(timeout=2)
            self._thread = None


# ---------------------------------------------------------------------------
# Config generation
# ---------------------------------------------------------------------------


def build_test_config(mock_tunasync_port: int) -> str:
    """Generate a test config.yaml as a string."""
    endpoint = f"http://127.0.0.1:{mock_tunasync_port}/tunasync.json"

    template = textwrap.dedent(f"""\
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
      cache_ttl: 10s
      fetch_timeout: 5s

    hosts:
    - name: mock1
      fqdn: 127.0.0.1
      index:
        driver: nginx
        nginx:
          timeout: 3s
          base_url: http://127.0.0.1:1/api/index/
      sync_status:
        driver: tunasync
        ttl: 60s
        tunasync:
          endpoint: {endpoint}
          timeout: 5s
      mirrors:
    __HOST1_MIRRORS__
    - name: mock2
      fqdn: 127.0.0.1
      index:
        driver: nginx
        nginx:
          timeout: 3s
          base_url: http://127.0.0.1:1/api/index/
      sync_status:
        driver: tunasync
        ttl: 60s
        tunasync:
          endpoint: {endpoint}
          timeout: 5s
      mirrors:
    __HOST2_MIRRORS__
    """)

    # After dedent, ``mirrors:`` sits at 2-space indent under the host
    # block.  Mirror list items need 4-space indent (2 more).
    host1 = textwrap.indent(
        _mirrors_yaml(MOCK_MIRRORS_HOST1, "/mock1"), "    "
    )
    host2 = textwrap.indent(
        _mirrors_yaml(MOCK_MIRRORS_HOST2, "/mock2"), "    "
    )
    return template.replace("__HOST1_MIRRORS__", host1).replace(
        "__HOST2_MIRRORS__", host2
    )


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
        tunasync_port: int,
        prism_port: int,
        output_dir: pathlib.Path,
    ):
        self.interval = interval
        self.concurrency = concurrency
        self.tunasync_port = tunasync_port
        self.prism_port = prism_port
        self.prism_listen = f"127.0.0.1:{prism_port}"
        self.output_dir = output_dir

        self._tunasync_mock: Optional[TunasyncMockServer] = None
        self._prism: Optional[subprocess.Popen] = None
        self._tmpdir: Optional[pathlib.Path] = None
        self._stop_event = threading.Event()
        self._metrics = MetricsCollector()

    # -- Process management --------------------------------------------------

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

    def _start_prism(self, binary: pathlib.Path) -> subprocess.Popen:
        assert self._tmpdir is not None
        config_path = self._tmpdir / "config.yaml"
        config_path.write_text(self._config_yaml)
        proc = subprocess.Popen(
            [str(binary), "-config", str(config_path)],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            cwd=str(self._tmpdir),
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
        self._config_yaml = build_test_config(self.tunasync_port)

        # Temp directory for prism's working dir (where pprof files land)
        self._tmpdir = pathlib.Path(
            tempfile.mkdtemp(prefix="prism-stress-")
        )

        # Start mock tunasync server (in-process, daemon thread)
        print("Starting mock tunasync server...")
        self._tunasync_mock = TunasyncMockServer(self.tunasync_port, TUNAYNC_JSON)
        self._tunasync_mock.start()

        # Health-check mock tunasync
        if not health_check(self._tunasync_mock.url):
            raise RuntimeError("mock tunasync failed to start")
        print("  Mock tunasync healthy.")

        # Start prism
        print("Starting prism...")
        self._prism = self._start_prism(prism_binary)

        # Health-check prism
        if not health_check(f"http://{self.prism_listen}/api/ping"):
            raise RuntimeError("prism failed to start")
        print("  Prism healthy.")

        # Warm up: hit /api/mirrors so the cache is populated (blocks until fetch completes)
        print("  Warming cache (GET /api/mirrors)...")
        status, elapsed = http_get(
            f"http://{self.prism_listen}/api/mirrors", timeout=10.0
        )
        if status != 200:
            raise RuntimeError(f"prism /api/mirrors returned {status}")
        print(f"  /api/mirrors warm-up OK ({elapsed*1000:.1f}ms)")

    def teardown(self, collect_profiles: bool = True) -> None:
        """Stop all servers gracefully."""
        print("\nShutting down...")

        # Stop prism first (SIGINT triggers deferred pprof writes)
        kill_process(self._prism, "prism")
        self._prism = None

        # Collect pprof files after prism has exited and flushed buffers
        if collect_profiles:
            self._collect_pprof()

        # Stop mock tunasync
        if self._tunasync_mock is not None:
            self._tunasync_mock.stop()
            self._tunasync_mock = None

        if self._tmpdir:
            shutil.rmtree(self._tmpdir, ignore_errors=True)
            self._tmpdir = None

    # -- Stress test ---------------------------------------------------------

    def _worker(self) -> None:
        """Worker thread: hit /api/mirrors in a loop until stopped."""
        url = f"http://{self.prism_listen}/api/mirrors"
        while not self._stop_event.is_set():
            status, latency = http_get(url)
            self._metrics.record(status, latency)

    def run(self) -> dict:
        """Run the stress test and return metrics snapshot."""
        print(
            f"\nStarting stress test: {self.interval}s, "
            f"{self.concurrency} workers"
        )
        print(f"Target: http://{self.prism_listen}/api/mirrors\n")

        start_time = time.monotonic()

        with concurrent.futures.ThreadPoolExecutor(
            max_workers=self.concurrency
        ) as executor:
            futures = [
                executor.submit(self._worker)
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
        print(
            f"  Throughput:         {metrics['throughput_req_per_sec']} req/s"
        )
        print(
            f"  Errors:             {metrics['errors']} "
            f"({metrics['error_rate']*100:.2f}%)"
        )
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
        for pattern in ["*.pprof"]:
            for f in self._tmpdir.glob(pattern):
                dest = self.output_dir / f.name
                shutil.copy2(f, dest)
                size = dest.stat().st_size
                print(f"  Collected profile: {dest} ({size} bytes)")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main() -> None:
    ts = timestamp()

    parser = argparse.ArgumentParser(
        description="Stress test prism's /api/mirrors endpoint",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=textwrap.dedent("""\
            Examples:
              %(prog)s --interval 30 --concurrency 10
              %(prog)s --interval 60 --concurrency 50
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
        "--tunasync-port",
        type=int,
        default=DEFAULT_TUNAYNC_PORT,
        help=f"Port for mock tunasync server (default: {DEFAULT_TUNAYNC_PORT})",
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
        default=PROJECT_ROOT / f"stress-mirrors-results-{ts}",
        help=f"Output directory for reports and profiles "
             f"(default: stress-mirrors-results-<timestamp>)",
    )
    args = parser.parse_args()

    runner = StressRunner(
        interval=args.interval,
        concurrency=args.concurrency,
        tunasync_port=args.tunasync_port,
        prism_port=args.prism_port,
        output_dir=args.output_dir,
    )

    # Handle Ctrl-C gracefully during setup
    def _sig_handler(signum, frame):
        print("\nInterrupted during setup — cleaning up...")
        runner.teardown(collect_profiles=False)
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
