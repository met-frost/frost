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

# run services with test-only write restrictions enabled
run-test: build
    {{test_compose}} up -d --wait

# remove the docker containers
destroy:
    {{test_compose}} down
