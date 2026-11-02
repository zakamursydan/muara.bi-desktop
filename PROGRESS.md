# PROGRESS — muara.bi Desktop

## 1. Cara melanjutkan (prompt standar)

```
Halo, saya melanjutkan pengembangan muara.bi Desktop (produk offline single-user, terpisah dari
muara.bi web).
Tolong baca PLAN.md, docs/brd-review.md, docs/engine-map.md, dan PROGRESS.md. BRD produk ada di BRD.md (v2).
Lanjutkan eksekusi task berikutnya sesuai checklist di PLAN.md!
```

### Versi prompt untuk mesin Windows (2026-11-01)

Tempel ini di sesi pertama di Windows — isinya supaya dia tidak mengulang pengukuran yang sudah selesai
dan langsung jatuh ke empat pertanyaan yang memang hanya bisa dijawab di sana:

```
Saya pindah ke mesin Windows untuk mengerjakan spike M0 muara.bi Desktop.
Baca dulu: PLAN.md (M0), docs/engine-map.md §12-§13, PROGRESS.md §3 dan §4.

Kondisi mesin ini: Windows, repo ada di path yang kutulis di bawah. Engine TIDAK boleh disalin —
desktop memakai `replace muara.bi/engine => ../muara.bi-service/backend/engine`, jadi kedua repo
harus berada sebagai folder sepupu dengan nama persis itu.

Alat yang harus ada sebelum mulai (kalau belum, bilang dulu, jangan kerjakan sambil menebak):
- Go 1.27.1 ke atas — `go.mod` minta versi itu dan `GOTOOLCHAIN=auto` akan mengunduhnya (butuh jaringan sekali).
- gcc dari **mingw-w64** di PATH (`gcc --version` harus keluar). Tanpa ini CGO mati dan tidak ada
  DuckDB sama sekali. MSVC tidak dipakai.
- Node + npm untuk `npm ci && npm run build` di `frontend/`. Urutannya wajib: `frontend/dist`
  di-gitignore dan `go:embed` membutuhkannya lebih dulu, jadi `go build` tanpa `npm run build` gagal
  dengan `pattern all:frontend/dist: no matching files found`.
- Di PowerShell set env-nya `$env:CGO_ENABLED="1"` (bukan `CGO_ENABLED=1 go build` seperti di shell unix).

Kerjakan empat pertanyaan Windows di PLAN.md M0, satu per satu, masing-masing dengan bukti.
Urutan langkahnya sudah ditulis sebagai CI di `.github/workflows/desktop.yml` — jalankan langkah
yang sama di mesin ini; workflow itu **belum pernah jalan di runner mana pun**, jadi hasil di
mesin Windows adalah pembuktian pertamanya.
1. Link `cmd/spike` dengan CGO. Asumsi yang sudah terbukti dari modul: go-duckdb v1.8.5 punya
   `deps/windows_amd64/libduckdb.a` dan direktif cgo-nya memakai `--static -lstdc++`, jadi
   yang dibutuhkan adalah gcc mingw-w64, bukan MSVC. Kalau link gagal, catat error pertamanya.
2. Bisa tidak biner hasil link itu `LOAD` extension dari folder aplikasi? **Tidak perlu Postgres
   untuk pertanyaan ini** — sudah ada fixture SQLite di `tests\fixtures\spike.sqlite` (8 KB, ikut
   ter-commit) dan `cmd/spike -sqlite-file`. Buat bundelnya di mesin ini (butuh jaringan sekali),
   dari folder `backend` repo web, dengan path **absolut** ke repo desktop:
   `go run ./cmd/duckdb-bundle -out <path-repo-desktop>\build\duckdb-extensions`
   (foldernya harus jadi `v1.1.3\windows_amd64`; kalau berkasnya tidak ada, itu jawaban untuk
   pertanyaan 3 dan jadi titik putar ke Tauri/sidecar sesuai exit criteria M0).
3. Buktikan bundel cukup tanpa jaringan: arahkan `DUCKDB_EXTENSION_DIRECTORY` ke folder tadi lalu
   jalankan spike dengan `-sqlite-file tests\fixtures\spike.sqlite -table orders` (harus keluar
   `tables:1` dan `rows_in_orders:6` tanpa satu pun koneksi keluar), lalu ulangi dengan DSN
   Postgres lewat env, bukan yang di-hardcode. Sertakan juga hasil
   `go test ./internal/meminfo -run TestTotalReportsASanePhysicalSize` — pembaca RAM Windows
   (`kernel32!GlobalMemoryStatusEx`) sudah ditulis dan sudah di-cross-compile, tapi belum pernah
   dijalankan di mesin Windows sungguhan. `go test ./...` di repo desktop juga menjalankan
   `TestSpikeSQLiteFixtureReachesTheGrid`, jadi kalau ia hijau, jalur binding→engine→grid sudah
   terbukti di Windows tanpa perlu server apa pun.
4. WebView2: pastikan **runtime**-nya ada, bukan browser Edge-nya — yang dicari folder
   `C:\Program Files (x86)\Microsoft\EdgeWebView\Application` (atau kunci registri
   `HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`).
   Windows 11 umumnya sudah punya, Windows 10 belum tentu. Catat versinya, jalankan aplikasi tanpa runtime
   kalau bisa diuji, dan putuskan apa yang dilakukan installer kalau hilang (unduh Evergreen atau tolak
   install). Ini ikut menentukan lantai Windows produk (10 vs 11).

Aturan yang tetap berlaku: tulis hasil dalam bahasa Indonesia mengikuti gaya bullet yang sudah ada,
tick checklist PLAN.md, commit tapi JANGAN push tanpa kubilang, jangan pernah commit rahasia/`.env`/
`.muara`/`*.duckdb_extension`, engine tidak boleh tahu soal tenant, dan `MACOSX_DEPLOYMENT_TARGET` /
pin `CGO_LDFLAGS=-mmacosx-version-min=14.0` itu urusan macOS — jangan dipakai di sini.
```

**Sebelum pindah, dua hal yang hanya bisa kamu lakukan:** (a) repo desktop ini **belum punya remote**, jadi
bawa kedua folder sebagai salinan (`muara.bi-service` dan `muara.bi-desktop`, tetap sebagai folder sepupu —
`.tools/` di-gitignore, jadi CLI `wails3` harus dipasang ulang di Windows), atau buat dulu remote GitHub-nya;
(b) `gh` tidak terpasang di mesin ini, jadi aku tidak bisa membuatkan repo itu sendiri.

## 2. Sumber kebenaran

| Dokumen | Isi | Catatan |
|---|---|---|
| [BRD.md](BRD.md) | produk, keputusan arsitektur, FR-1..FR-15, kontrak wajib §7, roadmap, skenario penerimaan §13 | **revisi v2 (2026-10-30)**, menggantikan `~/Downloads/DESKTOP-BRD.md` v1 |
| [docs/brd-review.md](docs/brd-review.md) | koreksi BRD terhadap kode nyata, dengan file:line | baca ini sebelum mengerjakan §9 BRD |
| [docs/adr/0001-engine-module.md](docs/adr/0001-engine-module.md) | kenapa engine jadi modul sendiri | keputusan diterima |
| [docs/engine-map.md](docs/engine-map.md) | batas modul M1 yang diukur dari `go list` + hasil CGO fail-fast | baca ini sebelum `git mv` |
| [PLAN.md](PLAN.md) | urutan M1→M0→M2..M6 + exit criteria | yang dieksekusi |

## 3. Status terkini

- **Update terakhir:** 2026-11-02
- **Fase aktif:** **M1 selesai 2026-10-31.** M1.1 (peta paket engine), M1.2 (CGO fail-fast), **M1.3 (ekstraksi
  `muara.bi/engine`)**, **M1.4 (kontrak `QueryResult.truncated`)** dan **M1.5 (`max_memory` jadi
  konfigurasi)** tuntas, dan exit criterion terakhir dibuktikan `make image-core` — image web benar-benar
  dibangun dari engine di `muara.bi-service/backend/engine/` (lewat `replace`), bukan dari salinan kedua.
  Spike M0 berjalan **2026-11-01**: modul Go pertama di repo ini, skeleton Wails v3.0.0 berdiri, dan
  **React/Vite sudah jadi UI-nya** (binding memanggil engine, `frontend/dist` ter-embed di biner).
- **Capaian sesi ini:** revisi BRD v2 dirampungkan dan diselaraskan dengan keputusan "View boleh lintas
  project" (BRD §5.2/FR-9/FR-10/FR-11/§13 langkah 8/§15, PLAN M3, wireframe S4a) → **M1.1** pengukuran
  batas engine dengan `go list` ([docs/engine-map.md](docs/engine-map.md)) → **M1.2** CGO fail-fast di
  engine web + dua tes baru, suite hijau dua-duanya (`CGO_ENABLED=1` dan `=0`) → **BRD §5.7 baru**
  ("Tiga jalur transport": langsung · VPN · bastion-sebagai-forwarder-lokal) lahir dari pertanyaan user
  "host-nya privat tapi laptop sudah connect VPN, apakah muara.bi masih bisa connect?" — jawabannya ya,
  karena yang menghubungi database adalah proses di laptop, ikut routing & DNS VPN, sama seperti DataGrip.
  Tiga blocker ikut ditemukan dan masuk BRD §10 butir 7-9. → **M1.3** ekstraksi engine di repo web:
  9 `git mv` (semua tercatat sebagai rename, tidak pernah ada salinan kedua) → `engine/go.mod`
  (`module muara.bi/engine`) + `replace` di `backend/go.mod` → import ditulis ulang di 6 titik → prefix
  `entity.` dilepas dan `package service`/`entity` jadi `package builder` → `widget_service.go` memanggil
  `builder.CompileBuilderSQL` → `duckdb.SemanticModel` kehilangan `WorkspaceID`/`ProjectID` →
  `Dockerfile` + `Makefile` disetel ulang untuk modul bersarang. Gerbang 8/8: build/vet/test di dua modul ×
  `CGO_ENABLED=1/0`, `gofmt -l` bersih. → **M1.4** kontrak pemotongan ditegakkan di engine:
  `Session.Query` menerima `QueryOptions{MaxRows, CappedSources}` (web **berhenti** memanggil `ApplyLimit`
  sendiri — lihat catatan di §5), `PlanLimit` memisahkan teks-police dari akuntansi cap, eksekusi memakai
  LIMIT cap+1 sebagai baris penanda yang dibuang, dan `StreamMongoCollections` mengembalikan `[]MongoMount`.
  7 tes baru di `truncation_test.go` + `TestPlanLimit` + `TestCappedMongoViews`; BRD §10 butir 4 ikut
  closed sisi engine. → **M1.5** `GuardrailConfig.MaxMemory` (`duckdb.DefaultMaxMemory = "2GB"`, sama dengan
  angka hardcode yang digantikannya) divalidasi `NormalizeMaxMemory` di `NewEngine` sehingga config salah
  gagal di startup, dan web membacanya dari `DUCKDB_MAX_MEMORY`; tiga tes baru (`TestNormalizeMaxMemory`,
  `TestEngineAppliesConfiguredMemoryBudget`, `TestEngineRejectsUnusableMemoryBudget`), gerbang empat-mode
  tetap hijau. → **Bug DSN + validasi `ssl_mode` ditutup di repo web** (bukan butir checklist M1; permintaan
  user 2026-10-31): `service.postgresDSN` jadi satu perakit conninfo untuk SyncSchema **dan** ATTACH dengan
  `dsnValue` (quote aturan libpq), `normalizeSSLMode` menegakkan enam mode libpq di titik tulis + saat merakit,
  dan tiga handler koneksi berhenti memaksa error validasi jadi 500. Enam tes baru, `go test ./...` hijau di
  dua mode CGO; **masih belum teruji lewat HTTP** — butuh `muara_api` dibangun ulang, dan stack user sedang jalan.
  → **Penutup M1** (`make image-core`, 2026-10-31): build image web hijau dari awal sampai akhir —
  `go mod download` 33 dtk dengan stage build men-COPY `engine/go.mod` ke `./engine/` lebih dulu, tiga binary
  CGO ter-link 206 dtk, extensions `postgres_scanner`/`mysql_scanner`/`sqlite_scanner` ter-bundle,
  `muara-bi-core:dev` 369 MB. Bukti yang paling relevan untuk desktop: `go version -m /usr/local/bin/api`
  (diambil dari dalam container) mencantumkan `muara.bi/engine v0.0.0` **tanpa hash**, jadi engine di-resolve
  dari direktori lokal — satu salinan engine, dua produk, tidak ada duplikasi yang diam-diam.
  → **Profil memori desktop diputuskan (2026-11-01)** (BRD §15 butir 6): probe langsung ke DuckDB membatalkan
  dua catatan sesi kemarin (angka polos justru ditolak DuckDB, bukan dibaca byte; dan `2GiB` selama ini
  ditolak engine padahal DuckDB menerimanya), menghasilkan engine yang sekarang menerima unit biner
  `KiB|MiB|GiB|TiB`, tes budget dua kasus (desimal + biner, bandingkan byte), dan default
  `min(4GiB, RAM/4)` dengan slider FR-15 berlabel GiB. Fakta pembanding yang tidak sengaja ketemu:
  kalau tidak di-SET, DuckDB mengambil **80% RAM** sendiri (`6.3 GiB` di host 8 GB). Keputusan user menyusul
  pagi 2026-11-01: **default web sekalian ikut `2GiB`** (`duckdb.DefaultMaxMemory`, `DUCKDB_MAX_MEMORY`,
  fallback `docker-compose.yml`) supaya dua produk membaca angka yang sama dengan arti yang sama — efeknya
  plafon compose naik 7% jadi 2 × 2,15 GB, yang tetap aman karena DuckDB spill ke disk, bukan mengalokasikan
  plafonnya.
- **Spike M0 dimulai (2026-11-01) — janji M1 diuji dari sisi konsumen kedua:** `go.mod` desktop
  (`module muara.bi/desktop`) memakai engine lewat `replace muara.bi/engine =>
  ../muara.bi-service/backend/engine`, dan `cmd/spike/` memanggilnya tanpa satu baris kode web pun.
  Terukur di mesin ini: default engine sampai ke DuckDB dari modul non-web (`max_memory` → `2.0 GiB`,
  `threads` → 2), dan build `CGO_ENABLED=0` menolak jalan dengan pesan M1.2
  (`duckdb engine unavailable: binary built without CGO`, exit 1) — jadi fail-fast itu milik engine,
  bukan tambalan web. **Temuan yang mengubah rencana bundel:** `ExtensionDirectory` saja tidak menegakkan
  offline — dengan folder bundle kosong, `AttachPostgres` **tetap berhasil** karena DuckDB mengunduh
  `postgres_scanner` ke folder tersebut (sebab `autoinstall_known_extensions` default true dan
  `loadExtension` selalu mengirim `INSTALL`). Diperbaiki di engine: kedua setting di-set `false` dan
  `INSTALL` dilompati selama bundle terkonfigurasi, ditambah
  `TestBundledExtensionDirectoryStaysOffline` (gagal cepat + folder harus tetap kosong). Implikasi M0:
  berkas extension wajib benar-benar ikut ter-copy, dan pesan "extension ... is unavailable" harus
  diterjemahkan UI — saran DuckDB `Install it first using "INSTALL postgres"` menyesatkan di desktop.
  Detail di [docs/engine-map.md](docs/engine-map.md) §8.
- **Skeleton Wails v3 berdiri (2026-11-01, masih M0):** versi **v3.0.0** dikunci di `go.mod` — bukan
  `v3.1.0-nightly.*` yang juga tersedia di proxy, karena spike tidak boleh berdiri di atas moving target.
  Lisensinya **MIT (c) 2018-Present Lea Anthony**, dibaca dari `LICENSE` di dalam modul hasil `go get`
  (`$(go env GOMODCACHE)/github.com/wailsapp/wails/v3@v3.0.0/LICENSE`), bukan dari branch utama — persis
  yang diminta butir checklist. CLI-nya `go install` ke `.tools/bin/wails3` (gitignored, jadi versi tool
  mengikat pada repo, bukan pada PATH mesin). Bentuknya: `main.go` (aplikasi + guard mesin),
  `query_service.go` (satu-satunya jembatan webview→engine: `Status`/`AttachSample`/`Run`/`Close`),
  `assets/index.html` (HTML polos + binding hasil `wails3 generate bindings`) — bentuk pertama yang
  membuktikan binding memanggil engine, **sudah digantikan React/Vite dan dihapus** lihat bullet berikutnya.
  Terukur: `go build` CGO=1 menghasilkan binary 64 MB berisi DuckDB + Wails sekaligus; `go test` lulus —
  `Status()` membalas `max_memory = 2.0 GiB` dan `threads = 2`, `Run()` mempertahankan kontrak M1.4
  (`range(20000)` dengan cap 10000 → `truncated=true`, `truncated_by=row_limit`), dan `DROP TABLE` ditolak
  `ErrForbiddenStatement` **di jembatan**, bukan di UI. Dijalankan 10 dtk: proses hidup dan AssetServer
  melayani `/` lalu `bindings/muara.bi/desktop/{index,queryservice,models}.js` — halaman benar-benar dimuat
  dan binding-nya ter-resolve. Konfirmasi visual (angka muncul di jendela) masih milik user.
- **React/Vite berdiri di atas skeleton (2026-11-01, M0 butir ke-3 tertutup):** `frontend/` sekarang jadi
  satu-satunya UI — `vite 8.3.5` + `react 19.3.1` (versi dikunci di `package.json`), `index.html` +
  `src/{main,App}.jsx` + `src/styles.css` (token warna §12.2 disalin apa adanya dari
  [docs/wireframe.html](docs/wireframe.html), termasuk `prefers-color-scheme: dark`). UI memanggil binding
  hasil `wails3 generate bindings -d frontend/bindings`, jadi tidak ada satu pun jalur HTTP baru.
  Estimasi ruang sebelumnya salah besar: `npm install` react+vite **+plugin** ternyata hanya
  **39 MB** (bukan ±200 MB) dan `@wailsio/runtime 3.0.0` **2 paket, 276 KB** — seluruh M0-c selesai dengan
  ruang yang sudah ada, jadi tidak ada cache yang dihapus. Yang diperlukan supaya build jalan: runtime itu
  jadi **dependensi npm**, karena binding yang di-generate meng-import `@wailsio/runtime`
  (`Call`/`CancellablePromise`/`Create`) — tanpa paket itu `vite build` gagal di
  `Rolldown failed to resolve import "@wailsio/runtime"`. Alternatif `wails3 generate bindings -b`
  (runtime dari AssetServer, bukan npm) sengaja tidak dipakai: bundel JS-nya membuat halaman **tidak**
  bergantung pada `/wails/runtime.js`, sehingga UI tetap utuh kalau Wails mengubah rute internalnya.
  Terukur: `npm run build` → 48 modul, `dist/assets/index-*.js` **247,72 kB** (gzip 78,17 kB) — runtime +
  binding ter-bundle ke dalamnya, artinya tidak ada satu pun unduhan saat aplikasi jalan.
  `main.go` sekarang `//go:embed all:frontend/dist` + `BundledAssetFileServer`, dan HTML polos
  `assets/` **dihapus** (`git rm -r`) supaya cuma ada satu UI. Buktinya jadi tes, bukan run manual:
  `TestEmbeddedViteBuildIsServed` (`embed_assets_test.go`) menyajikan embed lewat handler Wails di
  `httptest`, menuntut `/` memuat `id="root"`, menuntut **setiap** `src`/`href` `/assets/*` di halaman ikut
  ter-embed (kalau Vite mulai menghasilkan fail yang tidak ada di tree embed, tes ini yang berteriak),
  menolak halaman yang masih mengandalkan `/wails/runtime.js`, dan menuntut string `duckdbVersion` ada di
  bundel JS — bukti
  JSON tag engine (§9) menyeberang sampai artefak final. `go test ./...` hijau 4/4.
  Konsekuensi build yang harus ditulis di CI: **`npm run build` sebelum `go build`**, karena `frontend/dist`
  di-gitignore dan `go:embed` gagal dengan `no matching files found` kalau tree-nya belum ada.
- **Empat temuan yang mengubah rencana M0 (2026-11-01):**
  1. **CGO-off tidak bisa dibangun di macOS sama sekali** — `CGO_ENABLED=0 go build ./...` gagal di paket
     `pkg/application` (`undefined: macosApp`, `newPlatformApp`, `mac.GetBundleID`). Butir "matikan CGO →
     aplikasi menolak start dengan pesan jelas" karena itu **bukan** tes runtime desktop: gerbangnya pindah
     ke build/CI (artefak CGO-off tidak boleh ada), sementara `ErrCGODisabled` tetap jadi penjaga runtime
     di web, yang memang bisa dibangun tanpa CGO.
  2. **JSON tag engine menyeberang apa adanya ke JavaScript**: `QueryResult` datang sebagai
     `row_count`/`duration_ms`/`truncated_by`/`row_limit` (snake_case dari tag Go) sementara struct desktop
     sendiri `duckdbVersion`/`maxMemory` (camelCase). Kontrak UI §12 harus menulis nama field per sumber,
     bukan berharap satu gaya.
  3. **React/Vite tertahan ruang, bukan alat — dan estimasi ruangnya yang salah, bukan ruangnya.** Node
     sendiri **ada** — milik GoLand, di
     `/Users/macbookpro/Library/Application Support/JetBrains/GoLand2025.2/node/versions/24.22.0/bin`
     (`node v24.22.0`, `npm 11.19.1`; `npm` hanya jalan kalau folder itu masuk `PATH` karena shebang-nya
     `#!/usr/bin/env node`). Angka "±200 MB" yang menahan butir ini sepanjang hari ternyata **39 MB**
     (terukur, dan `@wailsio/runtime` hanya menambah 2 paket). Pelajarannya dicatat supaya tidak diulang:
     jangan menahan milestone di atas estimasi ukuran — ukur dulu, `npm install` murah.
     Yang masih bisa dibersihkan (keputusan user, bukan tindakan diam-diam): `go clean -cache` ±3,1 GB,
     cache modul Go ±4,6 GB (naik ±1,4 GB hari ini karena pin Wails), `Docker.raw` ±15 GB,
     `frontend/node_modules` web ±747 MB.
  4. **Kesimpulan "`MACOSX_DEPLOYMENT_TARGET` tidak mengubah apa pun" adalah false negative — dan sudah
     dikoreksi di hari yang sama (2026-11-01).** Kesalahannya metode, bukan alat: nilainya diuji dengan `14.0`,
     yang kebetulan **sama dengan default mesin** (tanpa flag, clang memakai versi mayor SDK — diukur dengan
     satu berkas C polos: `minos 14.0`). Tidak ada perubahan lalu disimpulkan tidak berpengaruh. Diuji ulang
     dengan angka yang berbeda: `MACOSX_DEPLOYMENT_TARGET=11.0` pada build non-Wails menurunkan `minos` ke 11.0
     dan memunculkan 43 warning. Kenapa biner **app** tetap 11.0: Wails v3.0.0 menulis
     `#cgo LDFLAGS: -framework Cocoa -mmacosx-version-min=10.13` di `pkg/application/application_darwin.go:8`,
     dan env var itu hanya *default* — ia kalah oleh flag eksplisit dari paket orang lain; ld64 menjepit 10.13
     ke 11.0 (lantai arm64). Yang bisa menyetel angka produk:
     `CGO_LDFLAGS="-mmacosx-version-min=14.0"` → `minos 14.0`, nol warning. Keputusan produk ikut ditetapkan:
     **minimum macOS 14 (Sonoma)**, karena 14.0 target terendah yang link bersih (13.0 → 49 warning, 11.0 → 50)
     dan tidak ada mesin 12/13 untuk membuktikan klaim yang lebih rendah. Tabel lengkap + recipe build ada di
     [docs/engine-map.md](docs/engine-map.md) §13. Pelajaran yang disimpan: **jangan simpulkan sebuah knob tidak
     berpengaruh sebelum mengetesnya dengan nilai yang berbeda dari default.**
- **RAM per OS terbaca, dan default `min(4GiB, RAM/4)` lahir dari angka mesin nyata (2026-11-01, M0
  butir "cara membaca RAM total"):** paket baru `internal/meminfo` — satu fungsi `Total()` dengan tiga
  implementasi per-OS (`meminfo_darwin.go`: `unix.SysctlUint64("hw.memsize")`; `meminfo_linux.go`: baris
  `MemTotal` dari `/proc/meminfo` dikali 1024; `meminfo_windows.go`: `kernel32!GlobalMemoryStatusEx`), plus
  `Budget(total)` yang memotong ke `min(4GiB, RAM/4)`, membulatkan **ke bawah** ke kelipatan MiB, dan tidak
  pernah keluar dari rentang 256 MiB–4 GiB.
  **Jawaban Windows yang tidak diharapkan:** `golang.org/x/sys/windows` **tidak** mengekspor
  `GlobalMemoryStatusEx` (digrep di `x/sys@v0.49.0`: nol hasil), sementara alternatifnya rapuh — `wmic`
  deprecated dan sudah dihapus di build Windows 11 terbaru, WMI adalah service yang bisa mati di mesin
  tempat produk ini justru harus dipasang. Yang jalan: `syscall.NewLazyDLL("kernel32.dll")` + struct
  `MEMORYSTATUSEX` deklarsi sendiri (72 byte, dua `uint32` dulu supaya `uint64` jatuh di offset 8 sama
  seperti layout C). Tanpa CGO, tanpa dependensi baru, tanpa hak admin.
  `golang.org/x/sys v0.49.0` **sudah ada di graph** sebagai indirect (dibawa Wails) — `go mod tidy` hanya
  memindahkannya ke baris require langsung, jadi tidak ada modul baru yang diunduh.
  Terukur: `CGO_ENABLED=0 GOOS/GOARCH ∈ {darwin,linux,windows}×{amd64,arm64} go build ./internal/meminfo`
  **enam-enamnya ok**, dan `go vet` per-GOOS (darwin/linux/windows) bersih — artinya CI bisa membuktikan
  "kode pembaca RAM tersusun di ketiga OS" dari satu mesin, jauh sebelum pertanyaan runtime Windows dijawab.
  Mesin ini: `hw.memsize` = **8589934592 byte** (8 GiB, sama dengan angka `sysctl`), jadi
  `Budget()` = **`2GiB`** — kebetulan identik dengan default engine, dan itu sengaja ditegaskan lewat tes.
  `TestRAMBudgetReachesDuckDB` (`memory_budget_test.go`) mengunci rantai sisanya: `NewEngine` dengan string
  hasil `Budget()` → `current_setting('max_memory')` dibaca → **dibandingkan dalam byte**. Tiga kasus
  (8 GiB→`2GiB`→2³¹, 6 GiB→`1536MiB`→1610612736, 1 GiB→`256MiB`→268435456) lulus. Ukuran RAM-nya
  **sintetis, bukan dari mesin tes**, karena DuckDB menggemakan nilai dalam GiB dengan satu desimal: pada
  mesin ber-RAM ganjil (mis. 7 GiB → 1792 MiB → dikembalikan sebagai "1.7 GiB") gema itu tidak bisa dibalik
  persis, jadi assert terhadap RAM asli akan membuat CI gagal tanpa ada yang salah. `go test ./...` hijau
  dua paket.
  Yang **belum** dikerjakan: memasang angka ini ke `NewQueryService` (sengaja masih kosong supaya spike terus
  membuktikan default engine — pemasangan adalah pekerjaan pengaturan FR-15 di M4). `threads` menyusul
  **terpasang 2026-11-02** lewat profil mesin sendiri (lihat blok `threads` di §3) — beda dari memori karena
  jumlahnya tidak perlu slider.
- **Bundel extension nyata, dan offline terbukti tanpa sudo (2026-11-01, M0):** `cmd/duckdb-bundle`
  (di repo web) dijalankan untuk darwin/arm64 ke folder luar repo → **103 MB** berisi
  `v1.1.3/osx_arm64/{postgres,mysql,sqlite}_scanner.duckdb_extension` (+ tiga `.info` 165–168 byte).
  Angka yang sebelumnya belum pernah diukur siapa pun: **extension connector = 103 MB**
  (postgres 32 MB, sqlite 32 MB, mysql 38 MB) di atas biner aplikasi 62 MiB — jadi ukuran installer produk
  ini realistically ±170 MB, dan itu harus disebut di BRD §9 sebelum ada yang menjanjikan "unduhannya kecil".
  Versi DuckDB yang dikunci go-duckdb v1.8.5 ikut terukur: **v1.1.3**, dan nama folder `<versi>/<platform>`
  adalah alasan bundel **wajib** dibuat per-OS (mesin ini menghasilkan `osx_arm64`, bukan `linux_amd64`).
  **Buktinya bukan "coba dan lihat berhasil":** folder bundel disalin lalu di-`chmod -R a-w` (hanya-baca) dan
  proses dijalankan dengan `HOME=/tmp/muara-fakehome`. Kalau DuckDB mencoba mengunduh apa pun, ia harus
  menulis ke salah satu dari dua tempat — folder hanya-baca (gagal keras) atau HOME default-nya (terlihat).
  Hasil: `ATTACH` Postgres jalan, `tables:1`, `rows_in_orders:6`, **6 file bundel tidak berubah**, dan
  **HOME palsu tetap 0 byte** — kombinasi yang hanya mungkin kalau connector dibaca dari berkas di folder
  aplikasi. Ini sisi positif dari kontrak yang sisi negatifnya sudah dijaga
  `TestBundledExtensionDirectoryStaysOffline` (bundle kosong → gagal cepat, folder tetap kosong).
  Cara mereproduksi: `CGO_ENABLED=1 go build -o /tmp/duckdb-bundle ./cmd/duckdb-bundle` (repo web),
  `/tmp/duckdb-bundle -out /tmp/muara-ext`, lalu
  `HOME=/tmp/muara-fakehome /tmp/muara-spike-cli -extension-directory=<folder-hanya-baca> -pg-dsn="…" -table orders`.
  `*.duckdb_extension` sudah di-gitignore, jadi tidak ada jalur diam-diam untuk ikut ter-commit.
  **Sisa butir ini:** teknik yang sama belum diterapkan di linux dan windows — dan keduanya justru bisa
  dibuktikan lebih kuat di CI (container `--network none` untuk linux, runner windows-latest untuk windows).
  macOS adalah OS yang paling sulit dibuktikan "tidak ada jaringan" tanpa sudo, jadi bukti di atas adalah
  bentuk terkuat yang bisa dicapai dari mesin user.
- **`.app` macOS dibangun dan dipasang (2026-11-01, permintaan user: "build yang mac dulu, install di mac"):**
  `wails3 build` tidak bisa dipakai — ia berhenti di `ERROR task: No Taskfile found` karena skeleton M0 tidak
  pernah punya `build/darwin/Taskfile.yml`, dan `task` tidak dipasang (disk 95% penuh). Gantinya skrip sendiri,
  `build/macos-app.sh`, yang mengerjakan tiga hal yang selama ini cuma jadi wacana di PLAN: (1) build dengan
  `CGO_LDFLAGS="-mmacosx-version-min=14.0"` lalu **membatalkan diri** kalau `otool -l` tidak membalas
  `minos 14.0` — assertion yang dijadwalkan untuk CI sudah aktif di level resep; (2) menulis `Info.plist` dengan
  **`LSMinimumSystemVersion = 14.0`**, kunci yang sebenarnya ditegakkan Finder/Gatekeeper (`minos` di Mach-O tidak
  dibaca siapa pun, jadi lantai macOS 14 baru benar-benar berlaku sekarang, bukan cuma diumumkan) — divalidasi
  `plutil -lint`; (3) menyalin folder bundel extension ke `Contents/Resources/duckdb/extensions` dan
  `codesign --sign -` (ad-hoc). Hasil: `build/bin/muara-bi.app` **149 MB** = biner 62 MiB + bundel 103 MB −
  overlap, terpasang di `/Applications/muara-bi.app`, UI dimuat dari `frontend/dist` ter-embed, `Status()`
  menjawab dari DuckDB. Konsekuensi kode yang harus ikut: `query_service.go` sekarang punya
  `bundledExtensionDir()` → `extensionDirFromBinary()` yang menemukan bundel **relatif ke biner** (dua level di
  atas executable, lalu `Resources/duckdb/extensions`), dengan `DUCKDB_EXTENSION_DIRECTORY` tetap menang untuk
  build scratch — jadi aplikasi hasil double-click di Finder membawa connector-nya tanpa variabel lingkungan apa
  pun, yang juga menjawab kenapa `AttachSample` hanya bisa diuji dari shell: DSN-nya sengaja lewat env, bukan
  dari UI, dan path M4-nya adalah keychain. Dua tes baru (`TestExtensionDirIsFoundFromAppBundleLayout` menyusun
  struktur `.app` di `t.TempDir()` dan menolak path longgar, `TestExtensionDirEnvOverridesTheBundle`) plus suite
  lama: `CGO_ENABLED=1 go test ./...` hijau (`muara.bi/desktop` 3,2 dtk, `internal/meminfo` 1,9 dtk), `gofmt -l`
  bersih. **Dua hal yang terbukti dari proses yang sedang hidup:** `lsof -p <pid>` memetakan
  `/Applications/muara-bi.app/Contents/Resources/duckdb/extensions/v1.1.3/osx_arm64/postgres_scanner.duckdb_extension`,
  dan `Attach` gagal **setelah** itu dengan `FATAL: could not open file "global/pg_filenode.map": I/O error` —
  jadi kegagalannya milik server Postgres sample (stack user: `muara_api`/`muara_web`/`muara_redis` unhealthy,
  host 94% penuh), bukan milik desktop. Sisa M6 tidak berubah: Developer ID + notarization + dmg, karena ad-hoc
  tidak tebus Gatekeeper untuk distribusi.
- **Bukti connector tanpa server: fixture SQLite + CI per-OS ditulis (2026-11-01, gelombang kedua M0):**
  sisa M0 semuanya tersandar pada satu hal yang tidak ada di mesin ini — Windows, hosting CI, dan Postgres
  sample yang sedang rusak. Yang dilepas hari ini adalah **instrumentnya**, supaya pertanyaan-pertanyaan itu
  tinggal dijalankan, bukan dirancang ulang di mesin lain:
  1. `tests/fixtures/spike.sqlite` (8 KB, ter-commit) + `-sqlite-file` di `cmd/spike` +
     `AttachSampleSQLite()` di jembatan. Alasannya spesifik: SQLite adalah satu-satunya sumber nyata yang bisa
     dicapai **tanpa server, tanpa kredensial, tanpa jaringan**, dan runner Windows tidak punya Postgres — jadi
     pertanyaan "biner yang di-link statis bisa `LOAD` extension dari bundel tidak?" tidak perlu lagi menunggu
     database. Terukur di mesin ini: `HOME=/tmp/muara-fakehome spike -extension-directory=<hanya-baca>
     -sqlite-file=tests/fixtures/spike.sqlite -table orders` → `tables:1`, `rows_in_orders:6`, bundel tetap
     hanya-baca, HOME tetap kosong. Satu jebakan yang ketemu: DuckDB membaca tabel attached sqlite sebagai
     `sample.orders`, **tanpa** skema `public` seperti Postgres (`sample.public.orders` gagal) — jadi query
     bawaan UI ikut dua bentuk, dan tombol SQLite hanya menimpa editor kalau isinya masih query default
     (jangan pernah membuang SQL yang diketik user).
  2. `TestSpikeSQLiteFixtureReachesTheGrid` membuktikan jalur binding→engine→grid dengan angka nyata (3 baris,
     3 kolom, `truncated=false`, baris teratas `wholesale` karena revenue 540) — ini yang menutup sisi macOS
     dari butir "SELECT nyata tampil di grid" tanpa menunggu Postgres user sembuh.
     `CGO_ENABLED=1 go test ./...` hijau (`muara.bi/desktop` 1,8 dtk), `gofmt -l` bersih.
  3. `.github/workflows/desktop.yml` — **belum pernah jalan di runner mana pun**, jadi isinya klaim, bukan
     hasil. Job-nya satu matriks (macos-14 / ubuntu-24.04 / windows-latest) yang mengodekan setiap invariant
     yang ditemukan hari ini: `npm run build` sebelum `go build`, pin `CGO_LDFLAGS` + assert `otool` lewat
     `build/macos-app.sh`, bundel per-OS dari `cmd/duckdb-bundle` di repo engine, bukti LOAD lewat fixture
     SQLite, pengulangan bukti dengan bundel hanya-baca + `HOME` kosong, gerbang cross-build `internal/meminfo`
     enam kombinasi, dan langkah WebView2 yang membaca kunci registri `EdgeUpdate\Clients\{F3017226-…}` — bukan
     browser Edge. Ada job kedua yang bentuknya "harus gagal": `CGO_ENABLED=0 go build .` di desktop **tidak
     boleh** menghasilkan biner (temuan 1 §3), jadi gerbangnya adalah build itu sendiri, bukan runtime.
     Yang direhearsal lokal: langkah macOS, langkah bukti offline, gerbang meminfo, `go test`. Yang butuh
     mesin lain: tiga langkah Windows.
     Dua syarat yang harus dipenuhi sebelum workflow ini bisa hijau: repo desktop **punya remote**, dan
     `ENGINE_REPO_TOKEN` (PAT) — `GITHUB_TOKEN` tidak bisa menyeberang repository, padahal `replace
     ../muara.bi-service/backend/engine` menuntut kedua repo ada sebagai folder sepupu dengan nama persis.
  4. `build/macos-app.sh` sekarang **menolak** argumen bundel yang tidak ada dan **tidak lagi** membuat folder
     `Resources/duckdb/extensions` kosong. Itu bug yang baru kelihatan waktu CI ditulis: folder kosong tetap
     ter-resolve oleh `bundledExtensionDir()`, engine mengira ada bundel, `INSTALL` dilompati, dan setiap
     connector melaporkan "unavailable" — kegagalan yang jauh lebih membingungkan daripada tidak dibundle sama sekali.
- **Sumber file v1 dibuka di engine: reader core, tanpa extension per-OS (2026-11-02, M2 / PLAN.md butir
  "Sumber file v1"):**
  1. `duckdb.FileSourceQuery(path)` + `Session.RegisterFileSource(ctx, viewName, path)` memetakan ekstensi ke
     reader yang sudah dikompilasi ke biner DuckDB: `.csv`/`.tsv` → `read_csv_auto`, `.parquet`/`.pq` →
     `read_parquet`, `.json`/`.jsonl` → `read_json_auto`. Bentuknya **temp view dari reader**, bukan attach,
     dan itu pilihan yang disengaja: tiga reader tersebut tidak butuh bundel, jadi produk bisa membuka berkas
     user tanpa menyentuh jaringan — beda fundamental dari Postgres/MySQL/SQLite yang tetap butuh scanner.
     Di luar daftar itu ditolak lewat `ErrUnsupportedFileSource` dengan pesan yang menyebut ekstensi yang
     didukung; tidak ada jalur diam-diam yang mencoba `INSTALL` lalu gagal setelah 30 detik.
  2. Helper `quoteIdent`/`quoteLiteral` **dipindah** dari `session_cgo.go` ke `sqlquote.go` (tanpa tag build).
     Alasannya bukan kerapian: `filesource.go` harus tetap terkompilasi dan teruji pada `CGO_ENABLED=0`, dan
     selama quoting hidup di berkas ber-tag `//go:build cgo` mesin tidak bisa membuktikan path penamaan view
     di mode itu. Gerbang empat-mode (engine+web × CGO 1/0, `build`+`vet`+`test`, `gofmt -l`) hijau.
  3. Terukur offline di mesin ini dengan bundel **kosong dan hanya-baca** serta `HOME` dikosongkan:
     `spike.csv` → 6 baris, `spike.json` → 6 baris, dan `DESCRIBE SELECT * FROM spike_file` mengembalikan
     `id BIGINT`, `channel VARCHAR`, `total DOUBLE` — jadi inferensi tipe yang jadi dasar "kolom" di produk
     sudah terbukti lewat jalur yang sama, bukan lewat perkiraan. `spike-notes.txt` ditolak dengan pesan
     reader. `.tsv` juga terbaca benar (DuckDB mencium delimiter sendiri; `total` malah jadi `BIGINT`).
     Bukti paling penting justru dari berkas yang **tidak ada**: `read_parquet('/tmp/does-not-exist.parquet')`
     membalas `IO Error: No files found that match the pattern …`, bukan "extension tidak tersedia" — artinya
     reader Parquet memang ada di dalam biner, dan kegagalan berikutnya cuma urusan berkas.
  4. **Fixture Parquet menyusul di hari yang sama** — dan cara membuatnya layak dicatat. Berkas
     `tests/fixtures/spike.parquet` (564 B) ditulis oleh sebuah generator sementara yang memanggil
     `go-duckdb` **langsung**, bukan lewat `Session.Query`: guardrail engine melarang `COPY … TO`, dan itu
     benar untuk produk (aplikasi desktop tidak boleh menulis sembarang berkas), jadi jalur pembuatan fixture
     harus sengaja berada di luar jalur produk — generatornya sudah dihapus, tidak ikut ter-commit, dan
     provenance-nya cuma ada di catatan ini plus pesan commit. Hasilnya: parquet terbaca 6 baris dengan
     `id BIGINT`, `channel VARCHAR`, `total DOUBLE` dalam keadaan bundel kosong dan hanya-baca.
     Bedanya juga kelihatan: pada CSV/JSON tipe itu **hasil inferensi**, pada parquet tipe **dibawa oleh
     berkasnya** — jadi schema drift yang user lihat hanya mungkin dari file teks.
  5. `cmd/spike` dapat `-file` + `-view` (env `MUARA_SPIKE_FILE`, sama polanya dengan DSN dan fixture SQLite:
     path datang dari lingkungan, webview tidak pernah bisa menyodorkan path yang akan dibuka aplikasi).
     Di CI env-nya ditambahkan ke **dua langkah bukti yang sudah ada**, bukan bikin langkah baru, karena
     langkah itu jalan di tiga OS dan prefiks `VAR=value cmd` hanya valid di shell POSIX. Langkah pertama
     memakai `spike.csv`, langkah hanya-baca memakai `spike.parquet` (reader yang paling tidak jelas sudah
     terkompilasi — percobaan unduh di situ akan muncul sebagai penolakan menulis, bukan sebagai sukses).
  6. `TestSpikeFileFixturesReadWithNoBundledConnector` (desktop) membaca **ketiga** fixture lewat
     `RegisterFileSource` dengan `ExtensionDirectory` menunjuk folder kosong, lalu memastikan folder itu tetap
     tidak berisi apa pun. Ini yang membuat pertanyaan Windows jadi bisa dijawab di runner: `go test ./...` sudah ada di
     workflow, dan tes inilah yang membuktikan "berkas user terbaca di OS ini tanpa bundel", bukan hanya
     pemetaan di engine. `CGO_ENABLED=1 go test ./...` hijau (desktop 1,5 dtk), `gofmt -l` bersih.
  7. **Keputusan XLSX-source tetap terbuka, tapi sekarang punya dasar:** engine menolak `.xlsx`, sementara
     `engine/exporter/WriteXLSX` (excelize, pure Go) hanya melayani **ekspor**. Jadi "impor .xlsx sebagai
     sumber" bukan sekadar belum dikerjakan — ia menuntut pilihan yang punya konsekuensi janji produk: parser
     Go-side (tetap offline, biaya pengembangan) vs extension `excel` (melanggar claims "sumber file tidak
     butuh bundel") vs "minta user simpan sebagai CSV/Parquet" (murah, tapi memindahkan kerja ke user).
- **CA privat untuk Postgres selesai di web (2026-11-02, M2 / BRD §10 butir 8):** `ConnectionConfig.RootCert`
  (`root_cert`) ikut tersimpan di payload terenkripsi JSON, jadi **tidak ada migrasi DB** — dan satu perakit DSN
  (`service.postgresDSN`) berarti field itu otomatis terpakai di dua jalur: `SyncSchema` (pgx) dan ATTACH DuckDB.
  Dua hal di dalamnya diukur, bukan ditebak:
  1. **Kata kuncinya `sslrootcert`, bukan `rootcert`.** BRD dan catatan lama menulis "rootcert" — nama yang
     ternyata ditolak: probe `ATTACH` dengan DSN `host=127.0.0.1 port=1 ... sslmode=require rootcert=/tmp/ca.pem`
     membalas `invalid connection option "rootcert"`, **pesan yang sama** dengan opsi karangan sendiri
     (`boguscert=`), sedangkan `sslrootcert=` lolos parsing dan berhenti di "server tidak menerima koneksi".
     Karena libpq memvalidasi nama parameter sebelum menghubungkan apa pun, probe ini tidak butuh server TLS —
     dan bentuk tesnya ikut jadi pola: bandingkan kata kunci yang ditolak dengan kata kunci rekaan.
     Tes kedua membuktikan CA-nya benar-benar *dibuka*: `pgx.ParseConfig` gagal saat path menunjuk berkas yang
     tidak ada. Tanpa tes itu, "parameternya diterima" bisa berarti "diabaikan".
  2. **CA hanya diterima bersama `verify-ca`/`verify-full`.** `require` menegosiasi TLS tetapi menerima
     sertifikat apa pun, jadi menyimpan CA di samping `require` menghasilkan profil yang **terlihat** terverifikasi
     padahal tidak ada pemeriksaan — cacat kelas sama dengan `verify` yang sudah ditutup 2026-10-31. Ditambah
     aturan path **absolut** (path relatif resolved terhadap CWD proses server → dua environment memuat CA
     berbeda dari satu profil) dan penolakan teks PEM (orang menempel isi sertifikat, padahal libpq butuh nama
     berkas). Validation di `UpsertEnvironment` → 400 sebelum enkripsi, persis pola §3 yang lalu.
     Lima tes baru di `session_preparer_test.go`, termasuk satu CA self-signed yang benar-benar dibuat
     (`crypto/x509`) supaya `TLSConfig.RootCAs` bisa dipakai sebagai bukti, dan satu tes injeksi: password berisi
     `sslrootcert=/etc/hosts` tidak boleh menambah parameter (pgx akan error kalau mencoba membacanya).
     Gerbang empat-mode hijau; `gofmt -l` bersih.
  Belum: CA untuk **MongoDB** (`tlsCAFile`, masih mungkin karena Mongo lewat driver Go), path relatif CA saat
  bundle pindah mesin (M3), dan UI form web (hari ini `root_cert` hanya bisa lewat API).
- **Builder MySQL dibetulkan + grammar `mysql_scanner` diukur per karakter (2026-11-02, temuan 2026-10-31):**
  cacat sampingan yang muncul waktu membaca builder CA — `attachConnection` web merakit
  `%s:%s@tcp(%s:%d)/%s`, bentuk driver Go, dan **scanner menolaknya** (`Invalid dsn … - expected key=value
  pairs separated by spaces`). Jadi jalur query MySQL tidak pernah bisa jalan lewat DuckDB sama sekali, bukan
  hanya rapuh terhadap karakter aneh; password berisi `@` atau `/` merusak string lebih dulu, persis pola bug
  `sslmode` ganda.
  1. **Grammar diukur, bukan ditebak.** Probe pakai alamat mati `127.0.0.1:1`: scanner memvalidasi nama opsi
     dan bentuk token *saat membaca string*, jadi "Can't connect" = stringnya dimengerti, `Invalid dsn` /
     `Unrecognized configuration parameter` = ditolak. Yang terbuktikan (tabel lengkap di engine-map §17):
     kunci hanya `host, port, user, passwd, db, socket` · nilai kosong harus dihilangkan (`passwd=` jadi opsi
     bernama kosong) · spasi/tab/`=` wajib dalam kutip ganda · di dalam kutip hanya `\\` dan `\"` yang sah,
     di luar kutip backslash **verbatim** (`C:\path` benar apa adanya) · kutip di tengah nilai aman, tapi nilai
     yang **diawali** `"` wajib diekscape (`unterminated quote`) · `port=0` diam-diam jadi 3306.
     Dua kali tebakan saya sendiri salah dan ketahuan cuma karena diukur: awalinya mengira backslash selalu
     perlu dilipatgandakan, dan lupa bahwa kutip di posisi pertama mengubah mode parsing.
  2. **Perakitnya pindah ke engine** (`muara.bi/engine/duckdb/mysql.go`: `MySQLSource`, `MySQLDSN`,
     `ErrUnusableMySQLSource`) karena aturannya milik scanner — dua produk harus mengirim string yang sama.
     Web tinggal `attachConnection` → `duckdb.MySQLDSN(...)`, dan sumber yang mustahil (host kosong, port di
     luar 0-65535) berhenti sebelum `ATTACH`.
  3. **CA untuk MySQL ditutup sebagai "tidak bisa di jalur ini", bukan ditunda.** `ssl-ca`, `ssl-capath`, dan
     `ssl-mode` semua `Unrecognized configuration parameter`, dan satu-satunya opsi ATTACH yang dikenal scanner
     adalah `read_only` — kredensial pun tidak bisa lewat opsi. Menyimpan `root_cert` di profil MySQL berarti
     profil yang **terlihat** terverifikasi padahal TLS-nya mati, jadi `checkRootCertDriver` menolaknya di titik
     tulis (400 dengan nama driver + alasannya). MySQL self-signed tetap bisa diakses lewat bastion/VPN (§5.7
     aturan 1&2).
  Bukti: `engine/duckdb/mysql_test.go` (grammar murni, 5 tes) + `mysql_scanner_test.go` (tingkat scanner —
  11 sumber termasuk semua bentuk jebakan diterima scanner, dan 4 bentuk yang **tidak** kami emit ditolak:
  `user:pass@tcp()/db`, `mysql://…`, `ssl-ca=`, `rootcert=`) + 3 tes sisi web di `session_preparer_test.go`
  (`recordingSession` membuktikan DSN yang dikirim ke `AttachMySQL` dan bahwa sumber ditolak tidak pernah
  sampai ATTACH). Tes scanner butuh `DUCKDB_EXTENSION_DIRECTORY` menunjuk bundel berisi `mysql_scanner`; tanpa
  itu di-*skip*, karena hasilnya adalah pernyataan tentang scanner, bukan tentang kode kita. Batas yang jujur:
  yang terukur adalah **parsing**, bukan byte yang dikirim ke server — error scanner menampilkan string asli,
  bukan hasil unescape, jadi kepastian bit-per-bit butuh server MySQL sungguhan. Gerbang empat-mode
  (engine+web × `CGO_ENABLED=1/0`) hijau; `gofmt -l` bersih.
- **CA MongoDB jadi `tlsCAFile` (2026-11-02, penutup butir 2(c) §4):** sumber Mongo tidak lewat DuckDB —
  `StreamMongoCollections` memakai **driver Go**, jadi CA punya tempat di sini, kebalikan dari `mysql_scanner`.
  1. **Probenya sama seperti §17 dan bekerja karena alasan yang sama:** `mongo.Connect` ke `127.0.0.1:1` dengan
     `serverSelectionTimeoutMS=400`; berkas sertifikat dibuka **sebelum** server disentuh, jadi
     `open …: no such file` adalah bukti bahwa kata kuncinya dibaca. Tanpa berkas itu, "tidak ada error" tidak
     berarti apa pun — `bogusCAFile` juga tidak menghasilkan error, karena driver **tidak memvalidasi** nama opsi
     URI. Tabel lengkap di engine-map §18.
  2. **Yang berbahaya bukan hanya "belum ada", tapi "sudah ditulis dan diabaikan":** nama warisan `sslCAFile`
     ditoleransi parser lalu **tidak pernah dibaca**. Profil `?sslCAFile=…` mengira dirinya terverifikasi padahal
     tidak ada yang diperiksa, jadi `withMongoTLS` **menolak** URI yang masih memakannya daripada
     menerjemahkannya — dan menolak pula `tlsCAFile` yang nilainya berbeda dari `root_cert` (dua sumber kebenaran
     untuk satu CA).
  3. **Nilai kosong = error** (`tlsCAFile=` → `open : no such file`), persis pelajaran `passwd=` di MySQL:
     kunci yang tidak dipakai harus dihilangkan, bukan dikosongkan.
  4. **Mode fallback sengaja tidak dipaksa.** Driver cuma bisa nyala/mati: `require|verify-ca|verify-full` →
     `tls=true`; `disable` **dan** fallback libpq `allow|prefer` → tetap polos. Alasannya konkret, bukan selera —
     `environment-form.tsx` mengirim `ssl_mode: "prefer"` untuk **semua** driver, jadi menolak atau memaksa
     fallback ke TLS akan merusak profil Mongo yang hari ini jalan (termasuk sample mongo di stack user, yang
     tidak ber-TLS). Yang dijaga ketat: `root_cert` di samping mode tanpa TLS **ditolak**, karena CA yang
     tersimpan tapi tak terpakai itulah yang membuat profil berbohong. `verify-ca` vs `verify-full` tidak bisa
     dibedakan driver (Go selalu memverifikasi rantai dan nama host selama `tlsInsecure` tidak dipasang), jadi di
     Mongo bedanya cuma menandai niat operator.
  Titik tulis sekarang bertanya ke connector-nya sendiri (`normalizeStoredRootCert` menggantikan
  `checkRootCertDriver` yang berumur satu hari): gerbang mode libpq untuk Postgres, gerbang TLS untuk Mongo,
  ditolak dengan alasan untuk sisanya. Bukti: 4 tes URI/validasi baru di `session_preparer_test.go` +
  `TestMongoDriverOpensTLSCAFileButNotSSLCaFile` di engine. Belum: verifikasi terhadap server Mongo
  bersertifikat sungguhan (infra TLS milik user) — yang terbukti adalah berkas CA benar-benar dibuka.
  Gerbang empat-mode hijau; `gofmt -l` bersih.
- **`threads` ikut profil mesin (2026-11-02, penutup butir 1 §4 yang terakhir):** pertanyaan M0
  "apakah `threads` perlu ikut profil — masih hardcoded 2" tidak bisa dijawab dari repo ini karena angkanya
  hidup di engine. Urutannya jadi dua langkah: engine diberi pintu (`GuardrailConfig.Threads` +
  `NormalizeThreads`, knob web `DUCKDB_THREADS` default `2`), lalu desktop memilih angkanya.
  - **Yang diukur (DuckDB 1.1.3, `SET threads` di mesin 8 core):** tidak pernah disetel → **8** (default DuckDB
    = semua core); `0` dan `-1` → `Syntax Error: Must have at least 1 thread!` dan **nilai sebelumnya tetap
    berlaku** — kegagalan SET tidak membuat sesi aman, hanya membuat konfigurasi terlihat jalan; `999` →
    **diterima**, jadi tidak ada cap internal yang bisa diandalkan; `abc` → error cast.
  - **Aturan engine:** `0` = "belum dikonfigurasi" → `DefaultThreads = 2` (nilai nol itulah yang Go berikan untuk
    int yang tidak diisi, padahal DuckDB menolaknya); negatif **ditolak di `NewEngine`** sama seperti `0GiB`;
    di atas core yang terlihat proses **dijepit** mengikuti preseden `ApplyLimit` — angka yang benar-benar
    berlaku tetap terbaca dari `current_setting('threads')`, dibuktikan `TestEngineAppliesConfiguredThreads`
    (1, 2, dan 999 masing-masing kembali persis seperti yang sudah dijepit).
  - **Profil desktop:** `threadBudget() = runtime.NumCPU()-1`, minimal 1 → **7 di mesin ini**, dipasang di
    `NewQueryService`. Alasannya: satu user, satu sesi aktif, jadi hampir seluruh mesin boleh dipakai dan satu
    core ditinggal untuk jendela aplikasinya sendiri. **Ini kebijakan, bukan ambang hasil ukur** — yang terbukti
    cuma bahwa angkanya sampai ke DuckDB (`TestSpikeStatusReportsEngineDefaults` membaca ulang
    `current_setting('threads')`, bukan membandingkan dengan angka hardcoded) dan selalu di dalam mesin
    (`TestThreadBudgetStaysInsideTheMachine`). `max_memory` **tetap** dibiarkan kosong di sini: pemasangannya
    menunggu UI pengaturan FR-15 karena nilainya harus bisa ditimpa user, sedangkan jumlah thread tidak.
    `cmd/spike` tidak disentuh — tugasnya justru membuktikan default engine sampai tanpa konfigurasi.
  - **Yang masih terbuka:** apakah `cores-1` benar-benar lebih baik daripada 2 untuk `ATTACH` lewat jaringan
    adalah pertanyaan performa dan butuh jalur Postgres non-localhost (infra milik user); dan `runtime.NumCPU()`
    **belum diukur** di container dengan `--cpus` (quota tidak mengubah affinity), jadi di web angka itu ditulis
    sebagai batasan yang harus disetel manual, bukan sebagai fakta.
  - Gerbang empat-mode hijau (engine + web, `CGO_ENABLED=1/0`), `gofmt -l` bersih, `go test ./...` desktop
    hijau. Detail pengukuran di docs/engine-map.md §19.
- **Keputusan user tambahan (2026-10-30, sudah masuk BRD v2):**


  - `Project` membungkus **Dashboard**; `View` tidak punya `project_id` dan **boleh direferensi dashboard dari
    project mana pun** (revisi 2026-10-31: keanggotaan View = pin/referensi, bukan kepemilikan, supaya query
    yang sama tidak perlu digandakan per klien). Connection/alias/semantic model tetap berlaku satu berkas.
  - Berkas koneksi saat export dipecah **tiga tingkat**: selalu ikut (alias/driver/database) · ikut dengan
    toggle (host/port/user/SSH) · tidak pernah ikut default (password/token → keychain per mesin).
    Ini memperbaiki v1 yang mengeluarkan host dan membuat "share ke rekan satu server" menyakitkan.
  - Ekspor hasil ke CSV/XLSX/JSON/Parquet = **reuse** `engine/exporter/` (dulu `internal/exporter/`;
    `WriteXLSX` sudah ada, excelize streaming, pure Go) dan **tidak mewarisi LIMIT tampilan**.
  - Impor `.xlsx` sebagai *sumber* data ditunda dari v1 (berbeda dari ekspor — jangan tertukar).
- **Keputusan yang sudah dikunci:**
  1. Ekstraksi engine **mendahului** spike platform (membalik urutan §14 BRD) — karena aturan
     `internal/` memaksa; diverifikasi dengan build gagal: `use of internal package
     muara.bi/core/internal/duckdb not allowed`.
  2. Windows ikut di spike M0 (bukan menyusul), dengan tiga pertanyaan yang harus dijawab pakai bukti.
  3. Sumber file CSV/Parquet/JSON masuk v1; XLSX diputuskan terpisah; konsekuensinya bundle butuh
     aturan **path relatif** yang belum ada di BRD.
  4. Penetapan pemegang engine ditunda; prioritas ke desktop. Rambu anti-duplikasi: tidak ada salinan
     engine di repo ini, perubahan engine harus hijau di dua konsumen.
- **Verifikasi teknis yang jadi dasar keputusan (semua dari `muara.bi-service` hari ini):**
  - Suite web hijau: `go build ./... && go vet ./... && go test ./...` exit 0, `gofmt -l` bersih.
  - **Kemerdekaan engine, dibuktikan dengan grep (2026-10-31):** nol `muara.bi/core` di seluruh `backend/engine/`,
    dan dependensi eksternalnya cuma empat — `go-duckdb`, `excelize/v2`, `mongo-driver`, `golang.org/x/crypto/ssh`.
    Tidak ada `pgx`, tidak ada `chi`, tidak ada `asynq`: desktop tidak akan pernah menarik server web.
  - **Jebakan modul bersarang** (berlaku juga untuk repo desktop nanti): begitu `engine/go.mod` ada, `./...`
    di `backend/` **berhenti** menyentuh paket engine — tes DuckDB bisa hilang tanpa ada yang sadar. Karena itu
    `make test/vet/tidy` kini menjalankan dua modul, engine lebih dulu, dan `Dockerfile` men-COPY
    `engine/go.mod`+`go.sum` sebelum `go mod download`.
  - BRD §9.1 (Mongo tidak ter-attach di jalur query) **sudah selesai**: `session_preparer.go:208`.
  - BRD §9.2 benar: `connection_service.go:321` hanya Postgres, `:367` `return nil` untuk driver lain.
  - BRD §9.3 benar dan lebih parah: `ColumnDef` hanya `{Name, Type}` (`connection_service.go:344`).
  - BRD §9.4 benar: `DefaultMongoDocLimit = 10000` (`nosql_adapter.go:18`, dipakai di `:201`).
  - Temuan baru yang tidak ada di BRD: `session_fallback.go` (jalur `!cgo`) mengembalikan `nil` untuk
    semua `Attach*` dan menjawab `Query` dari memori — build tanpa CGO akan **berhasil palsu**, bukan
    gagal. **Dibereskan 2026-10-31** (M1.2): `NewEngine` kini mengembalikan `duckdb.ErrCGODisabled` dan
    stub tidak bisa lagi menghasilkan `QueryResult`; dibuktikan `cgo_disabled_test.go` dan
    `cgo_enabled_test.go`.
  - Fakta lisensi untuk produk closed-source: Wails MIT, DuckDB MIT, go-duckdb MIT. Syarat: notis
    pihak ketiga ikut didistribusikan. Verifikasi ulang di tag Wails v3 yang dikunci saat M0.
  - **Transport (BRD §5.7), diverifikasi dari kode 2026-10-31:** bastion hanya di-dial pada Test Connection
    (`connection_service.go:252-262`); `SyncSchema` (`:321`) dan `attachConnection`
    (`attachConnection` → `service.postgresDSN`) memakai `cfg.Host` mentah → di belakang bastion, UI hijau tapi query gagal.
    DuckDB tidak menerima `net.Conn`, jadi solusinya forwarder TCP lokal + DSN `127.0.0.1:<port>`.
  - Bug produk web, **direproduksi lalu DIPERBAIKI 2026-10-31**: `SyncSchema` menghasilkan
    `... sslmode= sslmode=disable` saat `ssl_mode` kosong; `pgx.ParseConfig` menolaknya dengan
    `cannot parse ...: failed to configure TLS (sslmode is invalid)` → SyncSchema Postgres gagal total untuk
    koneksi tanpa `ssl_mode`, sementara jalur query memberi default `disable` (dua jalur, dua perilaku).
    Sekarang satu perakit DSN dipakai keduanya (`service.postgresDSN`), dan nilai dari user di-quote per aturan
    libpq sehingga spasi tidak memecah nilai dan kredensial tak bisa menyuntik parameter lain. Enam tes
    (`session_preparer_test.go`, salah satunya membuktikan bentuk DSN lama benar-benar ditolak pgx).
    Manfaat untuk M2: pekerjaan forwarder bastion sekarang cuma menyentuh satu fungsi.
  - **Lanjutan yang penting untuk cara desktop menulis API (2026-10-31):** `ssl_mode` ditutup **di titik tulis**
    (`normalizeSSLMode` enam mode libpq → 400 sebelum kredensial dienkripsi/disimpan) **dan** di
    `postgresDSN` (kembali `(string, error)`) supaya profil lama tidak diam-diam turun mode — `verify` yang
    maksudnya `verify-full` berarti tanpa pemeriksaan sertifikat. Ternyata ada cacat kedua di belakangnya:
    handler `Create`/`UpsertEnvironment`/`SyncSchema` memetakan **semua** error service ke 500, jadi error
    validasi yang sudah dibuat dengan benar pun tidak pernah sampai ke pemanggil sebagai 400. Pelajaran untuk
    desktop: (1) validasi di titik tulis, (2) gerbang saat membaca/mengeksekusi juga, (3) peta error mapper
    service → status HTTP itu wajib, dan (4) UI tidak cukup jadi bukti — form web sudah membatasi lewat
    `<Select>`, hanya API-nya yang terbuka.
  - Belum ada tempat untuk CA privat (`rootcert`); default `sslmode=disable` salah untuk DB internal
    self-signed. **Sisi Postgres ditutup 2026-11-02** (`root_cert` dirakit sebagai `sslrootcert`; lihat blok CA
    di §3). Masih terbuka: MySQL/Mongo, UI form, dan path relatif CA untuk bundle → FR-1 + butir 8 §10 BRD.
  - **Kontrak pemotongan (M1.4, diverifikasi 2026-10-31):** engine tidak menebak "halaman penuh = terpotong".
    Statement dijalankan dengan `LIMIT cap+1`; kalau baris ke-(cap+1) benar-benar ada, hasilnya ditandai
    `truncated_by=row_limit` dan baris penanda itu dibuang. 12 baris dengan cap 12 → **tidak** ditandai
    (dibuktikan tes), cap 5 dari 12 → ditandai. Karena probe itu bergantung pada LIMIT yang engine tulis
    sendiri, **web tidak boleh lagi mempre-apply `ApplyLimit`**: SQL yang sudah berisi `LIMIT n` membuat
    probe tidak pernah melihat baris ekstra, sehingga semua hasil tampak utuh. Ini kontrak baru antara
    engine dan pemanggil — desktop wajib meneruskan cap lewat `QueryOptions`, bukan lewat teks SQL.
  - **Batas memori (M1.5, diverifikasi 2026-10-31; dikoreksi 2026-11-01):** `SET max_memory` datang dari
    `GuardrailConfig.MaxMemory` (`"2GiB"` bila tidak diisi — ejaan biner sejak 2026-11-01, sebelumnya `2GB`
    desimal) dan `NewEngine` menolak nilai yang tidak berbentuk angka+unit. Catatan lama di sini salah dua hal,
    dan keduanya sudah dibetulkan di engine:
    `"2048"` **bukan** dibaca DuckDB sebagai byte — DuckDB menolaknya sendiri
    (`Parser Error: Unknown unit for memory_limit`); dan engine tadinya **menolak `2GiB`** padahal itu
    ejaan biner yang DuckDB terima. Empat hasil probe langsung (2026-11-01): `2GiB` → tepat 2.147.483.648
    byte, `256MB` → 256.000.000 byte tapi dibalas `244.1 MiB`, `-1` → diterima DuckDB sebagai tanpa batas
    (engine tetap menolak), dan **kalau tidak di-SET DuckDB mengambil 80% RAM** (`6.3 GiB` di host 8 GB).
    Konsekuensinya untuk FR-15 sekarang konkret, dan pertanyaan BRD §15 butir 6 terjawab:
    default `min(4GiB, RAM/4)` (2 GiB di laptop 8 GB), slider berlabel **GiB** yang string-nya dikirim apa
    adanya, mulai dari satu sesi paralel, dan `temp_directory` tetap perlu disentuh (DuckDB spill ke disk
    saat budget kurang — jadi budget kecil tidak memotong hasil, hanya memperlambat).

## 4. Task berikutnya (urutan)

1. Lanjutkan spike **M0**. Yang sudah terbukti: modul desktop mengonsumsi engine lewat
   `replace muara.bi/engine => ../muara.bi-service/backend/engine` (bukan salinan), default memorinya
   sampai ke DuckDB, CGO-off menolak dengan jelas, dan bundle sekarang benar-benar offline.
   **Skeleton Wails v3.0.0 juga sudah berdiri** (binding `Status/AttachSample/Run` menjawab dari DuckDB)
   dan **React/Vite sudah menggantikannya sebagai UI** (`frontend/dist` ter-embed, dibuktikan tes).
   Yang belum (dan semuanya tersandar pada hal yang tidak ada di mesin ini): pemasangan `meminfo.Budget`
   ke `NewQueryService` (butuh UI pengaturan FR-15, M4); bundel extension di **linux & windows** —
   langkahnya sudah tertulis di `.github/workflows/desktop.yml`, tinggal dijalankan; **tiga pertanyaan
   Windows** butuh mesin Windows; **CI per-OS** butuh remote + `ENGINE_REPO_TOKEN` (PAT, karena `GITHUB_TOKEN`
   tidak menyeberang repository dan `replace ../muara.bi-service/backend/engine` menuntut dua repo bersebelahan).
   Sudah terjawab 2026-11-01: **cara membaca RAM total per OS** (`internal/meminfo`, tiga OS enam GOARCH
   ter-cross-compile, `hw.memsize` = 8 GiB di mesin ini → `2GiB`), **bundel extension di macOS**
   (103 MB, ATTACH terbukti dari folder hanya-baca dengan HOME kosong), **minimum macOS = 14 (Sonoma)**
   lewat pin `CGO_LDFLAGS` (temuan 4 di §3 sudah dikoreksi dan §13 engine-map menutup pertanyaannya), dan
   **artefak `.app` macOS** (`build/macos-app.sh` → `/Applications/muara-bi.app`, `LSMinimumSystemVersion`
   tertulis, connector dimuat dari dalam bundle oleh `bundledExtensionDir()`).
   Sudah terjawab 2026-11-02: **"apakah `threads` perlu ikut profil"** — ya, dan tidak lagi hardcoded.
   Engine menerima `GuardrailConfig.Threads` (`NormalizeThreads`: 0 → default 2, negatif ditolak di startup,
   di atas core yang terlihat dijepit), desktop memasang `threadBudget() = NumCPU-1` = **7 di mesin ini**,
   dibaca kembali dari `current_setting('threads')`. Yang **tersisa** dari pertanyaan itu cuma sisi performa:
   apakah `cores-1` lebih baik daripada 2 untuk `ATTACH` lewat jaringan — tetap butuh jalur Postgres yang
   bukan localhost.
   Cek cepat oleh user: `cd frontend && npm run build` (Node milik GoLand, lihat §5), lalu
   `sh build/macos-app.sh <folder-bundle>` dan jalankan binernya dari shell dengan
   `MUARA_SPIKE_SQLITE_FILE` dan/atau `MUARA_SPIKE_PG_DSN` di-set (resep §5) — klik "Attach SQLite fixture"
   (tidak butuh server apa pun) atau "Attach Postgres sample" → "Jalankan query" untuk melihat grid terisi.
   Untuk jalur file (sejak 2026-11-02, tidak butuh connector sama sekali):
   `MUARA_SPIKE_FILE=tests/fixtures/spike.csv go run ./cmd/spike` — mencetak jumlah baris dan hasil `DESCRIBE`.
   Jangan lewat langkah bundel: sejak 2026-11-01 `build/macos-app.sh` **menolak** path bundel yang tidak ada,
   karena sebelumnya ia membuat folder kosong dan kegagalan yang muncul adalah "extension unavailable", bukan
   "kamu lupa mem-bundle".
   Berkas extension hasil spike (`/tmp/muara-ext`, `/tmp/duckdb-bundle`) **sudah dihapus 2026-11-01** sesuai
   permintaan; untuk menjalankan langkah itu lagi, buat bundelnya lebih dulu lewat alat di repo web
   (`cd ../muara.bi-service/backend && CGO_ENABLED=1 go run ./cmd/duckdb-bundle -out <folder>`) lalu arahkan
   `DUCKDB_EXTENSION_DIRECTORY` ke sana.
   Kalau `go build` gagal dengan `pattern all:frontend/dist: no matching files found`, itu artinya langkah
   `npm run build` dilewat, bukan ada yang rusak di Go.
2. **CA privat — sisa setelah Postgres selesai (2026-11-02).** `root_cert` sudah dirakit sebagai `sslrootcert`
   di satu builder DSN (terpakai `SyncSchema` **dan** ATTACH), sudah divalidasi di titik tulis, dan sudah terbukti
   parameternya benar-benar dibaca. Yang tersisa, urut dari yang paling dibutuhkan user:
   (a) **UI** — web hanya menerima `root_cert` lewat API; form koneksi belum punya kolomnya, dan sesuai cara
   kerja yang disepakati: **wireframe/mockup dulu sebelum koding UI**, bukan langsung input. Wireframe sudah
   disajikan 2026-11-02 (satu drawer, empat state driver, pesan validasi yang sama dengan yang dipakai API) dan
   membawa **tiga usulan yang masih menunggu konfirmasi**: input path teks (bukan file picker — berkasnya hidup
   di mesin API, browser tidak bisa memberi path absolut mesin lain), kolom **tampil tapi nonaktif** untuk
   MySQL/SQLite (bukan disembunyikan, supaya operator tahu connectornya memang tidak membaca CA), dan tanpa
   saklar TLS baru (mode `ssl_mode` yang sudah ada yang menentukan nyala/mati);
   (b) **MySQL — selesai 2026-11-02, dan jawabannya "tidak bisa di jalur DuckDB".** Builder
   `%s:%s@tcp(%s:%d)/%s` yang ternyata **ditolak scanner** itu sudah diganti dengan `duckdb.MySQLDSN` di engine
   (grammar lengkap dan tabel hasil probe ada di engine-map §17), dan `ssl_ca` tidak pernah ada tempatnya:
   scanner tidak
   mengenal satu pun kata kunci TLS, jadi `checkRootCertDriver` menolak `root_cert` di profil MySQL dengan 400
   beralasan. Sisa pekerjaan MySQL hanya dua: **sampel server MySQL sungguhan** untuk membuktikan byte yang
   dikirim (yang terukur sekarang baru parsing), dan jalur TLS lewat driver Go kalau nanti sumber MySQL memang
   perlu CA — keduanya bukan blocker M2;
   (c) **MongoDB — selesai 2026-11-02** (`root_cert` → `tlsCAFile`, plus penolakan `sslCAFile` yang diabaikan
   driver; lihat §3 dan engine-map §18);
   (d) **path relatif CA** ikut §5.5, baru relevan ketika `.muara` jadi storage di M3.
3. **Sumber file v1 — sisa setelah engine selesai (2026-11-02).** Yang sudah ada: `FileSourceQuery` +
   `Session.RegisterFileSource` (CSV/TSV/Parquet/JSON lewat reader core, tanpa bundel), ditolak rapi untuk tipe
   lain, probe `-file`, dan tiga fixture ter-commit (`spike.csv`/`spike.json`/`spike.parquet`) yang dibaca
   `TestSpikeFileFixturesReadWithNoBundledConnector` di `go test ./...` — jadi runner Windows/Linux ikut
   membuktikan jalur file tanpa perlu bundel. Yang masih terbuka, urut dari yang paling murah:
   (a) **aturan path relatif di `.muara`** — view hasil `RegisterFileSource` menyimpan path absolut di dalam
     definisinya, sehingga bundle yang dipindah ke mesin lain menunjuk berkas yang tidak ada; ini butir §5.5
     BRD yang belum ditulis sama sekali, dan ia **harus** diputuskan sebelum fitur impor diumumkan;
   (b) **UI impor + registrasi view per sumber file** di produk (bukan spike) — termasuk menampilkan tipe hasil
     inferensi apa adanya;
   (c) **keputusan XLSX-source** dengan tiga opsi yang konsekuensinya sudah tercatat di §3.
   (Fixture parquet **tidak lagi** termasuk daftar ini — tertutup 2026-11-02, lihat §3.)
4. Verifikasi yang masih menunggu: perubahan DSN/validasi ini **baru terbukti di level unit**. HTTP-nya
   (400 sungguhan dari `POST .../environments`) butuh `muara_api` dibangun ulang dari image baru, dan
   stack user sedang berjalan di 18080/18000 — restart adalah aksi milik user, harus izin dulu.
   *Penutup M1 (`make image-core`) sudah selesai 2026-10-31 — lihat §3.*

## 5. Kendala & catatan lingkungan

- Mesin ini: macOS arm64, Docker Desktop VM 4 GB. `go` di `/usr/local/go/bin` adalah 1.25.0, tapi
  `GOTOOLCHAIN=auto` memakai toolchain **1.27.1** yang di-download ke module cache (dua `go.mod`
  meminta `go 1.27.1`) — artinya build di mesin ini butuh cache itu; offline tanpa cache build gagal
  dengan pesan "go.mod requires go >= 1.27.1". DuckDB linux/darwin sudah terverifikasi
  di produk web; **Windows belum pernah disentuh sama sekali** — M0 adalah pertama kalinya.
- **Node ada, tapi di luar PATH** (2026-11-01): milik GoLand di
  `/Users/macbookpro/Library/Application Support/JetBrains/GoLand2025.2/node/versions/24.22.0/bin` →
  `node v24.22.0`, `npm 11.19.1`. Tidak ada nvm/volta/mise/homebrew node, dan `npm -v` gagal dengan
  `env: node: No such file or directory` selama folder itu belum di-`export PATH` (shebang npm-nya
  `#!/usr/bin/env node`). Bukan satu-satunya JS di mesin: Electron-as-Node milik Qoder
  (`ELECTRON_RUN_AS_NODE=1 /Applications/Qoder.app/Contents/MacOS/Qoder` → v24.18.0) ada tapi **tidak**
  membawa npm. Egress sehat (`registry.npmjs.org` 200; `vite` 8.3.5, `react` 19.3.1,
  `@vitejs/plugin-react` 6.1.2, `@wailsio/runtime` 3.0.0, dan vite 8 minta Node `^20.19 || >=22.12` —
  yang tersedia memenuhi). *Catatan koreksi: bullet versi sebelumnya mengklaim "Node/npm tidak ada di
  mesin ini" dan sudah dihapus — salah, dan klaim itu yang menahan M0-c sehari penuh.*
- **Ruang disk masih batasan, tapi bukan lagi alasan M0 tertahan.** Sisa **±1,9 GiB dari 228 GB (99%
  penuh)** sebelum dan sesudah M0-c: seluruhnya (react+vite+plugin 39 MB, runtime 2 paket) muat tanpa
  menghapus apa pun. Yang menahan pekerjaan berat tetap sama, dan tetap **keputusan user, bukan tindakan
  diam-diam**: `go clean -cache` ±3,1 GB, cache modul Go ±4,6 GB, `Docker.raw` ±15 GB,
  `frontend/node_modules` web ±747 MB. Catatan ukur: `go build`/`go test` yang me-link DuckDB membuat
  sisa turun ke **820 MiB** sementara lalu kembali ke 1,9 GiB ketika file temp linker habis — jadi angka
  `df` di tengah build bukan selalu ruang yang hilang permanen: selama build kedua sisa turun ke
  **820 MiB**, lalu kembali ke 1,9 GiB setelah `bin/` (62 MiB) dihapus dan linker temp selesai — jadi angka
  `df` saat build sedang jalan tidak bisa dibaca sebagai "ruang untuk keputusan besar".
- Perintah build yang benar, dan urutannya wajib (dist di-gitignore, `go:embed` membutuhkannya lebih dulu):

  ```sh
  export PATH="/Users/macbookpro/Library/Application Support/JetBrains/GoLand2025.2/node/versions/24.22.0/bin:$PATH"
  cd frontend && npm ci && npm run build && cd ..
  CGO_ENABLED=1 CGO_LDFLAGS="-mmacosx-version-min=14.0" go build -o bin/muara-spike .
  otool -l bin/muara-spike | grep -A4 LC_BUILD_VERSION   # harus minos 14.0
  ```

  Untuk artefak aplikasi (`.app` berisi bundel connector, siap dipasang):

  ```sh
  cd ../muara.bi-service/backend && CGO_ENABLED=1 go run ./cmd/duckdb-bundle -out /tmp/muara-ext  # 103 MB, per-OS
  cd ../../muara.bi-desktop && sh build/macos-app.sh /tmp/muara-ext   # cek minos + plutil dilakukan sendiri
  cp -R build/bin/muara-bi.app /Applications/
  env -u DUCKDB_EXTENSION_DIRECTORY \
    MUARA_SPIKE_SQLITE_FILE="$PWD/tests/fixtures/spike.sqlite" \
    MUARA_SPIKE_PG_DSN="host=localhost port=5435 …" \
    /Applications/muara-bi.app/Contents/MacOS/muara-bi
  ```

  **Ruang disk sekarang memblokir langkah ini, dan itu harus disebut sebelum ada yang menyalahkan skripnya:**
  link DuckDB menaikkan pemakaian puncak ±1,1 GiB (terukur 2026-11-01: sisa turun dari 1,9 GiB ke 820 MiB di
  tengah build), sementara sisa disk mesin ini sudah jatuh ke **±500 MiB (95%)** — jadi `go build` besar
  sengaja tidak dijalankan lagi sampai ruang dibersihkan. Yang tetap bisa dikerjakan tanpa ruang: `go test`,
  `npm run build`, `go build ./cmd/spike` (inkremental), dan seluruh langkah bukti SQLite di atas.

  `env -u` di baris terakhir itu penting sebagai bukti, bukan kebersihan: ia menunjukkan bahwa path connector
  berasal dari dalam `.app`, bukan dari variabel lingkungan yang tertinggal di shell (`lsof -p <pid>` mengonfirmasi
  file yang di-map). Launch dari Finder tidak membawa `MUARA_SPIKE_PG_DSN` sama sekali — itu memang desain:
  kredensial tidak boleh diketik ke webview, dan tempatnya di keychain baru di M4.
  Catatan: skrip ini **bukan** pengganti jalur produksi. `wails3 build`/`package` (butuh `task`, belum dipasang)
  tetap jalur yang benar untuk M6 karena ia ikut mengurus Developer ID, entitlements, notarization dan dmg;
  tanda tangan ad-hoc dari `codesign --sign -` tidak akan tebus Gatekeeper di mesin orang lain.

  Pin `CGO_LDFLAGS` itu bukan hiasan: tanpa ia, `minos` ditulis oleh direktif Wails (11.0) atau oleh SDK mesin
  build, dan keduanya bukan angka yang boleh kita iklankan — lihat temuan 4 di §3 dan
  [docs/engine-map.md](docs/engine-map.md) §13.

  Untuk mengubah jembatan: tambah method **terekspor** di `query_service.go`, lalu
  `.tools/bin/wails3 generate bindings -d frontend/bindings` (butuh `CGO_ENABLED=1`), baru `npm run build`
  lagi — binding yang di-generate adalah sumber kebenaran nama field di JS.
  `bin/` sudah di-gitignore; hapus setelah dipakai kalau ruang menipis.
- Cara menjalankan CLI spike (bukan app): `CGO_ENABLED=1 go build -o /tmp/muara-spike ./cmd/spike` lalu
  `MUARA_SPIKE_PG_DSN="host=localhost port=5435 user=sample_user password=sample_password
  dbname=sample_ecommerce sslmode=disable" /tmp/muara-spike`. DSN **tidak** pernah di-hardcode di
  dalam kode — flag/env saja, supaya kredensial sample tidak ikut ter-commit. Fixture di stack user
  hari ini: `sample_ecommerce` punya satu tabel (`orders`, 6 baris), jadi `tables:1` adalah angka yang
  benar, bukan tanda koneksi gagal.
- BRD §18 menyuruh membaca `muara-bi-desktop-wireframes.canvas.tsx`, tapi file itu tidak ada di
  repo web, di repo ini, maupun di `Downloads`. Sebagai gantinya sudah dibuat wireframe sendiri:
  [docs/wireframe.html](docs/wireframe.html) — 10 layar §12.3, token warna §12.2, toggle dark mode,
  dibuka langsung di browser. Disetujui dan sudah diselaraskan dengan keputusan View lintas project
  (layar S4a: "Pin ke proyek lain", bukan duplikasi).
- Keputusan user 2026-10-30: **opsi A disetujui** — repo web boleh disentuh satu commit untuk
  `git mv` engine + ganti import path (M1.3).
- Repo ini **belum ada remote**; commit lokal berisi dokumen plus kode pertama di repo (`cmd/spike/`,
  `go.mod`, `go.sum`), lalu skeleton Wails (`main.go`, `query_service.go`), lalu UI React/Vite
  (`frontend/` + `embed_assets_test.go`). `assets/` (HTML polos) sempat masuk `fb900dd` dan dihapus lagi
  hari yang sama — satu UI saja, supaya tidak ada dua versi yang bisa saling tertukar saat dibaca.
