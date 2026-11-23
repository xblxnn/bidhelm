# Third-party notices

BidHelm code is all rights reserved. The following components and data retain their own licenses.

## Go runtime and dependencies

Dependency versions are pinned in `go.mod` and `go.sum`. License and notice texts are included under `third_party/` and copied into `/app/licenses` in the container.

| Component | Version | License files |
| --- | --- | --- |
| github.com/dustin/go-humanize | v1.1.0 | [LICENSE](third_party/github.com/dustin/go-humanize/LICENSE) |
| github.com/google/uuid | v1.6.0 | [LICENSE](third_party/github.com/google/uuid/LICENSE) |
| github.com/hashicorp/golang-lru | v1.0.2 | [LICENSE](third_party/github.com/hashicorp/golang-lru/LICENSE) |
| github.com/mattn/go-isatty | v0.0.24 | [LICENSE](third_party/github.com/mattn/go-isatty/LICENSE) |
| github.com/ncruces/go-strftime | v1.1.0 | [LICENSE](third_party/github.com/ncruces/go-strftime/LICENSE) |
| github.com/oschwald/maxminddb-golang | v1.13.1 | [LICENSE](third_party/github.com/oschwald/maxminddb-golang/LICENSE) |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | [LICENSE](third_party/github.com/remyoudompheng/bigfft/LICENSE) |
| github.com/ua-parser/uap-go | v0.0.0-20261028195011-4cb30e998c4c | [LICENSE](third_party/github.com/ua-parser/uap-go/LICENSE), [uaparser/LICENSE.md](third_party/github.com/ua-parser/uap-go/uaparser/LICENSE.md) |
| go.yaml.in/yaml/v3 | v3.0.5 | [LICENSE](third_party/go.yaml.in/yaml/v3/LICENSE), [NOTICE](third_party/go.yaml.in/yaml/v3/NOTICE) |
| golang.org/x/crypto | v0.58.0 | [LICENSE](third_party/golang.org/x/crypto/LICENSE) |
| golang.org/x/sys | v0.49.0 | [LICENSE](third_party/golang.org/x/sys/LICENSE) |
| modernc.org/libc | v1.80.1 | [LICENSE](third_party/modernc.org/libc/LICENSE), [LICENSE-3RD-PARTY.md](third_party/modernc.org/libc/LICENSE-3RD-PARTY.md) |
| modernc.org/mathutil | v1.7.1 | [LICENSE](third_party/modernc.org/mathutil/LICENSE) |
| modernc.org/memory | v1.12.1 | [LICENSE](third_party/modernc.org/memory/LICENSE), [LICENSE-MMAP-GO](third_party/modernc.org/memory/LICENSE-MMAP-GO), [LICENSE-LOGO](third_party/modernc.org/memory/LICENSE-LOGO), [LICENSE-GO](third_party/modernc.org/memory/LICENSE-GO) |
| modernc.org/sqlite | v1.62.0 | [LICENSE](third_party/modernc.org/sqlite/LICENSE), [LICENSE-SQLITE](third_party/modernc.org/sqlite/LICENSE-SQLITE), [LICENSE-3RD-PARTY.md](third_party/modernc.org/sqlite/LICENSE-3RD-PARTY.md), [LICENSE-SQLITE_VEC](third_party/modernc.org/sqlite/LICENSE-SQLITE_VEC) |

Go runtime: [BSD-style license](third_party/go/LICENSE).

## Browser assets

- Bootstrap: [MIT license](internal/backoffice/assets/BOOTSTRAP-LICENSE).
- HTMX: [BSD 2-Clause license](internal/backoffice/assets/HTMX-LICENSE).
- Inter Tight and the outlined BidHelm wordmark: [SIL Open Font License](internal/backoffice/assets/fonts/INTER-TIGHT-LICENSE).
- IBM Plex Mono: [SIL Open Font License](internal/backoffice/assets/fonts/IBM-PLEX-MONO-LICENSE).

## Country data

IP Geolocation by [DB-IP](https://db-ip.com). The bundled November 2026 DB-IP Country Lite database is licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). See [data provenance](assets/geo/NOTICE.md) and the [license text](assets/geo/CC-BY-4.0.txt). The data content is unmodified.

The synthetic MaxMind fixture under `internal/runtime/testdata` is used only by tests and is covered by its accompanying [Apache 2.0 license](internal/runtime/testdata/LICENSE-APACHE). It is not used as production country data.

## Container packages

The runtime image is based on Debian. Package copyrights and licenses are retained under `/usr/share/doc` in the image; Debian package versions are determined by its build.
