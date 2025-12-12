# SurfBoard

The surfboard program is a personal-use Telegram bot for installing and managing software on a Keenetic router with pre-installed Entware.


**Roadmap:**

- [x] Import `vless://`, `trojan://`, `ss://` links and add them to the X-wave balancer and Benchmark.
- [x] VPN connectivity check using the leastLoad method (X-wave, Benchmark).
- [x] Manual VPN server selection, overriding the balancer (X-wave, Benchmark).
- [x] Install with version selection / update / uninstall for X-wave, S-wave, Surfboard, youtubeUnblock, nfqws.
- [x] Download/Replace configuration files: `system-default.json`, `routing-balancers.json`, `{x-wave,s-wave}config/config.json`.
- [x] Add/update VPNs via subscription (X-wave, S-wave).
- [ ] Additional Xray (Benchmark) instance launch to check AI service availability via VPN.
- [x] Quick-add websites to rules for testing without saving to config (Add routing rules).
- [ ] Multi-language support.
- [ ] S-wave: route all non-Russia traffic through VPN; route WebRTC, STUN traffic (if AI services fail to work).