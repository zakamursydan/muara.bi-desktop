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
| `duckdb.QueryResult` (tag Go snake_case) | `columns`, `rows`, `row_count`, `duration_ms`, `row_limit`, `truncated`, `truncated_by` |
| Struct desktop (`EngineStatus`) | `duckdbVersion`, `maxMemory`, `threads`, `tempDirectory`, `extensionDirectory` |

Tiga hal lain yang terbukti di jembatan ini:

- Guardrail tetap milik engine, tapi **`ValidateQuery` tidak dipanggil `Session.Query`** — ia lapisan
  pemanggil. Jembatan desktop memanggilnya sendiri sebelum `Query`, jadi `DROP TABLE` ditolak dengan
  `ErrForbiddenStatement` di sisi Go, bukan disembunyikan di UI. Web melakukan hal yang sama di
  `internal/service`; dua konsumen, dua titik panggil, satu implementasi.
- Kontrak M1.4 bertahan melintasi serialisasi: `SELECT * FROM range(20000)` dengan cap default
  menghasilkan `row_count = 10000`, `truncated = true`, `truncated_by = "row_limit"` di JS.
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



