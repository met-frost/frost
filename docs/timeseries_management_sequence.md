sequenceDiagram
    autonumber
    participant Client as API Client
    participant Router as Frost External Router
    participant Obs as Obs Route Handlers
    participant Restrict as Restriction Layer
    participant SBE as Storage Backend
    participant Registry as TS Registry

    Client->>Router: POST /api/v1/obs/{tstype}/ts/create
    Router->>Obs: applyWriteOperation
    Obs->>Restrict: Check write authorization
    Obs->>SBE: Create time series
    Obs->>Registry: Add time series metadata
    Obs-->>Client: 200 JSON { rejected, accepted, applied }