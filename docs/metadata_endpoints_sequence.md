sequenceDiagram
    autonumber
    participant Client as API Client
    participant Router as Frost External Router
    participant Meta as Meta Handlers

    Client->>Router: GET /api/v1/about
    Router->>Meta: HandleAbout
    Meta-->>Client: 200 JSON-LD service metadata

    Client->>Router: GET /api/v1/healthz
    Router->>Meta: HandleHealthz
    Meta-->>Client: 200 JSON health report