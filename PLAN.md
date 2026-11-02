# PLAN — muara.bi Desktop

Dokumen eksekusi. Produk & UX ada di [BRD.md](BRD.md) (v2, 2026-10-30 — angka babnya mengikuti v2);
koreksi teknis terhadap kode lama ada di
[docs/brd-review.md](docs/brd-review.md); batas paket engine hasil pengukuran ada di
[docs/engine-map.md](docs/engine-map.md); dasar pemisahan engine ada di
[docs/adr/0001-engine-module.md](docs/adr/0001-engine-module.md).

## Keputusan yang sudah dikunci (2026-10-30)

1. Engine diekstraksi jadi modul Go sendiri **sebelum** spike platform (membalik urutan roadmap BRD §11).
2. Windows ikut di spike platform — bukan menyusul.
3. Sumber file (CSV/Parquet/JSON) masuk v1; XLSX diputuskan terpisah.
4. Penetapan pemegang engine ditunda; desktop yang memprioritaskan bentuk API-nya.

## Mengapa urutan dibalik dari BRD

BRD menaruh spike Wails (M0) di depan ekstraksi (M1). Dengan aturan `internal/` yang sudah diverifikasi,
spike yang butuh engine hanya bisa dikerjakan dengan menyalin engine ke repo desktop — dan salinan itu
kemudian menjadi fondasi M2-M6. Ekstraksi lebih dulu menghapus godaan itu dan sekaligus memaksa kontrak
"engine tidak mengenal tenant" ditetapkan selagi masih murah.

Spike tetap memakai kriteria gagal/berhasil BRD §11 & §11.1, hanya posisinya setelah modul ada.

---

## M1 · Modul engine (di repo web)

Keluaran: `muara.bi/engine` sebagai modul Go; web tidak lagi punya salinan logika analitik.

- [x] **Batas modul diukur (2026-10-31), bukan ditebak** — lihat [docs/engine-map.md](docs/engine-map.md):
      closure `go list -deps` atas `duckdb`, `exporter`, `ai`, `ssh`, `helper/cryptohelper`,
      `helper/sqlident` hanya berisi keenam paket itu (+`sqlident`). Tidak ada `config`/`domain`/`dto`/
      `repository`/`cache`/`limiter` di dalamnya → ekstraksi tidak menyeret istilah tenant.
- [x] **`git mv` selesai (2026-10-31)** — `backend/internal/{duckdb,exporter,ssh,ai}` dan
      `backend/internal/helper/{cryptohelper,sqlident}` → `backend/engine/<nama>`, lalu dua berkas builder:
      `internal/service/widget_builder.go` → `engine/builder/compile.go` (package `builder`) dan
      `internal/domain/entity/widget_builder.go` → `engine/builder/state.go`. Semua tercatat sebagai
      **rename** di `git status` (tidak pernah ada dua salinan). `helper/dberr` tinggal di web — ia memetakan
      kode error Postgres, urusan milik orchestrator. Paket `domain/entity` yang multi-tenant tidak ikut pindah.
- [x] **Web mengganti import path (2026-10-31)** — 6 titik (`app/server`, `cache`, `dto/response`, `service`,
      `worker`, `exporter`) ditulis ulang dengan `find -print0 | xargs -0 perl -pi -e`; grep memastikan nol sisa
      `muara.bi/core/internal/{duckdb,exporter,ssh,ai,cryptohelper,sqlident}`. Modul baru `backend/engine/go.mod`
      (`module muara.bi/engine`, go 1.27.1) + `go.sum`; `backend/go.mod` mendapat
      `require muara.bi/engine v0.0.0` + `replace muara.bi/engine => ./engine`.
      **Gerbang hijau:** `go build/vet/test ./...` di KEDUA modul, `CGO_ENABLED=1` dan `CGO_ENABLED=0`
      (delapan kombinasi, semuanya lolos; `gofmt -l` bersih).
- [x] **Vocabulary tenant dikeluarkan dari API engine (2026-10-31)** — `duckdb.SemanticModel` tidak lagi
      membawa `WorkspaceID`/`ProjectID` (engine buta tenant; kepemilikan tetap di `entity.SemanticModel` web);
      satu-satunya sisa `workspace_id` di engine sudah dihapus, grep kini bersih.
      **`session_preparer.go` memang tidak dipindah** — bagian Postgres-nya (`:45-49`, `:193-226`) tetap di web
      dan meneruskan alias/driver/DSN ke engine sebagai struct plain. Kunci `cryptohelper` tetap diterima
      sebagai `[]byte`. Tidak ada satu pun berkas `engine/` yang mengimpor `muara.bi/core/...`
      (diverifikasi grep) → tidak ada siklus.
- [x] **CGO fail-fast (2026-10-31)**: `NewEngine` jalur `!cgo` mengembalikan `duckdb.ErrCGODisabled`;
      seluruh metode stub mengembalikan error yang sama dan tidak mungkin lagi menghasilkan `QueryResult`
      (sebelumnya `session_fallback.go:42,49,56,83` menjawab `nil`/baris karangan). Ditest dua arah:
      `cgo_disabled_test.go` (refusal) + `cgo_enabled_test.go` (`SELECT 1` nyata, guard agar build tag
      tidak kebalik). `cmd/duckdb-bundle` dapat `//go:build cgo` karena `go-duckdb` v1.8.5 memang tidak
      bisa di-compile `CGO_ENABLED=0` — sebelumnya itu yang membuat jalur `!cgo` mustahil diuji di CI.
- [x] **Alat build ikut disesuaikan (2026-10-31)** — `Dockerfile` stage `build` men-COPY
      `backend/engine/go.mod` + `go.sum` ke `./engine/` **sebelum** `go mod download` (modul yang di-`replace`
      ke direktori lokal harus sudah ada saat resolve), dan `Makefile` menjalankan `test`/`vet`/`tidy` di
      **dua** modul dengan engine lebih dulu — kalau tidak, `./...` di `backend/` diam-diam berhenti
      menyentuh logika DuckDB. **Dibuktikan `make image-core` 2026-10-31:** image `muara-bi-core:dev` 369 MB
      build hijau, dan `go version -m` atas `/usr/local/bin/api` keluar dari container mencantumkan
      `muara.bi/engine v0.0.0` tanpa hash — artinya engine di-resolve dari `replace => ./engine`, bukan disalin.
      Ini bukti yang desktop butuhkan: satu salinan engine, dua produk.
- [x] **`max_memory` DuckDB jadi konfigurasi (2026-10-31, M1.5)** — `GuardrailConfig.MaxMemory string`
      dengan `duckdb.DefaultMaxMemory = "2GB"` saat M1.5 ditutup (sama dengan angka yang tadinya di-hardcode di
      `session_cgo.go`); `NewEngine` memvalidasi lewat `NormalizeMaxMemory` sehingga **konfigurasi salah
      gagal di startup**, bukan diam-diam memakai budget lama. Nilai masuk ke `SET max_memory=` lewat
      `quoteLiteral`. Web membacanya dari `config.DuckDBConfig.MaxMemory` (`DUCKDB_MAX_MEMORY`, default
      `2GiB` sejak 2026-11-01, sebelumnya `2GB`) → `internal/app/server/service.go:59`; **desktop mengambil
      field yang sama dari profilnya** (FR-15), tidak ada jalur kedua.
      Profil desktop kini **sudah diputuskan** (BRD §15 butir 6, 2026-11-01): default `min(4GiB, RAM/4)` — 2 GiB di
      laptop 8 GB, 4 GiB di 16 GB ke atas, dan selalu satu sesi lebih dulu.
      Empat hal yang perlu diingat saat menulis settings desktop — semuanya diprobe langsung ke DuckDB pada
      2026-11-01, dan dua di antaranya **membatalkan catatan sesi sebelumnya**: (1) DuckDB menerima unit
      **biner** juga (`2GiB` = tepat 2.147.483.648 byte) di samping desimal (`256MB` = 256 × 10⁶ byte,
      yang dibalasnya `244.1 MiB`), jadi FR-15 memakai label GiB dan meneruskan string apa adanya — engine
      sudah menerimanya, padahal sebelum 2026-11-01 justru `2GiB` yang ditolak; (2) angka polos seperti
      `"2048"` ditolak **oleh DuckDB sendiri** (`Unknown unit for memory_limit`), bukan dibaca sebagai byte
      seperti yang dicatat semula; (3) kalau tidak di-SET, DuckDB mengambil **80% RAM** (terukur 6.3 GiB di
      host 8 GB), jadi nilai harus selalu eksplisit; (4) plafon memori = **jumlah sesi × budget per sesi**,
      jadi settings menampilkan keduanya bersama — dan ini plafon, bukan alokasi: setelah melewatinya DuckDB
      spill ke `temp_directory`, sehingga budget yang lebih besar dari RAM melambatkan query, tidak langsung
      mematikan aplikasi.
      `threads` sempat sengaja dibiarkan hardcoded 2 di titik ini dan ditandai untuk M0 — **ditutup
      2026-11-02** lewat `GuardrailConfig.Threads` (lihat butir `threads` di bawah).
      Ditest: `TestNormalizeMaxMemory` (CGO-free), `TestEngineAppliesConfiguredMemoryBudget` dan
      `TestEngineRejectsUnusableMemoryBudget` (CGO, bandingkan **byte** lewat `current_setting`).
- [x] **Kontrak hasil `truncated` (2026-10-31, M1.4)** — `duckdb.QueryResult` menambah `row_limit`,
      `truncated`, `truncated_by` (`row_limit` · `source_cap`) dan `capped_sources`. Cara kerjanya:
      `QueryOptions{MaxRows}` dipindah ke `Session.Query`, jadi **engine** yang menegakkan cap
      (`web` tidak lagi memanggil `ApplyLimit` sebelum eksekusi — kalau masih, probe-nya mati karena
      SQL sudah punya `LIMIT n`), statement dijalankan dengan cap+1 sebagai baris penanda, lalu
      dibuang; baris penanda itulah bukti pemotongan, bukan dugaan "halaman penuh". Cap sumber ikut
      dilaporkan: `StreamMongoCollections` kini mengembalikan `[]MongoMount` (jumlah dokumen +
      `Truncated`) dan `CappedMongoViews()` meringkasnya. Ditest 7 kasus di `truncation_test.go`
      (termasuk "pas penuh di bawah cap ≠ terpotong" dan prioritas `row_limit` di atas `source_cap`)
      + `TestPlanLimit` + `TestCappedMongoViews`. Web memancarkan field ke `ExecuteQueryResponse`
      (query biasa & share publik); **penanda di UI masuk M2** karena UI desktop belum ada.

**Exit:** dua-duanya build & tes hijau dari satu salinan engine; paket `internal/` web tidak lagi
berisi logika DuckDB; satu tes membuktikan build tanpa CGO gagal cepat.
*(2026-10-31: **M1 ditutup.** `go build/vet/test` hijau dua modul × dua mode CGO; `internal/` sudah bersih dari
DuckDB/exporter/ssh/ai/builder; kontrak §7.3 & §7.4 ditegakkan di engine; `max_memory` jadi konfigurasi; dan
`make image-core` membuktikan image web dibangun dari engine yang sama. Naik ke M0.)*

## M0 · Spike platform desktop (setelah M1)

Keluaran: jawaban ya/tidak untuk "Wails v3 × CGO × DuckDB asli × offline di tiga OS", dengan CI.

*Sudah tercapai 2026-11-01 (spike `cmd/spike/`, belum app): modul desktop memakai engine lewat `replace`,
default `2GiB` sampai ke DuckDB, dan CGO-off menolak start. Bundel extension sekarang ditegakkan engine —
lihat [docs/engine-map.md](docs/engine-map.md) §8. Butir `- [x]` di bawah hanya berarti "terbukti di spike".*

- [x] Skeleton Wails v3 terpasang, **versi dikunci `v3.0.0`** (bukan `v3.1.0-nightly.*` yang juga ada di
      proxy), `LICENSE` dibaca dari modul hasil `go get` di tag itu: **MIT (c) 2018-Present Lea Anthony**.
      CLI `wails3` ikut dikunci di `.tools/bin` (gitignored) supaya versinya mengikat pada repo.
- [x] **React/Vite** di atas skeleton itu (2026-11-01): `frontend/` = vite 8.3.5 + react 19.3.1, dikunci di
      `package.json`; binding di-generate ke `frontend/bindings`, `main.go` meng-embed `frontend/dist`, dan
      HTML polos `assets/` dihapus. Node **tidak** perlu dipasang — milik GoLand, di luar PATH
      (`…/GoLand2025.2/node/versions/24.22.0/bin`, lihat PROGRESS §5). Estimasi "±200 MB" yang menahan butir
      ini ternyata **39 MB**; `@wailsio/runtime 3.0.0` (dikunci = versi module Go, di-bundle ke dalam
      `index-*.js` 247 kB sehingga halaman tidak bergantung `/wails/runtime.js`) menambah 2 paket. Buktinya
      tes, bukan run manual: `TestEmbeddedViteBuildIsServed`. Urutan build untuk CI: **`npm run build`
      sebelum `go build`** (dist di-gitignore → `go:embed` gagal tanpa itu).
- [ ] Satu binding memanggil engine dan `SELECT` nyata dari Postgres tampil di grid — **di Windows**.
      Sisi macOS sudah tertutup 2026-11-01 lewat jalur yang tidak butuh server: `AttachSampleSQLite` +
      `TestSpikeSQLiteFixtureReachesTheGrid` menuntut 3 baris agregat nyata dari fixture lewat jembatan yang
      sama dengan yang dipakai tombol "Jalankan query" di jendela (`columns` 3, `truncated=false`), dan
      `cmd/spike -sqlite-file` membuktinya dari biner yang sama.
- [ ] Empat pertanyaan Windows yang harus dijawab, masing-masing dengan bukti, bukan asumsi (tiga dari
      checklist awal; WebView2 ditambahkan 2026-11-01 karena BRD tidak pernah menyebutnya):
  - [ ] go-duckdb v1.8.5 menyediakan `libduckdb.a` untuk windows/amd64? ABI-nya cocok dengan mingw-w64?
        **Sudah terjawab separuh dari isi modulnya (dibaca 2026-11-01 di cache mesin ini):**
        `deps/windows_amd64/libduckdb.a` **ada** (75 MB) dan `cgo_static.go:4` memasangkan build tag
        `(windows && amd64)` dengan `#cgo windows,amd64 LDFLAGS: -lws2_32 -lwsock32 -lrstrtmgr -lstdc++ -lm
        --static -L${SRCDIR}/deps/windows_amd64`. Bentuk `--static` + `-lstdc++` itu **hanya dimengerti
        GCC/MinGW**, jadi jawaban alatnya: **mingw-w64 wajib**, MSVC `link.exe` tidak akan bisa. Yang tersisa
        hanyalah link nyata di Windows — arsip `.a` belum tentu kompatibel dengan build MinGW yang terpasang.
  - [ ] biner yang di-link statis bisa `LOAD` extension di Windows (Unix pakai `-extldflags=-Wl,-E`
        untuk `dlopen`; Windows tidak punya `RTLD_GLOBAL`)? **Instrumentnya sudah ada dan sudah terbukti di
        macOS (2026-11-01):** `cmd/spike -sqlite-file` + fixture `tests/fixtures/spike.sqlite` (8 KB,
        ter-commit) menjawab pertanyaan ini **tanpa server, tanpa kredensial, tanpa jaringan** — dijalankan
        dengan bundel di-`chmod a-w` dan `HOME` folder kosong, keluar `tables:1` dan `rows_in_orders:6`.
        Bentuk yang sama sudah ditulis sebagai langkah CI, jadi sisa Windows-nya tinggal menjalankan.
  - [x] `postgres_scanner`/`mysql_scanner`/`sqlite_scanner` tersedia sebagai berkas `windows_amd64`?
        **Terjawab positif 2026-11-01 tanpa menunggu mesin Windows:** HEAD ke
        `https://extensions.duckdb.org/v1.1.3/<platform>/<scanner>.duckdb_extension.gz` mengeluarkan 200 —
        `windows_amd64/postgres_scanner` 22,95 MB, `windows_amd64/mysql_scanner` 10,80 MB, dan
        `linux_amd64` juga ada (postgres 16,37 MB, mysql 14,29 MB). Jadi bundel linux bisa dibuat di CI tanpa
        menunggu apa pun, dan pertanyaan Windows yang tersisa bukan lagi ketersediaan berkas tapi `LOAD`-nya.
        Catatan cara ukur, supaya tidak diulang: URL **tanpa** `.gz` adalah 404 untuk *semua* platform —
        termasuk `osx_arm64` yang sudah terbukti jalan — jadi HEAD tanpa `.gz` bukan bukti berkasnya tidak ada.
  - [ ] **WebView2** — pertanyaan keempat yang belum pernah ditulis di BRD/PLAN: Wails di Windows tidak
        merender sendiri, ia memakai WebView2 Runtime. Di Windows 11 umumnya sudah ada, di Windows 10 belum
        tentu. Yang harus dipastikan di mesin Windows: versi runtime yang terpasang, apakah aplikasi jalan tanpanya,
        dan apa yang dilakukan installer kalau runtime hilang (unduh Evergreen atau tolak install).
        Ini ikut menentukan **lantai Windows produk** (10 vs 11) — pertanyaan yang sama seperti lantai macOS
        yang baru ditutup 2026-11-01 ([docs/engine-map.md](docs/engine-map.md) §13).
- [ ] Extension dibundle dan `extension_directory` diarahkan ke folder aplikasi — **sisi engine sudah
      menegakkannya** (2026-11-01: `INSTALL` dilompati + autoinstall `false`, dibuktikan gagal cepat dan
      folder tetap kosong). **darwin/arm64 sudah terbukti positif** (2026-11-01): 103 MB berkas
      `v1.1.3/osx_arm64/*_scanner.duckdb_extension` hasil `cmd/duckdb-bundle`, ATTACH Postgres jalan dengan
      folder di-`chmod a-w` **dan** `HOME` dipindah ke folder kosong yang tetap 0 byte — jadi tidak ada tempat
      untuk mengunduh dan tidak ada kebutuhan mengunduh. **2026-11-01: arahannya juga sudah jadi kode**, bukan
      lagi env developer — `bundledExtensionDir()` (`query_service.go`) mencari
      `Contents/Resources/duckdb/extensions` relatif ke biner, jadi `.app` yang dijalankan dari Finder membawa
      bundelnya sendiri tanpa satu pun variabel lingkungan, dan `DUCKDB_EXTENSION_DIRECTORY` tetap menang untuk
      build scratch; keduanya tertutup tes (`TestExtensionDirIsFoundFromAppBundleLayout`,
      `TestExtensionDirEnvOverridesTheBundle`). Bukti runtime: `lsof` atas proses aplikasi yang terpasang
      memetakan `.../Resources/duckdb/extensions/v1.1.3/osx_arm64/postgres_scanner.duckdb_extension`.
      **Kurang: linux & windows**, dan keduanya lebih
      mudah dibuktikan di CI daripada di mesin user (`--network none` untuk linux, runner windows untuk
      windows). macOS memang yang paling sulit, jadi jangan pakai kemacetan di sini sebagai alasan menunda
      dua OS lainnya.
- [ ] CI per-OS (macOS/Linux/Windows). Bundel extension **tidak** di-commit dari satu mesin:
      `cmd/duckdb-bundle` (di repo web, `backend/cmd/duckdb-bundle`) dijalankan per-OS karena folder hasilnya
      per-platform. Untuk langkah macOS, build wajib menyetel `CGO_LDFLAGS="-mmacosx-version-min=14.0"` dan
      mengassert `otool -l` mengeluarkan `minos 14.0` — tanpa pin, angka itu ikut SDK runner dan biner dari
      runner macOS 15 tidak akan jalan di Sonoma. Jangan pakai jumlah warning `built for newer` sebagai gerbang:
      `go.o` bikinan toolchain ikut menghasilkan warning meskipun tidak ada yang salah
      ([docs/engine-map.md](docs/engine-map.md) §13).
      **Status 2026-11-01: file-nya sudah ditulis (`.github/workflows/desktop.yml`), runner-nya belum pernah
      menjalankannya satu kali pun.** Repo desktop belum punya remote, dan job-nya butuh dua hal yang belum ada:
      checkout **repo engine sebagai folder sepupu** (karena `replace … => ../muara.bi-service/backend/engine`,
      jadi satu repo saja tidak cukup) dengan `ENGINE_REPO_TOKEN` — token PAT, karena `GITHUB_TOKEN` tidak bisa
      menyeberang repository. Yang sudah direhearsal di mesin ini: langkah macOS (pin + assert `otool` +
      `plutil` lewat `build/macos-app.sh`), langkah bundel + bukti LOAD offline (folder hanya-baca, `HOME` kosong),
      gerbang cross-build `internal/meminfo` (enam GOOS/GOARCH ok), dan `go test ./...` hijau. Yang **tidak** bisa
      dibuktikan dari mesin ini: tiga langkah Windows (mingw, `LOAD`, WebView2) — persis pertanyaan yang jadi
      alasan file itu ada.
- [x] Bukti negatif CGO — **bentuknya berubah dari asumsi checklist** (2026-11-01): di macOS
      `CGO_ENABLED=0 go build ./...` **gagal kompilasi** di `pkg/application` (`undefined: macosApp`,
      `newPlatformApp`), jadi aplikasi desktop tidak bisa menghasilkan artefak CGO-off yang "menolak start".
      Yang terbukti: (a) build CGO-off gagal keras — itu gerbangnya, dan (b) pada CLI spike (`cmd/spike`)
      yang tidak memakai Wails, biner CGO-off jalan dan menolak dengan `duckdb engine unavailable: binary
      built without CGO…` (exit 1). Konsekuensinya untuk CI: **jangan** pakai `CGO_ENABLED=0` sebagai
      matriks build desktop; cukup pastikan build CGO=1 hijau dan `go test ./...` (engine) juga menguji
      pesan `ErrCGODisabled` untuk web.
- [x] **Cara membaca RAM total di Windows/macOS/Linux (2026-11-01):** `internal/meminfo` —
      darwin `unix.SysctlUint64("hw.memsize")`, linux `MemTotal` dari `/proc/meminfo` (×1024),
      windows `kernel32!GlobalMemoryStatusEx`. **Tanpa gopsutil, sesuai larangan butir ini**: satu-satunya
      modul yang dipakai `golang.org/x/sys v0.49.0`, yang **sudah ada di graph** sebagai indirect bawaan
      Wails — `go mod tidy` hanya memindahkannya ke require langsung. Kejutan Windows: x/sys **tidak**
      mengekspor `GlobalMemoryStatusEx`, dan `wmic`/WMI bukan alternatif yang layak (deprecated, dibuang di
      build 11 terbaru, service bisa mati saat installer justru perlu RAM) — jadi proc-nya dipanggil lewat
      `syscall.NewLazyDLL` dengan struct `MEMORYSTATUSEX` dideklasi sendiri. Paket ini **tidak butuh CGO**:
      `CGO_ENABLED=0 go build` hijau untuk darwin/linux/windows × amd64/arm64 (enam kombinasi) dan `go vet`
      per-GOOS bersih, sehingga "tersusun di ketiga OS" bisa dijaga CI dari satu mesin.
      `Budget()` = `min(4GiB, RAM/4)`, **dibulatkan ke bawah** ke kelipatan MiB (plafon bukan alokasi —
      kurang sedikit hanya berarti spill lebih awal, tidak memotong hasil), dijepit 256 MiB–4 GiB supaya
      pembacaan aneh tidak pernah menghasilkan budget bernilai nol, dan ditulis dalam GiB kalau utuh
      supaya string-nya lolos `NormalizeMaxMemory` dan satuannya sama dengan label slider FR-15.
      Terukur di mesin ini: 8589934592 byte → `2GiB` (identik dengan default engine, dan itu kebetulan yang
      menguntungkan, bukan asumsi). `TestRAMBudgetReachesDuckDB` membuktikan string-nya sampai sebagai byte
      yang benar — dengan ukuran RAM **sintetis**, karena gema satu-desimal DuckDB tidak bisa dibalik persis
      pada RAM ganjil. Yang masih terbuka: angka ini **belum dipasang** ke `NewQueryService` (makanannya
      pengaturan FR-15 di M4).
- [x] **`threads` ikut profil mesin (2026-11-02):** pertanyaan lama "masih hardcoded 2 di `NewEngine`" ditutup
      dengan dua langkah, karena angkanya hidup di engine. Engine: `GuardrailConfig.Threads` +
      `NormalizeThreads` — `0` berarti "belum dikonfigurasi" dan jadi `DefaultThreads = 2` (DuckDB menolak `0`
      dan **mempertahankan nilai lama** saat `SET` gagal, jadi angka itu tidak boleh diteruskan), negatif
      ditolak di `NewEngine`, dan nilai di atas core yang terlihat proses **dijepit** (DuckDB menerima `999`
      tanpa cap internal). Web: knob `DUCKDB_THREADS` default `2` karena CPU satu pod =
      `threads × DUCKDB_MAX_CONCURRENCY`. Desktop: `threadBudget() = NumCPU-1` → **7 di mesin ini**, dipasang
      di `NewQueryService`; satu core ditinggal untuk jendela aplikasi. Ini **kebijakan produk, bukan ambang
      hasil ukur** — yang terbukti adalah angkanya sampai ke DuckDB (`current_setting('threads')` dibaca ulang
      di `TestSpikeStatusReportsEngineDefaults`) dan selalu di dalam mesin. Pengukuran performanya (apakah
      `cores-1` lebih baik daripada 2 lewat jaringan) tetap terbuka di butir bawah.
      Detail: [docs/engine-map.md](docs/engine-map.md) §19.
- [x] **Deployment target macOS ditetapkan: minimum macOS 14 (Sonoma), dipin lewat `CGO_LDFLAGS`**
      (2026-11-01, bukti di [docs/engine-map.md](docs/engine-map.md) §13). Link biner app mengeluarkan 50
      warning `object file ... was built for newer 'macOS' version (14.0) than being linked (11.0)` dan
      `otool -l` menulis `minos 11.0` — sumbernya **Wails**, bukan linker Go:
      `pkg/application/application_darwin.go:8` (v3.0.0) membawa `#cgo LDFLAGS: … -mmacosx-version-min=10.13`
      dan ld64 menjepitnya ke 11.0 karena itu lantai arm64. **Catatan koreksi:** klaim "`MACOSX_DEPLOYMENT_TARGET`
      tidak berpengaruh" yang ditulis lebih awal hari ini adalah false negative — nilainya (`14.0`) sama dengan
      default SDK mesin, jadi tidak ada yang berubah. Env var itu memang hanya *default* dan kalah oleh flag
      eksplisit; diuji lagi dengan `11.0` pada build non-Wails, `minos` turun dan warning muncul 43 kali.
      Yang terbukti jalan: `CGO_LDFLAGS="-mmacosx-version-min=14.0"` mengalahkan direktif Wails — dilihat dari
      hasil link-nya (`minos 14.0`, **nol warning**), dan direproduksi tanpa Wails lewat satu paket berisi
      `#cgo LDFLAGS: -mmacosx-version-min=10.13` yang berubah dari `minos 11.0` menjadi `14.0` begitu flag
      environment dipasang. Angka 14 dipilih karena ia target terendah yang link bersih (13.0 → 49 warning,
      11.0 → 50); Wails mendukung 12.0 tapi itu di bawah lantai kode engine, dan tidak ada mesin 12/13 di sini
      untuk membuktikannya. Konsekuensi yang harus ikut ke CI:
      build tanpa pin **tidak bisa direproduksi** (`minos` mengikuti SDK runner, jadi runner macOS 15 akan
      menerbitkan biner yang menolak jalan di Sonoma) — langkah macos wajib menyetel pin dan men-assert
      `otool -l` terhadap angka yang diiklankan.
- [x] **Artefak aplikasi macOS berdiri dan terpasang di mesin ini (2026-11-01).** `wails3 build` tetap berhenti
      di `ERROR task: No Taskfile found at "/Users/Go/muara.bi-desktop"` — skeleton M0 tidak pernah punya
      `build/darwin/Taskfile.yml` dan `task` tidak dipasang (disk 95% penuh, dan menambah toolchain untuk satu
      spike bukan tukar yang adil), jadi `.app` dirakit oleh skrip deterministik sendiri:
      `sh build/macos-app.sh <folder-bundle>` → `build/bin/muara-bi.app` **149 MB**, `minos=14.0`, tiga
      `*_scanner.duckdb_extension` ikut di dalam, signature-nya ad-hoc. Yang dulu disebut "harus ikut dibuka di
      sana" keduanya selesai: (a) `Info.plist` sekarang menulis **`LSMinimumSystemVersion = 14.0`** — kunci yang
      sebenarnya ditegakkan Finder/Gatekeeper, karena `minos` di Mach-O tidak dibaca oleh siapa pun, jadi lantai
      macOS 14 kini berlaku dan bukan cuma diumumkan; (b) pin 12.0 milik Taskfile Wails tidak jadi soal karena
      Taskfile-nya tidak dipakai — yang dipakai `CGO_LDFLAGS="-mmacosx-version-min=14.0"` di dalam skrip (§13),
      dan skripnya **membatalkan build** kalau `otool -l` tidak membalas 14.0, jadi assertion yang tadinya
      dijadwalkan untuk CI sudah ada di recipes lokal. `plutil -lint` ikut dijalankan atas plist-nya.
      Terpasang di `/Applications/muara-bi.app` dan berjalan: UI dimuat dari biner ter-embed
      (`/assets/index-*.css`, `/assets/index-*.js`) dan `Status()` menjawab dari DuckDB sungguhan. Sisa M6
      tidak berubah: Developer ID + notarization + dmg (adhoc tidak tebus Gatekeeper untuk distribusi), dan
      `AttachSample` ke sample Postgres lokal masih gagal di **pihak server**
      (`FATAL: could not open file "global/pg_filenode.map": I/O error`, stack user + disk 94%) — aplikasi
      sudah memetakan connector-nya sebelum kegagalan itu, jadi bukan bug desktop.
- [ ] Diukur lewat jaringan nyata, bukan lewat localhost: **berapa koneksi yang dibuka `postgres_scanner` per
      query paralel** (`threads` sekarang dari profil: `threadBudget()` → `GuardrailConfig.Threads`),
      latensi round-trip, dan berapa lama forwarder bastion bertahan hidup saat VPN putus-nyambung. Angkanya
      menentukan batas `max_connections` yang layak dan **apakah `NumCPU-1` jumlah yang benar** — angka profil
      itu sekarang kebijakan, bukan hasil ukur, dan pengukuran inilah yang mengubahnya jadi fakta.

**Exit:** installer dev berjalan di Windows dan macOS tanpa peringatan Gatekeeper/SmartScreen untuk
skenario uji, query federated nyata jalan offline, dan CI per-OS hijau. Kalau salah satu pertanyaan
Windows gagal, ini titik putar ke Tauri + sidecar Go atau macOS-only — diputuskan di sini, bukan di M4.

## M2 · Bersihkan blocker data (di engine, dipakai dua produk)

- [x] **Budget nol (`0GiB`) lolos validasi engine — ditutup 2026-11-01.** `maxMemoryPattern` hanya
      melihat bentuk (`^\d+(\.\d+)?(unit)$`), jadi nol lolos validasi startup — padahal kontrak M1.5 justru
      "config salah gagal di startup". Perbaikannya di engine supaya dua produk ikut aman: angka di depan
      unit kini di-parse dan `0`/`0.0` ditolak dengan `ErrInvalidMemoryBudget`, plus dua gerbang tes
      (`TestNormalizeMaxMemory` tujuh bentuk nol, `TestEngineRejectsUnusableMemoryBudget` kasus `NewEngine`).
      Perilaku DuckDB untuk `0GiB` sengaja **tidak** di-probe: engine tidak lagi mengirim nol ke DuckDB, jadi
      tidak ada yang perlu dipastikan di sisinya. Desktop tidak pernah mengirimnya (`meminfo.Budget` dijepit
      ≥256 MiB) — yang terbuka adalah web, lewat `DUCKDB_MAX_MEMORY`, dan itu sekarang tertutup. Detail di
      web `PROGRESS.md` (blok "Budget `max_memory` bernilai nol").



- [x] `SyncSchema` lintas driver — selesai 2026-11-02 (web `connection_service.go`, sebelumnya hanya Postgres
      dan **diam**: sync MySQL/SQLite/MongoDB membalas sukses tanpa pernah menulis `cached_schema`, jadi tree
      kosong tanpa satu pun pesan). Yang dibuka lebih dulu adalah pengukuran, bukan peraturan:
      `litesrc.information_schema.columns` membalas `Catalog Error: Table with name columns does not exist!`
      untuk `sqlite_scanner`, sedangkan `duckdb_columns()` melihat catalog hasil ATTACH seperti biasa — maka
      fungsi katalog DuckDB-lah jalurnya, bukan information_schema sumber
      (`Session.DescribeSource` + `duckdb/catalog.go`, detail di `docs/engine-map.md` §20). Web: Postgres
      sengaja **tetap di pgx** karena sample Postgres mati di sisinya (`pg_filenode.map: I/O error`) sehingga
      pemindahan jalur tidak bisa diukur, MySQL + SQLite lewat engine (build web tidak punya Go driver untuk
      keduanya), MongoDB dan driver tak dikenal **menolak dengan alasan** — kolom collection hanya bisa diinferensi
      dengan membaca dokumen, dan aturan metadata-only melarang memindahkan data ke cache. Dua bug ikut tertutup di
      jalur error: `AppError.Error()` menempelkan cause mentah (bocor `passwd=…` di log) dan discovery nol tabel
      pernah menimpa cache dengan `{}`. Bentuk `cached_schema` tidak diubah: empat pembacanya masih parse
      `{tabel: [{name,type}]}`.
- [ ] Skema terkualifikasi penuh: `alias.schema.tabel`, PK/FK, nullability, komentar tabel/kolom,
      jumlah baris — mengganti peta `{tabel: [{name,type}]}` (`cachedColumn` di web, dulu
      `map[string][]ColumnDef`). **Sisi engine selesai 2026-11-02** dan isinya diukur, bukan diasumsikan:
      `DescribeSource` sekarang membaca tiga fungsi katalog (`duckdb_columns()` untuk `is_nullable` +
      `column_default` + `comment`, `duckdb_constraints()` untuk PK & FK lengkap dengan kolom rujukannya,
      `duckdb_tables()` untuk `comment` + `estimated_size`), dibuktikan pada katalog `temp` DuckDB — dua tabel
      temp berkunci komposit + satu FK, tanpa server apa pun — dan pada fixture SQLite. Tiga angka yang
      mengubah bentuk API: LIST tiba sebagai `[]interface{}`, **`estimated_size` fixture = 5 padahal
      `count(*)` = 6** (maka `EstimatedRows`, dan `-1` = tidak dilaporkan, bukan 0), dan `is_nullable` adalah
      deklarasi (`id INTEGER PRIMARY KEY` tetap nullable). View tidak punya baris `duckdb_tables()`, jadi
      ukurannya `UnknownRowCount`. Filter schema sistem ikut pindah ke WHERE karena baris bookkeeping memakan
      budget baris (`MaxCatalogColumns`), sementara aturannya yang teruji tetap di Go.
      **Yang tersisa** cuma satu hal dan ia butuh server: bentuk `cached_schema` + empat pembacanya
      (`ai_service.go:358`, `stores/schema-store.ts`, `hooks/use-schema-tree.ts`, `lib/monaco-completion.ts`)
      harus berpindah sekaligus, dan jalur Postgres (pgx) belum bisa diukur — sample-nya masih mati di sisinya,
      dicoba ulang 2026-11-02 lewat `AttachPostgres`. Mengubah JSON sekarang = menebak apa yang pgx kembalikan
      untuk `is_nullable`/`pg_description`. Detail pengukuran di `docs/engine-map.md` §21.
      Ini prasyarat FR-2, FR-5, FR-10.
- [x] Penanda pemotongan **sampai ke UI** — selesai 2026-11-02 untuk FR-3. Engine sudah memutuskannya sejak
      M1.4 (`MongoMount.Truncated` → `QueryResult.TruncatedBy = source_cap`), dan **web hanya mengirim**
      keempat field itu tanpa pernah membacanya (grep `truncated` di `frontend/` = nol): guardrail membuang
      baris, layar tetap menulis "10.000 baris". Sekarang satu komponen (`components/query/truncation-note.tsx`)
      merender `truncated`/`truncated_by`/`capped_sources`/`row_limit` di lima permukaan — callout amber di Data
      Studio, drawer widget, editor semantic model; sufiks + ikon ⚠ di judul kartu widget canvas dan halaman
      share/embed (permukaan publik sebelumnya tidak punya baris metadata sama sekali). Grid spike desktop ikut
      melengkapinya: `capped_sources` tampil dan alasannya tidak lagi dieja mentah, dibandingkan lewat
      `TruncateReason` dari binding. Amber, bukan merah, karena query-nya **berhasil** — yang sebagian hanya
      cakupannya. **Pre-flight bundle (FR-11) sengaja dikeluarkan dari butir ini** atas keputusan user dan tetap
      jadi pekerjaan M4. Bukti: `internal/dto/response/query_test.go` (nama key di wire),
      `engine/duckdb/result_wire_test.go` (flag lolos JSON round-trip, CGO=1 & CGO=0), DOM hasil render route
      sementara (penanda muncul 3×+2×, ikon judul 2×, hasil utuh = nol elemen), `tsc --noEmit` + `next build`
      bersih, `vite build` desktop bersih, gerbang empat-mode `GATE-DONE` dan `DESKTOP-DONE fail=0`. Round trip
      Redis sungguhnya menunggu infra (lihat PROGRESS §3 web).
- [ ] Sumber file v1: CSV + Parquet + JSON lewat `read_csv_auto`/`read_parquet`/`read_json_auto`
      (core DuckDB, tanpa extension per-OS). **Sisi engine selesai 2026-11-02**: `duckdb.FileSourceQuery` +
      `Session.RegisterFileSource` memetakan ekstensi ke reader core (`.csv`/`.tsv`/`.parquet`/`.pq`/`.json`/
      `.jsonl`) dan menolak sisanya dengan `ErrUnsupportedFileSource` — dibuktikan offline dengan bundel
      kosong-hanya-baca, dan `read_parquet` terbukti ada di biner karena berkas hilang membalas `IO Error`,
      bukan "extension tidak tersedia". Ditutup hari yang sama: tiga fixture ter-commit (csv/json/parquet) dibaca
      `TestSpikeFileFixturesReadWithNoBundledConnector` dengan bundel kosong. **Aturan path relatif §5.5 ikut
      tertutup 2026-11-02** — diukur dulu (DuckDB v1.1.3: path relatif diselesaikan terhadap folder kerja *setiap
      kali baca*, `../` terbaca, `~` tidak diekspansi, `* ? [ ]` diperlakukan sebagai pattern sampai tiga reader,
      dan `disable_glob_matching` ditolak sebagai parameter), lalu aturannya dipasang di engine
      (`duckdb.CanonicalFileSourcePath` + `ResolveFileSourcePath` + `ErrFileSourceGlobPattern` di
      `FileSourceQuery`); tabel pengukurannya di `docs/engine-map.md` §22 dan keputusan produknya di BRD §5.5.
      Dibuktikan dengan bundle yang benar-benar dipindah: `TestBundleRelativeSourceSurvivesTheFolderMoving`
      (desktop) menyimpan `data/orders.csv`, membaca 6 baris di folder pertama, lalu membaca jumlah yang sama dari
      temp root lain. Yang tersisa: **UI impor** (registrasi view per sumber file di produk, menampilkan tipe hasil
      inferensi apa adanya) dan keputusan XLSX.
      **Keputusan XLSX tercatat**: extension vs parse Go-side vs
      "simpan sebagai CSV/Parquet" — tanpa jalur diam-diam.
- [ ] **Transport ke sumber privat (BRD §5.7)** — bastion jadi **forwarder TCP lokal** dan dipakai oleh
      *semua* jalur, bukan hanya Test Connection. Ini wajib dikerjakan di engine karena `attachConnection`
      → `service.postgresDSN` (di web, `internal/service/session_preparer.go`) mengirim `cfg.Host` mentah ke DuckDB
      dan DuckDB **tidak bisa** menerima
      `net.Conn` (koneksi dibuat di sisi C dari string DSN) — jadi satu-satunya bentuk yang mungkin adalah
      DSN yang menunjuk `127.0.0.1:<port>` milik forwarder, dengan umur = umur sesi.
- [x] **Default `sslmode` di SATU tempat + tes regresi (2026-10-31, bug produk web juga).** Dulu duplikat dan
      kontradiktif: `session_preparer.go` memberi `disable`, sedangkan `connection_service.go` menempel
      `sslmode=disable` **kedua** di belakang `sslmode=` kosong — direproduksi, `pgx.ParseConfig` menolaknya
      (`cannot parse ...: failed to configure TLS (sslmode is invalid)`), artinya SyncSchema Postgres gagal total
      untuk koneksi tanpa `ssl_mode`. Sekarang keduanya memanggil `service.postgresDSN(cfg)`
      (`muara.bi-service/backend/internal/service/session_preparer.go`) yang men-default-kan `disable` sekali,
      dan nilai yang berasal dari user di-quote sesuai aturan libpq (`dsnValue`) sehingga spasi tidak lagi
      memecah nilai dan kredensial tidak bisa menyuntik parameter lain (tes: password
      `"x sslmode=disable application_name=evil"` tidak menurunkan TLS, diverifikasi lewat `pgx.ParseConfig`).
      **Ini juga memudahkan FR-1/M2:** forwarder bastion kini hanya perlu mengganti satu perakit DSN.
- [x] **`ssl_mode` divalidasi di titik tulis (2026-10-31).** `service.normalizeSSLMode` (web) hanya menerima enam
      nilai libpq (`disable · allow · prefer · require · verify-ca · verify-full`), trim spasi tepi, kosong ⇒
      `disable`. Ditegak di dua tempat: `UpsertEnvironment` menolak **sebelum** kredensial dienkripsi & disimpan
      (400 + daftar nilai yang boleh), dan `postgresDSN` ikut menolak (kini `(string, error)`) supaya profil lama
      tidak diam-diam turun mode — `verify` yang maksudnya `verify-full` berarti *tanpa pemeriksaan sertifikat*,
      jadi tidak boleh diredam. Frontend web tidak berubah (`environment-form.tsx` sudah `<Select>` enam nilai itu);
      yang kurang memang hanya gerbang API-nya. **Pelajaran untuk desktop:** validasi harus di titik tulis, dan
      handler wajib memetakan error validasi ke 400 — di web `Create`/`UpsertEnvironment`/`SyncSchema` tadinya
      memaksa semua error jadi 500, jadi pesan validasi tidak pernah sampai ke pemanggil.
- [x] **CA privat masuk spesifikasi attach (2026-11-02, sisi Postgres).** `ConnectionConfig.RootCert`
      (`root_cert`, di payload terenkripsi JSON → tanpa migrasi) dan `service.postgresDSN` merakitnya sebagai
      **`sslrootcert`** — nama itu diukur, bukan diasumsikan: `rootcert=` ditolak libpq bawaan DuckDB sebagai
      `invalid connection option` (pesan yang sama dengan opsi karangan sendiri), dan tes yang menunjuk CA ke
      berkas hilang membuktikan parameternya benar-benar dibuka. Aturan validasi di titik tulis: wajib absolut
      (path relatif resolved terhadap CWD proses, jadi dua environment bisa memuat CA berbeda dari satu profil),
      bukan teks PEM, dan hanya bersama `verify-ca`/`verify-full` — `require` menegosiasi TLS tapi menerima
      sertifikat apa pun, sehingga CA di sampingnya **terlihat** terverifikasi padahal tidak ada pemeriksaan.
      Default `sslmode=disable` tetap tidak diubah: itu keputusan per koneksi, bukan lagi cacat perakit DSN.
- [x] **Builder MySQL dibetulkan (2026-11-02, temuan 2026-10-31):** web dulu merakit
      `user:pass@tcp(host:port)/db` — bentuk driver Go yang **ditolak** `mysql_scanner` (`Invalid dsn …`), dan
      password berisi `@` atau `/` sudah merusak string sebelum TLS dibicarakan. Grammar scanner diukur per
      karakter (engine-map §17): kunci yang dikenal hanya `host, port, user, passwd, db, socket`, nilai kosong
      harus **dihilangkan** (bukan `passwd=` — itu opsi bernama kosong), spasi/tab/`=` wajib dalam kutip ganda,
      di dalam kutip hanya `\\` dan `\"` yang sah, sedangkan di luar kutip backslash verbatim, dan nilai yang
      **diawali** `"` harus diekscape atau parsing mati `unterminated quote`. Perakitnya pindah ke engine
      (`duckdb.MySQLDSN`) karena aturannya milik scanner, bukan milik salah satu produk.
- [x] **CA untuk MongoDB selesai 2026-11-02:** `root_cert` dirakit jadi **`tlsCAFile`** di URI (sumber Mongo
      tidak lewat DuckDB tapi lewat driver Go, jadi punya tempat untuk CA — kebalikan dari MySQL §17 engine-map).
      Yang diukur: driver membuka berkas CA **sebelum** menyentuh server (`127.0.0.1:1` cukup sebagai probe),
      nama warisan `sslCAFile` **diabaikan diam-diam** — karena itu URI yang masih memakannya ditolak, bukan
      diterjemahkan — dan `tlsCAFile=` kosong adalah error sehingga kunci yang tidak dipakai dihilangkan.
      Pemetaan mode: `require|verify-ca|verify-full` → `tls=true`; `disable` dan fallback `allow|prefer` tetap
      polos, karena form web mengirim `prefer` untuk semua driver dan memaksanya ke TLS akan merusak profil
      yang hari ini jalan. CA di samping mode tanpa TLS ditolak di titik tulis.
- [x] **UI kolom CA di form koneksi web (2026-11-02).** Tiga usulan wireframe dikonfirmasi user, lalu dipasang:
      kolomnya per-driver mengikuti aturan yang sudah diukur (`sslrootcert` + gerbang mode untuk Postgres,
      `tlsCAFile` + nyala/mati untuk Mongo, **tampil tapi nonaktif dengan alasan** untuk MySQL/SQLite, input path
      teks bukan file picker, tanpa saklar TLS baru). Pekerjaan yang membuatnya aman justru di belakangnya:
      `EnvironmentInfoResponse` tidak pernah mengembalikan `ssl_mode`/`root_cert`, jadi form yang selalu mulai dari
      `"prefer"` + CA kosong itu **menghapus path CA tersimpan setiap kali environment lama disimpan ulang** —
      sekarang keduanya ikut kembali (password & uri tetap write-only karena bisa membawa kredensial).
      Tes round-trip + gerbang empat-mode hijau; uji klik di `/sources` menunggu `muara_api`/`muara_web`
      dibangun ulang (infra milik user).
- [ ] **Sisa CA privat (BRD §10 butir 8):** **path relatif CA ikut §5.5** begitu bundle `.muara` jadi storage
      (M3) — jalur desktop belum menyimpan CA di manapun, dan `uri`/`password` write-only masih berarti form
      web menimpa nilai lama dengan yang kosong (dikonfirmasi sebagai celah yang diketahui, bukan bug form).

**Exit:** tree menampilkan kolom Mongo dan Postgres berdampingan; satu query lintas dua alias jalan;
pemotongan selalu ditandai di API, bukan hanya di log; dan sumber di belakang bastion/VPN terbukti jalan pada
**jalur query**, bukan hanya lolos Test Connection.

## M3 · Bundle `.muara` sebagai storage

- [x] **Semantik `Project` diputuskan (2026-10-30, direvisi 2026-10-31):** project **hanya membungkus
      Dashboard**. `View` hidup satu berkas tanpa `project_id` dan **boleh direferensi dashboard dari project
      mana pun** — keanggotaan project atas View berupa daftar pin (`view_pins[]`), bukan kepemilikan.
      Connection, alias, semantic model, dan Profil lokal tetap satu berkas dan tidak dimiliki project.
      Praktisnya: `projects.json` (id, nama, warna, catatan, `dashboard_refs[]`, `view_pins[]`) di manifest
      §6.2, `dashboard_refs[]` menunjuk Dashboard, `View.folder` jadi sub-tata letak.
      **Alasan revisi:** kalau View dimiliki satu project, query yang sama harus digandakan setiap kali klien
      lain minta dashboard serupa — persis pekerjaan duplikasi yang dihindari produk ini. Konsekuensi yang
      harus ikut dibangun: hapus project tidak menghapus View yang dipin-nya; export cakupan "Proyek" menarik
      View yang direferensi, bukan dimiliki; import bundel yang dua kali harus **idempoten** (dedupe by
      ID + hash isi), dan View hasil pin tidak terbawa dua kali ke bundel lain.
- [ ] **Tiga tingkat ikut-tidaknya definisi koneksi saat export** (menggantikan §5.3/§5.4 yang hanya
      mengenal "alias + driver, tanpa host"):
  - selalu ikut: alias, driver, nama database, filter skema, catatan — tanpa ini SQL tidak resolve;
  - ikut, bisa dimatikan: **host, port, user, target SSH, sslmode**. Ini bukan secret, tapi juga bukan
    informasi yang layak dikirim ke luar tim — jadi toggle eksplisit "sertakan alamat server", default
    NYALA untuk berbagi dengan rekan yang menunjuk server yang sama;
  - tidak pernah ikut default: password/token/key. Hidup di keychain mesin masing-masing; hanya lewat
    `secrets.enc` ber-passphrase kalau user memilihnya.
- [ ] **Ekspor hasil ke CSV / XLSX = reuse, bukan fitur baru.** `internal/exporter/` sudah punya
      `WriteXLSX` (excelize v2.11.0, streaming writer, satu worksheet per sheet, header bergaya) dan
      CSV/JSON. Pure Go, tanpa CGO. Yang perlu diputuskan: jalur ekspor **tidak boleh** mewarisi LIMIT
      tampilan 1000 baris — menulis ke disk laptop user adalah justru kasus di mana guardrail baris tidak
      diperlukan, dan itu yang membuat "angka terpotong di layar" bisa diselesaikan tanpa Parquet.
- [x] **Format & manifest berversi, store dengan save atomik (tmp → rename), entity `View` kelas satu**
      (2026-11-02, `internal/bundle/`): `.muara` ditulis sebagai zip berisi **7 entri JSON** — `manifest.json`
      plus satu berkas per tipe entitas — dan `Open` membaca `manifest.json` **lebih dulu**. Format lain
      (`muara.weird`), `version` ≠ 1, `min_app_version` di atas versi app, entri tak dikenal, entri wajib
      yang hilang, entri ganda, dan `secrets.enc` (KDF-nya belum ditulis) **ditolak dengan menyebut nama field
      dan angkanya**, bukan error misterius. Save = `CreateTemp` di folder yang sama → tulis zip → `Sync` →
      `Rename`; kegagalan di titik mana pun membuat berkas lama tetap **identik byte** dan tidak ada `.tmp`
      tersisa. Menyimpan bundle yang sama dua kali menghasilkan keluaran **identik byte** (satu timestamp untuk
      seluruh archive) — itu prasyarat dedupe hash-isi di M4. `View` jadi kelas satu: `kind` dan `query_mode`
      divalidasi, `SYNCED` wajib membawa `builder_state` dan `CUSTOM_SQL` wajib membawa `raw_sql` (kalau tidak,
      View dibuka kembali sebagai widget kosong), `max_rows`/`cache_ttl_seconds` ikut tersimpan, dan
      `builder_state`/`presentation` tetap JSON mentah — bentuknya milik FR-4 dan renderer, bukan store.
      **Dua fakta yang cuma bisa didapat dengan mengukur:** `json.Marshal` atas slice `nil` menghasilkan `null`,
      jadi entri kosong sengaja ditulis `[]` supaya bundle masih enak dibaca tangan; dan `MarshalIndent` Go 1.27
      ikut **meng-indent JSON mentah yang tertanam di dalamnya**, sehingga round-trip dibandingkan sebagai hasil
      dekode, bukan sebagai byte — spasi bukan perubahan View.
      Aturan §5.4 ditegakkan pada **bentuk tipe**, bukan pada langkah menyaring saat menulis: `Connection` tidak
      punya field password/token/key/DSN sama sekali, jadi "grep isi bundle = nol string kredensial" adalah
      konsekuensi struktur. Tingkat 2 (`host`/`port`/`user`/`ssl_mode` + target SSH) dikumpulkan dalam satu blok
      opsional supaya export nanti bisa membuangnya sebagai satu keputusan, bukan satu flag per field.
- [ ] **Aturan path file** (lubang yang dibuka oleh keputusan #3): path relatif terhadap berkas `.muara`
      sebagai bentuk kanonik; resolver profile untuk remap lokal; ukuran/jumlah file dibatasi.
      **Aturannya sudah ditulis dan ditegakkan sejak 2026-11-02** (`duckdb.CanonicalFileSourcePath` +
      `ResolveFileSourcePath` + penolakan glob/tilde di `FileSourceQuery`; BRD §5.5, engine-map §22).
      **Resolvernya sekarang dipanggil store** (2026-11-02): `Save` menulis tiap path sumber dalam bentuk
      kanonik relatif ke folder `.muara` (di luar folder → ditinggal absolut), `Open` dan
      `Bundle.ResolveFileSources` membacanya ulang terhadap anchor yang sama, dan `~/…` atau `../…` yang
      ditemukan di dalam berkas orang lain membuat bundle ditolak dengan alasan — sedangkan berkas yang
      **hilang** tidak, karena itu justru nasib yang harus dilaporkan pre-flight (§5.5).
      **Pemakai keduanya sekarang aplikasi itu sendiri** (2026-11-02, `document.go`): saat `Open`, tiap path hasil
      resolver di-`os.Stat` lebih dulu supaya "tidak ada" dan "ternyata folder" punya laporan berbeda, CSV/Parquet/
      JSON masuk sebagai temp view (`RegisterFileSource`) dan SQLite sebagai katalog (`AttachSQLite`) — terukur satu
      `Open` membuat `SELECT ... FROM order_lines` menjawab 3 baris tanpa potongan lewat model semantik, dan
      `wh.orders` menjawab 6 baris lewat katalog ter-attach. **SQLite ikut aturan path ini**: koneksi ber-driver
      `sqlite` wajib membawa blok `file`, jadi SaveAs menyimpan `data/orders.csv` **dan** `data/warehouse.sqlite`
      relatif terhadap folder barunya.
      **Angka awal untuk batasnya:** bundle kosong 1189 byte; 1000 View dengan `builder_state` + `raw_sql`
      berbeda = 625 KB `views.json` tak terkompresi → **16 KB** di dalam zip. Jadi ukuran bundle ditentukan
      berkas data di *samping* `.muara`, bukan definisi di dalamnya — sisa butir ini tinggal mengukur jumlah
      & ukuran file sumber yang ikut folder bundle, dan itu baru punya angka nyata di M4 waktu berkasnya
      benar-benar ikut dipindah. Exit "dua berkas di folder berbeda di mesin yang sama tetap resolve" sudah
      terbukti dua lapis: `TestBundleRelativeSourceSurvivesTheFolderMoving` (engine) dan
      `TestBundleAndDataInDifferentFoldersStillResolve` (store).
- [x] **Aplikasi bekerja pada satu berkas terbuka + `Profile` lokal** (2026-11-02, `document.go`, `dialogs.go`,
      `internal/profile/`; BRD §5.1 + §6): `QueryService` punya `Open` / `NewUntitled` / `Save` / `SaveAs` /
      `Current`, dan **satu sesi DuckDB per dokumen** — dokumen dan sesi diganti bersamaan karena DuckDB menyimpan
      katalog ter-attach dan temp view seumur sesinya. Terukur: `Open` satu bundle lalu `Run` SQL milik View-nya
      menjawab 3 baris; setelah `Open` bundle kedua, temp view bundle pertama **gagal dibaca** (itu justru
      yang diingini) sementara sumber bundle kedua menjawab.
      **"Mulai kosong" menulis berkas sungguhan**: `Untitled.muara` di `~/Documents/muara.bi` (folder konfigurasi
      per-user kalau `Documents` tidak ada — di mesin ini ada, jadi itulah jalurnya), dan berkas itu tidak pernah
      ditimpa: panggilan kedua menghasilkan `Untitled-2.muara` sedangkan byte berkas pertama tetap sama.
      `SaveAs` **memindah** dokumen kerja (BRD §5.1 "Export = Save As") lalu me-mount ulang terhadap folder baru,
      jadi hasil salin-folder langsung jalan tanpa satu pun path perlu ditunjuk ulang.
      **Satu berkas yang rusak tidak membuang yang sedang dikerjakan:** `Open` atas berkas teks biasa menolak
      dengan `ErrNotABundle`, sesi lama tetap hidup, dan query yang tadinya menjawab masih menjawab.
      **`Profile` berdiri sebagai berkas per mesin di luar bundle** (`~/Library/Application Support/muara.bi/
      profile.json`, `os.UserConfigDir()`; tidak ada field string berkata-kreditensial — yang tersimpan cuma
      `secret{kind,label}`; `Secret()` menolak `keychain` dengan menyebut labelnya, bukan membalas password kosong,
      karena "" akan sampai ke server sebagai "authentication failed" dan menunjuk pintu yang salah).
      Laporan mount menjawab §5.2: alamat **dari profil mengalahkan yang dari bundle** (terukur: profil
      `10.20.3.4:5432` tampil, `db.internal` tidak), dan tanpa profil bundle tetap menampilkan alamatnya sendiri.
      Yang **belum** bisa di mesin ini dan tertulis apa adanya di laporan: koneksi ber-server tidak ter-attach —
      `mongodb` karena sesi desktop belum punya jalur attach-nya sama sekali, dan `postgres`/`mysql` karena
      conninfo libpq masih tinggal di `muara.bi/core` (web) yang tidak boleh diimport desktop, sementara keychain
      baru M6. Dialog berkas (`OpenWithDialog`/`SaveAsWithDialog`) memakai dialog Wails di sisi Go — path tidak
      pernah diketik di webview.
      **Bar dokumen sekarang sampai ke jendelanya juga** (2026-11-02, `frontend/src/App.jsx` + `styles.css`, setelah
      sketsanya disetujui): satu deret `Mulai kosong` · `Buka…` · `Simpan` · `Simpan sebagai…`, nama berkas dengan
      `●` saat belum disimpan, baris kedua folder + jumlah View/model semantik/sumber + "belum disimpan",
      `profileError` ditampilkan kalau profil mesin tidak bisa dibaca, dan panel **Sumber** yang menghitung
      "N ter-mount dari M" lalu menuliskan alasan tiap `✗` di bawah barisnya. Editor SQL tidak lagi dibuka dengan
      query fixture: begitu bundle terbuka, isinya diganti `SELECT * FROM <model pertama> LIMIT 20` — **hanya kalau
      user belum mengetik apa pun**, karena menekan satu tombol tidak boleh membuang query yang sedang ditulis.
      `Simpan`/`Simpan sebagai…` dimatikan selama tidak ada berkas terbuka, supaya penolakan di Go tidak pernah
      tampak sebagai error misterius. Dua tombol spike (`Attach Postgres sample`, `Attach SQLite fixture`) keluar
      dari UI: fixture-nya tetap teruji dari Go (`query_service_test.go`), dan menyimpannya di layar membuat orang
      menyangka itu sumber sungguhan. Bindings di-generate ulang → **12 metode, 5 model**.
      Bagian jendela dijaga `TestDocumentControlsAreInShippedBundle`: label kontrol dan JSON tag `duckdbVersion` /
      `profileError` / `semanticModels` wajib ada di bundle JS yang ter-embed — nama metode binding **tidak** bisa
      dipakai sebagai penanda karena generator menulis `$Call.ByID(<hash>)` dan minifikasi menghapus sisanya.
      **Yang masih menunggu manusia:** satu klik nyata pada dialog Wails. Buildnya sudah selesai — user
      membersihkan ruang (19 GiB bebas), `sh build/macos-app.sh` lolos `minos=14.0` + 3 extension, dan hasilnya
      terpasang di `/Applications/muara-bi.app` (2026-11-02) — yang belum pernah terjadi hanyalah dialog OS itu
      sendiri muncul di jendela sungguhan.
      Bahasanya sengaja mengikuti `docs/wireframe.html` (sumber hijau/merah + alasan di barisnya), tapi jendela ini
      **belum** separuh layar itu: S0 "terakhir dibuka" + Import bundle dengan laporan pre-flight masih M4, dan
      pohon View/Dashboard di S1 masih M5. Yang dibangun sekarang cuma barisan yang menjawab §5.1.
- [ ] **Satu klik manusia pada dialog berkas** — `OpenWithDialog` / `SaveAsWithDialog` adalah satu-satunya jalur
      tempat sebuah path masuk ke aplikasi, dan keduanya belum pernah berjalan di jendela sungguhan. `.app`-nya
      sudah terpasang di `/Applications/muara-bi.app` (2026-11-02), jadi yang kurang cuma dua klik (pilih berkas,
      pilih folder); gerbang otomatis sudah menjaga label kontrol ada di bundle, tapi tidak ada tes Go yang bisa
      membuktikan dialog OS menampilkan `*.muara` dan mengembalikan path yang benar.
- [x] Kredensial tidak pernah masuk bundle kecuali blok `secrets.enc` opt-in (argon2id → AES-256-GCM).
      KDF-nya kode baru; `cryptohelper` hari ini hanya AES-GCM dengan key dari env.
      **Terukur 2026-11-02 (`internal/secrets/`, M3b):** `SaveWithSecrets` satu-satunya jalur blok tertulis,
      `Save` polos menolak bundle ber-blok (`ErrSealedBlock`, menjatuhkan blok harus eksplisit), dan grep atas
      bundle ber-secrets dengan payload penuh kata kredensial menemukan **nol kata** — deterministik karena isi
      biner ditulis hex. Envelope JSON berversi menyimpan salt/nonce/param KDF; salah passphrase dan blok rusak
      sama-sama `ErrWrongPassphrase` (GCM tidak bisa membedakannya). Dua save ber-secrets **sengaja** tidak
      identik byte — dedupe M4 membandingkan tujuh entri entitas saja. Yang menunggu manusia: prompt passphrase
      di dialog export — milik M3(c) bersama toggle tiga tingkat.

**Exit:** save lalu buka ulang mempertahankan seluruh View utuh; grep isi bundle menemukan nol string
kredensial; dua berkas di folder berbeda di mesin yang sama tetap resolve.

## M4 · Round-trip export/import

- [ ] FR-10 export (cakupan + resolusi dependensi transitif + pilihan kredensial tiga tingkat) dan
      FR-11 import (pre-flight: bentrok alias, remap, validasi skema merah/kuning, beda driver,
      **path file hilang/berubah skema**, **View duplikat: ID + hash isi**).
- [ ] Tidak ada partial import.
- [ ] Skenario penerimaan BRD §13 jadi tes regresi otomatis, dijalankan di dua mesin di CI, dengan
      langkah tambahan: file sumber ada di folder berbeda di PC B, dan langkah 8 (View di-pin dua
      project, import bundel sama dua kali → tetap satu View).

**Exit:** skenario §13 + varian file lulus tanpa satu karakter SQL diubah.

## M5 · Jalur pemula

- [ ] FR-4 builder 4 langkah dengan preview live + SQL read-only + ringkasan satu kalimat; FR-5
      suggested joins (bergantung PK/FK dari M2); FR-6 template; FR-7 inspector.
- [ ] Port modul TS dari web, bukan tulis ulang: `lib/echarts-options.ts`, `lib/color.ts`,
      `lib/table-theme.ts`, `grid.tsx`, inspector drawer, `chart-palette-fields.tsx`, datapabel.
      Yang diganti hanya lapisan transport (fetch → binding Wails).
- [ ] Helper pemakaian kolom array Mongo (`UNNEST`/`json_extract`) di builder — kolomnya sengaja
      disimpan sebagai string JSON, dan tanpa helper itu FR-4 tidak bisa group-by array.

**Exit:** orang yang tidak bisa SQL membuat table view yang benar tanpa membuka editor SQL.

## M6 · Kanvas, alert, dan rilis

- [ ] FR-8 dashboard canvas + Present; FR-13 alert (cron in-process, hanya jalan saat aplikasi terbuka —
      harus dinyatakan jujur di UI); keychain; dark mode.
- [ ] Signing + notarization + auto-update (BRD §14 risiko & §15 — diputuskan sebelum M6, bukan di sini).
- [ ] Berkas third-party notices ikut installer (Wails/DuckDB/go-duckdb sama-sama MIT; syaratnya
      notis hak cipta didistribusikan). Penamaan produk mematuhi pedoman merek DuckDB.

**Exit:** installer bertanda tangan jalan di Windows & macOS tanpa peringatan OS.

---

## Struktur

```
muara.bi-desktop/     repo produk ini: shell, bundle, profile, schema, UI
muara.bi/engine       modul bersama (hasil M1) — dev lokal via go.work
muara.bi-service/     web multi-tenant; hanya mengganti import path di M1
```

Nama direktori mengikuti gaya repo lama (`muara.bi-*`), bukan `muara-bi-*` seperti ditulis draf BRD v1 —
konfirmasi sebelum repo di-push.

## Yang belum diputuskan

- ~~Semantik `Project`~~ → diputuskan 2026-10-30, direvisi 2026-10-31 (View lintas project), lihat M3.
- Platform mana yang jadi target rilis pertama kalau Windows gagal di M0.
- Batas ukuran bundle (BRD §15 butir 3) — butuh angka nyata dari tes save-atomik M3.
- Mode Present butuh ekspor PDF/snapshot atau tidak (BRD §15 butir 4).
- ~~Wireframe~~ → `muara-bi-desktop-wireframes.canvas.tsx` yang dirujuk BRD v1 memang tidak ada di mesin
  mana pun. Digantikan [docs/wireframe.html](docs/wireframe.html) — 10 layar §12.3, token §12.2, dark mode,
  dan **sudah disetujui user** (2026-10-31, termasuk View lintas project di S4a). Jadi acuan visual M5.
