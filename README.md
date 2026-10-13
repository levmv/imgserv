# image server 

Simple image server to preprocess and store images (in s3-compatible storages) and to fetch that images with resizing
them on the fly. Also, it can generate social share images.

So it is logical evolution of go-resizer to encapsulate all image-related processing 
in one service.

Resizer-part is almost drop-in replacement for `php-resizer` (so it's reimplements all strange things as well).

Note: it's basically my first time code in golang, so, code quality is not good :)

### Development and usage

Requires Go (see [go.mod](go.mod)), libvips headers and pkg-config.
`make test`; `make build`; `imgserv server -config config.json`.
`make build` embeds the Git tag (without `v`) or commit, plus `-dirty` for
uncommitted changes; plain `go build` reports `dev`.
CLI overrides: `imgserv server -h`. Statistics: `imgserv stat -config config.json`.
JSON fields and defaults: [config/config.go](config/config.go).
Relative paths resolve from the working directory.

### Behavior

- Image URLs are `/<params>/<key>`: resize, crop, watermarks and presets are
  defined in [params/params.go](params/params.go). Output defaults to JPEG/WebP
  negotiation; proxy caches must respect `Vary: Accept`.
- `/upload` accepts raw image bytes; `/upload_file` reads a server-side file.
  Both normalize orientation, optionally shrink, strip metadata and store JPEG.
  `/delete?key=...` removes an original; `/share?key=...&text=...` generates a
  social preview with the configured `sharer` font and logo.
- The HTTP API assumes trusted callers. Image URL signatures do not protect
  upload/delete routes; legacy `st3` only strips a prefix already checked by nginx.
- Storage modes: `s3` (default), `local`, `overlay`. Overlay reads local first,
  falls back to S3, and keeps all writes/deletes local.
- S3 `endpoint` is a service URL without the bucket; `region` controls signing.
  Without an endpoint, `ru-central1` selects Yandex; other regions use AWS.
  Credentials use the `[default]` profile in an AWS shared credentials file.
- S3 caches originals and misses locally; cleanup is automatic.
  Policy constants live in [storage/cache_janitor.go](storage/cache_janitor.go).
