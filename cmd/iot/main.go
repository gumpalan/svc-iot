package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gumpalan/svc-iot/internal/config"
	"github.com/gumpalan/svc-iot/internal/core"
	"github.com/gumpalan/svc-iot/internal/database"
	"github.com/gumpalan/svc-iot/internal/httpapi"
	"github.com/gumpalan/svc-iot/internal/mqtt"
	"github.com/gumpalan/svc-iot/internal/store"
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

	st := store.New(db, dialect)

	// 1. Muat deklarasi perangkat dari berkas YAML
	devConfigPath := os.Getenv("DEVICES_CONFIG_PATH")
	if devConfigPath == "" {
		devConfigPath = "config/devices.yaml"
		if _, err := os.Stat(devConfigPath); os.IsNotExist(err) {
			devConfigPath = "config/devices.example.yaml"
		}
	}

	devCfg, err := config.LoadDevicesFile(devConfigPath)
	if err != nil {
		log.Printf("[Peringatan] Gagal memuat %s: %v. Menggunakan registri kosong.", devConfigPath, err)
		devCfg = &config.Config{}
	} else {
		log.Printf("Berhasil memuat konfigurasi perangkat dari %s (%d perangkat terdaftar)", devConfigPath, len(devCfg.Devices))
	}

	// 2. Inisialisasi Registry dan State Hub
	registry := core.NewDeviceRegistry(devCfg)
	hub := core.NewStateHub(registry, nil)

	// 3. Inisialisasi MQTT client
	mqttCfg := mqtt.LoadConfig()
	mqttClient, err := mqtt.NewClient(mqttCfg, func(topic string, payload []byte) {
		hub.HandleInboundMQTT(topic, payload)
	})
	if err != nil {
		log.Printf("[MQTT] Gagal menyambung ke broker %s: %v (berjalan tanpa koneksi broker aktif)", mqttCfg.Broker, err)
	} else {
		defer mqttClient.Close()
		hub.SetPublisher(mqttClient)

		// Subscribe topik seluruh perangkat
		subTopics := registry.AllSubscriptionTopics()
		if len(subTopics) > 0 {
			if err := mqttClient.SubscribeTopics(subTopics); err != nil {
				log.Printf("[MQTT] Gagal subscribe topik: %v", err)
			}
		}
	}

	// 4. Inisialisasi HTTP & WebSocket Router
	mux := http.NewServeMux()
	handler := httpapi.NewHandler(registry, hub, st)
	handler.RegisterRoutes(mux)

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
