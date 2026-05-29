sequenceDiagram
    autonumber
    participant Client as API Client
    participant Router as Frost External Router
    participant Meta as Meta Handlers
    participant Obs as Obs Route Handlers
    participant TS as TimeSeries Implementation
    participant Restrict as Restriction Layer
    participant SBE as Storage Backend
    participant Registry as TS Registry

    rect rgb(240, 248, 255)
    note over Client,Meta: Metadata endpoints
    Client->>Router: GET /api/v1/about
    Router->>Meta: HandleAbout
    Meta-->>Client: 200 JSON-LD service metadata

    Client->>Router: GET /api/v1/healthz
    Router->>Meta: HandleHealthz
    Meta-->>Client: 200 JSON health report
    end

    rect rgb(245, 255, 245)
    note over Client,SBE: Read observations
    Client->>Router: GET /api/v1/obs/{tstype}/get?incobs=true&time=...
    Router->>Obs: HandleGet
    Obs->>TS: Resolve matching time series
    Obs->>SBE: Read observations for matching series
    SBE-->>Obs: Dataset rows
    Obs->>Restrict: Apply read restriction policy
    Obs-->>Client: 200 JSON { data: dataset }
    end

    rect rgb(255, 250, 240)
    note over Client,Registry: Time series management
    Client->>Router: POST /api/v1/obs/{tstype}/ts/create
    Router->>Obs: applyWriteOperation
    Obs->>Restrict: Check write authorization
    Obs->>SBE: Create time series
    Obs->>Registry: Add time series metadata
    Obs-->>Client: 200 JSON { rejected, accepted, applied }
    end

    rect rgb(255, 245, 245)
    note over Client,SBE: Observation ingest
    Client->>Router: POST /api/v1/obs/{tstype}/put
    Router->>Obs: HandlePut
    Obs->>Restrict: Check write authorization
    Obs->>Registry: Verify series exists
    Obs->>TS: Run ingest hook
    Obs->>SBE: Write observations
    SBE-->>Obs: inserted/updated/deleted counters
    Obs-->>Client: 200 JSON { inserted, updated, deleted }
    end