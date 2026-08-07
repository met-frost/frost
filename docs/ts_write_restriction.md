# Time series write restriction

## Background

Frost offers a simple mechanism to globally restrict write access for the operations represented by the
following routes:

- `/obs/_tstype_/ts/create`
- `/obs/_tstype_/ts/update`
- `/obs/_tstype_/ts/delete`
- `/obs/_tstype_/put`

(_tstype_ is a placeholder for the actual time series type, like _sst_).

The mechanism is likely to be useful in a scenario where only a small set of write access rules
need to be specified (otherwise rules may be hard to maintain).

The mechanism works by allowing any client who possesses the token _tk_ to apply a time series _ts_
of type _tstype_ using the operation _op_ if _tk_ is registered (in Frost) to be associated with
_tstype_, _op_, and a _header pattern_ that matches _ts_. The time series (_ts_) is included in the
dataset passed to the operation, i.e. in the body of a request to the route that represents the
operation.

## Procedure

### STEP 1

Generate a fresh write key using `uuidgen`:

```text
$ uuidgen
70fb4f76-40df-4ec1-a61a-a07d1728b363
```

The write key must be kept secret.

### STEP 2

Run Frost repeatedly as follows to generate one or more write tokens, passing as the `WRITEKEY`
variable the key generated in STEP 1:

```text
$ go build -o mainexec cmd/frost/main.go && DUMPWRITETOKEN=true WRITEKEY=70fb4f76-40df-4ec1-a61a-a07d1728b363 ./mainexec 2>/dev/null
q0HpG1YzppenqZFxL3ZFJszwXDYsxFPBERmbwLSoSw6P
$ go build -o mainexec cmd/frost/main.go && DUMPWRITETOKEN=true WRITEKEY=70fb4f76-40df-4ec1-a61a-a07d1728b363 ./mainexec 2>/dev/null
h38otvt9Drn04lAtgDQbj-b4faXhutq35Dpzr415jw8l
...
```

The write tokens must be distributed in a secure way to the clients that are going to use them
(using [GPG](https://gnupg.org/) for example).

### STEP 3

Set up a file (see example [here](../test/tswriteaccess_examples/example1.json)) with write access
rules in the form of a JSON object defined as follows:

```text
{
  "rules": [
    {
      "descr": <optional array of strings describing this rule; ignored by Frost>
      "token": <write token>
      "tstype": <time series type, e.g. "frost0">,
      "tscreate": <whether this rule allows access to /obs/tstype/ts/create>,
      "tsupdate": <whether this rule allows access to /obs/tstype/ts/update>,
      "tsdelete": <whether this rule allows access to /obs/tstype/ts/delete>,
      "put": <whether this rule allows access to /obs/tstype/put>,
      "hdridmatch": <header ID match pattern, e.g. "{\"id\":{\"source\":\"SN187*\"}}">,
    },
    ...
  ]
}
```

The file must be kept secret (since it contains secret tokens).

_**Rule application**_

When a request to a writable endpoint arrives, Frost first finds the set of rules _R_ that match
on tstype, token, and operation (ts/create, ts/update, ts/delete, or put). Each time series
_ts_ (in the dataset passed with the request) that matches at least one of the header patterns
in _R_ is then applied to the storage backend using the specified operation.

### STEP 4

Run Frost in regular "service" mode, passing as the `WRITEKEY` variable the key generated in
STEP 1 and as the `TSWRITEACCESS` variable the name of the file defined in STEP 3
(in addition to other variables):

```text
go build -o mainexec cmd/frost/main.go && WRITEKEY=70fb4f76-40df-4ec1-a61a-a07d1728b363 TSWRITEACCESS=<write access rules> OBSBACKEND=postgres ... ./mainexec
```

### STEP 5

Make a request to a Frost endpoint that requires write access (like `/obs/*/put`), passing the
write token in the `X-Frost-Writetoken` header):

```text
curl -i -X POST "http://localhost:8080/api/v1/obs/frost0/put" -H "Content-Type: application/json" -H "X-Frost-Writetoken:q0HpG1YzppenqZFxL3ZFJszwXDYsxFPBERmbwLSoSw6P" -d @dataset.json -w '\n'
```

If all goes well, Frost will respond with 200 Ok.

## Disabling the mechanism

The mechanism can be disabled altogether simply by omitting the `WRITEKEY` variable when running
Frost in regular "service" mode. In that case any request has full write access and any
`X-Frost-Writetoken` header will be ignored.
