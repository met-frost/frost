// Package obssbepostgres ... TO BE DOCUMENTED.
package obssbepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	_ "github.com/lib/pq" // (need comment here to avoid go-lint warning!)
	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	storagebackends "gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/timeseries"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/tsregistry"
)

// Postgres is an implementation of the StorageBackend interface that accesses
// data through a Postgres database.
type Postgres struct {
	Db *sql.DB
}

// NewPostgres creates and initializes a new instance of Postgres.
// Returns (the initialized instance, nil) on success, otherwise (nil, error).
func NewPostgres(host, portS, user, password string) (*Postgres, error) {
	port, err := strconv.Atoi(portS)
	if err != nil {
		return nil, fmt.Errorf("failed to convert port number to int: %v", err)
	}

	const dbname = "datasets" // hard-coded for now

	// create and validate connection to Postgres server
	pgInfo := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", pgInfo)
	if err != nil {
		return nil, fmt.Errorf("sql.Open() failed: %v", err)
	}
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("db.Ping() failed: %v", err)
	}

	// create and return backend struct
	sbe := new(Postgres)
	sbe.Db = db
	return sbe, nil
}

// Description ... (see documentation in StorageBackend interface)
func (sbe *Postgres) Description() string {
	return "Postgres database"
}

// getTSAbsID tries to find the absolute ID for tstype/tsid in the database.
// Returns (..., ..., error) if an error occurs, (..., false, nil) if not found,
// or (id, true, nil) if found.
func (sbe *Postgres) getTSAbsID(tstype, tsid string) (int, bool, error) {
	query := `SELECT id FROM ts WHERE tstype=$1 AND lower(tsid::text)::jsonb=lower($2)::jsonb`
	rows, err := sbe.Db.Query(query, tstype, tsid)
	if err != nil {
		return -1, false, fmt.Errorf("sbe.Db.Query() failed: %v", err)
	}
	defer rows.Close()

	var tsaid int

	if rows.Next() {
		err = rows.Scan(&tsaid)
		if err != nil {
			return -1, false, fmt.Errorf("rows.Scan() failed: %v", err)
		}
	} else {
		return -1, false, nil // not found
	}

	return tsaid, true, nil // found
}

// CreateTimeSeries ... (see documentation in StorageBackend interface)
func (sbe *Postgres) CreateTimeSeries(tstype string, hdr timeseries.Header) (string, int, error) {
	var err error

	bstsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("json.Marshal(id) failed: %v", err)
	}

	stsid := string(bstsid)

	_, found, err := sbe.getTSAbsID(tstype, stsid)
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("getTSAbsID() failed: %v", err)
	}

	if found {
		// return successfully since time series already exists in the database
		return stsid, -1, nil
	}

	// add time series to the database

	bstsextra, err := json.Marshal(hdr["extra"])
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("json.Marshal(extra) failed: %v", err)
	}

	query := `INSERT INTO ts (tstype, tsid, tsextra) VALUES ($1, $2, $3)`
	_, err = sbe.Db.Exec(query, tstype, string(bstsid), string(bstsextra))
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("sbe.Db.Exec() failed: %v", err)
	}

	return stsid, -1, nil // time series successfully created
}

// RemoveTimeSeries ... (see documentation in StorageBackend interface)
func (sbe *Postgres) RemoveTimeSeries(tstype string, hdr timeseries.Header) (int, error) {
	var err error

	bstsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("json.Marshal(id) failed: %v", err)
	}

	tsaid, found, err := sbe.getTSAbsID(tstype, string(bstsid))
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("getTSAbsID() failed: %v", err)
	}

	if !found {
		return -1, nil // return successfully since time series already non-existent in the database
	}

	// remove time series from the database ...
	query := `DELETE FROM ts WHERE id = $1`
	_, err = sbe.Db.Exec(query, tsaid)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("sbe.Db.Exec() failed: %v", err)
	}

	return -1, nil // time series successfully removed
}

// UpdateTimeSeries ... (see documentation in StorageBackend interface)
func (sbe *Postgres) UpdateTimeSeries(tstype string, hdr timeseries.Header) (int, error) {
	return http.StatusNotImplemented, fmt.Errorf(
		"UpdateTimeSeries() not implemented for Postgres storage backend")
}

// ReadSingleTS ... (see documentation in StorageBackend interface)
func (sbe *Postgres) ReadSingleTS(
	tstype string, ts0 *timeseries.TimeSeries, hdr timeseries.Header, t1, t2 int64,
	obsBodyModify func(*timeseries.TimeSeries, time.Time, *map[string]interface{}) (int, error),
	obsFilter func(*timeseries.TimeSeries, time.Time, map[string]interface{}) (bool, int, error),
	limit int, observations *[]dataset.Observation, reqInfo timeseries.RequestInfo) (
	bool, int, error) {

	if limit == 0 {
		return false, http.StatusInternalServerError,
			fmt.Errorf("limit must be either negative or positive")
	}

	var err error

	bstsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return false, http.StatusInternalServerError, fmt.Errorf("json.Marshal(id) failed: %v", err)
	}
	stsid := string(bstsid)

	tsaid, found, err := sbe.getTSAbsID(tstype, stsid)
	if err != nil {
		return false, http.StatusInternalServerError, fmt.Errorf("getTSAbsID() failed: %v", err)
	}

	if !found {
		return false, http.StatusNotFound, fmt.Errorf("time series not found")
	}

	// ensure that the time series already exists in the registry
	if exists, reason := tsregistry.TimeSeriesExists(tstype, stsid); !exists {
		return false, http.StatusInternalServerError,
			fmt.Errorf("time series not found in internal registry: %s", reason)
	}

	// read observations
	baseQuery := `
		SELECT tstamp, body
		FROM obs
	    WHERE ts_id=$1
	    AND $2 <= extract(epoch from tstamp)
	    AND extract(epoch from tstamp) < $3
		ORDER BY tstamp ASC`

	var rows *sql.Rows
	if limit > 0 {
		// try to read at most limit + 1 observations values within [t1, t2>
		query := baseQuery + " LIMIT $4"
		rows, err = sbe.Db.Query(query, tsaid, t1, t2, limit+1)
		if err != nil {
			return false, http.StatusInternalServerError,
				fmt.Errorf("limited query failed: %v", err)
		}
	} else {
		query := baseQuery
		rows, err = sbe.Db.Query(query, tsaid, t1, t2)
		if err != nil {
			return false, http.StatusInternalServerError,
				fmt.Errorf("unlimited query failed: %v", err)
		}
	}
	defer rows.Close()

	*observations = nil // remove any existing observations

	var excess bool // exceeding limit? (false by default)

	for rows.Next() {

		var t time.Time
		var bodyBytes []byte

		err = rows.Scan(&t, &bodyBytes)
		if err != nil {
			return false, http.StatusInternalServerError, fmt.Errorf("rows.Scan() failed: %v", err)
		}
		var body map[string]interface{}
		err = json.Unmarshal(bodyBytes, &body)
		if err != nil {
			return false, http.StatusInternalServerError,
				fmt.Errorf("json.Unmarshal() failed: %v", err)
		}

		// read observation in valid time interval (ensured by query)

		obs := dataset.Observation{Time: &t, Body: body}

		// apply obs body modifier
		statusCode, err := obsBodyModify(ts0, *obs.Time, &obs.Body)
		if err != nil {
			return false, statusCode, fmt.Errorf("obsBodyModify() failed: %v", err)
		}

		if (limit < 0) || (len(*observations) < limit) { // still room for more

			// apply obs filter
			obsFilterPassed, statusCode, err := obsFilter(ts0, *obs.Time, obs.Body)
			if err != nil {
				return false, statusCode, fmt.Errorf("obsFilter() failed: %v", err)
			}

			if obsFilterPassed { // obs passed the obs filter, so add it
				*observations = append(*observations, obs)
			}

		} else {
			//assert(len(*observations) == limit)
			// no more room, but indicate that there is now at least one more value in [t1,t2>
			excess = true
			break
		}
	}

	return excess, -1, nil // zero or more values successfully retrieved
}

// ReadMultiTS ... (see documentation in StorageBackend interface)
func (sbe *Postgres) ReadMultiTS(
	tstype string, tsSeq *timeseries.InstanceSeq, hdrs []timeseries.Header, t1, t2 int64,
	obsBodyModify func(*timeseries.TimeSeries, time.Time, *map[string]interface{}) (int, error),
	obsFilter func(*timeseries.TimeSeries, time.Time, map[string]interface{}) (bool, int, error),
	latestLimit, itemLimit int, obs *[][]dataset.Observation,
	reqInfo timeseries.RequestInfo, ctx context.Context) (int, int, error) {

	// for now delegate to ReadMultiTSAdapter; TODO: consider if it would be faster to
	// implement directly using SQL
	return storagebackends.ReadMultiTSAdapter(
		sbe, tstype, tsSeq, hdrs, t1, t2, obsBodyModify, obsFilter, latestLimit, itemLimit, obs,
		reqInfo)
}

// obsExists finds out if an observation already exists in a time series at a certain time.
// Returns (true/false, nil) upon success, otherwise (false, error).
func (sbe *Postgres) obsExists(tsaid int, tstamp time.Time) (bool, error) {

	queryExists := `SELECT count(*) FROM obs WHERE ts_id=$1 AND tstamp=$2`

	rows, err := sbe.Db.Query(queryExists, tsaid, tstamp)
	if err != nil {
		return false, fmt.Errorf("sbe.Db.Query(queryExisted) failed: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return false, fmt.Errorf("rows.Next() returned false")
	}
	var existsCount int
	err = rows.Scan(&existsCount)
	if err != nil {
		return false, fmt.Errorf("rows.Scan() failed: %v", err)
	}
	if existsCount > 1 {
		return false, fmt.Errorf("existsCount > 1: %d", existsCount)
	}

	return existsCount == 1, nil
}

// Write ... (see documentation in StorageBackend interface)
func (sbe *Postgres) Write(
	tstype string, hdr timeseries.Header, observations *[]dataset.Observation,
	ingestHookErrors []error) (storagebackends.ObsWriteSummary, int, error) {

	// TODO: make use of ingestHookErrors

	var err error

	bstsid, err := json.Marshal(hdr["id"])
	if err != nil {
		return storagebackends.ObsWriteSummary{}, http.StatusInternalServerError,
			fmt.Errorf("json.Marshal(id) failed: %v", err)
	}
	stsid := string(bstsid)

	// ensure that the time series already exists in the registry
	if exists, reason := tsregistry.TimeSeriesExists(tstype, stsid); !exists {
		return storagebackends.ObsWriteSummary{}, http.StatusInternalServerError,
			fmt.Errorf("time series not found in internal registry: %s", reason)
	}

	// look up time series in database
	tsaid, found, err := sbe.getTSAbsID(tstype, stsid)
	if err != nil {
		return storagebackends.ObsWriteSummary{}, http.StatusInternalServerError,
			fmt.Errorf("getTSAbsID() failed: %v", err)
	}
	if !found {
		return storagebackends.ObsWriteSummary{}, http.StatusNotFound,
			fmt.Errorf("time series not found in database (tstype: %s; id: %v)", tstype, hdr["id"])
	}

	// delete/insert/update observations

	summary := storagebackends.ObsWriteSummary{}

	queryDelete := `DELETE FROM obs WHERE tstamp=$1`

	queryUpsert := `
	INSERT INTO obs (ts_id, tstamp, body) VALUES ($1, $2, $3)
	ON CONFLICT ON CONSTRAINT obs_pkey
	DO UPDATE SET body=$3
	`

	for _, obs := range *observations {

		existed, err := sbe.obsExists(tsaid, *obs.Time)
		if err != nil {
			return storagebackends.ObsWriteSummary{}, http.StatusInternalServerError,
				fmt.Errorf("sbe.ObsExists() failed: %v", err)
		}

		if obs.Body == nil { // delete obs at obs.Time (no-op if no such obs existed)
			_, err = sbe.Db.Exec(queryDelete, obs.Time)
			if err != nil {
				return storagebackends.ObsWriteSummary{}, http.StatusInternalServerError,
					fmt.Errorf("sbe.Db.Exec(queryDelete) failed: %v", err)
			}
			if existed {
				summary.Deleted++
			}
		} else { // insert or update obs at obs.Time

			// serialize obs body
			mbody, err := json.Marshal(obs.Body)
			if err != nil {
				return storagebackends.ObsWriteSummary{}, http.StatusInternalServerError,
					fmt.Errorf("json.Marshal() failed: %v", err)
			}

			// insert or update observation
			_, err = sbe.Db.Exec(queryUpsert, tsaid, obs.Time, mbody)
			if err != nil {
				return storagebackends.ObsWriteSummary{}, http.StatusInternalServerError,
					fmt.Errorf("sbe.Db.Exec(queryUpsert) failed: %v", err)
			}
			if existed {
				summary.Updated++
			} else {
				summary.Inserted++
			}
		}
	}

	return summary, -1, nil // all observations successfully processed
}

// SupportsClear ... (see documentation in StorageBackend interface)
func (sbe *Postgres) SupportsClear() bool {
	return common.Getenv("PSBALLOWCLEAR", "false") == "true"
}

// Clear ... (see documentation in StorageBackend interface)
func (sbe *Postgres) Clear() (int, error) {
	_, err := sbe.Db.Exec("DELETE FROM ts")
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("query failed: %v", err)
	}
	return -1, nil
}

// LoadTimeSeries ... (see documentation in StorageBackend interface)
func (sbe *Postgres) LoadTimeSeries(
	optionalFeatures common.StringSet, nonFatalErrors *[]error) error {

	rows, err := sbe.Db.Query("SELECT DISTINCT tstype, tsid, tsextra FROM ts")
	if err != nil {
		return fmt.Errorf("query failed: %v", err)
	}
	defer rows.Close()

	log.Printf("loading time series headers from database ...\n")
	loadedCount := 0
	skippedCount := 0
	for rows.Next() {
		var tstype, tsid, tsextra0 sql.NullString
		err = rows.Scan(&tstype, &tsid, &tsextra0)
		if err != nil {
			return fmt.Errorf("rows.Scan() failed: %v", err)
		}

		if !tstype.Valid {
			return fmt.Errorf("tstype unexpectedly NULL")
		}

		if optionalFeatures.ContainsMatch(tstype.String) { // only load active ts types
			if !tsid.Valid {
				return fmt.Errorf("tsid unexpectedly NULL")
			}
			tsextra := "{}" // default value in case of (equivalent to) NULL
			if tsextra0.Valid && common.IsNonEmptyJSONObject(tsextra0.String) {
				tsextra = tsextra0.String
			}

			stshdr := fmt.Sprintf(`{"id": %s, "extra": %s}`, tsid.String, tsextra)
			// convert object keys to lowercase
			stshdr0, err := common.ConvertObjKeysToLower2(stshdr)
			if err != nil {
				return fmt.Errorf("common.ConvertObjKeysToLower2() failed: %v", err)
			}

			// just set fromTime and toTime as "not in use" for now (TODO?)
			fromTime := int64(math.MinInt64)
			toTime := int64(math.MinInt64)

			_, err = tsregistry.AddTimeSeries(
				tstype.String, tsid.String, stshdr0, tsid.String, tsextra, fromTime, toTime,
				nonFatalErrors)
			if err != nil {
				return fmt.Errorf("tsregistry.AddTimeSeries() failed: %v", err)
			}

			loadedCount++
		} else {
			skippedCount++
		}
	}
	log.Printf(
		"successfully registered %d time series (and skipped %d)\n", loadedCount, skippedCount)

	return nil
}

// Cleanup ... (see documentation in StorageBackend interface)
func (sbe *Postgres) Cleanup() {
	sbe.Db.Close()
}

// Print ... (see documentation in StorageBackend interface)
func (sbe *Postgres) Print() {
	fmt.Printf("\ncurrent contents of the Postgres storage backend:\n" +
		"NOT IMPLEMENTED!\n")
}
