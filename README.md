# svc-iot

Layanan backend IoT modular untuk platform Gumpalan. Dirancang agar dapat disesuaikan dengan beragam perangkat keras otomasi rumah tanpa mengubah kode inti.

> **Catatan modul**: Path modul `github.com/gumpalan/svc-iot` pada `go.mod` adalah placeholder.

## Status

**Aktif & Siap Terintegrasi**:
- Parsing deklarasi perangkat statis fleksibel (`config/devices.yaml`) berbasis channel (input, output, bidirectional, tipe data, hint UI).
- Driver kustom fleksibel dan driver preset bawaan (Tasmota).
- Integrasi MQTT client (Paho MQTT) dengan auto-reconnect dan langganan dinamis.
- Cache state perangkat in-memory dan endpoint real-time WebSocket (`/iot/v1/ws`).
- Penyimpanan database SQLite/MySQL/PostgreSQL untuk layout dashboard pengguna (`iot_dashboards`) dan hak akses kontrol (`iot_device_permissions`).
- Fitur pencarian dan kloning layout dashboard pengguna lain dengan validasi izin perangkat yang sama.

## Struktur Repositori

```
svc-iot/
├── cmd/iot/main.go               # Titik masuk aplikasi (:8002)
├── pkg/driver/                   # Antarmuka driver publik
│   └── driver.go
├── internal/
│   ├── config/                   # Parser konfigurasi devices.yaml
│   ├── core/                     # Registri perangkat, cache state, WebSocket hub
│   ├── database/                 # Koneksi multi-mesin, dialect rebind, auto-migrate
│   ├── discovery/                # Listener topik discovery otomatis
│   ├── driver/                   # Driver bawaan (tasmota, genericmqtt)
│   ├── httpapi/                  # REST handler & WebSocket endpoint
│   ├── mqtt/                     # Manajemen koneksi MQTT broker (Paho MQTT)
│   └── store/                    # Penyimpanan dashboard pengguna & izin perangkat
├── config/
│   ├── devices.example.yaml      # Contoh deklarasi perangkat & channel
│   └── devices.yaml              # Konfigurasi aktif perangkat
├── migrations/
│   ├── 0001_init.sql             # Skema goose tabel iot_devices
│   ├── 0002_dashboards_and_permissions.sql # Skema tabel dashboard & izin akses
│   └── fs.go                     # embed.FS berkas migrasi
├── .env.example                  # Variabel lingkungan
├── Makefile                      # Build, test, dan cross-compile arm64
└── README.md
```

## Menjalankan Layanan

### Menjalankan Langsung:
```bash
make run
# atau
go run ./cmd/iot
```

### Menjalankan Pengujian:
```bash
make test
# atau
go test -v ./...
```

### Build Binary:
```bash
make build
```

## Konfigurasi Lingkungan (.env)

Variabel lingkungan yang didukung:

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
| `DB_AUTO_MIGRATE` | `true`, `false` | `true` | Menjalankan migrasi goose otomatis saat startup |
| `PORT` | Angka | `8002` | Port listener HTTP/WebSocket |
| `DEVICES_CONFIG_PATH`| Path berkas | `config/devices.yaml` | Lokasi berkas YAML deklarasi perangkat |
| `MQTT_BROKER` | Host / URL | `tcp://127.0.0.1:1883` | Alamat broker MQTT (misal `broker.emqx.io:1883`) |
| `MQTT_CLIENT_ID` | String | `gumpalan_svc_iot_<timestamp>` | ID client MQTT |
| `MQTT_USERNAME` | String | - | Username otentikasi broker MQTT |
| `MQTT_PASSWORD` | String | - | Password otentikasi broker MQTT |

## Format Deklarasi Perangkat (`devices.yaml`)

```yaml
devices:
  - id: "living-room-sensor"
    name: "Sensor Ruang Tamu"
    driver: "custom" # custom | tasmota | generic_mqtt
    channels:
      - id: "temperature"
        name: "Suhu"
        direction: "input" # input | output | bidirectional
        topic: "gumpalan/device/living-room/tele/temp"
        type: "number" # number | boolean | string | enum
        unit: "°C"
        min: 0
        max: 50
        ui_hint: "gauge"

      - id: "fan_power"
        name: "Power Kipas"
        direction: "bidirectional"
        command_topic: "gumpalan/device/living-room/cmd/fan"
        state_topic: "gumpalan/device/living-room/stat/fan"
        type: "boolean"
        ui_hint: "toggle"
```

## Daftar Endpoint REST & WebSocket

Semua endpoint modul berada di bawah prefix `/iot/v1/`:

| Metode | Endpoint | Keterangan |
| --- | --- | --- |
| `GET` | `/iot/v1/healthz` | Health check layanan |
| `GET` | `/iot/v1/ws` | Stream WebSocket dua arah (telemetri & command) |
| `GET` | `/iot/v1/devices` | Daftar perangkat dan status channel terkini |
| `GET` | `/iot/v1/devices/{id}` | Detail satu perangkat |
| `POST` | `/iot/v1/devices/{id}/command` | Kirim perintah ke channel perangkat (`{"channel":"...","value":...}`) |
| `GET` | `/iot/v1/dashboards` | Daftar layout dashboard milik pengguna aktif |
| `POST` | `/iot/v1/dashboards` | Buat layout dashboard baru |
| `GET` | `/iot/v1/dashboards/{id}` | Ambil layout dashboard berdasarkan ID |
| `PUT` | `/iot/v1/dashboards/{id}` | Perbarui layout dashboard |
| `DELETE`| `/iot/v1/dashboards/{id}` | Hapus layout dashboard |
| `GET` | `/iot/v1/dashboards/explore` | Jelajahi layout publik pengguna lain (terfilter perangkat yang sama) |
| `POST` | `/iot/v1/dashboards/{id}/clone` | Kloning layout dashboard publik ke akun pengguna aktif |
| `GET` | `/iot/v1/permissions` | Daftar izin akses perangkat (admin only, filter `?deviceId=...`) |
| `POST` | `/iot/v1/permissions` | Buat izin akses baru untuk role/user (admin only) |
| `DELETE`| `/iot/v1/permissions/{id}` | Cabut izin akses perangkat (admin only) |

## Perintah CLI Administrasi

Perintah CLI `iot` dapat dijalankan langsung melalui binary hasil compile atau `go run ./cmd/iot`:

```bash
# 1. Tampilkan daftar perangkat terdaftar dari konfigurasi YAML
go run ./cmd/iot device list

# 2. Tampilkan semua izin perangkat yang tersimpan di database
go run ./cmd/iot perm list
go run ./cmd/iot perm list living-room-sensor

# 3. Beri izin akses ke user atau role tertentu
go run ./cmd/iot perm grant living-room-sensor role_user --role
go run ./cmd/iot perm grant living-room-sensor usr_123456
go run ./cmd/iot perm grant living-room-sensor usr_123456 --readonly

# 4. Cabut izin akses
go run ./cmd/iot perm revoke perm_4879dad00e24
```

> **Catatan Akses:** Jika suatu perangkat belum memiliki baris izin di database, maka perangkat tersebut dapat diakses oleh semua pengguna terautentikasi (default terbuka). Setelah ada satu atau lebih izin, hanya user atau role yang terdaftar (atau role `admin`) yang dapat melihat dan mengontrol perangkat tersebut.

## Lisensi dan Dependensi

- `modernc.org/sqlite` (BSD-3-Clause)
- `github.com/go-sql-driver/mysql` (MPL-2.0)
- `github.com/jackc/pgx/v5` (MIT)
- `github.com/pressly/goose/v3` (MIT)
- `github.com/joho/godotenv` (MIT)
- `gopkg.in/yaml.v3` (MIT / Apache-2.0)
- `github.com/eclipse/paho.mqtt.golang` (EPL-2.0 / BSD-3-Clause)
- `github.com/gorilla/websocket` (BSD-2-Clause)
