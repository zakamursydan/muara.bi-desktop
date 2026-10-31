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
      menyentuh logika DuckDB. Belum diverifikasi lewat build image: daemon Docker mati (milik user);
      jalankan `make image-core` untuk menutup sisa exit criterion ini.
- [ ] `max_memory` DuckDB jadi konfigurasi (sekarang hardcoded 2 GB/sesi di `session_cgo.go`).
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
*(2026-10-31: terpenuhi untuk `go build/vet/test` dua modul × dua mode CGO; `internal/` sudah bersih dari
DuckDB/exporter/ssh/ai/builder; kontrak §7.3 & §7.4 sudah ditegakkan di engine. Yang tersisa hanya bukti
`docker build` (`make image-core`) dan butir `max_memory` di atas.)*

## M0 · Spike platform desktop (setelah M1)

Keluaran: jawaban ya/tidak untuk "Wails v3 × CGO × DuckDB asli × offline di tiga OS", dengan CI.

- [ ] Skeleton Wails v3 (versi dikunci, `LICENSE` di tag itu dibaca, bukan dari branch utama) + React/Vite.
- [ ] Satu binding memanggil engine dan `SELECT` nyata dari Postgres tampil di grid — **di Windows**.
- [ ] Tiga pertanyaan Windows yang harus dijawab, masing-masing dengan bukti, bukan asumsi:
  - [ ] go-duckdb v1.8.5 menyediakan `libduckdb.a` untuk windows/amd64? ABI-nya cocok dengan mingw-w64?
  - [ ] biner yang di-link statis bisa `LOAD` extension di Windows (Unix pakai `-extldflags=-Wl,-E`
        untuk `dlopen`; Windows tidak punya `RTLD_GLOBAL`)?
  - [ ] `postgres_scanner`/`mysql_scanner`/`sqlite_scanner` tersedia sebagai berkas `windows_amd64`?
- [ ] Extension dibundle dan `extension_directory` diarahkan ke folder aplikasi; diverifikasi dengan
      jaringan **dimatikan** di ketiga OS.
- [ ] CI per-OS (macOS/Linux/Windows). Bundel extension **tidak** di-commit dari satu mesin:
      `cmd/duckdb-bundle` dijalankan per-OS karena folder hasilnya per-platform.
- [ ] Bukti negatif: matikan CGO → aplikasi menolak start dengan pesan jelas (bukan menampilkan kosong).
- [ ] Diukur lewat jaringan nyata, bukan lewat localhost: **berapa koneksi yang dibuka `postgres_scanner` per
      query paralel** (`threads` disetel di `session_cgo.go:39`), latensi round-trip, dan berapa lama forwarder
      bastion bertahan hidup saat VPN putus-nyambung. Angkanya menentukan batas `max_connections` yang layak
      dan apakah `threads` perlu ikut profil, bukan asumsi.

**Exit:** installer dev berjalan di Windows dan macOS tanpa peringatan Gatekeeper/SmartScreen untuk
skenario uji, query federated nyata jalan offline, dan CI per-OS hijau. Kalau salah satu pertanyaan
Windows gagal, ini titik putar ke Tauri + sidecar Go atau macOS-only — diputuskan di sini, bukan di M4.

## M2 · Bersihkan blocker data (di engine, dipakai dua produk)

- [ ] `SyncSchema` lintas driver (`connection_service.go:321,367` saat ini hanya Postgres dan diam).
- [ ] Skema terkualifikasi penuh: `alias.schema.tabel`, PK/FK, nullability, komentar tabel/kolom,
      jumlah baris — mengganti `map[string][]ColumnDef` (`:344-348`). Ini prasyarat FR-2, FR-5, FR-10.
- [ ] Penanda pemotongan **sampai ke UI**: sisinya engine sudah selesai di M1.4 (`MongoMount.Truncated` →
      `QueryResult.TruncatedBy = source_cap`), yang tersisa adalah menampilkan `truncated`/`truncated_by`/
      `capped_sources` di result grid (FR-3) dan di pre-flight bundle (FR-11).
- [ ] Sumber file v1: CSV + Parquet + JSON lewat `read_csv_auto`/`read_parquet`/`read_json_auto`
      (core DuckDB, tanpa extension per-OS). **Keputusan XLSX tercatat**: extension vs parse Go-side vs
      "simpan sebagai CSV/Parquet" — tanpa jalur diam-diam.
- [ ] **Transport ke sumber privat (BRD §5.7)** — bastion jadi **forwarder TCP lokal** dan dipakai oleh
      *semua* jalur, bukan hanya Test Connection. Ini wajib dikerjakan di engine karena `attachConnection`
      (`session_preparer.go:193-202`) mengirim `cfg.Host` mentah ke DuckDB dan DuckDB **tidak bisa** menerima
      `net.Conn` (koneksi dibuat di sisi C dari string DSN) — jadi satu-satunya bentuk yang mungkin adalah
      DSN yang menunjuk `127.0.0.1:<port>` milik forwarder, dengan umur = umur sesi.
- [ ] **`rootcert` / CA privat** masuk spesifikasi attach + path relatif (§5.5); default `sslmode` ditetapkan
      di SATU tempat. Hari ini duplikat dan kontradiktif: `session_preparer.go:196-199` memberi `disable`,
      sedangkan `connection_service.go:322-326` menempel `sslmode=disable` kedua di belakang `sslmode=` kosong —
      direproduksi 2026-10-31, `pgx.ParseConfig` menolaknya
      (`cannot parse ...: failed to configure TLS (sslmode is invalid)`), artinya SyncSchema Postgres gagal
      total untuk koneksi tanpa `ssl_mode`. Sekaligus jadi tes regresi, karena ini bug produk web juga.

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
- [ ] Format & manifest berversi, store dengan save atomik (tmp → rename), entity `View` kelas satu.
- [ ] **Aturan path file** (lubang yang dibuka oleh keputusan #3): path relatif terhadap berkas `.muara`
      sebagai bentuk kanonik; resolver profile untuk remap lokal; ukuran/jumlah file dibatasi.
- [ ] Kredensial tidak pernah masuk bundle kecuali blok `secrets.enc` opt-in (argon2id → AES-256-GCM).
      KDF-nya kode baru; `cryptohelper` hari ini hanya AES-GCM dengan key dari env.

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
