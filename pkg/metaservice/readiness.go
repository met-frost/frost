package metaservice

import (
	"net/http"
)

type ReadinessStatus bool

var (
	readiness ReadinessStatus = false
)

func SetReadinessStatus(status ReadinessStatus) {
	readiness = status
}
func GetReadinessStatus() ReadinessStatus {
	return readiness
}

// HandleHealthz runs the callback function check and serializes and sends the result of that check.
func HandleReadinessStatus(check func() (*ReadinessStatus, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ready, err := check()
		if err != nil {
			http.Error(w, "Could not check the readiness of the service.", http.StatusInternalServerError)
		}
		if !*ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Error: Frost is still loading metadata, so is not yet ready to serve requests"))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}
	}
}
