package httputil

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/gorilla/handlers"
)

// WriteJSON writes a json object on a http.ResponseWriter with the given code,
// panics on marshaling error
func WriteJSON(w http.ResponseWriter, r *http.Request, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	if pretty, _ := BoolFromQuery(r, "pretty", false); pretty { //nolint:errcheck
		enc.SetIndent("", "  ")
	}
	if err, ok := v.(error); ok {
		v = map[string]interface{}{"error": err.Error()}
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}

// ReadJSON reads the request body to a json object.
func ReadJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// BoolFromQuery obtains a boolean from a query entry.
func BoolFromQuery(r *http.Request, key string, defaultVal bool) (bool, error) {
	switch q := r.URL.Query().Get(key); q {
	case "true", "on", "1":
		return true, nil
	case "false", "off", "0":
		return false, nil
	case "":
		return defaultVal, nil
	default:
		return false, fmt.Errorf("invalid '%s' query value of '%s'", key, q)
	}
}

// WriteLog writes request and response parameters using format that
// works well with logging.Logger.
func WriteLog(writer io.Writer, params handlers.LogFormatterParams) {
	host, _, err := net.SplitHostPort(params.Request.RemoteAddr)
	if err != nil {
		host = params.Request.RemoteAddr
	}

	fmt.Fprintf(
		writer, "%s - \"%s %s %s\" %d\n",
		host, params.Request.Method, params.URL.String(), params.Request.Proto, params.StatusCode,
	)
}
