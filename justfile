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

# test ingesting and retrieving data
test: run
    python ingest.py  # inserts data to PSB via Frost
    python retrieve.py  # retrieves data from PSB via Frost
    python verify.py  # verifies that input data matches output data

# remove the docker containers
destroy:
    docker compose down
