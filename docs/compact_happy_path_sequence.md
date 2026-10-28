%%{init: {
  "theme": "base",
  "sequence": {
    "wrap": false,
    "actorMargin": 100,
    "messageMargin": 50,
    "actorFontSize": 19,
    "messageFontSize": 19,
    "noteFontSize": 19
  },
  "themeVariables": {
    "fontSize": "19px",
    "noteBkgColor": "#e9eef2",
    "noteBorderColor": "#7b8794",
    "noteTextColor": "#1f2933"
  },
  "themeCSS": "text { font-size: 19px !important; } .actor { font-size: 19px !important; } .noteText { font-size: 19px !important; }"
}}%%
sequenceDiagram
  autonumber
    participant Client as API Client
    participant Router as Frost External<br/>Router
    participant Obs as Obs Route<br/>Handlers
    participant TS as TimeSeries<br/>Implementation
    participant Restrict as Restriction<br/>Layer
    participant Registry as TS Registry
    participant SBE as Storage<br/>Backend

    rect rgb(255, 250, 240)
    note over Client,SBE: Time series management
    Client->>Router: POST /obs/{tstype}/ts/{operation}
    Router->>Obs: Handle time-series action
    Obs->>Restrict: Check write authorization
    Obs->>SBE: {Add|Update|Delete} time series headers
    Obs->>Registry: {Add|Update|Delete} time series headers
    Obs-->>Client: 200 JSON { rejected, accepted, applied }
    end

    rect rgb(255, 245, 245)
    note over Client,SBE: Ingest observations
    Client->>Router: POST /obs/{tstype}/put
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
    Client->>Router: GET /obs/{tstype}/get?incobs&time
    Router->>Obs: HandleGet
    Obs->>TS: Resolve matching series
    TS->>Registry: Resolve matching series
    Obs->>SBE: Read observations for matching series headers
    SBE-->>Obs: Dataset
    Obs->>Restrict: Check read authorization
    Obs-->>Client: 200 JSON { dataset }
    end
