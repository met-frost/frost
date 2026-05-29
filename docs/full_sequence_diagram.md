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
    Client->>Router: GET /api/v1/obs/{tstype}/get?...
    Router->>Obs: HandleGet
    Obs->>Obs: Extract and validate query params
    Obs->>TS: CreateCustomReqInfo(query)
    Obs->>Obs: Parse time spec + incobs + itemlimit
    Obs->>TS: GetInstances(query)
    Obs->>TS: HeaderFilterSpecial / HeaderPxmtyFilter / FinalizeInstanceOrder

    alt incobs=false
        Obs->>Obs: Build header-only dataset
    else incobs=true
        Obs->>SBE: ReadMultiTS for matching series and time constraints
        SBE-->>Obs: Matching observations
        Obs->>Restrict: Apply read restriction masking
    end

    Obs-->>Client: 200 JSON { data: dataset }
    end

    rect rgb(255, 250, 240)
    note over Client,Registry: Write operations
    Client->>Router: POST /api/v1/obs/{tstype}/ts/create|ts/update|ts/delete|put
    Router->>Obs: applyWriteOperation
    Obs->>Restrict: Get write restriction
    Obs->>Obs: Validate media type and dataset schema

    loop each timeseries in dataset
        Obs->>Obs: Match header id against writable patterns
        alt op = ts/create
            Obs->>SBE: CreateTimeSeries
            Obs->>Registry: AddTimeSeries
        else op = ts/update
            Obs->>SBE: UpdateTimeSeries
            Obs->>Registry: UpdateTimeSeries
        else op = ts/delete
            Obs->>SBE: RemoveTimeSeries
            Obs->>Registry: RemoveTimeSeries
        else op = put
            Obs->>Registry: Ensure timeseries exists
            Obs->>TS: IngestHook
            Obs->>SBE: Write observations
            SBE-->>Obs: inserted/updated/deleted counters
        end
    end

    Obs-->>Client: 200 JSON summary
    end