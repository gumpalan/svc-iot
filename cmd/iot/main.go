package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gumpalan/svc-iot/internal/database"
	"github.com/gumpalan/svc-iot/migrations"
)

func main() {
	log.Println("Memulai Gumpalan IoT Service...")

	cfg, err := database.LoadConfig("iot")
	if err != nil {
		log.Fatalf("Gagal memuat konfigurasi database: %v", err)
	}

	db, dialect, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("Gagal membuka database: %v", err)
	}
	defer db.Close()

	if cfg.AutoMigrate {
		log.Printf("Menjalankan migrasi database (dialek: %s)...", dialect)
		if err := database.Migrate(db, dialect, migrations.FS, ".", "iot_migrations"); err != nil {
			log.Fatalf("Migrasi gagal: %v", err)
		}
		log.Println("Migrasi database berhasil.")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"svc-iot"}`))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8002"
	}

	addr := fmt.Sprintf("127.0.0.1:%s", port)
	log.Printf("Server svc-iot berjalan di http://%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server berhenti dengan galat: %v", err)
	}
}
