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
- **Fase aktif:** **M1 berjalan.** M1.1 (peta paket engine) dan M1.2 (CGO fail-fast) selesai; M1.3
  (`git mv` ke modul `muara.bi/engine`) berikutnya. Belum ada `go.mod` desktop, belum ada skeleton
  Wails — keduanya memang di belakang M1.
- **Capaian sesi ini:** revisi BRD v2 dirampungkan dan diselaraskan dengan keputusan "View boleh lintas
  project" (BRD §5.2/FR-9/FR-10/FR-11/§13 langkah 8/§15, PLAN M3, wireframe S4a) → **M1.1** pengukuran
  batas engine dengan `go list` ([docs/engine-map.md](docs/engine-map.md)) → **M1.2** CGO fail-fast di
  engine web + dua tes baru, suite hijau dua-duanya (`CGO_ENABLED=1` dan `=0`) → **BRD §5.7 baru**
  ("Tiga jalur transport": langsung · VPN · bastion-sebagai-forwarder-lokal) lahir dari pertanyaan user
  "host-nya privat tapi laptop sudah connect VPN, apakah muara.bi masih bisa connect?" — jawabannya ya,
  karena yang menghubungi database adalah proses di laptop, ikut routing & DNS VPN, sama seperti DataGrip.
  Tiga blocker ikut ditemukan dan masuk BRD §10 butir 7-9.
- **Keputusan user tambahan (2026-10-30, sudah masuk BRD v2):**
  - `Project` membungkus **Dashboard**; `View` tidak punya `project_id` dan **boleh direferensi dashboard dari
    project mana pun** (revisi 2026-10-31: keanggotaan View = pin/referensi, bukan kepemilikan, supaya query
    yang sama tidak perlu digandakan per klien). Connection/alias/semantic model tetap berlaku satu berkas.
  - Berkas koneksi saat export dipecah **tiga tingkat**: selalu ikut (alias/driver/database) · ikut dengan
    toggle (host/port/user/SSH) · tidak pernah ikut default (password/token → keychain per mesin).
    Ini memperbaiki v1 yang mengeluarkan host dan membuat "share ke rekan satu server" menyakitkan.
  - Ekspor hasil ke CSV/XLSX/JSON/Parquet = **reuse** `internal/exporter/` (`WriteXLSX` sudah ada, excelize
    streaming, pure Go) dan **tidak mewarisi LIMIT tampilan**.
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
    (`session_preparer.go:193-202`) memakai `cfg.Host` mentah → di belakang bastion, UI hijau tapi query gagal.
    DuckDB tidak menerima `net.Conn`, jadi solusinya forwarder TCP lokal + DSN `127.0.0.1:<port>`.
  - Bug produk web, **direproduksi** (bukan dugaan): `connection_service.go:322-326` menghasilkan
    `... sslmode= sslmode=disable` saat `ssl_mode` kosong; `pgx.ParseConfig` menolaknya dengan
    `cannot parse ...: failed to configure TLS (sslmode is invalid)` → SyncSchema Postgres gagal total untuk
    koneksi tanpa `ssl_mode`. `session_preparer.go:196-199` memberi default `disable`, jadi dua jalur
    berbeda perilaku. Perbaikan masuk M2 (satu tempat, plus tes).
  - Belum ada tempat untuk CA privat (`rootcert`); default `sslmode=disable` salah untuk DB internal
    self-signed. Masuk FR-1 + blocker §10 butir 8.

## 4. Task berikutnya (urutan)

1. **M1.3 `git mv`** engine keluar `internal/` ke modul `muara.bi/engine` dalam **satu commit** di repo
   web: 6 paket (§1 engine-map) + `service/widget_builder.go` + `domain/entity/widget_builder.go`, lalu
   ganti import di 6 titik konsumen. Gerbang: `go build/vet/test ./...` hijau di web.
   **Aturan yang harus ikut ditulis:** modul engine dilarang mengimpor `muara.bi/core/pkg/...`
   (customerror/logging) — dependensi balik menutup siklus; dan `session_preparer.go` **tidak** pindah.
2. **M1.4** `QueryResult.truncated` + alasannya sebagai kontrak engine (BRD §4.2.3), sekaligus
   `max_memory` jadi konfigurasi (sekarang hardcoded 2 GB di `session_cgo.go:38`).
3. Baru setelah itu: spike M0 (Wails + CI per-OS).

## 5. Kendala & catatan lingkungan

- Mesin ini: macOS arm64, Go 1.25.0, Docker Desktop VM 4 GB. DuckDB linux/darwin sudah terverifikasi
  di produk web; **Windows belum pernah disentuh sama sekali** — M0 adalah pertama kalinya.
- BRD §18 menyuruh membaca `muara-bi-desktop-wireframes.canvas.tsx`, tapi file itu tidak ada di
  repo web, di repo ini, maupun di `Downloads`. Sebagai gantinya sudah dibuat wireframe sendiri:
  [docs/wireframe.html](docs/wireframe.html) — 10 layar §12.3, token warna §12.2, toggle dark mode,
  dibuka langsung di browser. Disetujui dan sudah diselaraskan dengan keputusan View lintas project
  (layar S4a: "Pin ke proyek lain", bukan duplikasi).
- Keputusan user 2026-10-30: **opsi A disetujui** — repo web boleh disentuh satu commit untuk
  `git mv` engine + ganti import path (M1.3).
- Repo ini sudah `git init` tapi belum ada commit dan belum ada remote.
