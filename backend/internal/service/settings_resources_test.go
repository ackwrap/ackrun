package service

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestGeoIPProviderResources(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewSettingsService(db)

	response, err := svc.ListGeoIPProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Items) == 0 || len(response.Templates) == 0 || response.Items[0].Key != "songzixian" || !response.Items[0].IsDefault {
		t.Fatalf("seeded providers = %+v", response)
	}

	created, err := svc.CreateGeoIPProvider(&model.GeoIPProviderRequest{Name: "Custom Geo", Template: "songzixian", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if created.Builtin || created.URL == "" || created.Mapping.Country == "" {
		t.Fatalf("created provider = %+v", created)
	}
	created.IsDefault = true
	updated, err := svc.UpdateGeoIPProvider(created.ID, &model.GeoIPProviderRequest{
		Name: created.Name, Template: created.Template, URL: created.URL, IPParameter: created.IPParameter,
		Mapping: created.Mapping, Enabled: true, IsDefault: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.IsDefault {
		t.Fatalf("updated provider = %+v", updated)
	}
}

func TestGeoIPProviderRejectsPrivateCustomEndpoint(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewSettingsService(db)
	_, err = svc.CreateGeoIPProvider(&model.GeoIPProviderRequest{
		Name: "Private", Template: "custom", URL: "https://127.0.0.1/geo", IPParameter: "ip",
		Mapping: model.GeoIPFieldMapping{Country: "country"}, Enabled: true,
	})
	if !errors.Is(err, ErrGeoIPProviderInvalid) {
		t.Fatalf("error = %v", err)
	}
}

func TestConnectivityTargetCannotRemoveActiveTarget(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewSettingsService(db)
	target, err := svc.CreateConnectivityTarget(&model.ConnectivityTargetRequest{Name: "Custom", URL: "https://connectivity.example/check", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetConnectivitySettings(&model.ConnectivitySettings{TestURL: target.URL, IntervalSeconds: 300}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteConnectivityTarget(target.ID); !errors.Is(err, ErrConnectivitySettingsInvalid) {
		t.Fatalf("delete error = %v", err)
	}
	if _, err := svc.UpdateConnectivityTarget(target.ID, &model.ConnectivityTargetRequest{Name: target.Name, URL: "https://other.example/check", Enabled: true}); !errors.Is(err, ErrConnectivitySettingsInvalid) {
		t.Fatalf("update active URL error = %v", err)
	}
}
