# ADR-0001 — Engine jadi modul Go sendiri, pemisahan bisnis ditunda

Status: **diterima** (keputusan teknis); penetapan entitas pemilik **ditunda** sesuai arahan
Tanggal: 2026-10-30

## Konteks

Desktop dan web akan jadi dua bisnis terpisah. Web multi-tenant; desktop single-user berbasis berkas.
BRD menolak dua opsi yang tampak gampang: menulis ulang engine (membuang semua pengerasan yang sudah
diverifikasi) dan menyalin engine (drift dalam hitungan minggu).

Sisa satu opsi — engine dipakai bersama — hari ini terkunci oleh aturan paket `internal/` Go. Ini
diverifikasi dari modul di luar repo, dengan `replace muara.bi/core => .../backend`:

```
use of internal package muara.bi/core/internal/duckdb not allowed
```

Seluruh DuckDB bridge, guardrail, widget compiler, exporter, cryptohelper, dan semantic layer berada
di `backend/internal/`. Hanya `backend/pkg/` yang bersifat publik.

Keputusan lain yang membingkai ADR ini: prioritas diarahkan ke desktop, dan penetapan siapa pemegang
engine ditunda.

## Keputusan

1. **Satu modul engine, dibuat dengan `git mv`, bukan dengan salinan.** `muara.bi/engine` lahir dari
   `backend/internal/{duckdb,service/widget_builder.go,exporter,helper/cryptohelper,domain/entity,ai/pruner.go}`
   dalam satu commit di repo web, dan web langsung mengganti import-nya. Dengan cara ini tidak pernah
   ada titik waktu di mana kode engine ada dua di filesystem; "repo terpisah" jadi langkah pemisahan
   setelah modul ini stabil, bukan syarat untuk mulai.
2. **Potong berdasarkan pemakaian desktop, bukan berdasarkan estetika paket.** Yang keluar `internal/`
   hanya yang dipakai desktop: sesi DuckDB + guardrail, builder→SQL compiler, semantic layer, streaming
   Mongo, exporter, AES-GCM primitif, dan kontrak `TableSchema`. Yang tetap di backend: auth, workspace/
   members/roles, RLS-as-tenant-policy, share link + signed embed, Asynq + scheduler, Redis cache &
   semaphore, Postgres metadata.
3. **API engine tidak boleh mengenal kata tenant.** Ini syarat supaya dua produk bisa berpisah tanpa
   refactor kedua nanti. Praktisnya: fungsi cache menerima *scope* opaque (bukan `workspace_id` +
   `env_tag`), kredensial dibaca lewat *key provider* (web menyuntik master key dari env, desktop
   menyuntik dari keychain/argon2id), dan `ApplyRLS` diekspos sebagai aturan baris generik tanpa
   vocabulary user.
4. **Rambu agar penundaan tidak berubah jadi duplikasi.** Selama engine dipakai dua produk dari satu
   tempat: (a) tidak ada salinan engine di repo desktop, (b) setiap perubahan engine harus lewat tes
   kedua konsumen — `go build ./... && go test ./...` di web **dan** di desktop, (c) keputusan pemisahan
   formal (versi/tag publik, penetapan pemegang) ditinjau lagi begitu desktop punya rilis pertama atau
   ada pihak luar di salah satu entitas.

## Opsi yang ditolak

- **Menaruh engine di repo desktop dan web mengimpor bolak-balik.** Arah dependensi jadi melingkar:
  web, yang justru tempat engine ini dikeraskan, bergantung pada repo produk lain.
- **Menunda ekstraksi dan menyalin engine ke desktop untuk kecepatan.** Ini membeli satu minggu dan
  membayar dengan dua bug fix untuk setiap temuan — dan BRD sudah memuat dua temuan yang menyakitkan
  (`flattenMap` untuk `primitive.A`/`ObjectID`, klamp `LIMIT` di ekor pernyataan) yang bukti nyata
  biayanya kalau duplikat.
- **Monorepo gabungan untuk kedua produk.** Menyatukan dua bisnis dengan cadence rilis, signing, dan
  basis pengguna yang berbeda; desktop butuh CI per-OS, web butuh CI Docker.

## Konsekuensi

- Ada satu-satunya pekerjaan di produk web dalam rencana ini: mengganti import path. Biayanya dibatasi
  dengan melakukan itu sekali di M1, dan suite web yang hari ini hijau berfungsi sebagai jaring
  pengaman — refactor yang salah potong akan kelihatan sebagai gagal kompilasi, bukan sebagai bug
  produksi.
- Desktop membangun dengan go.work lokal; begitu M1 selesai, engine diberi versi supaya kedua repo
  bisa di-build tanpa direktori saudara.
- Kosa kata tenant yang keluar dari engine (keputusan 3) adalah bagian terbesar dari M1 dan tidak bisa
  dihindari dengan `git mv` semata; kalau diputuskan untuk ditunda, desktop akan mewarisi `wsID`/`envTag`
  di seluruh binding-nya dan membaliknya nanti jauh lebih mahal.
- Penetapan pemegang engine yang tertunda tidak mengubah struktur kode apa pun, tapi menjadi
  prasyarat sebelum ada pihak kedua (non-anda) di salah satu entitas, dan sebelum produk desktop
  mengirim biner yang memuat kode web ke pasar.
