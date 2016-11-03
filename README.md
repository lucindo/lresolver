# lresolver

[![Build Status](https://drone.io/github.com/lucindo/lresolver/status.png)](https://drone.io/github.com/lucindo/lresolver/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/lucindo/lresolver)](https://goreportcard.com/report/github.com/lucindo/lresolver)
[![MIT Licence](https://badges.frapsoft.com/os/mit/mit.png?v=103)](https://opensource.org/licenses/mit-license.php)

`lresolver` is a simple local DNS resolver. It try to solve a very specific problem when you need to deal with more than one internal DNS server.

Some features:

- Load balancing using round-robin
- Try to resolve on all servers (in parallel, if not found on first attempt)
- No limit on the number of DNS servers
- In-memory cache

## Credits

This projects uses the great DNS library [miekg/dns](https://github.com/miekg/dns) by Miek Gieben. The first version of this code was based on [StalkR/dns-reverse-proxy](https://github.com/StalkR/dns-reverse-proxy).

## Install & Config

Download the static binary here: [lresolver/releases](https://github.com/lucindo/lresolver/releases).

To complie from sources (you need Go installed):

```
go get github.com/lucindo/lresolver
```

### Configuration

The supported formats for configuration are: YAML, JSON, TOML and HCL. On starting up `lresolver` will try to find the file `lresolver.{yml,yaml,json,toml,hcl}` in `/etc/lresolver/` or in the current directory. You can also specify the configuration file with the `-config` flag.

Configuration directives:

| Directive       | Required | Default | Description                                 |
| ----------------|:--------:|:-------:|---------------------------------------------|
|`bind`           |Yes       |-        | Address to bind the server, e.g `127.0.0.1` |
|`nameservers`    |Yes       |-        | List of DNS servers                         |
|`cache`          |No        |`true`   | Turn on/off internal caching                |
|`negative_cache` |No        |`true`   | Cache non-NOERROR responses                 |
|`max_cache_ttl`  |No        |`300`    | Internal cache TTL in seconds               |
|`tcp`            |No        |`true`   | Listen to TCP as well                       |

Sample Configuration:

```yaml
bind: 127.0.0.1
cache: true
negative_cache: true
max_cache_ttl: 300
tcp: true
nameservers:
- 8.8.8.8
- 8.8.4.4
```

### Running

Point `/etc/resolv.conf` to `127.0.0.1`:

```
# /etc/resolv.conf
nameserver 127.0.0.1
```

You can leave your other `resolv.conf` directives (`search`, `domain`, etc) unchanged.

In my production systems I put the `lresolver.yml` in `/etc/lresolver/` directory and run the server this way:

```
/sbin/lresolver -log_dir /var/log/lresolver/
```

You can clear the cache by sending an `USR1` signal to the running server.

## To Do

- [ ] Update expired entries in background
- [ ] Packages for popular Linux distros (deb and rpm)
- [ ] Statistics on requests and nameservers
- [ ] Option to replace round-robin to dynamic weighted round-robin based on server's response time
- [ ] External configuration on `etcd`
- [ ] Suffix-based request routing
- [ ] Automatic configuration reload

## Contributing

1. Fork it
2. Create your feature branch: `git checkout -b my-awesome-new-feature`
3. Commit your changes: `git commit -m 'Add some awesome feature'`
4. Push to the branch: `git push origin my-awesome-new-feature`
5. Submit a pull request
