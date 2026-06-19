sequenceDiagram
    autonumber
    participant Client as API Client
    participant Router as Frost External Router
    participant Obs as Obs Route Handlers
    participant TS as TimeSeries Implementation
    participant Restrict as Restriction Layer
    participant Registry as TS Registry
    participant SBE as Storage Backend

    rect rgb(255, 250, 240)
    note over Client,SBE: Time series management
    Client->>Router: POST /api/v1/obs/{tstype}/ts/{create|update|delete}  { dataset }
    Router->>Obs: HandleTs{Create|Update|Delete}
    Obs->>Restrict: Check write authorization
    Obs->>SBE: {Add|Update|Delete} time series headers
    Obs->>Registry: {Add|Update|Delete} time series headers
    Obs-->>Client: 200 JSON { rejected, accepted, applied }
    end

    rect rgb(255, 245, 245)
    note over Client,SBE: Ingest observations
    Client->>Router: POST /api/v1/obs/{tstype}/put { dataset }
    Router->>Obs: HandlePut
    Obs->>Restrict: Check write authorization
    Obs->>Registry: Verify that time series headers exist
    Obs->>TS: Run ingest hook
    Obs->>SBE: Write observations
    SBE-->>Obs: inserted/updated/deleted counters
    Obs-->>Client: 200 JSON { inserted, updated, deleted }
    end

    rect rgb(245, 255, 245)
    note over Client,SBE: Retrieve observations
    Client->>Router: GET /api/v1/obs/{tstype}/get?incobs=true&time=...
    Router->>Obs: HandleGet
    Obs->>TS: Resolve matching time series headers
    TS->>Registry: Resolve matching time series headers
    Obs->>SBE: Read observations for matching series headers
    SBE-->>Obs: Dataset
    Obs->>Restrict: Check read authorization
    Obs-->>Client: 200 JSON { dataset }
    end
