# muara.bi Desktop

Versi **offline, satu pengguna** dari muara.bi — dijual sebagai produk terpisah dari
[muara.bi-service](../muara.bi-service) (BI self-hosted untuk tim).

Analogi posisinya: DataGrip untuk database, tapi untuk analitik dan BI. Aplikasi desktop yang
memanggil sumber data jarak jauh (Postgres, MySQL, MongoDB, file lokal) dari laptop user, dengan
DuckDB sebagai mesin query dan AI Co-Pilot yang hanya membaca metadata.

Status: **pra-kode.** Keputusan arsitektur sudah diambil (2026-10-30); modul engine belum diekstraksi,
skeleton Wails belum dibuat. Eksekusi ada di [PLAN.md](PLAN.md), titik lanjut ada di [PROGRESS.md](PROGRESS.md).

## Batas dengan muara.bi-service

| | muara.bi-service (web/tim) | muara.bi Desktop (offline) |
|---|---|---|
| Pembeli | tim, butuh governance | analis / konsultan / data scientist |
| Proses | selalu hidup (api + worker + scheduler) | hanya saat aplikasi terbuka |
| Multi-user | workspace, role, RLS, share link, embed | satu operator, satu workspace |
| Infra | Postgres metadata, Redis, Asynq | tidak ada; metadata dan cache lokal |
| Kredensial | terenkripsi di server user | terenkripsi di laptop user |
| Privasi AI | DDL-only, sama | DDL-only, plus data tidak pernah keluar laptop |
| Distribution | `docker compose` | DMG/MSI bertanda tangan, auto-update |

Yang **tidak** ada di versi Desktop dan tidak akan pernah dibuat: halaman share anonim, embed
ber-signature, RLS antar-user, alert/digest yang jalan saat aplikasi tertutup, dan kolaborasi
real-time. Fitur-fitur itu adalah alasan orang membeli versi web; memindahkannya ke desktop hanya
akan membuat dua-duanya lebih lemah.

## Kendala teknis yang sudah diverifikasi

Engine lama (`muara.bi/core`) tidak bisa diimpor dari modul Go lain. Diverifikasi 2026-10-19 dari
modul percobaan dengan `replace muara.bi/core => ../muara.bi-service/backend`:

```
main.go:3:8: use of internal package muara.bi/core/internal/duckdb not allowed
```

Seluruh DuckDB bridge, service, dan guardrail ada di bawah `internal/`; hanya `pkg/`
(`customerror`, `httputil`, `logging`, `envparser`, `telemetry`, `container`) yang bersifat publik.
Keputusannya: engine dikeluarkan dari `internal/` menjadi modul Go sendiri lewat `git mv` (satu
salinan, dua konsumen, tanpa fork), dan itu dikerjakan **sebelum** spike platform. Dasar dan opsi yang
ditolak ada di [docs/adr/0001-engine-module.md](docs/adr/0001-engine-module.md).

Frontend desktop bukan tulisan dari nol: modul visual Phase 4 (`lib/echarts-options.ts`, `lib/color.ts`,
`lib/table-theme.ts`, grid, inspector, datapabel) adalah TypeScript murni dan diport apa adanya; yang
diganti hanya lapisan transport ke binding Wails.

## Cara melanjutkan

Baca [BRD.md](BRD.md) (produk & keputusan, revisi v2), lalu [PLAN.md](PLAN.md) (urutan eksekusi +
exit criteria), lalu [PROGRESS.md](PROGRESS.md) (titik terakhir). Wireframe:
[docs/wireframe.html](docs/wireframe.html). Konvensi kerja sama dengan repo induk: dokumen ini adalah
artefak handoff antar-sesi, bukan riwayat chat.
