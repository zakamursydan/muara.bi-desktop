# Peta paket engine (M1.1) — apa yang bisa dibawa desktop, apa yang tidak

Diukur 2026-10-31 dari `muara.bi-service/backend` (module `muara.bi/core`), bukan dari tebakan:
`go list -f '{{join .Imports " "}}'` per paket dan `go list -deps` untuk closure.

## 1. Enam paket leaf = closure bersih

Kandidat engine (yang disebut BRD §16/ADR-0001) dan import internal mereka:

| Paket | Import `muara.bi/core/...` | Dependensi non-standar |
|---|---|---|
| `internal/duckdb` | `internal/helper/sqlident` | `github.com/marcboeker/go-duckdb` v1.8.5, `go.mongodb.org/mongo-driver` |
| `internal/exporter` | `internal/duckdb` | `github.com/xuri/excelize/v2` |
| `internal/ssh` | — | `golang.org/x/crypto/ssh` |
| `internal/helper/cryptohelper` | — | (stdlib crypto only) |
| `internal/helper/sqlident` | — | (stdlib regexp only) |
| `internal/ai` | — | (stdlib + `net/http` only) |

`go list -deps` atas keenamnya **hanya** menghasilkan keenam paket itu + `helper/sqlident`. Tidak ada
`config`, `domain`, `dto`, `repository`, `cache`, `limiter`, `tasks`, `pkg/*`. Artinya: ekstraksi
menjadi modul Go sendiri tidak menyeret satu pun istilah tenant, dan tidak perlu adapter. Ini batas
M1.3.

## 2. Konsumen di sisi web (yang harus tetap hijau sesudah `git mv`)

| Paket engine | Konsumen |
|---|---|
| `duckdb` | `internal/app/server`, `internal/cache`, `internal/dto/response`, `internal/exporter`, `internal/service`, `internal/worker` |
| `exporter` | `internal/worker` |
| `ai` | `internal/service` |
| `ssh` | `internal/app/server`, `internal/service` |
| `helper/cryptohelper` | `internal/app/server`, `internal/service` |
| `helper/sqlident` | `internal/duckdb`, `internal/service` |

Enam titik impor, semuanya di bawah `internal/` — penggantiannya search-replace per file, tidak ada
perubahan semantik. `cmd/api` dan `cmd/worker` ikut karena transitif.

## 3. Yang TIDAK bisa ikut pindah apa adanya: lapisan orkestrasi

`internal/service` adalah satu paket (satu `package service`) berisi dua jenis kode yang tercampur.
Impor per berkas, engine-adjacent vs tenant-bound:

| Berkas | Import tenant | Catatan |
|---|---|---|
| `widget_builder.go` | `domain/entity`, `helper/sqlident` | **hampir bersih** — lihat §4 |
| `rls.go` | `domain/entity`, `helper/sqlident`, `pkg/customerror` | aturan baris generik, tapi pakai entity |
| `session_preparer.go` | `domain/entity`, `repository/pgsql`, `limiter`, `pkg/customerror`, `pkg/logging` | satu jalur eksekusi yang BRD sebut sebagai reuse |
| `connection_service.go` | + `dto/request`, `dto/response`, `cryptohelper`, `ssh`, `repository/pgsql` | CRUD koneksi multi-tenant |
| `semantic_service.go`, `query_service.go`, `alert_service.go` | `repository/pgsql`, `cache`, `tasks`, `dto/*` | tetap di web |

Bagian yang menempel tenant di `session_preparer.go` konkret dan sedikit (`:45-49`): dua interface
repository (`pgsql.ConnectionRepositoryProvider`, `pgsql.SemanticRepositoryProvider`), satu
`*limiter.Semaphore`, satu `*logging.Logger`, plus `customerror` untuk peta HTTP dan `entity.DataConnection` /
`entity.ConnectionConfig` sebagai tipe input.

**Keputusan M1.3:** jangan pindahkan `session_preparer.go`. Pindahkan *potongan yang tidak tahu
tenant* ke engine sebagai fungsi murni dengan tipe plain:

- guardrail → bind → RLS → LIMIT sudah ada di `internal/duckdb/guardrails.go` (sudah bersih, tidak
  perlu diapa-apakan);
- yang perlu diekspos dari engine: `Attach*` dan `MountSemanticModels` menerima **struct input milik
  engine** (alias, driver, DSN, daftar model), bukan `entity.DataConnection`. Web memetakan entity →
  struct itu; desktop memetakan entri bundle → struct yang sama;
- `limiter` dan `cache` jadi parameter opsional di sisi pemanggil, bukan knowledge engine;
- `cryptohelper` hari ini memakai master key dari env. Untuk desktop, kunci datang dari keychain →
  engine hanya menerima `[]byte` key, tidak pernah mencari sendiri (konsisten dengan kontrak keamanan
  "engine tidak melakukan lookup kredensial").

Konsekuensi yang harus diputuskan sebelum `git mv`: **engine module tidak boleh mengimpor
`muara.bi/core/pkg/...`** (customerror, logging) — itu akan membuat dependensi balik ke modul web dan
menutup siklus. `pkg/` tidak masuk closure §1, jadi hari ini tidak ada pelanggaran; tapi orkestrasi
baru di engine harus memakai error sentinel biasa dan membiarkan lapisan HTTP web yang memetakannya.

## 4. `widget_builder.go`: satu-satunya berkas service yang benar-benar ikut

Kompilator BuilderState → SQL hanya memakai `entity.WidgetBuilderState`, `entity.BuilderMetric`,
`entity.BuilderFilter`. Ketiganya didefinisikan di `internal/domain/entity/widget_builder.go` — 31
baris, **tanpa satu pun import**, tanpa kolom tenant. Jadi yang pindah adalah berkas entity itu,
bukan paket `domain/entity` (yang berisi `user.go`, `permission.go`, `project.go`, `share.go`,
`alert.go` — semuanya istilah multi-tenant dan tidak boleh ada di engine).

Revisi daftar `git mv` di PLAN M1: `service/widget_builder.go` + `domain/entity/widget_builder.go`
(dua berkas), bukan `domain/entity` (satu paket).

## 5. Verifikasi M1.2 — CGO fail-fast

Sebelum: `session_fallback.go` (`//go:build !cgo`) membuat **build tanpa CGO berhasil dan
berbohong** — `AttachPostgres/AttachMySQL/AttachSQLite` mengembalikan `nil` setelah menyimpan string
DSN, dan `Query` menjawab dari `jsonTables` atau baris `{status: OK, message: "query executed via
fallback engine"}`. Produk desktop yang ter-build tanpa mingw akan menampilkan angka karangan tanpa
suara.

Sesudah (`internal/duckdb/session_fallback.go` ditulis ulang): `NewEngine` mengembalikan
`duckdb.ErrCGODisabled` (sentinel baru di `types.go`), dan setiap metode stub mengembalikan error yang
sama — tidak ada satu pun jalur yang bisa menghasilkan `QueryResult`. Tipe tetap dideklarasikan agar
`go build`/`go vet` di mesin tanpa C toolchain tetap jalan. `internal/app/server/service.go:60` sudah
`logger.Fatal` saat `NewEngine` error, jadi fail-fast-nya mengalir sampai proses tanpa perubahan API.

Dua tes baru: `cgo_disabled_test.go` (`//go:build !cgo`, membuktikan refusal dan `res == nil`) dan
`cgo_enabled_test.go` (`//go:build cgo`, membuktikan `SELECT 1` dijawab DuckDB — guard agar build tag
tidak pernah kebalik). Tiga tes lama yang memakai engine kini `t.Skip(ErrCGODisabled)` alih-alih
`t.Fatalf`, supaya mesin tanpa CGO bisa menjalankan suite tanpa hasil palsu.

Satu temuan ikut dibereskan: `cmd/duckdb-bundle/main.go` mengimpor `go-duckdb` tanpa build tag, dan
`go-duckdb` v1.8.5 **tidak bisa di-compile sama sekali** dengan `CGO_ENABLED=0`
(`transaction.go:6:5: undefined: Conn`). Sebelum ini, `CGO_ENABLED=0 go build ./...` selalu gagal
karena tool itu — sehingga kontrak "gagal cepat tanpa CGO" tidak bisa diuji di CI. Sekarang diberi
`//go:build cgo`.

Hasil di host (darwin/arm64):

```
go build ./...                     → ok
go test ./...                      → EXIT=0
CGO_ENABLED=0 go build ./...       → ok
CGO_ENABLED=0 go vet ./...         → ok
CGO_ENABLED=0 go test ./...        → EXIT=0  (TestEngine_WithoutCGORefuses PASS, tes engine lain SKIP)
```

## 6. Eksekusi M1.3 (2026-10-31) — batas di atas jadi modul sungguhan

Tabel §1-§4 di atas adalah **pengukuran sebelum pindah**; paths-nya memakai nama lama. Setelah M1.3,
yang dibaca desktop adalah:

```
muara.bi-service/backend/
├── go.mod                  module muara.bi/core   + require muara.bi/engine v0.0.0
│                                                    + replace muara.bi/engine => ./engine
└── engine/                 module muara.bi/engine (go 1.27.1, punya go.sum sendiri)
    ├── duckdb/  exporter/  ai/  ssh/  cryptohelper/  sqlident/  builder/
```

`builder/` isi dua berkas hasil penggabungan yang direncanakan §4: `state.go` (3 struct, dulu
`domain/entity/widget_builder.go`) dan `compile.go` (dulu `service/widget_builder.go`). `package
service`/`entity` → `package builder`; prefix `entity.` dihapus; satu-satunya konsumen
`internal/service/widget_service.go` kini memanggil `builder.CompileBuilderSQL(builder.WidgetBuilderState)`.

Bukti akhir, diukur (bukan diasumsikan):

- `grep -r "muara.bi/core" backend/engine/` → **nol**. Aturan anti-siklus §3 tegak.
- Dependensi eksternal engine tepat empat: `marcboeker/go-duckdb`, `xuri/excelize/v2`,
  `go.mongodb.org/mongo-driver`, `golang.org/x/crypto/ssh`. Tidak `pgx`, tidak `chi`, tidak `asynq`,
  tidak `goose`, tidak `redis` → desktop tidak menarik server web.
- Istilah tenant di API engine dinolkan: `duckdb.SemanticModel` kehilangan `WorkspaceID`/`ProjectID`
  (dulu satu-satunya bocoran, `semantic_layer.go:14-15`). Kepemilikan tetap di `entity.SemanticModel`
  web; pemetaan terjadi di `session_preparer.go:142` yang **tidak** pindah — persis keputusan §3.
- `helper/dberr` sengaja ditinggal di web: ia memetakan kode error Postgres (23505 → 409), urusan
  orkestrator, dan akan menyeret `pgx` ke dalam engine.
- Gerbang 8/8 hijau: `go build`/`go vet`/`go test` di **dua** modul × `CGO_ENABLED=1` dan `CGO_ENABLED=0`;
  `gofmt -l` bersih di repo.

Dua jebakan yang ditemukan saat menutup M1.3 dan sudah ikut dibetulkan:

1. **`./...` berhenti menjangkau engine.** Modul bersarang otomatis dikeluarkan dari main module, jadi
   target lama (`make test`, `go test ./...`) tetap exit 0 tanpa pernah menjalankan satu tes DuckDB pun.
   `Makefile` sekarang menjalankan engine lebih dulu, baru core, di `test`/`vet`/`tidy`.
2. **Urunan COPY di Dockerfile.** `go mod download` dengan `replace` ke direktori lokal butuh `go.mod`
   targetnya ada; stage build kini men-COPY `backend/engine/go.mod` + `go.sum` ke `./engine/` sebelum
   download. Sudah dibuktikan dengan `make image-core` pada 2026-10-31: download 33 dtk tanpa error,
   dan binary di dalam image mencatat `muara.bi/engine v0.0.0` tanpa hash (= resolve dari `replace` lokal).
   Untuk desktop ini bukti penting: engine yang sama bisa dipakai dua produk tanpa salinan kedua.

## 7. Permukaan API engine setelah M1.4–M1.5 (2026-10-31)

Dua kontrak berubah di sisi yang harus dipakai desktop; keduanya sudah ditegakkan **di engine**, jadi
implementasi desktop tidak perlu mengulang policing-nya:

- `Session.Query(ctx, sql string, opts QueryOptions, args ...any)` — `QueryOptions{MaxRows, CappedSources}`.
  Cap ditegakkan di sini (eksekusi memakai `LIMIT cap+1`, baris ekstra jadi bukti pemotongan lalu dibuang),
  **bukan** lewat teks SQL. `QueryResult` membawa `RowLimit`, `Truncated`, `TruncatedBy`
  (`row_limit` · `source_cap`) dan `CappedSources`; prioritas `row_limit` di atas `source_cap`.
  Kalau desktop memanggil `ApplyLimit` sendiri sebelum `Query`, probe-nya mati dan semua hasil tampak utuh.
- `GuardrailConfig.MaxMemory` (`"2GB"` default, `duckdb.DefaultMaxMemory`) — divalidasi `NormalizeMaxMemory`
  di `NewEngine`, sehingga nilai salah mengembalikan error alih-alih budget lama. Desktop memasok ini dari
  settings (FR-15); web dari `DUCKDB_MAX_MEMORY`.
  **Unit yang diterima (diprobe 2026-11-01): `B|KB|MB|GB|TB` desimal *dan* `KiB|MiB|GiB|TiB` biner**,
  angka wajib + tanpa spasi. Artinya FR-15 cukup menulis `GiB` dan mengirimnya utuh — tidak ada konversi,
  tidak ada label yang bohong. Yang ditolak engine walau DuckDB menerima: `"-1"` (tanpa batas), `"2 GB"`.
  Kalau `max_memory` tidak di-SET sama sekali DuckDB mengambil **80% RAM**, jadi nilai harus selalu eksplisit.
