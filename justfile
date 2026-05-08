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
    #TODO: ingest.py   - calls Frost endpoints /ts/create and /put to ingest data from a.csv
    #TODO: retrieve.py - calls Frost endpoint /get to retrieve data into b.csv
    #TODO: diff.py     - verifies that a.csv == b.csv
