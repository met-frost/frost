import csv
import json
import sys

import requests


def fetch_dataset(frost_api_base, params=None):
    """
    Fetch observations from the badevann GET endpoint.
    Returns the dataset dict (same shape as dataset.json).
    """
    url = f"{frost_api_base}/api/v1/obs/badevann/get"
    default_params = {"incobs": "true"}
    if params:
        default_params.update(params)

    print(f"Fetching from {url} with params {default_params} ...")
    r = requests.get(url, params=default_params)

    if r.status_code != 200:
        try:
            detail = r.json()
        except Exception:
            detail = r.text
        raise Exception(
            f"Request failed with status {r.status_code}: {detail}"
        )

    body = r.json()
    # Response wraps the dataset under a "data" key.
    return body.get("data", body)


def dataset_to_csv(dataset, csv_path):
    """
    Flatten a dataset dict (same shape as dataset.json) into CSV rows,
    one row per observation, with columns matching input_example.csv.
    """
    fieldnames = ["source", "buoyid", "parameter", "name", "lon", "lat", "time", "value"]

    rows = []
    for ts in dataset.get("tseries", []):
        hdr_id = ts.get("header", {}).get("id", {})
        source = hdr_id.get("source", "")
        buoyid = hdr_id.get("buoyid", "")
        parameter = hdr_id.get("parameter", "")

        extra = ts.get("header", {}).get("extra") or {}
        name = extra.get("name", "")
        pos = extra.get("pos") or {}
        lon = pos.get("lon", "")
        lat = pos.get("lat", "")

        for obs in ts.get("observations", []):
            body = obs.get("body") or {}
            rows.append({
                "source": source,
                "buoyid": buoyid,
                "parameter": parameter,
                "name": name,
                "lon": lon,
                "lat": lat,
                "time": obs.get("time", ""),
                "value": body.get("value", ""),
            })

    with open(csv_path, "w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)

    print(f"Wrote {len(rows)} observation rows to {csv_path}")


def retrieve(frost_api_base, csv_path, params=None):
    dataset = fetch_dataset(frost_api_base, params)
    dataset_to_csv(dataset, csv_path)
    return dataset


if __name__ == "__main__":
    frost_api_base = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080"
    out_path = sys.argv[2] if len(sys.argv) > 2 else "output.csv"

    # Optional extra query params can be passed as key=value pairs after the output path.
    # Example: buoyids=011 time=2020-06-16T00:00:00Z/2020-06-17T00:00:00Z
    extra_params = {}
    for arg in sys.argv[3:]:
        if "=" in arg:
            k, v = arg.split("=", 1)
            extra_params[k] = v

    retrieve(frost_api_base, out_path, extra_params or None)
