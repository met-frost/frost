# WARNING: the below code has been generated exlusively using LLM-style AI (Microsoft Copilot)

import argparse
import copy
import json
from pathlib import Path
import time
import unittest

import requests

from ingest import csv_to_dataset
from FrostImportUtils import TEST_WRITE_TOKEN


FIXTURE = Path(__file__).parent / "test/examples/sst/input_week.csv"
SOURCE = "frost-e2e-week"
START = "2020-06-16T00:00:00Z"
END = "2020-06-23T00:00:00Z"
PARAMS = {
    "sources": SOURCE,
    "parameters": "temperature",
    "incobs": "true",
    "time": f"{START}/{END}",
    "inside": json.dumps(
        [{"type": "circle", "lon": 10.73, "lat": 59.84, "radius": 5}],
        separators=(",", ":"),
    ),
    "geopostype": "stationary",
    "itemlimit": "100",
}


class IngestionExamples(unittest.TestCase):
    base_url = "http://localhost:8080"

    @classmethod
    def setUpClass(cls):
        deadline = time.monotonic() + 30
        while True:
            try:
                response = requests.get(
                    f"{cls.base_url}/api/v1/obs/sst/get",
                    params={"sources": SOURCE, "incobs": "false"},
                    timeout=2,
                )
                if response.status_code == 200 or (
                    response.status_code == 404
                    and response.json().get("error") == "no matching time series found"
                ):
                    break
            except requests.RequestException:
                pass
            if time.monotonic() >= deadline:
                raise RuntimeError(f"Frost did not become ready at {cls.base_url}")
            time.sleep(0.5)

    def request(self, method, path, **kwargs):
        response = requests.request(
            method, f"{self.base_url}{path}", timeout=30, **kwargs
        )
        print(f"\n{response.request.method} {response.request.url}")
        if response.request.body:
            print(f"Content-Type: {response.request.headers['Content-Type']}")
            body = response.request.body
            print(body.decode() if isinstance(body, bytes) else body)
        print(f"HTTP {response.status_code}")
        return response

    def get_dataset(self, params, allow_empty=False):
        response = self.request("GET", "/api/v1/obs/sst/get", params=params)
        if allow_empty and response.status_code == 404:
            self.assertEqual(response.json()["error"], "no matching time series found")
            return {"tstype": "sst", "tseries": []}
        self.assertEqual(response.status_code, 200, response.text)
        body = response.json()
        self.assertIsNone(body.get("next"), "Expected one complete response page")
        return body["data"]

    def assert_dataset(self, actual, expected):
        self.assertEqual(actual["tstype"], "sst")
        actual_series = actual["tseries"] or []
        self.assertEqual(len(actual_series), len(expected))
        self.assertEqual(
            {series["header"]["id"]["buoyid"] for series in actual_series},
            set(expected),
        )
        for series in actual_series:
            header = series["header"]
            buoyid = header["id"]["buoyid"]
            with self.subTest(buoyid=buoyid):
                self.assertEqual(header["id"], expected[buoyid]["header"]["id"])
                self.assertEqual(header["extra"], expected[buoyid]["header"]["extra"])
                self.assertEqual(
                    series["observations"], expected[buoyid]["observations"]
                )

    def test_week_radius(self):
        dataset = csv_to_dataset(FIXTURE)
        self.assertEqual(len(dataset["tseries"]), 3)
        self.assertEqual(
            sum(len(series["observations"]) for series in dataset["tseries"]), 33
        )
        headers_only = copy.deepcopy(dataset)
        for series in headers_only["tseries"]:
            series["observations"] = []
        write_headers = {"X-Frost-Writetoken": TEST_WRITE_TOKEN}
        for path, payload in (
            ("/api/v1/obs/sst/ts/create", headers_only),
            ("/api/v1/obs/sst/put", dataset),
        ):
            response = self.request("POST", path, headers=write_headers, json=payload)
            self.assertEqual(response.status_code, 200, response.text)

        expected = {}
        for series in dataset["tseries"]:
            ordered = copy.deepcopy(series)
            ordered["observations"].sort(key=lambda observation: observation["time"])
            expected[series["header"]["id"]["buoyid"]] = ordered
        control_params = dict(PARAMS)
        del control_params["inside"]
        control_params["time"] = "2020-06-15T00:00:00Z/2020-06-24T00:00:00Z"
        self.assert_dataset(self.get_dataset(control_params), expected)

        weekly = copy.deepcopy(expected)
        for series in weekly.values():
            series["observations"] = [
                observation for observation in series["observations"]
                if START <= observation["time"] < END
            ]
            self.assertEqual(len(series["observations"]), 8)
            self.assertEqual(series["observations"][0]["time"], START)
            self.assertEqual(series["observations"][-1]["time"], "2020-06-22T23:59:59Z")
        control_params["time"] = PARAMS["time"]
        self.assert_dataset(self.get_dataset(control_params), weekly)
        del weekly["003"]
        self.assert_dataset(self.get_dataset(PARAMS), weekly)
        print("PASS: 3 series / 33 observations ingested; weekly circle: 2 / 16.")
        print("Interval [start, end); observations ascending; series order unspecified.")

    def test_invalid_structure(self):
        before = self.get_dataset(PARAMS, allow_empty=True)
        payload = {"tstype": "sst", "tseries": {"header": {}, "observations": []}}
        response = self.request(
            "POST", "/api/v1/obs/sst/put",
            headers={"X-Frost-Writetoken": TEST_WRITE_TOKEN},
            json=payload,
        )
        self.assertEqual(response.status_code, 400, response.text)
        error = response.json()
        error_text = json.dumps(error)
        self.assertIn("failed to validate request body", error_text)
        self.assertIn("tseries", error_text)
        self.assertIn("array", error_text)
        print(json.dumps(error, indent=2))
        after = self.get_dataset(PARAMS, allow_empty=True)
        expected = {
            series["header"]["id"]["buoyid"]: series
            for series in (before["tseries"] or [])
        }
        self.assert_dataset(after, expected)
        print("PASS: invalid tseries object rejected with HTTP 400; data unchanged.")

    def test_authorization_rejects_missing_and_invalid_tokens(self):
        payload = {"tstype": "sst", "tseries": []}
        requests_to_reject = (
            ("missing", {}),
            ("invalid", {"X-Frost-Writetoken": "AAAAAAAAAAAAAAAAAAAAAA=="}),
        )
        for label, headers in requests_to_reject:
            with self.subTest(token=label):
                response = self.request(
                    "POST", "/api/v1/obs/sst/put", headers=headers, json=payload
                )
                self.assertEqual(response.status_code, 401, response.text)
        print("PASS: missing and invalid write tokens rejected with HTTP 401.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "case",
        choices=("week", "invalid", "authorization", "all"),
        nargs="?",
        default="all",
    )
    parser.add_argument("--base-url", default="http://localhost:8080")
    args = parser.parse_args()
    IngestionExamples.base_url = args.base_url.rstrip("/")
    names = {
        "week": ["test_week_radius"],
        "invalid": ["test_invalid_structure"],
        "authorization": ["test_authorization_rejects_missing_and_invalid_tokens"],
        "all": [
            "test_week_radius",
            "test_invalid_structure",
            "test_authorization_rejects_missing_and_invalid_tokens",
        ],
    }[args.case]
    suite = unittest.TestSuite(IngestionExamples(name) for name in names)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    raise SystemExit(not result.wasSuccessful())