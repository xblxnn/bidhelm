package main

import (
	"bidhelm/internal/adapters"
	"bidhelm/internal/adapters/exads_json_pop"
	"bidhelm/internal/app"
	rt "bidhelm/internal/runtime"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func main() {
	// Explicit adapter registration: adding a package requires rebuilding the image.
	registry := adapters.Registry{}
	exads := exads_json_pop.Adapter{}
	registry[exads.Key()] = exads
	geo, err := rt.NewEnricher(app.Env("MMDB_PATH", "data/country.mmdb"))
	if geo == nil {
		log.Fatal(err)
	}
	defer geo.Close()
	engine := &rt.Engine{Registry: registry, Token: app.Token(), Enrich: geo.Facts}
	if err != nil {
		engine.GeoError = err.Error()
		log.Print(err)
	}
	upstream, _ := url.Parse("http://127.0.0.1:8081")
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	public := http.NewServeMux()
	public.HandleFunc("/source/", engine.Traffic)
	public.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if !engine.Status().Ready {
			http.Error(w, "not ready", 503)
			return
		}
		w.Write([]byte("ready"))
	})
	public.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/") {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})
	ctx, cancel := app.Context()
	defer cancel()
	if err = app.Serve(ctx, app.Server(app.Env("LISTEN_ADDR", "127.0.0.1:8080"), public), app.Server("127.0.0.1:8082", engine.Internal())); err != nil {
		log.Fatal(err)
	}
}
