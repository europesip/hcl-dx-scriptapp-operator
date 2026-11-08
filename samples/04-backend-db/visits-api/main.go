// visits-api: the tiny backend of sample 04. It counts the visits of each DX user in PostgreSQL.
//   GET  /dxop-visits/api/visits          -> {"total": n, "users": n}
//   POST /dxop-visits/api/visits?user=uid -> {"user": "...", "count": n, "total": n}
// DATABASE_URL comes from the Secret the operator writes for the DXDatabase resource (key "uri").
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	for i := 0; ; i++ { // the table, once the database answers
		_, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS visits (uid text PRIMARY KEY, count int NOT NULL DEFAULT 0, last timestamptz)`)
		if err == nil || i == 30 {
			break
		}
		log.Printf("waiting for the database: %v", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal(err)
	}
	reply := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(v)
	}
	http.HandleFunc("/dxop-visits/api/visits", func(w http.ResponseWriter, r *http.Request) {
		var total, users int
		if r.Method == http.MethodPost {
			uid := r.URL.Query().Get("user")
			if uid == "" || len(uid) > 128 {
				http.Error(w, "user is required", http.StatusBadRequest)
				return
			}
			var count int
			if err := pool.QueryRow(r.Context(), `INSERT INTO visits (uid, count, last) VALUES ($1, 1, now())
				ON CONFLICT (uid) DO UPDATE SET count = visits.count + 1, last = now() RETURNING count`, uid).Scan(&count); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = pool.QueryRow(r.Context(), `SELECT coalesce(sum(count),0), count(*) FROM visits`).Scan(&total, &users)
			reply(w, map[string]any{"user": uid, "count": count, "total": total, "users": users})
			return
		}
		_ = pool.QueryRow(r.Context(), `SELECT coalesce(sum(count),0), count(*) FROM visits`).Scan(&total, &users)
		reply(w, map[string]any{"total": total, "users": users})
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	log.Println("visits-api listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil)) //nolint:gosec
}
