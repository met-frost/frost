import argparse
from datetime import datetime, timedelta, timezone
import json
import time
from uuid import uuid4

import requests

from FrostImportUtils import TEST_WRITE_TOKEN


BASE_URL = "http://localhost:8080"
MAX_SAFE_REQUEST_BYTES = 900_000
MAX_UNLIMITED_RESPONSE_SECONDS = 7_200
START_TIME = datetime(2020, 1, 1, tzinfo=timezone.utc)
HEADERS = {
    "Content-Type": "application/json",
    "X-Frost-Writetoken": TEST_WRITE_TOKEN,
}


def timestamp_at(offset_seconds):
    return (START_TIME + timedelta(seconds=offset_seconds)).strftime(
        "%Y-%m-%dT%H:%M:%SZ"
    )


def post_dataset(base_url, path, dataset):
    body = json.dumps(dataset, separators=(",", ":")).encode("utf-8")
    if len(body) > MAX_SAFE_REQUEST_BYTES:
        raise ValueError(
            f"request body is {len(body)} bytes; reduce batch size or series count "
            f"to stay below {MAX_SAFE_REQUEST_BYTES} bytes"
        )
    response = requests.post(
        f"{base_url}/api/v1/obs/sst/{path}",
        headers=HEADERS,
        data=body,
        timeout=120,
    )
    if response.status_code != 200:
        raise RuntimeError(
            f"POST {path} returned HTTP {response.status_code}: {response.text[:1000]}"
        )
    return response.json(), len(body)


def make_headers(source, series_count):
    return [
        {
            "id": {
                "source": source,
                "buoyid": f"{index:05d}",
                "parameter": "temperature",
            },
            "extra": {
                "name": f"Performance station {index:05d}",
                "pos": {"lon": "10.73", "lat": "59.84"},
            },
        }
        for index in range(series_count)
    ]


def run_benchmark(base_url, series_count, observations_per_series, batch_size):
    source = f"frost-e2e-perf-{uuid4().hex[:12]}"
    headers = make_headers(source, series_count)
    dataset_headers = {
        "tstype": "sst",
        "tseries": [
            {"header": header, "observations": []} for header in headers
        ],
    }

    create_start = time.perf_counter()
    create_result, _ = post_dataset(base_url, "ts/create", dataset_headers)
    create_seconds = time.perf_counter() - create_start
    if (
        create_result.get("rejected") != 0
        or create_result.get("accepted") != series_count
        or create_result.get("applied") != series_count
    ):
        raise RuntimeError(f"unexpected time-series create result: {create_result}")

    uploaded_observations = 0
    uploaded_bytes = 0
    upload_start = time.perf_counter()
    for batch_start in range(0, observations_per_series, batch_size):
        batch_end = min(batch_start + batch_size, observations_per_series)
        batch = {
            "tstype": "sst",
            "tseries": [
                {
                    "header": header,
                    "observations": [
                        {
                            "time": timestamp_at(offset),
                            "body": {"value": str(offset)},
                        }
                        for offset in range(batch_start, batch_end)
                    ],
                }
                for header in headers
            ],
        }
        result, body_bytes = post_dataset(base_url, "put", batch)
        batch_observations = series_count * (batch_end - batch_start)
        if (
            result.get("inserted") != batch_observations
            or result.get("updated") != 0
            or result.get("deleted") != 0
        ):
            raise RuntimeError(f"unexpected observation write result: {result}")
        uploaded_observations += batch_observations
        uploaded_bytes += body_bytes
    upload_seconds = time.perf_counter() - upload_start

    expected_times = [
        timestamp_at(offset) for offset in range(observations_per_series)
    ]
    retrieve_seconds = 0.0
    response_bytes = 0
    retrieved_observations = 0
    retrieval_requests = 0
    for header in headers:
        buoyid = header["id"]["buoyid"]
        for window_start in range(
            0, observations_per_series, MAX_UNLIMITED_RESPONSE_SECONDS
        ):
            window_end = min(
                window_start + MAX_UNLIMITED_RESPONSE_SECONDS,
                observations_per_series,
            )
            params = {
                "sources": source,
                "buoyids": buoyid,
                "parameters": "temperature",
                "incobs": "true",
                "time": f"{timestamp_at(window_start)}/{timestamp_at(window_end)}",
            }
            request_start = time.perf_counter()
            response = requests.get(
                f"{base_url}/api/v1/obs/sst/get", params=params, timeout=120
            )
            if response.status_code != 200:
                raise RuntimeError(
                    f"GET observations for buoy {buoyid} returned "
                    f"HTTP {response.status_code}: {response.text[:1000]}"
                )
            result = response.json()
            retrieve_seconds += time.perf_counter() - request_start
            if result.get("next") is not None:
                raise RuntimeError(
                    f"retrieval for buoy {buoyid} was paginated in window "
                    f"{window_start}:{window_end}"
                )
            retrieved = result.get("data", {}).get("tseries", [])
            if len(retrieved) != 1:
                raise RuntimeError(
                    f"expected one series for buoy {buoyid}, got {len(retrieved)}"
                )
            observations = retrieved[0].get("observations", [])
            actual_times = [observation["time"] for observation in observations]
            if actual_times != expected_times[window_start:window_end]:
                raise RuntimeError(
                    f"buoy {buoyid} returned unexpected timestamps or ordering "
                    f"in window {window_start}:{window_end}"
                )
            retrieved_observations += len(observations)
            response_bytes += len(response.content)
            retrieval_requests += 1
    print(f"source: {source}")
    print(
        f"ingested: {uploaded_observations:,} observations across {series_count} "
        f"series in {uploaded_bytes / 1_000_000:.2f} MB of upload bodies"
    )
    print(f"series creation: {create_seconds:.3f} s")
    print(
        f"client upload wall time (generation, encoding, and HTTP): "
        f"{upload_seconds:.3f} s "
        f"({uploaded_observations / upload_seconds:,.0f} observations/s)"
    )
    print(
        f"retrieval and JSON decoding: {retrieve_seconds:.3f} s "
        f"({retrieved_observations / retrieve_seconds:,.0f} observations/s; "
        f"{response_bytes / 1_000_000:.2f} MB across {retrieval_requests} requests)"
    )
    print(
        f"verified: {series_count} series / {retrieved_observations:,} observations"
    )
    print("timings are informational; no machine-dependent speed threshold is used")


def positive_int(value):
    parsed = int(value)
    if parsed < 1:
        raise argparse.ArgumentTypeError("must be a positive integer")
    return parsed


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Benchmark bulk SST ingestion and retrieval against local Frost."
    )
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--series-count", type=positive_int, default=10)
    parser.add_argument(
        "--observations-per-series", type=positive_int, default=9_000
    )
    parser.add_argument("--batch-size", type=positive_int, default=1_000)
    args = parser.parse_args()
    run_benchmark(
        args.base_url.rstrip("/"),
        args.series_count,
        args.observations_per_series,
        args.batch_size,
    )