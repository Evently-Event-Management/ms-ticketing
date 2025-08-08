package main

import (
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewRouter()

	// Root handler - shows message on visiting http://localhost:8080/
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "API Gateway is running! 🚀")
	})

	// Mount microservice routes
	r.Mount("/order_api/v1/order", proxy("http://order:8001"))
	r.Mount("/order_api/v1/seating", proxy("http://seating:8002"))
	r.Mount("/order_api/v1/payment", proxy("http://payment:8003"))

	// Health check endpoint
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "OK")
	})

	log.Println("🌐 API Gateway listening on :8080")
	err := http.ListenAndServe(":8080", r)
	if err != nil {
		log.Fatalf("❌ Gateway failed: %v", err)
	}
}

func proxy(target string) http.Handler {
	remote, err := url.Parse(target)
	if err != nil {
		panic("Invalid target URL: " + target)
	}

	return httputil.NewSingleHostReverseProxy(remote)
}
