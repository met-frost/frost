# list recipes
default:
    @just --list

set positional-arguments

# build the docker containers
build:
    BUILDKIT_PROGRESS=plain docker compose build --no-cache

# run the docker containers (Frost and PSB)
run: build
    docker compose up -d

# test ingesting and retrieving data
test: run
    #ingest.py   - calls Frost endpoints /ts/create and /put to ingest data from a.csv
    #retrieve.py - calls Frost endpoint /get to retrieve data into b.csv
    #diff.py     - verifies that a.csv == b.csv
