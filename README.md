# image server 

Simple image server to preprocess and store images (in s3-compatible storages) and to fetch that images with resizing
them on the fly. Also, it can generate social share images.

So it is logical evolution of go-resizer to encapsulate all image-related processing 
in one service.

Resizer-part is almost drop-in replacement for `php-resizer` (so it's reimplements all strange things as well).

Note: it's basically my first time code in golang, so, code quality is not good :)

### Service setup

Copy and edit `imgserv.service` to `/etc/systemd/system/imgserv.service`
Install it with `sudo systemctl enable imgserv --now`

### Disk cache policy

The S3 original cache at `storage.cache_path` maintains itself and needs no
size or TTL configuration:

- positive entries expire after six hours without a cache hit;
- cached `404` responses expire after 30 minutes and hits do not extend them;
- temporary cache files expire after one hour;
- 1 GiB is a soft ceiling; when cleanup observes usage above it,
  least-recently-used entries are removed until usage is at or below 75
  percent, so the cache can briefly exceed the ceiling between cleanup runs;
- cleanup runs on startup, every 30 minutes, and after each 64 MiB of cache
  growth.

Positive cache hits update file modification time, so the size cleanup keeps
recently used originals. Cleanup uses atomic cache-file renames and can safely
run while images are being served.

