package serve

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func unknownRemoteHost(name string) error {
	return refusal(http.StatusNotFound, "remote.unknown_host", fmt.Errorf("no machine named %q in the host book", name), map[string]any{"host": name})
}

func decodeRemoteBody(w http.ResponseWriter, r *http.Request, into any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(into); err != nil {
		badBody(w)
		return false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		badBody(w)
		return false
	}
	return true
}
