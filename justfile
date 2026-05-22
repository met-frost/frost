# list recipes
default:
    @just --list

set positional-arguments

# build the docker containers
build:
    BUILDKIT_PROGRESS=plain docker compose build --no-cache

# run the docker containers (Frost and PSB)
run:
    docker compose up -d

# test ingesting and retrieving data
test:
    python ingest.py  # inserts data to PSB via Frost
    python retrieve.py  # retrieves data from PSB via Frost
    # python diff.py ...  # verifies that input data matches output data

destroy:
    docker compose down
