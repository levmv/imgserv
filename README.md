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



