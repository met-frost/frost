CREATE DATABASE datasets;
\c datasets

-- Time series table
CREATE TABLE ts (
    id integer PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    tstype text NOT NULL,
    tsid jsonb NOT NULL,
    tsextra jsonb,
    UNIQUE(tstype, tsid)
);
CREATE UNIQUE INDEX ts_id_idx ON ts (id);
CREATE UNIQUE INDEX ts_tstype_tsid_idx ON ts (tstype, tsid);

-- Observations table
CREATE TABLE obs (
    ts_id integer REFERENCES ts(id) ON DELETE CASCADE,
    tstamp timestamp, -- NOT NULL, but this is implied by being part of PK
    body jsonb NOT NULL,
    PRIMARY KEY(ts_id, tstamp)
);
CREATE UNIQUE INDEX obs_ts_tstamp_idx ON obs (ts_id, tstamp);

-- -- insert a few test values
INSERT INTO ts (tstype, tsid) VALUES(
    'frost0', '{"source": "s1", "sensorLevel": "sl1", "element": "e1"}'::jsonb
);
INSERT INTO ts (tstype, tsid, tsextra) VALUES(
    'frost0', '{"source": "s2", "sensorLevel": "sl1", "element": "e1"}'::jsonb,
    '{"organizations": ['
    '{"name": "MET",   "from": "2010-01-01T00:00:00Z", "to": "2030-01-01T00:00:00Z"}, '
    '{"name": "NIBIO", "from": "2011-01-01T00:00:00Z", "to": "2031-01-01T00:00:00Z"}]}'::jsonb
);
INSERT INTO ts (tstype, tsid, tsextra) VALUES(
    'frost0', '{"source": "s3", "sensorLevel": "sl1", "element": "e1"}'::jsonb,
    '{"pos": {"lon": "10.001", "lat": "59.001"}}'::jsonb
);
INSERT INTO ts (tstype, tsid, tsextra) VALUES(
    'frost0', '{"source": "s4", "sensorLevel": "sl1", "element": "e1"}'::jsonb,
    '{"organizations": ['
    '{"name": "MET", "from": "2011-02-01T00:00:00Z", "to": "2031-02-01T00:00:00Z"}], '
    '"pos": {"lon": "10.001", "lat": "59.001"}}'::jsonb
);

INSERT INTO obs (ts_id, tstamp, body)
    SELECT id, '2020-06-26T23:59:58',
    '{"pos": {"lon": "10.752221", "lat": "59.913886"}, "value": "-12.31", "quality": ""}'::jsonb
    FROM ts
    WHERE tstype='frost0'
        AND tsid='{"source": "s1", "element": "e1", "sensorLevel": "sl1"}'::jsonb;
INSERT INTO obs (ts_id, tstamp, body)
    SELECT id, '2020-06-26T23:59:59',
    '{"pos": {"lon": "10.752222", "lat": "59.913887"}, "value": "-12.32", "quality": ""}'::jsonb
    FROM ts
    WHERE tstype='frost0'
        AND tsid='{"source": "s1", "element": "e1", "sensorLevel": "sl1"}'::jsonb;
INSERT INTO obs (ts_id, tstamp, body)
    SELECT id, '2020-06-27T00:00:00',
    '{"pos": {"lon": "10.752223", "lat": "59.913888"}, "value": "-12.33", "quality": ""}'::jsonb
    FROM ts
    WHERE tstype='frost0'
        AND tsid='{"source": "s1", "element": "e1", "sensorLevel": "sl1"}'::jsonb;
INSERT INTO obs (ts_id, tstamp, body)
    SELECT id, '2020-06-27T00:00:01',
    '{"pos": {"lon": "10.752224", "lat": "59.913889"}, "value": "-12.34", "quality": ""}'::jsonb
    FROM ts
    WHERE tstype='frost0'
        AND tsid='{"source": "s1", "element": "e1", "sensorLevel": "sl1"}'::jsonb;
INSERT INTO obs (ts_id, tstamp, body)
    SELECT id, '2020-06-27T00:00:02',
    '{"value": "-12.35", "quality": ""}'::jsonb
    FROM ts
    WHERE tstype='frost0'
        AND tsid='{"source": "s1", "element": "e1", "sensorLevel": "sl1"}'::jsonb;
