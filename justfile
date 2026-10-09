# list recipes
default:
    @just --list

set positional-arguments

# build the docker containers (Frost and PSB)
build:
    #BUILDKIT_PROGRESS=plain docker compose build --no-cache
    docker compose build

# run the docker containers
run: build
    docker compose up -d

# run all ingestion and retrieval validation cases
test: test-roundtrip test-week test-invalid-ingestion

# CSV ingestion and latest-observation round-trip validation
test-roundtrip: run
    python ingest.py  # inserts data to PSB via Frost
    python retrieve.py http://localhost:8080 output.csv sources=badevann.no buoyids=004,011 parameters=temperature
    python verify.py  # verifies that input data matches output data

# fixed one-week ingestion and radius-filtered end-to-end validation
test-week: run
    python test_ingestion.py week

# malformed ingestion must fail with HTTP 400 without changing observations
test-invalid-ingestion: run
    python test_ingestion.py invalid

# remove the docker containers
destroy:
    docker compose down
