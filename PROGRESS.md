# PROGRESS — muara.bi Desktop

## 1. Cara melanjutkan (prompt standar)

```
Halo, saya melanjutkan pengembangan muara.bi Desktop (produk offline single-user, terpisah dari
muara.bi web).
Tolong baca PLAN.md, docs/brd-review.md, docs/engine-map.md, dan PROGRESS.md. BRD produk ada di BRD.md (v2).
Lanjutkan eksekusi task berikutnya sesuai checklist di PLAN.md!
```

## 2. Sumber kebenaran

| Dokumen | Isi | Catatan |
|---|---|---|
| [BRD.md](BRD.md) | produk, keputusan arsitektur, FR-1..FR-15, kontrak wajib §7, roadmap, skenario penerimaan §13 | **revisi v2 (2026-10-30)**, menggantikan `~/Downloads/DESKTOP-BRD.md` v1 |
| [docs/brd-review.md](docs/brd-review.md) | koreksi BRD terhadap kode nyata, dengan file:line | baca ini sebelum mengerjakan §9 BRD |
| [docs/adr/0001-engine-module.md](docs/adr/0001-engine-module.md) | kenapa engine jadi modul sendiri | keputusan diterima |
| [docs/engine-map.md](docs/engine-map.md) | batas modul M1 yang diukur dari `go list` + hasil CGO fail-fast | baca ini sebelum `git mv` |
| [PLAN.md](PLAN.md) | urutan M1→M0→M2..M6 + exit criteria | yang dieksekusi |

## 3. Status terkini

- **Update terakhir:** 2026-11-01
- **Fase aktif:** **M1 selesai 2026-10-31.** M1.1 (peta paket engine), M1.2 (CGO fail-fast), **M1.3 (ekstraksi
  `muara.bi/engine`)**, **M1.4 (kontrak `QueryResult.truncated`)** dan **M1.5 (`max_memory` jadi
  konfigurasi)** tuntas, dan exit criterion terakhir dibuktikan `make image-core` — image web benar-benar
  dibangun dari engine di `muara.bi-service/backend/engine/` (lewat `replace`), bukan dari salinan kedua.
  Belum ada skeleton Wails; **spike M0 sudah dimulai 2026-11-01** dengan modul Go pertama di repo ini.
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
    self-signed. Masuk FR-1 + blocker §10 butir 8.
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
   Yang belum: skeleton **Wails v3** (kunci versinya, baca lisensi dari tag itu) + React/Vite; **cara
   membaca RAM total per OS** (dibutuhkan default `min(4GiB, RAM/4)`); bundel extension + verifikasi
   dengan jaringan **dimatikan** di tiga OS; CI per-OS; tiga pertanyaan Windows; dan apakah `threads`
   perlu ikut profil — sekarang masih hardcoded 2 (`engine/duckdb/session_cgo.go`, `NewEngine`).
2. Sisa pekerjaan SSL setelah bug DSN + validasi `ssl_mode` ditutup (masuk M2, jangan tertukar dengan yang
   sudah selesai): `rootcert`/CA privat (BRD §10 butir 8) — perakit DSN sudah satu tempat dan sudah
   memvalidasi mode, jadi yang kurang hanya field CA + alur "percayai sertifikat ini" (§5.7 aturan 3).
3. Verifikasi yang masih menunggu: perubahan DSN/validasi ini **baru terbukti di level unit**. HTTP-nya
   (400 sungguhan dari `POST .../environments`) butuh `muara_api` dibangun ulang dari image baru, dan
   stack user sedang berjalan di 18080/18000 — restart adalah aksi milik user, harus izin dulu.
   *Penutup M1 (`make image-core`) sudah selesai 2026-10-31 — lihat §3.*

## 5. Kendala & catatan lingkungan

- Mesin ini: macOS arm64, Docker Desktop VM 4 GB. `go` di `/usr/local/go/bin` adalah 1.25.0, tapi
  `GOTOOLCHAIN=auto` memakai toolchain **1.27.1** yang di-download ke module cache (dua `go.mod`
  meminta `go 1.27.1`) — artinya build di mesin ini butuh cache itu; offline tanpa cache build gagal
  dengan pesan "go.mod requires go >= 1.27.1". DuckDB linux/darwin sudah terverifikasi
  di produk web; **Windows belum pernah disentuh sama sekali** — M0 adalah pertama kalinya.
- Cara menjalankan spike: `CGO_ENABLED=1 go build -o /tmp/muara-spike ./cmd/spike` lalu
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
- Repo ini sudah `git init` dengan 10 commit dokumen (per 2026-11-01) dan **belum ada remote**;
  `cmd/spike/`, `go.mod` dan `go.sum` adalah kode pertama di repo ini.
