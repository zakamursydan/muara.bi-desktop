# BRD — muara.bi Desktop (Offline Edition) · v2

Status: **revisi v2 · 2026-10-30** · Menggantikan `DESKTOP-BRD.md` v1 (`~/Downloads`, tetap sebagai arsip).
Dokumen ini self-contained: cukup dibaca bersama [PLAN.md](PLAN.md) untuk melanjutkan desain & implementasi
di mesin/sesi lain tanpa riwayat percakapan. Wireframe: [docs/wireframe.html](docs/wireframe.html)
(10 layar, toggle dark mode, dibuka langsung di browser).

Koreksi teknis yang mendasari revisi ini diverifikasi terhadap `muara.bi-service` branch `development`
(suite `go build/vet/test ./...` exit 0) — rinciannya di [docs/brd-review.md](docs/brd-review.md).

## Apa yang berubah dari v1

| Bagian | Perubahan | Alasan |
|---|---|---|
| §9 blocker | **§9.1 dicoret (sudah selesai)**; §9.2/9.3/9.4 dikonfirmasi dengan file:line | Mongo sudah ter-attach di jalur query (`session_preparer.go:208`) |
| §4.2 kontrak | **+2 kontrak wajib**: engine harus gagal keras tanpa CGO; penanda "hasil terpotong" bagian dari result set | `session_fallback.go` membuat build non-CGO **berhasil palsu** — **kedua kontrak sudah ditegakkan di engine 2026-10-31**, lihat §10 butir 5 & 7.3 |
| §4 stack | Go **1.25+** (bukan 1.27); UI = **port module, bukan port app** | toolchain nyata; logika visual Phase 4 sudah TS murni |
| §5.3/5.4 bundle | Kredensial vs **alamat server dipisah jadi tiga tingkat** | v1 mengeluarkan host, yang merusak skenario "share ke rekan satu server" |
| §6 model | **Entitas `Project` ditambahkan** — hanya membungkus Dashboard, keanggotaan View berupa referensi (bukan kepemilikan) | keputusan user 2026-10-30/31 |
| §3 cakupan | Sumber **file (CSV/Parquet/JSON) masuk v1**; XLSX ekspor reuse, impor XLSX ditunda | keputusan user; DuckDB core membaca CSV/Parquet/JSON tanpa extension |
| §5.3 + FR-10 | Berkas `.muara` wajib menyimpan **path file relatif** + pre-flight remap path & deteksi skema file berubah | konsekuensi langsung dari sumber file di v1 |
| §14 roadmap | **M1 (modul engine) mendahului M0 (spike platform)**; Windows masuk M0 dengan 3 pertanyaan terbuka | aturan `internal/` Go memaksa; spike tanpa ekstraksi = memaksa menyalin engine |
| §14.1 | Bundel extension **dibangun di CI per-OS**, tidak di-commit dari satu mesin | folder extension per-platform (`osx_arm64`, `linux_amd64`, `windows_amd64`) |
| §8 | argon2id **kode baru**, bukan reuse cryptohelper | cryptohelper hanya AES-256-GCM dengan key dari env |
| §18 | acuan wireframe v1 (`*.canvas.tsx`) tidak ada di mesin mana pun → digantikan `docs/wireframe.html` | §18 v1 patah di mesin baru |
| §5.7 **baru** | Transport ke sumber data dinyatakan eksplisit: langsung · VPN · bastion-sebagai-forwarder-lokal, plus `rootcert` dan pesan kegagalan tiga-sebab | diverifikasi 2026-10-31 dari kode: bastion hanya dipakai Test Connection, jalur query melewatinya — tanpa bab ini skenario "DB privat + laptop sudah VPN" terlihat didukung padahal hijau palsu |

## 1. Visi & posisi

muara.bi Desktop adalah aplikasi BI & analitik **offline, single-user, berbasis berkas**:

1. Connect ke sumber data (Postgres / MySQL / SQLite / MongoDB / file) semudah DataGrip — tanpa signup, tanpa server.
2. Menyusun query, table view, dan chart lewat UI yang ramah pemula maupun pengguna SQL.
3. **Meng-export seluruh settingan** (koneksi, view, dashboard, semantic model) ke satu berkas `.muara`, lalu
   **meng-import-nya di PC lain** sehingga orang lain mendapat dashboard dan koneksi yang sama.

Bukan port dari muara.bi web. Web adalah produk **multi-tenant** (workspace, role, RLS, share link, alert
terjadwal) dan tetap berjalan sebagai entitas bisnis terpisah. Desktop adalah produk **satu operator di satu
laptop**. Keduanya memakai **satu engine analitik** yang sama — dipakai ulang sebagai modul Go, tidak disalin.

Nilai jual yang tidak dimiliki kompetitor gratis: federasi lintas sumber dalam satu kueri (Postgres ⋈ MongoDB
⋈ CSV), dan AI yang **hanya membaca metadata** sehingga tidak ada satu baris data pun yang keluar dari laptop.

## 2. Persona

- **Analis pemula** — tidak bisa SQL. Butuh visual builder, template, ringkasan bahasa manusia.
- **Analis/insinyur mahir** — menulis SQL sendiri. Butuh schema tree, autocomplete, result grid, ekspor.
- **Lead / konsultan** — membuat bundle sekali, membagikannya ke rekan atau klien yang meng-import di PC masing-masing.

## 3. Cakupan

**In scope v1:** connect multi-driver (Postgres, MySQL, SQLite, MongoDB, **CSV/Parquet/JSON**), schema sync + tree,
query editor, visual builder, suggested joins, template, table view & chart, **Project** sebagai pengelompok,
dashboard canvas, semantic model, export/import bundle, profil lokal + keychain, alert sederhana (hanya saat
aplikasi terbuka), dark mode, ekspor hasil ke CSV/XLSX/JSON/Parquet.

**Out of scope v1:** multi-user / RBAC / RLS antar-user, share link publik, embed ber-signature, kolaborasi
realtime, mobile, plugin marketplace, **impor `.xlsx` sebagai sumber data**, email alert (butuh SMTP).

**Future:** impor dari muara.bi web, AI co-pilot penuh, scheduled digest, sumber `.xlsx`, kolaborasi lewat git/bundle.

## 4. Platform & teknologi

- **Shell: Wails v3** — engine Go dipakai apa adanya, hanya lapisan HTTP chi yang diganti bindings. Electron/Tauri
  tetap bisa memakai Go lewat sidecar (bukan "rewrite"), jadi alasan memilih Wails adalah **satu proses tanpa
  sidecar**, bukan ketidakmungkinan opsi lain. Risiko: v3 baru stabil — kunci versi di tag, baca `LICENSE` pada
  tag tersebut, dan spike dulu (M0).
- **Wajib CGO.** Jalur `!cgo` (`internal/duckdb/session_fallback.go`) hari ini mengembalikan `nil` untuk semua
  `Attach*` (:42/:49/:56) dan menjawab `Query` dari memori (:83) — artinya build tanpa CGO **tampak berhasil**.
  Kontrak baru: engine non-CGO harus mengembalikan error, dan build desktop menolak kompilasi tanpa tag `cgo`.
- **Modul engine bersama** (`muara.bi/engine`, hasil M1): `duckdb/` (sesi CGO, guardrails, semantic layer,
  `StreamMongoCollections`), `service/widget_builder.go` (compiler BuilderState → SQL), `exporter/`
  (CSV/JSON/XLSX streaming — `WriteXLSX` memakai excelize v2.11.0), `helper/cryptohelper` (AES-256-GCM),
  `helper/sqlident`, `domain/entity` (termasuk `alert_recipient`), `ai/pruner.go` (kontrak `TableSchema`),
  `worker/notifier.go` (kanal Slack/Telegram/Webhook). Dev lokal ditautkan dengan `go.work`.
- **UI: port module, bukan port app.** Yang Next-specific hanya routing halaman dan API client `fetch /api/*`.
  Modul ini dipakai ulang langsung dari `muara.bi-service/frontend/src/`: `lib/echarts-options.ts` (tick `315K`,
  area gradasi, donut tengah, `outerBoundsMode:'same'` untuk ECharts 6, pemangkasan tanggal ISO), `lib/color.ts`
  (validasi hex ketat), `lib/table-theme.ts`, `grid.tsx`, inspector drawer, `chart-palette-fields.tsx`, datapabel.
- **Dibuang dari stack server:** Postgres metadata, Redis, Asynq. Gantinya: pool goroutine in-process untuk job
  async, LRU in-memory + SQLite lokal untuk cache/history (SQLite via `modernc.org/sqlite`, pure Go).

Stack UI: React 19 + TypeScript + Vite · Tailwind v4 + shadcn/ui dengan token CSS · lucide-react · ECharts ·
Monaco (completion ke schema cache baru) · react-grid-layout · Zustand · TanStack Table + Virtual (10.000 baris).

## 5. Keputusan arsitektur (mengikat)

### 5.1 Berkas bundle = database
Tidak ada lapisan metadata untuk settingan. Satu `.muara` (zip berisi JSON) adalah satu-satunya penyimpanan:
**Export = Save As, Import = Open.** Tidak ada serializer terpisah yang bisa drift dari skema. Yang append-only
dan besar (query history, log run alert, cache hasil) tinggal di SQLite terpisah yang **tidak ikut ter-export**.
Aplikasi selalu bekerja pada satu berkas terbuka; "mulai kosong" membuat `Untitled.muara`.

### 5.2 View dan Project
`View` adalah unit settingan terkecil yang bisa di-export. `Dashboard` hanyalah layout yang mereferensi ID View.
**`Project`** adalah lapisan organisasi di atasnya, dan **keanggotaannya berupa referensi, bukan kepemilikan**:

- `View` hidup **satu berkas** dan boleh direferensi dashboard dari project mana pun. Tidak ada `project_id` di View.
  Kalau View dimiliki satu project, query yang sama harus digandakan tiap kali klien lain minta dashboard serupa —
  itu pekerjaan duplikasi yang justru dihindari produk ini.
- `Project` menyimpan daftar referensi: `dashboard_refs[]` dan `view_pins[]` (View yang disematkan di library proyek
  tanpa lewat dashboard). Sumber kebenaran tunggalnya ada di project, bukan dua-duanya.
- Connection, alias, semantic model, dan Profil lokal berlaku **satu berkas** dan tidak terkait project.
- Konsekuensi yang harus dinyatakan ke user: mengubah host sebuah alias di Profil lokal mengubah **semua** project
  yang memakainya. Menghapus project **tidak** menghapus View yang dipin-nya — hanya dashboard dan pin milik project itu.
- Konsekuensi untuk import (§FR-11): View yang sama bisa datang dua kali lewat dua bundle berbeda. Identitasnya
  dicek dari ID View + hash isi: sama persis → lewati; ID sama tapi isi beda → **tanyakan**, jangan diam-diam menimpa.

### 5.3 Alias = kontrak portabilitas
Query hanya menyebut alias katalog (`sales.orders`), tidak pernah `connection_id`, host, atau DSN. Ini yang
membuat export/import bekerja, dan sudah benar di engine lama (tiap koneksi di-attach sebagai katalog bernama
`alias_name`). Konsekuensi positif: dialek SQL selalu DuckDB terlepas sumbernya, jadi View tetap jalan walau
sumbernya beda engine.

### 5.4 Tiga tingkat ikut-tidaknya koneksi saat export
Menggantikan aturan v1 ("hanya alias + driver, tanpa host") yang merusak skenario berbagi dengan rekan satu server.

| Bagian | Ikut bundle | Catatan |
|---|---|---|
| alias, driver, nama database, filter skema, catatan | **selalu** | tanpa ini SQL tidak resolve |
| host, port, user, target SSH, sslmode | **ikut**, toggle "sertakan alamat server" (default nyala) | bukan secret, tapi informasi infrastruktur — wajib bisa dimatikan untuk bundle yang dikirim ke luar tim |
| password, token, kunci | **tidak pernah** secara default | hidup di keychain mesin masing-masing; hanya lewat `secrets.enc` ber-passphrase bila user memilihnya |

Alur import yang jadi target: pre-flight menampilkan koneksi yang alamatnya sudah lengkap dan **hanya meminta
password diisi sekali**; setelah itu semua View jalan. Untuk rekan dengan server sendiri, pre-flight menampilkan
bentrok alias dan user mengikat alias ke koneksinya sendiri. Bila Anda tidak ingin mengirim koneksi sama sekali,
pilih cakupan "hanya View & Dashboard" — resolver dependensi akan menyatakan "butuh alias `sales` yang belum ada".

Kredensial ber-passphrase tetap tersedia tapi **tidak disarankan**: kunci yang dikirim lewat chat tidak bisa
ditarik kembali, dan rotasi jadi milik bersama.

### 5.5 Path file relatif
Karena sumber file masuk v1, yang bersifat mesin-lokal bukan kredensial melainkan **path-nya**. Bentuk kanonik di
bundle adalah path **relatif terhadap berkas `.muara`**; resolver Profil lokal menangani remap. Pre-flight wajib
membedakan tiga nasib: path ada · path tidak ada (bisa ditunjuk ulang) · file ada tapi **skemanya berubah**
(kolom bertambah/tipe berubah). Batas jumlah & ukuran file per bundle ditetapkan di M3 dari angka nyata.

### 5.6 Tanpa drag-and-drop untuk menyusun query
Membangun query = tugas seleksi; klik/checkbox lebih cepat dan hambatan pemula itu konseptual, bukan mekanis.
DnD hanya untuk: kanvas dashboard (posisi/ukuran kartu) dan urutan kolom table view.

### 5.7 Tiga jalur transport ke sumber data (langsung · VPN · bastion)

Sumber data hampir selalu beralamat privat. Di sinilah desktop punya keunggulan yang tidak bisa dibeli
versi web: **yang membuka koneksi adalah proses di laptop analis**, jadi ia ikut routing table dan DNS yang
di-setup VPN — mekanisme yang sama yang dipakai DataGrip. Tidak ada port publik, tidak ada ingress. Yang di UI
harus selalu terlihat: jalur mana yang sedang dipakai.

| Jalur | Cara kerja | Syarat | Ikut bundle? |
|---|---|---|---|
| **Langsung** | DSN ke host publik / satu LAN | — | ya, host portabel |
| **VPN** | IP privat atau nama internal, di-dial proses aplikasi | route VPN terpasang; **nama internal butuh split-DNS** dari VPN, tanpa itu pakai IP atau entri hosts | host tetap ikut (tingkat 2 §5.4) — tapi hanya berarti bagi orang di VPN yang sama |
| **Bastion** | **forwarder TCP lokal**: aplikasi listen `127.0.0.1:port`, salurkan ke `host:port` target lewat SSH | bastion reachable + kunci/password | yang masuk bundle = **target asli** + referensi bastion; `127.0.0.1:port` adalah detail Profil lokal mesin ini |

Empat aturan mengikat:

1. **Test Connection wajib menembus jalur yang sama dengan jalur query.** Di edisi web hari ini bastion hanya
   di-dial pada Test Connection (`ConnectionService.TestConnection`), sementara `SyncSchema` membuka pgx dengan DSN
   polos dan `attachConnection` mengirim `cfg.Host` apa adanya ke DuckDB (keduanya lewat
   `service.postgresDSN`, `internal/service/session_preparer.go`).
   Hasilnya: "koneksi sukses, 12 ms" lalu seluruh query gagal. Desktop tidak boleh mewarisi bentuk ini.
2. **DuckDB tidak menerima `net.Conn`** — scanner postgres/mysql membuat koneksinya sendiri di sisi C dari string
   DSN. Jadi bastion **tidak bisa** dipakai ala pgx (`WithDialFunc`); implementasinya harus forwarder dengan DSN
   menunjuk ke `127.0.0.1:<port>`, dan umur forwarder = umur sesi.
3. **CA privat.** Database di jaringan internal umumnya bersertifikat self-signed, sehingga `sslmode` saja tidak
   cukup: dibutuhkan `rootcert` (path, ikut aturan relatif §5.5) dan alur "percayai sertifikat ini". Hari ini DSN
   hanya merakit `sslmode` dan default-nya `disable` (`service.postgresDSN` di web); tidak ada tempat menaruh CA.
4. **Pesan kegagalan harus membedakan tiga sebab** yang tindakannya beda: *host tidak terjangkau* (belum VPN /
   firewall), *nama tidak teresolusi* (DNS tidak dikirim VPN — jangan tulis "connection refused"), dan
   *kredensial ditolak / TLS gagal*. Pre-flight import (§FR-11) memakai pesan yang sama: "10.20.3.4:5432 tidak
   terjangkau — sedang VPN?" bukan "password salah".

Satu hal yang belum terukur dan jadi catatan M0: scanner DuckDB dapat membuka **lebih dari satu koneksi** per
query paralel (paralelisme diatur `threads`, `session_cgo.go:39`). Lewat VPN itu berarti latensi berlipat dan
batas `max_connections` server jadi terlihat — angka nyatanya dicari di spike, bukan dikira-kira di sini.

## 6. Model data & format berkas

**Entitas.** `Project` (id, nama, warna/label, catatan, **dashboard_refs[]**, **view_pins[]**) · `Connection` (id,
alias unik, driver, definisi per-driver, ssh_config, catatan — tanpa secret) · `Profile` (lokal, tidak di-export:
pemetaan alias → host & kredensial mesin ini) · `SemanticModel` (id, nama, deskripsi, raw_sql, alias dependensi) ·
`View` (id, nama, folder, kind `table|line|bar|pie|kpi|scatter|area`, query_mode `SYNCED|CUSTOM_SQL`,
builder_state, raw_sql, presentation, max_rows, cache_ttl, alias dependensi) · `Dashboard` (id, nama, layout,
referensi View, global_filters, refresh_interval) · `Alert` (id, referensi View, threshold, kanal, jadwal).

```
manifest.json        { format:"muara.bundle", version:1, min_app_version, created_at, exported_by, app }
projects.json        [ Project ]
connections.json     [ Connection ]            // tingkat §5.4; tanpa secret kecuali secrets.enc
semantic_models.json [ SemanticModel ]
views.json           [ View ]
dashboards.json      [ Dashboard ]
alerts.json          [ Alert ]
secrets.enc          (opsional) blob AES-256-GCM + salt/nonce, KDF argon2id — KODE BARU, bukan reuse cryptohelper
```

Import membaca `manifest.json` lebih dulu; versi format atau `min_app_version` yang tidak dikenal → **tolak dengan
pesan jelas**, bukan error misterius. Bidang `origin_workspace_id`/`origin_project_id` di manifest berupa **string
opaque** (asal bundle, bukan keanggotaan) supaya bundle kelak bisa diimport ke edisi web tanpa mengasumsikan semantik
tenant — `Project` desktop dan `Project` web tidak wajib berarti sama.

## 7. Kontrak wajib (MUST)

Kelima hal ini tidak boleh hilang saat kode baru ditulis — di v1 statusnya emergent, di sini tersurat:

1. **Alias.** Query hanya menyebut `alias.tabel.kolom`. Tidak ada `connection_id`/host/DSN di dalam View.
2. **Guardrail.** Parameter selalu terikat `$n` tanpa konkatenasi nilai user; penolakan DDL/DML; auto-LIMIT; timeout eksekusi.
3. **Penanda pemotongan.** `QueryResult` membawa `truncated` + alasannya (LIMIT disuntik · cap sumber · penolakan
   sebagian). UI wajib menandainya. Berlaku untuk agregasi Mongo di atas 10.000 dokumen dan untuk sumber file parsial.
   **Ditegakkan di engine sejak 2026-10-31 (M1.4):** `duckdb.QueryResult` kini punya `row_limit`, `truncated`,
   `truncated_by` (`row_limit` · `source_cap`) dan `capped_sources`. Deteksinya bukan tebakan: statement dijalankan
   dengan LIMIT = cap+1 sebagai baris penanda; baris ekstra dibuang, dan keberadaannya adalah bukti bahwa jawaban
   dipotong. Satu set yang pas penuh di bawah cap **tidak** ditandai. Cap dokumen Mongo dilaporkan per view
   (`MongoMount.Truncated` → `CappedMongoViews`). Web sudah memancarkan field ini lewat `ExecuteQueryResponse`;
   penanda di UI tetap masuk M2.
4. **Gagal keras tanpa CGO.** Engine tidak boleh menjawab hasil dari memori saat DuckDB asli tidak ter-link.
5. **AI metadata-only.** AI hanya membaca DDL/nama kolom/tipe/FK; tidak pernah membaca baris data; tidak pernah
   lookup kredensial. Output AI berupa **BuilderState**, bukan SQL mentah, sehingga tetap inspectable dan lewat
   compiler + guardrail yang sama.

## 8. Kebutuhan fungsional

**FR-1 Connect.** Wizard per-driver termasuk sumber file. Alias wajib & unik dengan penjelasan inline bahwa alias
adalah nama katalog DuckDB. Test Connection (latency + versi server; Mongo di-ping dengan driver, bukan TCP dial)
**harus lewat transport yang sama dengan jalur query** (§5.7) — di web, bastion hanya diuji saat Test Connection
sehingga hasilnya bisa hijau palsu. Pilih transport eksplisit di form: Langsung / VPN / Bastion, dengan status
forwarder (`127.0.0.1:5432 → db.internal:5432`) terlihat. TLS: `sslmode` **dan** `rootcert` untuk CA internal.
Pesan kegagalan terbagi tiga: host tidak terjangkau · nama tidak teresolusi · kredensial/TLS ditolak.
Setelah sukses: auto schema-sync dengan progress per tabel dan penanda kegagalan eksplisit.

**FR-2 Schema sync & tree.** Sync untuk **semua** driver (hari ini hanya Postgres: `connection_service.go:321`,
sisanya `return nil` di :367). Tree `alias → schema → tabel → kolom` dengan jumlah baris, komentar, PK/FK,
nullability (hari ini hanya `{Name, Type}` di `connection_service.go:344`). Dua afordansi per kolom: klik nama =
sisip nama terkualifikasi di kursor; centang = masuk builder. Search fuzzy lintas alias. Status per alias
(ok/gagal/basi) selalu terlihat.

**FR-3 Query editor.** Monaco dialek DuckDB, autocomplete sadar `alias.tabel.kolom`. Run / EXPLAIN / Format.
Result grid: sort, pin kolom, salin CSV. Status bar wajib menampilkan jumlah baris, durasi, **apakah LIMIT disuntik**,
dan **apakah hasil terpotong**.

**FR-4 Visual builder (jalur pemula).** Stepper vertikal 4 langkah (Sumber → Field → Filter → Urutkan & batasi);
langkah 3–4 opsional. Klasifikasi otomatis dimension/measure dengan agregasi default yang bisa diganti (chip).
Input filter menyesuaikan tipe kolom. **Preview live 10 baris + SQL read-only + satu kalimat ringkasan bahasa
manusia**, debounced ≤300 ms. Kolom array Mongo (disimpan sebagai string JSON supaya tipe kolomnya stabil) harus
menawarkan `UNNEST`/`json_extract` di UI, kalau tidak ia tidak bisa di-group.

**FR-5 Suggested joins.** Kandidat dari FK terdeklarasi + inferensi nama & tipe lintas alias, dengan alasan
keyakinan dan persentase irisan nilai. Satu klik menyisipkan JOIN (INNER/LEFT). Join lintas alias diberi peringatan
performa ("DuckDB menarik data ke memori laptop ini").

**FR-6 Template.** "Top N per kategori", "Tren waktu", "Distribusi", "Perbandingan periode" → builder terisi awal.

**FR-7 Table view & chart presentation.** Inspector: urutan kolom (DnD), visibilitas, lebar, perataan, format
angka/tanggal, conditional formatting; chart: tipe, mapping x/y/series, override ECharts, palet (hex divalidasi
ketat sebelum masuk CSS/canvas). Tersimpan di `View.presentation` → ikut ter-export.

**FR-8 Dashboard canvas.** Grid 12 kolom; DnD posisi/ukuran; "Tambah kartu" memilih **View yang sudah ada**;
filter global mengikat `{{param}}` lintas kartu; refresh interval; TTL per kartu; mode Present fullscreen.

**FR-9 Proyek & daftar dashboard.** Dashboard dikelompokkan per `Project`: filter proyek, kartu dashboard dengan
pratinjau + jumlah View + penanda View merah, baris agregat proyek (View / semantic model / alert), daftar alias
yang dipakai proyek, "Export proyek ini saja", dan **"Pin ke proyek lain"** — menambah referensi, bukan menyalin
View. Proyek yang berbagi alias **atau View** dengan proyek lain harus menampilkannya; menghapus proyek tidak
menghapus View yang dipin-nya.

**FR-10 Export.** Pilih cakupan (View / Folder / Dashboard / Proyek / Semua) → daftar **dependensi transitif**
(semantic model, View lain, alias). Cakupan "Proyek" membawa View yang *direferensi* proyek itu, bukan View yang
*dimiliki* — proyek lain yang mempin View yang sama tidak ikut terbawa. Bagian koneksi mengikuti tiga tingkat §5.4;
bagian file mengikuti §5.5. Output `.muara`.

**FR-11 Import dengan laporan pre-flight.** Sebelum menulis apa pun: bentrok alias (pakai yang ada / ganti nama /
ikat ke koneksi lain); validasi skema per View (tabel/kolom tak resolve = merah, tipe tak cocok = kuning);
peringatan beda driver; permintaan password untuk koneksi tanpa secret; **path file hilang / skema file berubah**;
**View duplikat** (ID sama + hash sama → lewati; ID sama + hash beda → tanyakan, jangan diam-diam menimpa; View
baru → tambah, lalu tanyakan proyek mana yang boleh mempin-nya). Koneksi yang tidak terhubung diberi sebab yang
benar (§5.7): *host tidak terjangkau — sedang VPN?* bukan *password salah*.
**Tidak ada import sebagian** — batal berarti tidak ada yang berubah.

**FR-12 Ekspor hasil.** CSV / XLSX / JSON / Parquet dari result grid, kartu datapabel, atau seluruh dashboard
(satu worksheet per tabel) — reuse `internal/exporter/`. **Jalur ekspor tidak mewarisi LIMIT tampilan** — menulis ke
disk user adalah kasus ketika batas baris tidak diperlukan; tetap tampilkan peringatan bila sumbernya yang
membatasi (mis. cap Mongo).

**FR-13 Alert.** Threshold pada View, kanal Telegram/Slack/Webhook (reuse `Notifier`), jadwal sederhana. Jujur di
UI: alert hanya jalan saat aplikasi terbuka. Email menyusul (belum ada SMTP sender di mana pun).

**FR-14 AI assist (fallback, bukan pintu depan).** "Jelaskan yang kamu mau" → **BuilderState**, bukan SQL mentah.
Kontrak §7.5 mengikat. BYOK: API key user sendiri, disimpan di keychain.

**FR-15 Settings.** Profil lokal (pemetaan alias → host + kredensial, status keychain terlihat), penyedia AI,
kanal alert, **batas memori DuckDB per sesi** (engine menerima `GuardrailConfig.MaxMemory` sejak M1.5 2026-10-31
dan, sejak 2026-11-01, juga unit biner — jadi slider UI menulis `GiB` dan string itu dikirim apa adanya; lihat §15
butir 6), jumlah kueri paralel, batas baris guardrail, tampilan (dark mode).

## 9. Kebutuhan non-fungsional & keamanan

- Kredensial di **OS keychain** (`zalando/go-keyring`: Windows Credential Manager / macOS Keychain / libsecret),
  bukan berkas terenkripsi yang berdekatan dengan datanya. Tidak pernah masuk `.muara` kecuali opt-in §5.4.
- Startup < 2 dtk untuk bundle ≤ 50 View; preview builder debounced ≤ 300 ms.
- Semua teks UI bahasa Indonesia dulu, struktur siap i18n.
- Berkas **third-party notices** ikut installer: Wails MIT, DuckDB MIT, go-duckdb MIT, excelize BSD-3 — semuanya
  permissive, syaratnya notis hak cipta didistribusikan. Penamaan produk mematuhi pedoman merek DuckDB
  ("ditenagai DuckDB", bukan nama produk DuckDB).

## 10. Blocker teknis — status terverifikasi

| # | Blocker | Status |
|---|---|---|
| 1 | Mongo tidak ter-attach di jalur query | **SELESAI** — `attachConnection` (web, `internal/service/session_preparer.go`) memanggil `StreamMongoCollections` |
| 2 | `SyncSchema` hanya Postgres | **BUKA** — `connection_service.go:321`, silent `return nil` di :367 |
| 3 | Schema cache miskin (tanpa kualifikasi schema, PK/FK, nullability, komentar, jumlah baris) | **BUKA** — `ColumnDef` = `{Name, Type}` di `connection_service.go:344` |
| 4 | Cap Mongo 10.000 dokumen & batas jumlah collection tanpa penanda | **SELESAI di engine 2026-10-31** — `StreamMongoCollections` mengembalikan `[]MongoMount` (jumlah dokumen + `Truncated`), `QueryResult.TruncatedBy = source_cap` membawa daftar view yang terpotong; `MaxMongoCollections` tetap gagal keras (bukan hasil parsial). **Yang tersisa cuma penanda di UI** → M2 |
| 5 | Build non-CGO mengembalikan hasil palsu | **SELESAI 2026-10-31** — `NewEngine` mengembalikan `duckdb.ErrCGODisabled`, stub tidak bisa menghasilkan `QueryResult` lagi; dibuktikan `cgo_disabled_test.go` + `cgo_enabled_test.go` (kontrak §7.4 kini ditegakkan di engine) |
| 6 | `max_memory` DuckDB hardcoded 2 GB/sesi | **SELESAI di engine** — `GuardrailConfig.MaxMemory` + `NormalizeMaxMemory` (gagal di startup kalau bukan angka+unit). Sejak 2026-11-01 engine menerima `KiB/MiB/GiB/TiB` dan **default-nya `2GiB` di kedua produk** (web `DUCKDB_MAX_MEMORY`, desktop dari profil). §15 butir 6 menjawab default desktop: `min(4GiB, RAM/4)`. **Yang tersisa cuma UI-nya** → FR-15 + M6 |
| 7 | Bastion hanya dipakai Test Connection — jalur query & schema-sync tidak lewat tunnel | **BUKA, temuan 2026-10-31** — `TestConnection` mende-dial bastion, sedangkan `SyncSchema` dan `attachConnection` memakai `cfg.Host` lewat `service.postgresDSN` → §5.7 aturan 1 & 2. Untuk DuckDB tidak ada jalan lain selain forwarder TCP lokal |
| 8 | Tidak ada tempat untuk CA privat (`rootcert`); `sslmode` default `disable` | **BUKA, temuan 2026-10-31** — perakit DSN kini satu tempat (`service.postgresDSN`, lihat butir 9) tapi hanya merakit `sslmode` dengan default `disable`; tidak ada field CA. → §5.7 aturan 3 |
| 9 | `SyncSchema` Postgres selalu gagal kalau `ssl_mode` kosong | **SELESAI 2026-10-31 (di produk web, desktop ikut sehat)** — `SyncSchema` dulu menghasilkan `... sslmode= sslmode=disable` dan `pgx.ParseConfig` menolaknya (`cannot parse ...: failed to configure TLS (sslmode is invalid)`). Keduanya kini memakai `service.postgresDSN(cfg)`: satu default `disable`, nilai user di-quote per aturan libpq (`dsnValue`) sehingga kredensial berspasi maupun yang menyamar jadi parameter lain tidak bisa menembus DSN. Enam tes di `session_preparer_test.go`, salah satunya membuktikan bentuk DSN lama memang ditolak. **`ssl_mode` juga divalidasi di titik tulis (2026-10-31):** `normalizeSSLMode` menolak nilai di luar enam mode libpq, `UpsertEnvironment` menjawab 400 sebelum menyimpan, dan handler `Create`/`UpsertEnvironment`/`SyncSchema` ikut dibetulkan supaya error validasi tidak lagi dipaksa jadi 500 |

## 11. Roadmap

Urutan disengaja: M1 memaksa kontrak engine ditetapkan selagi masih murah, lalu M0 memvalidasi asumsi termahal
sebelum satu fitur pun ditulis. Exit criteria lengkap ada di [PLAN.md](PLAN.md).

| Milestone | Isi | Exit criteria |
|---|---|---|
| **M1 · Modul engine** — **SELESAI 2026-10-31** | `git mv` paket §4 keluar `internal/` jadi `muara.bi/engine` (satu commit, web ganti import); keluarkan istilah tenant dari API engine (scope opaque, `KeyProvider`); kontrak §7.3 & §7.4; `max_memory` jadi config | build & tes hijau di kedua konsumen dari **satu salinan**; satu tes membuktikan build tanpa CGO gagal cepat. *(Dibuktikan di sisi web; sisi konsumen kedua baru ada di M0. `make image-core` memastikan image web build dari engine lokal, bukan salinan.)* |
| **M0 · Spike platform** ← sekarang | Skeleton Wails v3 + CGO + DuckDB asli + extension offline di **Windows, macOS, Linux**; CI per-OS | `SELECT` nyata tampil di grid di Windows; `LOAD` extension saat jaringan dimatikan; jawab 3 pertanyaan Windows (§11.1); matikan CGO → aplikasi menolak start |
| **M2 · Bersihkan blocker** | Sync schema semua driver (#2), schema terkualifikasi + PK/FK (#3), penanda pemotongan sampai UI (#4), sumber file CSV/Parquet/JSON | tree menampilkan kolom Mongo & Postgres berdampingan; query lintas dua alias jalan; pemotongan selalu ditandai |
| **M3 · Bundle sebagai storage** | Format `.muara` + `projects.json`, save atomik, View kelas satu, **path relatif** (§5.5), `secrets.enc` argon2id | save → buka ulang mempertahankan View utuh; grep isi bundle = nol string kredensial |
| **M4 · Round-trip** | FR-10 + FR-11 termasuk pre-flight, remap alias, remap path | lulus skenario §13 sebagai **tes regresi otomatis**, bukan checklist manual |
| **M5 · Jalur pemula** | FR-4 builder, FR-5 joins, FR-6 template, FR-7 inspector, FR-9 proyek; port modul TS §4 | orang yang tidak bisa SQL membuat table view yang benar tanpa membuka editor SQL |
| **M6 · Kanvas & rilis** | FR-8 dashboard, FR-12 ekspor, FR-13 alert, FR-15 settings, keychain, dark mode, signing + auto-update | installer bertanda tangan jalan di Windows & macOS tanpa peringatan OS |

### 11.1 Tiga pertanyaan Windows yang wajib dijawab di M0

DuckDB memuat driver ATTACH (`postgres_scanner`, `mysql_scanner`, `sqlite_scanner`) sebagai extension yang normalnya
diunduh saat pertama dipakai. Produk ini offline, jadi extension ikut ter-bundle dan `extension_directory` diarahkan
ke folder aplikasi — sudah terverifikasi di web untuk **linux (amd64/arm64) dan darwin/arm64** lewat
`cmd/duckdb-bundle` + probe `--network none`. Perlu diperjelas (spike M0 2026-11-01): probe itu membuktikan
**berkas yang ada** termuat tanpa jaringan, bukan bahwa yang **tidak ada** tidak diunduh — selama ini
DuckDB justru mengunduh extension yang hilang ke folder bundle, dan engine sekarang sudah menutup celah itu
(`INSTALL` dilompati + autoinstall dimatikan saat `extension_directory` terisi). Yang belum pernah diverifikasi sama sekali:

1. Apakah `go-duckdb` v1.8.5 menyediakan pustaka statis untuk `windows/amd64`, dan ABI-nya cocok dengan mingw-w64?
2. Web mengandalkan `-extldflags=-Wl,-E` agar ekstensi yang di-`dlopen` melihat simbol DuckDB — itu flag **ELF/Unix**.
   Bagaimana pemuatan extension pada biner Windows yang di-link statis?
3. Apakah `_scanner` tersedia sebagai berkas `windows_amd64`?

Bundel extension **harus** dihasilkan di CI per-OS (namanya per-platform), tidak boleh di-commit dari satu mesin.
Kalau salah satu jawaban "tidak", titik putarnya di sini: Tauri + sidecar Go, atau macOS-only dulu — bukan di M4.

## 12. Spesifikasi UI

**Bahasa desain.** Kartu airy bersudut membulat, icon rail kiri tipis dengan indikator aktif bulat, topbar dengan
search pill + chip profil + avatar, whitespace luas, bayangan halus dua lapis, tipografi kecil. Base warna **biru**
(bukan monokrom) dan **dark mode**.

**Token.** Light: bg `#f4f6fb`, surface `#ffffff`, surface2 `#f8fafd`, border `#e6ebf4`, text `#16233a`, text2
`#5b6b85`, text3 `#93a1b8`, accent `#2563eb`, accentText `#1d4fd7`, accentSoft `#e8f0fe`, ok `#16a34a`, warn
`#b45309`, err `#dc2626`. Dark: bg `#0b1220`, surface `#111a2c`, surface2 `#0f1728`, border `#1e2a41`, text
`#e6ecf7`, text2 `#93a3bd`, text3 `#64748f`, accent `#3b82f6`, accentText `#8ab4ff`, accentSoft `#14243f`, ok
`#34d399`, warn `#fbbf24`, err `#f87171`. Radius kartu 16px, kontrol 10px, pill 999px.

**Shell.** Rail kiri 56px (logo + Data Sources, Views, Dashboards, Alerts, Settings, ⇄ Export/Import) · topbar 52px
(breadcrumb termasuk nama proyek, search pill, chip Profil, toggle tema, avatar) · body = tree kiri + editor bertab
tengah + Inspector kanan (collapsible) + result grid/log bawah (resizable).

**Layar** (semua sudah terilustrasi di `docs/wireframe.html`): S0 Awal (tanpa login; buka/buat/import + recent
beserta status "perlu tindakan"/"terkunci") · S1 Connect (driver + form + alias dengan help inline + **pemilih
transport langsung/VPN/bastion + status forwarder** + `sslmode`/`rootcert` + uji koneksi +
sync per-tabel dengan penanda gagal) · S2 Query (tree + editor + **peringatan pemotongan** + result grid + inspector)
· S3 Builder (stepper + chip field + preview + ringkasan + SQL read-only + hint `UNNEST` untuk array Mongo) ·
**S4a Proyek & daftar dashboard** · S4 Dashboard (filter global + grid + Present, dengan nama proyek di breadcrumb)
· S5 Export/Import (cakupan + dependensi transitif + tingkat koneksi + path file ‖ laporan pre-flight) ·
S6 Suggested joins · S7 Inspector chart · S8 Settings (Profil + status keychain, Performa, AI dengan banner
metadata-only).

**Komponen.** IconRail, TopBar, ProjectSwitcher, ProjectGroupHeader, SchemaTree, SqlEditor, ResultGrid
(+StatusBar peringatan), TransportPicker (langsung/VPN/bastion), ForwarderStatus (`127.0.0.1:5432 → …`),
RootcertPicker, BuilderStepper, FieldChips, FilterRow, SuggestedJoinList, CardGrid, KpiCard, ChartCard,
InspectorColumns, ExportScopePicker, DependencyList, ConnectionTierPicker, ImportPreflightReport, TemplateGallery,
Dialog, Tag(ok/warn/err), EmptyState.

## 13. Skenario penerimaan round-trip (definition of done)

Satu-satunya tes yang membuktikan janji utama produk. Dijalankan apa adanya di dua mesin, dan menjadi tes regresi
otomatis di M4.

1. **PC A:** connect `sales` (Postgres) + `events` (MongoDB) + `targets` (CSV di `data/targets.csv`); buat 1
   semantic model; 3 View (table, line, kpi) di berkas, dipin ke Project "Retail ID"; 1 dashboard berisi
   ketiganya. Export → `regional.muara`.
2. **Inspeksi berkas:** unzip → tidak ada satu pun string kredensial (kecuali opt-in passphrase: hanya blob + salt).
   Host/port/user **boleh** ada bila toggle §5.4 nyala. Path file tercatat **relatif**.
3. **PC B** (host, port, user, password berbeda; CSV ada di folder lain; semantic model belum ada): import →
   pre-flight **wajib** menampilkan bentrok alias, kolom yang tidak resolve, dan path `data/targets.csv` yang perlu ditunjuk ulang.
4. User me-remap alias ke koneksi lokal, mengisi password satu kali, menunjuk folder CSV baru, lalu menerapkan.
5. **Hasil:** ketiga View jalan **tanpa satu karakter SQL diubah**; dashboard tampil; View yang menunjuk tabel yang
   benar-benar tidak ada ditandai **merah**, bukan diam-diam nol baris; setiap hasil terpotong diberi penanda.
6. Export lagi dari PC B → import balik ke PC A → tidak ada View yang hilang atau berubah bentuk.
7. **Varian berbagi:** export dengan cakupan "hanya View & Dashboard" (tanpa koneksi) → di PC C pre-flight meminta
   alias `sales` dibuat lebih dulu, dan setelah diikat, View tetap jalan.
8. **Varian View lintas proyek:** pin View `kpi` ke Project "Klien Lain" juga → export cakupan Project "Retail ID"
   hanya sekali, import di PC D **dua kali** (bundle yang sama) → tetap **satu** View `kpi` di berkas, tidak ada
   duplikat dan tidak ada prompt timpa; keduanya project menampilkan View yang sama.

## 14. Risiko & mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Wails v3 baru stabil | bug framework di jalur kritis | M0 khusus; kunci versi; baca `LICENSE` di tag; fallback v2/Tauri-sidecar |
| CGO mati diam-diam di build rilis | produk menampilkan angka palsu | kontrak §7.4 + tes build non-CGO gagal |
| Extension DuckDB butuh internet / tidak ada untuk Windows | produk offline tidak bisa connect sama sekali | bundel per-OS di CI + verifikasi M0 dengan jaringan dimatikan |
| DuckDB menarik data jarak jauh ke laptop | lambat & boros RAM | cache lokal + peringatan join lintas sumber + FR-12 ekspor tanpa LIMIT + `max_memory` config |
| Cap 10.000 dokumen Mongo | angka salah yang tampak benar | kontrak §7.3; naikkan batas dengan streaming, bukan blob |
| Save seluruh berkas tiap perubahan | lambat/korup di bundle besar | tulis atomik tmp→rename; ukur batas di M3 |
| Kode baru melupakan §7 | export/import mati senyap | §13 jadi tes otomatis |
| Path file absolut di bundle | import di mesin lain rusak | §5.5, diuji di langkah 3–5 §13 |
| Code signing & notarization | instalasi ditahan OS | putuskan sebelum M6 (biaya sertifikat & waktu, bukan detail) |
| Dua produk berbagi satu engine | perubahan engine merusak salah satu | §7 kontrak + tes kedua konsumen wajib hijau; tidak ada salinan engine |
| Sumber data di jaringan privat (VPN/bastion) | "koneksi sukses" tapi semua query gagal | §5.7: transport dipilih eksplisit, Test Connection memakai jalur yang sama, bastion jadi forwarder lokal, pre-flight membedakan tiga sebab |

## 15. Pertanyaan terbuka

1. ~~Apakah satu Project boleh memakai View dari Project lain?~~ **Dijawab 2026-10-31: ya** — View tidak punya
   `project_id`, project hanya mempin referensi (§5.2, diuji di §13 langkah 8).
2. Perlu warna/label project di rail, atau cukup di daftar dashboard?
3. Batas ukuran & jumlah file per bundle — diukur di M3, bukan ditebak sekarang.
4. Mode Present butuh ekspor PDF/snapshot?
5. Impor `.xlsx` sebagai sumber: extension DuckDB vs parse Go-side — ditunda, tapi perlu keputusan sebelum v1.x.
6. ~~Batas memori default per sesi untuk laptop 8 GB/16 GB (FR-15)~~ **Dijawab 2026-11-01: `min(4GiB, RAM/4)`,**
   jadi 2 GiB di laptop 8 GB dan 4 GiB di 16 GB ke atas. Empat angka yang mendasarinya, semuanya terukur hari ini:
   - **kalau tidak di-SET, DuckDB mengambil 80% RAM sendiri** (terukur `6.3 GiB` di host 8 GB) — terlalu rakus untuk
     aplikasi yang hidup berdampingan dengan browser, jadi angkanya harus eksplisit, bukan warisan default;
   - budget lebih kecil tidak memotong hasil: DuckDB **spill ke `temp_directory`**, hanya lebih lambat — jadi 4 GiB
     sebagai batas atas tidak mengorbankan kebenaran, hanya kecepatan;
   - **label UI dan nilai yang dikirim harus sama.** DuckDB menerima unit biner (`2GiB` = tepat 2.147.483.648 byte),
     sedangkan `2GB` desimal diam-diam 1,86 GiB. Slider FR-15 karena itu memakai **GiB** dan meneruskan string-nya
     apa adanya — engine menerima ini sejak 2026-11-01, sebelumnya justru ditolak. `duckdb.DefaultMaxMemory`
     sekalian dipindah ke `2GiB` atas keputusan user hari itu, supaya web (`DUCKDB_MAX_MEMORY`) dan desktop
     membaca angka yang sama dengan arti yang sama;
   - puncak = budget × jumlah sesi paralel. Desktop single-user mulai dengan **satu** sesi, jadi default aman; begitu
     FR-15 membuka "kueri paralel = 2", default harus diturunkan atau dihitung ulang.
   Yang **tidak** diputuskan di sini: cara membaca RAM total di tiga OS. Itu pertanyaan spike M0 — jangan tambah
   dependensi sebelum dibuktikan; selama belum ada, fallback-nya `2GiB`.

## 16. Cara melanjutkan di mesin/model baru

Baca tiga berkas ini saja — dokumen ini sengaja self-contained:

1. `BRD.md` (dokumen ini) — produk & keputusan
2. [PLAN.md](PLAN.md) — urutan eksekusi + exit criteria
3. [PROGRESS.md](PROGRESS.md) — titik terakhir & fakta terverifikasi

Mulai dari **M0** (spike platform) — **M1 sudah selesai 2026-10-31**, jangan diulang; statusnya ada di
PROGRESS §3. Bukan UI dulu. Detail koreksi terhadap kode lama (dengan file:line) ada di
[docs/brd-review.md](docs/brd-review.md); dasar keputusan modul engine ada di
[docs/adr/0001-engine-module.md](docs/adr/0001-engine-module.md).
