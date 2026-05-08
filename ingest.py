import csv
import json
import sys
from collections import OrderedDict

from FrostImportUtils import FrostImportUtils as frost


REQUIRED_COLUMNS = {"source", "buoyid", "parameter", "time", "value"}


def _clean(value):
    if value is None:
        return ""
    return value.strip()


def _validate_headers(fieldnames):
    fieldnames = fieldnames or []
    missing = sorted(REQUIRED_COLUMNS - set(fieldnames))
    if missing:
        raise ValueError(
            f"Missing required CSV columns: {', '.join(missing)}. "
            f"Found: {', '.join(fieldnames)}"
        )


def _extra_from_row(row):
    name = _clean(row.get("name"))
    lon = _clean(row.get("lon"))
    lat = _clean(row.get("lat"))

    if not any([name, lon, lat]):
        return None

    extra = {}
    if name:
        extra["name"] = name
    if lon or lat:
        pos = {}
        if lon:
            pos["lon"] = lon
        if lat:
            pos["lat"] = lat
        extra["pos"] = pos

    return extra if extra else None


def csv_to_dataset(csv_path):
    grouped = OrderedDict()

    with open(csv_path, "r", encoding="utf-8", newline="") as f:
        reader = csv.DictReader(f)
        _validate_headers(reader.fieldnames)

        for row in reader:
            source = _clean(row.get("source"))
            buoyid = _clean(row.get("buoyid"))
            parameter = _clean(row.get("parameter"))
            key = (source, buoyid, parameter)

            if key not in grouped:
                grouped[key] = {
                    "header": {
                        "id": {
                            "source": source,
                            "buoyid": buoyid,
                            "parameter": parameter,
                        },
                        "extra": _extra_from_row(row),
                    },
                    "observations": [],
                }

            time_value = _clean(row.get("time"))
            obs_value = _clean(row.get("value"))

            if time_value and obs_value:
                grouped[key]["observations"].append(
                    {
                        "time": time_value,
                        "body": {"value": obs_value},
                    }
                )

    return {
        "tstype": "badevann",
        "tseries": list(grouped.values()),
    }


def ingest_csv(csv_path):
    dataset = csv_to_dataset(csv_path)
    
    frost.create_timeseries(frost_api_base, dataset)
    frost.upload_dataset(frost_api_base, dataset)
    
    return dataset


if __name__ == "__main__":
    frost_api_base = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080"
    in_path = sys.argv[2] if len(sys.argv) > 2 else "input_example.csv"
    dataset = ingest_csv(in_path)
    print(
        f"Ingested dataset with {len(dataset['tseries'])} tseries "
        f"from {in_path}"
    )