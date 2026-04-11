package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autosizev1 "github.com/muandane/saturdai/api/v1"
)

func TestAPI_profiles_returnsJSON(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	if err := autosizev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	srv := httptest.NewServer(NewMux(cl))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/dashboard/v1/profiles")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body ProfilesResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
}

func TestStatic_dashboard_servesIndex(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	_ = autosizev1.AddToScheme(scheme)
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	srv := httptest.NewServer(NewMux(cl))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/dashboard/index.html")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}
