onewaymirror
============
Onewaymirror reflects anything sent to it right back to the sender.

It is useful to for testing client-server programs.  Eventually, it will have the option of recording the whole session.

```
  -addr=":23": [Address and] port on which to listen.
  -banner="Connection proxied by onewaymirror.": A banner to send to connecting clients.  This may be set to the empty string (-banner="") for no banner.  A newline will be appended to the banner after sending.
  -buflen=1024: Read buffer size.
  -no4=false: Disable IPv4.
  -no6=false: Disable IPv6.
```
Typical usage:
```bash
Test an IRC server:
onewaymirror -addr=6667 -banner="IRC Test"

Test a telnet client:
onewaymirror -banner=""
```
