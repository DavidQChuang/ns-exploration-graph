# Dependency and license inventory

Audit scope: repository `HEAD 93e054b` (2026-10-27). This is an engineering inventory, not legal advice or legal review. It identifies the module graph reported by `go list -m -json all` and the license texts present for those exact versions in the local Go module cache.

## Resolution limits

`go list -m -json all` completed offline when run with `GOMODCACHE` set to the repository's `.local/gomodcache` and `GOPROXY=off`, using `.local/go/bin/go.exe`. It reported 28 graph modules: 3 direct and 25 indirect. Seven selected module archives were then downloaded with `go mod download module@version` into `.local/gomodcache`; this left `go.mod` and `go.sum` unchanged. The exact selected license files were read and SHA-256 hashed locally for all 28 graph entries. `go.sum` also contains older go.mod-only versions; they are not selected module graph entries and are not included in the count.

The module cache sources were under `.local/gomodcache`, which is ignored developer state and is not included in the tracked-source inventory or intended release contents. License-file hashes below are SHA-256 of the exact local file bytes. Identifiers are a practical reading of the included license text; multi-license modules are marked mixed/dual where their text so states.

## Resolved module graph

| Class | Module and version | License identifier / note | License evidence (relative to module cache) | SHA-256 |
|---|---|---|---|---|
| Indirect | `cloud.google.com/go/compute/metadata v0.3.0` | Apache-2.0 | `cloud.google.com/go/compute/metadata@v0.3.0/LICENSE` | `cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30` |
| Indirect | `github.com/davecgh/go-spew v1.1.1` | ISC | `github.com/davecgh/go-spew@v1.1.1/LICENSE` | `1b93a317849ee09d3d7e4f1d20c2b78ddb230b4becb12d7c224c927b9d470251` |
| Indirect | `github.com/dlclark/regexp2 v1.11.0` | MIT | `github.com/dlclark/regexp2@v1.11.0/LICENSE` | `9be5d04bb4d706914d5bf943710da4afeb42048f7c529902fb57c82762a991a9` |
| Indirect | `github.com/golang-jwt/jwt/v5 v5.3.1` | MIT | `github.com/golang-jwt/jwt/v5@v5.3.1/LICENSE` | `fe26ca41577b9b2b4448050a24b25e5753af66b5d5945d5d36094e7790bfcb2f` |
| Indirect | `github.com/google/go-cmp v0.7.0` | BSD-3-Clause | `github.com/google/go-cmp@v0.7.0/LICENSE` | `17b5d209ba8f9684257ecfcff87df6ceda6194143a8fbd074f29727cff6f0c40` |
| Direct | `github.com/jackc/pgx/v5 v5.9.2` | MIT | `github.com/jackc/pgx/v5@v5.9.2/LICENSE` | `467f95e074fe23079a5623ed652619682692041b8551da27e3c2ddb9659a1507` |
| Direct | `github.com/modelcontextprotocol/go-sdk v1.8.0` | MIT and Apache-2.0 contributions; project text describes a licensing transition and per-contribution terms | `github.com/modelcontextprotocol/go-sdk@v1.8.0/LICENSE` | `af679003d933f045393a6a029f43da113f9ae364eac651d9ae268392985580f5` |
| Direct | `github.com/santhosh-tekuri/jsonschema/v6 v6.0.3` | Apache-2.0 | `github.com/santhosh-tekuri/jsonschema/v6@v6.0.3/LICENSE` | `c8858a5a76440bbca484e134cf7df46385d090dd18b2c58e650f939258802e5b` |
| Indirect | `github.com/google/jsonschema-go v0.4.3` | MIT | `github.com/google/jsonschema-go@v0.4.3/LICENSE` | `2d56c53449691d85d9aea245eb8dac12713e9075d70d5557b82ae1e94805b357` |
| Indirect | `github.com/jackc/pgpassfile v1.0.0` | MIT | `github.com/jackc/pgpassfile@v1.0.0/LICENSE` | `adb1663fda031df8f4344aa68f299fd87d80353e31339406742ded21dae65702` |
| Indirect | `github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761` | MIT | `github.com/jackc/pgservicefile@v0.0.0-20240606120523-5a60cdf6a761/LICENSE` | `fc505773403fe869ed64cc2235cdd13988a427bb7e3a7e7004a3f4b27420f8fc` |
| Indirect | `github.com/jackc/puddle/v2 v2.2.2` | MIT | `github.com/jackc/puddle/v2@v2.2.2/LICENSE` | `2d50e98a4900b4d6457a38d39c1432fdc156fc2f7b365f2e33ec9344acbb0057` |
| Indirect | `github.com/segmentio/asm v1.1.3` | MIT | `github.com/segmentio/asm@v1.1.3/LICENSE` | `e2a78de21d6d8ded2dff0f3189cd32e011630d785da127ebfbc8949012c0947b` |
| Indirect | `github.com/segmentio/encoding v0.5.4` | MIT; a separate nested fuzz directory also has an Apache-2.0 license, not identified here as a module-level selected dependency | `github.com/segmentio/encoding@v0.5.4/LICENSE` | `d6d71a1f7dc6539e371120cc7af6e3257e55ca79634d473211f217b8965b0f16` |
| Indirect | `github.com/yosida95/uritemplate/v3 v3.0.2` | BSD-3-Clause style text | `github.com/yosida95/uritemplate/v3@v3.0.2/LICENSE` | `0761aadfb1921103752869ee942d4a71bdd54494697684d4b13dc17ad9781191` |
| Indirect | `golang.org/x/oauth2 v0.35.0` | BSD-style Go Authors license | `golang.org/x/oauth2@v0.35.0/LICENSE` | `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` |
| Indirect | `golang.org/x/sync v0.22.0` | BSD-style Go Authors license | `golang.org/x/sync@v0.22.0/LICENSE` | `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` |
| Indirect | `golang.org/x/sys v0.41.0` | BSD-style Go Authors license | `golang.org/x/sys@v0.41.0/LICENSE` | `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` |
| Indirect | `golang.org/x/text v0.41.0` | BSD-style Go Authors license | `golang.org/x/text@v0.41.0/LICENSE` | `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` |
| Indirect | `golang.org/x/time v0.15.0` | BSD-style Go Authors license | `golang.org/x/time@v0.15.0/LICENSE` | `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` |

| Indirect | `github.com/kr/pretty v0.3.0` | MIT | `github.com/kr/pretty@v0.3.0/License` | `283c28b781bc7487bd7bcaf453cbd811f5ee8d7ce625cfcf8dcacae6ba1f4bb5` |
| Indirect | `github.com/pmezard/go-difflib v1.0.0` | BSD-3-Clause | `github.com/pmezard/go-difflib@v1.0.0/LICENSE` | `2eb550be6801c1ea434feba53bf6d12e7c71c90253e0a9de4a4f46cf88b56477` |
| Indirect | `github.com/stretchr/objx v0.1.0` | MIT | `github.com/stretchr/objx@v0.1.0/LICENSE` | `b2663894033a05fd80261176cd8da1d72546e25842d5c1abcc852ca23b6b61b0` |
| Indirect | `github.com/stretchr/testify v1.11.1` | MIT | `github.com/stretchr/testify@v1.11.1/LICENSE` | `f8e536c1c7b695810427095dc85f5f80d44ff7c10535e8a9486cf393e2599189` |
| Indirect | `golang.org/x/mod v0.38.0` | BSD-3-Clause | `golang.org/x/mod@v0.38.0/LICENSE` | `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` |
| Indirect | `golang.org/x/tools v0.48.0` | BSD-3-Clause | `golang.org/x/tools@v0.48.0/LICENSE` | `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` |
| Indirect | `gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c` | BSD-3-Clause | `gopkg.in/check.v1@v1.0.0-20201130134442-10cb98267c6c/LICENSE` | `69ce77f2b1c9c608d27f2d749b0e7d0c13960ed3b0dc07b973ac940e362e5d9c` |
| Indirect | `gopkg.in/yaml.v3 v3.0.1` | Mixed MIT and Apache-2.0; module LICENSE identifies MIT-covered ported files and NOTICE carries Apache-2.0 attribution | `gopkg.in/yaml.v3@v3.0.1/LICENSE`; `gopkg.in/yaml.v3@v3.0.1/NOTICE` | LICENSE `d18f6323b71b0b768bb5e9616e36da390fbd39369a81807cca352de4e4e6aa0b`; NOTICE `f6c2dd3a67b576eafb89b80200b8b1627230bf3821a0c14cb99a22ac19107d00` |

The `golang.org/x/*` license files use BSD 3-Clause wording (copyright, source/binary redistribution conditions, non-endorsement clause and disclaimer). File hashes match because the projects ship the same license text. All 28 selected module versions have a local license file represented in this inventory; this remains a source-cache inventory, not an archive-level scanner or legal review.

Reproduce the module graph on this checkout with PowerShell: `$env:GOMODCACHE="$PWD/.local/gomodcache"; $env:GOPROXY='off'; .\.local\go\bin\go.exe list -m -json all`. Reproduce the source inventory with `python scripts/release-inventory.py`; that script intentionally uses `git ls-files` and checksums only tracked working-tree bytes.

## Distribution notes

- The project tree carries Apache-2.0 text in `LICENSE` and project attribution in `NOTICE`. Distribution of a binary or source bundle should include applicable third-party license and notice texts for all modules actually included in that artifact, including the contribution-specific terms in the MCP Go SDK and the mixed license attribution in any included YAML package. This table alone does not establish artifact contents or complete compliance.
- The Go standard library is supplied by the recipient's Go toolchain at build time; no Go toolchain or standard-library source/binary is bundled or intended to ship in this source release. A built Go executable may statically include standard-library code; if distributing such an executable, inspect the actual binary/build provenance and include applicable notices.
- PostgreSQL is a separately provisioned external service/runtime in the documented setup. The project does not bundle PostgreSQL binaries, and the SQL migration is project source. The `.local/` PostgreSQL and Go downloads are developer-only ignored files and must not be copied into a release.
- No blanket compatibility determination is made. Recheck dependency versions, license files, and the actual archive on every release candidate; resolve the incomplete module graph and review all included files before distribution.

