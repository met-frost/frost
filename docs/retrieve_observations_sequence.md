sequenceDiagram
    autonumber
    participant Client as API Client
    participant Router as Frost External Router
    participant Obs as Obs Route Handlers
    participant TS as TimeSeries Implementation
    participant Restrict as Restriction Layer
    participant SBE as Storage Backend

    Client->>Router: GET /api/v1/obs/{tstype}/get?incobs=true&time=...
    Router->>Obs: HandleGet
    Obs->>TS: Resolve matching time series
    Obs->>SBE: Read observations for matching series
    SBE-->>Obs: Dataset rows
    Obs->>Restrict: Apply read restriction policy
    Obs-->>Client: 200 JSON { data: dataset }