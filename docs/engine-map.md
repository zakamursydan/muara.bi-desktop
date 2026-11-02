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
- `GuardrailConfig.MaxMemory` (`duckdb.DefaultMaxMemory = "2GiB"` bila kosong) — divalidasi `NormalizeMaxMemory`
  di `NewEngine`, sehingga nilai salah mengembalikan error alih-alih budget lama. Desktop memasok ini dari
  settings (FR-15); web dari `DUCKDB_MAX_MEMORY`.
  **Unit yang diterima (diprobe 2026-11-01): `B|KB|MB|GB|TB` desimal *dan* `KiB|MiB|GiB|TiB` biner**,
  angka wajib + tanpa spasi. Artinya FR-15 cukup menulis `GiB` dan mengirimnya utuh — tidak ada konversi,
  tidak ada label yang bohong. Yang ditolak engine walau DuckDB menerima: `"-1"` (tanpa batas), `"2 GB"`.
  Kalau `max_memory` tidak di-SET sama sekali DuckDB mengambil **80% RAM**, jadi nilai harus selalu eksplisit.
- `GuardrailConfig.Threads` (`duckdb.DefaultThreads = 2` bila `0`) — divalidasi `NormalizeThreads` di
  `NewEngine` (2026-11-02): negatif **ditolak**, nilai di atas `runtime.NumCPU()` **dijepit**. Web memasok ini
  dari `DUCKDB_THREADS`, desktop dari `threadBudget()`. Detail dan tabel probe di §19.
- `Session.RegisterFileSource(ctx, viewName, path string) error` (2026-11-02) — mendaftar **satu berkas**
  sebagai temp view lewat reader core DuckDB; pemetaan ekstensi dan bentuk query ada di
  `duckdb.FileSourceQuery` (§15). Nama view divalidasi `sqlident.IsPlain` dan path masuk sebagai string literal
  hasil `quoteLiteral`, jadi desktop tidak perlu (dan tidak boleh) merakit SQL-nya sendiri. Ini jalur sumber
  file; `Attach*` tetap untuk database.
- `Session.DescribeSource(ctx, alias string) ([]CatalogTable, error)` (2026-11-02, diperluas hari yang sama
  — §21) — membaca **metadata** tabel, kolom, kunci, komentar dan taksiran baris dari catalog yang sudah
  di-ATTACH, lewat fungsi katalog DuckDB sendiri. Ini satu-satunya jalur yang
  berlaku untuk ketiga scanner SQL (alasan terukur di §20), jadi desktop tidak perlu — dan tidak boleh — menulis
  query `information_schema` sendiri. Tidak ada baris data yang dibaca; hasil kosong berarti "tidak ada yang
  terlihat", bukan error, sehingga keputusan "cache tidak boleh tertimpa skema kosong" ada di pemanggil.

## 8. Spike M0 (2026-11-01) — engine hidup di modul kedua, dan bundel extension ternyata tidak offline

Bukti pertama bahwa janji M1 berlaku: `muara.bi-desktop/go.mod` berisi
`replace muara.bi/engine => ../muara.bi-service/backend/engine` dan `cmd/spike/` memanggil engine
tanpa satu baris pun kode web. Yang terukur di mesin ini:

- `SELECT 1, current_setting('max_memory'), current_setting('threads')` → `2.0 GiB` dan `threads: 2`.
  Spike sengaja **tidak** mengisi `MaxMemory`, jadi angka ini membuktikan default engine — bukan config
  web — yang sampai ke DuckDB.
- `CGO_ENABLED=0 go build` tetap berhasil, dan menjalankan binary-nya menolak dengan
  `duckdb engine unavailable: binary built without CGO (rebuild with CGO_ENABLED=1 and a C toolchain)`
  (exit 1). Perilaku M1.2 berlaku di konsumen kedua.
- Sumber identifier untuk probe: `sqlident.IsPlain` dipakai untuk memvalidasi alias dan nama tabel;
  engine **tidak** mengekspor pembungkus identifier (`quoteIdent` tidak berhuruf kapital di
  `session_cgo.go`), jadi pemanggil hanya perlu bentuk "tolak/terima".

Temuan yang mengubah rencana bundel M0: `GuardrailConfig.ExtensionDirectory` alone tidak menegakkan
offline. Dengan direktori bundle yang **kosong**, `AttachPostgres` tetap berhasil — DuckDB mengunduh
`v1.1.3/osx_arm64/postgres_scanner.duckdb_extension` ke folder yang seharusnya berisi bawaan aplikasi.
Dua penyebab: `autoinstall_known_extensions`/`autoload_known_extensions` default true, dan `loadExtension`
mengirim `INSTALL` apa pun setting-nya. Sejak 2026-11-01 engine men-set keduanya `false` **dan** melompati
`INSTALL` selama `ExtensionDirectory` terisi, sehingga:

| Kondisi | Perilaku engine hari ini |
|---|---|
| Tanpa `ExtensionDirectory` (dev web) | `INSTALL` best effort, autoinstall aktif — masih bisa unduh |
| `ExtensionDirectory` terisi, file ada | `LOAD` dari bundle, nol jaringan |
| `ExtensionDirectory` terisi, file hilang | gagal cepat: `duckdb extension "postgres" is unavailable: IO Error: Extension ... not found` |

Ditegaskan `TestBundledExtensionDirectoryStaysOffline` (direktori harus tetap kosong setelah gagal).
Artinya untuk M0: berkas extension wajib benar-benar ter-copy ke folder aplikasi, dan **pesan errornya
harus diterjemahkan UI** ("connector belum terpasang"), bukan diteruskan mentah — DuckDB menutupnya dengan
`Install it first using "INSTALL postgres"`, saran yang di desktop justru salah.


## 9. Apa yang dilihat JavaScript dari engine (skeleton Wails, 2026-11-01)

Binding `wails3 generate bindings` memetakan struct Go ke model JS **per JSON tag**, bukan per nama field,
jadi bentuk yang sampai ke UI adalah campuran dua gaya yang harus ditulis eksplisit di kontrak UI §12:

| Sumber | Field yang terlihat di JS |
|---|---|
| `duckdb.QueryResult` (tag Go snake_case) | `columns`, `rows`, `row_count`, `duration_ms`, `row_limit`, `truncated`, `truncated_by`, `capped_sources` |
| Struct desktop (`EngineStatus`) | `duckdbVersion`, `maxMemory`, `threads`, `tempDirectory`, `extensionDirectory` |

Tiga hal lain yang terbukti di jembatan ini:

- Guardrail tetap milik engine, tapi **`ValidateQuery` tidak dipanggil `Session.Query`** — ia lapisan
  pemanggil. Jembatan desktop memanggilnya sendiri sebelum `Query`, jadi `DROP TABLE` ditolak dengan
  `ErrForbiddenStatement` di sisi Go, bukan disembunyikan di UI. Web melakukan hal yang sama di
  `internal/service`; dua konsumen, dua titik panggil, satu implementasi.
- Kontrak M1.4 bertahan melintasi serialisasi: `SELECT * FROM range(20000)` dengan cap default
  menghasilkan `row_count = 10000`, `truncated = true`, `truncated_by = "row_limit"` di JS.
- Binding juga mengekspor **`TruncateReason`** (`bindings/muara.bi/engine/duckdb/models.js:151`), jadi UI
  membandingkan `truncated_by` dengan `TruncateReason.TruncateSourceCap`, bukan dengan string literal — ejaan
  alasannya tetap milik engine. Sejak 2026-11-02 badge grid spike memakainya dan ikut menampilkan
  `capped_sources`: `source_cap` tanpa nama view yang terpotong tidak memberi pembaca apa-apa.
- Metode service yang **diekspor** otomatis jadi binding, termasuk `Close()`. Tidak berbahaya di spike
  (aset hanya lokal), tapi produk tidak boleh memberi webview tombol untuk mematikan session — nanti pakai
  `application.ServiceShutdown` (`ServiceShutdown() error`) atau nama yang tidak diekspor.

## 10. Bundel UI akhir: apa yang benar-benar masuk biner (React/Vite, 2026-11-01)

Rantai build-nya dua langkah dan urutannya wajib: `npm run build` → `frontend/dist`, baru
`go build` yang meng-embed hasilnya (`//go:embed all:frontend/dist`). `frontend/dist` di-gitignore, jadi
clone bersih tanpa langkah pertama gagal di `go:embed` dengan `pattern all:frontend/dist: no matching
files found` — pesan itu milik tooling, bukan kerusakan engine.

Terukur di mesin ini:

| Artefak | Ukuran | Catatan |
|---|---|---|
| `dist/assets/index-*.js` | 247,72 kB (gzip 78,17 kB) | 48 modul: React 19.3.1 + `@wailsio/runtime` 3.0.0 + binding hasil generate |
| `dist/assets/index-*.css` | 2,27 kB | token §12.2, termasuk dark mode |
| `frontend/node_modules` | 39 MB + 2 paket runtime | estimasi awal "±200 MB" terlalu besar; jangan tahan milestone di atas angka yang belum diukur |
| `bin/muara-spike` | 62 MiB (≈65 MB) | DuckDB + Wails + UI dalam satu file; angka "64 MB" di §3 PROGRESS adalah fail yang sama, satuannya beda |

Tiga keputusan yang perlu dicatat karena akan ditanya lagi:

- **Runtime Wails diambil dari npm, bukan dari flag `-b`.** Binding yang di-generate selalu
  `import { Call, CancellablePromise, Create } from "@wailsio/runtime"`, dan Vite mem-bundle paket itu ke
  dalam `index-*.js`. Alasannya bukan ukuran: dengan rute ini halaman **tidak** memanggil
  `/wails/runtime.js`, jadi UI tidak ikut rusak kalau Wails mengubah rute internalnya di versi berikutnya.
  `-b` tetap jalur yang sah kalau nanti kita ingin satu salinan runtime per versi Wails — tapi ia
  mengikat bentuk halaman ke AssetServer.
- **Versi `@wailsio/runtime` disamakan dengan versi Go module (`3.0.0`).** Dua sisi jembatan tidak boleh
  punya versi sendiri; `package.json` dan `go.mod` harus naik bersama, dan itu tugas CI.
- **Offline tetap offline.** Bundel JS berisi runtime-nya sendiri, tidak ada CDN, dan `go:embed` tidak
  punya jalur jaringan. `TestEmbeddedViteBuildIsServed` menjaga ini dari sisi Go: setiap `src`/`href`
  `/assets/*` yang ditulis halaman harus ada di tree embed, dan string `duckdbVersion` (JSON tag engine)
  harus ada di bundel — kalau tidak, yang gagal adalah bukti bahwa engine sampai ke UI, bukan sekadar
  aset yang hilang.

## 11. Profil memori: dari RAM mesin sampai `SET max_memory` (2026-11-01)

Yang dimiliki engine cuma dua hal: `GuardrailConfig.MaxMemory` (string) dan `NormalizeMaxMemory` yang
menolak string di luar grammar-nya. Sisanya — "RAM mesin ini berapa" — adalah milik desktop, dan itu
sekarang paket sendiri: `internal/meminfo`, sengaja **tanpa CGO dan tanpa engine** supaya CI bisa
mengompilasi silangnya di tiga OS.

| OS | Cara baca | Kenapa bukan yang lain |
|---|---|---|
| darwin | `unix.SysctlUint64("hw.memsize")` | satu syscall; bukan "memory available" yang berubah-ubah |
| linux | baris `MemTotal:` di `/proc/meminfo`, × 1024 | `sysconf(_SC_PHYS_PAGES)` butuh CGO/inline asm; `/proc` bisa dibaca build CGO=0 |
| windows | `kernel32!GlobalMemoryStatusEx` lewat `syscall.NewLazyDLL` | x/sys **tidak** mengekspor proc ini; `wmic` deprecated & dibuang di build 11, WMI service bisa mati |

`Budget(total)` menerapkan `min(4GiB, RAM/4)` dan punya tiga aturan yang masing-masing ada alasannya:

- **bulat ke bawah, ke kelipatan MiB** — `max_memory` itu plafon, bukan alokasi; kurang sedikit berarti spill
  lebih awal, bukan hasil yang terpotong.
- **dijepit 256 MiB … 4 GiB** — pembacaan aneh (VM 512 MB, mock di tes) tidak boleh pernah menghasilkan
  budget nol. Engine sekarang juga menolak nol (`NormalizeMaxMemory` meng-parse angkanya, 2026-11-01), jadi
  batas bawah ini bukan satu-satunya penjaga — ia menjaga produk dari *janji* yang tidak masuk akal, engine
  menjaga dari konfigurasi operator yang salah.
- **GiB kalau utuh, MiB kalau tidak** — string harus lolos `NormalizeMaxMemory` dan satuannya harus sama
  dengan label slider FR-15; `1536MiB` dikirim apa adanya dan DuckDB menyimpannya persis 1610612736 byte.

Rantainya diuji dua lapis: `Budget()` diuji sebagai fungsi murni (10 kasus, termasuk cap 64 GiB → `4GiB`
dan floor 512 MiB → `256MiB`) dan setiap keluarannya langsung dicek lolos `duckdb.NormalizeMaxMemory` —
kalau grammar engine berubah, tes desktop ini yang gagal lebih dulu. `TestRAMBudgetReachesDuckDB`
(cgo, paket root) membuktikan sisi berikutnya dengan **membandingkan byte**, bukan string: ukuran RAM
sintetis → string → `SET` → `current_setting('max_memory')`. Ukuran tidak diambil dari mesin tes karena
DuckDB menggemakan GiB dengan satu desimal — di mesin 7 GiB, `1792MiB` kembali sebagai "1.7 GiB" dan tidak
bisa dibalik persis, jadi assert terhadap RAM asli akan membuat CI merah tanpa ada yang rusak.

Angka mesin ini: `hw.memsize` = **8589934592** (8 GiB) → `Budget` = **`2GiB`** = sama dengan
`duckdb.DefaultMaxMemory`. Coincidensi ini sengaja dipertahankan di spike: `NewQueryService` **tidak**
mengisi `MaxMemory`, sehingga `Status()` terus membuktikan default engine bekerja tanpa konfigurasi, dan
pemasangan `meminfo.Budget` baru terjadi bersama UI pengaturan (FR-15, M4).

## 12. Bundel extension yang sebenarnya, dan cara membuktikannya offline di macOS (2026-11-01)

Isi folder yang dihasilkan `cmd/duckdb-bundle` di mesin ini — **103 MB**, nama folder mengikuti DuckDB
yang di-link statis (v1.1.3), bukan versi go-duckdb (v1.8.5):

```
v1.1.3/osx_arm64/postgres_scanner.duckdb_extension   32 MB
v1.1.3/osx_arm64/sqlite_scanner.duckdb_extension     32 MB
v1.1.3/osx_arm64/mysql_scanner.duckdb_extension      38 MB
v1.1.3/osx_arm64/*.duckdb_extension.info          165–168 B  (sidecar metadata DuckDB; isinya tidak dibaca)
```

`<versi DuckDB>/<platform>` adalah jawaban kenapa bundel tidak boleh di-commit dari satu mesin: nama
foldernya milik OS tempat ia dibuat. Angka ini juga yang membuat §9 BRD (ukuran installer) sekarang punya
dasar: ±170 MB per installer sebelum signing, dan tidak bisa dipakai silang antar-OS.

**Bagaimana membuktikan "tidak ada jaringan" tanpa `sudo` di macOS.** `--network none` cuma punya Docker
(linux), `pfctl`/`dnctl` butuh hak root, dan `fs_usage` juga. Yang tersedia tanpa privileges adalah membuat
**tidak ada tempat untuk menulis hasil unduhan**:

1. salin bundel, lalu `chmod -R a-w` — setiap unduhan ke `extension_directory` gagal keras;
2. jalankan dengan `HOME=/tmp/muara-fakehome` — kalau DuckDB pakai direktori defaultnya, hasilnya muncul
   di sana, dan foldernya diketahui kosong sejak awal.

Hasilnya pada run ini: ATTACH Postgres berhasil (`tables:1`, `rows_in_orders:6`), jumlah file bundel tetap
6, dan HOME palsu tetap **0 byte**. Engine sudah menutup dua jalur unduhan lain (`INSTALL` dilompati,
`autoinstall_known_extensions=false`, `autoload_known_extensions=false`), jadi run ini bukan sekadar
"kebetulan belum sempat download" — ia adalah bukti bahwa berkas di folder aplikasi **cukup**.
Sisi negatifnya dijaga `TestBundledExtensionDirectoryStaysOffline` di engine (bundle kosong → `LOAD` gagal,
folder tetap kosong). Untuk linux/windows, pakai `--network none` dan runner windows di CI; itu bukti yang
lebih kuat daripada trik di atas, bukan pengulangan pekerjaan.

**`Session.AttachSQLite` membuat bukti itu tidak butuh server (2026-11-01).** Connector `sqlite_scanner`
membaca berkas, bukan menghubungi host: dengan satu fixture 8 KB di
`muara.bi-desktop/tests/fixtures/spike.sqlite`, pertanyaan "biner ini bisa LOAD connector dari bundel?"
terjawab di mesin mana pun tanpa Postgres, tanpa kredensial, dan tanpa koneksi yang bisa timeout — bentuk
yang tepat untuk runner Windows, yang tidak punya database apa pun.
Dua perilaku engine yang harus diketahui pemanggil, keduanya terukur hari ini:

1. **Catalog attached sqlite tidak punya skema `public`.** `SELECT … FROM sample.orders` lulus (1 tabel,
   6 baris), `sample.public.orders` gagal — jadi nama relasi lengkap mengikuti bentuk sumbernya, dan query
   bawaan tidak bisa dipakai silang antara Postgres dan SQLite.
2. `SELECT count(*) FROM duckdb_tables() WHERE database_name = 'sample'` adalah cara menghitung tabel
   sumber **tanpa** menginterpolasi identifier dari katalog remote; `information_schema` per-catalog
   (`sample.information_schema.tables`) hanya berlaku untuk Postgres.

## 13. Deployment target macOS: siapa yang menentukan `minos`, dan apa yang boleh diiklankan (2026-11-01)

Pertanyaannya bukan "berapa warning yang kita dapat", tapi "biner yang kita kirim mengklaim bisa jalan di OS
berapa". Angka itu ditulis di `LC_BUILD_VERSION.minos`, dan hari ini ia **bukan pilihan kita**: ia hasil
direktif `#cgo LDFLAGS` Wails dan default SDK mesin build. Tabelnya dari build nyata di mesin ini (`otool -l`,
warning dihitung dari log link):

| build | flag | `minos` | warning `built for newer` |
|---|---|---|---|
| `go build ./cmd/spike` (tanpa Wails) | — | **14.0** | 0 |
| `go build ./cmd/spike` | `MACOSX_DEPLOYMENT_TARGET=11.0` | 11.0 | 43 |
| `go build .` (app Wails) | — | **11.0** | 50 |
| `go build .` | `MACOSX_DEPLOYMENT_TARGET=12.0` | 11.0 | 50 (tidak berubah) |
| `go build .` | `CGO_LDFLAGS=-mmacosx-version-min=13.0` | 13.0 | 49 |
| `go build .` | `CGO_LDFLAGS=-mmacosx-version-min=14.0` | **14.0** | **0** |

**Tiga mekanisme, dan kenapa catatan lama "`MACOSX_DEPLOYMENT_TARGET` tidak berpengaruh" salah separuh.**

1. Tanpa flag apa pun, clang memakai **versi mayor SDK mesin** sebagai default — diukur dengan satu berkas
   C polos: `clang t.c` → `minos 14.0` (SDK 14.4). Karena itu biner `cmd/spike` hari ini 14.0, dan biner yang
   sama di runner macOS 15 akan mengklaim 15.0. Angka produk tidak boleh diturunkan dari SDK mesin build.
2. `MACOSX_DEPLOYMENT_TARGET` **hanya default**, bukan flag. Ia kalah oleh `-mmacosx-version-min` yang
   ditulis eksplisit di mana pun — termasuk di dalam kode orang lain. Test lama memakai nilai `14.0` yang
   kebetulan **sama dengan default mesin**, jadi "tidak ada perubahan" adalah false negative: dengan
   `11.0` pada build non-Wails, `minos` benar-benar turun dan 43 warning muncul. Pelajarannya dicatat
   di PROGRESS §3 (temuan 4): jangan simpulkan sebuah knob tidak berpengaruh sebelum
   mengetesnya dengan nilai yang berbeda dari default.
3. Yang menulis `11.0` di biner app adalah Wails: `pkg/application/application_darwin.go:8` (v3.0.0) membawa
   `#cgo LDFLAGS: -framework Cocoa -mmacosx-version-min=10.13`, dan ld64 menjepit 10.13 ke **11.0** karena
   itu lantai arm64. Jadi setiap biner yang men-link `pkg/application` mengiklankan Big Sur — padahal isi
   arsip statis DuckDB di-compile untuk 14.0. Dibuktikan tanpa Wails lewat reproduksi tiga berkas: satu paket
   dengan `#cgo LDFLAGS: -mmacosx-version-min=10.13` memberi `minos 11.0`, dan paket yang sama dibangun dengan
   `CGO_LDFLAGS="-mmacosx-version-min=14.0"` memberi `minos 14.0` — jadi flag environment **mengalahkan**
   direktif di dalam kode orang lain. Urutan persisnya di baris perintah clang tidak di-trace (link Go
   memakai `-x` tidak menampilkan flag eksternal dengan rapi di mesin ini); yang diterima sebagai bukti adalah
   hasilnya, dan itu sudah cukup untuk menyetel angka produk.

**Keputusan: pin `CGO_LDFLAGS="-mmacosx-version-min=14.0"` saat build macOS, dan iklankan minimum
macOS 14 (Sonoma).** 14.0 adalah target terendah yang masih link tanpa satu pun warning (50 warning muncul
lagi di 11.0, 49 di 13.0 — persis seperti komposisi object arsip: 49 pada 14.0, 1 pada 13.0). Wails sendiri
mendukung 12.0 (Taskfile darwin-nya mem-pin 12.0 dan ada tes yang mengasertinya), tapi klaim 12.0 lebih
rendah dari lantai kode engine kita, dan mesin dengan macOS 12/13 tidak tersedia di sini — jadi tidak ada yang
bisa membuktikan bahwa ia benar-benar jalan. Memilih angka yang lebih tinggi adalah
satu-satunya klaim yang bisa dipertanggungjawabkan hari ini; menurunkannya nanti butuh bukti, bukan optimisme.

**Yang ikut dibawa ke butir CI per-OS:** build tanpa pin tidak bisa direproduksi (`minos` ikut SDK runner), jadi
langkah `macos` wajib menyetel pin **dan** membaca `otool -l <biner> | grep -A4 LC_BUILD_VERSION` sebagai
gerbang — assert `minos` sama dengan angka yang diiklankan. Warning `built for newer` tidak dipakai sebagai
gerbang: ia juga muncul dari `go.o` buatan toolchain Go itu sendiri (terbukti pada program cgo tiga baris),
jadi ia bising bahkan ketika tidak ada yang salah.

**`minos` bukan yang ditegakkan — `LSMinimumSystemVersion` yang itu.** Bedanya baru terlihat waktu artefak
`.app` dirakit (2026-11-01, `build/macos-app.sh`): Mach-O dengan `minos 14.0` tetap bisa di-double-click di
macOS 12 dan baru gagal saat runtime, karena Finder dan Gatekeeper membaca kunci `LSMinimumSystemVersion` di
`Info.plist`, bukan load command build version. Jadi angka §13 harus ditulis **dua kali** — sekali sebagai flag
link, sekali sebagai plist — dan `.app` hari ini melakukan keduanya (`LSMinimumSystemVersion => 14.0`,
diverifikasi `plutil -lint` dan `defaults read`). Konsekuensi yang belum selesai ikut tercatat di sana:
`codesign --sign -` menghasilkan tanda tangan ad-hoc, yang cukup untuk menjalankan build di mesin pembuatnya
tetapi tidak akan tebus Gatekeeper di mesin orang lain; Developer ID + notarization + dmg tetap pekerjaan M6.

## 14. Batas yang dipasang Wails v3.0.0 pada jalur build dan pada bindings (2026-11-01)

Dua hal yang baru kelihatan waktu spike menyentuh sisi build, bukan sisi runtime:

1. **`wails3 build` tidak bisa dipakai tanpa `task`.** CLI-nya mendelegasi build/package ke
   `build/darwin/Taskfile.yml` dan berhenti dengan `ERROR task: No Taskfile found` pada skeleton yang
   dirakit tangan; `task` sendiri bukan dependensi Go, jadi ia harus dipasang terpisah. Artefak M0 karena itu
   dirakit `build/macos-app.sh` — dan itu **bukan** jalur produksi: Taskfile Wails ikut mengurus entitlements,
   Developer ID, notarization dan dmg, jadi M6 tetap harus membuka jalur itu, bukan memperpanjang skrip.
2. **Bindings bukan berkas yang boleh diedit tangan.** `frontend/bindings/**` berisi pemanggilan
   `$Call.ByID(<hash>)` — nomor metode hasil hash dari generator, bukan nama. Menambah satu method di Go
   (`AttachSampleSQLite`, 2026-11-01) tidak akan pernah terlihat di UI sampai
   `.tools/bin/wails3 generate bindings -d frontend/bindings` dijalankan; menulis ulang berkasnya manual
   menghasilkan binding yang memanggil ID milik metode lain, dan kegagalan itu baru muncul saat diklik.
   Generator itu sendiri bersih dan cepat (369 paket, 5 metode, 2,2 dtk) dan tidak butuh `task`.

## 15. Sumber file: reader core, bukan extension (2026-11-02)

Butir M2 "sumber file v1" ternyata tidak menyentuh bundel sama sekali, dan perbedaan itu cukup penting untuk
dicatat terpisah — jangan tertukar dengan §12, yang mengurus extension. **Cara path diperlakukan baru diukur
2026-11-02 dan aturannya ada di §22**: reader memperlakukan argumennya sebagai pattern yang diselesaikan terhadap
folder kerja, jadi tidak ada path yang boleh sampai ke SQL dalam bentuk relatif.

- **Tiga reader sudah dikompilasi ke biner DuckDB**: `read_csv_auto` (juga untuk `.tsv` — delimiter dicium
  otomatis), `read_parquet`, `read_json_auto` (objek per baris maupun array). Karena tidak ada extension yang
  dimuat, klaim "offline" untuk sumber file lebih kuat daripada untuk Postgres/MySQL/SQLite: tidak ada folder
  bundel, tidak ada CDN, tidak ada `LOAD`.
- **Cara membuktikan reader ada tanpa berkasnya**: `read_parquet` diuji dengan path yang tidak ada, dan yang
  keluar adalah `IO Error: No files found that match the pattern "/tmp/does-not-exist.parquet"` — bukan
  "extension tidak tersedia". Dengan bundel **kosong dan hanya-baca** plus `HOME` kosong, kegagalan itu hanya
  bisa berarti satu hal: namanya sudah dikenal DuckDB. Trik yang berlaku umum di spike: kalau sebuah fungsi
  menolak karena *file*, ia sudah ter-resolve. Bukti ini kemudian diganti yang lebih kuat: `spike.parquet`
  (564 B, ter-commit) terbaca 6 baris dalam keadaan yang sama.
- **Cara fixture parquet dibuat, dan mengapa tidak lewat engine**: `COPY … TO` dilarang `ValidateQuery` dan
  itu aturan yang benar untuk produk — aplikasi desktop tidak boleh menulis berkas sembarang. Fixtures
  karena itu dihasilkan oleh generator sementara yang memakai `go-duckdb` langsung, di luar `Session.Query`,
  lalu berkasnya saja yang di-commit dan generatornya dibuang. Aturan turunannya: alat pembuat fixture tidak
  boleh jadi jalur produk, dan tidak boleh tinggal di repo.
- **Inferensi tipe datang dari `DESCRIBE SELECT * FROM view`**, bukan dari engine sendiri: CSV/JSON fixture
  menghasilkan `id BIGINT`, `channel VARCHAR`, `total DOUBLE`. Kolom dan tipenya lewat `ColumnMeta` yang sama
  dengan hasil query biasa, jadi grid tidak perlu jalur khusus. `.tsv` numerik bisa muncul sebagai `BIGINT`
  sementara `.csv` dengan angka desimal menjadi `DOUBLE` — tipe kolom sumber file adalah apa yang DuckDB
  simpulkan, dan itu harus ditampilkan apa adanya di UI, jangan digores "text" supaya seragam. Beda penting:
  pada **parquet tipenya dibawa berkas**, jadi schema drift yang bisa dilihat user hanya datang dari file teks.
- **Yang ditolak, ditolak dengan nama:** `.txt`, `.xlsx`, `Makefile`, `.pqt`, path kosong mengembalikan
  `ErrUnsupportedFileSource` dengan pesan yang menyebutkan daftar yang didukung. Ini pilihan desain, bukan
  kelalaian: membiarkan `.xlsx` lolos berarti engine diam-diam mencoba extension `excel`, yang akan mengunduh
  dari CDN atau gagal setelah timeout — dua-duanya melanggar janji desktop.
- **Quote helper dipindah ke berkas tanpa tag build** (`duckdb/sqlquote.go`). Selama `quoteIdent`/`quoteLiteral`
  tinggal di `session_cgo.go`, paket `duckdb` pada `CGO_ENABLED=0` tidak bisa menguji logika path/nama, dan
  gerbang empat-mode ikut kehilangan coverage untuk bagian yang justru paling rawan injection.
- **Dibaca di mana saja, tanpa bundel:** `TestSpikeFileFixturesReadWithNoBundledConnector` (repo desktop)
  membuka ketiga fixture dengan `ExtensionDirectory` = folder kosong, menjalankan agregasi dan `DESCRIBE`, lalu
  memastikan folder itu tetap kosong. Karena ia jalan di `go test ./...`, runner Windows dan Linux ikut
  menjawab "berkas user terbaca di OS ini?" tanpa connector, tanpa server, tanpa jaringan.
- **Sisa yang belum terbukti:** aturan **path relatif** di `.muara` (temp view saat ini menyimpan path absolut,
  jadi bundle yang dipindah mesin lain menunjuk berkas yang tidak ada) dan UI impornya.

## 16. Kata kunci conninfo yang benar-benar diterima `postgres_scanner` (2026-11-02)

`AttachPostgres` menyerahkan string conninfo ke libpq yang dibundel di dalam scanner, jadi nama parameter bukan
sesuai selera produk — dan ini bisa diukur **tanpa server TLS**, karena libpq memvalidasi nama opsi *sebelum*
berusaha connect. Probe (spike, `host=127.0.0.1 port=1`, koneksi pasti gagal):

| conninfo | hasil | arti |
| --- | --- | --- |
| `sslrootcert=/tmp/probe-ca.pem` | `Is the server running on that host and accepting TCP/IP connections?` | dikenal → parsing selesai, gagal di TCP |
| `rootcert=/tmp/probe-ca.pem` | `invalid connection option "rootcert"` | **tidak dikenal** |
| `boguscert=/tmp/probe-ca.pem` | `invalid connection option "boguscert"` | pembanding: pesan yang sama = opsi karangan |

Dua pelajaran yang lebih luas dari satu tabel:
1. **Bandingkan kata kunci yang diuji dengan kata kunci rekaan.** Tanpa baris ketiga, "tidak ada error" bisa
   berarti "opsinya diabaikan" — dan kegagalan senyap itulah yang ingin dihindari untuk CA.
2. **Buktikan berkasnya benar-benar dibuka, bukan cuma diterima.** Di sisi pgx (jalur `SyncSchema` web), tes
   menunjuk CA ke berkas yang tidak ada dan menuntut `pgx.ParseConfig` **gagal**; plus satu CA self-signed yang
   benar-benar dibuat di tes (`crypto/x509`) supaya `TLSConfig.RootCAs` bisa diperiksa. Nama field produk tetap
   `root_cert` (jelas untuk user), yang dikirim ke driver adalah `sslrootcert`.

Belum terukur dan butuh sumber Postgres **bersertifikat TLS**: apakah scanner memuat CA itu sampai
verifikasi berhasil/gagal (`sslmode=verify-ca` sungguhan). Sample postgres di mesin ini sedang rusak dan
infrastruktur milik user, jadi pembuktiannya ditunda — yang sudah terbukti adalah string-nya diterima.

## 17. Grammar DSN `mysql_scanner`, diukur per karakter (2026-11-02)

`AttachMySQL` juga menerima string mentah ke scanner, tapi grammar-nya **berbeda total** dari libpq: ini
`key=value` dipisah spasi, bukan URI dan bukan bentuk driver Go. Cara mengukur sama seperti §16 — scanner
memvalidasi nama opsi dan bentuk token *saat membaca string*, jadi `host=127.0.0.1 port=1` (alamat mati)
sudah cukup memisahkan "stringnya dimengerti" (`Can't connect to MySQL server`) dari "stringnya ditolak"
(`Invalid dsn …` / `Unrecognized configuration parameter …`). Hasil lengkap, DuckDB 1.1.3 + `mysql_scanner`
dari bundel aplikasi:

| bentuk | hasil | aturan |
| --- | --- | --- |
| `host=db.internal port=3306 user=reader passwd=s3cret db=sales` | parsed | kunci yang diterima **hanya** `host, port, user, passwd, db, socket` |
| `root:s3cret@tcp(127.0.0.1:1)/sales` | `Invalid dsn …` | bentuk driver Go **ditolak** — inilah yang selama ini dikirim web |
| `mysql://root:s3cret@127.0.0.1:1/sales` | `Invalid dsn …` | URI juga ditolak |
| `passwd=` / `db=` | `Unrecognized configuration parameter ""` | nilai kosong dipecah jadi opsi bernama kosong; **kunci yang tidak ada harus dihilangkan**, bukan dikosongkan |
| `passwd=a b` | `expected key=value pairs separated by spaces` | spasi (dan tab) wajib dalam kutip ganda |
| `passwd=a=b` | `Unrecognized configuration parameter ""` | `=` di dalam nilai ikut memecah → wajib dikutip |
| `passwd=p@ss/w:ord` | parsed | `@ / :` aman apa adanya di bentuk tanpa kutip |
| `passwd=C:\path` | parsed | **di luar kutip, backslash verbatim** — jangan dilipatgandakan |
| `passwd="C:\path"` | `backslash can only escape \ or "` | di dalam kutip hanya `\\` dan `\"` yang sah |
| `passwd="C:\\path"` | parsed | bentuk benar untuk backslash di nilai berspasi, mis. `C:\My Files` |
| `passwd=say"hi"` | parsed | kutip di tengah nilai aman selama tidak di awal |
| `passwd="leading` | `unterminated quote` | nilai yang **diawali** `"` masuk mode kutip → wajib `passwd="\"leading"` |
| `passwd=""` | parsed | nilai kosong bisa diwakili, jadi `""` bukan selalu bug |
| `port=0` | parsed, connects as 3306 | jangan kirim `port=0`; hilangkan kuncinya |
| `ssl-ca=…` / `ssl-capath=…` / `ssl-mode=…` | `Unrecognized configuration parameter …` | **scanner ini tidak punya opsi TLS apa pun** |

Tiga kesimpulan produk:
1. **`MySQLDSN` milik engine, bukan web.** Aturan kutipnya aneh dan milik scanner; ditaruh di
   `muara.bi/engine/duckdb/mysql.go` supaya kedua produk mengirim string yang sama. Web tinggal
   `attachConnection` → `duckdb.MySQLDSN(duckdb.MySQLSource{…})`.
2. **CA privat untuk MySQL tidak bisa dikerjakan di jalur ini.** Bukan "belum", tapi tidak ada kata kunci
   tempat meletakkannya, dan kredensial pun tidak bisa lewat opsi ATTACH (satu-satunya opsi yang dikenal
   `read_only`). Jadi validasi di titik tulis menolak `root_cert` pada profil MySQL — menyimpan path yang
   diam-diam diabaikan lebih buruk daripada ditolak dengan pesan beralasan. MySQL di belakang TLS tetap bisa
   lewat bastion/VPN (§5.7), dan `mysql://?ssl-ca=` hanya mungkin kalau sumber MySQL pindah ke pembaca Go.
3. **Batas pembuktian:** yang terukur adalah *parsing*, bukan nilai yang benar-benar dikirim ke server —
   error scanner menampilkan kembali string aslinya, bukan hasil unescape. Password berisi karakter aneh
   terbukti "diterima scanner", tapi byte yang identik di atas kabel baru bisa dipastikan dengan server
   MySQL sungguhan. Tes tingkat scanner (`engine/duckdb/mysql_scanner_test.go`) karena itu diberi batas:
   jalan hanya kalau `DUCKDB_EXTENSION_DIRECTORY` menunjuk bundel berisi `mysql_scanner`.

## 18. Kata kunci TLS yang benar-benar dibaca driver MongoDB (2026-11-02)

Sumber Mongo tidak lewat DuckDB: `StreamMongoCollections` memakai **driver Go**, jadi CA punya tempat di sini —
beda total dari `mysql_scanner` (§17). Pengukurannya pakai trik yang sama: `mongo.Connect` terhadap
`127.0.0.1:1` dengan `serverSelectionTimeoutMS=400`, dan **berkas sertifikat dibuka sebelum server disentuh**,
sehingga error "berkas tidak ada" adalah bukti bahwa kata kuncinya dibaca. Driver v1.17.10:

| URI (sebagian) | hasil | arti |
| --- | --- | --- |
| `?tls=true&tlsCAFile=/berkas/yang/tidak/ada` | `open …: no such file or directory` | **`tlsCAFile` dibaca** |
| `?tls=true&sslCAFile=/berkas/yang/tidak/ada` | error nil | nama warisan **diabaikan diam-diam** |
| `?tls=true&bogusCAFile=/…` | error nil | driver **tidak** memvalidasi nama opsi — "tanpa error" bukan bukti |
| `?tlsCAFile=/berkas/tidak/ada` (tanpa `tls=`) | `open …: no such file` | berkasnya tetap dibuka walau TLS tidak diminta |
| `?tls=true&tlsCAFile=` (nilai kosong) | `open : no such file` | sama seperti `passwd=` di MySQL: **hilangkan kuncinya** |
| `?tls=true&tlsInsecure=true` | error nil | ada, dan berarti verifikasi dimatikan — jangan sampai produk memancangnya |

Tiga aturan yang lahir dari tabel ini:
1. **Bukti sebuah kata kunci = berkasnya dibuka, bukan pesan error hilang.** Baris `bogusCAFile` adalah
   pembanding yang wajib; tanpa itu `sslCAFile` akan terlihat "diterima".
2. **`sslCAFile` adalah jebakan keamanan**, bukan sekadar nama lama: profil yang memakannya mengira dirinya
   terverifikasi sementara driver tidak membaca apa pun. Karena itu `withMongoTLS` (web) **menolak** URI yang
   masih memakannya alih-alih menerjemahkannya.
3. **Mode fallback tidak bisa dipetakan.** Driver cuma punya nyala/mati. `require`/`verify-ca`/`verify-full` →
   `tls=true`; `disable` dan fallback libpq `allow`/`prefer` → tetap polos. Ini bukan kemalasan: form
   koneksi web mengirim `ssl_mode: "prefer"` untuk **semua** driver, jadi menolak atau memaksanya ke TLS akan
   merusak profil Mongo yang hari ini bekerja. Yang dijaga ketat adalah kombinasinya: `root_cert` di samping
   mode tanpa TLS **ditolak di titik tulis**, karena CA yang tidak dipakai itulah yang membuat profil berbohong.

Artinya `verify-ca` vs `verify-full` tidak bisa dibedakan di sisi driver (Go selalu memverifikasi rantai
**dan** nama host selama `tlsInsecure`/`tlsAllowInvalidCertificates` tidak dipasang), jadi bedanya di Mongo
hanya menandai niat operator, bukan perilaku berbeda. Yang belum terukur: verifikasi sungguhan terhadap server
Mongo bersertifikat (butuh infra TLS milik user) — bukti yang ada adalah berkas CA benar-benar dibuka.

## 19. `threads`: dari profil mesin ke `SET threads` (2026-11-02)

Pertanyaan M0 "apakah `threads` perlu ikut profil" tidak bisa dijawab dari sisi desktop saja: angkanya dirakit
di `engine/duckdb/session_cgo.go` (`NewEngine` → `SET threads=%d` di `NewSession`). Jadi bentuk pekerjaannya dua
lapis — engine membuka pintunya, tiap produk memberi angkanya sendiri. Probe langsung (DuckDB 1.1.3, mesin 8 core,
satu koneksi in-memory, `current_setting('threads')` dibaca pada koneksi yang sama):

| `SET threads=` | hasil | arti |
| --- | --- | --- |
| tidak pernah disetel | `8` | default DuckDB = **semua core yang terlihat** |
| `1`, `2`, `4`, `8` | diterima, terbaca kembali persis | tidak ada kejutan di rentang wajar |
| `0` | `Syntax Error: Must have at least 1 thread!` dan nilai **tetap `8`** | `0` bukan "auto", dan gagalnya SET tidak menghentikan sesi |
| `-1` | error yang sama, nilai tetap `8` | negatif pun demikian |
| `999` | **diterima** | tidak ada cap internal DuckDB |
| `abc` | error cast | |

Empat aturan yang lahir dari tabel itu:
1. **`0` harus berarti "belum dikonfigurasi", bukan diteruskan.** Go memberikan `0` untuk `int` yang tidak diisi
   di `GuardrailConfig`, sementara DuckDB menolaknya — kalau diteruskan, kegagalan `SET` tidak membuat sesi jadi
   aman, hanya membuat konfigurasi **terlihat** jalan (baris `0`/`-1` di atas tetap menyisakan 8 thread).
   Engine memetakannya ke `DefaultThreads = 2`.
2. **Negatif gagal di `NewEngine`, bukan di sesi pertama** — preseden `0GiB` dari M1.5: kesalahan konfigurasi
   harus terdengar saat proses start.
3. **Cap adalah pekerjaan produk.** DuckDB menerima `999` tanpa protes, jadi `NormalizeThreads` menjepit ke
   `runtime.NumCPU()`. Penjepitan ini terlihat dari luar karena `current_setting('threads')` mengembalikan angka
   yang benar-benar berlaku, dan itu yang diuji `TestEngineAppliesConfiguredThreads` (1, 2, 999).
4. **Dua produk, dua angka.** Web: `DUCKDB_THREADS` default `2`, sebab CPU satu pod saat ramai adalah
   `threads × DUCKDB_MAX_CONCURRENCY` dan multi-tenant tidak boleh mewarisi "semua core host" cuma karena lupa
   menyetel. Desktop: `threadBudget() = NumCPU-1` (minimal 1) — satu user, satu sesi, satu core ditinggal untuk
   jendelanya sendiri.

Batas yang harus ditulis jujur: **`NumCPU-1` adalah kebijakan, bukan hasil ukur.** Yang teruji hanya rantai
konfigurasi → DuckDB (`TestSpikeStatusReportsEngineDefaults` membaca ulang `current_setting('threads')` dari sesi
sungguhan, bukan membandingkan dengan konstanta) dan invarian `1 ≤ threadBudget() ≤ NumCPU`
(`TestThreadBudgetStaysInsideTheMachine`). Dua hal masih asumsi: bahwa satu core cadangan memang membuat UI
tetap mulus saat query berjalan, dan bahwa `runtime.NumCPU()` adalah jumlah core yang benar-benar boleh dipakai
di dalam container — `--cpus` membatasi lewat *quota*, bukan affinity, dan itu belum diukur di sini, karena itu
`.env.example` menyuruh operator menyetelnya sendiri di sana. Pertanyaan performa yang sebenarnya
("berapa koneksi yang dibuka `postgres_scanner` saat query paralel, dan berapa thread yang masuk akal lewat
VPN") butuh jaringan nyata, bukan localhost — tetap butir terbuka di PLAN.md M0.

Yang **tidak** ikut berubah: `cmd/spike` dan `MaxMemory` di `NewQueryService`. Spike justru ada untuk membuktikan
default engine sampai tanpa konfigurasi, dan pemasangan budget memori menunggu UI pengaturan FR-15 karena nilainya
harus bisa ditimpa user; jumlah thread tidak.






## 20. Katalog sumber: `duckdb_columns()` adalah satu-satunya jalur lintas scanner (2026-11-02)

Butir M2 web ("`SyncSchema` lintas driver") memaksa satu pertanyaan dijawab dengan pengukuran, bukan
asumsi: *kalau engine sudah bisa men-ATTACH sebuah sumber, bagaimana cara membaca daftar tabel & kolomnya?*
Probe dilakukan pada DuckDB 1.1.3 + bundel extension aplikasi (`/Applications/muara-bi.app/Contents/Resources/
duckdb/extensions`), sumbernya fixture SQLite repo ini (`tests/fixtures/spike.sqlite`).

| Percobaan (sesudah `ATTACH … AS litesrc (TYPE SQLITE, READ_ONLY)`) | Hasil terukur |
| --- | --- |
| `litesrc.information_schema.columns` | **Gagal**: `Catalog Error: Table with name columns does not exist!` — `sqlite_scanner` tidak memajang information_schema di catalog yang di-attach |
| `litesrc.duckdb_columns()` | bukan fungsi di sana — `duckdb_columns()` adalah fungsi **DuckDB**, dipanggil tanpa prefix |
| `SELECT … FROM duckdb_columns() WHERE database_name='litesrc'` | **Berhasil**, 3 baris: `id BIGINT`, `channel VARCHAR`, `total DOUBLE` (urutan `column_index` = urutan deklarasi) |
| `duckdb_tables() WHERE database_name='litesrc'` | Berhasil; membawa `has_primary_key`, `estimated_size`, `column_count`, dan `sql` (DDL penuh) |
| `duckdb_constraints() WHERE database_name='litesrc'` | Berhasil; `constraint_type` = `PRIMARY KEY`/`NOT NULL`, `constraint_column_names`, `referenced_table` |
| `duckdb_schemas()` | Berhasil; schema fixture adalah `main` |

Dua hal yang keluar dari angka di atas, dan keduanya ikut cara engine dipakai:

1. **Tipe yang dilaporkan adalah tipe scanner, bukan tipe deklarasi sumber.** Fixture mendeklarasikan `INTEGER`
   dan scanner membalas `BIGINT`. Untuk cache yang dipakai menulis SQL (tree, autocomplete, prompt AI) itu yang
   benar: yang dijanjikan adalah tipe yang dilihat planner, bukan tipe yang ditulis di sumbernya.
2. **`is_nullable` BOOLEAN dan `data_type` VARCHAR ada di `duckdb_columns()`**, jadi Butir M2 berikutnya
   (skema terkualifikasi + PK/FK + nullability + komentar + jumlah baris) tidak butuh connector baru — cukup
   perluas `CatalogTable`, lalu ubah bentuk `cached_schema` dan empat pembaca-nya sekaligus.

Yang **tidak** bisa diukur: sisi Postgres. Container sample mati di sisinya
(`FATAL: could not open file "global/pg_filenode.map": I/O error`, VM Docker yang sama yang membuat
`docker exec` ikut gagal), jadi belum ada pembanding spelling tipe Postgres dari `postgres_scanner` versus
`information_schema` milik sumber. `cmd/spike` sudah membuktikan `pgsrc.information_schema.tables` berfungsi
untuk Postgres, jadi kedua jalur ada — dan karena itu web sengaja **mempertahankan pgx untuk Postgres** dan
baru memindahkannya ke katalog DuckDB setelah ada sample Postgres yang hidup. Restart infra itu milik user.

Permukaannya (`backend/engine/duckdb/catalog.go`): `DescribeSource` mem-*bind* alias sebagai parameter
(`database_name = ?`) setelah `sqlident.IsPlain`, menyaring `internal = false`, mengurutkan
`schema_name, table_name, column_index`, dan membaca `MaxCatalogColumns+1 = 5001` baris sebagai **sentinel**
(preseden M1.4: baris ekstra adalah bukti pemotongan). Source yang lebih lebar dari itu ditolak, tidak
disimpan separuh dan tampak lengkap. Denylist schema sistem (`information_schema`, `pg_catalog`, `mysql`,
`performance_schema`, `sys`) dijalankan di Go supaya teruji tanpa sumber nyata. Build tanpa CGO mengembalikan
`ErrCGODisabled` seperti method `Session` yang lain.

---

## 21. Isi katalog penuh: kunci, nullability, komentar, dan taksiran yang bukan jumlah baris (2026-11-02)

§20 butir 2 bilang perluasan `CatalogTable` "tidak butuh connector baru". Kalimat itu diuji dulu sebelum
ditulis kode: **apa saja yang benar-benar ada di tiga fungsi katalog itu**, diukur pada DuckDB 1.1.3 + bundel
aplikasi, lewat dua sumber metadata — fixture SQLite repo ini, dan **katalog `temp` punya DuckDB sendiri**
(dua tabel temp berkunci komposit plus satu FK), karena hanya jalur kedua yang bisa dijalankan tanpa server
apa pun.

| Fungsi | Kolom yang ada (terukur) | Yang dipakai |
| --- | --- | --- |
| `duckdb_columns()` | `database_name`, `schema_name`, `table_name`, `column_name`, `column_index`, `data_type`, `is_nullable` (BOOLEAN), `column_default`, `comment`, `character_maximum_length`, `numeric_precision`, `numeric_precision_radix`, `numeric_scale`, `internal` | nama, tipe, `is_nullable`, `column_default`, `comment` |
| `duckdb_constraints()` | `schema_name`, `table_name`, `constraint_index`, `constraint_type`, `constraint_text`, `constraint_name`, `constraint_column_names` (**LIST**), `constraint_column_indexes` (`BIGINT[]`), `referenced_table`, `referenced_column_names` (**LIST**) — **tidak** punya `internal`, **tidak** punya `referenced_schema` | `PRIMARY KEY` → kunci; `FOREIGN KEY` → tabel & kolom rujukan |
| `duckdb_tables()` | `comment`, `has_primary_key`, `estimated_size`, `column_count`, `index_count`, `check_constraint_count`, `temporary`, `sql` (DDL penuh), `internal` | `comment`, `estimated_size` |

Enam konsekuensi yang langsung mengubah bentuk kodenya:

1. **LIST tiba di Go sebagai `[]interface{}`** (terukur: `[]interface {}([pid org])`). Satu perata
   (`catalogStrings`) yang membuang elemen non-string/kosong/`nil` — nama kolom kosong tidak akan pernah
   bisa dipakai JOIN. Fungsinya hidup di `catalog.go` tanpa build tag supaya aturannya tetap teruji di build
   `CGO_ENABLED=0`.
2. **`estimated_size` bukan jumlah baris.** Fixture berisi **6** baris (`SELECT count(*)` = 6); scanner
   melaporkan **5**. Kolomnya karena itu bernama `EstimatedRows`, dan dokumentasinya mewajibkan UI menyebut
   angka itu taksiran. Konsumennya tidak boleh memakai angka ini untuk memutuskan query benar atau salah.
3. **View tidak punya baris di `duckdb_tables()`.** `CREATE TEMP VIEW` muncul di `duckdb_columns()` (jadi
   kolomnya ter-cache) tetapi tidak di `duckdb_tables()`. Kalau "tidak ada baris" dipetakan ke `0`, tree
   melaporkan "tabel kosong" untuk sesuatu yang tidak pernah diukur — maka `UnknownRowCount = -1`.
   Teks yang sama berlaku untuk `estimated_size` yang NULL.
4. **`is_nullable` adalah deklarasi, bukan konsekuensi kunci.** `id INTEGER PRIMARY KEY` dilaporkan
   **nullable** (rowid alias SQLite), `channel TEXT NOT NULL` dan `total REAL NOT NULL` tidak. Aturan untuk
   katalog yang tidak bilang apa-apa: baca **nullable**, karena mengarang `NOT NULL` adalah arah yang membuat
   SQL hasil AI salah.
5. **Baris bookkeeping memakan budget baris.** `duckdb_columns()` untuk catalog Postgres yang di-ATTACH akan
   membawa `information_schema`/`pg_catalog` mirror — ribuan kolom sebelum satu kolom pengguna — dan cap
   `MaxCatalogColumns` adalah alasan penolakan, bukan fitur. Filter schema sistem sekarang ada **dua kali**:
   di WHERE (biar fetch murah; nama-namnya konstanta package, alias tetap ter-bind `database_name = ?`) dan
   di Go (`systemSchemas`, tetap aturan yang teruji dan case-insensitive). Testi mengunci bentuk terketatnya:
   satu-satunya teks berquote yang boleh muncul di statement katalog adalah anggota `systemSchemas`.
6. **`constraint_type` tidak difilter di SQL.** Ejaannya milik tiap scanner — `PRIMARY KEY`/`NOT NULL`
   terukur dari `sqlite_scanner`, `FOREIGN KEY` (+ `referenced_column_names`) terukur dari katalog DuckDB.
   Salah eja di WHERE membuat cache melaporkan "tabel ini tidak punya kunci": kegagalan diam yang persis
   seperti yang baru ditutup di §20. Baris `NOT NULL` dibuang di Go karena `is_nullable` sudah membawanya;
   `CHECK` tidak dibawa karena tidak berarti untuk tree, completion, maupun hint join.

Permukaannya sekarang: `DescribeSource` menjalankan tiga query itu pada satu session (jadi jawabannya
selalu sama dengan yang bisa dilihat query yang dijalankan session itu), masing-masing dengan sentinel
`LIMIT 5001`, lalu `buildCatalog` menggabungkan. **Daftar kolom yang menentukan relasi apa saja yang
muncul** — constraint atau baris tabel untuk relasi tanpa kolom dibuang, tidak dibuatkan entri. Testi
kunci/`FOREIGN KEY`/nullability/taksiran berjalan tanpa server (`TestDescribeSourceReadsKeysFromADuckDBCatalog`),
jalur scanner SQLite-nya tetap lewat bundel + fixture.

Yang masih belum terukur di katalog ini: `comment` untuk Postgres/MySQL (sumbernya tidak ada di mesin ini),
spelling `constraint_type` MySQL nyata, dan apakah `postgres_scanner` memajang mirror `information_schema`
seperti yang dikhawatirkan butir 5. Semua angka di §20 tentang Postgres tetap berlaku: sample-nya mati di
sisinya sendiri, dan dicoba ulang 2026-11-02 lewat `AttachPostgres` masih membalas
`FATAL: could not open file "global/pg_filenode.map": I/O error`.

## 22. Path sumber file: apa yang dilakukan DuckDB pada sebuah string (2026-11-02)

BRD §5.5 memutuskan "bentuk kanonik = relatif terhadap folder `.muara`", tapi alasannya belum pernah diukur.
Pengukurannya satu hal: **kirim path apa adanya ke reader, lalu lihat berkas mana yang terbaca**. Semua angka di
bawah dari DuckDB **v1.1.3** (dibaca lewat `SELECT version()` di session yang sama), darwin/arm64, dan sekarang
dikunci oleh `engine/duckdb/filesource_path_cgo_test.go` — jadi upgrade DuckDB yang mengubah perilaku akan gagal di
tes, bukan diam-diam membaca berkas yang salah di mesin user.

| Perlakuan | Percobaan | Terukur | Aturan yang lahir |
|---|---|---|---|
| Path **relatif** | `read_csv_auto('orders.csv')` dengan CWD=A, lalu view yang sama dibaca setelah `chdir` ke B | baris berubah (1 → 4): resolusinya **terjadi lagi setiap pembacaan**, bukan sekali saat `CREATE VIEW` | resolver harus menelan path jadi absolut **sebelum** string itu masuk SQL |
| Path **absolut** | berkas yang sama didaftarkan dari dua CWD yang tidak berhubungan | 3 baris dari kedua CWD | bentuk yang dipakai engine hari ini sudah benar; yang bocor hanyalah bentuk tersimpan |
| `../` | `read_parquet('../p1.parquet')` dengan target ada | **terbaca** (2 baris) | penolakan "keluar folder bundle" bukan kemampuan DuckDB, jadi ia harus ditegakkan di Go |
| `~` | daftarkan `~/muara-no-such-file-9c1a.csv` | `IO Error: No files found that match the pattern "~/muara-no-such-file-9c1a.csv"` — literalnya utuh | `~` ditolak, bukan diekspansi diam-diam |
| `*` `?` | dua berkas `x1.csv` (1 baris) + `x2.csv` (2 baris), dibaca lewat `x*.csv` dan `x?.csv` | **3 baris** — keduanya terbaca, di csv, json, dan parquet | satu sumber = satu berkas; karakter glob ditolak dengan alasan |
| `[ ]` | nama berkas sungguhan `log[1].csv` | terbaca, tapi karena `[1]` adalah **kelas karakter** yang kebetulan cocok | ikut ditolak: `log[12].csv` akan membaca `log1.csv`/`log2.csv`, bukan dirinya |
| `disable_glob_matching` | `read_csv_auto('…', disable_glob_matching=true)` | `Binder Error: Invalid named parameter … for function read_csv_auto` | tidak ada jalan keluar dari sisi reader, jadi aturannya ada di kita |
| Berkas hilang vs folder vs pattern tak cocok | `/nope/gone.csv`, sebuah folder, `''` | ketiganya membalas **pesan yang sama**: `No files found that match the pattern "<ditulis persis>"` | pre-flight M3 tidak boleh membaca error DuckDB; ia `os.Stat` di Go (dan `DESCRIBE` untuk nasib "skemanya berubah") |

Pelajarannya bukan "DuckDB aneh" tapi **di mana letak keputusannya**: argumen reader adalah *pattern yang
diselesaikan terhadap folder kerja*, bukan nama berkas. Karena itu aturan path disimpan di engine
(`backend/engine/duckdb/filesource_path.go`) dan bukan di salah satu produk — web menyimpan path server, desktop
menyimpan path relatif-bundle, keduanya sekarang mendapat kalimat error yang sama.

Bentuk API-nya dua fungsi, dan arahnya penting:

- `CanonicalFileSourcePath(path, anchorDir)` — **saat menyimpan**: berkas di dalam folder bundle jadi
  `data/orders.csv` (slash, dinormalisasi, `./` dan `..` yang kembali ke dalam dihilangkan); berkas di luar
  tetap absolut dan **tidak** ditulis ulang jadi `../...`, supaya definisinya jujur bilang "ini milik mesin ini".
- `ResolveFileSourcePath(stored, anchorDir)` — **saat membuka**: absolut tinggal dibersihkan, relatif digabung ke
  anchor lalu diuji tidak keluar (`ErrFileSourceOutsideAnchor`). `anchorDir` kosong berarti "tidak ada bundle"
  (jalur web) → hanya absolut yang diterima, **tidak pernah** resolve terhadap CWD.

Satu-satunya tempat ia bisa salah adalah asumsi lintas-OS, dan itu sengaja diukur dua arah: `C:\…`, `C:/…` dan
`\\server\…` dikenali sebagai absolut di setiap platform (`filepath.IsAbs` hanya menjawab untuk platform tempat
binernya dikompilasi), sementara `data\orders.csv` di Unix memang jadi satu nama berkas yang tidak ada — nasib
"path tidak ada, bisa ditunjuk ulang", bukan path yang dilempar ke `Dir`. **Yang belum terukur di mesin ini**:
perilaku resolver pada bundle yang benar-benar ditulis di Windows (butuh mesin Windows, M0 pertanyaan 1–3) dan
batas jumlah/ukuran berkas per bundle (M3, dari angka nyata).

**Konsumen kedua sudah memakainya (2026-11-02, M3).** `internal/bundle/store.go` memanggil
`CanonicalFileSourcePath` dengan anchor = folder berkas `.muara` saat **menyimpan**, dan
`ResolveFileSourcePath` dengan anchor yang sama saat **membuka** (`Bundle.ResolveFileSources`). Tiga hal yang
perlu diingat orang berikutnya yang menyentuh engine:

- Store **tidak** menambah aturan path sendiri. `~/…` dan `../…` yang ditemukan di dalam berkas orang lain
  ditolak dengan `ErrFileSourceHomeRelative`/`ErrFileSourceOutsideAnchor` dari engine, dibungkus nama alias yang
  bersangkutan — satu kalimat error, dua produk.
- Yang **tidak** dilempar engine dan tidak boleh ditebak pemanggil: berkas yang hilang. Engine membalas
  `No files found that match the pattern` untuk berkas hilang, folder, dan pattern tak cocok (baris terakhir
  tabel di atas), jadi `Open` sengaja **membuka** bundle tanpa sumbernya ada dan membiarkan pre-flight M4
  memisahkan tiga nasib itu dengan `os.Stat` + `DESCRIBE`.
- Batas ukuran ikut terukur dari sisi definisi: **bundle kosong 1189 byte**, dan 1000 View dengan
  `builder_state` + `raw_sql` yang berbeda-beda = **625 KB** `views.json` tak terkompresi → **16 KB** di dalam
  zip. Angka itu menjawab sebagian pertanyaan "batas berapa" — definisi murah, yang berat adalah berkas data di
  *samping* `.muara`, dan itu belum punya angka sampai bundel benar-benar ikut dipindah.
