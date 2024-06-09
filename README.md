# Loadshift

A simple reverse proxy tool to select the fastest backend among multiple hosts.

## Usage
```bash
$ loadshift -help
Usage of loadshift:
  -check-interval duration
        Interval to check host latency (default 10s)
  -hosts string
        Comma-separated list of host addresses (default "host1.example.com:80,host2.example.com:80")
  -port int
        Port to listen on (default 8080)
```