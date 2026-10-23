# svc-iot

Layanan backend IoT modular untuk platform Gumpalan. Dirancang agar dapat disesuaikan dengan beragam perangkat keras otomasi rumah tanpa mengubah kode inti.

> **Catatan modul**: Path modul `github.com/gumpalan/svc-iot` pada `go.mod` adalah placeholder.

## Status

**Scaffold** (kerangka awal dengan migrasi SQLite/MySQL/PostgreSQL, abstraksi driver murni, stub driver Tasmota & Generic MQTT, serta healthz endpoint).

## Empat Tingkat Penyesuaian IoT

Sistem dirancang dengan 4 level ekstensibilitas (dari yang paling mudah):

1. **Konfigurasi (`config/devices.yaml`)**:
   Hubungkan perangkat baru ke driver bawaan (`tasmota` atau `generic_mqtt`) tanpa menulis kode satu baris pun.
2. **Discovery Otomatis (MQTT)** *(rencana)*:
   Perangkat mengirim payload retained ke topik `gumpalan/discovery/<id>` berisi daftar kapabilitasnya. `svc-iot` akan mendaftarkannya otomatis ke tabel database `iot_devices`.
3. **Adapter Eksternal** *(rencana)*:
   Proses mandiri (Python, Node.js, Go) yang menerjemahkan protokol pihak ketiga (Zigbee2MQTT, BLE, Tuya, dll.) ke struktur pesan topik MQTT Gumpalan.
4. **Driver Go Mandiri (`pkg/driver`)**:
   Repositori atau paket eksternal mengimpor `pkg/driver`, mengimplementasikan antarmuka murni `driver.Driver`, dan didaftarkan saat inisialisasi tanpa melakukan fork kode inti.

## Struktur Repositori

```
svc-iot/
├── cmd/iot/main.go               # Titik masuk aplikasi (:8002)
├── pkg/driver/                   # Antarmuka driver publik (dapat di-import pihak ketiga)
│   └── driver.go
├── internal/
│   ├── config/                   # Parser config/devices.yaml
│   ├── core/                     # Registri perangkat, cache state, event bus
│   ├── database/                 # Koneksi multi-mesin, dialect rebind, auto-migrate
│   ├── discovery/                # Listener topik discovery otomatis
│   ├── driver/                   # Driver bawaan (tasmota, genericmqtt)
│   ├── httpapi/                  # REST handler OpenAPI
│   ├── mqtt/                     # Manajemen koneksi MQTT broker lokal
│   └── store/                    # Akses tabel database iot_devices
├── config/
│   └── devices.example.yaml      # Contoh deklarasi perangkat statis
├── migrations/
│   ├── 0001_init.sql             # Skema goose portabel tabel iot_devices
│   └── fs.go                     # embed.FS berkas migrasi
├── .env.example                  # Variabel lingkungan
├── Makefile                      # Build, test, dan cross-compile arm64
└── README.md
```

## Konfigurasi Database

Database ditentukan via environment variable (kompatibel Laravel):

| Variabel | Pilihan Nilai | Default | Keterangan |
| --- | --- | --- | --- |
| `DB_CONNECTION` | `sqlite`, `mysql`, `mariadb`, `pgsql` | `sqlite` | Mesin database |
| `DB_DATABASE` | String | `./data/iot.db` | Path berkas sqlite atau nama database |
| `DB_HOST` | Host / IP | `127.0.0.1` | Host database server |
| `DB_PORT` | Angka | `3306` / `5432` | Port server database |
| `DB_USERNAME` | String | - | Username database |
| `DB_PASSWORD` | String | - | Password database |
| `DB_SSLMODE` | `disable`, `require`, dll | `disable` | Khusus PostgreSQL |
| `DB_URL` | DSN lengkap | - | Menimpa variabel di atas jika diisi |
| `DB_AUTO_MIGRATE` | `true`, `false` | `true` | Menjalankan migrasi saat startup |

## Rencana Pengujian Database Tambahan

Unit test database saat ini memverifikasi parser konfigurasi, dialek rebind PostgreSQL, dan operasi SQLite in-memory. Pengujian integrasi MariaDB dan PostgreSQL akan ditambahkan melalui skrip pengujian berbasis kontainer.

## Lisensi dan Dependensi

Lisensi repositori ini **belum ditentukan**.

### Dependensi Langsung:
- `modernc.org/sqlite` (BSD-3-Clause) - Driver SQLite pure Go tanpa CGO.
- `github.com/go-sql-driver/mysql` (MPL-2.0) - Driver MySQL/MariaDB (copyleft tingkat berkas; aman digunakan sebagai pustaka tanpa modifikasi berkas driver).
- `github.com/jackc/pgx/v5` (MIT) - Driver PostgreSQL stdlib.
- `github.com/pressly/goose/v3` (MIT) - Pustaka migrasi database.
- `github.com/joho/godotenv` (MIT) - Parser berkas .env.
- `gopkg.in/yaml.v3` (MIT / Apache-2.0) - Parser konfigurasi YAML.

### Memeriksa Lisensi:
```bash
go run github.com/google/go-licenses@latest report ./...
```
