# AGENTS.md

Go CCTV recorder + Vue 3 UI (Vite+ toolchain, pnpm). Go is stdlib-only.

## Build order (matters)

```
pnpm --dir ui install && pnpm --dir ui build && go build .
```

- UI: `ui/` is a Vite+ (`vite-plus`, `vp` CLI) + Vue 3 + Tailwind v4 + `hls.js` app. Dev: `pnpm --dir ui dev` (proxies `/api /videos /live` → `:8181`). `vite-plus` is pinned to exactly `1.1.0` — do not bump to latest blindly: pnpm's `minimumReleaseAge` supply-chain policy rejects packages published within ~24h and breaks `pnpm install`/`build`.
- `ui/dist/` is gitignored but **required at compile time**: `webui.go` has `//go:embed ui/dist`, so `go build` fails with `no matching files found` on a fresh clone without `pnpm --dir ui build`.
- A partial `dist/` (no `index.html`) panics at startup via `cctv.SetWebFS` (`cctv/web.go`) instead of serving a blank page.
- Must run the binary from repo root (default `config.json` / `storage/` resolve via `os.Getwd()`).
- Checks: `go vet ./...`. No Go tests. UI check: `pnpm --dir ui check` (`vp check`).
- Requires `ffmpeg` on `PATH` or `ffmpeg_bin` in config, else startup panics.

## Config (`cctv/structs.go`, `sample.config.json`)

- Keys: `serve_address` (default `:8181`), `storage_dir` (default `<cwd>/storage`), `retention_days` (default 2), `rtsp_transport` (default `tcp`), per-camera `name rtsp_url retention_days rtsp_transport color`.
- `config.json` is gitignored. First run with a missing file writes defaults then returns an error → startup panics; second run succeeds. Copy `sample.config.json` first.

## Backend (`cctv/`)

- Entry `main.go` → `cctv.Start()` (`cctv/cctv.go`): per-camera `recordLoop`, `storageCleaner`, `reapLiveLoop`, `getMux()`.
- Recording: 6h blocks (`00-06/06-12/12-18/18-24` via `getBlockStart`), one ffmpeg per camera: `-rtsp_transport <t> -i <url> -t <remaining> -c:v copy -an -f hls -hls_time 10 -hls_list_size 0`. Layout `storage/<cam>/<YYYY-MM-DD>/<HH-HH>/<timestamp>.m3u8 + <timestamp>.%04d.ts`. Clean exit (`-t` expiry) rolls to the next block; errors back off 1/5/10/30/60m.
- Live (on demand, 10s segments, not retained): `cctv/live.go` starts one ffmpeg per viewed camera (`-hls_time 10 -hls_list_size 6 -hls_flags delete_segments+omit_endlist`) into `storage/live/<cam>/`, reaped after 5m idle. Trigger: `POST /api/live/:cam/start` or first `GET /live/:cam/...`.
- Coverage: `cctv/coverage.go` parses `.m3u8` (`EXTINF` + on-disk `.ts` check) into 10s-slot bitmaps (2160 slots/block). `parseM3u8` is implemented — do not re-add the old `panic("Unimplemented")` stub.
- Routes: `GET /` → embedded SPA (with `index.html` fallback; entry served directly because `http.FileServer` redirects `*/index.html` → `./`), `GET /videos/` → `storage_dir`, `GET /live/:cam/...`, `GET /api/cameras|dates|blocks|timeline`, `POST /api/live/:cam/start`. Legacy `GET /api/camera/...` kept for compat.

## UI (`ui/src/`)

- `App.vue`: Live / Review tabs. Live = auto-fit grid (`gridCols` in `api.ts`: 1→1, 2–4→2, 5–9→3, else 4), click tile → fullscreen + Back. Review = calendar modal (highlighted = footage present) → 4-block day view → review multiview + per-camera `TimelineScrub` (green = footage, grey = gap; click seeks all tiles via playlist lookup + `currentTime`).
- Camera dropdowns: live = free multi-select; review = pool scope + in-view list where no-footage cameras are disabled.
- `CameraTile.vue` wraps `hls.js` (falls back to native HLS); review tiles start paused (`autoplay=false`) and are driven by scrub seeks.
