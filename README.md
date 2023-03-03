# go-resizer
### Drop-in (almost) replacement for [php-resizer](https://github.com/levmv/php-resizer).

Somewhat sloppy and crude Go implementation of php-resizer. Support all usable features of php version (and for legacy reasons it's reimplements all strange things as well).


TODO:
- fix multiple watermarks and wm position feature
- text processing and filters
- adequate caching


### setup

Copy and edit `go-resizer.service` to `/etc/systemd/system/go-resizer.service`

Install it with `sudo systemctl enable go-resizer --now`



