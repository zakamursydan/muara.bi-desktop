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

- **Update terakhir:** 2026-10-31
- **Fase aktif:** **M1 berjalan.** M1.1 (peta paket engine), M1.2 (CGO fail-fast), **M1.3 (ekstraksi
  `muara.bi/engine`)**, **M1.4 (kontrak `QueryResult.truncated`)** dan **M1.5 (`max_memory` jadi
  konfigurasi)** selesai 2026-10-31 — engine sekarang satu modul di `muara.bi-service/backend/engine/`
  dengan `go.mod` sendiri, dipakai web lewat `replace`.
  Belum ada `go.mod` desktop, belum ada skeleton Wails — keduanya memang di belakang M1. Sisa M1:
  **satu bukti `docker build` (`make image-core`)** yang butuh daemon Docker milik user.
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
  dua mode CGO; **belum teruji lewat HTTP** (daemon Docker mati).
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
  - **Batas memori (M1.5, diverifikasi 2026-10-31):** `SET max_memory` sekarang datang dari
    `GuardrailConfig.MaxMemory` (`"2GB"` bila tidak diisi) dan `NewEngine` menolak nilai yang tidak
    berbentuk angka+unit — `"2048"` ditolak karena DuckDB membacanya sebagai **byte**, bukan 2 GB.
    Dua fakta yang harus ikut masuk settings desktop (FR-15): DuckDB menyimpan `"256MB"` sebagai
    256 × 10⁶ byte tapi `current_setting('max_memory')` menampilkannya dalam MiB (`244.1 MiB`), jadi
    label "2 GB" efektifnya 1,86 GiB; dan puncak pemakaian = **jumlah sesi × budget per sesi**.
    Untuk pertanyaan BRD §18 butir 6 (laptop 8 GB / 16 GB): secara teknis sudah bisa diubah per pengguna,
    tinggal diputuskan angka default + pengingat bahwa DuckDB juga bisa menulis ke disk saat kelebihan
    memori (`temp_directory` — belum pernah disentuh sama sekali).

## 4. Task berikutnya (urutan)

1. **Penutup M1 (butuh Docker aktif):** `make image-core` membuktikan stage `build` benar dengan modul
   bersarang. Ini satu-satunya exit criterion M1 yang belum punya bukti — semua butir checklist lain
   (M1.1–M1.5) sudah selesai dan hijau pada 2026-10-31.
2. **Keputusan kecil yang terbuka (BRD §18 butir 6):** angka default `max_memory` untuk laptop 8 GB/16 GB
   dan apakah settings desktop memaknai "GB" sebagai GiB agar tidak membingungkan (engine sudah menerima
   `MB`/`GB`/`TB` desimal dari DuckDB).
3. Baru setelah itu: spike M0 (Wails + CI per-OS) — dan repo desktop ikut memakai engine lewat
   `replace muara.bi/engine => ../muara.bi-service/backend/engine`, bukan salinan. `threads` per sesi
   masih hardcoded 2 (`engine/duckdb/session_cgo.go`) dan sengaja dibawa ke M0 bersama profil CPU mesin.
4. Sisa pekerjaan SSL setelah bug DSN + validasi `ssl_mode` ditutup (masuk M2, jangan tertukar dengan yang
   sudah selesai): `rootcert`/CA privat (BRD §10 butir 8) — perakit DSN sudah satu tempat dan sudah
   memvalidasi mode, jadi yang kurang hanya field CA + alur "percayai sertifikat ini" (§5.7 aturan 3).
5. Verifikasi yang masih menunggu infra: perubahan DSN/validasi ini **baru terbukti di level unit**;
   HTTP-nya (400 sungguhan dari `POST .../environments`) belum bisa dites karena daemon Docker mati,
   jadi API + Postgres/Redis lokal tidak jalan.

## 5. Kendala & catatan lingkungan

- Mesin ini: macOS arm64, Docker Desktop VM 4 GB. `go` di `/usr/local/go/bin` adalah 1.25.0, tapi
  `GOTOOLCHAIN=auto` memakai toolchain **1.27.1** yang di-download ke module cache (dua `go.mod`
  meminta `go 1.27.1`) — artinya build di mesin ini butuh cache itu; offline tanpa cache build gagal
  dengan pesan "go.mod requires go >= 1.27.1". DuckDB linux/darwin sudah terverifikasi
  di produk web; **Windows belum pernah disentuh sama sekali** — M0 adalah pertama kalinya.
- BRD §18 menyuruh membaca `muara-bi-desktop-wireframes.canvas.tsx`, tapi file itu tidak ada di
  repo web, di repo ini, maupun di `Downloads`. Sebagai gantinya sudah dibuat wireframe sendiri:
  [docs/wireframe.html](docs/wireframe.html) — 10 layar §12.3, token warna §12.2, toggle dark mode,
  dibuka langsung di browser. Disetujui dan sudah diselaraskan dengan keputusan View lintas project
  (layar S4a: "Pin ke proyek lain", bukan duplikasi).
- Keputusan user 2026-10-30: **opsi A disetujui** — repo web boleh disentuh satu commit untuk
  `git mv` engine + ganti import path (M1.3).
- Repo ini sudah `git init` tapi belum ada commit dan belum ada remote.
