import csv
import sys
from datetime import datetime


REQUIRED_COLUMNS = {
    "source",
    "buoyid",
    "parameter",
    "name",
    "lon",
    "lat",
    "time",
    "value",
}


def _parse_time(time_value):
    # Accept timestamps like 2020-06-16T06:40:00Z.
    return datetime.fromisoformat(time_value.replace("Z", "+00:00"))


def _read_rows(path):
    with open(path, "r", encoding="utf-8", newline="") as f:
        reader = csv.DictReader(f)
        fieldnames = set(reader.fieldnames or [])
        missing = sorted(REQUIRED_COLUMNS - fieldnames)
        if missing:
            raise ValueError(
                f"{path} is missing required columns: {', '.join(missing)}"
            )
        return list(reader)


def _latest_by_buoyid(rows):
    latest = {}
    for row in rows:
        buoyid = (row.get("buoyid") or "").strip()
        time_value = (row.get("time") or "").strip()

        if not buoyid or not time_value:
            continue

        current_time = _parse_time(time_value)
        previous = latest.get(buoyid)
        if previous is None:
            latest[buoyid] = row
            continue

        previous_time = _parse_time((previous.get("time") or "").strip())
        if current_time >= previous_time:
            latest[buoyid] = row

    return latest


def _map_output_by_buoyid(rows):
    mapped = {}
    duplicates = []

    for row in rows:
        buoyid = (row.get("buoyid") or "").strip()
        if not buoyid:
            continue

        if buoyid in mapped:
            duplicates.append(buoyid)
            continue

        mapped[buoyid] = row

    return mapped, duplicates


def verify(input_csv_path, output_csv_path):
    input_rows = _read_rows(input_csv_path)
    output_rows = _read_rows(output_csv_path)

    expected = _latest_by_buoyid(input_rows)
    actual, duplicates = _map_output_by_buoyid(output_rows)

    errors = []

    if duplicates:
        unique_dups = sorted(set(duplicates))
        errors.append(
            "Duplicate buoyid rows in output: " + ", ".join(unique_dups)
        )

    expected_ids = set(expected.keys())
    actual_ids = set(actual.keys())

    missing_ids = sorted(expected_ids - actual_ids)
    if missing_ids:
        errors.append("Missing buoyid in output: " + ", ".join(missing_ids))

    extra_ids = sorted(actual_ids - expected_ids)
    if extra_ids:
        errors.append("Unexpected buoyid in output: " + ", ".join(extra_ids))

    columns_to_compare = [
        "source",
        "buoyid",
        "parameter",
        "name",
        "lon",
        "lat",
        "time",
        "value",
    ]

    for buoyid in sorted(expected_ids & actual_ids):
        exp_row = expected[buoyid]
        act_row = actual[buoyid]

        for column in columns_to_compare:
            exp_val = (exp_row.get(column) or "").strip()
            act_val = (act_row.get(column) or "").strip()
            if exp_val != act_val:
                errors.append(
                    f"Mismatch for buoyid {buoyid}, column '{column}': "
                    f"expected '{exp_val}', got '{act_val}'"
                )

    if errors:
        print("VERIFICATION FAILED")
        for err in errors:
            print(f"- {err}")
        return False

    print("VERIFICATION PASSED")
    print(
        f"output matches latest observations for {len(expected_ids)} buoyid(s) "
        f"from {input_csv_path}"
    )
    return True


if __name__ == "__main__":
    input_csv = sys.argv[1] if len(sys.argv) > 1 else "input_example.csv"
    output_csv = sys.argv[2] if len(sys.argv) > 2 else "output.csv"

    ok = verify(input_csv, output_csv)
    sys.exit(0 if ok else 1)