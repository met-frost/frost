# list recipes
default:
    @just --list

set positional-arguments

test_compose := "docker compose -f docker-compose.yml -f docker-compose.test.yml"

# build the docker containers (Frost and PSB)
build:
    #BUILDKIT_PROGRESS=plain docker compose build --no-cache
    docker compose build

# run the docker containers
run: build
    docker compose up -d

# run all ingestion and retrieval validation cases
test: test-roundtrip test-week test-invalid-ingestion test-authorization

# CSV ingestion and latest-observation round-trip validation
test-roundtrip: run-test
    python ingest.py  # inserts data to PSB via Frost
    python retrieve.py http://localhost:8080 output.csv sources=badevann.no buoyids=004,011 parameters=temperature
    python verify.py  # verifies that input data matches output data

# fixed one-week ingestion and radius-filtered end-to-end validation
test-week: run-test
    python test_ingestion.py week

# malformed ingestion must fail with HTTP 400 without changing observations
test-invalid-ingestion: run-test
    python test_ingestion.py invalid

# valid-token writes succeed; missing and invalid tokens are rejected
test-authorization: run-test
    python test_ingestion.py authorization

# opt-in bulk ingestion and retrieval benchmark (writes 90,000 observations)
test-performance: run-test
    python test_performance.py

# run services with test-only write restrictions enabled
run-test: build
    {{test_compose}} up -d --wait

# remove the docker containers
destroy:
    {{test_compose}} down

# recipes for platform-neutral execution of Frost examples from the paper
# Prerequisites: Python (v3.11+), Docker Engine, Docker Compose

# Build and run the infrastructure
build-and-run:
    docker compose build
    docker compose up -d
    docker ps -a

# Verify that the initial database tables are created (Section 3.1)
verify-tables:
    docker exec psb psql -U postgres -d datasets -c "\dt"

# Initialize an empty time series header of type 'sst' (Section 3.1)
create-sst-header:
    @python3 -c '\
    import urllib.request, json; \
    url = "http://localhost:8080/api/v1/obs/sst/ts/create"; \
    dataset = {"tstype": "sst", "tseries": [{"header": {"id": {"source": "badevann.no", "buoyid": "004", "parameter": "temperature"}, "extra": {"name": "Oslofjorden", "pos": {"lon": "10.73", "lat": "59.84"}}}, "observations": [{"time": "2026-08-21T13:04:58Z", "body": {"value": "8.72"}}]}]}; \
    req = urllib.request.Request(url, data=json.dumps(dataset).encode(), headers={"Content-Type": "application/json"}, method="POST"); \
    print(urllib.request.urlopen(req).read().decode())'

# Check the contents of the 'ts' metadata table (Section 3.1)
view-ts-table:
    docker exec psb psql -U postgres -d datasets -c "SELECT * FROM ts;"

# Ingest a single observation into the storage backend (Section 3.1)
put-sst-observation:
    @python3 -c '\
    import urllib.request, json; \
    url = "http://localhost:8080/api/v1/obs/sst/put"; \
    dataset = {"tstype": "sst", "tseries": [{"header": {"id": {"source": "badevann.no", "buoyid": "004", "parameter": "temperature"}, "extra": {"name": "Oslofjorden", "pos": {"lon": "10.73", "lat": "59.84"}}}, "observations": [{"time": "2026-08-21T13:04:58Z", "body": {"value": "8.72"}}]}]}; \
    req = urllib.request.Request(url, data=json.dumps(dataset).encode(), headers={"Content-Type": "application/json"}, method="POST"); \
    print(urllib.request.urlopen(req).read().decode())'

# Check the contents of the 'obs' observation data table (Section 3.1)
view-obs-table:
    docker exec psb psql -U postgres -d datasets -c "SELECT * FROM obs;"

# Retrieve the ingested observation via a GET query (Section 3.1)
get-sst-observation:
    @python3 -c '\
    import urllib.request; \
    url = "http://localhost:8080/api/v1/obs/sst/get?incobs=true&time=2026-08-21T13:04:58Z/2026-08-21T13:04:59Z"; \
    print(urllib.request.urlopen(url).read().decode())'

# Retrieve live/latest operational data from MET Norway (Section 3.2)
# Usage: just get-operational-data <YOUR_CLIENT_ID>
get-operational-data client_id:
    @python3 -c '\
    import urllib.request, base64, sys; \
    client_id = sys.argv[1]; \
    url = "https://frost-rc.met.no"; \
    req = urllib.request.Request(url); \
    auth = base64.b64encode(f"{client_id}:".encode()).decode(); \
    req.add_header("Authorization", f"Basic {auth}"); \
    print(urllib.request.urlopen(req).read().decode())' {{client_id}}
