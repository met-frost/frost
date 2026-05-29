sequenceDiagram
    autonumber
    participant Client as API Client
    participant Router as Frost External Router
    participant Obs as Obs Route Handlers
    participant Restrict as Restriction Layer
    participant TS as TimeSeries Implementation
    participant Registry as TS Registry
    participant SBE as Storage Backend

    Client->>Router: POST /api/v1/obs/{tstype}/put
    Router->>Obs: HandlePut
    Obs->>Restrict: Check write authorization
    Obs->>Registry: Verify series exists
    Obs->>TS: Run ingest hook
    Obs->>SBE: Write observations
    SBE-->>Obs: inserted/updated/deleted counters
    Obs-->>Client: 200 JSON { inserted, updated, deleted }