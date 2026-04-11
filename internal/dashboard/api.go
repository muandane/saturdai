package dashboard

import (
	"encoding/json"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Handler returns an http.Handler for dashboard JSON APIs.
func apiHandler(c client.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/dashboard/v1/profiles":
			handleProfiles(w, r, c)
		default:
			http.NotFound(w, r)
		}
	})
}

func handleProfiles(w http.ResponseWriter, r *http.Request, c client.Client) {
	ctx := r.Context()
	data, err := BuildProfilesResponse(ctx, c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
