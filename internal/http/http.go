package localhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	gorilla "github.com/gorilla/handlers"
)

// TODO: fill in documentation
type serverError struct {
	ErrMsg string `json:"error"`
}

// GetParamFloat ... (TODO: fill in documentation)
func GetParamFloat(queryParams url.Values, name string) (float32, error) {
	val, ok := queryParams[name]
	if !ok || len(val) == 0 {
		return 0, errors.New("Missing value for " + name)
	}
	if len(val) > 1 {
		return 0, errors.New("Too many values for " + name)
	}
	ret, err := strconv.ParseFloat(val[0], 32)
	if err != nil {
		return 0, errors.New("Value for " + name + " cannot be parsed as float")
	}
	value := float32(ret)

	return value, nil
}

// GetParamInteger ... (TODO: fill in documentation)
func GetParamInteger(queryParams url.Values, name string) (int64, error) {
	val, ok := queryParams[name]
	if !ok || len(val) == 0 {
		return 0, errors.New("Missing value for " + name)
	}
	if len(val) > 1 {
		return 0, errors.New("Too many values for " + name)
	}
	ret, err := strconv.ParseInt(val[0], 10, 64)
	if err != nil {
		return 0, errors.New("Value for " + name + " cannot be parsed as integer")
	}
	value := int64(ret)

	return value, nil
}

// GetParamString ... (TODO: fill in documentation)
func GetParamString(queryParams url.Values, name string) (string, error) {
	val, ok := queryParams[name]
	if !ok || len(val) == 0 {
		return "", errors.New("Missing value for " + name)
	}
	if len(val) > 1 {
		return "", errors.New("Too many values for " + name)
	}

	return val[0], nil
}

// GetParamStringList ... (TODO: fill in documentation)
func GetParamStringList(queryParams url.Values, name string) ([]string, error) {
	val, ok := queryParams[name]
	if !ok || len(val) == 0 {
		return []string{}, errors.New("Missing value for " + name)
	}
	values := strings.Split(val[0], ",")
	//fmt.Println(values, " len: ", len(values))

	return values, nil
}

// GetOptionalParamString ... (TODO: fill in documentation)
func GetOptionalParamString(queryParams url.Values, name string) (string, error) {
	val, ok := queryParams[name]
	if !ok || len(val) == 0 {
		return "", nil // this param is optional so ok if empty
	}
	if len(val) > 1 {
		return "", errors.New("Too many values for " + name)
	}

	return val[0], nil
}

// SetOkResponse ... (TODO: fill in documentation)
func SetOkResponse(payload []byte, responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Cache-Control", "max-age=86400")
	responseWriter.Header().Set("Content-Type", "application/json")
	_, err := responseWriter.Write(payload)
	if err != nil {
		log.Printf(
			"failed to send Ok response (status code 200) for request %q: %v", request.URL, err)
	}
}

// SetErrorResponse ... (TODO: fill in documentation)
func SetErrorResponse(
	statusCode int, errMsg error, respWriter http.ResponseWriter, request *http.Request) {
	errResponse := serverError{
		ErrMsg: errMsg.Error(),
	}

	payload, err := json.Marshal(errResponse)
	if err != nil {
		http.Error(respWriter, "Failed to serialize data.", http.StatusInternalServerError)
		return
	}
	respWriter.WriteHeader(statusCode)

	respWriter.Header().Set("Content-Type", "application/json")
	_, err = respWriter.Write(payload)
	if err != nil {
		log.Printf(
			"failed to send error response (status code: %d) for request %q: %v", statusCode,
			request.URL, err)
	}
}

// GetHTTPHandlerForBehindProxy returns an HTTP handler that has the scheme and host set correctly
// when behind a proxy. Usually needed when the response consists of urls to the service.
func GetHTTPHandlerForBehindProxy(next func(w http.ResponseWriter, r *http.Request)) http.Handler {
	setSchemeIfEmpty := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Scheme == "" {
			r.URL.Scheme = "http"
		}
		next(w, r)
	}
	return gorilla.ProxyHeaders(http.HandlerFunc(setSchemeIfEmpty))
}

// GetHTTPHandler returns an HTTP handler that has CORS enabled iff corsEnabled is true.
func GetHTTPHandler(next http.Handler, corsEnabled bool) http.Handler {
	if corsEnabled {
		//fmt.Println("CORS is being enabled")
		headers := gorilla.AllowedHeaders([]string{"Content-Type", "Authorization"})
		methods := gorilla.AllowedMethods([]string{"GET"})
		next2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try to get pop up when using url to access the api
			w.Header().Set("WWW-Authenticate", `Basic realm="auth frost"`)
			next.ServeHTTP(w, r)
		})

		return gorilla.CORS(headers, methods)(next2)
		/*
			// attempt to do without gorilla package
			// currently results in err in javascript:
			// from origin 'null' has been blocked by CORS policy: Response to preflight request doesn't pass access control check: It does not have HTTP ok status.
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Access-Control-Allow-Origin", "*")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
					next.ServeHTTP(w, r)
				})
		*/
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to get pop up when using url to access the api
		w.Header().Set("WWW-Authenticate", `Basic realm="auth frost"`)
		next.ServeHTTP(w, r)
	})
}

// ExtractHeader extracts header fields from an HTTP request.
func ExtractHeader(request *http.Request) (http.Header, error) {
	header := request.Header

	// assert only one value per header field
	for k, v := range header {
		if len(v) > 1 {
			return nil, fmt.Errorf("header field %s specified multiple times", k)
		}
	}

	return header, nil
}

// ExtractQueryParameters extracts query parameters from an HTTP request.
func ExtractQueryParameters(request *http.Request) (url.Values, error) {
	var err error
	var queryParams url.Values

	switch method := request.Method; method {
	case "GET":
		queryParams, err = url.ParseQuery(request.URL.RawQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to parse query in GET request: %v", err)
		}
	case "POST":
		err = request.ParseForm()
		if err != nil {
			return nil, fmt.Errorf("failed to parse form in POST request: %v", err)
		}
		queryParams = request.PostForm
	default:
		return nil, fmt.Errorf("unsupported HTTP request method: %s", method)
	}

	return queryParams, nil
}
