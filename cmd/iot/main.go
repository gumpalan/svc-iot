package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gumpalan/svc-iot/internal/config"
	"github.com/gumpalan/svc-iot/internal/core"
	"github.com/gumpalan/svc-iot/internal/database"
	"github.com/gumpalan/svc-iot/internal/httpapi"
	"github.com/gumpalan/svc-iot/internal/mqtt"
	"github.com/gumpalan/svc-iot/internal/store"
	"github.com/gumpalan/svc-iot/migrations"
)

func main() {
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
		if err := database.Migrate(db, dialect, migrations.FS, ".", "iot_migrations"); err != nil {
			log.Fatalf("Migrasi gagal: %v", err)
		}
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
		devCfg = &config.Config{}
	}

	// 2. Inisialisasi Registry
	registry := core.NewDeviceRegistry(devCfg)

	// CLI Dispatcher
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "perm", "permission":
			handlePermCLI(st, args[1:])
			return
		case "device":
			handleDeviceCLI(registry, args[1:])
			return
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}

	log.Println("Memulai Gumpalan IoT Service...")
	log.Printf("Berhasil memuat konfigurasi perangkat dari %s (%d perangkat terdaftar)", devConfigPath, len(devCfg.Devices))

	// 3. Inisialisasi State Hub
	hub := core.NewStateHub(registry, nil)

	// 4. Inisialisasi MQTT client secara asinkron agar tidak memblokir HTTP listener jika broker offline
	mqttCfg := mqtt.LoadConfig()
	go func() {
		mqttClient, err := mqtt.NewClient(mqttCfg, func(topic string, payload []byte) {
			hub.HandleInboundMQTT(topic, payload)
		})
		if err != nil {
			log.Printf("[MQTT] Gagal menyambung ke broker %s: %v (berjalan tanpa broker)", mqttCfg.Broker, err)
			return
		}
		hub.SetPublisher(mqttClient)
		log.Printf("[MQTT] Client aktif terhubung ke broker %s", mqttCfg.Broker)

		// Subscribe topik seluruh perangkat
		subTopics := registry.AllSubscriptionTopics()
		if len(subTopics) > 0 {
			if err := mqttClient.SubscribeTopics(subTopics); err != nil {
				log.Printf("[MQTT] Gagal subscribe topik: %v", err)
			}
		}
	}()

	// 5. Inisialisasi HTTP & WebSocket Router
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

func handlePermCLI(st *store.Store, args []string) {
	ctx := context.Background()
	if len(args) == 0 || args[0] == "list" {
		devID := ""
		if len(args) > 1 {
			devID = args[1]
		}
		perms, err := st.ListPermissions(ctx, devID)
		if err != nil {
			log.Fatalf("Gagal mengambil daftar permission: %v", err)
		}
		if len(perms) == 0 {
			fmt.Println("Belum ada izin perangkat khusus terdaftar (semua perangkat bebas diakses default).")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID	DEVICE ID	TARGET	TYPE	READ	WRITE	CREATED AT")
		for _, p := range perms {
			target := ""
			tType := ""
			if p.RoleID != nil && *p.RoleID != "" {
				target = *p.RoleID
				tType = "role"
			} else if p.UserID != nil && *p.UserID != "" {
				target = *p.UserID
				tType = "user"
			}
			tStr := time.Unix(p.CreatedAt, 0).Format("2006-01-02 15:04:05")
			fmt.Fprintf(w, "%s	%s	%s	%s	%v	%v	%s\n", p.ID, p.DeviceID, target, tType, p.CanRead, p.CanWrite, tStr)
		}
		w.Flush()
		return
	}

	switch args[0] {
	case "grant":
		var positionals []string
		isRole := false
		canWrite := true

		for _, arg := range args[1:] {
			switch arg {
			case "--role", "-r":
				isRole = true
			case "--readonly", "-ro":
				canWrite = false
			default:
				if strings.HasPrefix(arg, "-") {
					fmt.Printf("Flag tidak dikenal: %s\n", arg)
					os.Exit(1)
				}
				positionals = append(positionals, arg)
			}
		}

		if len(positionals) < 2 {
			fmt.Println("Penggunaan: iot perm grant <device_id> <target> [--role] [--readonly]")
			fmt.Println("Contoh:     iot perm grant lampu-kamar admin --role")
			fmt.Println("            iot perm grant lampu-kamar --role admin")
			os.Exit(1)
		}

		devID := positionals[0]
		target := positionals[1]

		b := make([]byte, 6)
		_, _ = rand.Read(b)
		permID := "perm_" + hex.EncodeToString(b)

		p := &store.DevicePermission{
			ID:       permID,
			DeviceID: devID,
			CanRead:  true,
			CanWrite: canWrite,
		}
		if isRole {
			p.RoleID = &target
		} else {
			p.UserID = &target
		}

		if err := st.CreatePermission(ctx, p); err != nil {
			log.Fatalf("Gagal memberikan izin: %v", err)
		}
		targetType := "user"
		if isRole {
			targetType = "role"
		}
		fmt.Printf("Izin berhasil diberikan! ID: %s (%s: %s -> device: %s, write: %v)\n", permID, targetType, target, devID, canWrite)

	case "revoke":
		if len(args) < 2 {
			fmt.Println("Penggunaan: iot perm revoke <perm_id>")
			os.Exit(1)
		}
		permID := args[1]
		if err := st.DeletePermission(ctx, permID); err != nil {
			log.Fatalf("Gagal mencabut izin: %v", err)
		}
		fmt.Printf("Izin %s berhasil dicabut.\n", permID)

	default:
		printUsage()
	}
}

func handleDeviceCLI(registry *core.DeviceRegistry, args []string) {
	devs := registry.ListDevices()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "DEVICE ID	NAME	DRIVER	CHANNELS")
	for _, d := range devs {
		chNames := make([]string, 0, len(d.Channels))
		for _, ch := range d.Channels {
			chNames = append(chNames, fmt.Sprintf("%s(%s)", ch.ID, ch.Type))
		}
		fmt.Fprintf(w, "%s	%s	%s	%s\n", d.ID, d.Name, d.Driver, strings.Join(chNames, ", "))
	}
	w.Flush()
}

func printUsage() {
	fmt.Println("Gumpalan IoT CLI")
	fmt.Println("\nPenggunaan:")
	fmt.Println("  iot perm list [device_id]                           Tampilkan daftar izin akses perangkat")
	fmt.Println("  iot perm grant <device_id> <target> [--role] [--readonly]  Beri izin akses ke user atau role")
	fmt.Println("  iot perm revoke <perm_id>                           Cabut izin akses")
	fmt.Println("  iot device list                                     Tampilkan perangkat terdaftar dari config")
}
