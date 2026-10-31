# Review DESKTOP-BRD.md terhadap kode `muara.bi/core`

Tanggal: 2026-10-30 · Verifikasi terhadap repo `/Users/Go/muara.bi-service` (branch `development`,
suite `go build/vet/test ./...` exit 0).

Dokumen ini adalah koreksi, bukan penggantian. BRD tetap dokumen produk; file ini catatan teknis
yang membuktikan atau membantah klaimnya.

## 1. Status klaim §9 (Blocker Teknis)

### §9.1 Mongo tidak ter-attach di jalur query — SUDAH SELESAI, jangan dikerjakan ulang

`case "mongodb":` memanggil `duckdb.StreamMongoCollections` dari `attachConnection` di
`backend/internal/service/session_preparer.go:208`, termasuk penolakan database yang punya nol
collection. Diregresi oleh `internal/duckdb/federated_e2e_test.go` (JOIN tiga sumber) dan
`internal/service/session_preparer_test.go`.

Konsekuensi untuk M2: item ini hilang dari backlog. Yang tersisa justru **cap di dalamnya** (lihat §4 file ini).

### §9.2 `SyncSchema` hanya Postgres — BENAR

`backend/internal/service/connection_service.go:321`guarded `if conn.DriverType == "postgres"`,
dan fungsi `return nil` di :367 untuk mysql/sqlite/mongodb. Persis "keluar diam-diam tanpa menyimpan
apa pun". FR-2 tidak bisa terpenuhi tanpa ini, dan FR-10 (pre-flight import) ikut gagal karena tidak
ada data untuk divalidasi.

### §9.3 Schema cache miskin — BENAR, lebih buruk dari yang ditulis BRD

Tipe yang disimpan hanya `{Name, Type}` (`connection_service.go:344-347`) dalam
`map[string][]ColumnDef` (`:348`) — key-nya nama tabel tanpa kualifikasi schema, jadi dua tabel
bernama sama di schema berbeda saling menimpa. Yang diminta FR-2 dan belum ada: kualifikasi
`alias.schema.tabel`, PK/FK, nullability, komentar tabel, komentar kolom, dan jumlah baris.
FR-5 (suggested joins) mutlak bergantung pada PK/FK.

### §9.4 Dua landmine Mongo — BENAR keduanya, plus satu yang tidak disebut

- Cap dokumen: `DefaultMongoDocLimit = 10000` (`internal/duckdb/nosql_adapter.go:18-21`), diterapkan
  lewat `findOpts.SetLimit` (:201-204). Agregasi di atas 10k dokumen salah tanpa penanda — §4.2.3 BRD
  benar dan belum ada mekanismenya di engine.
- Cap jumlah collection: `nosql_adapter.go:89` menolak database dengan terlalu banyak collection.
  BRD tidak menyebut ini; FR-10 wajib membawanya ke UI sebagai "sumber ditolak sebagian", bukan error 500.
- Array → string JSON: ini **keputusan sengaja**, bukan bug. Driver Mongo mendekode array sebagai
  `primitive.A`; kalau dibiarkan, kolomnya jadi DuckDB `LIST` yang tipe elemennya bisa berubah antar
  dokumen (skema kolom tidak stabil). Sudah diregresi oleh `TestFlattenMapHandlesMongoDriverTypes`.
  Yang benar-benar kurang untuk FR-4 adalah **lapisan pemakaian**: builder harus menawarkan
  `UNNEST`/`json_extract` pada kolom array, bukan menganggap kolom itu tidak bisa di-group.

## 2. Yang paling berisiko dan tidak disebut BRD: `session_fallback.go` bukan "stub"

`backend/internal/duckdb/session_fallback.go` (`//go:build !cgo`):

- `AttachPostgres` (:42), `AttachMySQL` (:49), `AttachSQLite` (:56) mengembalikan `nil` dan hanya
  menyimpan string DSN ke map `attachments`.
- `Query` (:83) menjawab dari `jsonTables` di memori.

Artinya build desktop yang kehilangan CGO **tidak gagal** — ia melaporkan "connect sukses" ke server
yang tidak pernah disentuh, lalu menampilkan tabel kosong atau data contoh sebagai hasil nyata. Untuk
produk yang janji utamanya "angka ini benar", ini pelanggaran kontrak §4.2.3 di lapisan paling bawah,
dan akan merusak seluruh spike M0 kalau ketahuan di M4.

Tindakan wajib sebelum M0: pada jalur non-CGO, `NewEngine` mengembalikan error; atau build desktop
menolak kompilasi tanpa tag `cgo`. Exit criteria M0 harus berbunyi "aplikasi gagal cepat saat CGO
mati", bukan "aplikasi menampilkan hasil".

## 3. §4.1: Go 1.27 tidak ada

Toolchain terpasang di mesin ini `go1.25.0`. Tulis "Go 1.25+".

## 4. §4.1 "pakai cryptohelper yang ada" untuk passphrase opt-in — sebagian baru

`internal/helper/cryptohelper` adalah AES-256-GCM dengan key dari env var. Yang diminta §8 untuk
bundle (argon2id → AES-256-GCM) tidak ada di kode: KDF-nya baru, beserta penyimpanan salt/nonce di
`secrets.enc`, dan kemungkinan dependensi baru `golang.org/x/crypto/argon2`. Jangan dihitung sebagai reuse.

## 5. §5.3 + keputusan "file source masuk v1" = lubang portabilitas yang belum ada solusinya

Kontrak alias menyatakan bundle membawa **alias + driver** dan tidak membawa host/kredensial, karena
host/kredensial hidup di Profil lokal. Untuk sumber file, yang bersifat mesin-lokal bukan kredensial —
**melainkan path-nya itu sendiri** (`/Users/a/Downloads/sales.csv`). Konsekuensi yang harus ditulis ke
BRD sebelum M3:

1. Path file wajib disimpan relatif terhadap berkas `.muara` (atau terhadap anchor yang dipilih user),
   bukan absolut. Save "relatif" harus jadi keputusan format bundle di M3, bukan detail implementasi di M4.
2. FR-10 pre-flight wajib membedakan tiga nasib: path ada, path tidak ada tapi bisa di-remap manual,
   dan file ada tapi **skemanya beda** (kolom bertambah/berubah tipe) — yang terakhir tidak berlaku untuk
   DB dan tidak punya padanan di desain sekarang.
3. §16 langkah 5 (View jalan tanpa satu karakter SQL diubah) harus diuji dengan file yang diletakkan di
   folder berbeda pada PC B, bukan hanya dengan kredensial berbeda.
4. Perlu batas eksplisit: berapa ukuran file / jumlah file per bundle, karena ZIP `.muara` tidak membawa
   data tapi tree schema dan preview bergantung pada file yang tersedia saat import.

## 6. XLSX bukan fitur gratis di DuckDB

`read_csv_auto`, `read_parquet`, `read_json_auto` tersedia di core DuckDB. Membaca `.xlsx` bukan bagian
dari core pada versi ini dan akan menuntut salah satu dari dua jalur: ekstensi DuckDB terkait (ikut
membuat berkas extension per-OS di CI) atau parse Go-side (mis. `excelize`) lalu daftarkan sebagai tabel.
Keputusan ini harus diambil di M2, bukan discovered di M5. Rekomendasi sementara: v1 CSV + Parquet +
JSON, XLSX sebagai konversi eksplisit "simpan sebagai CSV/Parquet" supaya tidak ada jalur diam-diam.

## 7. §14.1 extension offline: sudah separuh terverifikasi, dan sisanya per-OS

Yang sudah terbukti di produk web: `backend/cmd/duckdb-bundle` menjalankan `INSTALL`+`LOAD` lewat DuckDB
sendiri sehingga subdirektori `<versi>/<platform>` dipilih otomatis; `DUCKDB_EXTENSION_DIRECTORY` →
`SET extension_directory` per sesi; `loadExtension` gagal keras bila `LOAD` gagal; dan probe `--network
none` mengonfirmasi `LOAD` postgres/mysql/sqlite tanpa egress. Jadi risiko "extension butuh internet"
sudah dijawab untuk **linux (amd64/arm64) dan darwin/arm64**.

Yang belum terbukti, dan ini persis risiko Windows yang Anda pilih untuk M0:

- go-duckdb v1.8.5 bundel statis untuk linux/darwin saja; jalur Windows (mingw-w64 vs MSVC) **belum
  pernah diverifikasi di repo ini**. ABI-nya beda; kalau bundel Windows tidak tersedia, desktop butuh
  libduckdb sendiri yang dibangun dengan toolchain yang sama.
- `-extldflags=-Wl,-E` yang dipakai di web agar ekstensi yang di-`dlopen` melihat simbol DuckDB adalah
  flag **ELF/Unix**. Windows tidak punya `dlopen`/`RTLD_GLOBAL`; pemuatan ekstensi di sana lewat mekanisme
  berbeda dan wajib dibuktikan, bukan diasumsikan ikut jalan.
- Berkas extension `.duckdb_extension` dinamai per platform (`osx_arm64`, `linux_amd64`, `windows_amd64`).
  Artinya bundel extension **harus dibangun di CI per-OS** dan tidak boleh di-commit dari satu mesin.

## 8. §4.1 reuse frontend: "port module", bukan "port app"

Yang Next-specific di Phase 4 hanya routing halaman dan API client `fetch /api/*`. Logika visualnya
modul TS murni dan bisa dipakai langsung tanpa perubahan: `frontend/src/lib/echarts-options.ts` (memuat
semua temuan ECharts 6, mis. `outerBoundsMode: 'same'` menggantikan `containLabel` yang deprecated, dan
aturan pemangkasan tanggal ISO), `frontend/src/lib/color.ts` (validasi hex ketat),
`frontend/src/lib/table-theme.ts`, `grid.tsx`, inspector drawer, `chart-palette-fields.tsx`, dan datapabel
(sort per kolom, paginasi, format angka/tanggal). Ini memangkas M5/M6 dan mencegah bug lama lahir kembali.

## 9. §18 mati di mesin baru

`muara-bi-desktop-wireframes.canvas.tsx` tidak ditemukan di root `muara.bi-service`, tidak di
`muara.bi-desktop`, dan tidak di `Downloads`. §5 dan §18 mereferensikannya sebagai salah satu dari dua
berkas yang wajib dibaca. Sebelum M0: ikutkan wireframe ke repo desktop (`muara.bi-desktop/frontend/`
atau `docs/`), kalau tidak, seluruh §11 tidak punya acuan visual saat sesi baru dimulai.

## 10. Ringkasan yang perlu masuk BRD revisi

| # | Aksi | Efek |
|---|---|---|
| 1 | Coret §9.1 (sudah selesai) | M2 lebih kecil |
| 2 | Tambah "engine wajib error saat CGO mati" ke §4.2 + exit criteria M0 | mencegah M0 menghasilkan angka palsu |
| 3 | Perluas §9.3: tabel-level metadata (row count, komentar) + PK/FK | FR-2 dan FR-5 bisa dikerjakan |
| 4 | Tulis aturan path relatif + remap file + skema-file-bedah ke §5.3/FR-10/§16 | v1 file-source tidak merusak janji ekspor/impor |
| 5 | Pecah M2: CSV/Parquet/JSON dulu, keputusan XLSX eksplisit | tidak ada jalur diam-diam |
| 6 | Ganti §14.1 jadi "CI per-OS"; Windows = 3 pertanyaan di M0 | risiko nyata Windows terukur |
| 7 | §4 "port module, bukan port app" + daftar file | M5/M6 lebih murah |
| 8 | Koreksi Go 1.27 → 1.25+; pindahkan argon2id dari "reuse" ke "baru" | estimasi jujur |
| 9 | Masukkan wireframe ke repo ini | §18 berlaku lagi |
